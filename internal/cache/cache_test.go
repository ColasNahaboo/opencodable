package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreLoadSafeKey(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	payload := []byte(`{"ok":true}`)
	if err := Store("../../evil.json", payload); err != nil {
		t.Fatal(err)
	}
	got, err := Load("../../evil.json", time.Hour, false, true, func() ([]byte, error) {
		t.Fatal("offline load must not fetch")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Errorf("payload = %s, want %s", got, payload)
	}

	d, err := dir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d, ".._.._evil.json")); err != nil {
		t.Errorf("sanitized cache file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheHome, "evil.json")); !os.IsNotExist(err) {
		t.Errorf("cache key escaped the cache directory: %v", err)
	}
}

func TestLoadFetchesThenCachesAndAge(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	calls := 0
	fetch := func() ([]byte, error) {
		calls++
		return []byte(`{"v":1}`), nil
	}
	if _, err := Load("x.json", time.Hour, false, false, fetch); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("x.json", time.Hour, false, false, fetch); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("fetch calls = %d, want 1 (second load should hit cache)", calls)
	}
	age, ok := Age("x.json")
	if !ok || age < 0 || age > time.Minute {
		t.Errorf("Age = %v, %v; want a fresh duration", age, ok)
	}
}

func TestPeek(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if _, ok := Peek("nope.json"); ok {
		t.Error("Peek on missing key = true")
	}
	payload := []byte(`{"v":2}`)
	if err := Store("peeked.json", payload); err != nil {
		t.Fatal(err)
	}
	got, ok := Peek("peeked.json")
	if !ok || string(got) != string(payload) {
		t.Errorf("Peek = %s, %v; want %s", got, ok, payload)
	}
	if _, ok := Peek("../../etc/passwd"); ok {
		t.Error("Peek must stay inside the cache directory")
	}
}

func TestLoadStaleFallsBackOnFetchError(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := Store("stale.json", []byte(`{"old":true}`)); err != nil {
		t.Fatal(err)
	}
	got, err := Load("stale.json", 0, true, false, func() ([]byte, error) {
		return nil, os.ErrDeadlineExceeded
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"old":true`) {
		t.Errorf("stale payload = %s", got)
	}
}
