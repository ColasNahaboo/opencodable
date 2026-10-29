package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/ColasNahaboo/opencodable/internal/config"
	"github.com/ColasNahaboo/opencodable/internal/table"
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
	exclude = fs.String("exclude", "", "")
	maxCost = fs.Float64("max-cost", 0, "")
	minSpeed = fs.Float64("min-speed", 0, "")
	minQuality = fs.String("min-quality", "", "")
	missing = fs.String("missing", "keep", "")
	colsFlag = nil
	excolsFlag = nil
	fs.Var(&colsFlag, "cols", "")
	fs.Var(&excolsFlag, "excols", "")
	peak = fs.Bool("peak", false, "")
	refresh = fs.Bool("refresh", false, "")
	offline = fs.Bool("offline", false, "")
	probeAll = fs.Bool("probe", false, "")
	probeOne = fs.String("probe-model", "", "")
	probeMax = fs.Float64("probe-max-allowance", 0, "")
	showUse = fs.Bool("usage", false, "")
	now = fs.Bool("now", false, "")
	fullView = fs.Bool("full", false, "")
	mainView = fs.Bool("main", false, "")
	winEst = fs.Bool("window-estimate", false, "")
	doctor = fs.Bool("doctor", false, "")
	spent = fs.String("spent", "", "")
	vertical = fs.Bool("vertical", false, "")
	jsonOut = fs.Bool("json", false, "")
	nohead = fs.Bool("nohead", false, "")
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

func TestBuildFilterFromConfig(t *testing.T) {
	writeConf(t, "exclude=grok*,kimi-*\nmax-cost=0.01\nmin-speed=60\nmin-quality=aa_int=40,aa_code=35\nmissing=drop\n")
	fs := registerOptions(t)
	if err := applyConfig(fs); err != nil {
		t.Fatal(err)
	}
	flt, err := buildFilter(fs)
	if err != nil {
		t.Fatal(err)
	}
	if len(flt.Exclude) != 2 || flt.Exclude[0] != "grok*" || flt.Exclude[1] != "kimi-*" {
		t.Errorf("Exclude = %v", flt.Exclude)
	}
	if flt.MaxCost == nil || *flt.MaxCost != 0.01 {
		t.Errorf("MaxCost = %v, want 0.01", flt.MaxCost)
	}
	if flt.MinSpeed == nil || *flt.MinSpeed != 60 {
		t.Errorf("MinSpeed = %v, want 60", flt.MinSpeed)
	}
	if flt.MinQuality["aa_int"] != 40 || flt.MinQuality["aa_code"] != 35 {
		t.Errorf("MinQuality = %v", flt.MinQuality)
	}
	if !flt.DropMissing {
		t.Error("missing=drop must set DropMissing")
	}
}

func TestBuildFilterDefaults(t *testing.T) {
	writeConf(t, "")
	fs := registerOptions(t)
	if err := applyConfig(fs); err != nil {
		t.Fatal(err)
	}
	flt, err := buildFilter(fs)
	if err != nil {
		t.Fatal(err)
	}
	if flt.MaxCost != nil || flt.MinSpeed != nil {
		t.Error("numeric bounds must start unset")
	}
	if flt.DropMissing {
		t.Error("-missing must default to keep")
	}
	if len(flt.Exclude) != 0 || len(flt.MinQuality) != 0 {
		t.Errorf("filters must start empty: %+v", flt)
	}
}

// TestBuildFilterExplicitZero guards the pointer bounds: -max-cost=0 is a
// real bound (free models only), not "unset".
func TestBuildFilterExplicitZero(t *testing.T) {
	writeConf(t, "")
	fs := registerOptions(t)
	if err := fs.Parse([]string{"-max-cost=0"}); err != nil {
		t.Fatal(err)
	}
	if err := applyConfig(fs); err != nil {
		t.Fatal(err)
	}
	flt, err := buildFilter(fs)
	if err != nil {
		t.Fatal(err)
	}
	if flt.MaxCost == nil || *flt.MaxCost != 0 {
		t.Errorf("MaxCost = %v, want a 0 bound", flt.MaxCost)
	}
}

func TestBuildFilterErrors(t *testing.T) {
	for _, arg := range []string{
		"-min-quality=bogus=1",
		"-min-quality=aa_int=abc",
		"-missing=maybe",
	} {
		writeConf(t, "")
		fs := registerOptions(t)
		if err := fs.Parse([]string{arg}); err != nil {
			t.Fatalf("%s: %v", arg, err)
		}
		if err := applyConfig(fs); err != nil {
			t.Fatalf("%s: %v", arg, err)
		}
		if _, err := buildFilter(fs); err == nil {
			t.Errorf("%s must fail", arg)
		}
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

func TestBuildColumnsUnionConfigAndCLI(t *testing.T) {
	writeConf(t, "cols=cost_task,aa_int\nexcols=src\n")
	fs := registerOptions(t)
	if err := fs.Parse([]string{"-cols", "p_tok_s", "-excols", "ctx_k"}); err != nil {
		t.Fatal(err)
	}
	if err := applyConfig(fs); err != nil {
		t.Fatal(err)
	}
	sel, err := buildColumns()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, n := range append(append([]string{}, sel.includes...), sel.excludes...) {
		got[n] = true
	}
	for _, want := range []string{"cost_task", "aa_int", "p_tok_s", "src", "ctx_k"} {
		if !got[want] {
			t.Errorf("union missing %q: %+v", want, sel)
		}
	}
}

func TestBuildColumnsUnknown(t *testing.T) {
	for _, args := range [][]string{
		{"-cols", "bogus"},
		{"-excols", "bogus"},
	} {
		writeConf(t, "")
		fs := registerOptions(t)
		if err := fs.Parse(args); err != nil {
			t.Fatal(err)
		}
		if err := applyConfig(fs); err != nil {
			t.Fatal(err)
		}
		if _, err := buildColumns(); err == nil {
			t.Errorf("%v must fail", args)
		}
	}
}

// TestBuildColumnsCrossViewOK guards the global-preset rule: a name valid in
// another view validates fine, even when the current view lacks it.
func TestBuildColumnsCrossViewOK(t *testing.T) {
	writeConf(t, "cols=burn_req\n") // -now-only column
	fs := registerOptions(t)
	if err := applyConfig(fs); err != nil {
		t.Fatal(err)
	}
	sel, err := buildColumns()
	if err != nil {
		t.Fatal(err)
	}
	got, err := table.SelectTitles(table.HeaderCells(), sel.includes, sel.excludes)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "#name" {
		t.Errorf("main view with only cross-view cols = %v, want [#name]", got)
	}
}

// TestBuildFilterNegativeScale guards the no-range-check rule: bounds are
// parsed, never validated, so a negative floor is accepted (whatever the
// metric's scale).
func TestBuildFilterNegativeScale(t *testing.T) {
	writeConf(t, "")
	fs := registerOptions(t)
	if err := fs.Parse([]string{"-min-quality=btr=-0.1"}); err != nil {
		t.Fatal(err)
	}
	if err := applyConfig(fs); err != nil {
		t.Fatal(err)
	}
	flt, err := buildFilter(fs)
	if err != nil {
		t.Fatal(err)
	}
	if flt.MinQuality["btr"] != -0.1 {
		t.Errorf("MinQuality = %v", flt.MinQuality)
	}
}
