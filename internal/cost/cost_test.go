package cost

import (
	"math"
	"testing"

	"github.com/ColasNahaboo/opencodable/internal/modelsdev"
)

func TestFromTokens(t *testing.T) {
	m := modelsdev.Model{
		ID: "deepseek-v4.1-flash",
		Cost: modelsdev.Cost{
			Input: 0.15, Output: 0.60, CacheRead: 0.003, CacheWrite: 0.01,
		},
	}
	got := FromTokens(m, 1_000_000, 1_000_000, 1_000_000, 1_000_000, false)
	want := 0.15 + 0.003 + 0.60 + 0.01
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("off-peak = %v, want %v", got, want)
	}
	got = FromTokens(m, 1_000_000, 1_000_000, 1_000_000, 1_000_000, true)
	want = 0.30 + 0.006 + 1.20 + 0.01
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("peak = %v, want %v", got, want)
	}
}

func TestPerRequestUsesOfficialProfile(t *testing.T) {
	m := modelsdev.Model{
		ID: "deepseek-v4.1-flash",
		Cost: modelsdev.Cost{
			Input: 0.15, Output: 0.60, CacheRead: 0.003,
		},
	}
	// goplan profile: 410 input, 71300 cached, 310 output tokens.
	got := PerRequest(m, false)
	want := (410*0.15 + 71_300*0.003 + 310*0.60) / 1e6
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("PerRequest = %v, want %v", got, want)
	}
}
