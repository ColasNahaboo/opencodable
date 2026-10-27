// Package cache stores fetched JSON payloads on disk with a TTL.
package cache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	_ = os.Chmod(d, 0o700)
	return d, nil
}

// safeKey keeps an untrusted key (provider names, model IDs) inside the cache
// directory. It is not a cryptographic mapping; it just prevents traversal.
func safeKey(key string) string {
	key = strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(key)
	key = filepath.Base(key)
	switch key {
	case "", ".", "..":
		return "invalid-key.json"
	}
	return key
}

func readEntry(path string) (entry, bool) {
	var ent entry
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &ent) != nil {
		return entry{}, false
	}
	return ent, true
}

// writeEntry writes atomically: a crash or concurrent run must not leave a
// truncated JSON envelope in the cache.
func writeEntry(path string, ent entry) error {
	b, err := json.Marshal(ent)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// Load returns the cached payload for key if fresh enough, otherwise calls
// fetch and stores the result. refresh bypasses a fresh cache; offline makes
// a stale cache mandatory.
func Load(key string, ttl time.Duration, refresh, offline bool, fetch func() ([]byte, error)) ([]byte, error) {
	d, err := dir()
	if err != nil {
		return nil, err
	}
	key = safeKey(key)
	path := filepath.Join(d, key)
	ent, ok := readEntry(path)
	if ok {
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
	if err := writeEntry(path, entry{FetchedAt: time.Now(), Payload: json.RawMessage(b)}); err != nil {
		// A cache write failure should not discard a good network result.
		return b, nil
	}
	return b, nil
}

// Store persists a payload unconditionally (used for live measurements).
func Store(key string, payload []byte) error {
	d, err := dir()
	if err != nil {
		return err
	}
	return writeEntry(filepath.Join(d, safeKey(key)), entry{FetchedAt: time.Now(), Payload: json.RawMessage(payload)})
}

// Age reports how long ago the cached entry was fetched.
func Age(key string) (time.Duration, bool) {
	d, err := dir()
	if err != nil {
		return 0, false
	}
	ent, ok := readEntry(filepath.Join(d, safeKey(key)))
	if !ok || ent.FetchedAt.IsZero() {
		return 0, false
	}
	return time.Since(ent.FetchedAt), true
}
