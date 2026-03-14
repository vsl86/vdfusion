package utils

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"vdfusion/internal/config"
)

func Resolve(name string) string {
	dataDir := config.GetDefaultDataDir()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	localPath := filepath.Join(dataDir, "bin", name+ext)
	if _, err := os.Stat(localPath); err == nil {
		return localPath
	}

	if path, err := exec.LookPath(name); err == nil {
		return path
	}

	if runtime.GOOS == "darwin" {
		if name == "open" {
			return "/usr/bin/open"
		}

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

	return name
}
