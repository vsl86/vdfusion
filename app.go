package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"vdfusion/internal/config"
	"vdfusion/internal/db"
	"vdfusion/internal/engine"
	"vdfusion/internal/media"
	"vdfusion/internal/syslog"
)

// App struct
type App struct {
	ctx            context.Context
	db             *db.Database
	settings       *config.SettingsManager
	compare        *engine.ComparisonEngine
	scanner        *engine.Scanner
	resultsManager *engine.ResultsManager
	reporter       *WailsReporter
}

type WailsReporter struct {
	ctx context.Context
}

func (r *WailsReporter) BroadcastProgress(current, total int, phase string, lastFile string, durationSeconds float64) {
	if r.ctx != nil {
		runtime.EventsEmit(r.ctx, "scan_progress", map[string]any{
			"current":          current,
			"total":            total,
			"phase":            phase,
			"last_file":        lastFile,
			"duration_seconds": durationSeconds,
		})
	}
}

func (r *WailsReporter) BroadcastLog(severity, message string) {
	if r.ctx != nil {
		runtime.EventsEmit(r.ctx, "app_log", map[string]any{
			"severity": severity,
			"message":  message,
			"time":     time.Now().Format("15:04:05"),
		})
	}
}

func (a *App) BroadcastSystemLog(line string) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "system_log", line)
	} else {
		syslog.RawLog("syslog: BroadcastSystemLog called but a.ctx is nil")
	}
}

// NewApp creates a new App application struct
func NewApp(database *db.Database, sm *config.SettingsManager) *App {
	reporter := &WailsReporter{}
	walker := engine.NewWalker(database, nil)
	compare := engine.NewComparisonEngine()
	resultsManager := engine.NewResultsManager()

	scanner := engine.NewScanner(walker, database, reporter, compare, resultsManager)
	walker.SetReporter(scanner) // Walker → Scanner → WailsReporter

	return &App{
		db:             database,
		settings:       sm,
		compare:        compare,
		scanner:        scanner,
		resultsManager: resultsManager,
		reporter:       reporter,
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.reporter.ctx = ctx
}

func (a *App) StartScan(paths []string) {
	if len(paths) == 0 {
		paths = a.settings.Get().IncludeList
	}
	a.reporter.BroadcastLog("info", fmt.Sprintf("Starting scan with %d directories", len(paths)))
	a.scanner.Start(a.ctx, paths, a.settings.Get())
}

func (a *App) StopScan() {
	a.reporter.BroadcastLog("warning", "Scan stopped by user")
	a.scanner.Stop()
}

func (a *App) GetResults(offset, limit int) engine.ResultsResponse {
	return a.resultsManager.GetResultsWrapped(offset, limit)
}

func (a *App) GetSettings() config.Settings {
	return a.settings.Get()
}

func (a *App) SaveSettings(cfg config.Settings) error {
	return a.settings.Update(cfg)
}

func (a *App) ExcludeGroup(label string, paths []string) error {
	if label == "" {
		label = "Ignored Group"
	}
	// Efficiently resolve paths to hashes using the new DB method
	files, err := a.db.GetFilesByPaths(paths)
	if err != nil {
		fmt.Printf("Backend: Error resolving paths for exclusion: %v\n", err)
		return err
	}

	pathMap := make(map[string]db.FileRecord)
	for _, f := range files {
		pathMap[f.Path] = f
	}

	var hashes []string
	for _, p := range paths {
		if rec, ok := pathMap[p]; ok {
			hashes = append(hashes, rec.GetIdentifierHash())
		}
	}

	if len(hashes) < 2 {
		return fmt.Errorf("could not resolve at least 2 files to identifier hashes")
	}

	fmt.Printf("Backend: Excluding group '%s' with %d hashes\n", label, len(hashes))
	a.reporter.BroadcastLog("info", fmt.Sprintf("Excluded %d files under group '%s'", len(hashes), label))
	return a.db.AddIgnoredGroup(label, hashes)
}

func (a *App) GetIgnoredGroups() ([]db.IgnoredGroup, error) {
	return a.db.GetIgnoredGroups()
}

func (a *App) DeleteIgnoredGroup(id int64) error {
	a.reporter.BroadcastLog("info", fmt.Sprintf("Restored ignored group (ID: %d)", id))
	return a.db.DeleteIgnoredGroup(id)
}

func (a *App) PurgeBlacklist() error {
	a.reporter.BroadcastLog("info", "Purging all manual exclusions (blacklist)")
	return a.db.DeleteAllIgnoredGroups()
}

func (a *App) ResetSettings() error {
	a.reporter.BroadcastLog("info", "Resetting all settings to defaults")
	return a.settings.Reset()
}

// extractThumbnailFn is a hook to allow extracting logic to test tags
var extractThumbnailFn = func(path string, duration float64, count int, i int) ([]byte, error) {
	timestamp := media.GetStableTimestamp(i, duration)
	data, err := media.ExtractThumbnailNative(context.Background(), path, timestamp, 160, 0)
	if err != nil {
		fmt.Printf("Native thumbnail extract failed for %s (%v), falling back to CLI\n", path, err)
		data, err = media.ExtractThumbnail(context.Background(), path, timestamp, 160, 90)
	}
	return data, err
}

// GetThumbnails fetches N thumbnails for a video file.
func (a *App) GetThumbnails(path string, duration float64, count int) ([]string, error) {
	if count <= 0 {
		count = 4
	}
	// Fast fail when file does not exist to avoid repeated fallback logs
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			log.Printf("GetThumbnails: file not found: %s", path)
			return []string{}, fmt.Errorf("file not found")
		}
		// For other stat errors, return as well
		log.Printf("GetThumbnails: stat error for %s: %v", path, err)
		return []string{}, err
	}
	var results []string
	for i := 0; i < count; i++ {
		data, err := extractThumbnailFn(path, duration, count, i)

		if err != nil || data == nil {
			continue // Skip failed frames
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		results = append(results, fmt.Sprintf("data:image/jpeg;base64,%s", b64))
	}
	return results, nil
}

