package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"vdfusion/internal/config"
	"vdfusion/internal/db"
	"vdfusion/internal/engine"
	"vdfusion/internal/neural"
)

func main() {
	log.Printf("=== test_neural_flow starting ===")

	// 1. Create a temporary directory for our test DB
	tmpDir, err := os.MkdirTemp("", "test-neural-flow-*")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	dbPath := filepath.Join(tmpDir, "test.db")
	log.Printf("Test DB path: %s", dbPath)

	// 2. Initialize the database
	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	// 3. Create a settings manager with neural backend enabled (using default URL if not set)
	sm := config.NewSettingsManager(filepath.Join(tmpDir, "settings.json"))
	cfg := sm.Get()
	cfg.NeuralBackendEnabled = true
	// Get neural backend URL from env or use default
	neuralURL := os.Getenv("NEURAL_URL")
	if neuralURL == "" {
		neuralURL = "http://127.0.0.1:8765"
	}
	cfg.NeuralBackendURL = neuralURL
	if err := sm.Update(cfg); err != nil {
		log.Fatalf("Failed to update settings: %v", err)
	}

	// 4. Create scanner and walker, wire up neural client
	walker := engine.NewWalker(database, nil)
	scanner := engine.NewScanner(walker, database, nil, engine.NewComparisonEngine(), engine.NewResultsManager())
	walker.SetReporter(scanner)

	log.Printf("=== setting up neural client ===")
	if cfg.NeuralBackendEnabled && cfg.NeuralBackendURL != "" {
		client := neural.NewClient(cfg.NeuralBackendURL)
		scanner.SetNeuralClient(client)
	}

	// 5. Create a temporary directory with a test video file!
	testVideoDir := filepath.Join(tmpDir, "test_videos")
	if err := os.MkdirAll(testVideoDir, 0755); err != nil {
		log.Fatalf("Failed to create test video dir: %v", err)
	}
	testVideoPath := filepath.Join(testVideoDir, "test_video.mp4")
	log.Printf("Creating test video at: %s", testVideoPath)

	// Use FFmpeg to create a 1-second test video with a solid color
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "color=c=red:s=1280x720:r=30", // red color, 1280x720, 30fps
		"-t", "1", // duration 1 second
		"-c:v", "libx264", "-crf", "23",
		"-y", // overwrite
		testVideoPath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to create test video with FFmpeg: %v", err)
	}
	log.Printf("Test video created successfully!")

	// 6. Try to run the scan!
	log.Printf("=== starting scan ===")
	ctx := context.Background()
	_, err = walker.IndexPaths(ctx, []string{testVideoDir}, cfg)
	if err != nil {
		log.Fatalf("IndexPaths failed: %v", err)
	}

	log.Printf("=== test_neural_flow done ===")
}
