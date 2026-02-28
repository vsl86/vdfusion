package engine

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"vdfusion/internal/config"
	"vdfusion/internal/db"
	"vdfusion/internal/media"
	"vdfusion/internal/phash"
)

type Walker struct {
	db       *db.Database
	reporter ProgressReporter
}

func NewWalker(d *db.Database, reporter ProgressReporter) *Walker {
	return &Walker{db: d, reporter: reporter}
}

func (w *Walker) SetReporter(reporter ProgressReporter) {
	w.reporter = reporter
}

// IndexPaths walks the given directories and indexes video files into the database.
func (w *Walker) IndexPaths(ctx context.Context, paths []string, cfg config.Settings) error {
	startTime := time.Now()

	// ── Phase 1: Discovery ──────────────────────────────────────────────
	// Walk all directories first to get the full file list.
	w.report(0, 0, "discovery", "", startTime)

	var allFiles []string
	var mu sync.Mutex
	var discoveryWg sync.WaitGroup
	var discoveredCount int64

	for _, p := range paths {
		log.Printf("Walker: Scanning directory: %s", p)
		discoveryWg.Add(1)
		go func(root string) {
			defer discoveryWg.Done()
			_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if d.IsDir() {
					return nil
				}
				if isBlacklisted(path, cfg.BlackList) {
					return nil
				}
				info, err := d.Info()
				if err == nil && cfg.FilterByFileSize {
					size := info.Size()
					if cfg.MinimumFileSize > 0 && size < cfg.MinimumFileSize {
						return nil
					}
					if cfg.MaximumFileSize > 0 && size > cfg.MaximumFileSize {
						return nil
					}
				}
				ext := strings.ToLower(filepath.Ext(path))
				if isVideoExt(ext) {
					n := atomic.AddInt64(&discoveredCount, 1)
					mu.Lock()
					allFiles = append(allFiles, path)
					mu.Unlock()
					if n%100 == 0 {
						w.report(0, int(n), "discovery", path, startTime)
					}
				}
				return nil
			})
		}(p)
	}

	discoveryWg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}

	totalFiles := len(allFiles)

	discoveredMap := make(map[string]bool, len(allFiles))
	for _, f := range allFiles {
		discoveredMap[f] = true
	}

	existingCount := 0
	removedCount := 0
	// Get all files from DB that are under the scanned paths to find "removed"
	existingRecords, _ := w.db.GetFilesByPrefixes(paths)
	for _, r := range existingRecords {
		if discoveredMap[r.Path] {
			existingCount++
		} else {
			removedCount++
		}
	}

	newFilesCount := totalFiles - existingCount
	summary := fmt.Sprintf("Discovery: Found %d files (%d new, %d already indexed, %d removed from disk)", totalFiles, newFilesCount, existingCount, removedCount)
	log.Printf("Walker: %s", summary)
	if w.reporter != nil {
		w.reporter.BroadcastLog("info", summary)
	}

	w.report(0, totalFiles, "scanning", "", startTime)

	// ── Phase 2: Processing ─────────────────────────────────────────────
	// Feed the known list to workers with an accurate total.
	var processedCount int64
	var workerWg sync.WaitGroup
	filesChan := make(chan string, 100)

	numWorkers := cfg.Concurrency
	if numWorkers < 1 {
		numWorkers = 4
	}
	for i := 0; i < numWorkers; i++ {
		workerWg.Add(1)
		workerID := i
		go func(id int) {
			defer workerWg.Done()
			for path := range filesChan {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if cfg.DebugLogging {
					log.Printf("Walker: [Worker %d] Processing %s", id, path)
				}

				fileInfo, err := os.Stat(path)
				if err != nil {
					if cfg.DebugLogging {
						log.Printf("Walker: [Worker %d] Stat error for %s: %v", id, path, err)
					}
					atomic.AddInt64(&processedCount, 1)
					continue
				}

				thumbCount := cfg.Thumbnails
				if thumbCount < 1 {
					thumbCount = 4
				}

				// 1. Check if we already have this exact path/size/mtime indexed
				existing, _ := w.db.GetFileByPath(path)
				shouldSkip := existing != nil && existing.Size == fileInfo.Size() && existing.Modified == fileInfo.ModTime().Unix()
				if shouldSkip && cfg.RecheckSuspicious && len(existing.Warnings) > 0 {
					log.Printf("Walker: Forcing re-probe of suspicious file: %s", path)
					shouldSkip = false
				}

				if shouldSkip {
					// We have a result. Can we reuse or upgrade it?
					if len(existing.PHashV2s) >= thumbCount {
						// Current target is satisfied by existing data.
						// We don't need to re-extract, but we might want to ensure metadata is present.
						if existing.Codec != "" && existing.Width > 0 && existing.Height > 0 {
							newProcessed := atomic.AddInt64(&processedCount, 1)
							w.throttledReport(int(newProcessed), totalFiles, "scanning", path, startTime)
							continue
						}
						// Metadata missing -> Enrich
						if cfg.DebugLogging {
							log.Printf("Walker: Enriching metadata for %s", path)
						}
						meta, err := media.ProbeNative(path)
						if err != nil {
							meta, _ = media.Probe(ctx, path)
						}
						if meta != nil && meta.Codec != "" {
							err = w.db.UpsertFile(path, existing.Size, existing.Modified, meta.Duration, meta.Width, meta.Height, existing.PHashV2s, meta.Codec, meta.Bitrate, meta.FPS, meta.Warnings)
							if err == nil {
								newProcessed := atomic.AddInt64(&processedCount, 1)
								w.throttledReport(int(newProcessed), totalFiles, "scanning", path, startTime)
								continue
							}
						}
					} else {
						// INCREMENTAL UPGRADE: We need more hashes.
						log.Printf("Walker: Upgrading hashes for %s (%d -> %d)", path, len(existing.PHashV2s), thumbCount)
						meta, err := media.ProbeNative(path)
						if err != nil {
							meta, _ = media.Probe(ctx, path)
						}
						if meta == nil {
							atomic.AddInt64(&processedCount, 1)
							continue
						}

						hashes := make([]uint64, len(existing.PHashV2s))
						copy(hashes, existing.PHashV2s)

						for i := len(existing.PHashV2s); i < thumbCount; i++ {
							timestamp := media.GetStableTimestamp(i, meta.Duration)
							gray, err := media.ExtractGray32x32Native(ctx, path, timestamp)
							if err != nil {
								gray, err = media.ExtractGray32x32(ctx, path, timestamp)
							}
							if err == nil {
								hashes = append(hashes, phash.ComputeV2(gray))
							} else {
								log.Printf("Walker: Failed to extract extra hash %d for %s", i, path)
							}
						}

						err = w.db.UpsertFile(path, existing.Size, existing.Modified, meta.Duration, meta.Width, meta.Height, hashes, meta.Codec, meta.Bitrate, meta.FPS, meta.Warnings)
						if err == nil {
							newProcessed := atomic.AddInt64(&processedCount, 1)
							w.throttledReport(int(newProcessed), totalFiles, "scanning", path, startTime)
							continue
						}
					}
				}

				// 2. Check for "Content Peers" (same size/mtime at different paths)
				// ... (Keeping peering logic but using the same "satisfies" or "upgrade" check)
				peers, _ := w.db.GetFilesByContent(fileInfo.Size(), fileInfo.ModTime().Unix())
				var reusablehashes []uint64
				var reusedMeta struct {
					Duration float64
					Width    int
					Height   int
					Codec    string
					Bitrate  int64
					FPS      float64
					Warnings []string
				}
				foundPeer := false

				for _, p := range peers {
					if len(p.PHashV2s) < thumbCount {
						// Peer is not "better" or equal to what we want, but let's see if it's the only one
						continue
					}

					// Check if peer still exists on disk
					_, err := os.Stat(p.Path)
					if os.IsNotExist(err) {
						// Peer is a "Ghost" -> Likely a MOVE
						log.Printf("Walker: Detected MOVE: %s -> %s", p.Path, path)
						w.db.UpdatePath(p.Path, path)
						reusablehashes = p.PHashV2s
						reusedMeta.Duration = p.Duration
						reusedMeta.Width = p.Width
						reusedMeta.Height = p.Height
						reusedMeta.Codec = p.Codec
						reusedMeta.Bitrate = p.Bitrate
						reusedMeta.FPS = p.FPS
						reusedMeta.Warnings = p.Warnings
						foundPeer = true
						break
					} else {
						// Peer exists -> Likely a COPY
						log.Printf("Walker: Detected COPY: %s -> %s", p.Path, path)
						reusablehashes = p.PHashV2s
						reusedMeta.Duration = p.Duration
						reusedMeta.Width = p.Width
						reusedMeta.Height = p.Height
						reusedMeta.Codec = p.Codec
						reusedMeta.Bitrate = p.Bitrate
						reusedMeta.FPS = p.FPS
						reusedMeta.Warnings = p.Warnings
						foundPeer = true
					}
				}

				if foundPeer {
					// We might have more hashes than target if we reused from a larger record
					finalHashes := reusablehashes
					if len(finalHashes) > thumbCount {
						finalHashes = finalHashes[:thumbCount]
					}

					err = w.db.UpsertFile(path, fileInfo.Size(), fileInfo.ModTime().Unix(), reusedMeta.Duration, reusedMeta.Width, reusedMeta.Height, finalHashes, reusedMeta.Codec, reusedMeta.Bitrate, reusedMeta.FPS, reusedMeta.Warnings)
					if err == nil {
						newProcessed := atomic.AddInt64(&processedCount, 1)
						w.throttledReport(int(newProcessed), totalFiles, "scanning", path, startTime)
						continue
					}
				}

				// 3. Full Re-index (or start from scratch)
				meta, err := media.ProbeNative(path)
				if err != nil {
					log.Printf("Native probe failed for %s (%v), falling back to CLI", path, err)
					meta, err = media.Probe(ctx, path)
					if err != nil {
						log.Printf("Fallback CLI probe failed for %s: %v", path, err)
						atomic.AddInt64(&processedCount, 1)
						continue
					}
				}

				var hashes []uint64
				for i := 0; i < thumbCount; i++ {
					timestamp := media.GetStableTimestamp(i, meta.Duration)
					gray, err := media.ExtractGray32x32Native(ctx, path, timestamp)
					if err != nil {
						log.Printf("Native extract failed frame %d for %s (%v), falling back to CLI", i, path, err)
						gray, err = media.ExtractGray32x32(ctx, path, timestamp)
						if err != nil {
							log.Printf("Fallback CLI extract failed frame %d for %s: %v", i, path, err)
							continue
						}
					}
					hashes = append(hashes, phash.ComputeV2(gray))
				}
				if len(hashes) == 0 {
					log.Printf("No hashes extracted for %s, skipping", path)
					atomic.AddInt64(&processedCount, 1)
					continue
				}

				err = w.db.UpsertFile(path, fileInfo.Size(), fileInfo.ModTime().Unix(), meta.Duration, meta.Width, meta.Height, hashes, meta.Codec, meta.Bitrate, meta.FPS, meta.Warnings)
				if err != nil {
					log.Printf("Failed to save to DB %s: %v", path, err)
				}

				newProcessed := atomic.AddInt64(&processedCount, 1)
				if cfg.DebugLogging {
					log.Printf("Walker: [Worker %d] Finished %s", id, path)
				}

				w.throttledReport(int(newProcessed), totalFiles, "scanning", path, startTime)
			}
		}(workerID)
	}

	// Feed files to workers
	go func() {
		defer close(filesChan)
		for _, f := range allFiles {
			select {
			case filesChan <- f:
			case <-ctx.Done():
				return
			}
		}
	}()

	workerWg.Wait()
	log.Printf("Walker: Indexing complete.")
	w.report(totalFiles, totalFiles, "comparing", "Grouping duplicates...", startTime)
	return nil
}

func (w *Walker) report(current, total int, phase string, lastFile string, startTime time.Time) {
	if w.reporter != nil {
		w.reporter.BroadcastProgress(current, total, phase, lastFile, time.Since(startTime).Seconds())
	}
}

var lastReport atomic.Value

func (w *Walker) throttledReport(current, total int, phase string, lastFile string, startTime time.Time) {
	now := time.Now()
	val := lastReport.Load()
	if val != nil {
		if lastTime, ok := val.(time.Time); ok && now.Sub(lastTime) < 200*time.Millisecond {
			return
		}
	}
	w.report(current, total, phase, lastFile, startTime)
	lastReport.Store(now)
}

func isVideoExt(ext string) bool {
	switch ext {
	case ".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".mpg", ".mpeg":
		return true
	}
	return false
}

func isBlacklisted(path string, blacklist []string) bool {
	lowerPath := strings.ToLower(path)
	for _, term := range blacklist {
		if strings.Contains(lowerPath, strings.ToLower(term)) {
			return true
		}
	}
	return false
}
