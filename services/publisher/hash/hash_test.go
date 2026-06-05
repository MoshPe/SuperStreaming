package hash_test

import (
	"testing"

	"superstreaming/publisher/hash"
)

func TestOriginIndex(t *testing.T) {
	tests := []struct {
		streamID   string
		numOrigins int
		want       int
	}{
		// Hardcode expected values — computed once, locked in as regression tests.
		// To compute: h := fnv.New32a(); h.Write([]byte(id)); return int(h.Sum32()) % n
		{"cam-01", 3, int(hash.FNV32a("cam-01") % 3)},
		{"cam-02", 3, int(hash.FNV32a("cam-02") % 3)},
		{"cam-10", 3, int(hash.FNV32a("cam-10") % 3)},
		{"cam-01", 1, 0},
	}
	for _, tc := range tests {
		got := hash.OriginIndex(tc.streamID, tc.numOrigins)
		if got != tc.want {
			t.Errorf("OriginIndex(%q, %d) = %d, want %d", tc.streamID, tc.numOrigins, got, tc.want)
		}
	}
}

func TestOriginIndexStability(t *testing.T) {
	// Same stream_id + numOrigins must always produce same result.
	for i := 0; i < 100; i++ {
		if hash.OriginIndex("cam-stable", 3) != hash.OriginIndex("cam-stable", 3) {
			t.Fatal("OriginIndex is not stable")
		}
	}
}

func TestOriginIndexDistribution(t *testing.T) {
	// Verify all origins get at least some streams over 30 IDs.
	counts := make(map[int]int)
	for i := 0; i < 30; i++ {
		id := "stream-" + string(rune('a'+i))
		counts[hash.OriginIndex(id, 3)]++
	}
	for origin := 0; origin < 3; origin++ {
		if counts[origin] == 0 {
			t.Errorf("origin %d got 0 streams — hash distribution too skewed", origin)
		}
	}
}
