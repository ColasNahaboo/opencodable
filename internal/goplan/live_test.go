package goplan

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ColasNahaboo/opencodable/internal/modelsdev"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mergedFixture(t *testing.T) *LiveSnapshot {
	t.Helper()
	ep, err := ParseEndpointModels(fixture(t, "go-models.json"))
	if err != nil {
		t.Fatal(err)
	}
	console, err := ParseConsoleGo(fixture(t, "console-go.html"))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := Merge(ep, console)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func TestParseEndpointModels(t *testing.T) {
	ep, err := ParseEndpointModels(fixture(t, "go-models.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := ep["deepseek-v4.1-flash"]
	if m == nil || m.Factor != 2 || m.In != 0.15 || m.Out != 0.6 {
		t.Fatalf("deepseek endpoint entry = %+v", m)
	}
	if m.GoAllowance != 100 || m.PlusAllowance != 200 {
		t.Fatalf("deepseek allowances = %v/%v, want 100/200", m.GoAllowance, m.PlusAllowance)
	}
	if m.Training {
		t.Fatal("deepseek reported as training")
	}
	if other := ep["glm-5.3-flash"]; other == nil || other.Factor != 0 || other.Unlimited {
		t.Fatalf("glm endpoint entry = %+v", other)
	}
	// Free models: no usage block and zero prices.
	if beam := ep["beam-free"]; beam == nil || !beam.Unlimited || beam.In != 0 {
		t.Fatalf("beam-free endpoint entry = %+v", beam)
	}
	if ice := ep["iceman-contributor-free"]; ice == nil || !ice.Unlimited || !ice.Training {
		t.Fatalf("iceman endpoint entry = %+v", ice)
	}
}

func TestParseConsoleGo(t *testing.T) {
	console, err := ParseConsoleGo(fixture(t, "console-go.html"))
	if err != nil {
		t.Fatal(err)
	}
	// Display names are matched with punctuation and case stripped, so the
	// endpoint's "LongCat-2.5" finds the console's "LongCat 2.5".
	if k := console["longcat25"]; k.Name != "LongCat 2.5" {
		t.Fatalf("longcat row = %+v", k)
	}
	if r := console["kimik3"]; r.Reqs["go"] != 98 || r.Reqs["go-plus"] != 392 {
		t.Fatalf("kimi-k3 requests = %+v", r.Reqs)
	}
	if u := console["kimik3"]; u.Usage["go"] != 15 || u.Usage["go-plus"] != 60 {
		t.Fatalf("kimi-k3 usage = %+v", u.Usage)
	}
	if r := console["glm53flash"]; r.Reqs["go"] != 6320 {
		t.Fatalf("glm-5.3-flash requests = %+v", r.Reqs)
	}
	// Free models carry an infinity marker, parsed as absent.
	if beam := console["beamfree"]; beam.Reqs["go"] != 0 {
		t.Fatalf("beam-free requests = %+v", beam.Reqs)
	}
}

func TestMergeSnapshot(t *testing.T) {
	snap := mergedFixture(t)
	r := snap.Rows["glm-5.3-flash"]
	if r == nil || r.GoAllowance != 60 || r.PlusAllowance != 180 {
		t.Fatalf("glm-5.3-flash = %+v", r)
	}
	// 5h budget $12 over 6320 requests; a month is 5 windows.
	if r.CostReq < 0.00189 || r.CostReq > 0.00191 || r.ReqsMo != 5*r.Reqs5h {
		t.Fatalf("glm-5.3-flash cost/reqs = %v/%v", r.CostReq, r.ReqsMo)
	}
	ds := snap.Rows["deepseek-v4.1-flash"]
	if ds == nil || ds.Peak == nil || ds.Peak[0] != 0.30 || ds.Peak[1] != 1.20 {
		t.Fatalf("deepseek peak = %+v", ds)
	}
	if ds.Factor != 2 || ds.GoAllowance != 100 || ds.Reqs5h != 43300 {
		t.Fatalf("deepseek = %+v", ds)
	}
	// LongCat-2.5: the endpoint and console names differ in punctuation.
	lc := snap.Rows["longcat-2.5"]
	if lc == nil || lc.GoAllowance != 60 || lc.PlusAllowance != 240 || lc.CostReq <= 0 {
		t.Fatalf("longcat-2.5 = %+v", lc)
	}
	if beam := snap.Rows["beam-free"]; beam == nil || !beam.Unlimited || beam.CostReq != 0 {
		t.Fatalf("beam-free = %+v", beam)
	}
	if ice := snap.Rows["iceman-contributor-free"]; ice == nil || !ice.Training {
		t.Fatalf("iceman = %+v", ice)
	}
	for id, row := range snap.Rows {
		if row.ID != id {
			t.Fatalf("row %s has ID %q", id, row.ID)
		}
	}
	if len(snap.Rows) < 25 {
		t.Fatalf("rows = %d", len(snap.Rows))
	}
}

func TestMergeRejectsBrokenSources(t *testing.T) {
	ep, err := ParseEndpointModels(fixture(t, "go-models.json"))
	if err != nil {
		t.Fatal(err)
	}
	console, err := ParseConsoleGo(fixture(t, "console-go.html"))
	if err != nil {
		t.Fatal(err)
	}
	// A console page that lost a model's row must fail the snapshot, not
	// silently degrade cost_task to the generic profile.
	delete(console, "kimik3")
	if _, err := Merge(ep, console); err == nil || !strings.Contains(err.Error(), "kimi-k3") {
		t.Fatalf("missing console row error = %v", err)
	}
	// Console/endpoint disagreement on the monthly limits must fail too.
	console2, _ := ParseConsoleGo(fixture(t, "console-go.html"))
	if c, ok := console2["kimik3"]; ok {
		c.Usage["go"] = 16
		console2["kimik3"] = c
	}
	if _, err := Merge(ep, console2); err == nil || !strings.Contains(err.Error(), "console go limit") {
		t.Fatalf("usage mismatch error = %v", err)
	}
}

func TestMergeRoundTripsJSON(t *testing.T) {
	snap := mergedFixture(t)
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var back LiveSnapshot
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Rows) != len(snap.Rows) {
		t.Fatalf("rows %d vs %d", len(back.Rows), len(snap.Rows))
	}
	if back.Rows["glm-5.3-flash"].CostReq != snap.Rows["glm-5.3-flash"].CostReq {
		t.Fatal("cost_req lost in round-trip")
	}
	if !hasCostReq(&back) {
		t.Fatal("round-tripped snapshot flagged as legacy")
	}
}

// modelsdevFixture prices the snapshot models at their endpoint prices so the
// cross-source gate passes unless mutated.
func modelsdevFixture(t *testing.T, snap *LiveSnapshot) []modelsdev.Model {
	t.Helper()
	out := make([]modelsdev.Model, 0, len(snap.Rows))
	for id, r := range snap.Rows {
		out = append(out, modelsdev.Model{
			ID:   id,
			Name: id,
			Cost: modelsdev.Cost{Input: r.In, Output: r.Out, CacheRead: r.CacheRead},
		})
	}
	return out
}

func TestValidateAcceptsFixture(t *testing.T) {
	snap := mergedFixture(t)
	if errs := Validate(snap, modelsdevFixture(t, snap)); len(errs) > 0 {
		t.Fatalf("fixture rejected: %v", errs)
	}
}

func TestValidateGates(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*LiveSnapshot)
		want   string
	}{
		{"empty snapshot", func(s *LiveSnapshot) { s.Rows = map[string]*LiveRow{} }, "rows"},
		{"unknown model id", func(s *LiveSnapshot) {
			s.Rows["not-a-model"] = &LiveRow{ID: "not-a-model", GoAllowance: 10, In: 1, Out: 2, CacheRead: 0.03, Reqs5h: 100, CostReq: 0.03}
		}, "not-a-model"},
		{"price out of domain", func(s *LiveSnapshot) { s.Rows["glm-5.3-flash"].Out = 900 }, "domain"},
		{"allowance out of domain", func(s *LiveSnapshot) { s.Rows["glm-5.3-flash"].GoAllowance = 5000 }, "domain"},
		{"cost per request out of domain", func(s *LiveSnapshot) { s.Rows["glm-5.3-flash"].CostReq = 3 }, "domain"},
		{"missing estimated requests", func(s *LiveSnapshot) { s.Rows["glm-5.3-flash"].Reqs5h = 0 }, "completeness"},
		{"plus-plan cost disagrees", func(s *LiveSnapshot) { s.Rows["glm-5.3-flash"].Reqs5hPlus = 1 }, "requests"},
		{"price disagrees with catalog", func(s *LiveSnapshot) {
			r := s.Rows["glm-5.3-flash"]
			r.Out *= 3
			r.CostReq /= 3
		}, "catalog"},
		{"drift beyond builtin", func(s *LiveSnapshot) {
			r := s.Rows["kimi-k3"]
			r.GoAllowance = 15 * 8 // > 4× the built-in 15
		}, "drift"},
		{"peak factor mismatch", func(s *LiveSnapshot) {
			s.Rows["deepseek-v4.1-flash"].Peak[0] *= 3
		}, "peak"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap := mergedFixture(t)
			catalog := modelsdevFixture(t, mergedFixture(t))
			tc.mutate(snap)
			errs := Validate(snap, catalog)
			if len(errs) == 0 {
				t.Fatal("expected rejection")
			}
			joined := errJoin(errs)
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("errors %q do not mention %q", joined, tc.want)
			}
		})
	}
}

