package media

import (
	"slices"
	"strings"
	"sync"

	"github.com/asticode/go-astiav"
)

var (
	logWarnings []string
	logMu       sync.Mutex
	logOnce     sync.Once
)

func initLogger() {
	astiav.SetLogCallback(func(c astiav.Classer, level astiav.LogLevel, fmt, msg string) {
		if level > astiav.LogLevelWarning {
			return
		}

		if !isInteresting(msg) {
			return
		}

		trimmed := strings.TrimSpace(msg)
		if trimmed == "" {
			return
		}

		logMu.Lock()
		defer logMu.Unlock()

		if slices.Contains(logWarnings, trimmed) {
			return
		}
		logWarnings = append(logWarnings, trimmed)
	})
}

func isInteresting(msg string) bool {
	interesting := []string{
		"packed B-frames",     // Wasteful encoding, should use bsf to fix
		"non-interleaved AVI", // May cause playback issues
		"non-standard",        // Non-standard encoding
		"invalid data",        // Corrupt data
		"error decoding",      // Decode errors
		"corrupt",             // Corruption
	}
	lower := strings.ToLower(msg)
	for _, term := range interesting {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func ResetWarnings() []string {
	logOnce.Do(initLogger)
	logMu.Lock()
	defer logMu.Unlock()
	w := logWarnings
	logWarnings = nil
	return w
}
