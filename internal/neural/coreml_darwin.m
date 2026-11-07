//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <CoreML/CoreML.h>
#import <CoreGraphics/CoreGraphics.h>
#import <ImageIO/ImageIO.h>
#import <math.h>
#import <string.h>
#import <stdlib.h>
#import "coreml.h"

static const float CLIP_MEAN[3] = {0.48145466f, 0.4578275f, 0.40821073f};
static const float CLIP_STD[3]  = {0.26862954f, 0.26130258f, 0.27577711f};

static MLModel *g_coreml_model = nil;
static NSLock *g_model_lock = nil;

static void set_error(char **error_out, NSString *msg) {
    if (error_out != NULL) {
        const char *utf8 = [msg UTF8String];
        *error_out = strdup(utf8 ? utf8 : "Unknown error");
    }
}

static BOOL is_valid_mlmodelc(NSFileManager *fm, NSString *path) {
    BOOL isDir = NO;
    if (![fm fileExistsAtPath:path isDirectory:&isDir] || !isDir) {
        return NO;
    }
    NSString *weightsBin = [path stringByAppendingPathComponent:@"weights/weight.bin"];
    NSString *espressoNet = [path stringByAppendingPathComponent:@"model.espresso.net"];
    return [fm fileExistsAtPath:weightsBin] || [fm fileExistsAtPath:espressoNet];
}

int coreml_init(const char *model_path, char **error_out) {
    if (error_out != NULL) {
        *error_out = NULL;
    }

    if (g_model_lock == nil) {
        g_model_lock = [[NSLock alloc] init];
    }

    [g_model_lock lock];
    @autoreleasepool {
        if (g_coreml_model != nil) {
            // Already initialized
            [g_model_lock unlock];
            return 0;
        }

        if (model_path == NULL || strlen(model_path) == 0) {
            set_error(error_out, @"Empty model path provided");
            [g_model_lock unlock];
            return 1;
        }

        NSString *pathStr = [NSString stringWithUTF8String:model_path];
        // Ensure absolute path
        if (![pathStr isAbsolutePath]) {
            NSString *cwd = [[NSFileManager defaultManager] currentDirectoryPath];
            pathStr = [cwd stringByAppendingPathComponent:pathStr];
        }

        NSURL *sourceURL = [NSURL fileURLWithPath:pathStr];
        NSError *error = nil;
        NSURL *compiledURL = nil;

        NSFileManager *fm = [NSFileManager defaultManager];
        if (![fm fileExistsAtPath:pathStr]) {
            set_error(error_out, [NSString stringWithFormat:@"Model path does not exist: %@", pathStr]);
            [g_model_lock unlock];
            return 2;
        }

        if ([pathStr hasSuffix:@".mlmodelc"]) {
            if (!is_valid_mlmodelc(fm, pathStr)) {
                set_error(error_out, [NSString stringWithFormat:@"Invalid .mlmodelc directory at %@", pathStr]);
                [g_model_lock unlock];
                return 3;
            }
            compiledURL = sourceURL;
        } else {
            // Check if valid cached .mlmodelc exists alongside .mlpackage
            NSString *cachedCPath = [[pathStr stringByDeletingPathExtension] stringByAppendingPathExtension:@"mlmodelc"];
            if (is_valid_mlmodelc(fm, cachedCPath)) {
                compiledURL = [NSURL fileURLWithPath:cachedCPath];
            } else {
                // If an invalid stub exists, remove it first
                if ([fm fileExistsAtPath:cachedCPath]) {
                    [fm removeItemAtPath:cachedCPath error:nil];
                }

                // Compile the model now
                NSURL *tempCompiledURL = [MLModel compileModelAtURL:sourceURL error:&error];
                if (error != nil || tempCompiledURL == nil) {
                    set_error(error_out, [NSString stringWithFormat:@"CoreML compile failed: %@", [error localizedDescription]]);
                    [g_model_lock unlock];
                    return 4;
                }

                // Try to cache the compiled model alongside source model
                NSError *copyErr = nil;
                if ([fm copyItemAtURL:tempCompiledURL toURL:[NSURL fileURLWithPath:cachedCPath] error:&copyErr]) {
                    compiledURL = [NSURL fileURLWithPath:cachedCPath];
                } else {
                    compiledURL = tempCompiledURL;
                }
            }
        }

        MLModelConfiguration *config = [[MLModelConfiguration alloc] init];
        // Allow CoreML to run on Apple Neural Engine (ANE), GPU, and CPU
        config.computeUnits = MLComputeUnitsAll;

        MLModel *loadedModel = [MLModel modelWithContentsOfURL:compiledURL configuration:config error:&error];
        if (error != nil || loadedModel == nil) {
            set_error(error_out, [NSString stringWithFormat:@"CoreML load failed: %@", [error localizedDescription]]);
            [g_model_lock unlock];
            return 5;
        }

        g_coreml_model = loadedModel;
    }
    [g_model_lock unlock];
    return 0;
}

