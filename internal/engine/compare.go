package engine

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/bits"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"vdfusion/internal/config"
	"vdfusion/internal/db"
	"vdfusion/internal/media"
	"vdfusion/internal/neural"
)

type ComparisonEngine struct {
	db       *db.Database
	embCache sync.Map
}

func NewComparisonEngine(d ...*db.Database) *ComparisonEngine {
	e := &ComparisonEngine{}
	if len(d) > 0 {
		e.db = d[0]
	}
	return e
}

func (e *ComparisonEngine) SetDatabase(d *db.Database) {
	e.db = d
}

func (e *ComparisonEngine) getOrLoadEmbeddings(r *db.FileRecord) [][]float32 {
	if len(r.NeuralEmbeddings) > 0 {
		return r.NeuralEmbeddings
	}
	if e.db == nil {
		return nil
	}
	if cached, ok := e.embCache.Load(r.ID); ok {
		return cached.([][]float32)
	}
	embs, err := e.db.GetNeuralEmbeddingsByID(r.ID)
	if err != nil || len(embs) == 0 {
		return nil
	}
	e.embCache.Store(r.ID, embs)
	return embs
}

func (e *ComparisonEngine) Compare(ctx context.Context, records []db.FileRecord, ignoredGroups []db.IgnoredGroup, cfg config.Settings, reporter ProgressReporter) []DuplicateGroup {
	if len(records) < 2 {
		return nil
	}

	if ctx.Err() != nil {
		return nil
	}

	ignoredPairs := make(map[string]map[string]bool)
	for _, ig := range ignoredGroups {
		for i := 0; i < len(ig.IdentifierHashes); i++ {
			for j := i + 1; j < len(ig.IdentifierHashes); j++ {
				p1 := ig.IdentifierHashes[i]
				p2 := ig.IdentifierHashes[j]
				if ignoredPairs[p1] == nil {
					ignoredPairs[p1] = make(map[string]bool)
				}
				if ignoredPairs[p2] == nil {
					ignoredPairs[p2] = make(map[string]bool)
				}
				ignoredPairs[p1][p2] = true
				ignoredPairs[p2][p1] = true
			}
		}
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].Duration < records[j].Duration
	})

	parent := make([]int, len(records))
	for i := range parent {
		parent[i] = i
	}

	var find func(int) int
	find = func(i int) int {
		if parent[i] == i {
			return i
		}
		parent[i] = find(parent[i])
		return parent[i]
	}

	union := func(i, j int) {
		rootI := find(i)
		rootJ := find(j)
		if rootI != rootJ {
			parent[rootI] = rootJ
		}
	}

	adj := make(map[int][]int)
	edges := make(chan [2]int, 65536)
	var aggWG sync.WaitGroup
	aggWG.Add(1)
	go func() {
		defer aggWG.Done()
		for {
			select {
			case p, ok := <-edges:
				if !ok {
					return
				}
				i, j := p[0], p[1]
				adj[i] = append(adj[i], j)
				adj[j] = append(adj[j], i)
				union(i, j)
			case <-ctx.Done():
				return
			}
		}
	}()

	workers := cfg.Concurrency
	if workers <= 0 {
		workers = 4
	}

	missingHashes := 0
	for _, r := range records {
		if len(r.PHashV2s) == 0 {
			missingHashes++
		}
	}
	if missingHashes > 0 {
		msg := fmt.Sprintf("Data Health: %d files skipped (missing hashes)", missingHashes)
		log.Printf("ComparisonEngine: %s", msg)
		if reporter != nil {
			reporter.BroadcastLog("warning", msg)
		}
	}

	var totalPairs int64
	right := 0
	for left := 0; left < len(records); left++ {
		maxT := e.getDurationTolerance(records[left].Duration, cfg)
		if right <= left {
			right = left + 1
		}
		for right < len(records) && records[right].Duration-records[left].Duration <= maxT {
			right++
		}
		totalPairs += int64(right - left - 1)
	}

	if totalPairs == 0 {
		// Just report files if no pairs to compare
		totalPairs = int64(len(records))
	}

	var wg sync.WaitGroup
	var processed int64 // We'll count completed pairs for progress
	cstats := make([]counters, workers)

	doneCh := make(chan struct{})
	comparisonStartTime := time.Now()
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if reporter != nil {
					cur := atomic.LoadInt64(&processed)
					elapsed := time.Since(comparisonStartTime).Seconds()
					eta := 0.0
					if elapsed > 0.5 && cur > 0 {
						rate := float64(cur) / elapsed
						eta = float64(totalPairs-cur) / rate
					}
					reporter.BroadcastProgress(int(cur), int(totalPairs), "comparing", "", elapsed, eta)
				}
			case <-doneCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		wi := w
		go func() {
			defer wg.Done()
			local := &cstats[wi]
			for i := wi; i < len(records); i += workers {
				select {
				case <-ctx.Done():
					return
				default:
				}

				r := records[i]
				maxTolerance := e.getDurationTolerance(r.Duration, cfg)

				pairsProcessedInLoop := 0
				for j := i + 1; j < len(records); j++ {
					comp := records[j]
					diffSeconds := comp.Duration - r.Duration
					if diffSeconds > maxTolerance {
						break
					}
					e.comparePair(r, comp, i, j, maxTolerance, ignoredPairs, local, edges, cfg)
					pairsProcessedInLoop++
				}
				atomic.AddInt64(&processed, int64(pairsProcessedInLoop))
			}
		}()
	}
	wg.Wait()
	close(edges)
	aggWG.Wait()
	close(doneCh)

	if ctx.Err() != nil {
		return nil
	}

	totalComp := 0
	totalSkipD := 0
	totalSkipH := 0
	var maxSim float64
	for _, s := range cstats {
		totalComp += s.comp
		totalSkipD += s.skipD
		totalSkipH += s.skipH
		if s.maxSim > maxSim {
			maxSim = s.maxSim
		}
	}
	totalNeuralComp := 0
	totalNeuralGateSkip := 0
	for _, s := range cstats {
		totalNeuralComp += s.neuralComp
		totalNeuralGateSkip += s.neuralGateSkip
	}
	statsMsg := fmt.Sprintf("Comparison Stats: Performed=%d, Neural=%d, NeuralGateSkip=%d, SkipDuration=%d, SkipHash=%d, MaxSimFound=%.1f%%", totalComp, totalNeuralComp, totalNeuralGateSkip, totalSkipD, totalSkipH, maxSim*100.0)
	log.Printf("%s", statsMsg)
	if reporter != nil {
		reporter.BroadcastLog("info", statsMsg)
	}

	clusters := make(map[int][]int)
	for i := range records {
		root := find(i)
		clusters[root] = append(clusters[root], i)
	}

	var results []DuplicateGroup
	for _, cluster := range clusters {
		if len(cluster) < 2 {
			continue
		}

		if len(cluster) > 500 {
			log.Printf("ComparisonEngine: Cap reached for cluster of size %d.", len(cluster))
			cluster = cluster[:500]
		}

		hasIgnored := false
		for idx1 := 0; idx1 < len(cluster); idx1++ {
			for idx2 := idx1 + 1; idx2 < len(cluster); idx2++ {
				i, j := cluster[idx1], cluster[idx2]
				hI, hJ := records[i].GetIdentifierHash(), records[j].GetIdentifierHash()
				if ignoredPairs[hI] != nil && ignoredPairs[hI][hJ] {
					hasIgnored = true
					break
				}
			}
			if hasIgnored {
				break
			}
		}

		if !hasIgnored {
			results = append(results, e.buildGroup(cluster, records, cfg))
		} else {
			covered := make(map[int]bool)
			for _, start := range cluster {
				if covered[start] {
					continue
				}
				group := []int{start}
				for _, other := range cluster {
					if start == other {
						continue
					}
					if !slices.Contains(adj[start], other) {
						continue
					}

					conflict := false
					hOther := records[other].GetIdentifierHash()
					for _, inG := range group {
						if ignoredPairs[hOther] != nil && ignoredPairs[hOther][records[inG].GetIdentifierHash()] {
							conflict = true
							break
						}
					}
					if !conflict {
						group = append(group, other)
						covered[other] = true
					}
				}
				if len(group) > 1 {
					covered[start] = true
					results = append(results, e.buildGroup(group, records, cfg))
				}
			}
		}
	}

	seenGroups := make(map[string]bool)
	var finalResults []DuplicateGroup
	for _, g := range results {
		if !seenGroups[g.ID] {
			seenGroups[g.ID] = true
			finalResults = append(finalResults, g)
		}
	}
	return finalResults
}