func TestLegacyCachedSnapshotRejected(t *testing.T) {
	// Snapshots from the docs-page era carry token profiles but no
	// derived per-request cost; they must not become the live overlay.
	legacy := &LiveSnapshot{Rows: map[string]*LiveRow{
		"glm-5.3-flash": {ID: "glm-5.3-flash", GoAllowance: 60, In: 0.15, Out: 0.5, CacheRead: 0.03},
	}}
	b, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var back LiveSnapshot
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if hasCostReq(&back) {
		t.Fatal("legacy snapshot has cost_req")
	}
}

func TestOverlayAccessors(t *testing.T) {
	defer Use(nil)
	snap := mergedFixture(t)
	Use(snap)

	if a, _ := Allowance("glm-5.3-flash", "go"); a != 60 {
		t.Fatalf("allowance = %v", a)
	}
	if a, _ := Allowance("glm-5.3-flash", "go-plus"); a != 180 {
		t.Fatalf("go-plus allowance = %v", a)
	}
	if c, ok := CostRequest("glm-5.3-flash"); !ok || c < 0.0018 || c > 0.002 {
		t.Fatalf("cost request = %v %v", c, ok)
	}
	if _, ok := CostRequest("totally-unknown-model"); ok {
		t.Fatal("unknown model has a live cost request")
	}
	if _, ok := CostRequest("beam-free"); ok {
		t.Fatal("free model has a live cost request")
	}
	if f, ok := PeakFactor("deepseek-v4.1-flash"); !ok || f != 2 {
		t.Fatalf("peak factor = %v %v", f, ok)
	}
	if _, ok := PeakFactor("glm-5.3-flash"); ok {
		t.Fatal("flat-priced model has a peak factor")
	}
	if trains, known := Privacy("iceman-contributor-free"); !trains || !known {
		t.Fatalf("privacy = %v/%v, want true/true", trains, known)
	}
	if trains, known := Privacy("longcat-2.5"); trains || !known {
		t.Fatalf("privacy = %v/%v, want false/true", trains, known)
	}
	// Built-in-only models keep answering after the overlay is installed.
	if !Known("gemini-4-argon") {
		t.Fatal("builtin model lost")
	}
	if _, ok := Allowance("totally-unknown-model", "go"); ok {
		t.Fatal("unknown model reported known")
	}
	if src := Source(); !strings.Contains(src, "live") {
		t.Fatalf("source = %q", src)
	}

	Use(nil)
	if src := Source(); !strings.Contains(src, SnapshotDate) {
		t.Fatalf("builtin source = %q", src)
	}
}
