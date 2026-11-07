//go:build !darwin || !cgo

package neural

import "errors"

var ErrCoreMLNotSupported = errors.New("CoreML is only supported on macOS (Apple Silicon)")

// IsCoreMLAvailable returns false on non-macOS or non-Cgo platforms.
func IsCoreMLAvailable() bool {
	return false
}

// IsCoreMLLoaded returns false on non-macOS platforms.
func IsCoreMLLoaded() bool {
	return false
}

// InitCoreML returns an error on non-macOS platforms.
func InitCoreML(modelPath string) error {
	return ErrCoreMLNotSupported
}

// EmbedImage returns an error on non-macOS platforms.
func EmbedImage(imageBytes []byte) ([]float32, error) {
	return nil, ErrCoreMLNotSupported
}

// EmbedImages returns an error on non-macOS platforms.
func EmbedImages(images [][]byte) ([][]float32, error) {
	return nil, ErrCoreMLNotSupported
}

// CloseCoreML is a no-op on non-macOS platforms.
func CloseCoreML() {}

