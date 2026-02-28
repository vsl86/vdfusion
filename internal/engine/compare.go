package engine

import (
	"context"
	"log"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"vdfusion/internal/config"
	"vdfusion/internal/db"
)

type ComparisonEngine struct{}

func NewComparisonEngine() *ComparisonEngine {
	return &ComparisonEngine{}
}

// Compare finds duplicates among the provided records using duration bucketing and pHash.
func (e *ComparisonEngine) Compare(ctx context.Context, records []db.FileRecord, ignoredGroups []db.IgnoredGroup, cfg config.Settings, reporter ProgressReporter) []DuplicateGroup {
	if len(records) < 2 {
		return nil
	}

	// Check for initial cancellation
	if ctx.Err() != nil {
		return nil
	}

	// 0. Build ignored pairs lookup
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

	// 1. Sort records by duration for sliding window optimization
	// We sort indices to avoid reordering the original slice if it matters, but actually reordering is fine for results.
	// But to keep it clean, let's sort the records slice directly.
	sort.Slice(records, func(i, j int) bool {
		return records[i].Duration < records[j].Duration
	})

	// 2. Disjoint-Set / Union-Find for transitive grouping
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

	// 3. Comparison Loop (Sliding Window / Sweep Line)
	// Because records are sorted by duration, we only need to check forward until duration diff exceeds tolerance.
	adj := make(map[int][]int)
	edges := make(chan [2]int, 4096)
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
	var wg sync.WaitGroup
	var processed int64
	type counters struct {
		comp  int
		skipD int
		skipH int
	}
	cstats := make([]counters, workers)

	doneCh := make(chan struct{})
	go func() {
		t := time.NewTicker(200 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if reporter != nil {
					cur := int(atomic.LoadInt64(&processed))
					total := len(records)
					if cur > total {
						cur = total
					}
					reporter.BroadcastProgress(cur, total, "comparing", "", 0)
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
				for j := i + 1; j < len(records); j++ {
					comp := records[j]
					diffSeconds := comp.Duration - r.Duration
					if diffSeconds > maxTolerance {
						break
					}
					hashI := r.GetIdentifierHash()
					hashJ := comp.GetIdentifierHash()
					if ignoredPairs[hashI] != nil && ignoredPairs[hashI][hashJ] {
						continue
					}
					tolA := maxTolerance
					tolB := e.getDurationTolerance(comp.Duration, cfg)
					allowedSeconds := math.Min(tolA, tolB)
					if diffSeconds > allowedSeconds {
						local.skipD++
						continue
					}
					local.comp++
					isDup, _ := e.isDuplicate(r, comp, cfg)
					if isDup {
						select {
						case edges <- [2]int{i, j}:
						case <-ctx.Done():
							return
						}
					} else {
						local.skipH++
					}
				}
				atomic.AddInt64(&processed, 1)
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
	for _, s := range cstats {
		totalComp += s.comp
		totalSkipD += s.skipD
		totalSkipH += s.skipH
	}
	log.Printf("Comparison Stats: Performed=%d, SkippedByDuration=%d, SkippedByHash=%d",
		totalComp, totalSkipD, totalSkipH)

	// 4. Grouping Results (Candidate clusters from Union-Find)
	if reporter != nil {
		reporter.BroadcastProgress(len(records), len(records), "grouping", "", 0)
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

		// Optimized grouping strategy for large/messy clusters:
		// Instead of O(M^3) iterative star construction, we use a more stable approach.
		// If the cluster is reasonably small and has no conflicts, merge all.
		// If it's a huge "mega-cluster" (often from low similarity settings), we cap it to avoid hangs.
		if len(cluster) > 500 {
			log.Printf("ComparisonEngine: Cap reached for cluster of size %d. Possible low similarity setting causing mega-clusters.", len(cluster))
			// Just pick the first 500 to avoid locking up
			cluster = cluster[:500]
		}

		hasIgnored := false
		for idx1 := 0; idx1 < len(cluster); idx1++ {
			for idx2 := idx1 + 1; idx2 < len(cluster); idx2++ {
				i := cluster[idx1]
				j := cluster[idx2]
				hashI := records[i].GetIdentifierHash()
				hashJ := records[j].GetIdentifierHash()
				if ignoredPairs[hashI] != nil && ignoredPairs[hashI][hashJ] {
					hasIgnored = true
					break
				}
			}
			if hasIgnored {
				break
			}
		}

		if !hasIgnored {
			// Fast path: Return whole cluster as one group
			var gID strings.Builder
			var fileInfos []FileInfo
			sort.Ints(cluster)
			for _, idx := range cluster {
				gID.WriteString(records[idx].Path + "|")
				fileInfos = append(fileInfos, FileInfo{
					Path:           records[idx].Path,
					Size:           records[idx].Size,
					Duration:       records[idx].Duration,
					Width:          records[idx].Width,
					Height:         records[idx].Height,
					IdentifierHash: records[idx].GetIdentifierHash(),
					Codec:          records[idx].Codec,
					Bitrate:        records[idx].Bitrate,
					FPS:            records[idx].FPS,
					Similarity:     100.0, // Default for single group, will refine below
				})
			}

			// Refine similarity: Use the "best" file as reference (usually index 0 after sorting or biggest)
			// For simplicity during initial grouping, we designated everyone as 100%
			// but if we want it relative to the "best" in the group:
			bestIdx := 0
			for idx, f := range fileInfos {
				if f.Size > fileInfos[bestIdx].Size {
					bestIdx = idx
				}
			}

			// We need a way to get scores between arbitrary pairs in the cluster.
			// Let's re-calculate score relative to the best one for display.
			refRecord := records[cluster[bestIdx]]
			for idx := range fileInfos {
				actualRecord := records[cluster[idx]]
				if idx == bestIdx {
					fileInfos[idx].Similarity = 100.0
				} else {
					_, score := e.isDuplicate(refRecord, actualRecord, cfg)
					fileInfos[idx].Similarity = score * 100.0
				}
			}

			results = append(results, DuplicateGroup{ID: gID.String(), Files: fileInfos})
			continue
		}

		// Slow path: Split by star clusters to respect ignored pairs.
		// We use a simpler, more stable greedy cover with a hard limit on iterations.
		coveredNodes := make(map[int]bool)
		groupsFound := 0
		for _, startNode := range cluster {
			if groupsFound > 100 { // Max groups per cluster
				break
			}
			if coveredNodes[startNode] {
				continue
			}

			currentGroup := []int{startNode}
			for _, otherNode := range cluster {
				if startNode == otherNode {
					continue
				}
				if !contains(adj[startNode], otherNode) {
					continue
				}

				hasConflict := false
				hashOther := records[otherNode].GetIdentifierHash()
				for _, inGroup := range currentGroup {
					hashIn := records[inGroup].GetIdentifierHash()
					if ignoredPairs[hashOther] != nil && ignoredPairs[hashOther][hashIn] {
						hasConflict = true
						break
					}
				}

				if !hasConflict {
					currentGroup = append(currentGroup, otherNode)
					coveredNodes[otherNode] = true
				}
			}

			if len(currentGroup) > 1 {
				groupsFound++
				coveredNodes[startNode] = true
				sort.Ints(currentGroup)
				var gID strings.Builder
				var fileInfos []FileInfo
				for _, idx := range currentGroup {
					gID.WriteString(records[idx].Path + "|")
					fileInfos = append(fileInfos, FileInfo{
						Path:           records[idx].Path,
						Size:           records[idx].Size,
						Duration:       records[idx].Duration,
						Width:          records[idx].Width,
						Height:         records[idx].Height,
						IdentifierHash: records[idx].GetIdentifierHash(),
						Codec:          records[idx].Codec,
						Bitrate:        records[idx].Bitrate,
						FPS:            records[idx].FPS,
						Similarity:     100.0,
					})
				}

				// Relative similarity for slow-path groups
				bestIdx := 0
				for idx, f := range fileInfos {
					if f.Size > fileInfos[bestIdx].Size {
						bestIdx = idx
					}
				}
				refRecord := records[currentGroup[bestIdx]]
				for idx := range fileInfos {
					actualRecord := records[currentGroup[idx]]
					if idx == bestIdx {
						fileInfos[idx].Similarity = 100.0
					} else {
						_, score := e.isDuplicate(refRecord, actualRecord, cfg)
						fileInfos[idx].Similarity = score * 100.0
					}
				}

				results = append(results, DuplicateGroup{ID: gID.String(), Files: fileInfos})
			}
		}
	}

	// Deduplicate identical groups from results
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

func contains(slice []int, val int) bool {
	return slices.Contains(slice, val)
}

func (e *ComparisonEngine) getDurationTolerance(duration float64, cfg config.Settings) float64 {
	tolerance := duration * (cfg.PercentDurationDifference / 100.0)

	if cfg.DurationDifferenceMinSec > 0 {
		tolerance = math.Max(tolerance, cfg.DurationDifferenceMinSec)
	}
	if cfg.DurationDifferenceMaxSec > 0 {
		tolerance = math.Min(tolerance, cfg.DurationDifferenceMaxSec)
	}

	return tolerance
}

// isDuplicate checks if two records are duplicates by comparing their pHash sequences.
// Returns (isDuplicate, normalizedScore).
func (e *ComparisonEngine) isDuplicate(a, b db.FileRecord, cfg config.Settings) (bool, float64) {
	hashesA := a.PHashV2s
	hashesB := b.PHashV2s

	framesToCompare := min(len(hashesB), len(hashesA))
	if framesToCompare == 0 {
		return false, 0
	}

	requiredSimilarity := cfg.Percent / 100.0

	totalSimilarity := 0.0
	for i := range framesToCompare {
		dist := phashHamming(hashesA[i], hashesB[i])
		if dist > 32 { // per-frame floor ~50%: reject if ANY frame is too dissimilar
			return false, 0
		}
		similarity := 1.0 - (float64(dist) / 64.0)
		totalSimilarity += similarity
	}

	avgSimilarity := totalSimilarity / float64(framesToCompare)
	return avgSimilarity >= requiredSimilarity, avgSimilarity
}

func phashHamming(a, b uint64) int {
	return phashPopcount(a ^ b)
}

func phashPopcount(bits uint64) int {
	count := 0
	for bits != 0 {
		bits &= bits - 1
		count++
	}
	return count
}

func generateGroupID(seed string) string {
	return seed
}
