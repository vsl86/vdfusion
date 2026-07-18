package neural

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []float32
		expected float64
	}{
		{
			name:     "identical vectors",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{1.0, 0.0, 0.0},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{0.0, 1.0, 0.0},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{-1.0, 0.0, 0.0},
			expected: -1.0,
		},
		{
			name:     "arbitrary angles",
			a:        []float32{0.5, 0.5, 0.5, 0.5},
			b:        []float32{0.5, 0.5, 0.5, 0.5},
			expected: 1.0, // normalized: 0.5*0.5 * 4 = 1.0
		},
		{
			name:     "empty vectors",
			a:        []float32{},
			b:        []float32{},
			expected: 0.0,
		},
		{
			name:     "mismatched sizes",
			a:        []float32{1.0},
			b:        []float32{1.0, 2.0},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CosineSimilarity(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("CosineSimilarity() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestAverageCosineSimilarity(t *testing.T) {
	as := [][]float32{
		{1.0, 0.0},
		{0.0, 1.0},
	}
	bs := [][]float32{
		{1.0, 0.0},
		{0.0, -1.0},
	}

	avg, ok := AverageCosineSimilarity(as, bs)
	if !ok {
		t.Fatal("expected ok to be true")
	}

	// first pair similarity: 1.0
	// second pair similarity: -1.0
	// average: 0.0
	if avg != 0.0 {
		t.Errorf("expected average similarity 0.0, got %v", avg)
	}
}

func TestPackUnpackEmbeddings(t *testing.T) {
	original := [][]float32{
		{0.1, 0.2, 0.3},
		{0.4, 0.5, 0.6},
	}

	blob := PackEmbeddings(original)
	if len(blob) == 0 {
		t.Fatal("expected non-empty blob")
	}

	unpacked := UnpackEmbeddings(blob)
	if len(unpacked) != len(original) {
		t.Fatalf("expected len %d, got %d", len(original), len(unpacked))
	}

	for i := range original {
		if len(unpacked[i]) != len(original[i]) {
			t.Fatalf("mismatched vector dimension at %d", i)
		}
		for j := range original[i] {
			if unpacked[i][j] != original[i][j] {
				t.Errorf("mismatched value at [%d][%d]: got %v, expected %v", i, j, unpacked[i][j], original[i][j])
			}
		}
	}
}

func TestNeuralClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/info" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{"model": "test-model"})
			return
		}
		if r.URL.Path == "/embed" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Return dummy 3-D embeddings for 2 frames
			resp := embedResponse{
				Embeddings: [][]float32{
					{0.1, 0.2, 0.3},
					{0.4, 0.5, 0.6},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)

	// HealthCheck
	if !client.HealthCheck(context.Background()) {
		t.Error("expected healthcheck to pass")
	}

	// Info
	info, err := client.Info(context.Background())
	if err != nil {
		t.Fatalf("info failed: %v", err)
	}
	if info["model"] != "test-model" {
		t.Errorf("expected model 'test-model', got %v", info["model"])
	}

	// Embed
	frames := [][]byte{
		[]byte("dummy-frame-1"),
		[]byte("dummy-frame-2"),
	}
	embs, err := client.Embed(context.Background(), frames)
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	if len(embs) != 2 || len(embs[0]) != 3 {
		t.Errorf("unexpected embeddings shape: %v", embs)
	}
}
