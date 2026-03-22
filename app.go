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
	goruntime "runtime"
	"sort"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"vdfusion/internal/config"
	"vdfusion/internal/db"
	"vdfusion/internal/engine"
	"vdfusion/internal/media"
	"vdfusion/internal/syslog"
	"vdfusion/internal/utils"
)

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

func (r *WailsReporter) BroadcastProgress(current, total int, phase string, lastFile string, durationSeconds, estimatedRemainingSeconds float64) {
	if r.ctx != nil {
		runtime.EventsEmit(r.ctx, "scan_progress", map[string]any{
			"current":                     current,
			"total":                       total,
			"phase":                       phase,
			"last_file":                   lastFile,
			"duration_seconds":            durationSeconds,
			"estimated_remaining_seconds": estimatedRemainingSeconds,
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

func (r *WailsReporter) BroadcastResultsUpdated(action string, count int) {
	runtime.EventsEmit(r.ctx, "results_updated", map[string]any{
		"action": action,
		"count":  count,
		"time":   time.Now().Format("15:04:05"),
	})
}

func (a *App) BroadcastSystemLog(line string) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "system_log", line)
	} else {
		syslog.RawLog("syslog: BroadcastSystemLog called but a.ctx is nil")
	}
}

func NewApp(database *db.Database, sm *config.SettingsManager) *App {
	reporter := &WailsReporter{}
	walker := engine.NewWalker(database, nil)
	compare := engine.NewComparisonEngine()
	resultsManager := engine.NewResultsManager()

	scanner := engine.NewScanner(walker, database, reporter, compare, resultsManager)
	walker.SetReporter(scanner)

	return &App{
		db:             database,
		settings:       sm,
		compare:        compare,
		scanner:        scanner,
		resultsManager: resultsManager,
		reporter:       reporter,
	}
}

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

type DuplicateStats struct {
	TotalGroups int `json:"total_groups"`
	TotalFiles  int `json:"total_files"`
}

func (a *App) GetDuplicateStats() DuplicateStats {
	_, totalFiles := a.resultsManager.GetAll()
	return DuplicateStats{
		TotalGroups: a.resultsManager.GetResultsWrapped(0, 0).Total,
		TotalFiles:  totalFiles,
	}
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
	if err := a.db.AddIgnoredGroup(label, hashes); err != nil {
		return err
	}
	a.resultsManager.RemoveFiles(paths)
	a.reporter.BroadcastResultsUpdated("excluded", a.resultsManager.Count())
	return nil
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

var extractThumbnailFn = func(path string, duration float64, i int) ([]byte, error) {
	timestamp := media.GetStableTimestamp(i, duration)
	data, err := media.ExtractThumbnailNative(context.Background(), path, timestamp, 160, 0)
	if err != nil {
		fmt.Printf("Native thumbnail extract failed for %s (%v), falling back to CLI\n", path, err)
		data, err = media.ExtractThumbnail(context.Background(), path, timestamp, 160, 90)
	}
	return data, err
}

// GetThumbnails returns thumbnails for the given path, duration and count.
// TODO: TECH DEBT - Implement persistent thumbnail cache storing BLOBs in SQLite.
func (a *App) GetThumbnails(path string, duration float64, count int) ([]string, error) {
	if count <= 0 {
		count = 4
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			log.Printf("GetThumbnails: file not found: %s", path)
			return []string{}, fmt.Errorf("file not found")
		}
		log.Printf("GetThumbnails: stat error for %s: %v", path, err)
		return []string{}, err
	}
	type thumbResult struct {
		timestamp float64
		data      string
	}
	var thumbs []thumbResult

	for i := 0; i < count; i++ {
		ts := media.GetStableTimestamp(i, duration)
		data, err := extractThumbnailFn(path, duration, i)
		if err != nil || data == nil {
			continue // Skip failed frames
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		thumbs = append(thumbs, thumbResult{
			timestamp: ts,
			data:      fmt.Sprintf("data:image/jpeg;base64,%s", b64),
		})
	}

	sort.Slice(thumbs, func(i, j int) bool {
		return thumbs[i].timestamp < thumbs[j].timestamp
	})

	var results []string
	for _, t := range thumbs {
		results = append(results, t.data)
	}
	return results, nil
}

func (a *App) OpenFile(path string) error {
	log.Printf("App: Opening file %s", path)
	return exec.Command(utils.Resolve("open"), path).Run()
}

func (a *App) RenameFile(oldPath string, newPath string) error {
	err := os.Rename(oldPath, newPath)
	if err != nil {
		return err
	}
	a.resultsManager.RenameFile(oldPath, newPath)
	a.reporter.BroadcastResultsUpdated("renamed", a.resultsManager.Count())
	return nil
}

func (a *App) DeleteFiles(paths []string) error {
	fmt.Printf("Backend: Deleting %d files\n", len(paths))
	a.reporter.BroadcastLog("warning", fmt.Sprintf("Deleting %d files from disk", len(paths)))
	for _, p := range paths {
		err := os.Remove(p)
		if err != nil && !os.IsNotExist(err) {
			fmt.Printf("Error deleting file from disk %s: %v\n", p, err)
			a.reporter.BroadcastLog("error", fmt.Sprintf("Failed to delete from disk: %s", filepath.Base(p)))
		}
		err = a.db.DeleteFile(p)
		if err != nil {
			fmt.Printf("Error deleting file from DB %s: %v\n", p, err)
		} else {
			a.reporter.BroadcastLog("success", fmt.Sprintf("Deleted: %s", filepath.Base(p)))
		}
	}
	a.resultsManager.RemoveFiles(paths)
	a.reporter.BroadcastResultsUpdated("deleted", a.resultsManager.Count())
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

	if err := a.db.Close(); err != nil {
		return fmt.Errorf("failed to close current database: %v", err)
	}
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

	newDB, err := db.New(currentPath)
	if err != nil {
		return fmt.Errorf("failed to reconnect to imported database: %v", err)
	}
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

type DirListResponse struct {
	Path string   `json:"path"`
	Dirs []string `json:"dirs"`
}

func (a *App) ListDirs(path string) (DirListResponse, error) {
	if path == "" {
		path = "/"
	}
	// On Windows, if path is just a drive letter without slash, ReadDir might fail or behave weirdly.
	// But since the user is on Mac, we assume Unix paths.
	entries, err := os.ReadDir(path)
	if err != nil {
		return DirListResponse{}, err
	}

	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() && (len(entry.Name()) == 0 || entry.Name()[0] != '.') {
			dirs = append(dirs, entry.Name())
		}
	}

	return DirListResponse{
		Path: path,
		Dirs: dirs,
	}, nil
}

func (a *App) CheckDependencies() media.DependencyStatus {
	return media.CheckDependencies()
}

func (a *App) GetStats() (map[string]any, error) {
	return a.db.GetStats()
}

func (a *App) GetScanStatus() map[string]any {
	return a.scanner.Status()
}

func (a *App) DownloadDependencies() error {
	a.reporter.BroadcastLog("info", "Starting FFmpeg auto-download...")
	return media.DownloadDependencies(a.ctx, func(msg string, progress float64) {
		runtime.EventsEmit(a.ctx, "download_progress", map[string]any{
			"message":  msg,
			"progress": progress,
		})
	})
}

var AppVersion = "dev"

func (a *App) GetDebugInfo() map[string]any {
	instanceID := a.db.GetInstanceID()

	dbStats := map[string]any{}
	if s, err := a.db.GetStats(); err == nil {
		dbStats = s
	}

	cfg := a.settings.Get()

	return map[string]any{
		"instance_id": instanceID,
		"version":     AppVersion,
		"os":          goruntime.GOOS,
		"arch":        goruntime.GOARCH,
		"go_version":  goruntime.Version(),
		"db_path":     a.db.GetPath(),
		"db_stats":    dbStats,
		"settings": map[string]any{
			"thumbnails":    cfg.Thumbnails,
			"concurrency":   cfg.Concurrency,
			"similarity":    cfg.Percent,
			"duration_diff": cfg.PercentDurationDifference,
		},
	}
}
