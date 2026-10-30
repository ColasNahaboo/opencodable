package main

import (
	"math"
	"testing"

	"github.com/ColasNahaboo/opencodable/internal/goplan"
	"github.com/ColasNahaboo/opencodable/internal/modelsdev"
	"github.com/ColasNahaboo/opencodable/internal/usage"
)

func TestWindowDollars(t *testing.T) {
	goplan.Use(nil) // built-in snapshot: glm-5.3-flash $60, kimi-k3 $15
	models := []modelsdev.Model{
		{ID: "glm-5.3-flash"},
		{ID: "kimi-k3"},
		{ID: "beam-free"},       // unlimited: excluded from the base
		{ID: "not-in-the-plan"}, // unknown: excluded from the base
	}
	ws := []usage.Window{
		{Name: "5h", Percent: 35},
		{Name: "week", Percent: 40},
		{Name: "month", Percent: 21},
	}
	got := windowDollars(ws, models, "go")
	base := 75.0 // 60 + 15
	want := map[string]float64{
		"5h":    0.65 * 0.20 * base,
		"week":  0.60 * 0.50 * base,
		"month": 0.79 * base,
	}
	for name, w := range want {
		if math.Abs(got[name]-w) > 1e-9 {
			t.Errorf("%s = %v, want %v", name, got[name], w)
		}
	}
}

func TestWindowDollarsClampsAndEmpty(t *testing.T) {
	goplan.Use(nil)
	models := []modelsdev.Model{{ID: "glm-5.3-flash"}}
	ws := []usage.Window{
		{Name: "5h", Percent: 120}, // over-consumed aggregate: clamp to 0
		{Name: "month", Percent: -5},
	}
	got := windowDollars(ws, models, "go")
	if got["5h"] != 0 {
		t.Errorf("5h = %v, want 0", got["5h"])
	}
	if math.Abs(got["month"]-60) > 1e-9 { // glm-5.3-flash only: $60 × clamped 1.0
		t.Errorf("month = %v, want 60", got["month"])
	}
	empty := windowDollars([]usage.Window{{Name: "5h", Percent: 10}}, nil, "go")
	if v, ok := empty["5h"]; !ok || v != 0 {
		t.Errorf("no-model 5h = %v,%v, want 0,true", v, ok)
	}
}
