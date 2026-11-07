package neural

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

var (
	serverMu sync.Mutex
	srv      *http.Server
	srvAddr  string
)

// StartServer starts the neural HTTP server on the given address (e.g. "127.0.0.1:8765" or "0.0.0.0:8765").
// If CoreML is not yet loaded, it attempts to load it using the model found by GetModelPath().
func StartServer(bindAddr string) error {
	serverMu.Lock()
	defer serverMu.Unlock()

	if srv != nil {
		return nil // already running
	}

	if !IsCoreMLLoaded() {
		mPath := GetModelPath()
		if mPath == "" {
			return errors.New("neural model is not downloaded yet")
		}
		if err := InitCoreML(mPath); err != nil {
			return fmt.Errorf("failed to load CoreML model: %w", err)
		}
	}

	mux := http.NewServeMux()

	// GET /health
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !IsCoreMLLoaded() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": "model not loaded"})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})

	// GET /info
	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":     "ok",
			"model":      "clip-vit-b32-visual_fp16_bs1",
			"device":     "Apple Neural Engine (ANE)",
			"batch_size": 1,
			"backend":    "coreml-native",
		})
	})

	// POST /embed
	mux.HandleFunc("/embed", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := r.ParseMultipartForm(64 << 20); err != nil {
			http.Error(w, fmt.Sprintf("invalid multipart form: %v", err), http.StatusBadRequest)
			return
		}

		if r.MultipartForm == nil || r.MultipartForm.File == nil {
			http.Error(w, "no multipart files uploaded", http.StatusBadRequest)
			return
		}

		fileHeaders := r.MultipartForm.File["images"]
		if len(fileHeaders) == 0 {
			http.Error(w, "no 'images' files in form data", http.StatusBadRequest)
			return
		}

		images := make([][]byte, 0, len(fileHeaders))
		for _, fh := range fileHeaders {
			f, err := fh.Open()
			if err != nil {
				http.Error(w, fmt.Sprintf("open file %s: %v", fh.Filename, err), http.StatusBadRequest)
				return
			}
			data, err := io.ReadAll(f)
			_ = f.Close()
			if err != nil {
				http.Error(w, fmt.Sprintf("read file %s: %v", fh.Filename, err), http.StatusBadRequest)
				return
			}
			images = append(images, data)
		}

		embeddings, err := EmbedImages(images)
		if err != nil {
			http.Error(w, fmt.Sprintf("inference error: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"embeddings": embeddings,
		})
	})

	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return fmt.Errorf("failed to bind %s: %w", bindAddr, err)
	}

	actualAddr := listener.Addr().String()
	srvAddr = actualAddr

	s := &http.Server{
		Handler:      mux,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	srv = s

	go func() {
		_ = s.Serve(listener)
	}()

	return nil
}

// StopServer shuts down the HTTP server.
func StopServer() error {
	serverMu.Lock()
	defer serverMu.Unlock()

	if srv == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := srv.Shutdown(ctx)
	srv = nil
	srvAddr = ""
	return err
}

// IsServerRunning returns true if the local HTTP server is running.
func IsServerRunning() bool {
	serverMu.Lock()
	defer serverMu.Unlock()
	return srv != nil
}

// GetServerAddr returns the bound address of the server.
func GetServerAddr() string {
	serverMu.Lock()
	defer serverMu.Unlock()
	return srvAddr
}

// GetLocalIPs returns non-loopback IPv4 addresses of this machine.
func GetLocalIPs() []string {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && !ip.IsLoopback() && ip.To4() != nil {
				ips = append(ips, ip.String())
			}
		}
	}
	return ips
}

