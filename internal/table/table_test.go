package table

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/ColasNahaboo/opencodable/internal/cost"
	"github.com/ColasNahaboo/opencodable/internal/modelsdev"
)

func TestMarkPareto(t *testing.T) {
	rows := []Row{
		{Name: "cheap-fast", CostTask: 0.001, TokPerSec: 100, AAIntel: 40},
		{Name: "cheap-slow", CostTask: 0.001, TokPerSec: 10, AAIntel: 40},
		{Name: "pricey-best", CostTask: 0.02, TokPerSec: 90, AAIntel: 55},
		{Name: "dominated", CostTask: 0.03, TokPerSec: 50, AAIntel: 30},
		{Name: "nodata", CostTask: 0},
	}
	markPareto(rows)
	want := map[string]bool{
		"cheap-fast": true, "pricey-best": true,
		"cheap-slow": false, "dominated": false, "nodata": false,
	}
	for _, r := range rows {
		if r.Pareto != want[r.Name] {
			t.Errorf("%s: pareto=%v, want %v", r.Name, r.Pareto, want[r.Name])
		}
	}
}

func TestSort(t *testing.T) {
	rows := []Row{
		{Name: "b", TokPerSec: 10, AAIntel: 20, CostTask: 0.5, RQuota: 10},
		{Name: "a", TokPerSec: 50, AAIntel: 10, CostTask: 0.1, RQuota: 100},
		{Name: "c", TokPerSec: 20, AAIntel: 30, CostTask: 1, RQuota: 1},
	}
	Sort(rows, "speed")
	if rows[0].Name != "a" || rows[2].Name != "b" {
		t.Errorf("speed sort wrong: %v %v %v", rows[0].Name, rows[1].Name, rows[2].Name)
	}
	Sort(rows, "quality")
	if rows[0].Name != "c" {
		t.Errorf("quality sort wrong: first=%s", rows[0].Name)
	}
	Sort(rows, "price")
	if rows[0].Name != "a" {
		t.Errorf("price sort wrong: first=%s", rows[0].Name)
	}
	Sort(rows, "reqs")
	if rows[0].Name != "a" || rows[2].Name != "c" {
		t.Errorf("reqs sort wrong: %v %v %v", rows[0].Name, rows[1].Name, rows[2].Name)
	}
	Sort(rows, "bogus") // falls back to name
	if rows[0].Name != "a" {
		t.Errorf("fallback sort wrong: first=%s", rows[0].Name)
	}
}

func TestNumOr(t *testing.T) {
	cases := []struct {
		v       float64
		money   bool
		missing string
		want    string
	}{
		{0, true, "-", "-"},
		{0, true, "", ""},
		{10, true, "-", "$10"},
		{0.0027898000000000003, true, "", "$0.00279"},
		{0.00026, true, "", "$0.00026"},
		{1578.9, false, "-", "1578.9"},
	}
	for _, c := range cases {
		if got := numOr(c.v, c.money, c.missing); got != c.want {
			t.Errorf("numOr(%v,%v,%q)=%q want %q", c.v, c.money, c.missing, got, c.want)
		}
	}
}

func TestWriteTSVMachineMode(t *testing.T) {
	rows := []Row{
		{Name: "m1", PriceOut1M: 10, CostTask: 0.02, TokPerSec: 123.45, Pareto: true, Src: "aa:high"},
		{Name: "m2"},
	}
	var b strings.Builder
	if err := WriteTSV(&b, rows, nil, false, false, false); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	nCol := len(HeaderCells())
	headJoin := strings.Join(HeaderCells(), "\t")
	var row1, row2 []string
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "# "):
			t.Errorf("legend must be opt-in (-l): %q", l)
		case l == headJoin:
			// header
		default: // data row
			if got := len(strings.Split(l, "\t")); got != nCol {
				t.Errorf("line %q has %d fields, want %d", l, got, nCol)
			}
			if strings.Contains(l, "  ") {
				t.Errorf("machine mode must not pad: %q", l)
			}
			f := strings.Split(l, "\t")
			if f[0] == "m1" {
				row1 = f
			}
			if f[0] == "m2" {
				row2 = f
			}
		}
	}
	srcIdx := slices.Index(HeaderCells(), "src")
	if row1[srcIdx] != "aa:high" {
		t.Errorf("source cell = %q, want aa:high", row1[srcIdx])
	}
	for i, v := range row2 {
		if i == 0 {
			continue
		}
		if v != "" {
			t.Errorf("m2 empty-model field %q = %q, want empty", HeaderCells()[i], v)
		}
	}
}

