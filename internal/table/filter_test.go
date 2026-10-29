package table

import "testing"

func ptr(v float64) *float64 { return &v }

func TestFilterExcluded(t *testing.T) {
	f := Filter{Exclude: []string{"grok", "kimi-*", "*preview"}}
	cases := []struct {
		id, name string
		want     bool
	}{
		{"grok-4.1", "Grok 4.1", true},           // substring, case-insensitive
		{"GroK-2", "GROK 2", true},               // both sides lowercased
		{"kimi-k3.1-pro", "Kimi K3.1 Pro", true}, // prefix glob on the ID
		{"muse-spark-1.3", "Muse Preview", true}, // suffix glob on the name
		{"x-ai/grok-4", "Grok 4", true},          // * crosses / in provider IDs
		{"deepseek-v4.1-flash", "DeepSeek V4.1 Flash", false},
		{"muse-spark-1.3", "Muse", false},
	}
	for _, tc := range cases {
		if got := f.Excluded(tc.id, tc.name); got != tc.want {
			t.Errorf("Excluded(%q, %q) = %v, want %v", tc.id, tc.name, got, tc.want)
		}
	}
	if (Filter{}).Excluded("grok-4.1", "Grok 4.1") {
		t.Error("empty filter must exclude nothing")
	}
	if !(Filter{Exclude: []string{"*grok*"}}).Excluded("x-ai/grok-4", "Grok 4") {
		t.Error("wildcards must cross / in provider-prefixed IDs")
	}
}

func TestFilterIncluded(t *testing.T) {
	if !(Filter{}).Included("anything", "Anything") {
		t.Error("empty -include must keep everything")
	}
	f := Filter{Include: []string{"deepseek", "kimi-*"}, Exclude: []string{"*flash*"}}
	cases := []struct {
		id, name string
		want     bool
	}{
		{"deepseek-v4-pro", "DeepSeek V4 Pro", true},    // substring include
		{"kimi-k3.1-pro", "Kimi K3.1 Pro", true},        // prefix glob include
		{"KIMI-K2", "Kimi K2", true},                    // case-insensitive
		{"grok-4.1", "Grok 4.1", false},                 // matches nothing
		{"deepseek-v4.1-flash", "DeepSeek Flash", true}, // include match here...
	}
	for _, tc := range cases {
		if got := f.Included(tc.id, tc.name); got != tc.want {
			t.Errorf("Included(%q, %q) = %v, want %v", tc.id, tc.name, got, tc.want)
		}
	}
	// ...but -exclude still wins over -include in the combined predicate.
	combined := func(id, name string) bool {
		return f.Excluded(id, name) || !f.Included(id, name)
	}
	if !combined("deepseek-v4.1-flash", "DeepSeek Flash") {
		t.Error("exclude *flash* must drop the included flash model")
	}
	if combined("deepseek-v4-pro", "DeepSeek V4 Pro") {
		t.Error("included non-excluded model must survive")
	}
}

// TestExcludePeakBrackets guards the [peak] twin convention: brackets are
// literal in patterns (unlike path.Match), so '*[peak]' selects twins only
// while a family name hides both regimes.
func TestExcludePeakBrackets(t *testing.T) {
	twins := Filter{Exclude: []string{"*[peak]"}}
	if !twins.Excluded("deepseek-v4.1-flash-peak", "DeepSeek V4.1 Flash [peak]") {
		t.Error("*[peak] must match the twin")
	}
	if twins.Excluded("deepseek-v4.1-flash", "DeepSeek V4.1 Flash") {
		t.Error("*[peak] must not match the base row")
	}
	family := Filter{Exclude: []string{"deepseek"}}
	if !family.Excluded("deepseek-v4.1-flash-peak", "DeepSeek V4.1 Flash [peak]") {
		t.Error("family substring must hide the twin too")
	}
}

func TestFilterCost(t *testing.T) {
	f := Filter{MaxCost: ptr(0.01)}
	for _, tc := range []struct {
		cost float64
		want bool
	}{
		{0, true},    // free models always pass
		{0.01, true}, // bound is inclusive
		{0.0101, false},
		{1, false},
	} {
		if got := f.Keep(Row{CostTask: tc.cost}); got != tc.want {
			t.Errorf("Keep(cost_task=%v) = %v, want %v", tc.cost, got, tc.want)
		}
	}
	if !(Filter{}).Keep(Row{CostTask: 999}) {
		t.Error("unset -max-cost must keep expensive rows")
	}
}

