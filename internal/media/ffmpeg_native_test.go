package media

import (
	"context"
	"path/filepath"
	"testing"
)

func TestExtractGray32x32Native(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.mp4")
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			data, err := ExtractGray32x32Native(context.Background(), f, 1.0)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if len(data) != 1024 {
				t.Errorf("Expected 1024 bytes, got %d", len(data))
			}
		})
	}
}

func TestExtractThumbnailNative(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.mp4")
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			data, err := ExtractThumbnailNative(context.Background(), f, 1.0, 160, 90)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if len(data) == 0 {
				t.Errorf("Expected non-empty data")
			}
		})
	}
}

func TestExtractGray32x32NativeOutOfBounds(t *testing.T) {
	data, err := ExtractGray32x32Native(context.Background(), "testdata/testsrc.mp4", 999.0)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(data) != 1024 {
		t.Errorf("Expected 1024 bytes fallback frame, got %d", len(data))
	}
}
