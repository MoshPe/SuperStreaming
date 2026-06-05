package handlers

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamsHandler_SendsSSEOnConnect(t *testing.T) {
	h := NewStreamsHandler([]string{"127.0.0.1:19997"}, 100*time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/api/streams/live", nil)

	pr, pw := chanPipe()
	rw := &flushWriter{pw: pw, header: make(http.Header), code: 200}

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(rw, req)
	}()

	scanner := bufio.NewScanner(pr)
	lineCh := make(chan string, 1)
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "data:") {
				lineCh <- line
				return
			}
		}
	}()

	select {
	case line := <-lineCh:
		if !strings.Contains(line, "streams") && !strings.Contains(line, "heartbeat") {
			t.Errorf("unexpected SSE line: %q", line)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for first SSE event")
	}
}
