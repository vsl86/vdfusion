#ifndef COREML_BRIDGE_H
#define COREML_BRIDGE_H

#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

// coreml_init initializes or compiles and loads the CoreML model.
// Returns 0 on success, or non-zero error code.
// If an error occurs, *error_out will point to an allocated C string that must be freed with free().
int coreml_init(const char *model_path, char **error_out);

// coreml_embed_image processes raw image bytes (JPEG/PNG), scales to 224x224 RGB,
// normalizes with CLIP mean/std, runs inference, and writes 512 L2-normalized floats to out_vec.
// out_vec must be pre-allocated to at least 512 floats.
// Returns 0 on success, or non-zero error code.
int coreml_embed_image(const uint8_t *data, int len, float *out_vec, char **error_out);

// coreml_is_loaded returns true if model is loaded into memory and ready for inference.
bool coreml_is_loaded(void);

// coreml_close releases the model resources.
void coreml_close(void);

#ifdef __cplusplus
}
#endif

#endif // COREML_BRIDGE_H

