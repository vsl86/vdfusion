//go:build fakegen

package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
)

func init() {
	originalFn := extractThumbnailFn
	extractThumbnailFn = func(path string, duration float64, count int, i int) ([]byte, error) {
		if strings.HasPrefix(path, "/fake_files") {
			return generateFakeThumbnail(i), nil
		}
		return originalFn(path, duration, count, i)
	}
}

func generateFakeThumbnail(index int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 160, 90))
	c := color.RGBA{uint8(50 + index*20), uint8(100 + index*10), uint8(150), 255}
	for y := 0; y < 90; y++ {
		for x := 0; x < 160; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}
