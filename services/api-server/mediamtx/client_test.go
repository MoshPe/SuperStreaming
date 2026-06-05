package mediamtx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestListPaths_ReturnsOnlyReadyPaths(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/paths/list" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pathList{
			Items: []path{
				{Name: "cam-01", Ready: true},
				{Name: "cam-02", Ready: false},
				{Name: "cam-03", Ready: true},
			},
		})
	}))
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	parts := strings.SplitN(addr, ":", 2)
	port, _ := strconv.Atoi(parts[1])

	names, err := ListPaths(context.Background(), parts[0], port)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 ready paths, got %d: %v", len(names), names)
	}
	if names[0] != "cam-01" || names[1] != "cam-03" {
		t.Errorf("unexpected paths: %v", names)
	}
}

func TestListPaths_ServerDown_ReturnsError(t *testing.T) {
	_, err := ListPaths(context.Background(), "127.0.0.1", 1)
	if err == nil {
		t.Error("expected error for unreachable server, got nil")
	}
}
