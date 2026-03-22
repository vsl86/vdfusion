package api

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	goruntime "runtime"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"vdfusion/internal/config"
	"vdfusion/internal/db"
	"vdfusion/internal/engine"
	"vdfusion/internal/media"
	"vdfusion/internal/utils"
	"vdfusion/internal/version"
)

type Server struct {
	Router         *chi.Mux
	db             *db.Database
	hub            *Hub
	compare        *engine.ComparisonEngine
	scanner        *engine.Scanner
	resultsManager *engine.ResultsManager
	settings       *config.SettingsManager
	Assets         embed.FS
}

func NewServer(database *db.Database, hub *Hub, scanner *engine.Scanner, resultsManager *engine.ResultsManager, sm *config.SettingsManager, assets embed.FS) *Server {
	s := &Server{
		Router:         chi.NewRouter(),
		db:             database,
		hub:            hub,
		compare:        engine.NewComparisonEngine(),
		scanner:        scanner,
		resultsManager: resultsManager,
		settings:       sm,
		Assets:         assets,
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

func (s *Server) setupMiddleware() {
	s.Router.Use(s.quietLogger)
	s.Router.Use(middleware.Recoverer)
	s.Router.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
	}))
}

func (s *Server) setupRoutes() {
	s.Router.Get("/ws", s.handleWS)

	s.Router.Route("/api", func(r chi.Router) {
		r.Post("/scan/start", s.handleScanStart)
		r.Post("/scan/stop", s.handleScanStop)
		r.Get("/scan/status", s.handleGetScanStatus)
		r.Get("/results", s.handleGetResults)
		r.Get("/exclusions", s.handleGetExclusions)
		r.Post("/exclude", s.handleExcludeGroup)
		r.Get("/settings", s.handleGetSettings)
		r.Put("/settings", s.handlePutSettings)
		r.Get("/thumbnails", s.handleGetThumbnails)
		r.Get("/files/stream", s.handleStreamFile)
		r.Post("/files/delete", s.handleDeleteFiles)
		r.Post("/files/rename", s.handleRenameFile)
		r.Post("/files/open", s.handleOpenFile)
		r.Get("/ignored-groups", s.handleGetIgnoredGroups)
		r.Delete("/ignored-groups/{id}", s.handleDeleteIgnoredGroup)
		r.Post("/db/reset", s.handleResetDB)
		r.Post("/db/cleanup", s.handleCleanupDB)
		r.Post("/db/purge-blacklist", s.handlePurgeBlacklist)
		r.Get("/fs/ls", s.handleListDirs)
		r.Get("/suspicious-files", s.handleGetSuspiciousFiles)
		r.Get("/stats", s.handleGetStats)
		r.Get("/debug", s.handleGetDebugInfo)
		r.Get("/updates/check", s.handleCheckUpdates)
	})

	// Static assets (built frontend)
	// We expect assets to have frontend/dist in them because of the embed directive in main.go
	content, _ := fs.Sub(s.Assets, "frontend/dist")
	s.Router.Handle("/*", http.FileServer(http.FS(content)))
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	s.hub.AddClient(c)
}

func (s *Server) jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func (s *Server) handleScanStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paths []string `json:"paths"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	pathsToScan := body.Paths
	if len(pathsToScan) == 0 {
		pathsToScan = s.settings.Get().IncludeList
	}

	if len(pathsToScan) == 0 {
		s.jsonError(w, "no paths provided to scan", http.StatusBadRequest)
		return
	}

	s.scanner.Start(context.Background(), pathsToScan, s.settings.Get())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func (s *Server) handleScanStop(w http.ResponseWriter, r *http.Request) {
	s.scanner.Stop()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

func (s *Server) handleGetScanStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.scanner.Status())
}

func (s *Server) handleGetResults(w http.ResponseWriter, r *http.Request) {
	offsetStr := r.URL.Query().Get("offset")
	limitStr := r.URL.Query().Get("limit")

	var offset, limit int
	fmt.Sscanf(offsetStr, "%d", &offset)
	fmt.Sscanf(limitStr, "%d", &limit)

	w.Header().Set("Content-Type", "application/json")

	if limit <= 0 {
		if offsetStr == "" && limitStr == "" {
			resp := s.resultsManager.GetResultsWrapped(0, 0)
			json.NewEncoder(w).Encode(resp)
			return
		}
		limit = 50
	}

	resp := s.resultsManager.GetResultsWrapped(offset, limit)
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleGetExclusions(w http.ResponseWriter, r *http.Request) {
	ignoredGroups, err := s.db.GetIgnoredGroups()
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ignoredGroups)
}

func (s *Server) handleExcludeGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label            string   `json:"label"`
		IdentifierHashes []string `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if body.Label == "" {
		body.Label = "Ignored Group"
	}

	raw := body.IdentifierHashes
	var paths []string
	var hashes []string
	for _, item := range raw {
		trim := strings.TrimSpace(item)
		if len(trim) == 64 {
			isHex := true
			for i := range 64 {
				c := trim[i]
				if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
					isHex = false
					break
				}
			}
			if isHex {
				hashes = append(hashes, strings.ToUpper(trim))
				continue
			}
		}
		paths = append(paths, item)
	}

	if len(paths) > 0 {
		files, err := s.db.GetFilesByPaths(paths)
		if err == nil {
			for _, f := range files {
				hashes = append(hashes, f.GetIdentifierHash())
			}
		}
	}
	seen := map[string]bool{}
	var uniq []string
	for _, h := range hashes {
		hu := strings.ToUpper(h)
		if !seen[hu] {
			seen[hu] = true
			uniq = append(uniq, hu)
		}
	}
	if len(uniq) < 2 {
		s.jsonError(w, "at least two files required to exclude", http.StatusBadRequest)
		return
	}

	err := s.db.AddIgnoredGroup(body.Label, uniq)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.resultsManager.RemoveFiles(paths)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	s.hub.BroadcastLog("info", fmt.Sprintf("Excluded group: %s (%d files)", body.Label, len(uniq)))
	s.hub.BroadcastResultsUpdated("excluded", s.resultsManager.Count())
	json.NewEncoder(w).Encode(map[string]any{"status": "excluded", "files_count": len(uniq)})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.settings.Get())
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var newSettings config.Settings
	if err := json.NewDecoder(r.Body).Decode(&newSettings); err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.settings.Update(newSettings); err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(s.settings.Get())
}

