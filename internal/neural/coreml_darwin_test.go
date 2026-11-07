//go:build darwin && cgo

package neural

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCoreMLInference(t *testing.T) {
	modelPath := filepath.Join("..", "..", "neural", "models", "clip-vit-b32-visual_fp16_bs1.mlpackage")
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		t.Skip("Model file not found, skipping CoreML test")
	}

	err := InitCoreML(modelPath)
	if err != nil {
		t.Fatalf("InitCoreML failed: %v", err)
	}

	if !IsCoreMLLoaded() {
		t.Fatal("Expected IsCoreMLLoaded() to be true")
	}

	iconPath := filepath.Join("..", "..", "build", "appicon.png")
	data, err := os.ReadFile(iconPath)
	if err != nil {
		t.Fatalf("Failed to read test image: %v", err)
	}

	vec, err := EmbedImage(data)
	if err != nil {
		t.Fatalf("EmbedImage failed: %v", err)
	}

	if len(vec) != 512 {
		t.Fatalf("Expected 512 floats, got %d", len(vec))
	}

	// Verify L2 norm is ~1.0
	var sum float64
	for _, v := range vec {
		sum += float64(v * v)
	}
	if sum < 0.99 || sum > 1.01 {
		t.Fatalf("Vector not L2 normalized: sum of squares = %f", sum)
	}

	t.Logf("CoreML inference succeeded: 512 dimensions, norm = %f", sum)
}

