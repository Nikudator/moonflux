// Package uuid provides UUID generation without third-party dependencies.
package uuid

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// NewV7 returns a UUID version 7. The timestamp is UTC Unix milliseconds;
// the remaining bits are cryptographically random. IDs are time-sortable,
// but generation is not guaranteed to be monotonic within the same millisecond.
func NewV7() ([16]byte, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return id, fmt.Errorf("generate UUID randomness: %w", err)
	}
	ms := uint64(time.Now().UTC().UnixMilli())
	id[0] = byte(ms >> 40)
	id[1] = byte(ms >> 32)
	id[2] = byte(ms >> 24)
	id[3] = byte(ms >> 16)
	id[4] = byte(ms >> 8)
	id[5] = byte(ms)
	id[6] = (id[6] & 0x0f) | 0x70 // version 7
	id[8] = (id[8] & 0x3f) | 0x80 // RFC 4122 variant
	return id, nil
}

func String(id [16]byte) string {
	var b [36]byte
	hex.Encode(b[0:8], id[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], id[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], id[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], id[8:10])
	b[23] = '-'
	hex.Encode(b[24:36], id[10:16])
	return string(b[:])
}

// Parse decodes a canonical UUID string (8-4-4-4-12 hex with dashes).
func Parse(s string) ([16]byte, error) {
	var id [16]byte

	if len(s) != 36 {
		return id, fmt.Errorf("invalid UUID length: got %d, want 36", len(s))
	}
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return id, fmt.Errorf("invalid UUID format")
	}

	cleaned := strings.ReplaceAll(s, "-", "")
	if len(cleaned) != 32 {
		return id, fmt.Errorf("invalid UUID hex length")
	}
	if _, err := hex.Decode(id[:], []byte(cleaned)); err != nil {
		return id, fmt.Errorf("invalid UUID hex: %w", err)
	}
	return id, nil
}
