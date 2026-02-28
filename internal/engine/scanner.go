package engine

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"vdfusion/internal/config"
	"vdfusion/internal/db"
)

type ProgressReporter interface {
	BroadcastProgress(current, total int, phase string, lastFile string, durationSeconds float64)
	BroadcastLog(severity, message string)
}

type Scanner struct {
	walker         *Walker
	db             *db.Database
	reporter       ProgressReporter
	compare        *ComparisonEngine
	resultsManager *ResultsManager
	cancel         context.CancelFunc
	mu             sync.Mutex
	running        bool

	// State for status recovery
	current      int
	total        int
	phase        string
	lastFile     string
	startTime    time.Time
	lastDuration float64
}

func NewScanner(walker *Walker, db *db.Database, reporter ProgressReporter, compare *ComparisonEngine, resultsManager *ResultsManager) *Scanner {
	return &Scanner{
		walker:         walker,
		db:             db,
		reporter:       reporter,
		compare:        compare,
		resultsManager: resultsManager,
	}
}

func (s *Scanner) Start(ctx context.Context, paths []string, cfg config.Settings) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.phase = "discovery"
	s.current = 0
	s.total = 0
	s.lastFile = ""
	s.startTime = time.Now()

	// Clear previous results
	if s.resultsManager != nil {
		s.resultsManager.Clear()
	}

	scanCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
		startTime := time.Now()
		log.Printf("Scanner: Starting scan for paths: %v", paths)

		defer func() {
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			duration := time.Since(startTime).Seconds()
			log.Printf("Scanner: Scan finished (duration: %.2fs)", duration)
		}()

		err := s.walker.IndexPaths(scanCtx, paths, cfg)
		duration := time.Since(startTime).Seconds()
		if err != nil {
			log.Printf("Scanner: Error during indexing: %v", err)
			s.BroadcastProgress(0, 0, "error: "+err.Error(), "", duration)
			return
		}

		// Check for cancellation before comparison
		if scanCtx.Err() != nil {
			return
		}

		// Phase 3: Comparison
		log.Printf("Scanner: Starting comparison phase...")
		s.BroadcastProgress(0, 0, "comparing", "Loading files...", duration)

		files, err := s.db.GetFilesByPrefixes(paths)
		if err != nil {
			log.Printf("Scanner: Failed to load files: %v", err)
			s.BroadcastProgress(0, 0, "error: "+err.Error(), "", duration)
			return
		}

		// Scope-independent orphan cleanup: drop non-existent files and remove from DB
		// Also apply file-size filters from settings so comparison scope matches user expectations.
		var validFiles []db.FileRecord
		orphanCount := 0
		filteredCount := 0
		for _, f := range files {
			// Bypass stat check for fakegen files (which don't exist on disk)
			isFake := strings.HasPrefix(f.Path, "/fake_")

			_, statErr := os.Stat(f.Path)
			if isFake || statErr == nil {
				// Apply file size filter
				if cfg.FilterByFileSize {
					if cfg.MinimumFileSize > 0 && f.Size < cfg.MinimumFileSize {
						filteredCount++
						continue
					}
					if cfg.MaximumFileSize > 0 && f.Size > cfg.MaximumFileSize {
						filteredCount++
						continue
					}
				}
				validFiles = append(validFiles, f)
			} else {
				// File deleted from disk, remove from DB
				_ = s.db.DeleteFile(f.Path)
				orphanCount++
			}
		}
		if orphanCount > 0 {
			log.Printf("Scanner: Cleaned %d orphaned records (files missing on disk)", orphanCount)
		}
		if filteredCount > 0 {
			log.Printf("Scanner: Filtered %d files by size constraints", filteredCount)
		}

		// Check for cancellation after DB load
		if scanCtx.Err() != nil {
			return
		}

		log.Printf("Scanner: Starting comparison phase with %d files...", len(validFiles))
		ignoredGroups, err := s.db.GetIgnoredGroups()
		if err != nil {
			ignoredGroups = []db.IgnoredGroup{}
		}

		results := s.compare.Compare(scanCtx, validFiles, ignoredGroups, cfg, s)
		// Check for cancellation after comparison
		if scanCtx.Err() != nil {
			return
		}

		s.resultsManager.SetResults(results)
		log.Printf("Scanner: Comparison complete. Found %d duplicate groups. Results updated.", len(results))

		s.BroadcastProgress(100, 100, "completed", "", time.Since(startTime).Seconds())
	}()
}

func (s *Scanner) BroadcastProgress(current, total int, phase string, lastFile string, durationSeconds float64) {
	s.mu.Lock()
	s.current = current
	s.total = total
	s.phase = phase
	s.lastFile = lastFile
	s.lastDuration = durationSeconds
	s.mu.Unlock()

	if s.reporter != nil {
		s.reporter.BroadcastProgress(current, total, phase, lastFile, durationSeconds)
	}
}

func (s *Scanner) BroadcastLog(severity, message string) {
	if s.reporter != nil {
		s.reporter.BroadcastLog(severity, message)
	}
}

func (s *Scanner) Status() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	duration := s.lastDuration
	if s.running {
		duration = time.Since(s.startTime).Seconds()
	}

	return map[string]any{
		"running":          s.running,
		"current":          s.current,
		"total":            s.total,
		"phase":            s.phase,
		"last_file":        s.lastFile,
		"duration_seconds": duration,
	}
}

func (s *Scanner) Stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.running = false
	s.phase = "stopped"
	s.mu.Unlock()

	// Broadcast the stopped state immediately
	s.BroadcastProgress(s.current, s.total, "stopped", s.lastFile, s.lastDuration)
}
