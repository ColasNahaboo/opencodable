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
	docs, err := ParseDocsHTML(fixture(t, "go-docs.html"))
	if err != nil {
		t.Fatal(err)
	}
	ep, err := ParseEndpointModels(fixture(t, "go-models.json"))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := Merge(docs, ep)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func TestParseDocsPricing(t *testing.T) {
	docs, err := ParseDocsHTML(fixture(t, "go-docs.html"))
	if err != nil {
		t.Fatal(err)
	}
	if len(docs.Pricing["go"]) < 25 {
		t.Fatalf("go pricing rows = %d, want >= 25", len(docs.Pricing["go"]))
	}
	var flash *PricingRow
	var beam, gemini, dsPeak *PricingRow
	for i := range docs.Pricing["go"] {
		r := &docs.Pricing["go"][i]
		switch r.Name {
		case "GLM-5.3-Flash":
			flash = r
		case "Beam Free":
			beam = r
		case "Gemini 4 Argon":
			gemini = r
		}
		if r.Peak && r.BaseName == "DeepSeek V4.1 Flash" {
			dsPeak = r
		}
	}
	if flash == nil || flash.GoAllowance != 60 || flash.In != 0.15 || flash.Out != 0.50 || flash.CacheRead != 0.03 {
		t.Fatalf("GLM-5.3-Flash row = %+v", flash)
	}
	if beam == nil || !beam.Unlimited {
		t.Fatalf("Beam Free row = %+v, want unlimited", beam)
	}
	// Strikethrough discount: the live price is the last number in the cell.
	if gemini == nil || gemini.In != 2.00 || gemini.Out != 10.00 {
		t.Fatalf("Gemini 4 Argon row = %+v, want discounted prices", gemini)
	}
	if dsPeak == nil || dsPeak.BaseName != "DeepSeek V4.1 Flash" {
		t.Fatalf("peak row = %+v", dsPeak)
	}
	if plus := docs.Pricing["go-plus"]; len(plus) < 25 {
		t.Fatalf("go-plus rows = %d", len(plus))
	}
	for _, r := range docs.Pricing["go-plus"] {
		if r.Name == "GLM-5.3-Flash" && r.GoAllowance != 180 {
			t.Fatalf("go-plus GLM-5.3-Flash allowance = %v, want 180", r.GoAllowance)
		}
	}
}

func TestParseDocsProfilesAndRequests(t *testing.T) {
	docs, err := ParseDocsHTML(fixture(t, "go-docs.html"))
	if err != nil {
		t.Fatal(err)
	}
	p := docs.Profiles["glm-5.3-flash"]
	if p == nil || *p != (Profile{1000, 55000, 200}) {
		t.Fatalf("profile GLM-5.3-Flash = %+v", p)
	}
	// Slash shorthand "GLM-5.3/5.2" must expand to both models.
	for _, name := range []string{"glm-5.3", "glm-5.2"} {
		if p := docs.Profiles[name]; p == nil || *p != (Profile{700, 52000, 150}) {
			t.Fatalf("profile %s = %+v", name, p)
		}
	}
	if r := docs.Requests["glm-5.3-flash"]; r != 31580 {
		t.Fatalf("requests GLM-5.3-Flash = %v, want 31580", r)
	}
	if len(docs.NameToID) < 25 {
		t.Fatalf("name→id map size = %d", len(docs.NameToID))
	}
	if got := docs.NameToID["deepseek v4.1 flash"]; got != "deepseek-v4.1-flash" {
		t.Fatalf("name→id map missing DeepSeek: %q", got)
	}
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
	if other := ep["glm-5.3-flash"]; other == nil || other.Factor != 0 {
		t.Fatalf("glm endpoint entry = %+v, want factor 0", other)
	}
}

func TestMergeSnapshot(t *testing.T) {
	snap := mergedFixture(t)
	r := snap.Rows["glm-5.3-flash"]
	if r == nil || r.GoAllowance != 60 || r.PlusAllowance != 180 {
		t.Fatalf("glm-5.3-flash = %+v", r)
	}
	if r.Profile == nil || *r.Profile != (Profile{1000, 55000, 200}) {
		t.Fatalf("glm profile = %+v", r.Profile)
	}
	if r.ReqsMo != 31580 {
		t.Fatalf("glm reqs_mo = %v", r.ReqsMo)
	}
	ds := snap.Rows["deepseek-v4.1-flash"]
	if ds == nil || ds.Peak == nil || ds.Peak[0] != 0.30 || ds.Peak[1] != 1.20 {
		t.Fatalf("deepseek peak = %+v", ds)
	}
	if ds.Factor != 2 {
		t.Fatalf("deepseek factor = %v", ds.Factor)
	}
	if beam := snap.Rows["beam-free"]; beam == nil || !beam.Unlimited {
		t.Fatalf("beam-free = %+v", beam)
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
	if *back.Rows["glm-5.3-flash"].Profile != *snap.Rows["glm-5.3-flash"].Profile {
		t.Fatal("profile lost in round-trip")
	}
}

// modelsdevFixture prices the snapshot models at their docs prices so the
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
			s.Rows["not-a-model"] = &LiveRow{ID: "not-a-model", GoAllowance: 10, In: 1, Out: 2, CacheRead: 0.03, ReqsMo: 100}
		}, "not-a-model"},
		{"price out of domain", func(s *LiveSnapshot) { s.Rows["glm-5.3-flash"].Out = 900 }, "domain"},
		{"allowance out of domain", func(s *LiveSnapshot) { s.Rows["glm-5.3-flash"].GoAllowance = 5000 }, "domain"},
		{"requests arithmetic broken", func(s *LiveSnapshot) { s.Rows["glm-5.3-flash"].GoAllowance = 120 }, "requests"},
		{"price disagrees with catalog", func(s *LiveSnapshot) {
			r := s.Rows["glm-5.3-flash"]
			r.Out *= 3
			r.ReqsMo /= 3
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
	if p := ProfileFor("glm-5.3-flash"); p != (Profile{1000, 55000, 200}) {
		t.Fatalf("profile = %+v", p)
	}
	pk, ok := PeakPrice("deepseek-v4.1-flash")
	if !ok || pk[0] != 0.30 {
		t.Fatalf("peak = %v %v", pk, ok)
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
