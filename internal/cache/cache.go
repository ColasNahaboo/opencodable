// Package cache stores fetched JSON payloads on disk with a TTL.
package cache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type entry struct {
	FetchedAt time.Time       `json:"fetched_at"`
	Payload   json.RawMessage `json:"payload"`
}

func dir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(base, "opencodable")
	return d, os.MkdirAll(d, 0o755)
}

// Load returns the cached payload for key if fresh enough, otherwise calls
// fetch and stores the result. refresh bypasses a fresh cache; offline makes
// a stale cache mandatory.
func Load(key string, ttl time.Duration, refresh, offline bool, fetch func() ([]byte, error)) ([]byte, error) {
	d, err := dir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(d, key)
	var ent entry
	if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &ent) == nil {
		if !refresh && time.Since(ent.FetchedAt) < ttl {
			return ent.Payload, nil
		}
		if offline {
			return ent.Payload, nil
		}
	}
	if offline {
		return nil, errors.New("offline mode but no cache entry for " + key)
	}
	b, err := fetch()
	if err != nil {
		// Fall back to a stale cache rather than failing outright.
		if ent.Payload != nil {
			return ent.Payload, nil
		}
		return nil, err
	}
	ent = entry{FetchedAt: time.Now(), Payload: json.RawMessage(b)}
	if out, err := json.Marshal(ent); err == nil {
		_ = os.WriteFile(path, out, 0o644)
	}
	return b, nil
}

// Store persists a payload unconditionally (used for live measurements).
func Store(key string, payload []byte) error {
	d, err := dir()
	if err != nil {
		return err
	}
	ent := entry{FetchedAt: time.Now(), Payload: json.RawMessage(payload)}
	out, err := json.Marshal(ent)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, key), out, 0o644)
}
