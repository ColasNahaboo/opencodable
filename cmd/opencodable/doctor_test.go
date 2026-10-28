package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ColasNahaboo/opencodable/internal/aa"
	"github.com/ColasNahaboo/opencodable/internal/modelsdev"
	"github.com/ColasNahaboo/opencodable/internal/probe"
)

func TestBuildDoctorRows(t *testing.T) {
	glm := modelsdev.Model{
		ID: "glm-5.3", Name: "GLM-5.3",
		Limit: modelsdev.Context{Context: 1_000_000, Output: 131_072},
		Cost:  modelsdev.Cost{Input: 1.4, Output: 4.4},
	}
	space := modelsdev.Model{
		ID: "space-bunny", Name: "Space Bunny",
		Cost: modelsdev.Cost{Input: 1, Output: 2},
	}
	free := modelsdev.Model{ID: "cat-jam-free", Name: "Cat Jam Free"}
	rows := buildDoctorRows(
		[]modelsdev.Model{glm, space, free},
		[]aa.Model{{Slug: "glm-5-3"}},
		map[string]float64{"glm53": 70.0},
		map[string]probe.Result{
			"glm-5.3":     {ModelID: "glm-5.3", TokPerSec: 100, WindowSeconds: 3},
			"space-bunny": {ModelID: "space-bunny", TokPerSec: 900, WindowSeconds: 2, Suspect: true},
		},
		map[string]time.Duration{"glm-5.3": 2 * time.Hour, "space-bunny": time.Hour},
	)
	if len(rows) != 3 {
		t.Fatalf("got %d rows", len(rows))
	}
	byID := map[string]doctorRow{}
	for _, r := range rows {
		byID[r.ID] = r
	}

	g := byID["glm-5.3"]
	if g.Plan != "yes" || g.AA != "glm-5-3" || g.SWE != 70 || g.Probe != "ok 2h" || len(g.Warnings) != 0 {
		t.Errorf("glm row = %+v", g)
	}
	if !g.ProbeOK {
		t.Error("glm ProbeOK = false, want true for a trusted probe")
	}
	if g.CtxK != 1000 || g.OutK != 131.072 || g.Privacy != "safe" {
		t.Errorf("glm columns = ctx %v out %v privacy %q", g.CtxK, g.OutK, g.Privacy)
	}

	s := byID["space-bunny"]
	if s.Plan != "missing" || s.Probe != "suspect" {
		t.Errorf("space row = %+v", s)
	}
	for _, want := range []string{"no plan entry", "no AA match", "suspect probe"} {
		if !slices.Contains(s.Warnings, want) {
			t.Errorf("space warnings = %v, missing %q", s.Warnings, want)
		}
	}

	c := byID["cat-jam-free"]
	if c.Plan != "free" || !slices.Contains(c.Warnings, "free variant not in plan snapshot") {
		t.Errorf("free row = %+v", c)
	}
}

func TestDoctorRenderPipedAndTTY(t *testing.T) {
	rows := []doctorRow{{
		ID: "glm-5.3", Plan: "yes", AA: "glm-5-3", SWE: 70, Privacy: "safe",
		Probe: "ok 2h", ProbeOK: true,
	}}
	var piped strings.Builder
	if err := writeDoctor(&piped, rows, false, false); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(piped.String(), "\n"), "\n")
	header := ""
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "# "): // legend prose is opt-in
			t.Errorf("legend must be opt-in (-l): %q", l)
		case strings.HasPrefix(l, "#id"):
			if header == "" {
				header = l
			}
		default:
			if strings.Contains(l, "  ") {
				t.Errorf("piped doctor rows must not pad: %q", l)
			}
			if got, want := len(strings.Split(l, "\t")), 7; got != want {
				t.Errorf("piped row %q has %d fields, want %d", l, got, want)
			}
		}
	}
	if header != "#id\tplan\taa\tswe_b\tprivacy\tprobe\twarnings" {
		t.Errorf("piped header = %q", header)
	}
	var tty strings.Builder
	if err := writeDoctor(&tty, rows, true, false); err != nil {
		t.Fatal(err)
	}
	out := tty.String()
	headerFound := false
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "#id") {
			headerFound = true
		}
	}
	if !headerFound {
		t.Errorf("tty header must start with #id:\n%s", out[:min(200, len(out))])
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "glm-5.3") && !strings.Contains(l, "  ") {
			t.Errorf("tty mode must pad columns: %q", l)
		}
	}
	var with strings.Builder
	if err := writeDoctor(&with, rows, false, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(with.String(), "# id        models.dev model ID") {
		t.Error("-l must print the doctor legend")
	}
}

func TestCompactAge(t *testing.T) {
	cases := map[time.Duration]string{
		30 * time.Second: "30s",
		17 * time.Minute: "17m",
		3 * time.Hour:    "3h",
		50 * time.Hour:   "2d",
	}
	for d, want := range cases {
		if got := compactAge(d); got != want {
			t.Errorf("compactAge(%v) = %q, want %q", d, got, want)
		}
	}
	if !strings.HasPrefix(compactAge(0), "now") {
		t.Errorf("compactAge(0) = %q", compactAge(0))
	}
}