type counters struct {
	comp           int
	skipD          int
	skipH          int
	neuralComp     int
	neuralGateSkip int
	maxSim         float64
}

func (e *ComparisonEngine) comparePair(r, comp db.FileRecord, i, j int, maxTol float64, ignoredPairs map[string]map[string]bool, local *counters, edges chan [2]int, cfg config.Settings) bool {
	diff := math.Abs(comp.Duration - r.Duration)
	hashI := r.GetIdentifierHash()
	hashJ := comp.GetIdentifierHash()
	if ignoredPairs[hashI] != nil && ignoredPairs[hashI][hashJ] {
		return false
	}
	tolB := e.getDurationTolerance(comp.Duration, cfg)
	allowed := math.Min(maxTol, tolB)
	if diff > allowed {
		local.skipD++
		return false
	}
	local.comp++
	isDup, score := e.isDuplicateWithStats(r, comp, cfg, local)
	if score > local.maxSim {
		local.maxSim = score
	}
	if isDup {
		edges <- [2]int{i, j}
		return true
	}
	local.skipH++
	return false
}

func (e *ComparisonEngine) buildGroup(cluster []int, records []db.FileRecord, cfg config.Settings) DuplicateGroup {
	sort.Ints(cluster)
	var gID strings.Builder
	var fileInfos []FileInfo
	bestIdx := 0
	for i, idx := range cluster {
		r := records[idx]
		gID.WriteString(r.Path)
		gID.WriteString("|")
		fileInfos = append(fileInfos, FileInfo{
			Path: r.Path, Size: r.Size, Duration: r.Duration, Width: r.Width, Height: r.Height,
			IdentifierHash: r.GetIdentifierHash(), Codec: r.Codec, Bitrate: r.Bitrate, FPS: r.FPS, Similarity: 100.0,
		})
		if fileInfos[i].Size > fileInfos[bestIdx].Size {
			bestIdx = i
		}
	}
	refRecord := records[cluster[bestIdx]]
	for i := range fileInfos {
		if i == bestIdx {
			continue
		}
		_, score := e.isDuplicate(refRecord, records[cluster[i]], cfg)
		fileInfos[i].Similarity = score * 100.0
	}
	return DuplicateGroup{ID: gID.String(), Files: fileInfos}
}

