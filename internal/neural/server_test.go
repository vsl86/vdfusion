//go:build darwin && cgo

package neural

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCoreMLServerAndClient(t *testing.T) {
	modelPath := filepath.Join("..", "..", "neural", "models", "clip-vit-b32-visual_fp16_bs1.mlpackage")
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		t.Skip("Model file not found, skipping CoreML server test")
	}

	err := StartServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("StartServer failed: %v", err)
	}
	defer StopServer()

	if !IsServerRunning() {
		t.Fatal("Expected IsServerRunning to be true")
	}

	addr := GetServerAddr()
	if addr == "" {
		t.Fatal("Expected non-empty server address")
	}

	client := NewClient("http://" + addr)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if !client.HealthCheck(ctx) {
		t.Fatal("HealthCheck failed")
	}

	info, err := client.Info(ctx)
	if err != nil {
		t.Fatalf("Info failed: %v", err)
	}
	if info["device"] != "Apple Neural Engine (ANE)" {
		t.Fatalf("Unexpected info device: %v", info["device"])
	}

	iconPath := filepath.Join("..", "..", "build", "appicon.png")
	data, err := os.ReadFile(iconPath)
	if err != nil {
		t.Fatalf("Failed to read test image: %v", err)
	}

	embs, err := client.Embed(ctx, [][]byte{data})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if len(embs) != 1 || len(embs[0]) != 512 {
		t.Fatalf("Unexpected embeddings shape: %d x %d", len(embs), len(embs[0]))
	}

	t.Logf("CoreML server test passed! Address: %s, Device: %v", addr, info["device"])
}

