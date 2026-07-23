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
			settings:    config.Settings{Percent: 90, PercentDurationDifference: 20, Thumbnails: 4, Concurrency: 2},
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
			settings:   config.Settings{Percent: 90, PercentDurationDifference: 20, Thumbnails: 4, Concurrency: 2},
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
			settings:  config.Settings{Percent: 90},
			wantIs:    true,
			wantScore: 1.0,
		},
		{
			name:     "Different neural embeddings",
			a:        db.FileRecord{NeuralEmbeddings: testVec1},
			b:        db.FileRecord{NeuralEmbeddings: testVec3},
			settings: config.Settings{Percent: 90},
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
			settings: config.Settings{Percent: 90},
			wantIs:   false,
		},
	}

	e := NewComparisonEngine()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isDup, score := e.isDuplicate(tt.a, tt.b, tt.settings)
			if isDup != tt.wantIs {
				t.Errorf("isDuplicate() isDup = %v, want %v", isDup, tt.wantIs)
			}
			if tt.wantScore != 0.0 && score != tt.wantScore {
				t.Errorf("isDuplicate() score = %v, want %v", score, tt.wantScore)
			}
		})
	}
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
