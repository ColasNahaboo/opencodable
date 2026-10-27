package main

import (
	"math"
	"testing"

	"opencodable/internal/cost"
	"opencodable/internal/modelsdev"
)

func testModel(id, name string, in, out, cached float64) modelsdev.Model {
	return modelsdev.Model{
		ID:   id,
		Name: name,
		Cost: modelsdev.Cost{Input: in, Output: out, CacheRead: cached},
	}
}

func TestBuildNowRowsUsesPerModelLimits(t *testing.T) {
	kimi := testModel("kimi-k3.1-pro", "Kimi K3.1 Pro", 2, 10, 0.2)
	deepseek := testModel("deepseek-v4.1-flash", "DeepSeek V4.1 Flash", 0.15, 0.60, 0.003)
	rows := buildNowRows(
		[]modelsdev.Model{kimi, deepseek},
		map[string]spendInfo{}, true, "go", false, false, false, nil,
	)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	byID := map[string]nowRow{}
	for _, r := range rows {
		byID[r.id] = r
	}

	kt := cost.PerRequest(kimi, false)
	k := byID["kimi-k3.1-pro"]
	if k.burn != kt {
		t.Errorf("burn = %v, want official cost_task %v", k.burn, kt)
	}
	wantFive := int(0.2*15/kt + 1e-9)
	wantWeek := int(0.5*15/kt + 1e-9)
	wantMonth := int(15/kt + 1e-9)
	if k.five != wantFive || k.week != wantWeek || k.month != wantMonth {
		t.Errorf("Kimi windows = %d/%d/%d, want %d/%d/%d",
			k.five, k.week, k.month, wantFive, wantWeek, wantMonth)
	}
	if k.binding != "5h" {
		t.Errorf("Kimi binding = %s, want 5h", k.binding)
	}

	// Go Plus doubles the monthly limit: 20% of $60 / $0.0204 = 588.
	plus := buildNowRows([]modelsdev.Model{kimi}, nil, false, "go-plus", false, false, false, nil)
	if plus[0].five != 588 {
		t.Errorf("Go Plus Kimi 5h = %d, want 588", plus[0].five)
	}

	dt := cost.PerRequest(deepseek, false)
	d := byID["deepseek-v4.1-flash"]
	if d.burn != dt {
		t.Errorf("DeepSeek burn = %v, want %v", d.burn, dt)
	}
	if d.five != int(0.2*80/dt+1e-9) {
		t.Errorf("DeepSeek 5h = %d, want %d", d.five, int(0.2*80/dt+1e-9))
	}
}

func TestBuildNowRowsUsesMeasuredBurn(t *testing.T) {
	deepseek := testModel("deepseek-v4.1-flash", "DeepSeek V4.1 Flash", 0.15, 0.60, 0.003)
	spend := map[string]spendInfo{
		"deepseek-v4.1-flash": {usd: 0.50, reqs: 100, known: true},
	}
	rows := buildNowRows([]modelsdev.Model{deepseek}, spend, true, "go", false, false, false, nil)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	r := rows[0]
	if math.Abs(r.burn-0.005) > 1e-12 {
		t.Errorf("measured burn = %v, want 0.005", r.burn)
	}
	wantUsable := int(0.2 * 80 / 0.005)
	if r.usable != wantUsable {
		t.Errorf("usable = %d, want 5h cap %d", r.usable, wantUsable)
	}
}

func TestBuildNowRowsFallsBackToOfficialBurn(t *testing.T) {
	deepseek := testModel("deepseek-v4.1-flash", "DeepSeek V4.1 Flash", 0.15, 0.60, 0.003)
	spend := map[string]spendInfo{
		"deepseek-v4.1-flash": {usd: 0.50, reqs: 10, known: true}, // too few requests
	}
	rows := buildNowRows([]modelsdev.Model{deepseek}, spend, true, "go", false, false, false, nil)
	want := cost.PerRequest(deepseek, false)
	if math.Abs(rows[0].burn-want) > 1e-12 {
		t.Errorf("burn = %v, want official %v", rows[0].burn, want)
	}
}

func TestBuildNowRowsUnknownAndUnlimited(t *testing.T) {
	unknown := testModel("space-bunny", "Space Bunny", 1, 2, 0.1)
	free := testModel("beam-free", "Beam Free", 0, 0, 0)
	freeUnknown := testModel("cat-jam-free", "Cat Jam Free", 0, 0, 0)
	rows := buildNowRows(
		[]modelsdev.Model{unknown, free, freeUnknown},
		map[string]spendInfo{}, false, "go", false, false, false, nil,
	)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	byID := map[string]nowRow{}
	for _, r := range rows {
		byID[r.id] = r
	}
	if u := byID["space-bunny"]; u.known || u.binding != "unknown" {
		t.Errorf("unknown model = known %v, limit %q; want unknown", u.known, u.binding)
	}
	for _, id := range []string{"beam-free", "cat-jam-free"} {
		if f := byID[id]; !f.unlimited || f.binding != "unlimited" {
			t.Errorf("%s = unlimited %v, limit %q; want unlimited", id, f.unlimited, f.binding)
		}
	}
}

func TestBuildNowRowsFilters(t *testing.T) {
	muse := testModel("muse-spark-1.3-contributor", "Muse Spark", 0.10, 0.20, 0.002)
	free := testModel("beam-free", "Beam Free", 0, 0, 0)
	if rows := buildNowRows([]modelsdev.Model{muse}, nil, false, "go", false, false, true, nil); len(rows) != 0 {
		t.Errorf("-privacy kept %d contributor rows", len(rows))
	}
	if rows := buildNowRows([]modelsdev.Model{free}, nil, false, "go", false, true, false, nil); len(rows) != 0 {
		t.Errorf("-paid kept %d free rows", len(rows))
	}
}