func TestWriteVertical(t *testing.T) {
	rows := []Row{
		{Name: "m1", PriceOut1M: 10, Pareto: true, Src: "aa:high"},
		{Name: "m2", AACoding: 70},
	}
	var b strings.Builder
	if err := WriteVertical(&b, rows, nil, false); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	// every record starts with "name: ", fields are key: value, empties skipped
	if !strings.Contains(out, "name: m1\ncost_tok: $10\npareto: yes\nsrc: aa:high") {
		t.Errorf("m1 block wrong:\n%s", out)
	}
	if strings.Contains(out, "cost_task: ") {
		t.Error("empty fields must be skipped in vertical mode")
	}
	records := strings.Split(strings.TrimRight(out, "\n"), "\n\n")
	if len(records) != 2 {
		t.Errorf("want 2 records separated by a blank line, got %d:\n%s", len(records), out)
	}
	if !strings.HasPrefix(records[1], "name: m2\n") {
		t.Errorf("second record wrong: %q", records[1])
	}
}

func TestJSONKeysMatchTitles(t *testing.T) {
	rows := []Row{{Name: "m1", PriceOut1M: 10, Pareto: true, Src: "aa:high"}}
	b, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var recs []map[string]any
	if err := json.Unmarshal(b, &recs); err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	// every TSV column title (sans #) must exist as a JSON key
	for _, title := range HeaderCells() {
		key := strings.TrimPrefix(title, "#")
		if _, ok := recs[0][key]; !ok {
			t.Errorf("JSON missing key %q for column %q", key, title)
		}
	}
	if recs[0]["src"] != "aa:high" {
		t.Errorf("src = %v, want aa:high", recs[0]["src"])
	}
}

func TestWriteTSVTTYMode(t *testing.T) {
	rows := []Row{{Name: "m1", PriceOut1M: 10}}
	var b strings.Builder
	if err := WriteTSV(&b, rows, nil, true, false, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "# ") {
		t.Error("legend must be opt-in (-l)")
	}
	if !strings.Contains(b.String(), "-") {
		t.Error("tty mode must print - for missing values")
	}
	// header and data aligned by tabwriter: at least one padded gap
	for _, l := range strings.Split(b.String(), "\n") {
		if strings.HasPrefix(l, "m1") && !strings.Contains(l, "  ") {
			t.Errorf("tty mode must pad columns: %q", l)
		}
	}
}

func TestLegendOptIn(t *testing.T) {
	rows := []Row{{Name: "m1", PriceOut1M: 10}}
	for _, tty := range []bool{false, true} {
		var b strings.Builder
		if err := WriteTSV(&b, rows, nil, tty, true, false); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(b.String(), "# name        model display name") {
			t.Errorf("tty=%v: -l must print the legend", tty)
		}
	}
	var v strings.Builder
	if err := WriteVertical(&v, rows, nil, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v.String(), "# name        model display name") {
		t.Error("vertical -l must print the legend")
	}
}

func TestQualFallsBackToCoding(t *testing.T) {
	r := Row{AACoding: 70}
	if Quality(r) != 70 {
		t.Errorf("qual fallback = %v", Quality(r))
	}
	r2 := Row{AAIntel: 55, AACoding: 70}
	if Quality(r2) != 55 {
		t.Errorf("qual prefer intel = %v", Quality(r2))
	}
}

func TestSortDeterministicTies(t *testing.T) {
	rows := []Row{
		{ID: "c", Name: "same", AAIntel: 10},
		{ID: "a", Name: "same", AAIntel: 10},
		{ID: "b", Name: "same", AAIntel: 10},
	}
	Sort(rows, "quality")
	got := []string{rows[0].ID, rows[1].ID, rows[2].ID}
	want := []string{"a", "b", "c"}
	if !slices.Equal(got, want) {
		t.Errorf("tie order = %v, want %v", got, want)
	}
}

func TestSortReqsUnlimitedFirst(t *testing.T) {
	rows := []Row{
		{ID: "paid", RQuota: 10},
		{ID: "free", Unlimited: true, RQuota: 0},
	}
	Sort(rows, "reqs")
	if rows[0].ID != "free" {
		t.Errorf("unlimited model must sort first on reqs, got %s", rows[0].ID)
	}
}

