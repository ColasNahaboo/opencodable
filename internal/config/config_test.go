package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConf(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "opencodable"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencodable", "opencodable.conf"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestParse(t *testing.T) {
	writeConf(t, "# comment\n\n OPENCODE_API_KEY = sk-abc123 \nAA_API_KEY=\"aa-q\"\nNOEQUALS\nEMPTY=\nCRLF=1\r\n")
	for k, want := range map[string]string{"OPENCODE_API_KEY": "sk-abc123", "AA_API_KEY": "aa-q", "CRLF": "1"} {
		if got, ok := Value(k); !ok || got != want {
			t.Errorf("Value(%s) = %q, %v; want %q", k, got, ok, want)
		}
	}
	if _, ok := Value("NOEQUALS"); ok {
		t.Error("line without = must not produce a value")
	}
	if v, ok := Value("EMPTY"); !ok || v != "" {
		t.Errorf("Value(EMPTY) = %q, %v; want empty string, true", v, ok)
	}
	if _, ok := Value("MISSING"); ok {
		t.Error("unknown key found")
	}
}

func TestEnvBeatsFile(t *testing.T) {
	writeConf(t, "OPENCODE_API_KEY=file-key\nAA_API_KEY=file-aa\n")
	t.Setenv("OPENCODE_API_KEY", "env-key")
	if v, _ := Value("OPENCODE_API_KEY"); v != "env-key" {
		t.Errorf("Value = %q, want env-key", v)
	}
	if s := Source("OPENCODE_API_KEY"); s != "env" {
		t.Errorf("Source = %q, want env", s)
	}
	if s := Source("AA_API_KEY"); s != "config" {
		t.Errorf("Source = %q, want config", s)
	}
}

func TestNoFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, ok := Value("OPENCODE_API_KEY"); ok {
		t.Error("value without config file")
	}
	if s := Source("OPENCODE_API_KEY"); s != "" {
		t.Errorf("Source = %q, want empty", s)
	}
	if u := Unknown(); len(u) != 0 {
		t.Errorf("Unknown = %v, want none", u)
	}
}

func TestUnknownKeys(t *testing.T) {
	AddOptions("plan", "paid")
	writeConf(t, "OPENCODE_API_KEY=a\nOPENCODE_KEY=typo\nAA_API_KEY=b\nplan=go-plus\nbogus=1\n")
	u := Unknown()
	if len(u) != 2 || u[0] != "OPENCODE_KEY" || u[1] != "bogus" {
		t.Errorf("Unknown = %v, want [OPENCODE_KEY bogus]", u)
	}
}

func TestOption(t *testing.T) {
	writeConf(t, "plan=go-plus\npaid=yes\n")
	if v, ok := Option("plan"); !ok || v != "go-plus" {
		t.Errorf("Option(plan) = %q, %v", v, ok)
	}
	if _, ok := Option("sort"); ok {
		t.Error("Option(sort) found without a config entry")
	}
}

// TestSetPath covers -config: an explicit file replaces the per-user one for
// both kinds of keys, and a missing path is an error rather than an empty
// configuration.
func TestSetPath(t *testing.T) {
	writeConf(t, "plan=go-plus\n")
	other := filepath.Join(t.TempDir(), "ci.conf")
	if err := os.WriteFile(other, []byte("plan=go\nAA_API_KEY=ci-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = SetPath("") })
	if err := SetPath(other); err != nil {
		t.Fatal(err)
	}
	if v, ok := Option("plan"); !ok || v != "go" {
		t.Errorf("Option(plan) = %q, %v; want go from %s", v, ok, other)
	}
	if v, ok := Value("AA_API_KEY"); !ok || v != "ci-key" {
		t.Errorf("Value(AA_API_KEY) = %q, %v; want ci-key", v, ok)
	}
	if err := SetPath(filepath.Join(t.TempDir(), "missing.conf")); err == nil {
		t.Error("SetPath(missing) must fail")
	}
	if err := SetPath(t.TempDir()); err == nil {
		t.Error("SetPath(directory) must fail")
	}
}

func TestParseBool(t *testing.T) {
	for _, s := range []string{"true", "on", "1", "yes", "YES", "On"} {
		if v, err := ParseBool(s); err != nil || !v {
			t.Errorf("ParseBool(%q) = %v, %v; want true", s, v, err)
		}
	}
	for _, s := range []string{"false", "off", "0", "no", "NO", "Off"} {
		if v, err := ParseBool(s); err != nil || v {
			t.Errorf("ParseBool(%q) = %v, %v; want false", s, v, err)
		}
	}
	if _, err := ParseBool("maybe"); err == nil {
		t.Error("ParseBool(maybe) must fail")
	}
}
