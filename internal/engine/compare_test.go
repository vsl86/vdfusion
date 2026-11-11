package engine

import (
	"context"
	"testing"

	"vdfusion/internal/config"
	"vdfusion/internal/db"
)

func TestComparisonEngine(t *testing.T) {
	// Create test neural embeddings
	testVec1 := [][]float32{{1, 0, 0, 0}}
	testVec2 := [][]float32{{1, 0, 0, 0}}
	testVec3 := [][]float32{{0, 1, 0, 0}}

	tests := []struct {
		name        string
		records     []db.FileRecord
		ignored     []db.IgnoredGroup
		settings    config.Settings
		wantGroups  int
		wantEachLen []int
	}{
		{
			name: "Two identical files",
			records: []db.FileRecord{
				{Path: "/file1.mp4", Size: 100, Duration: 10, Width: 1920, Height: 1080, PHashV2s: []uint64{0xDEADBEEF}},
				{Path: "/file2.mp4", Size: 100, Duration: 10, Width: 1920, Height: 1080, PHashV2s: []uint64{0xDEADBEEF}},
			},
			ignored:     nil,
			settings:    config.Settings{Percent: 90, PercentDurationDifference: 20, Thumbnails: 4, Concurrency: 2},
			wantGroups:  1,
			wantEachLen: []int{2},
		},
		{
			name: "Three identical files",
			records: []db.FileRecord{
				{Path: "/file1.mp4", Size: 100, Duration: 10, Width: 1920, Height: 1080, PHashV2s: []uint64{0xDEADBEEF}},
				{Path: "/file2.mp4", Size: 100, Duration: 10, Width: 1920, Height: 1080, PHashV2s: []uint64{0xDEADBEEF}},
				{Path: "/file3.mp4", Size: 100, Duration: 10, Width: 1920, Height: 1080, PHashV2s: []uint64{0xDEADBEEF}},
			},
			ignored:     nil,
			settings:    config.Settings{Percent: 90, PercentDurationDifference: 20, Thumbnails: 4, Concurrency: 2},
			wantGroups:  1,
			wantEachLen: []int{3},
		},
		{
			name: "Two different files",
			records: []db.FileRecord{
				{Path: "/file1.mp4", Size: 100, Duration: 10, Width: 1920, Height: 1080, PHashV2s: []uint64{0xDEADBEEF}},
				{Path: "/file2.mp4", Size: 200, Duration: 20, Width: 3840, Height: 2160, PHashV2s: []uint64{0xBADC0FFE}},
			},
			ignored:    nil,
			settings:   config.Settings{Percent: 90, PercentDurationDifference: 20, Thumbnails: 4, Concurrency: 2},
			wantGroups: 0,
		},
		{
			name: "Ignored group should not appear",
			records: []db.FileRecord{
				{Path: "/file1.mp4", Size: 100, Duration: 10, Width: 1920, Height: 1080, PHashV2s: []uint64{0xDEADBEEF}},
				{Path: "/file2.mp4", Size: 100, Duration: 10, Width: 1920, Height: 1080, PHashV2s: []uint64{0xDEADBEEF}},
			},
			ignored: []db.IgnoredGroup{
				{IdentifierHashes: []string{
					(&db.FileRecord{Path: "/file1.mp4", Size: 100, Duration: 10}).GetIdentifierHash(),
					(&db.FileRecord{Path: "/file2.mp4", Size: 100, Duration: 10}).GetIdentifierHash(),
				}},
			},
			settings:   config.Settings{Percent: 90, PercentDurationDifference: 20, Thumbnails: 4, Concurrency: 2},
			wantGroups: 0,
		},
		{
			name: "Two files with identical neural embeddings",
			records: []db.FileRecord{
				{Path: "/file1.mp4", Size: 100, Duration: 10, Width: 1920, Height: 1080, NeuralEmbeddings: testVec1},
				{Path: "/file2.mp4", Size: 100, Duration: 10, Width: 1920, Height: 1080, NeuralEmbeddings: testVec2},
			},
			ignored:     nil,
			settings:    config.Settings{Percent: 90, PercentDurationDifference: 20, Thumbnails: 4, Concurrency: 2, NeuralBackendEnabled: true},
			wantGroups:  1,
			wantEachLen: []int{2},
		},
		{
			name: "Two files with different neural embeddings",
			records: []db.FileRecord{
				{Path: "/file1.mp4", Size: 100, Duration: 10, Width: 1920, Height: 1080, NeuralEmbeddings: testVec1},
				{Path: "/file2.mp4", Size: 100, Duration: 10, Width: 1920, Height: 1080, NeuralEmbeddings: testVec3},
			},
			ignored:    nil,
			settings:   config.Settings{Percent: 90, PercentDurationDifference: 20, Thumbnails: 4, Concurrency: 2, NeuralBackendEnabled: true},
			wantGroups: 0,
		},
	}

	e := NewComparisonEngine()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			groups := e.Compare(ctx, tt.records, tt.ignored, tt.settings, nil)
			if len(groups) != tt.wantGroups {
				t.Fatalf("Compare() got %d groups, want %d", len(groups), tt.wantGroups)
			}
			if tt.wantEachLen != nil {
				for i, g := range groups {
					if len(g.Files) != tt.wantEachLen[i] {
						t.Errorf("Group %d has %d files, want %d", i, len(g.Files), tt.wantEachLen[i])
					}
				}
			}
		})
	}
}

