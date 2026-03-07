package media

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"

	"vdfusion/internal/utils"
)

// ExtractGray32x32 extracts a single frame at the specified timestamp
// and returns it as a 32x32 Gray8 raw buffer (1024 bytes).
func ExtractGray32x32(ctx context.Context, filePath string, timestamp float64) ([]byte, error) {
	cmd := exec.CommandContext(ctx, utils.Resolve("ffmpeg"),
		"-ss", fmt.Sprintf("%f", timestamp),
		"-i", filePath,
		"-frames:v", "1",
		"-s", "32x32",
		"-f", "rawvideo",
		"-pix_fmt", "gray8",
		"-v", "error",
		"pipe:1",
	)

	var stderrMsg bytes.Buffer
	cmd.Stderr = &stderrMsg

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	buf := make([]byte, 1024)
	_, err = io.ReadFull(stdout, buf)
	if err != nil {
		cmd.Process.Kill() // Ensure ffmpeg is killed if read fails
		errStr := stderrMsg.String()
		if len(errStr) > 200 {
			errStr = errStr[:200]
		}
		return nil, fmt.Errorf("failed to read 1024 bytes from ffmpeg: %w, stderr: %s", err, errStr)
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("ffmpeg finished with error: %w, stderr: %s", err, stderrMsg.String())
	}

	return buf, nil
}

// ExtractThumbnail extracts a single frame at the specified timestamp
// and returns it as a JPEG byte slice.
func ExtractThumbnail(ctx context.Context, filePath string, timestamp float64, width, height int) ([]byte, error) {
	if width <= 0 && height <= 0 {
		width, height = 160, 90 // Default
	} else if width > 0 && height <= 0 {
		// allow ffmpeg to preserve aspect ratio
		height = -1
	} else if height > 0 && width <= 0 {
		width = -1
	}

	scaleArg := fmt.Sprintf("%d:%d:force_original_aspect_ratio=decrease", width, height)
	if width == -1 {
		scaleArg = fmt.Sprintf("-1:%d", height)
	} else if height == -1 {
		scaleArg = fmt.Sprintf("%d:-1", width)
	}

	cmd := exec.CommandContext(ctx, utils.Resolve("ffmpeg"),
		"-ss", fmt.Sprintf("%f", timestamp),
		"-i", filePath,
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%s", scaleArg),
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-v", "error",
		"pipe:1",
	)

	var stderrMsg bytes.Buffer
	cmd.Stderr = &stderrMsg

	out, err := cmd.Output()
	if err != nil {
		errStr := stderrMsg.String()
		if len(errStr) > 200 {
			errStr = errStr[:200]
		}
		return nil, fmt.Errorf("ffmpeg thumbnail extraction failed: %w, stderr: %s", err, errStr)
	}
	return out, nil
}

// StreamTranscoded starts an ffmpeg process to transcode a file to a streamable MP4 (H.264/AAC).
// It returns a reader for the stdout and the command object (so it can be killed).
func StreamTranscoded(ctx context.Context, filePath string) (io.ReadCloser, *exec.Cmd, error) {
	// We use fragmented MP4 for streaming via HTTP.
	// -movflags frag_keyframe+empty_moov+default_base_moof allows streaming without a seekable output.
	cmd := exec.CommandContext(ctx, utils.Resolve("ffmpeg"),
		"-i", filePath,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "28", // Lower quality for faster transcoding
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "frag_keyframe+empty_moov+default_base_moof",
		"-f", "mp4",
		"-v", "error",
		"pipe:1",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	return stdout, cmd, nil
}