// handleGetThumbnails extracts multiple thumbnails for a file.
// TODO: TECH DEBT - Implement persistent thumbnail cache (see App.GetThumbnails).
func (s *Server) handleGetThumbnails(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	durationStr := r.URL.Query().Get("duration")
	countStr := r.URL.Query().Get("count")

	if path == "" {
		s.jsonError(w, "missing path", http.StatusBadRequest)
		return
	}

	var duration float64
	fmt.Sscanf(durationStr, "%f", &duration)

	var count int
	fmt.Sscanf(countStr, "%d", &count)
	if count <= 0 {
		count = 4
	}

	type thumbResult struct {
		timestamp float64
		data      string
	}
	var thumbs []thumbResult

	for i := 0; i < count; i++ {
		timestamp := media.GetStableTimestamp(i, duration)
		data, err := media.ExtractThumbnailNative(r.Context(), path, timestamp, 160, 90)
		if err != nil {
			fmt.Printf("Native thumbnail extract failed for %s (%v), falling back to CLI\n", path, err)
			data, err = media.ExtractThumbnail(r.Context(), path, timestamp, 160, 90)
			if err != nil {
				fmt.Printf("CLI thumbnail extract ALSO failed for %s (%v)\n", path, err)
			}
		}
		if err != nil || data == nil {
			continue
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		thumbs = append(thumbs, thumbResult{
			timestamp: timestamp,
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (s *Server) handleDeleteFiles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	for _, p := range body.Paths {
		if err := os.Remove(p); err == nil {
			s.hub.BroadcastLog("info", "Deleted file: "+p)
		} else {
			s.hub.BroadcastLog("warning", fmt.Sprintf("Failed to delete %s: %v", p, err))
		}
		s.db.DeleteFile(p)
	}
	s.resultsManager.RemoveFiles(body.Paths)
	s.hub.BroadcastResultsUpdated("deleted", s.resultsManager.Count())

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleRenameFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OldPath string `json:"oldPath"`
		NewPath string `json:"newPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := os.Rename(body.OldPath, body.NewPath); err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.db.RenameFile(body.OldPath, body.NewPath); err != nil {
		log.Printf("DB Rename failed: %v", err)
		s.jsonError(w, fmt.Sprintf("DB Rename failed: %v", err), http.StatusInternalServerError)
		return
	}

	s.resultsManager.RenameFile(body.OldPath, body.NewPath)

	log.Printf("Renamed %s to %s", body.OldPath, body.NewPath)
	s.hub.BroadcastLog("info", fmt.Sprintf("Renamed %s to %s", body.OldPath, body.NewPath))
	s.hub.BroadcastResultsUpdated("renamed", s.resultsManager.Count())
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleStreamFile(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		s.jsonError(w, "missing path", http.StatusBadRequest)
		return
	}

	ext := strings.ToLower(filepath.Ext(path))
	isNative := ext == ".mp4" || ext == ".webm"
	if !isNative {
		log.Printf("Streaming: Transcoding %s for browser compatibility", path)

		stdout, cmd, err := media.StreamTranscoded(r.Context(), path)
		if err != nil {
			log.Printf("Transcoding failed to start: %v", err)
			s.jsonError(w, "transcoding failed", http.StatusInternalServerError)
			return
		}
		defer stdout.Close()

		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Transfer-Encoding", "chunked")
		w.WriteHeader(http.StatusOK)

		if _, err := io.Copy(w, stdout); err != nil {
			log.Printf("Transcoding stream interrupted: %v", err)
		}

		cmd.Process.Kill()
		cmd.Wait()
		return
	}

	http.ServeFile(w, r, path)
}

func (s *Server) handleOpenFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("Attempting to open file: %s", body.Path)

	// Use 'open' as the primary file opener
	if err := exec.Command(utils.Resolve("open"), body.Path).Run(); err != nil {
		log.Printf("Failed to open file with 'open': %v", err)
		s.jsonError(w, fmt.Sprintf("Failed to open file: %v", err), http.StatusInternalServerError)
		return
	} else {
		log.Printf("File opened successfully with 'open'")
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleGetIgnoredGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.db.GetIgnoredGroups()
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(groups)
}

func (s *Server) handleDeleteIgnoredGroup(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	fmt.Sscanf(idStr, "%d", &id)

	if err := s.db.DeleteIgnoredGroup(id); err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func (s *Server) handleResetDB(w http.ResponseWriter, r *http.Request) {
	if err := s.db.Reset(); err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "reset"})
}

func (s *Server) handleCleanupDB(w http.ResponseWriter, r *http.Request) {
	removed, err := s.db.Cleanup()
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	s.hub.BroadcastLog("info", fmt.Sprintf("Manual DB Cleanup: Removed %d orphaned records", removed))
	json.NewEncoder(w).Encode(map[string]int{"removed_count": removed})
}

func (s *Server) handlePurgeBlacklist(w http.ResponseWriter, r *http.Request) {
	if err := s.db.DeleteAllIgnoredGroups(); err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	s.hub.BroadcastLog("info", "Purged all manual exclusions (blacklist)")
	json.NewEncoder(w).Encode(map[string]string{"status": "purged"})
}

func (s *Server) handleGetSuspiciousFiles(w http.ResponseWriter, r *http.Request) {
	records, err := s.db.GetSuspiciousFiles()
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type warningInfo struct {
		Message string `json:"message"`
		Fix     string `json:"fix"`
	}
	type suspiciousFile struct {
		Path     string        `json:"path"`
		Warnings []warningInfo `json:"warnings"`
	}
	var result []suspiciousFile
	for _, r := range records {
		var warnings []warningInfo
		seen := map[string]bool{}
		for _, w := range r.Warnings {
			if seen[w] {
				continue
			}
			seen[w] = true
			warnings = append(warnings, warningInfo{
				Message: w,
				Fix:     suggestFix(w, r.Path),
			})
		}
		result = append(result, suspiciousFile{
			Path:     r.Path,
			Warnings: warnings,
		})
	}
	if result == nil {
		result = []suspiciousFile{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.db.GetStats()
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleGetDebugInfo(w http.ResponseWriter, r *http.Request) {
	instanceID := s.db.GetInstanceID()
	dbStats, _ := s.db.GetStats()
	cfg := s.settings.Get()

	info := map[string]any{
		"instance_id": instanceID,
		"version":     version.Version,
		"commit":      version.Commit,
		"os":          goruntime.GOOS,
		"arch":        goruntime.GOARCH,
		"db_path":     s.db.GetPath(),
		"db_stats":    dbStats,
		"settings": map[string]any{
			"thumbnails":    cfg.Thumbnails,
			"concurrency":   cfg.Concurrency,
			"similarity":    cfg.Percent,
			"duration_diff": cfg.PercentDurationDifference,
		},
		"dependencies": media.CheckDependencies(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func suggestFix(warning, path string) string {
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

func (s *Server) handleListDirs(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() && (len(entry.Name()) == 0 || entry.Name()[0] != '.') {
			dirs = append(dirs, entry.Name())
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"path": path,
		"dirs": dirs,
	})
}

func (s *Server) quietLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		t1 := time.Now()
		defer func() {
			status := ww.Status()
			if r.URL.Path == "/api/files/stream" && status == http.StatusPartialContent {
				return
			}

			log.Printf("\"%s %s %s\" from %s - %d %dB in %v",
				r.Method, r.URL.RequestURI(), r.Proto, r.RemoteAddr,
				status, ww.BytesWritten(), time.Since(t1),
			)
		}()
		next.ServeHTTP(ww, r)
	})
}

func (s *Server) handleCheckUpdates(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/vsl86/vdfusion/releases/latest")
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Body    string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"current":          version.Version,
		"latest":           release.TagName,
		"url":              release.HTMLURL,
		"notes":            release.Body,
		"update_available": s.isNewer(version.Version, release.TagName),
	})
}

func (s *Server) isNewer(current, latest string) bool {
	if current == "v0.0.0-dev" || current == "v0.0.0-latest" {
		return false
	}

	parse := func(v string) [3]int {
		v = strings.TrimPrefix(v, "v")
		parts := strings.Split(v, ".")
		var res [3]int
		for i := 0; i < len(parts) && i < 3; i++ {
			clean := parts[i]
			if idx := strings.Index(clean, "-"); idx != -1 {
				clean = clean[:idx]
			}
			fmt.Sscanf(clean, "%d", &res[i])
		}
		return res
	}

	c := parse(current)
	l := parse(latest)
	for i := 0; i < 3; i++ {
		if l[i] > c[i] {
			return true
		}
		if l[i] < c[i] {
			return false
		}
	}
	return false
}

func (s *Server) Start(addr string) error {
	return http.ListenAndServe(addr, s.Router)
}