func TestIsDuplicate(t *testing.T) {
	testVec1 := [][]float32{{1, 0, 0, 0}}
	testVec2 := [][]float32{{1, 0, 0, 0}}
	testVec3 := [][]float32{{0, 1, 0, 0}}

	tests := []struct {
		name      string
		a         db.FileRecord
		b         db.FileRecord
		settings  config.Settings
		wantIs    bool
		wantScore float64
	}{
		{
			name:      "Identical phashes",
			a:         db.FileRecord{PHashV2s: []uint64{0xDEADBEEF}},
			b:         db.FileRecord{PHashV2s: []uint64{0xDEADBEEF}},
			settings:  config.Settings{Percent: 90},
			wantIs:    true,
			wantScore: 1.0,
		},
		{
			name:     "Different phashes",
			a:        db.FileRecord{PHashV2s: []uint64{0xDEADBEEF}},
			b:        db.FileRecord{PHashV2s: []uint64{0xBADC0FFE}},
			settings: config.Settings{Percent: 90},
			wantIs:   false,
		},
		{
			name:      "Identical neural embeddings",
			a:         db.FileRecord{NeuralEmbeddings: testVec1},
			b:         db.FileRecord{NeuralEmbeddings: testVec2},
			settings:  config.Settings{Percent: 90, NeuralBackendEnabled: true},
			wantIs:    true,
			wantScore: 1.0,
		},
		{
			name:     "Different neural embeddings",
			a:        db.FileRecord{NeuralEmbeddings: testVec1},
			b:        db.FileRecord{NeuralEmbeddings: testVec3},
			settings: config.Settings{Percent: 90, NeuralBackendEnabled: true},
			wantIs:   false,
		},
		{
			name: "Neural match blocked by low pHash gate",
			a: db.FileRecord{
				PHashV2s:         []uint64{0},
				NeuralEmbeddings: testVec1,
			},
			b: db.FileRecord{
				PHashV2s:         []uint64{(1 << 39) - 1},
				NeuralEmbeddings: testVec2,
			},
			settings: config.Settings{Percent: 90, NeuralPhashGate: 60, NeuralBackendEnabled: true},
			wantIs:   false,
		},
		{
			// Cross-quality pair: same-content embeddings with score just below threshold,
			// but a large resolution gap (5x pixel ratio) gives a 3.5 pp bonus, making
			// it match. Models the real SD-DivX vs HD-HEVC remaster case.
			name: "Cross-quality duplicate caught by quality gap bonus",
			a: db.FileRecord{
				Width: 1440, Height: 1076,
				// High cosine similarity but just below 96.8% threshold
				NeuralEmbeddings: testVec1, // norm score = 1.0 against testVec1
			},
			b: db.FileRecord{
				Width: 640, Height: 480,
				NeuralEmbeddings: testVec1, // same vec -> score = 1.0
			},
			// At 96.8% threshold with 5x pixel ratio, bonus = 3.50 pp -> effective = 93.30%
			settings: config.Settings{Percent: 96.8, NeuralBackendEnabled: true},
			wantIs:   true,
		},
	}

	e := NewComparisonEngine()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isDup, score := e.isDuplicate(&tt.a, &tt.b, tt.settings)
			if isDup != tt.wantIs {
				t.Errorf("isDuplicate() isDup = %v, want %v", isDup, tt.wantIs)
			}
			if tt.wantScore != 0.0 && score != tt.wantScore {
				t.Errorf("isDuplicate() score = %v, want %v", score, tt.wantScore)
			}
		})
	}
}

