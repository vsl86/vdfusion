package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"time"

	"github.com/asticode/go-astiav"
)

// ExtractGray32x32Native uses native bindings to extract a 32x32 grayscale frame.
func ExtractGray32x32Native(ctx context.Context, filePath string, timestamp float64) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// 1. Open Input
	options := astiav.NewDictionary()
	defer options.Free()
	options.Set("probesize", "10000000", 0)
	options.Set("analyzeduration", "10000000", 0)

	fCtx := astiav.AllocFormatContext()
	if err := fCtx.OpenInput(filePath, nil, options); err != nil {
		return nil, fmt.Errorf("open input: %w", err)
	}
	defer fCtx.CloseInput()

	if err := fCtx.FindStreamInfo(nil); err != nil {
		return nil, err
	}

	// 2. Find video stream
	var vStream *astiav.Stream
	for _, s := range fCtx.Streams() {
		if s.CodecParameters().MediaType() == astiav.MediaTypeVideo {
			vStream = s
			break
		}
	}
	if vStream == nil {
		return nil, fmt.Errorf("no video stream found")
	}

	// 3. Setup Decoder
	codec := astiav.FindDecoder(vStream.CodecParameters().CodecID())
	if codec == nil {
		return nil, fmt.Errorf("decoder not found")
	}

	cCtx := astiav.AllocCodecContext(codec)
	defer cCtx.Free()
	if err := vStream.CodecParameters().ToCodecContext(cCtx); err != nil {
		return nil, err
	}
	if err := cCtx.Open(codec, nil); err != nil {
		return nil, err
	}

	// 4. Seek to timestamp
	ts := int64(timestamp * float64(vStream.TimeBase().Den()) / float64(vStream.TimeBase().Num()))
	if err := fCtx.SeekFrame(vStream.Index(), ts, astiav.NewSeekFlags(astiav.SeekFlagBackward)); err != nil {
		// If seek fails, we'll just try to read from the beginning
	}

	// 5. Decode and Scale
	packet := astiav.AllocPacket()
	defer packet.Free()
	frame := astiav.AllocFrame()
	defer frame.Free()

	var swsCtx *astiav.SoftwareScaleContext
	defer func() {
		if swsCtx != nil {
			swsCtx.Free()
		}
	}()

	dstFrame := astiav.AllocFrame()
	defer dstFrame.Free()
	dstFrame.SetWidth(32)
	dstFrame.SetHeight(32)
	dstFrame.SetPixelFormat(astiav.PixelFormatGray8)
	if err := dstFrame.AllocBuffer(0); err != nil {
		return nil, err
	}

	readRetries := 0
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		err := fCtx.ReadFrame(packet)
		if err != nil {
			if errors.Is(err, astiav.ErrEof) {
				// Flush decoder on EOF
			} else if errors.Is(err, astiav.ErrEagain) && readRetries < 50 {
				readRetries++
				time.Sleep(10 * time.Millisecond)
				continue
			} else {
				return nil, err
			}
		} else if packet.StreamIndex() != vStream.Index() {
			packet.Unref()
			continue
		}
		readRetries = 0

	sendPacket:
		var sendErr error
		if err == nil {
			sendErr = cCtx.SendPacket(packet)
		} else {
			sendErr = cCtx.SendPacket(nil)
		}

		if sendErr != nil && !errors.Is(sendErr, astiav.ErrEagain) && !errors.Is(sendErr, astiav.ErrEof) {
			if err == nil {
				packet.Unref()
			}
			return nil, sendErr
		}

		receivedFrame := false
		for {
			recErr := cCtx.ReceiveFrame(frame)
			if recErr != nil {
				if errors.Is(recErr, astiav.ErrEagain) || errors.Is(recErr, astiav.ErrEof) {
					break
				}
				if err == nil {
					packet.Unref()
				}
				return nil, recErr
			}
			receivedFrame = true

			// Got a frame! Late-bind swsCtx
			if swsCtx == nil {
				var err error
				swsCtx, err = astiav.CreateSoftwareScaleContext(
					frame.Width(), frame.Height(), frame.PixelFormat(),
					32, 32, astiav.PixelFormatGray8,
					astiav.NewSoftwareScaleContextFlags(astiav.SoftwareScaleContextFlagBilinear),
				)
				if err != nil {
					packet.Unref()
					return nil, err
				}
			}

			if scaleErr := swsCtx.ScaleFrame(frame, dstFrame); scaleErr != nil {
				if err == nil {
					packet.Unref()
				}
				return nil, scaleErr
			}

			data, bytesErr := dstFrame.Data().Bytes(1)
			if bytesErr != nil {
				if err == nil {
					packet.Unref()
				}
				return nil, bytesErr
			}
			if err == nil {
				packet.Unref()
			}
			return data, nil
		}

		if errors.Is(sendErr, astiav.ErrEagain) {
			if !receivedFrame {
				if err == nil {
					packet.Unref()
				}
				return nil, errors.New("decoder deadlock: EAGAIN on both send and receive")
			}
			goto sendPacket
		}

		if err == nil {
			packet.Unref()
		} else {
			break
		}
	}

	return nil, fmt.Errorf("could not extract frame")
}

