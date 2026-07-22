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
	"vdfusion/internal/neural"
	"vdfusion/internal/phash"
)

type Walker struct {
	db                  *db.Database
	reporter            ProgressReporter
	neuralClient        *neural.Client        // optional; nil = pHash-only mode
	neuralBatchEmbedder *neural.BatchEmbedder // optional; wraps neuralClient for batch processing
}

func NewWalker(d *db.Database, reporter ProgressReporter) *Walker {
	return &Walker{db: d, reporter: reporter}
}

func (w *Walker) SetReporter(reporter ProgressReporter) {
	w.reporter = reporter
}

func (w *Walker) SetNeuralClient(c *neural.Client) {
	log.Printf("Walker.SetNeuralClient called: c=%v", c)
	w.neuralClient = c
	if c != nil {
		w.neuralBatchEmbedder = neural.NewBatchEmbedder(c)
	} else {
		w.neuralBatchEmbedder = nil
	}
}

func (w *Walker) SetNeuralBatchEmbedder(be *neural.BatchEmbedder) {
	log.Printf("Walker.SetNeuralBatchEmbedder called: be=%v", be)
	w.neuralBatchEmbedder = be
	if be != nil {
		w.neuralClient = nil // Avoid using both at the same time
	}
}

func (w *Walker) IndexPaths(ctx context.Context, paths []string, cfg config.Settings) (map[string]bool, error) {
	startTime := time.Now()

	w.report(0, 0, "discovery", "", startTime)
	log.Printf("Walker: starting index with neural client: %v, batch embedder: %v", w.neuralClient != nil, w.neuralBatchEmbedder != nil)

	// Start batch embedder if available
	if w.neuralBatchEmbedder != nil {
		w.neuralBatchEmbedder.Start(ctx)
		log.Printf("Walker: started neural batch embedder")
	}

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
		return nil, ctx.Err()
	}

	totalFiles := len(allFiles)

	discoveredMap := make(map[string]bool, len(allFiles))
	for _, f := range allFiles {
		discoveredMap[f] = true
	}

	existingCount := 0
	enrichCount := 0
	removedCount := 0
	outsideScopeCount := 0

	thumbCount := cfg.Thumbnails
	if thumbCount < 1 {
		thumbCount = 4
	}

	existingRecords, _ := w.db.GetFilesByPrefixes(paths)
	existingByPath := make(map[string]*db.FileRecord, len(existingRecords))
	for i := range existingRecords {
		existingByPath[existingRecords[i].Path] = &existingRecords[i]
	}

	for _, r := range existingRecords {
		if discoveredMap[r.Path] {
			existingCount++
			if len(r.PHashV2s) < thumbCount {
				enrichCount++
			}
		} else {
			if _, err := os.Stat(r.Path); os.IsNotExist(err) {
				removedCount++
			} else {
				outsideScopeCount++
			}
		}
	}

	newFilesCount := totalFiles - existingCount
	parts := []string{
		fmt.Sprintf("%d new", newFilesCount),
		fmt.Sprintf("%d cached", existingCount),
	}
	if enrichCount > 0 {
		parts = append(parts, fmt.Sprintf("%d to enrich (→%d frames)", enrichCount, thumbCount))
	}
	if outsideScopeCount > 0 {
		parts = append(parts, fmt.Sprintf("%d outside filters", outsideScopeCount))
	}
	if removedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d removed from disk", removedCount))
	}
	summary := fmt.Sprintf("Discovery: Found %d files (%s)", totalFiles, strings.Join(parts, ", "))
	log.Printf("Walker: %s", summary)
	if w.reporter != nil {
		w.reporter.BroadcastLog("success", summary)
	}

	w.report(0, totalFiles, "scanning", "", startTime)

	var processedCount int64
	var workerWg sync.WaitGroup
	filesChan := make(chan string, 100)
	dirtyPaths := make(map[string]bool)
	var dirtyMu sync.Mutex

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

				existing, _ := w.db.GetFileByPath(path)
				modTime := fileInfo.ModTime().Unix()
				needsNeural := existing != nil && (w.neuralClient != nil || w.neuralBatchEmbedder != nil) && len(existing.NeuralEmbeddings) == 0
				isUnmodified := existing != nil && existing.Size == fileInfo.Size() && existing.Modified == modTime

				if isUnmodified && cfg.RecheckSuspicious && len(existing.Warnings) > 0 {
					log.Printf("Walker: Forcing re-probe of suspicious file: %s", path)
					isUnmodified = false
				}
				if isUnmodified {
					if !needsNeural {
						if len(existing.PHashV2s) >= thumbCount {
							if existing.Codec != "" && existing.Width > 0 && existing.Height > 0 {
								newProcessed := atomic.AddInt64(&processedCount, 1)
								w.throttledReport(int(newProcessed), totalFiles, "scanning", path, startTime)
								continue
							}

							// Just update metadata if missing
							log.Printf("Walker: Updating missing metadata for %s", path)
							meta, err := media.ProbeNative(path)
							if err != nil {
								meta, _ = media.Probe(ctx, path)
							}
							if meta != nil && meta.Codec != "" {
								err = w.db.UpsertFile(path, existing.Size, existing.Modified, meta.Duration, meta.Width, meta.Height, existing.PHashV2s, meta.Codec, meta.Bitrate, meta.FPS, meta.Warnings)
								if err == nil {
									dirtyMu.Lock()
									dirtyPaths[path] = true
									dirtyMu.Unlock()
									newProcessed := atomic.AddInt64(&processedCount, 1)
									w.throttledReport(int(newProcessed), totalFiles, "scanning", path, startTime)
									continue
								}
							}
						} else {
							// Upgrade hashes
							log.Printf("Walker: Upgrading hashes for %s (%d -> %d)", path, len(existing.PHashV2s), thumbCount)
							meta, err := media.ProbeNative(path)
							if err != nil {
								meta, _ = media.Probe(ctx, path)
							}
							if meta != nil {
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
									}
								}
								err = w.db.UpsertFile(path, existing.Size, existing.Modified, meta.Duration, meta.Width, meta.Height, hashes, meta.Codec, meta.Bitrate, meta.FPS, meta.Warnings)
								if err == nil {
									dirtyMu.Lock()
									dirtyPaths[path] = true
									dirtyMu.Unlock()
									newProcessed := atomic.AddInt64(&processedCount, 1)
									w.throttledReport(int(newProcessed), totalFiles, "scanning", path, startTime)
									continue
								}
							}
						}
					} else {
						// isUnmodified && needsNeural
						log.Printf("Walker: Enriching neural embeddings for existing file %s", path)
						meta, err := media.ProbeNative(path)
						if err != nil {
							meta, _ = media.Probe(ctx, path)
						}
						if meta != nil {
							var jpegFrames [][]byte
							for i := 0; i < thumbCount; i++ {
								timestamp := media.GetStableTimestamp(i, meta.Duration)
								jpeg, jerr := media.ExtractThumbnailNative(ctx, path, timestamp, 224, 0)
								if jerr != nil {
									jpeg, _ = media.ExtractThumbnail(ctx, path, timestamp, 224, 224)
								}
								if jpeg != nil {
									jpegFrames = append(jpegFrames, jpeg)
								}
							}
							if len(jpegFrames) > 0 {
								var embeddings [][]float32
								var nerr error
								if w.neuralBatchEmbedder != nil {
									embeddings, nerr = w.neuralBatchEmbedder.Embed(ctx, jpegFrames)
								} else if w.neuralClient != nil {
									embeddings, nerr = w.neuralClient.Embed(ctx, jpegFrames)
								}
								if nerr == nil {
									neuralBlob := neural.PackEmbeddings(embeddings)
									_ = w.db.UpdateNeuralEmbeddings(path, neuralBlob)
								}
							}
						}
						newProcessed := atomic.AddInt64(&processedCount, 1)
						w.throttledReport(int(newProcessed), totalFiles, "scanning", path, startTime)
						continue
					}
				}

				if cfg.DebugLogging {
					log.Printf("Walker: processing file %s, needsNeural=%v, neuralClient=%v, neuralBatchEmbedder=%v", path, needsNeural, w.neuralClient != nil, w.neuralBatchEmbedder != nil)
				}

				peers, _ := w.db.GetFilesByContent(fileInfo.Size(), modTime)
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
						continue
					}
					_, err := os.Stat(p.Path)
					if os.IsNotExist(err) {
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
					finalHashes := reusablehashes
					if len(finalHashes) > thumbCount {
						finalHashes = finalHashes[:thumbCount]
					}
					// Check if peer has neural embeddings
					var neuralBlob []byte
					for _, peerFile := range peers {
						if len(peerFile.NeuralEmbeddings) > 0 {
							neuralBlob = neural.PackEmbeddings(peerFile.NeuralEmbeddings)
							break
						}
					}

					if len(neuralBlob) > 0 {
						err = w.db.UpsertFileWithNeural(path, fileInfo.Size(), modTime, reusedMeta.Duration, reusedMeta.Width, reusedMeta.Height, finalHashes, reusedMeta.Codec, reusedMeta.Bitrate, reusedMeta.FPS, reusedMeta.Warnings, neuralBlob)
					} else {
						err = w.db.UpsertFile(path, fileInfo.Size(), modTime, reusedMeta.Duration, reusedMeta.Width, reusedMeta.Height, finalHashes, reusedMeta.Codec, reusedMeta.Bitrate, reusedMeta.FPS, reusedMeta.Warnings)
					}

					if err == nil {
						dirtyMu.Lock()
						dirtyPaths[path] = true
						dirtyMu.Unlock()
						newProcessed := atomic.AddInt64(&processedCount, 1)
						w.throttledReport(int(newProcessed), totalFiles, "scanning", path, startTime)
						continue
					}
				}

				// 3. Full Re-index
				if ctx.Err() != nil {
					return
				}
				meta, err := media.ProbeNative(path)
				if err != nil {
					meta, err = media.Probe(ctx, path)
					if err != nil {
						atomic.AddInt64(&processedCount, 1)
						continue
					}
				}

				var hashes []uint64
				var jpegFrames [][]byte // collected for neural embedding if client is active
				for i := 0; i < thumbCount; i++ {
					timestamp := media.GetStableTimestamp(i, meta.Duration)
					gray, err := media.ExtractGray32x32Native(ctx, path, timestamp)
					if err != nil {
						gray, err = media.ExtractGray32x32(ctx, path, timestamp)
					}
					if err == nil {
						hashes = append(hashes, phash.ComputeV2(gray))
						// Also grab JPEG for neural embedding when needed
						if w.neuralClient != nil || w.neuralBatchEmbedder != nil {
							jpeg, jerr := media.ExtractThumbnailNative(ctx, path, timestamp, 224, 0)
							if jerr != nil {
								jpeg, _ = media.ExtractThumbnail(ctx, path, timestamp, 224, 224)
							}
							if jpeg != nil {
								jpegFrames = append(jpegFrames, jpeg)
							}
						}
					}
				}
				if len(hashes) == 0 {
					atomic.AddInt64(&processedCount, 1)
					continue
				}

				// Attempt neural embedding for new/changed files
				var neuralBlob []byte
				log.Printf("Walker: about to check neural for %s: w.neuralClient=%v, w.neuralBatchEmbedder=%v, len(jpegFrames)=%d", path, w.neuralClient != nil, w.neuralBatchEmbedder != nil, len(jpegFrames))
				if (w.neuralClient != nil || w.neuralBatchEmbedder != nil) && len(jpegFrames) > 0 {
					var embeddings [][]float32
					var nerr error
					if w.neuralBatchEmbedder != nil {
						log.Printf("Walker: calling neuralBatchEmbedder.Embed for %s", path)
						embeddings, nerr = w.neuralBatchEmbedder.Embed(ctx, jpegFrames)
					} else if w.neuralClient != nil {
						log.Printf("Walker: calling neuralClient.Embed for %s", path)
						embeddings, nerr = w.neuralClient.Embed(ctx, jpegFrames)
					}
					if nerr != nil {
						log.Printf("Walker: neural embed failed for %s: %v", path, nerr)
					} else {
						neuralBlob = neural.PackEmbeddings(embeddings)
					}
				}

				err = w.db.UpsertFileWithNeural(path, fileInfo.Size(), modTime, meta.Duration, meta.Width, meta.Height, hashes, meta.Codec, meta.Bitrate, meta.FPS, meta.Warnings, neuralBlob)
				if err == nil {
					dirtyMu.Lock()
					dirtyPaths[path] = true
					dirtyMu.Unlock()
				}
				newProcessed := atomic.AddInt64(&processedCount, 1)
				w.throttledReport(int(newProcessed), totalFiles, "scanning", path, startTime)
			}
		}(workerID)
	}

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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	log.Printf("Walker: Indexing complete. Found %d new/modified files.", len(dirtyPaths))
	w.report(totalFiles, totalFiles, "comparing", "Grouping duplicates...", startTime)
	return dirtyPaths, nil
}

func (w *Walker) report(current, total int, phase string, lastFile string, startTime time.Time) {
	if w.reporter != nil {
		duration := time.Since(startTime).Seconds()
		eta := 0.0
		if phase == "scanning" && current > 0 && total > current && duration > 0 {
			rate := float64(current) / duration
			eta = float64(total-current) / rate
		}
		w.reporter.BroadcastProgress(current, total, phase, lastFile, duration, eta)
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