static int do_embed(const uint8_t *data, int len, float *out_vec, char **error_out) {
    NSData *imgData = [NSData dataWithBytesNoCopy:(void *)data length:len freeWhenDone:NO];
    CGImageSourceRef src = CGImageSourceCreateWithData((__bridge CFDataRef)imgData, NULL);
    if (!src) {
        set_error(error_out, @"ImageIO failed to create image source");
        return 3;
    }

    CGImageRef imgRef = CGImageSourceCreateImageAtIndex(src, 0, NULL);
    CFRelease(src);
    if (!imgRef) {
        set_error(error_out, @"ImageIO failed to decode image frame");
        return 4;
    }

    size_t width = 224;
    size_t height = 224;
    CGColorSpaceRef colorSpace = CGColorSpaceCreateDeviceRGB();
    uint8_t *rawData = (uint8_t *)calloc(height * width * 4, sizeof(uint8_t));
    if (!rawData) {
        CGColorSpaceRelease(colorSpace);
        CGImageRelease(imgRef);
        set_error(error_out, @"Out of memory allocating bitmap buffer");
        return 5;
    }

    CGContextRef ctx = CGBitmapContextCreate(rawData, width, height, 8, width * 4, colorSpace, kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big);
    CGColorSpaceRelease(colorSpace);

    if (!ctx) {
        free(rawData);
        CGImageRelease(imgRef);
        set_error(error_out, @"Failed to create CGBitmapContext");
        return 6;
    }

    CGContextSetInterpolationQuality(ctx, kCGInterpolationHigh);
    CGContextDrawImage(ctx, CGRectMake(0, 0, width, height), imgRef);
    CGImageRelease(imgRef);
    CGContextRelease(ctx);

    NSError *error = nil;
    NSArray<NSNumber *> *shape = @[@1, @3, @224, @224];
    MLMultiArray *pixelValues = [[MLMultiArray alloc] initWithShape:shape dataType:MLMultiArrayDataTypeFloat32 error:&error];
    if (error != nil || pixelValues == nil) {
        free(rawData);
        set_error(error_out, [NSString stringWithFormat:@"Failed to create MLMultiArray: %@", [error localizedDescription]]);
        return 7;
    }

    float *inputPtr = (float *)pixelValues.dataPointer;
    int planeSize = 224 * 224;
    float *planeR = inputPtr;
    float *planeG = inputPtr + planeSize;
    float *planeB = inputPtr + planeSize * 2;

    for (int y = 0; y < 224; y++) {
        for (int x = 0; x < 224; x++) {
            int srcIdx = (y * 224 + x) * 4;
            int dstIdx = y * 224 + x;

            float r = ((float)rawData[srcIdx]) / 255.0f;
            float g = ((float)rawData[srcIdx + 1]) / 255.0f;
            float b = ((float)rawData[srcIdx + 2]) / 255.0f;

            planeR[dstIdx] = (r - CLIP_MEAN[0]) / CLIP_STD[0];
            planeG[dstIdx] = (g - CLIP_MEAN[1]) / CLIP_STD[1];
            planeB[dstIdx] = (b - CLIP_MEAN[2]) / CLIP_STD[2];
        }
    }
    free(rawData);

    MLDictionaryFeatureProvider *input = [[MLDictionaryFeatureProvider alloc] initWithDictionary:@{@"pixel_values": pixelValues} error:&error];
    if (error != nil || input == nil) {
        set_error(error_out, [NSString stringWithFormat:@"Failed to create feature provider: %@", [error localizedDescription]]);
        return 8;
    }

    id<MLFeatureProvider> output = [g_coreml_model predictionFromFeatures:input error:&error];
    if (error != nil || output == nil) {
        set_error(error_out, [NSString stringWithFormat:@"CoreML prediction failed: %@", [error localizedDescription]]);
        return 9;
    }

    MLFeatureValue *embedVal = [output featureValueForName:@"image_embeds"];
    if (embedVal == nil) {
        set_error(error_out, @"Feature 'image_embeds' not found in model output");
        return 10;
    }

    MLMultiArray *embedArr = [embedVal multiArrayValue];
    if (embedArr == nil || embedArr.count < 512) {
        set_error(error_out, @"Invalid output dimension in image_embeds");
        return 11;
    }

    float *vec = (float *)embedArr.dataPointer;
    float norm = 0.0f;
    for (int i = 0; i < 512; i++) {
        norm += vec[i] * vec[i];
    }
    norm = sqrtf(norm);
    if (norm > 1e-12f) {
        for (int i = 0; i < 512; i++) {
            out_vec[i] = vec[i] / norm;
        }
    } else {
        for (int i = 0; i < 512; i++) {
            out_vec[i] = 0.0f;
        }
    }

    return 0;
}

int coreml_embed_image(const uint8_t *data, int len, float *out_vec, char **error_out) {
    if (error_out != NULL) {
        *error_out = NULL;
    }

    if (g_model_lock == nil || g_coreml_model == nil) {
        set_error(error_out, @"CoreML model not initialized");
        return 1;
    }

    if (data == NULL || len <= 0 || out_vec == NULL) {
        set_error(error_out, @"Invalid arguments passed to coreml_embed_image");
        return 2;
    }

    [g_model_lock lock];
    int res = 0;
    @autoreleasepool {
        res = do_embed(data, len, out_vec, error_out);
    }
    [g_model_lock unlock];
    return res;
}

bool coreml_is_loaded(void) {
    if (g_model_lock == nil) return false;
    [g_model_lock lock];
    bool loaded = (g_coreml_model != nil);
    [g_model_lock unlock];
    return loaded;
}

void coreml_close(void) {
    if (g_model_lock == nil) return;
    [g_model_lock lock];
    @autoreleasepool {
        g_coreml_model = nil;
    }
    [g_model_lock unlock];
}