// ExtractThumbnailNative extracts a thumbnail as a JPEG byte slice.
func ExtractThumbnailNative(ctx context.Context, filePath string, timestamp float64, width, height int) ([]byte, error) {
	// Check context immediately
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// 1. Open Input
	options := astiav.NewDictionary()
	defer options.Free()
	options.Set("probesize", "10000000", 0)
	options.Set("analyzeduration", "10000000", 0)

	fCtx := astiav.AllocFormatContext()
	if err := fCtx.OpenInput(filePath, nil, options); err != nil {
		return nil, fmt.Errorf("open input: %w", err)
	}
	defer fCtx.CloseInput()

	if err := fCtx.FindStreamInfo(nil); err != nil {
		return nil, err
	}

	// 2. Find video stream
	var vStream *astiav.Stream
	for _, s := range fCtx.Streams() {
		if s.CodecParameters().MediaType() == astiav.MediaTypeVideo {
			vStream = s
			break
		}
	}
	if vStream == nil {
		return nil, fmt.Errorf("no video stream found")
	}

	// 3. Setup Decoder
	codec := astiav.FindDecoder(vStream.CodecParameters().CodecID())
	if codec == nil {
		return nil, fmt.Errorf("decoder not found")
	}

	cCtx := astiav.AllocCodecContext(codec)
	defer cCtx.Free()
	if err := vStream.CodecParameters().ToCodecContext(cCtx); err != nil {
		return nil, err
	}
	if err := cCtx.Open(codec, nil); err != nil {
		return nil, err
	}

	// 4. Handle Aspect Ratio and sizing
	if width <= 0 && height <= 0 {
		width, height = 160, 90 // Default
	} else if width > 0 && height > 0 {
		// Fit within limits while preserving aspect ratio
		ratioW := float64(width) / float64(cCtx.Width())
		ratioH := float64(height) / float64(cCtx.Height())
		scale := ratioW
		if ratioH < scale {
			scale = ratioH
		}
		width = int(float64(cCtx.Width()) * scale)
		height = int(float64(cCtx.Height()) * scale)
	} else if width > 0 && height <= 0 {
		height = max((cCtx.Height()*width)/cCtx.Width(), 1)
	} else if height > 0 && width <= 0 {
		width = max((cCtx.Width()*height)/cCtx.Height(), 1)
	}
	// Ensure dimensions are even (good practice for many codecs, even though we use JPEG here)
	width = (width / 2) * 2
	height = (height / 2) * 2
	if width < 2 {
		width = 2
	}
	if height < 2 {
		height = 2
	}

	// 5. Seek to timestamp
	ts := int64(timestamp * float64(vStream.TimeBase().Den()) / float64(vStream.TimeBase().Num()))
	if err := fCtx.SeekFrame(vStream.Index(), ts, astiav.NewSeekFlags(astiav.SeekFlagBackward)); err != nil {
		// If seek fails, we'll just try to read from the beginning
	}

	// 6. Decode and Scale
	packet := astiav.AllocPacket()
	defer packet.Free()
	frame := astiav.AllocFrame()
	defer frame.Free()

	var swsCtx *astiav.SoftwareScaleContext
	defer func() {
		if swsCtx != nil {
			swsCtx.Free()
		}
	}()

	dstFrame := astiav.AllocFrame()
	defer dstFrame.Free()
	dstFrame.SetWidth(width)
	dstFrame.SetHeight(height)
	dstFrame.SetPixelFormat(astiav.PixelFormatRgb24)
	if err := dstFrame.AllocBuffer(0); err != nil {
		return nil, err
	}

	readRetries := 0
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		err := fCtx.ReadFrame(packet)
		if err != nil {
			if errors.Is(err, astiav.ErrEof) {
				// Flush decoder on EOF
			} else if errors.Is(err, astiav.ErrEagain) && readRetries < 50 {
				readRetries++
				time.Sleep(10 * time.Millisecond)
				continue
			} else {
				return nil, err
			}
		} else if packet.StreamIndex() != vStream.Index() {
			packet.Unref()
			continue
		}
		readRetries = 0

	sendPacketThumb:
		var sendErr error
		if err == nil {
			sendErr = cCtx.SendPacket(packet)
		} else {
			sendErr = cCtx.SendPacket(nil)
		}

		if sendErr != nil && !errors.Is(sendErr, astiav.ErrEagain) && !errors.Is(sendErr, astiav.ErrEof) {
			if err == nil {
				packet.Unref()
			}
			return nil, sendErr
		}

		receivedFrame := false
		for {
			recErr := cCtx.ReceiveFrame(frame)
			if recErr != nil {
				if errors.Is(recErr, astiav.ErrEagain) || errors.Is(recErr, astiav.ErrEof) {
					break
				}
				if err == nil {
					packet.Unref()
				}
				return nil, recErr
			}
			receivedFrame = true

			// Got a frame! Late-bind swsCtx
			if swsCtx == nil {
				var err error
				swsCtx, err = astiav.CreateSoftwareScaleContext(
					frame.Width(), frame.Height(), frame.PixelFormat(),
					width, height, astiav.PixelFormatRgb24,
					astiav.NewSoftwareScaleContextFlags(astiav.SoftwareScaleContextFlagBilinear),
				)
				if err != nil {
					packet.Unref()
					return nil, err
				}
			}

			if scaleErr := swsCtx.ScaleFrame(frame, dstFrame); scaleErr != nil {
				if err == nil {
					packet.Unref()
				}
				return nil, scaleErr
			}

			srcData, bytesErr := dstFrame.Data().Bytes(1)
			if bytesErr != nil {
				if err == nil {
					packet.Unref()
				}
				return nil, bytesErr
			}

			img := image.NewNRGBA(image.Rect(0, 0, width, height))
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					i := (y*width + x) * 3
					j := (y*width + x) * 4
					img.Pix[j] = srcData[i]     // R
					img.Pix[j+1] = srcData[i+1] // G
					img.Pix[j+2] = srcData[i+2] // B
					img.Pix[j+3] = 255          // A
				}
			}

			var buf bytes.Buffer
			if encErr := jpeg.Encode(&buf, img, nil); encErr != nil {
				if err == nil {
					packet.Unref()
				}
				return nil, encErr
			}
			if err == nil {
				packet.Unref()
			}
			return buf.Bytes(), nil
		}

		if errors.Is(sendErr, astiav.ErrEagain) {
			if !receivedFrame {
				if err == nil {
					packet.Unref()
				}
				return nil, errors.New("decoder deadlock: EAGAIN on both send and receive")
			}
			goto sendPacketThumb
		}

		if err == nil {
			packet.Unref()
		} else {
			break
		}
	}

	return nil, fmt.Errorf("could not extract thumbnail")
}
