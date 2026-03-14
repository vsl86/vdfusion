package media

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"vdfusion/internal/config"
	"vdfusion/internal/utils"
)

type DependencyStatus struct {
	FFmpeg  bool `json:"ffmpeg"`
	FFprobe bool `json:"ffprobe"`
	FFplay  bool `json:"ffplay"`
	Missing bool `json:"missing"`
}

func GetBinDir() (string, error) {
	dataDir := config.GetDefaultDataDir()
	binDir := filepath.Join(dataDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", err
	}
	return binDir, nil
}

func CheckDependencies() DependencyStatus {
	status := DependencyStatus{}
	tools := []string{"ffmpeg", "ffprobe", "ffplay"}
	present := make([]bool, 3)

	for i, tool := range tools {
		resolved := utils.Resolve(tool)
		if filepath.IsAbs(resolved) {
			if _, err := os.Stat(resolved); err == nil {
				present[i] = true
				continue
			}
		}

		if _, err := exec.LookPath(tool); err == nil {
			present[i] = true
		}
	}

	status.FFmpeg = present[0]
	status.FFprobe = present[1]
	status.FFplay = present[2]
	status.Missing = !status.FFmpeg || !status.FFprobe || !status.FFplay
	return status
}

func DownloadDependencies(ctx context.Context, progress func(string, float64)) error {
	binDir, err := GetBinDir()
	if err != nil {
		return err
	}

	urls := []string{}
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "arm64" {
			urls = []string{"https://github.com/tordona/ffmpeg-win-arm64/releases/download/v7.1/ffmpeg-7.1-win-arm64-static.zip"}
		} else {
			urls = []string{"https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"}
		}
	case "darwin":
		if runtime.GOARCH == "arm64" {
			urls = []string{
				"https://osxexperts.net/ffmpeg71arm.zip",
				"https://osxexperts.net/ffprobe71arm.zip",
				"https://osxexperts.net/ffplay71arm.zip",
			}
		} else {
			urls = []string{
				"https://evermeet.cx/ffmpeg/get/zip",
				"https://evermeet.cx/ffprobe/get/zip",
				"https://evermeet.cx/ffplay/get/zip",
			}
		}
	default:
		return fmt.Errorf("unsupported OS for auto-download: %s", runtime.GOOS)
	}

	for i, url := range urls {
		progress(fmt.Sprintf("Downloading component %d/%d...", i+1, len(urls)), 0.1+(float64(i)/float64(len(urls))*0.8))
		zipPath := filepath.Join(os.TempDir(), fmt.Sprintf("ffmpeg_download_%d.zip", i))

		if err := downloadFile(ctx, url, zipPath, func(p float64) {
			baseP := 0.1 + (float64(i) / float64(len(urls)) * 0.7)
			stepP := (1.0 / float64(len(urls))) * 0.7
			progress(fmt.Sprintf("Downloading component %d/%d...", i+1, len(urls)), baseP+(p*stepP))
		}); err != nil {
			return fmt.Errorf("download failed for %s: %w", url, err)
		}

		progress(fmt.Sprintf("Extracting component %d/%d...", i+1, len(urls)), 0.1+(float64(i+1)/float64(len(urls))*0.8))
		if err := extractFFmpeg(zipPath, binDir); err != nil {
			os.Remove(zipPath)
			return fmt.Errorf("extraction failed for %s: %w", url, err)
		}
		os.Remove(zipPath)
	}

	progress("Cleanup & Verification...", 0.95)
	return nil
}

func downloadFile(ctx context.Context, url, dest string, progress func(float64)) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	size := resp.ContentLength
	var current int64

	buffer := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			out.Write(buffer[:n])
			current += int64(n)
			if size > 0 {
				progress(float64(current) / float64(size))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func extractFFmpeg(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	targets := map[string]bool{
		"ffmpeg" + ext:  true,
		"ffprobe" + ext: true,
		"ffplay" + ext:  true,
	}

	for _, f := range r.File {
		base := filepath.Base(f.Name)
		if targets[strings.ToLower(base)] {
			rc, err := f.Open()
			if err != nil {
				return err
			}

			destPath := filepath.Join(destDir, base)
			outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				rc.Close()
				return err
			}

			_, err = io.Copy(outFile, rc)
			outFile.Close()
			rc.Close()
			if err != nil {
				return err
			}
			log.Printf("Downloader: Extracted %s to %s", base, destPath)
		}
	}

	return nil
}
