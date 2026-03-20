package engine

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"vdfusion/internal/config"
	"vdfusion/internal/db"
)

type ProgressReporter interface {
	BroadcastProgress(current, total int, phase string, lastFile string, durationSeconds, estimatedRemainingSeconds float64)
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
	stopping       bool
	current        int
	total          int
	phase          string
	lastFile       string
	startTime      time.Time
	lastDuration   float64
	etaSeconds     float64
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
	if s.running || s.stopping {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopping = false
	s.phase = "discovery"
	s.current = 0
	s.total = 0
	s.lastFile = ""
	s.startTime = time.Now()

	scanCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
		startTime := time.Now()
		log.Printf("Scanner: Starting scan for paths: %v", paths)

		defer func() {
			s.mu.Lock()
			s.running = false
			s.stopping = false
			s.mu.Unlock()
			duration := time.Since(startTime).Seconds()
			log.Printf("Scanner: Scan finished (duration: %.2fs)", duration)
		}()

		_, err := s.walker.IndexPaths(scanCtx, paths, cfg)
		duration := time.Since(startTime).Seconds()
		if err != nil {
			log.Printf("Scanner: Error during indexing: %v", err)
			s.BroadcastProgress(0, 0, "error: "+err.Error(), "", duration, 0)
			return
		}

		if scanCtx.Err() != nil {
			return
		}

		log.Printf("Scanner: Starting comparison phase...")
		s.BroadcastProgress(0, 0, "comparing", "Loading files...", duration, 0)

		files, err := s.db.GetFilesByPrefixes(paths)
		if err != nil {
			log.Printf("Scanner: Failed to load files: %v", err)
			s.BroadcastProgress(0, 0, "error: "+err.Error(), "", duration, 0)
			return
		}

		var validFiles []db.FileRecord
		orphanCount := 0
		filteredCount := 0
		for _, f := range files {
			isFake := strings.HasPrefix(f.Path, "/fake_")

			_, statErr := os.Stat(f.Path)
			if isFake || statErr == nil {
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
				if cfg.CleanupOrphans {
					_ = s.db.DeleteFile(f.Path)
					orphanCount++
				}
			}
		}
		if orphanCount > 0 {
			msg := fmt.Sprintf("Cleaned %d orphaned records (files missing on disk)", orphanCount)
			log.Printf("Scanner: %s", msg)
			s.BroadcastLog("info", msg)
		}
		if filteredCount > 0 {
			log.Printf("Scanner: Filtered %d files by size constraints", filteredCount)
		}

		if scanCtx.Err() != nil {
			return
		}

		var results []DuplicateGroup

		logMsg := fmt.Sprintf("Comparison Phase: Files=%d", len(validFiles))
		log.Printf("Scanner: %s", logMsg)
		s.BroadcastLog("info", logMsg)

		missingHashes := 0
		for _, f := range validFiles {
			if len(f.PHashV2s) == 0 {
				missingHashes++
			}
		}
		if missingHashes > 0 {
			healthMsg := fmt.Sprintf("Data Health: %d files have no hashes and will be ignored in comparison.", missingHashes)
			log.Printf("Scanner: %s", healthMsg)
			s.BroadcastLog("warning", healthMsg)
		}

		ignoredGroups, err := s.db.GetIgnoredGroups()
		if err != nil {
			ignoredGroups = []db.IgnoredGroup{}
		}

		results = s.compare.Compare(scanCtx, validFiles, ignoredGroups, cfg, s)

		if scanCtx.Err() != nil {
			return
		}

		s.resultsManager.SetResults(results)
		compSummary := fmt.Sprintf("Comparison complete. Found %d duplicate groups.", len(results))
		log.Printf("Scanner: %s", compSummary)
		s.BroadcastLog("success", compSummary)
		s.BroadcastProgress(len(validFiles), len(validFiles), "completed", "Finished", duration, 0)
	}()
}

func (s *Scanner) BroadcastProgress(current, total int, phase string, lastFile string, durationSeconds, estimatedRemainingSeconds float64) {
	s.mu.Lock()
	s.current = current
	s.total = total
	s.phase = phase
	s.lastFile = lastFile
	s.lastDuration = durationSeconds
	s.etaSeconds = estimatedRemainingSeconds
	s.mu.Unlock()

	if s.reporter != nil {
		s.reporter.BroadcastProgress(current, total, phase, lastFile, durationSeconds, estimatedRemainingSeconds)
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
		"stopping":         s.stopping,
		"current":          s.current,
		"total":            s.total,
		"phase":            s.phase,
		"last_file":        s.lastFile,
		"duration_seconds": duration,
		"eta_seconds":      s.etaSeconds,
	}
}

func (s *Scanner) Stop() {
	s.mu.Lock()
	if !s.running || s.stopping {
		s.mu.Unlock()
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.stopping = true
	s.phase = "stopping"
	s.etaSeconds = 0
	s.mu.Unlock()

	s.BroadcastProgress(s.current, s.total, "stopping", s.lastFile, s.lastDuration, 0)
}
