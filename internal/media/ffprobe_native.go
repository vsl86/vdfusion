package media

import (
	"fmt"
	"strings"

	"github.com/asticode/go-astiav"
)

func ProbeNative(filePath string) (*Metadata, error) {
	options := astiav.NewDictionary()
	defer options.Free()
	options.Set("probesize", "10000000", 0)
	options.Set("analyzeduration", "10000000", 0)

	ctx := astiav.AllocFormatContext()
	if err := ctx.OpenInput(filePath, nil, options); err != nil {
		return nil, fmt.Errorf("could not open input: %w", err)
	}
	defer ctx.CloseInput()

	ResetWarnings()

	if err := ctx.FindStreamInfo(nil); err != nil {
		return nil, fmt.Errorf("could not find stream info: %w", err)
	}

	meta := &Metadata{
		Warnings: ResetWarnings(),
	}

	meta.Duration = float64(ctx.Duration()) / float64(astiav.TimeBase)

	for _, s := range ctx.Streams() {
		if s.CodecParameters().MediaType() == astiav.MediaTypeVideo {
			meta.Width = s.CodecParameters().Width()
			meta.Height = s.CodecParameters().Height()
			meta.Codec = s.CodecParameters().CodecID().String()
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
