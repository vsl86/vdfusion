//go:build darwin && cgo

package neural

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework CoreML -framework CoreGraphics -framework ImageIO
#include <stdlib.h>
#include "coreml.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"
)

var (
	coremlMu sync.Mutex
)

// IsCoreMLAvailable returns true on macOS with Cgo enabled.
func IsCoreMLAvailable() bool {
	return true
}

// IsCoreMLLoaded returns true if the CoreML model is loaded in memory.
func IsCoreMLLoaded() bool {
	return bool(C.coreml_is_loaded())
}

// InitCoreML compiles (if needed) and loads the CoreML model located at modelPath.
// modelPath can be a .mlpackage or .mlmodelc.
func InitCoreML(modelPath string) error {
	coremlMu.Lock()
	defer coremlMu.Unlock()

	cPath := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cPath))

	var cErr *C.char
	ret := C.coreml_init(cPath, &cErr)
	if ret != 0 {
		var errMsg string
		if cErr != nil {
			errMsg = C.GoString(cErr)
			C.free(unsafe.Pointer(cErr))
		} else {
			errMsg = fmt.Sprintf("coreml_init failed with code %d", ret)
		}
		return errors.New(errMsg)
	}

	return nil
}

// EmbedImage extracts 512-dimensional CLIP embedding for a single image (JPEG or PNG).
func EmbedImage(imageBytes []byte) ([]float32, error) {
	if len(imageBytes) == 0 {
		return nil, errors.New("empty image data")
	}

	coremlMu.Lock()
	defer coremlMu.Unlock()

	out := make([]float32, 512)
	var cErr *C.char

	ret := C.coreml_embed_image(
		(*C.uint8_t)(unsafe.Pointer(&imageBytes[0])),
		C.int(len(imageBytes)),
		(*C.float)(unsafe.Pointer(&out[0])),
		&cErr,
	)

	if ret != 0 {
		var errMsg string
		if cErr != nil {
			errMsg = C.GoString(cErr)
			C.free(unsafe.Pointer(cErr))
		} else {
			errMsg = fmt.Sprintf("coreml_embed_image failed with code %d", ret)
		}
		return nil, errors.New(errMsg)
	}

	return out, nil
}

// EmbedImages processes a slice of images sequentially with batch size 1 to prevent memory spikes.
func EmbedImages(images [][]byte) ([][]float32, error) {
	if len(images) == 0 {
		return nil, nil
	}

	results := make([][]float32, len(images))
	for i, img := range images {
		emb, err := EmbedImage(img)
		if err != nil {
			return nil, fmt.Errorf("embed frame %d: %w", i, err)
		}
		results[i] = emb
	}

	return results, nil
}

// CloseCoreML releases the in-memory CoreML model.
func CloseCoreML() {
	coremlMu.Lock()
	defer coremlMu.Unlock()
	C.coreml_close()
}