func (a *App) OpenFile(path string) error {
	log.Printf("App: Opening file %s", path)
	// Try ffplay first for better preview control
	err := exec.Command("ffplay", "-i", path, "-autoexit", "-nodisp").Start()
	if err == nil {
		log.Printf("App: ffplay started")
		return nil
	}
	log.Printf("App: ffplay failed: %v, falling back to open", err)
	// Fallback to Mac 'open'
	return exec.Command("open", path).Run()
}

func (a *App) RenameFile(oldPath string, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (a *App) DeleteFiles(paths []string) error {
	fmt.Printf("Backend: Deleting %d files\n", len(paths))
	a.reporter.BroadcastLog("warning", fmt.Sprintf("Deleting %d files from disk", len(paths)))
	for _, p := range paths {
		// 1. Delete from disk
		err := os.Remove(p)
		if err != nil && !os.IsNotExist(err) {
			fmt.Printf("Error deleting file from disk %s: %v\n", p, err)
			a.reporter.BroadcastLog("error", fmt.Sprintf("Failed to delete from disk: %s", filepath.Base(p)))
		}
		// 2. Delete from DB even if disk deletion fails (so it doesn't stay in list)
		err = a.db.DeleteFile(p)
		if err != nil {
			fmt.Printf("Error deleting file from DB %s: %v\n", p, err)
		} else {
			a.reporter.BroadcastLog("success", fmt.Sprintf("Deleted: %s", filepath.Base(p)))
		}
	}
	return nil
}

func (a *App) ResetDB() error {
	return a.db.Reset()
}

func (a *App) CleanupDB() (int, error) {
	return a.db.Cleanup()
}

type WarningInfo struct {
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

type SuspiciousFile struct {
	Path     string        `json:"path"`
	Warnings []WarningInfo `json:"warnings"`
}

func (a *App) GetSuspiciousFiles() []SuspiciousFile {
	records, err := a.db.GetSuspiciousFiles()
	if err != nil {
		return nil
	}
	var result []SuspiciousFile
	for _, r := range records {
		var warnings []WarningInfo
		seen := map[string]bool{}
		for _, w := range r.Warnings {
			if seen[w] {
				continue
			}
			seen[w] = true
			warnings = append(warnings, WarningInfo{
				Message: w,
				Fix:     suggestFixCmd(w, r.Path),
			})
		}
		result = append(result, SuspiciousFile{
			Path:     r.Path,
			Warnings: warnings,
		})
	}
	return result
}

func suggestFixCmd(warning, path string) string {
	lower := strings.ToLower(warning)
	ext := strings.ToLower(filepath.Ext(path))
	dir := filepath.Dir(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	out := filepath.Join(dir, base+"_fixed"+ext)

	switch {
	case strings.Contains(lower, "packed b-frames"):
		return fmt.Sprintf("ffmpeg -i %q -codec copy -bsf:v mpeg4_unpack_bframes %q", path, out)
	case strings.Contains(lower, "non-interleaved"):
		outMp4 := filepath.Join(dir, base+"_fixed.mp4")
		return fmt.Sprintf("ffmpeg -i %q -codec copy %q", path, outMp4)
	case strings.Contains(lower, "corrupt"), strings.Contains(lower, "invalid data"), strings.Contains(lower, "error decoding"):
		return fmt.Sprintf("ffmpeg -i %q -codec copy -err_detect ignore_err %q", path, out)
	default:
		return ""
	}
}

// SaveLogToFile opens a save file dialog and saves the provided text to that path
func (a *App) SaveLogToFile(content string) error {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export Logs",
		DefaultFilename: fmt.Sprintf("vdfusion_logs_%s.txt", time.Now().Format("2006-01-02_15-04-05")),
		Filters:         []runtime.FileFilter{{DisplayName: "Text Files", Pattern: "*.txt"}},
	})
	if err != nil {
		return err
	}
	if path == "" {
		return nil // Canceled
	}

	return os.WriteFile(path, []byte(content), 0644)
}

// ExportDB opens a save file dialog and copies the current database to that path
func (a *App) ExportDB() error {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export Database",
		DefaultFilename: "vdfusion_backup.db",
		Filters:         []runtime.FileFilter{{DisplayName: "SQLite Database", Pattern: "*.db"}},
	})
	if err != nil {
		return err
	}
	if path == "" {
		return nil // Canceled
	}

	srcFile, err := os.Open(a.db.GetPath())
	if err != nil {
		return fmt.Errorf("failed to open current database: %v", err)
	}
	defer srcFile.Close()

	destFile, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create export file: %v", err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy database: %v", err)
	}
	return nil
}