func (e *ComparisonEngine) getDurationTolerance(duration float64, cfg config.Settings) float64 {
	tolerance := duration * (cfg.PercentDurationDifference / 100.0)
	if cfg.FilterByDuration {
		if cfg.DurationDifferenceMinSec > 0 {
			tolerance = math.Max(tolerance, cfg.DurationDifferenceMinSec)
		}
		if cfg.DurationDifferenceMaxSec > 0 {
			tolerance = math.Min(tolerance, cfg.DurationDifferenceMaxSec)
		}
	}
	return tolerance
}

const phashNeuralGate = 0.60

func (e *ComparisonEngine) isDuplicate(a, b db.FileRecord, cfg config.Settings) (bool, float64) {
	return e.isDuplicateWithStats(a, b, cfg, nil)
}

func (e *ComparisonEngine) isDuplicateWithStats(a, b db.FileRecord, cfg config.Settings, local *counters) (bool, float64) {
	required := cfg.Percent / 100.0
	phashScore := e.phashSimilarity(a, b)

	// Neural mode: pHash gate first (when hashes exist), then trim-tolerant
	// cosine similarity. This is tuned for the same underlying video across
	// quality/scale/trim changes, not broad semantic matching.
	if phashScore > 0 && phashScore < phashNeuralGate {
		if local != nil {
			local.neuralGateSkip++
		}
		return false, phashScore
	}

	aEmbs := e.getOrLoadEmbeddings(&a)
	bEmbs := e.getOrLoadEmbeddings(&b)

	if len(aEmbs) > 0 && len(bEmbs) > 0 {
		score, ok := e.neuralSimilarity(aEmbs, bEmbs)
		if ok {
			if local != nil {
				local.neuralComp++
			}
			// Normalise from [-1,1] to [0,1] for consistency with pHash scoring.
			normScore := (score + 1.0) / 2.0
			return normScore >= required, normScore
		}
	}

	if phashScore == 0 {
		return false, 0
	}
	return phashScore >= required, phashScore
}

func (e *ComparisonEngine) phashSimilarity(a, b db.FileRecord) float64 {
	return bestTemporalAlignmentScore(len(a.PHashV2s), len(b.PHashV2s), func(i, j int) float64 {
		dist := phashHamming(a.PHashV2s[i], b.PHashV2s[j])
		return 1.0 - (float64(dist) / 64.0)
	})
}

func (e *ComparisonEngine) neuralSimilarity(a, b [][]float32) (float64, bool) {
	score := bestTemporalAlignmentScore(len(a), len(b), func(i, j int) float64 {
		return neural.CosineSimilarity(a[i], b[j])
	})
	return score, score != 0
}

func bestTemporalAlignmentScore(n, m int, scoreAt func(i, j int) float64) float64 {
	if n == 0 || m == 0 {
		return 0
	}

	leftOrder := temporalSampleOrder(n)
	rightOrder := temporalSampleOrder(m)
	minFrames := min(n, m)
	minOverlap := max(1, minFrames/2)
	best := 0.0

	// Offset is applied after sorting Van der Corput samples by timestamp. The
	// extraction index order is low-discrepancy, not chronological, so aligning by
	// raw index would compare unrelated timestamps.
	for offset := -n + 1; offset <= m-1; offset++ {
		total := 0.0
		count := 0
		for li, sourceI := range leftOrder {
			rj := li + offset
			if rj < 0 || rj >= m {
				continue
			}
			total += scoreAt(sourceI, rightOrder[rj])
			count++
		}
		if count < minOverlap {
			continue
		}
		avg := total / float64(count)
		if avg > best {
			best = avg
		}
	}

	return best
}

func temporalSampleOrder(n int) []int {
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool {
		return media.VanDerCorput(order[i]+1) < media.VanDerCorput(order[j]+1)
	})
	return order
}

func phashHamming(a, b uint64) int {
	return phashPopcount(a ^ b)
}

func phashPopcount(b uint64) int {
	return bits.OnesCount64(b)
}
