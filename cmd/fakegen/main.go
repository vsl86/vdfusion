package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"path/filepath"
	"slices"
	"time"

	"vdfusion/internal/config"
	"vdfusion/internal/db"
	"vdfusion/internal/phash"
)

func main() {
	entries := flag.Int("entries", 50000, "Number of fake entries")
	seed := flag.Int64("seed", 42, "Random seed")
	thumbnails := flag.Int("thumbnails", 7, "Thumbnail count per entry")
	minDuration := flag.Float64("min-duration", 5.0, "Min duration in seconds")
	maxDuration := flag.Float64("max-duration", 3600.0, "Max duration in seconds")
	dupGroups := flag.Int("duplicate-groups", 100, "Number of duplicate groups to create")
	dupSize := flag.Int("duplicate-group-size", 2, "Number of files per duplicate group")
	output := flag.String("output", "vdf.db", "Output database path")
	pathPrefix := flag.String("path-prefix", "/fake_files", "Fake file path prefix")
	flag.Parse()

	if *maxDuration < *minDuration {
		log.Fatal("max-duration must be >= min-duration")
	}
	if *dupSize < 2 {
		*dupSize = 2
	}

	rng := rand.New(rand.NewSource(*seed))

	database, err := db.New(*output)
	if err != nil {
		log.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	start := time.Now()
	fmt.Printf("Generating %d entries (seed=%d, thumbnails=%d)...\n", *entries, *seed, *thumbnails)

	// 1. Generate unique entries
	for i := 0; i < *entries; i++ {
		path := filepath.Join(*pathPrefix, fmt.Sprintf("fake_%06d.mp4", i))
		size := int64(5_000_000 + rng.Intn(245_000_000))
		duration := *minDuration + rng.Float64()*(*maxDuration-*minDuration)
		modified := time.Now().Add(-time.Duration(i) * time.Minute).Unix()

		hashes := generateHashes(rng, duration, *thumbnails)

		// Fake width/height around 1920x1080 with some variation
		w := 1280 + rng.Intn(1280)
		h := 720 + rng.Intn(360)
		if err := database.UpsertFile(path, size, modified, duration, w, h, hashes, "h264", 2000000, 30.0, nil); err != nil {
			log.Printf("Failed to insert entry %d: %v", i, err)
		}

		if (i+1)%10000 == 0 {
			fmt.Printf("  ...%d/%d entries\n", i+1, *entries)
		}
	}

	// 2. Generate duplicate groups
	fmt.Printf("Generating %d duplicate groups (size=%d each)...\n", *dupGroups, *dupSize)
	nextIdx := *entries
	for g := 0; g < *dupGroups; g++ {
		// Create reference entry
		duration := *minDuration + rng.Float64()*(*maxDuration-*minDuration)
		size := int64(5_000_000 + rng.Intn(245_000_000))
		modified := time.Now().Add(-time.Duration(nextIdx) * time.Minute).Unix()
		refHashes := generateHashes(rng, duration, *thumbnails)

		refPath := filepath.Join(*pathPrefix, fmt.Sprintf("fake_%06d.mp4", nextIdx))
		nextIdx++
		w := 1280 + rng.Intn(1280)
		h := 720 + rng.Intn(360)
		if err := database.UpsertFile(refPath, size, modified, duration, w, h, refHashes, "h264", 2000000, 30.0, nil); err != nil {
			log.Printf("Failed to insert ref entry: %v", err)
		}

		// Create duplicates with identical hashes
		for c := 1; c < *dupSize; c++ {
			dupPath := filepath.Join(*pathPrefix, fmt.Sprintf("fake_dup_%06d.mp4", nextIdx))
			nextIdx++
			// Slightly vary the file size to simulate re-encodes
			dupSize := size + int64(rng.Intn(1_000_000)-500_000)
			if dupSize < 0 {
				dupSize = size
			}
			// Duration stays the same (within tolerance)
			dupDuration := duration + (rng.Float64()*2.0 - 1.0) // +-1s
			if dupDuration < 0 {
				dupDuration = duration
			}

			w2 := 1280 + rng.Intn(1280)
			h2 := 720 + rng.Intn(360)
			if err := database.UpsertFile(dupPath, dupSize, modified, dupDuration, w2, h2, refHashes, "h264", 2000000, 30.0, nil); err != nil {
				log.Printf("Failed to insert dup entry: %v", err)
			}
		}
	}

	totalEntries := *entries + *dupGroups**dupSize
	elapsed := time.Since(start)

	// Automatically add the fake path to the include list so the scanner picks it up
	settingsPath := filepath.Join(filepath.Dir(*output), "settings.json")
	sm := config.NewSettingsManager(settingsPath)
	cfg := sm.Get()
	found := slices.Contains(cfg.IncludeList, *pathPrefix)
	if !found {
		cfg.IncludeList = append(cfg.IncludeList, *pathPrefix)
		_ = sm.Update(cfg)
	}

	fmt.Printf("\nDone! Wrote %d entries to %s in %v\n", totalEntries, *output, elapsed.Round(time.Millisecond))
	fmt.Printf("  - %d unique entries\n", *entries)
	fmt.Printf("  - %d duplicate groups (%d files each)\n", *dupGroups, *dupSize)
	fmt.Printf("\n🚀 HOW TO TEST IN UI:\n")
	fmt.Printf("1. Run: VDF_DB_PATH=%s wails dev\n", *output)
	fmt.Printf("2. The path '%s' has been auto-added to your Scan Directories.\n", *pathPrefix)
	fmt.Printf("3. Click 'Start Scan'. The walker will skip the missing dir, but the comparison phase will load the %d DB records and find the %d duplicates!\n", totalEntries, *dupGroups)
}

// generateHashes creates N pHashes from evenly-spaced random gray frames.
func generateHashes(rng *rand.Rand, duration float64, thumbnailCount int) []uint64 {
	hashes := make([]uint64, thumbnailCount)
	for i := range thumbnailCount {
		gray := make([]byte, 32*32)
		rng.Read(gray)
		hashes[i] = phash.ComputeV2(gray)
	}
	return hashes
}

// For reference: pack hashes as little-endian uint64s (same as db.go)
func packHashes(hashes []uint64) []byte {
	buf := make([]byte, 8*len(hashes))
	for i, h := range hashes {
		binary.LittleEndian.PutUint64(buf[i*8:], h)
	}
	return buf
}