func TestFilterSpeed(t *testing.T) {
	min := 60.0
	f := Filter{MinSpeed: &min}
	for _, tc := range []struct {
		name string
		row  Row
		want bool
	}{
		{"measured wins", Row{TokPerSec: 20, ProbeTokS: 80}, true},
		{"AA only", Row{TokPerSec: 70}, true},
		{"slow", Row{TokPerSec: 20}, false},
		{"slow probe beats fast AA", Row{TokPerSec: 90, ProbeTokS: 40}, false},
		{"missing kept by default", Row{}, true},
	} {
		if got := f.Keep(tc.row); got != tc.want {
			t.Errorf("%s: Keep = %v, want %v", tc.name, got, tc.want)
		}
	}
	strict := f
	strict.DropMissing = true
	if strict.Keep(Row{}) {
		t.Error("-missing drop must remove rows with no speed")
	}
}

func TestFilterQuality(t *testing.T) {
	composite := Filter{MinQuality: map[string]float64{"quality": 40}}
	if !composite.Keep(Row{AAIntel: 50}) {
		t.Error("composite aa_int must pass")
	}
	if !composite.Keep(Row{AACoding: 60}) {
		t.Error("composite must fall back to aa_code when aa_int is missing")
	}
	if composite.Keep(Row{AAIntel: 30, AACoding: 90}) {
		t.Error("known aa_int below the bound must fail")
	}

	f := Filter{MinQuality: map[string]float64{"aa_int": 40, "aa_code": 35}}
	if !f.Keep(Row{AAIntel: 50}) {
		t.Error("a missing metric must not be judged by default")
	}
	if !f.Keep(Row{AAIntel: 50, AACoding: 40}) {
		t.Error("both known and above their bounds must pass")
	}
	if f.Keep(Row{AAIntel: 50, AACoding: 30}) {
		t.Error("metrics are ANDed: a known value below its bound must fail")
	}

	strict := Filter{MinQuality: map[string]float64{"aa_int": 40}, DropMissing: true}
	if strict.Keep(Row{AACoding: 90}) {
		t.Error("-missing drop must remove rows without aa_int")
	}
}

func TestFilterMissingSpeedAndQuality(t *testing.T) {
	// Both bounds set: -missing drop removes the row if either value is absent.
	f := Filter{
		MinSpeed:    ptr(50),
		MinQuality:  map[string]float64{"aa_int": 40},
		DropMissing: true,
	}
	if f.Keep(Row{TokPerSec: 80, AAIntel: 45}) != true {
		t.Error("complete row must pass")
	}
	if f.Keep(Row{TokPerSec: 80}) {
		t.Error("row without quality must fail under -missing drop")
	}
}

func TestQualityMetricNames(t *testing.T) {
	r := Row{
		AAIntel: 1, AACoding: 2, AAMath: 3, LiveCodeBench: 4, TerminalBench: 5,
		IFBench: 6, LCR: 7, SWEBench: 8, LBAgentic: 9, LBCoding: 10,
		Elo: 1500, BTRating: 0.05,
	}
	want := map[string]float64{
		"aa_int": 1, "aa_code": 2, "aa_math": 3, "code_b": 4, "term_b": 5,
		"if_b": 6, "aa_lcr": 7, "swe_b": 8, "lb_agent": 9, "lb_code": 10,
		"elo": 1500, "btr": 0.05,
	}
	if len(want) != len(QualityMetricNames) {
		t.Fatalf("QualityMetricNames has %d names, test covers %d", len(QualityMetricNames), len(want))
	}
	for _, name := range QualityMetricNames {
		if !IsQualityMetric(name) {
			t.Errorf("IsQualityMetric(%q) = false", name)
		}
		if v, ok := qualityValue(r, name); !ok || v != want[name] {
			t.Errorf("qualityValue(%q) = %v, %v; want %v", name, v, ok, want[name])
		}
	}
	if v, ok := qualityValue(Row{AACoding: 60}, "quality"); !ok || v != 60 {
		t.Errorf("composite fallback = %v, %v; want 60", v, ok)
	}
	if IsQualityMetric("bogus") {
		t.Error("IsQualityMetric(bogus) = true")
	}
}
