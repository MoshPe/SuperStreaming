package hash

import "hash/fnv"

// FNV32a returns the FNV-1a 32-bit hash of s.
// Exported so tests can compute expected values without duplicating logic.
func FNV32a(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

// OriginIndex returns the origin pod index for streamID given numOrigins pods.
// Matches the routing formula in the architecture spec:
//
//	fnv32a(stream_id) % num_origins
func OriginIndex(streamID string, numOrigins int) int {
	return int(FNV32a(streamID)) % numOrigins
}
