package table

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestFullTitlesOrder(t *testing.T) {
	full := FullTitles()
	main := HeaderCells()
	if !slices.Equal(full[:len(main)], main) {
		t.Error("full view must start with every main column in main order")
	}
	nowOnly := []string{"burn_req", "spent_mo", "reqs_5h", "reqs_wk", "reqs_mo", "usable", "limit"}
	if !slices.Equal(full[len(main):], nowOnly) {
		t.Errorf("full view tail = %v, want %v", full[len(main):], nowOnly)
	}
	seen := map[string]bool{}
	for _, title := range full {
		k := normTitle(title)
		if seen[k] {
			t.Errorf("duplicate column %q in full titles", title)
		}
		seen[k] = true
	}
}

func TestSelectTitles(t *testing.T) {
	view := HeaderCells()
	// nil selects all
	all, err := SelectTitles(view, nil, nil)
	if err != nil || !slices.Equal(all, view) {
		t.Errorf("nil selection = %v, %v; want all", all, err)
	}
	// include-only keeps view order, not flag order
	sub, err := SelectTitles(view, []string{"swe_b", "cost_task"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sub, []string{"#name", "cost_task", "swe_b"}) {
		t.Errorf("include-only = %v", sub)
	}
	// exclude-only drops names
	ex, err := SelectTitles(view, nil, []string{"src", "ctx_k", "out_k"})
	if err != nil {
		t.Fatal(err)
	}
	for _, drop := range []string{"src", "ctx_k", "out_k"} {
		if slices.Contains(ex, drop) {
			t.Errorf("exclude-only kept %q", drop)
		}
	}
	if !slices.Contains(ex, "#name") {
		t.Error("name must survive -excols")
	}
	// intersect-then-subtract
	both, err := SelectTitles(view, []string{"cost_task", "src"}, []string{"src"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(both, []string{"#name", "cost_task"}) {
		t.Errorf("cols minus excols = %v", both)
	}
	// name in -excols is ignored
	keep, err := SelectTitles(view, nil, []string{"name"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(keep, "#name") {
		t.Error("-excols name must be ignored")
	}
	// case-insensitive
	upper, err := SelectTitles(view, []string{"COST_TASK"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(upper, []string{"#name", "cost_task"}) {
		t.Errorf("case-insensitive = %v", upper)
	}
}

func TestSelectTitlesUnknownAndCrossView(t *testing.T) {
	if _, err := SelectTitles(HeaderCells(), []string{"bogus"}, nil); err == nil {
		t.Error("-cols bogus must fail")
	}
	if _, err := SelectTitles(HeaderCells(), nil, []string{"bogus"}); err == nil {
		t.Error("-excols bogus must fail")
	}
	// burn_req lives only in -now: valid everywhere, ignored in main.
	got, err := SelectTitles(HeaderCells(), []string{"burn_req"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"#name"}) {
		t.Errorf("cross-view include in main = %v, want [#name]", got)
	}
	if _, err := SelectTitles(NowTitles(), []string{"burn_req"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestProjectCells(t *testing.T) {
	full := []string{"#name", "a", "b"}
	row := []string{"n", "1", "2"}
	if got := ProjectCells(full, row, []string{"b", "#name"}); !slices.Equal(got, []string{"2", "n"}) {
		t.Errorf("project = %v", got)
	}
}

func TestFilterLegend(t *testing.T) {
	lines := []string{
		"# opencodable — prose header",
		"# name        model display name",
		"# cost_task   estimated USD per request",
	}
	got := FilterLegend(lines, []string{"#name", "cost_task"})
	if !slices.Equal(got, lines) {
		t.Errorf("full selection must keep everything: %v", got)
	}
	got = FilterLegend(lines, []string{"#name"})
	if !slices.Equal(got, lines[:2]) {
		t.Errorf("filtered legend = %v", got)
	}
}

func TestFullCells(t *testing.T) {
	base := Row{Name: "m", ID: "m", CostTask: 0.01}
	// known paid model: everything filled
	r := FullRow{Row: base, Burn: 0.02, SpentMo: 1.5, SpentKnown: true,
		Reqs5h: 10, ReqsWk: 20, ReqsMo: 30, Usable: 10, Limit: "5h", Known: true}
	c := FullCells(r, "-")
	if len(c) != len(FullTitles()) {
		t.Fatalf("full cells = %d fields, want %d titles", len(c), len(FullTitles()))
	}
	tail := c[len(HeaderCells()):]
	if !slices.Equal(tail, []string{"$0.02", "$1.5", "10", "20", "30", "10", "5h"}) {
		t.Errorf("known tail = %v", tail)
	}
	// free model: no counts, unlimited limit
	free := FullRow{Row: Row{Name: "f", Unlimited: true}, Limit: "unlimited"}
	tail = FullCells(free, "-")[len(HeaderCells()):]
	if !slices.Equal(tail, []string{"-", "-", "-", "-", "-", "-", "unlimited"}) {
		t.Errorf("free tail = %v", tail)
	}
	// unknown model: unknown spend renders as missing, unknown limit
	unk := FullRow{Row: base, Limit: "unknown"}
	tail = FullCells(unk, "")[len(HeaderCells()):]
	if !slices.Equal(tail, []string{"", "", "", "", "", "", "unknown"}) {
		t.Errorf("unknown tail = %v", tail)
	}
	// known-zero spend renders $0, not blank
	zero := FullRow{Row: base, Burn: 0.01, SpentKnown: true, Known: true,
		Reqs5h: 1, ReqsWk: 1, ReqsMo: 1, Usable: 1, Limit: "month"}
	if got := FullCells(zero, "")[len(HeaderCells())+1]; got != "$0" {
		t.Errorf("known-zero spent = %q, want $0", got)
	}
}

func TestSortFullUsable(t *testing.T) {
	rows := []FullRow{
		{Row: Row{ID: "b", Name: "b", CostTask: 0.01}, Usable: 5, Limit: "5h", Known: true},
		{Row: Row{ID: "a", Name: "a", CostTask: 0.02}, Usable: 50, Limit: "week", Known: true},
		{Row: Row{ID: "free", Name: "free", Unlimited: true}, Limit: "unlimited"},
		{Row: Row{ID: "unk", Name: "unk", CostTask: 0.01}, Limit: "unknown"},
	}
	SortFull(rows, "usable")
	got := []string{rows[0].Row.ID, rows[1].Row.ID, rows[2].Row.ID, rows[3].Row.ID}
	if !slices.Equal(got, []string{"a", "b", "free", "unk"}) {
		t.Errorf("usable sort = %v", got)
	}
	SortFull(rows, "price")
	// free models cost 0 and sort first, like -sort price in the main view
	if got := []string{rows[0].Row.ID, rows[1].Row.ID}; !slices.Equal(got, []string{"free", "b"}) {
		t.Errorf("price sort first two = %v, want [free b]", got)
	}
	SortFull(rows, "bogus")
	if rows[0].Row.ID != "a" {
		t.Errorf("fallback sort first = %s, want a", rows[0].Row.ID)
	}
}

func TestFullJSONKeysMatchTitles(t *testing.T) {
	rows := []FullRow{{Row: Row{Name: "m1", ID: "m1", Src: "aa:high"}, Limit: "month", Known: true}}
	b, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var recs []map[string]any
	if err := json.Unmarshal(b, &recs); err != nil {
		t.Fatal(err)
	}
	for _, title := range FullTitles() {
		key := strings.TrimPrefix(title, "#")
		if _, ok := recs[0][key]; !ok {
			t.Errorf("full JSON missing key %q", key)
		}
	}
}

func TestWriteTSVSelected(t *testing.T) {
	rows := []Row{{Name: "m1", PriceOut1M: 10, CostTask: 0.02}}
	sel := []string{"#name", "cost_task"}
	var b strings.Builder
	if err := WriteTSV(&b, rows, sel, false, false, false); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if lines[0] != "#name\tcost_task" {
		t.Errorf("selected header = %q", lines[0])
	}
	if lines[1] != "m1\t$0.02" {
		t.Errorf("selected row = %q", lines[1])
	}
}

func TestMainTitlesEndWithArena(t *testing.T) {
	main := HeaderCells()
	tail := main[len(main)-2:]
	if tail[0] != "elo" || tail[1] != "btr" {
		t.Errorf("main titles must end with elo, btr: %v", tail)
	}
	// -full inherits both through the main block, in order.
	full := FullTitles()
	if !slices.Equal(full[len(main)-2:len(main)], []string{"elo", "btr"}) {
		t.Errorf("full titles must carry elo, btr in the main block")
	}
}

func TestWriteTSVNoHead(t *testing.T) {
	rows := []Row{{Name: "m1", PriceOut1M: 10, CostTask: 0.02}}
	sel := []string{"#name", "cost_task"}
	var b strings.Builder
	if err := WriteTSV(&b, rows, sel, false, false, true); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimRight(b.String(), "\n"); got != "m1\t$0.02" {
		t.Errorf("headless output = %q, want data row only", got)
	}
	// The legend still prints: -nohead drops the header, not the docs.
	var l strings.Builder
	if err := WriteTSV(&l, rows, sel, false, true, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(l.String(), "# cost_task") {
		t.Errorf("legend must survive -nohead:\n%s", l.String())
	}
	if strings.Contains(l.String(), "#name\tcost_task") {
		t.Errorf("header must be gone:\n%s", l.String())
	}
}

func TestWriteFullTSVNoHead(t *testing.T) {
	rows := []FullRow{{Row: Row{Name: "m1", ID: "m1"}, Limit: "month", Known: true, Usable: 7}}
	sel := []string{"#name", "usable"}
	var b strings.Builder
	if err := WriteFullTSV(&b, rows, sel, false, false, nil, true); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimRight(b.String(), "\n"); got != "m1\t7" {
		t.Errorf("headless full output = %q", got)
	}
}
