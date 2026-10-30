package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ColasNahaboo/opencodable/internal/modelsdev"
)

// writeCacheFile drops raw bytes into the (isolated) cache dir.
func writeCacheFile(t *testing.T, name string, b []byte) {
	t.Helper()
	dir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	d := filepath.Join(dir, "opencodable")
	if err := os.MkdirAll(d, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, name), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEstimateWindowSpendProportional(t *testing.T) {
	kimi := testModel("kimi-k3.1-pro", "Kimi K3.1 Pro", 2, 10, 0.2)
	ds := testModel("deepseek-v4.1-flash", "DeepSeek V4.1 Flash", 0.15, 0.60, 0.003)
	models := []modelsdev.Model{kimi, ds}
	// base5 = 0.2*15 + 0.2*80 = 19; used 50% → $9.5 implied, all on kimi.
	spend := map[string]spendInfo{"kimi-k3.1-pro": {usd: 3.0, reqs: 10, known: true}}
	got, method := estimateWindowSpend(models, spend, false, "go", 0.5, 0, nil)
	if method != "proportional" {
		t.Errorf("method = %q, want proportional", method)
	}
	if got["kimi-k3.1-pro"].five != 9.5 || got["deepseek-v4.1-flash"].five != 0 {
		t.Errorf("proportional split = %v, want kimi 9.5, ds 0", got)
	}
	if got["kimi-k3.1-pro"].week != 0 {
		t.Errorf("unused week must stay zero: %v", got)
	}
}

func TestEstimateWindowSpendLedger(t *testing.T) {
	kimi := testModel("kimi-k3.1-pro", "Kimi K3.1 Pro", 2, 10, 0.2)
	ds := testModel("deepseek-v4.1-flash", "DeepSeek V4.1 Flash", 0.15, 0.60, 0.003)
	models := []modelsdev.Model{kimi, ds}
	spend := map[string]spendInfo{
		"kimi-k3.1-pro":       {usd: 3.0, reqs: 10, known: true},
		"deepseek-v4.1-flash": {usd: 1.0, reqs: 10, known: true},
	}
	// P5 = 9.5, ds attributed 0.5 → remainder 9.0 split 3:1.
	attr := map[string]windowSpend{"deepseek-v4.1-flash": {five: 0.5}}
	got, method := estimateWindowSpend(models, spend, false, "go", 0.5, 0, attr)
	if method != "ledger" {
		t.Errorf("method = %q, want ledger", method)
	}
	if got["kimi-k3.1-pro"].five != 6.75 || got["deepseek-v4.1-flash"].five != 2.75 {
		t.Errorf("ledger split = %v, want kimi 6.75, ds 2.75", got)
	}
}

func TestEstimateWindowSpendGuards(t *testing.T) {
	kimi := testModel("kimi-k3.1-pro", "Kimi K3.1 Pro", 2, 10, 0.2)
	bunny := testModel("space-bunny", "Space Bunny", 1, 2, 0.1) // unknown limit
	models := []modelsdev.Model{kimi, bunny}
	spend := map[string]spendInfo{
		"kimi-k3.1-pro": {usd: 3.0, reqs: 1, known: true},
		"space-bunny":   {usd: 5.0, reqs: 1, known: true},
	}
	got, _ := estimateWindowSpend(models, spend, false, "go", 0.5, 0, nil)
	if got["kimi-k3.1-pro"].five != 1.5 { // base 3, half implied, bunny outside universe
		t.Errorf("kimi = %v, want 1.5", got["kimi-k3.1-pro"])
	}
	if _, ok := got["space-bunny"]; ok {
		t.Error("unknown-limit models must stay out of the estimate")
	}
	// No spend at all: zeros, no division by zero.
	got, method := estimateWindowSpend(models, map[string]spendInfo{}, false, "go", 0.5, 0.5, nil)
	if method != "proportional" || got["kimi-k3.1-pro"] != (windowSpend{}) {
		t.Errorf("empty spend = %v, %q", got, method)
	}
	// Attributed beyond implied: remainder clamps, attribution kept.
	got, _ = estimateWindowSpend(models, spend, false, "go", 0.5, 0,
		map[string]windowSpend{"kimi-k3.1-pro": {five: 99}})
	if got["kimi-k3.1-pro"].five != 99 {
		t.Errorf("over-attributed kimi = %v, want 99", got["kimi-k3.1-pro"])
	}
	// Used fractions clamp to [0,1].
	got, _ = estimateWindowSpend(models, spend, false, "go", 2, -1, nil)
	if got["kimi-k3.1-pro"].five != 3 || got["kimi-k3.1-pro"].week != 0 {
		t.Errorf("clamped = %v, want five 3 week 0", got["kimi-k3.1-pro"])
	}
}

func TestOverlapFrac(t *testing.T) {
	t0 := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		a0, a1 time.Time
		b0, b1 time.Time
		want   float64
	}{
		{"full", t0, t0.Add(2 * time.Hour), t0.Add(-time.Hour), t0.Add(3 * time.Hour), 1},
		{"half", t0, t0.Add(2 * time.Hour), t0.Add(time.Hour), t0.Add(3 * time.Hour), 0.5},
		{"none", t0, t0.Add(time.Hour), t0.Add(2 * time.Hour), t0.Add(3 * time.Hour), 0},
		{"skew", t0.Add(time.Hour), t0, t0, t0.Add(2 * time.Hour), 0},
	} {
		if got := overlapFrac(tc.a0, tc.a1, tc.b0, tc.b1); got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestAttributeLedger(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	reset5h := now.Add(-time.Hour) // window (now-6h, now]
	resetWk := now.Add(-24 * time.Hour)
	ts := func(d time.Duration) string { return now.Add(d).UTC().Format(time.RFC3339) }
	led := spendLedger{
		Prev: &ledgerSnapshot{TS: ts(-2 * time.Hour), Models: map[string]ledgerModel{
			"m1": {USD: 4, Reqs: 10},
			"m2": {USD: 8, Reqs: 10},
		}},
		Probes: []ledgerProbe{
			{TS: ts(-30 * time.Minute), Model: "m1", USD: 0.5}, // both windows
			{TS: ts(-7 * time.Hour), Model: "m1", USD: 1.0},    // week only
			{TS: ts(time.Hour), Model: "m1", USD: 9.0},         // future: skip
		},
	}
	cur := map[string]spendInfo{
		"m1": {usd: 10, reqs: 20, known: true}, // delta +6, fully inside both windows
		"m2": {usd: 5, reqs: 20, known: true},  // delta -3: reset, skip
	}
	got := attributeLedger(led, cur, now, reset5h, resetWk)
	if got["m1"] != (windowSpend{five: 6.5, week: 7.5}) {
		t.Errorf("m1 = %+v, want {6.5 7.5}", got["m1"])
	}
	if _, ok := got["m2"]; ok {
		t.Errorf("reset model must be skipped: %v", got)
	}

	// Clock skew: snapshot ignored, probes still count.
	led.Prev.TS = ts(time.Hour)
	got = attributeLedger(led, cur, now, reset5h, resetWk)
	if got["m1"] != (windowSpend{five: 0.5, week: 1.5}) {
		t.Errorf("skewed m1 = %+v, want probes only {0.5 1.5}", got["m1"])
	}
}

func TestLedgerRoundtripAndPrune(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	led := recordSnapshot(spendLedger{}, map[string]spendInfo{
		"m1": {usd: 2.5, reqs: 7, known: true},
	}, now)
	led.Probes = append(led.Probes, ledgerProbe{TS: "2020-01-01T00:00:00Z", Model: "old", USD: 1})
	led = recordProbeBurn(led, "m1", 0.01, now) // prunes ancient, appends fresh
	saveLedger(led)

	got := loadLedger()
	if got.Prev == nil || got.Prev.Models["m1"] != (ledgerModel{USD: 2.5, Reqs: 7}) {
		t.Errorf("snapshot roundtrip = %+v", got.Prev)
	}
	if len(got.Probes) != 1 || got.Probes[0].Model != "m1" {
		t.Errorf("probes must prune ancient entries: %+v", got.Probes)
	}

	// Corrupt file reads as empty, never errors.
	writeCacheFile(t, "spend-ledger.json", []byte("nope{"))
	if got := loadLedger(); got.Prev != nil || len(got.Probes) != 0 {
		t.Errorf("corrupt ledger must read empty: %+v", got)
	}
}

func TestLedgerUsesRegimeIndependentBaseUSD(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	reset5h := now.Add(time.Hour)
	resetWk := now.Add(24 * time.Hour)
	// First run without -peak: usd=10, baseUSD=10.
	led := recordSnapshot(spendLedger{}, map[string]spendInfo{
		"deepseek-v4.1-flash": {usd: 10, baseUSD: 10, reqs: 100, known: true},
	}, now.Add(-30*time.Minute))
	// Second run with -peak on the exact same console tokens: usd=20 (peak), baseUSD=10 (off-peak).
	cur := map[string]spendInfo{
		"deepseek-v4.1-flash": {usd: 20, baseUSD: 10, reqs: 100, known: true},
	}
	got := attributeLedger(led, cur, now, reset5h, resetWk)
	if got["deepseek-v4.1-flash"] != (windowSpend{}) {
		t.Errorf("toggling -peak must not create a phantom ledger delta, got %+v", got["deepseek-v4.1-flash"])
	}
}
