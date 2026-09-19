package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/model"
)

// SSEHub coordinates real-time Server-Sent Events broadcasting to multiple connected clients.
type SSEHub struct {
	mu         sync.RWMutex
	clients    map[chan []byte]struct{}
	broadcast  chan []byte
	register   chan chan []byte
	unregister chan chan []byte
	stopChan   chan struct{}
}

// NewSSEHub creates and starts a new SSEHub.
func NewSSEHub() *SSEHub {
	hub := &SSEHub{
		clients:    make(map[chan []byte]struct{}),
		broadcast:  make(chan []byte, 256),
		register:   make(chan chan []byte),
		unregister: make(chan chan []byte),
		stopChan:   make(chan struct{}),
	}
	go hub.run()
	return hub
}

// run handles client registration, unregistration, and message fan-out.
func (h *SSEHub) run() {
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = struct{}{}
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client)
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client <- msg:
				default:
					// Slow consumer: skip to prevent blocking the hub
				}
			}
			h.mu.RUnlock()

		case <-heartbeat.C:
			// Send SSE comment as keep-alive heartbeat to prevent proxy timeout
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client <- []byte(": keep-alive\n\n"):
				default:
				}
			}
			h.mu.RUnlock()

		case <-h.stopChan:
			h.mu.Lock()
			for client := range h.clients {
				delete(h.clients, client)
				close(client)
			}
			h.mu.Unlock()
			return
		}
	}
}

// BroadcastSnapshot serializes and broadcasts a metric snapshot to all connected clients.
func (h *SSEHub) BroadcastSnapshot(snapshot model.SystemSnapshot) {
	data, err := json.Marshal(snapshot)
	if err != nil {
		log.Printf("[SSE] Error marshaling snapshot: %v", err)
		return
	}

	ssePayload := fmt.Sprintf("event: metrics\ndata: %s\n\n", data)
	select {
	case h.broadcast <- []byte(ssePayload):
	default:
		// Broadcast channel is full, drop to preserve latency
	}
}

// ClientCount returns the number of currently connected SSE clients.
func (h *SSEHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Close stops the SSE hub and disconnects all clients.
func (h *SSEHub) Close() {
	close(h.stopChan)
}

// HandleSSEStream handles GET /api/v1/stream HTTP connections.
func (h *APIHandler) HandleSSEStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported by client", http.StatusBadRequest)
		return
	}

	// Set standard SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create a client channel with buffer to handle burst transmissions
	clientChan := make(chan []byte, 32)
	h.sseHub.register <- clientChan
	defer func() {
		h.sseHub.unregister <- clientChan
	}()

	// Send initial greeting / connection confirmation
	fmt.Fprintf(w, ": connected to sugi sse\n\n")
	flusher.Flush()

	// Send latest snapshot immediately if available
	if latest, ok := h.ringBuffer.GetLatest(); ok {
		if data, err := json.Marshal(latest); err == nil {
			fmt.Fprintf(w, "event: metrics\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case msg, ok := <-clientChan:
			if !ok {
				return
			}
			_, err := w.Write(msg)
			if err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
