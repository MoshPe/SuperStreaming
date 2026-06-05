package mediamtx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type path struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}

type pathList struct {
	Items []path `json:"items"`
}

var httpClient = &http.Client{Timeout: 2 * time.Second}

// ListPaths returns the names of all ready paths from one MediaMTX origin pod.
// Returns an error if the origin is unreachable — callers should skip and continue.
func ListPaths(ctx context.Context, host string, port int) ([]string, error) {
	url := fmt.Sprintf("http://%s:%d/v3/paths/list", host, port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mediamtx %s:%d returned %d", host, port, resp.StatusCode)
	}

	var pl pathList
	if err := json.NewDecoder(resp.Body).Decode(&pl); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	var names []string
	for _, p := range pl.Items {
		if p.Ready {
			names = append(names, p.Name)
		}
	}
	return names, nil
}
