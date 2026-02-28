package media

import (
	"fmt"
	"strings"

	"github.com/asticode/go-astiav"
)

func ProbeNative(filePath string) (*Metadata, error) {
	// 1. Open input
	options := astiav.NewDictionary()
	defer options.Free()
	options.Set("probesize", "10000000", 0)
	options.Set("analyzeduration", "10000000", 0)

	ctx := astiav.AllocFormatContext()
	if err := ctx.OpenInput(filePath, nil, options); err != nil {
		return nil, fmt.Errorf("could not open input: %w", err)
	}
	defer ctx.CloseInput()

	// 2. Register for warnings
	ResetWarnings()

	// 3. Find stream info
	if err := ctx.FindStreamInfo(nil); err != nil {
		return nil, fmt.Errorf("could not find stream info: %w", err)
	}

	meta := &Metadata{
		Warnings: ResetWarnings(),
	}

	// Get duration from format (in microseconds)
	meta.Duration = float64(ctx.Duration()) / float64(astiav.TimeBase)

	// find video stream for resolution and metadata
	for _, s := range ctx.Streams() {
		if s.CodecParameters().MediaType() == astiav.MediaTypeVideo {
			meta.Width = s.CodecParameters().Width()
			meta.Height = s.CodecParameters().Height()
			meta.Codec = s.CodecParameters().CodecID().String()
			// Many IDs start with AV_CODEC_ID_, let's clean it up for UI
			if after, ok := strings.CutPrefix(meta.Codec, "AV_CODEC_ID_"); ok {
				meta.Codec = strings.ToLower(after)
			}
			meta.Bitrate = s.CodecParameters().BitRate()
			if meta.Bitrate == 0 {
				meta.Bitrate = ctx.BitRate()
			}
			if s.AvgFrameRate().Den() != 0 {
				meta.FPS = float64(s.AvgFrameRate().Num()) / float64(s.AvgFrameRate().Den())
			}
			break
		}
	}

	return meta, nil
}
