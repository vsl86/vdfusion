package api

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type Hub struct {
	clients   map[*client]struct{}
	broadcast chan []byte
	mu        sync.Mutex
}

type client struct {
	conn *websocket.Conn
}

func NewHub() *Hub {
	return &Hub{
		clients:   make(map[*client]struct{}),
		broadcast: make(chan []byte, 100),
	}
}

func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case msg := <-h.broadcast:
			h.mu.Lock()
			for c := range h.clients {
				// Use a timeout for each client write to prevent one slow client from blocking the hub
				writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				err := c.conn.Write(writeCtx, websocket.MessageText, msg)
				cancel()
				if err != nil {
					log.Printf("WS write error: %v", err)
					c.conn.Close(websocket.StatusPolicyViolation, "write timeout or error")
					delete(h.clients, c)
				}
			}
			h.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

func (h *Hub) AddClient(conn *websocket.Conn) {
	h.mu.Lock()
	h.clients[&client{conn: conn}] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) BroadcastProgress(current, total int, phase string, lastFile string, durationSeconds, estimatedRemainingSeconds float64) {
	msg, _ := json.Marshal(map[string]any{
		"type":                        "progress",
		"current":                     current,
		"total":                       total,
		"phase":                       phase,
		"last_file":                   lastFile,
		"duration_seconds":            durationSeconds,
		"estimated_remaining_seconds": estimatedRemainingSeconds,
	})
	select {
	case h.broadcast <- msg:
	default:
		// Drop message if buffer is full to avoid blocking the engine
	}
}

func (h *Hub) BroadcastLog(severity, message string) {
	msg, _ := json.Marshal(map[string]any{
		"type":     "app_log",
		"severity": severity,
		"message":  message,
		"time":     time.Now().Format("15:04:05"),
	})
	select {
	case h.broadcast <- msg:
	default:
		// Drop message if buffer is full
	}
}
func (h *Hub) BroadcastSystemLog(line string) {
	msg, _ := json.Marshal(map[string]any{
		"type": "system_log",
		"line": line,
	})
	select {
	case h.broadcast <- msg:
	default:
		// Drop message if buffer is full
	}
}
