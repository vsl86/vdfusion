package utils

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"vdfusion/internal/config"
)

// Resolve returns the absolute path to a command by checking common locations.
// This is especially useful on macOS app bundles where the PATH might be minimal.
func Resolve(name string) string {
	// 0. Check AppData/bin (where our downloader puts them)
	dataDir := config.GetDefaultDataDir()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	localPath := filepath.Join(dataDir, "bin", name+ext)
	if _, err := os.Stat(localPath); err == nil {
		return localPath
	}

	// 1. Try LookPath first (standard PATH check)
	if path, err := exec.LookPath(name); err == nil {
		return path
	}

	// 2. Platform-specific hardcoded lookups
	if runtime.GOOS == "darwin" {
		if name == "open" {
			return "/usr/bin/open"
		}

		// Common Homebrew/MacPorts/System paths for FFmpeg tools
		commonPaths := []string{
			"/opt/homebrew/bin",
			"/usr/local/bin",
			"/usr/bin",
			"/bin",
			"/usr/sbin",
			"/sbin",
			"/usr/local/opt/ffmpeg/bin",
		}

		for _, p := range commonPaths {
			fullPath := filepath.Join(p, name)
			if _, err := os.Stat(fullPath); err == nil {
				return fullPath
			}
		}
	}

	// 3. Fallback to original name and let exec.Command handle it (and likely fail)
	return name
}