func TestTrimTolerantAlignment(t *testing.T) {
	e := NewComparisonEngine()

	t.Run("phash shifted by trimmed intro", func(t *testing.T) {
		a := db.FileRecord{PHashV2s: phashesFromTimeline([]uint64{1, 2, 3, 4})}
		b := db.FileRecord{PHashV2s: phashesFromTimeline([]uint64{99, 1, 2, 3, 4})}
		if got := e.phashSimilarity(&a, &b); got != 1.0 {
			t.Fatalf("phashSimilarity() = %v, want 1", got)
		}
	})

	t.Run("neural shifted by trimmed intro", func(t *testing.T) {
		e1 := []float32{1, 0, 0, 0}
		e2 := []float32{0, 1, 0, 0}
		e3 := []float32{0, 0, 1, 0}
		other := []float32{0, 0, 0, 1}
		a := db.FileRecord{NeuralEmbeddings: embeddingsFromTimeline([][]float32{e1, e2, e3})}
		b := db.FileRecord{NeuralEmbeddings: embeddingsFromTimeline([][]float32{other, e1, e2, e3})}

		isDup, score := e.isDuplicate(&a, &b, config.Settings{Percent: 95, NeuralBackendEnabled: true})
		if !isDup {
			t.Fatalf("isDuplicate() = false, want true; score=%v", score)
		}
		if score != 1.0 {
			t.Fatalf("isDuplicate() score = %v, want 1", score)
		}
	})

	t.Run("Configurable NeuralPhashGate threshold", func(t *testing.T) {
		e := NewComparisonEngine()
		e1 := []float32{1, 0, 0, 0}
		// Identical embeddings (score = 1.0)
		a := db.FileRecord{
			// Low pHash similarity (~50%)
			PHashV2s:         []uint64{0x0000000000000000},
			NeuralEmbeddings: [][]float32{e1},
		}
		b := db.FileRecord{
			// 32 bits different out of 64 -> dist 32 -> score = 1 - 32/64 = 0.50
			PHashV2s:         []uint64{0x00000000FFFFFFFF},
			NeuralEmbeddings: [][]float32{e1},
		}

		// Case 1: Default gate at 60% -> 0.50 < 0.60 -> skipped by gate
		local := &counters{}
		isDup, _ := e.isDuplicateWithStats(&a, &b, config.Settings{Percent: 90, NeuralPhashGate: 60, NeuralBackendEnabled: true}, local, nil)
		if isDup {
			t.Fatalf("expected isDup=false when phash < gate")
		}
		if local.neuralGateSkip != 1 {
			t.Fatalf("expected neuralGateSkip=1, got %d", local.neuralGateSkip)
		}

		// Case 2: Lowered gate at 40% -> 0.50 >= 0.40 -> passes gate and matches via neural
		local2 := &counters{}
		isDup2, score2 := e.isDuplicateWithStats(&a, &b, config.Settings{Percent: 90, NeuralPhashGate: 40, NeuralBackendEnabled: true}, local2, nil)
		if !isDup2 {
			t.Fatalf("expected isDup=true when phash >= gate, got false with score=%v", score2)
		}
		if local2.neuralGateSkip != 0 {
			t.Fatalf("expected neuralGateSkip=0, got %d", local2.neuralGateSkip)
		}
		if local2.neuralComp != 1 {
			t.Fatalf("expected neuralComp=1, got %d", local2.neuralComp)
		}

		// Case 3: Gate disabled at 0% -> passes gate and matches via neural
		local3 := &counters{}
		isDup3, score3 := e.isDuplicateWithStats(&a, &b, config.Settings{Percent: 90, NeuralPhashGate: 0, NeuralBackendEnabled: true}, local3, nil)
		if !isDup3 {
			t.Fatalf("expected isDup=true when gate is 0, got false with score=%v", score3)
		}
		if local3.neuralGateSkip != 0 {
			t.Fatalf("expected neuralGateSkip=0, got %d", local3.neuralGateSkip)
		}

		// Case 4: In-memory centroid early exit (orthogonal centroids short-circuit without full embeddings)
		aWithID := a
		aWithID.ID = 101
		bWithID := b
		bWithID.ID = 102
		centroids := map[int64][]float32{
			101: {1, 0, 0, 0},
			102: {0, 1, 0, 0}, // orthogonal -> sim = 0.0 < minMeanThreshold
		}
		local4 := &counters{}
		isDup4, _ := e.isDuplicateWithStats(&aWithID, &bWithID, config.Settings{Percent: 90, NeuralPhashGate: 40, NeuralBackendEnabled: true}, local4, centroids)
		if isDup4 {
			t.Fatalf("expected isDup=false when centroids are orthogonal")
		}
		if local4.neuralMeanSkip != 1 {
			t.Fatalf("expected neuralMeanSkip=1 from centroid pre-filter, got %d", local4.neuralMeanSkip)
		}
		if local4.neuralComp != 0 {
			t.Fatalf("expected neuralComp=0 (no full embeddings loaded), got %d", local4.neuralComp)
		}

		// Case 5: Centroid map present but file lacks centroid -> short circuit
		centroidsIncomplete := map[int64][]float32{
			101: {1, 0, 0, 0},
			// 102 missing
		}
		local5 := &counters{}
		isDup5, _ := e.isDuplicateWithStats(&aWithID, &bWithID, config.Settings{Percent: 90, NeuralPhashGate: 40, NeuralBackendEnabled: true}, local5, centroidsIncomplete)
		if isDup5 {
			t.Fatalf("expected isDup=false when file lacks centroid in map")
		}
		if local5.neuralGateSkip != 1 {
			t.Fatalf("expected neuralGateSkip=1 when centroid missing, got %d", local5.neuralGateSkip)
		}
	})
}

