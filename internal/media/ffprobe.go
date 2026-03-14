package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
)

type ffprobeOutput struct {
	Streams []struct {
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Duration  string `json:"duration"`
		CodecName string `json:"codec_name"`
		BitRate   string `json:"bit_rate"`
		AvgFrame  string `json:"avg_frame_rate"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
}

func Probe(ctx context.Context, filePath string) (*Metadata, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-probesize", "10000000",
		"-analyzeduration", "10000000",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,duration,codec_name,bit_rate,avg_frame_rate:format=duration,bit_rate",
		"-of", "json",
		filePath,
	)
	meta := &Metadata{}

	ResetWarnings()

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	meta.Warnings = ResetWarnings()

	var data ffprobeOutput
	if err := json.Unmarshal(output, &data); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	if len(data.Streams) > 0 {
		s := data.Streams[0]
		meta.Width = s.Width
		meta.Height = s.Height
		meta.Codec = s.CodecName
		if d, err := strconv.ParseFloat(s.Duration, 64); err == nil {
			meta.Duration = d
		}
		if br, err := strconv.ParseInt(s.BitRate, 10, 64); err == nil {
			meta.Bitrate = br
		}
		var num, den float64
		if _, err := fmt.Sscanf(s.AvgFrame, "%f/%f", &num, &den); err == nil && den != 0 {
			meta.FPS = num / den
		}
	}

	if meta.Duration == 0 {
		if d, err := strconv.ParseFloat(data.Format.Duration, 64); err == nil {
			meta.Duration = d
		}
	}
	if meta.Bitrate == 0 {
		if br, err := strconv.ParseInt(data.Format.BitRate, 10, 64); err == nil {
			meta.Bitrate = br
		}
	}

	return meta, nil
}