// ImportDB opens an open file dialog, replaces the current database, and reloads it
func (a *App) ImportDB() error {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Import Database",
		Filters: []runtime.FileFilter{{DisplayName: "SQLite Database", Pattern: "*.db"}},
	})
	if err != nil {
		return err
	}
	if path == "" {
		return nil // Canceled
	}

	srcFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open import file: %v", err)
	}
	defer srcFile.Close()

	// Close the current database connection
	if err := a.db.Close(); err != nil {
		return fmt.Errorf("failed to close current database: %v", err)
	}

	// Copy the file over
	currentPath := a.db.GetPath()
	destFile, err := os.Create(currentPath)
	if err != nil {
		return fmt.Errorf("failed to overwrite current database: %v", err)
	}

	_, copyErr := io.Copy(destFile, srcFile)
	destFile.Close()

	if copyErr != nil {
		return fmt.Errorf("failed to copy imported database: %v", copyErr)
	}

	// Re-open the database
	newDB, err := db.New(currentPath)
	if err != nil {
		return fmt.Errorf("failed to reconnect to imported database: %v", err)
	}

	// Reset engine instances since they hold references to the old DB
	walker := engine.NewWalker(newDB, nil)
	compare := engine.NewComparisonEngine()
	resultsManager := engine.NewResultsManager()

	scanner := engine.NewScanner(walker, newDB, a.reporter, compare, resultsManager)
	walker.SetReporter(scanner)

	a.db = newDB
	a.compare = compare
	a.scanner = scanner
	a.resultsManager = resultsManager

	return nil
}