func TestMarkParetoAfterFilter(t *testing.T) {
	rows := []Row{
		{ID: "deepseek-v4.1-flash", Name: "DeepSeek", CostTask: 0.00046, ProbeTokS: 215.6, AAIntel: 45.1},
		{ID: "muse-spark-1.3-contributor", Name: "Muse", CostTask: 0.00026, ProbeTokS: 233.8, AAIntel: 48.1},
		{ID: "kimi-k3.1-pro", Name: "Kimi", CostTask: 0.0204, ProbeTokS: 78.7, AAIntel: 55.1},
	}
	MarkPareto(rows)
	if paretoOf(rows, "deepseek-v4.1-flash") {
		t.Fatal("DeepSeek should be dominated while the contributor model is visible")
	}
	rows = slices.DeleteFunc(rows, func(r Row) bool { return r.ID == "muse-spark-1.3-contributor" })
	MarkPareto(rows)
	if !paretoOf(rows, "deepseek-v4.1-flash") {
		t.Error("DeepSeek should be Pareto after -privacy removes the contributor model")
	}
}

func paretoOf(rows []Row, id string) bool {
	for _, r := range rows {
		if r.ID == id {
			return r.Pareto
		}
	}
	return false
}

func TestBuildContextAndPrivacyColumns(t *testing.T) {
	rows := Build([]modelsdev.Model{
		{
			ID: "glm-5.3", Name: "GLM-5.3",
			Limit: modelsdev.Context{Context: 1_000_000, Output: 131_072},
			Cost:  modelsdev.Cost{Input: 1.4, Output: 4.4, CacheRead: 0.26},
		},
		{
			ID: "space-bunny", Name: "Space Bunny",
			Limit: modelsdev.Context{Context: 200_000},
			Cost:  modelsdev.Cost{Input: 1, Output: 2},
		},
	}, nil, nil, nil, nil, nil, nil, "go", false)
	if len(rows) != 2 {
		t.Fatalf("got %d rows", len(rows))
	}
	byID := map[string]Row{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	if got := byID["glm-5.3"]; got.Privacy != "safe" || got.CtxK != 1000 || got.OutK != 131.072 {
		t.Errorf("glm row = privacy %q ctx %v out %v", got.Privacy, got.CtxK, got.OutK)
	}
	if got := byID["space-bunny"]; got.Privacy != "unknown" {
		t.Errorf("unknown privacy = %q, want unknown", got.Privacy)
	}
}

func TestWithPeakTwins(t *testing.T) {
	ds := modelsdev.Model{
		ID: "deepseek-v4.1-flash", Name: "DeepSeek V4.1 Flash",
		Cost: modelsdev.Cost{Input: 0.10, Output: 0.40, CacheRead: 0.002},
	}
	off, on := cost.PerRequest(ds, false), cost.PerRequest(ds, true)
	if on <= off || off <= 0 {
		t.Fatalf("fixture needs an off-peak/peak spread, got %v/%v", off, on)
	}
	plain := modelsdev.Model{ID: "plain", Name: "Plain", Cost: modelsdev.Cost{Input: 1, Output: 2}}
	rows := []Row{
		{ID: ds.ID, Name: ds.Name, CostTask: off, AllowanceUSD: 80, TokPerSec: 100, AAIntel: 50},
		{ID: "plain", Name: "Plain", CostTask: 0.01},
		{ID: "ghost", Name: "Ghost", CostTask: 0.01}, // not in models: left alone
	}
	got := WithPeakTwins(rows, []modelsdev.Model{ds, plain})
	if len(got) != 4 {
		t.Fatalf("got %d rows, want 4 (one twin)", len(got))
	}
	twin := got[1] // right after its base
	if twin.ID != "deepseek-v4.1-flash-peak" || twin.Name != "DeepSeek V4.1 Flash [peak]" {
		t.Errorf("twin identity = %q/%q", twin.ID, twin.Name)
	}
	if twin.CostTask != on {
		t.Errorf("twin cost = %v, want peak %v", twin.CostTask, on)
	}
	if twin.RQuota != 80/on {
		t.Errorf("twin quota = %v, want %v", twin.RQuota, 80/on)
	}
	if twin.TokPerSec != 100 || twin.AAIntel != 50 {
		t.Errorf("twin must inherit speed and scores: %+v", twin)
	}
	if got[0].ID != ds.ID || got[2].ID != "plain" || got[3].ID != "ghost" {
		t.Errorf("twin must sit right after its base: %v", got)
	}
	if got := WithPeakTwins(nil, nil); len(got) != 0 {
		t.Errorf("empty in must stay empty: %v", got)
	}
}

func TestBuildSWEBenchWithoutAA(t *testing.T) {
	rows := Build([]modelsdev.Model{
		{ID: "glm-5.3", Name: "GLM-5.3"},
	}, nil, nil, map[string]float64{"glm53": 70.0}, nil, nil, nil, "go", false)
	if len(rows) != 1 || rows[0].SWEBench != 70.0 {
		t.Errorf("SWEBench without AA = %+v, want 70.0", rows)
	}
}
