// Package config reads opencodable.conf, a small key=value file for API
// keys, from the per-user config directory (os.UserConfigDir):
// ~/.config/opencodable/ on Linux, ~/Library/Application Support/opencodable/
// on macOS, %AppData%\opencodable\ on Windows. Lookup order everywhere is
// environment variable, then this file, then a legacy cwd dot-file.
package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// known are the setting names opencodable understands, mirroring the
// environment variables.
var known = map[string]bool{
	"OPENCODE_API_KEY": true,
	"AA_API_KEY":       true,
}

func read() map[string]string {
	vals := map[string]string{}
	dir, err := os.UserConfigDir()
	if err != nil {
		return vals
	}
	b, err := os.ReadFile(filepath.Join(dir, "opencodable", "opencodable.conf"))
	if err != nil {
		return vals
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue // malformed lines are ignored; never echo secret-bearing lines
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		vals[k] = v
	}
	return vals
}

// Value returns the setting for name: environment first, then the config
// file.
func Value(name string) (string, bool) {
	if v := os.Getenv(name); v != "" {
		return v, true
	}
	v, ok := read()[name]
	return v, ok
}

// Source reports where the effective value comes from: "env", "config", or
// "" when the config layer has nothing (callers may still have a fallback).
func Source(name string) string {
	if os.Getenv(name) != "" {
		return "env"
	}
	if _, ok := read()[name]; ok {
		return "config"
	}
	return ""
}

// Unknown lists config-file keys that opencodable does not know, sorted —
// a typo catcher for -doctor. Key names only; values are never returned.
func Unknown() []string {
	var out []string
	for k := range read() {
		if !known[k] {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}
