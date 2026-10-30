// Package config reads opencodable.conf, a key=value file in the per-user
// config directory (os.UserConfigDir): ~/.config/opencodable/ on Linux,
// ~/Library/Application Support/opencodable/ on macOS, %AppData%\opencodable\
// on Windows.
//
// Key case is meaningful: an all-lowercase key names a command-line option
// (a default the flag overrides); any other casing names an environment-style
// setting such as an API key. Lookup order for keys: environment variable,
// this file, then a legacy cwd dot-file. For options: command line, this
// file, then the flag default.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// envKeys are the environment-style settings opencodable understands.
var envKeys = map[string]bool{
	"OPENCODE_API_KEY": true,
	"AA_API_KEY":       true,
}

// optionNames are the command-line options, registered by the command with
// AddOptions so new flags are accepted automatically.
var optionNames = map[string]bool{}

// file is the config file in use; empty means the per-user default.
var file string

// cached holds the last parsed config file so repeated Option/Value calls
// during startup do not re-read and re-parse the file dozens of times.
var (
	cachedPath  string
	cachedMTime time.Time
	cachedSize  int64
	cachedVals  map[string]string
)

// SetPath reads settings from p instead of the per-user opencodable.conf.
// An empty path restores the default. An explicit path must exist, so a typo
// is reported rather than silently yielding an empty configuration.
func SetPath(p string) error {
	cachedPath = ""
	if p == "" {
		file = ""
		return nil
	}
	fi, err := os.Stat(p)
	if err != nil {
		return err // already names the path
	}
	if fi.IsDir() {
		return fmt.Errorf("%q is a directory", p)
	}
	file = p
	return nil
}

// path returns the config file location, or "" when no config directory is
// available.
func path() string {
	if file != "" {
		return file
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "opencodable", "opencodable.conf")
}

// AddOptions marks flag names as understood config keys (lowercase).
func AddOptions(names ...string) {
	for _, n := range names {
		optionNames[n] = true
	}
}

func read() map[string]string {
	p := path()
	if p == "" {
		return map[string]string{}
	}
	fi, err := os.Stat(p)
	if err != nil {
		return map[string]string{}
	}
	if p == cachedPath && fi.ModTime().Equal(cachedMTime) && fi.Size() == cachedSize && cachedVals != nil {
		return cachedVals
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return map[string]string{}
	}
	vals := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue // malformed lines are ignored; never echo secret-bearing lines
		}
		k, v = strings.TrimSpace(k), cleanValue(v)
		vals[k] = v
	}
	cachedPath, cachedMTime, cachedSize, cachedVals = p, fi.ModTime(), fi.Size(), vals
	return vals
}

// cleanValue trims surrounding whitespace and quotes; for unquoted values it
// also strips trailing inline "# ..." comments preceded by whitespace.
func cleanValue(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
		return v[1 : len(v)-1]
	}
	for i := 0; i+1 < len(v); i++ {
		if (v[i] == ' ' || v[i] == '\t') && v[i+1] == '#' {
			v = strings.TrimSpace(v[:i])
			break
		}
	}
	return strings.Trim(v, `"'`)
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

// Option returns the config-file default for a command-line option name.
func Option(name string) (string, bool) {
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

// ParseBool accepts true/on/1/yes and false/off/0/no (any case).
func ParseBool(v string) (bool, error) {
	switch strings.ToLower(v) {
	case "true", "on", "1", "yes":
		return true, nil
	case "false", "off", "0", "no":
		return false, nil
	}
	return false, fmt.Errorf("invalid boolean %q: want true/on/1/yes or false/off/0/no", v)
}

// Unknown lists config-file keys that opencodable does not know, sorted —
// a typo catcher for -doctor. Key names only; values are never returned.
func Unknown() []string {
	var out []string
	for k := range read() {
		if strings.ToLower(k) == k {
			if !optionNames[k] {
				out = append(out, k)
			}
			continue
		}
		if !envKeys[k] {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}
