package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/model"
)

func TestSSEHub_Broadcast(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Close()

	client := make(chan []byte, 10)
	hub.register <- client

	// Give goroutine a moment to register
	time.Sleep(20 * time.Millisecond)
	if hub.ClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", hub.ClientCount())
	}

	snap := model.SystemSnapshot{
		Timestamp: time.Now().UTC(),
		CPU: model.CPUStats{
			TotalUsage: 55.0,
		},
	}
	hub.BroadcastSnapshot(snap)

	select {
	case msg := <-client:
		str := string(msg)
		if !strings.Contains(str, "event: metrics") {
			t.Errorf("expected event: metrics, got %s", str)
		}
		if !strings.Contains(str, "55") {
			t.Errorf("expected CPU 55 in payload, got %s", str)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for broadcast message")
	}

	hub.unregister <- client
	time.Sleep(20 * time.Millisecond)
	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients after unregister, got %d", hub.ClientCount())
	}
}

func TestHandleSSEStream_Integration(t *testing.T) {
	_, handler, _ := setupTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stream", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(w, req)
		close(done)
	}()

	// Let connection establish and receive initial headers/comments
	time.Sleep(50 * time.Millisecond)

	// Cancel context to simulate client closing the tab
	cancel()

	select {
	case <-done:
		// Connection closed cleanly
	case <-time.After(1 * time.Second):
		t.Fatal("stream handler did not exit after context cancel")
	}

	body := w.Body.String()
	scanner := bufio.NewScanner(strings.NewReader(body))
	var foundGreeting bool
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "connected to sugi sse") {
			foundGreeting = true
			break
		}
	}

	if !foundGreeting {
		t.Errorf("expected SSE greeting comment, got: %s", body)
	}
}
