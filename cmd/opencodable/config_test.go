package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/ColasNahaboo/opencodable/internal/config"
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
func registerOptions(t *testing.T) *flag.FlagSet {
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
	return fs
}

func TestApplyConfig(t *testing.T) {
	writeConf(t, "plan=go-plus\npaid=yes\nprobe-max-allowance=0.5\nsort=name\n")
	fs := registerOptions(t)
	if err := applyConfig(fs); err != nil {
		t.Fatal(err)
	}
	if *plan != "go-plus" || !*paid || *probeMax != 0.5 || *sortKey != "name" {
		t.Errorf("config not applied: plan=%q paid=%v max=%v sort=%q", *plan, *paid, *probeMax, *sortKey)
	}
}

func TestApplyConfigInvalidBool(t *testing.T) {
	writeConf(t, "paid=maybe\n")
	fs := registerOptions(t)
	if err := applyConfig(fs); err == nil {
		t.Error("invalid boolean must error")
	}
}

// TestApplyConfigBoolOverride covers -paid=false: a boolean written as
// paid=yes in the file must be turned off for one run by the command line.
func TestApplyConfigBoolOverride(t *testing.T) {
	writeConf(t, "paid=yes\nsort=name\n")
	fs := registerOptions(t)
	if err := fs.Parse([]string{"-paid=false"}); err != nil {
		t.Fatal(err)
	}
	if err := applyConfig(fs); err != nil {
		t.Fatal(err)
	}
	if *paid {
		t.Error("-paid=false must override paid=yes from the file")
	}
	if *sortKey != "name" {
		t.Errorf("options not set on the command line still come from the file: sort=%q", *sortKey)
	}
}

func TestApplyConfigPathOverride(t *testing.T) {
	writeConf(t, "sort=name\n")
	other := filepath.Join(t.TempDir(), "ci.conf")
	if err := os.WriteFile(other, []byte("sort=price\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = config.SetPath("") })
	if err := config.SetPath(other); err != nil {
		t.Fatal(err)
	}
	fs := registerOptions(t)
	if err := applyConfig(fs); err != nil {
		t.Fatal(err)
	}
	if *sortKey != "price" {
		t.Errorf("sort = %q, want price from the -config file", *sortKey)
	}
}
