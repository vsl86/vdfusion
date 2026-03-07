package api

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log" // Added for logging exec commands
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"vdfusion/internal/config"
	"vdfusion/internal/db"
	"vdfusion/internal/engine"
	"vdfusion/internal/media"
	"vdfusion/internal/utils"
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
		r.Get("/fs/ls", s.handleListDirs)
		r.Get("/suspicious-files", s.handleGetSuspiciousFiles)
		r.Get("/stats", s.handleGetStats)
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

	// If no pagination provided, check legacy behavior
	if limit <= 0 {
		if offsetStr == "" && limitStr == "" {
			// No params: Return ALL
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

	// Normalize: items may be identifier hashes OR file paths. Convert paths to identifier hashes.
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
	// Dedup hashes
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
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

	var results []string
	for i := 0; i < count; i++ {
		timestamp := media.GetStableTimestamp(i, duration)
		data, err := media.ExtractThumbnailNative(r.Context(), path, timestamp, 160, 90)
		if err != nil {
			fmt.Printf("Native thumbnail extract failed for %s (%v), falling back to CLI\n", path, err)
			data, err = media.ExtractThumbnail(r.Context(), path, timestamp, 160, 90)
			if err != nil {
				fmt.Printf("CLI thumbnail extract ALSO failed for %s (%v)\n", path, err)
			} else if data != nil {
				fmt.Printf("CLI thumbnail extract succeeded for %s\n", path)
			}
		}
		if err != nil || data == nil {
			continue
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		results = append(results, fmt.Sprintf("data:image/jpeg;base64,%s", b64))
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
		os.Remove(p)
		s.db.DeleteFile(p)
	}
	s.resultsManager.RemoveFiles(body.Paths)

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

	// Rename in DB
	if err := s.db.RenameFile(body.OldPath, body.NewPath); err != nil {
		log.Printf("DB Rename failed: %v", err)
		s.jsonError(w, fmt.Sprintf("DB Rename failed: %v", err), http.StatusInternalServerError)
		return
	}

	s.resultsManager.RenameFile(body.OldPath, body.NewPath)

	log.Printf("Renamed %s to %s", body.OldPath, body.NewPath)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleStreamFile(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		s.jsonError(w, "missing path", http.StatusBadRequest)
		return
	}

	ext := strings.ToLower(filepath.Ext(path))
	// Browsers don't support AVI, MKV, etc. natively.
	// For these, we transcode on the fly.
	if ext == ".avi" || ext == ".mkv" || ext == ".ts" || ext == ".m2ts" {
		log.Printf("Streaming: Transcoding %s for browser compatibility", path)

		stdout, cmd, err := media.StreamTranscoded(r.Context(), path)
		if err != nil {
			log.Printf("Transcoding failed to start: %v", err)
			s.jsonError(w, "transcoding failed", http.StatusInternalServerError)
			return
		}
		defer stdout.Close()

		// Flush headers
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Transfer-Encoding", "chunked")
		w.WriteHeader(http.StatusOK)

		// Copy stdout to response
		if _, err := io.Copy(w, stdout); err != nil {
			log.Printf("Transcoding stream interrupted: %v", err)
		}

		// Ensure process is cleaned up
		cmd.Process.Kill()
		cmd.Wait()
		return
	}

	// Basic security check: only serve if it's in a search directory or common video dir?
	// For now, trust the internal user as this is a local tool.
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

	// Try ffplay first
	cmd := exec.Command(utils.Resolve("ffplay"), "-i", body.Path, "-autoexit")
	if err := cmd.Start(); err != nil {
		log.Printf("ffplay failed: %v, trying 'open'", err)
		// Fallback to open
		if err2 := exec.Command(utils.Resolve("open"), body.Path).Run(); err2 != nil {
			log.Printf("open failed: %v", err2)
			s.jsonError(w, fmt.Sprintf("Failed to open file: %v", err2), http.StatusInternalServerError)
			return
		}
	} else {
		log.Printf("ffplay started successfully")
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
	json.NewEncoder(w).Encode(map[string]int{"removed_count": removed})
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
		// Filter out hidden directories and ensure it's a directory
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
			// Skip logging for successful streaming chunks (206 Partial Content)
			// effectively silencing the high-frequency logs during video playback.
			if r.URL.Path == "/api/files/stream" && status == http.StatusPartialContent {
				return
			}

			// For anything else, log it
			log.Printf("\"%s %s %s\" from %s - %d %dB in %v",
				r.Method, r.URL.RequestURI(), r.Proto, r.RemoteAddr,
				status, ww.BytesWritten(), time.Since(t1),
			)
		}()
		next.ServeHTTP(ww, r)
	})
}

func (s *Server) Start(addr string) error {
	return http.ListenAndServe(addr, s.Router)
}
