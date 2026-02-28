package media

import (
	"context"
	"path/filepath"
	"testing"
)

func TestExtractGray32x32(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.mp4")
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			data, err := ExtractGray32x32(context.Background(), f, 1.0)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if len(data) != 1024 {
				t.Errorf("Expected 1024 bytes, got %d", len(data))
			}
		})
	}
}

func TestExtractThumbnail(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.mp4")
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			data, err := ExtractThumbnail(context.Background(), f, 1.0, 160, 90)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if len(data) == 0 {
				t.Errorf("Expected non-empty data")
			}
		})
	}
}

func TestExtractGray32x32OutOfBounds(t *testing.T) {
	_, err := ExtractGray32x32(context.Background(), "testdata/testsrc.mp4", 999.0)
	if err == nil {
		t.Fatalf("Expected error for out of bounds timestamp")
	}
	t.Logf("Out of bounds error: %v", err)
}
