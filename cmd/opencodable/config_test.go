package main

import (
	"flag"
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

// registerOptions binds the package option vars to fresh values on a clean
// CommandLine-like state; tests never run main so names are free.
func registerOptions(t *testing.T) {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	provider = fs.String("provider", "opencode-go", "")
	plan = fs.String("plan", "go", "")
	sortKey = fs.String("sort", "quality", "")
	pareto = fs.Bool("pareto", false, "")
	paid = fs.Bool("paid", false, "")
	privacy = fs.Bool("privacy", false, "")
	peak = fs.Bool("peak", false, "")
	refresh = fs.Bool("refresh", false, "")
	offline = fs.Bool("offline", false, "")
	probeAll = fs.Bool("probe", false, "")
	probeOne = fs.String("probe-model", "", "")
	probeMax = fs.Float64("probe-max-allowance", 0, "")
	showUse = fs.Bool("usage", false, "")
	now = fs.Bool("now", false, "")
	winEst = fs.Bool("window-estimate", false, "")
	doctor = fs.Bool("doctor", false, "")
	spent = fs.String("spent", "", "")
	vertical = fs.Bool("vertical", false, "")
	jsonOut = fs.Bool("json", false, "")
}

func TestApplyConfig(t *testing.T) {
	writeConf(t, "plan=go-plus\npaid=yes\nprobe-max-allowance=0.5\nsort=name\n")
	registerOptions(t)
	if err := applyConfig(); err != nil {
		t.Fatal(err)
	}
	if *plan != "go-plus" || !*paid || *probeMax != 0.5 || *sortKey != "name" {
		t.Errorf("config not applied: plan=%q paid=%v max=%v sort=%q", *plan, *paid, *probeMax, *sortKey)
	}
}

func TestApplyConfigInvalidBool(t *testing.T) {
	writeConf(t, "paid=maybe\n")
	registerOptions(t)
	if err := applyConfig(); err == nil {
		t.Error("invalid boolean must error")
	}
}
