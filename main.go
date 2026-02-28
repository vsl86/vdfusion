package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"vdfusion/internal/api"
	"vdfusion/internal/config"
	"vdfusion/internal/db"
	"vdfusion/internal/engine"
	"vdfusion/internal/syslog"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	serverMode := false
	for _, arg := range os.Args {
		if arg == "--server" || arg == "--headless" {
			serverMode = true
			break
		}
	}

	dbPath := os.Getenv("VDF_DB_PATH")
	if dbPath == "" {
		dataDir := config.GetDefaultDataDir()
		dbPath = filepath.Join(dataDir, "vdf.db")
	}
	fmt.Printf("Using database: %s\n", dbPath)

	database, err := db.New(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: failed to connect database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	settingsPath := filepath.Join(filepath.Dir(dbPath), "settings.json")
	settingsManager := config.NewSettingsManager(settingsPath)

	if serverMode {
		runServer(database, settingsManager)
	} else {
		runWails(database, settingsManager)
	}
}

func runServer(database *db.Database, settingsManager *config.SettingsManager) {
	hub := api.NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	syslog.Start()
	syslog.AddListener(hub.BroadcastSystemLog)
	go hub.Run(ctx)

	walker := engine.NewWalker(database, nil)
	compare := engine.NewComparisonEngine()
	resultsManager := engine.NewResultsManager()

	scanner := engine.NewScanner(walker, database, hub, compare, resultsManager)
	walker.SetReporter(scanner) // Walker → Scanner → Hub

	server := api.NewServer(database, hub, scanner, resultsManager, settingsManager, assets)

	// Graceful Shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down server...")
		cancel()
		os.Exit(0)
	}()

	addr := os.Getenv("VDF_SERVER_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("VDF Server starting on %s (Headless Mode)", addr)
	if err := server.Start(addr); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func runWails(database *db.Database, settingsManager *config.SettingsManager) {
	// Create an instance of the app structure
	app := NewApp(database, settingsManager)

	syslog.Start()
	syslog.AddListener(app.BroadcastSystemLog)

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "VDFusion",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []any{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