func phashesFromTimeline(timeline []uint64) []uint64 {
	out := make([]uint64, len(timeline))
	for chronologicalPos, sampleIndex := range temporalSampleOrder(len(timeline)) {
		out[sampleIndex] = timeline[chronologicalPos]
	}
	return out
}

func embeddingsFromTimeline(timeline [][]float32) [][]float32 {
	out := make([][]float32, len(timeline))
	for chronologicalPos, sampleIndex := range temporalSampleOrder(len(timeline)) {
		out[sampleIndex] = timeline[chronologicalPos]
	}
	return out
}

func TestPHashHamming(t *testing.T) {
	tests := []struct {
		name string
		a    uint64
		b    uint64
		want int
	}{
		{"same", 0xDEADBEEF, 0xDEADBEEF, 0},
		{"different", 0xDEADBEEF, 0xBADC0FFE, 13},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := phashHamming(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("phashHamming(%x, %x) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func BenchmarkNeuralSimilarity(b *testing.B) {
	e := NewComparisonEngine()
	vecA := make([][]float32, 16)
	vecB := make([][]float32, 16)
	for i := range 16 {
		vecA[i] = make([]float32, 512)
		vecB[i] = make([]float32, 512)
		vecA[i][0] = 1.0
		vecB[i][0] = 0.9
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.neuralSimilarity(vecA, vecB)
	}
}

func BenchmarkPHashSimilarity(b *testing.B) {
	e := NewComparisonEngine()
	recA := db.FileRecord{PHashV2s: make([]uint64, 16)}
	recB := db.FileRecord{PHashV2s: make([]uint64, 16)}
	for i := range 16 {
		recA.PHashV2s[i] = uint64(i * 0x123456789abcdef0)
		recB.PHashV2s[i] = uint64(i * 0x0fedcba987654321)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.phashSimilarity(&recA, &recB)
	}
}

func BenchmarkPairComparisonWithMeanEarlyExit(b *testing.B) {
	e := NewComparisonEngine()
	vecA := make([][]float32, 16)
	vecB := make([][]float32, 16)
	for i := range 16 {
		vecA[i] = make([]float32, 512)
		vecB[i] = make([]float32, 512)
		vecA[i][0] = 1.0
		vecB[i][1] = 1.0 // orthogonal non-duplicate vectors
	}
	recA := db.FileRecord{ID: 1, Duration: 100, NeuralEmbeddings: vecA, PHashV2s: []uint64{0x12345678}}
	recB := db.FileRecord{ID: 2, Duration: 100, NeuralEmbeddings: vecB, PHashV2s: []uint64{0x12345678}}
	cfg := config.Settings{Percent: 96, NeuralPhashGate: 40, NeuralBackendEnabled: true}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.isDuplicate(&recA, &recB, cfg)
	}
}
