package neural

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	ModelFileName     = "clip-vit-b32-visual_fp16_bs1.mlpackage"
	ModelCompiledName = "clip-vit-b32-visual_fp16_bs1.mlmodelc"
	ModelZipURL       = "https://github.com/vsl86/vdfusion/releases/download/v1.2.0/clip-vit-b32-visual_fp16_bs1.zip"
)

var (
	downloadMu         sync.Mutex
	isDownloading      bool
	downloadPercent    float64
	downloadStatusText string
)

type DownloadProgress struct {
	Active     bool    `json:"active"`
	Downloaded int64   `json:"downloaded"`
	Total      int64   `json:"total"`
	Percent    float64 `json:"percent"`
	Status     string  `json:"status"`
	Error      string  `json:"error,omitempty"`
}

// GetModelDir returns the directory where neural models are stored.
func GetModelDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Library", "Application Support", "vdfusion", "models")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// GetModelPath returns the path to the model if it exists, checking:
// 1. App Support directory (~/Library/Application Support/vdfusion/models/)
// 2. Local dev repository path (neural/models/ walking up parent dirs)
func GetModelPath() string {
	// 1. App Support
	dir, err := GetModelDir()
	if err == nil {
		pkg := filepath.Join(dir, ModelFileName)
		if fi, err := os.Stat(pkg); err == nil && fi.IsDir() {
			return pkg
		}
		c := filepath.Join(dir, ModelCompiledName)
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c
		}
	}

	// 2. Walk up parent directories to find repo's neural/models
	searchDir := "."
	for i := 0; i < 5; i++ {
		candidate := filepath.Join(searchDir, "neural", "models", ModelFileName)
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			abs, err := filepath.Abs(candidate)
			if err == nil {
				return abs
			}
			return candidate
		}
		candidateC := filepath.Join(searchDir, "neural", "models", ModelCompiledName)
		if fi, err := os.Stat(candidateC); err == nil && fi.IsDir() {
			abs, err := filepath.Abs(candidateC)
			if err == nil {
				return abs
			}
			return candidateC
		}
		searchDir = filepath.Join(searchDir, "..")
	}

	return ""
}

// IsModelDownloaded returns true if a model package is available on disk.
func IsModelDownloaded() bool {
	return GetModelPath() != ""
}

// GetDownloadProgress returns the current model download progress.
func GetDownloadProgress() DownloadProgress {
	downloadMu.Lock()
	defer downloadMu.Unlock()
	return DownloadProgress{
		Active:  isDownloading,
		Percent: downloadPercent,
		Status:  downloadStatusText,
	}
}

// DownloadModel downloads the zip package from GitHub Release and extracts it.
func DownloadModel(progressCb func(p DownloadProgress)) error {
	downloadMu.Lock()
	if isDownloading {
		downloadMu.Unlock()
		return fmt.Errorf("download already in progress")
	}
	isDownloading = true
	downloadPercent = 0
	downloadStatusText = "Connecting..."
	downloadMu.Unlock()

	defer func() {
		downloadMu.Lock()
		isDownloading = false
		downloadMu.Unlock()
	}()

	updateProgress := func(downloaded, total int64, status string, errStr string) {
		downloadMu.Lock()
		if total > 0 {
			downloadPercent = float64(downloaded) / float64(total) * 100.0
		}
		downloadStatusText = status
		p := DownloadProgress{
			Active:     isDownloading,
			Downloaded: downloaded,
			Total:      total,
			Percent:    downloadPercent,
			Status:     status,
			Error:      errStr,
		}
		downloadMu.Unlock()
		if progressCb != nil {
			progressCb(p)
		}
	}

	targetDir, err := GetModelDir()
	if err != nil {
		updateProgress(0, 0, "Failed", err.Error())
		return err
	}

	tmpZipPath := filepath.Join(targetDir, "model.zip.download")
	_ = os.Remove(tmpZipPath)

	updateProgress(0, 0, "Downloading model archive (~155 MB)...", "")

	resp, err := http.Get(ModelZipURL)
	if err != nil {
		updateProgress(0, 0, "Network error", err.Error())
		return fmt.Errorf("failed to download model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("download returned HTTP %d", resp.StatusCode)
		updateProgress(0, 0, "Download failed", err.Error())
		return err
	}

	outFile, err := os.Create(tmpZipPath)
	if err != nil {
		updateProgress(0, 0, "File creation error", err.Error())
		return err
	}

	totalBytes := resp.ContentLength
	var downloadedBytes int64

	buf := make([]byte, 64*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := outFile.Write(buf[:n]); werr != nil {
				outFile.Close()
				_ = os.Remove(tmpZipPath)
				updateProgress(downloadedBytes, totalBytes, "Write error", werr.Error())
				return werr
			}
			downloadedBytes += int64(n)
			updateProgress(downloadedBytes, totalBytes, fmt.Sprintf("Downloading... %.1f%%", float64(downloadedBytes)/float64(totalBytes)*100.0), "")
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			outFile.Close()
			_ = os.Remove(tmpZipPath)
			updateProgress(downloadedBytes, totalBytes, "Download interrupted", rerr.Error())
			return rerr
		}
	}
	outFile.Close()

	updateProgress(downloadedBytes, totalBytes, "Extracting model package...", "")

	// Extract ZIP
	r, err := zip.OpenReader(tmpZipPath)
	if err != nil {
		_ = os.Remove(tmpZipPath)
		updateProgress(downloadedBytes, totalBytes, "Corrupt zip archive", err.Error())
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		cleanPath := filepath.Clean(f.Name)
		if strings.HasPrefix(cleanPath, "..") || strings.HasPrefix(cleanPath, "/") {
			continue
		}
		destPath := filepath.Join(targetDir, cleanPath)

		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(destPath, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		dst, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(dst, rc)
		dst.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}

	_ = os.Remove(tmpZipPath)
	updateProgress(totalBytes, totalBytes, "Ready", "")
	return nil
}
