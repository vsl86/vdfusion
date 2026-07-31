package neural

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestBatchEmbedderFlushesAndResets(t *testing.T) {
	var postCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embed" {
			http.NotFound(w, r)
			return
		}
		postCount.Add(1)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		files := r.MultipartForm.File["images"]
		if len(files) == 0 {
			http.Error(w, "No images provided", http.StatusUnprocessableEntity)
			return
		}
		if len(files) > 32 {
			http.Error(w, "Batch too large", http.StatusUnprocessableEntity)
			return
		}
		embs := make([][]float32, len(files))
		for i := range embs {
			embs[i] = []float32{float32(i)}
		}
		_ = json.NewEncoder(w).Encode(embedResponse{Embeddings: embs})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	be := NewBatchEmbedder(client)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	be.Start(ctx)

	img := []byte("fake-jpeg")
	if _, err := be.Embed(ctx, [][]byte{img, img, img}); err != nil {
		t.Fatalf("first embed: %v", err)
	}

	// Linger flush should not re-send the same batch.
	time.Sleep(50 * time.Millisecond)
	if got := postCount.Load(); got != 1 {
		t.Fatalf("expected 1 POST /embed, got %d", got)
	}

	if _, err := be.Embed(ctx, [][]byte{img}); err != nil {
		t.Fatalf("second embed: %v", err)
	}
	if got := postCount.Load(); got != 2 {
		t.Fatalf("expected 2 POST /embed after second request, got %d", got)
	}
}

func TestBatchEmbedderNeverExceedsMaxBatch(t *testing.T) {
	var maxSeen atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(32 << 20)
		n := len(r.MultipartForm.File["images"])
		for {
			cur := maxSeen.Load()
			if int32(n) <= cur || maxSeen.CompareAndSwap(cur, int32(n)) {
				break
			}
		}
		embs := make([][]float32, n)
		_ = json.NewEncoder(w).Encode(embedResponse{Embeddings: embs})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	be := NewBatchEmbedder(client)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	be.Start(ctx)

	img := []byte("fake-jpeg")
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 10; i++ {
			if _, err := be.Embed(ctx, [][]byte{img, img, img, img}); err != nil {
				t.Errorf("embed %d: %v", i, err)
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for embeds")
	}

	if got := maxSeen.Load(); got > 32 {
		t.Fatalf("batch size exceeded max: %d", got)
	}
}
