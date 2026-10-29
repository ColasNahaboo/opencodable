package bench

import (
	"errors"
	"strings"
	"testing"
)

func TestArenaBaseKey(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"GPT-6 Luna (Max)", "gpt6luna"},        // parenthetical effort goes away
		{"Muse Spark 1.3 (Max)", "musespark13"}, // spaces and dots collapse
		{"gpt-5.6-sol-xhigh", "gpt56sol"},       // dash effort, longest first
		{"claude-opus-4-6-high", "claudeopus46"},
		{"Step 5 Preview", "step5preview"},
		{"Qwen3.8-Max", "qwen38max"},
		{"MiMo-V2.6-Flash", "mimov26flash"},
		{"flow", "flow"}, // plain word ending in low must not strip
		{"  Kimi K3 (Max)  ", "kimik3"},
	} {
		if got := arenaBaseKey(tc.in); got != tc.want {
			t.Errorf("arenaBaseKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseElo(t *testing.T) {
	raw := []byte(`{"meta": {"leaderboard": "text"}, "models": [
		{"rank": 1, "model": "claude-fable-5-high", "score": 1504},
		{"rank": 2, "model": "claude-fable-5-xhigh", "score": 1490},
		{"rank": 3, "model": "gpt-5.6-sol-xhigh", "score": 1484},
		{"rank": 4, "model": "ghost", "score": null},
		{"rank": 5, "model": "zero", "score": 0},
		{"rank": 6, "model": "", "score": 1500}
	]}`)
	m := parseElo(raw)
	if len(m) != 2 {
		t.Fatalf("got %d families, want 2: %v", len(m), m)
	}
	if m["claudefable5"] != 1504 {
		t.Errorf("best variant must win: %v", m)
	}
	if m["gpt56sol"] != 1484 {
		t.Errorf("gpt56sol = %v", m["gpt56sol"])
	}
	for _, bad := range []string{"", "not json", `{"models": []}`, `{"meta": 1}`} {
		if got := parseElo([]byte(bad)); got != nil {
			t.Errorf("parseElo(%q) = %v, want nil", bad, got)
		}
	}
}

func TestResolveEloSnapshot(t *testing.T) {
	const good = `{"models": [{"model": "a-high", "score": 1400}]}`
	const empty = `{"models": []}`
	calls := 0
	fetch := func(url string) ([]byte, error) {
		calls++
		switch {
		case strings.HasSuffix(url, "/latest.json"):
			return []byte(`{"date": "2026-10-28", "path": "2026-10-28"}`), nil
		case strings.HasSuffix(url, "/2026-10-28/text.json"):
			return nil, errors.New("url: HTTP 404") // current snapshot missing: walk back
		case strings.HasSuffix(url, "/2026-10-27/text.json"):
			return []byte(empty), nil // empty snapshot: walk back
		case strings.HasSuffix(url, "/2026-10-26/text.json"):
			return []byte(good), nil
		default:
			t.Errorf("unexpected fetch %s", url)
			return nil, errors.New("url: HTTP 404")
		}
	}
	raw, err := resolveEloSnapshot(fetch)
	if err != nil || string(raw) != good {
		t.Errorf("resolve = %q, %v; want the Oct 26 snapshot", raw, err)
	}
	if calls != 4 {
		t.Errorf("resolve fetched %d URLs, want 4 (pointer + 3 days)", calls)
	}
}

func TestResolveEloSnapshotAbortsOnNetworkError(t *testing.T) {
	fetch := func(url string) ([]byte, error) {
		if strings.HasSuffix(url, "/latest.json") {
			return []byte(`{"date": "2026-10-28", "path": "2026-10-28"}`), nil
		}
		return nil, errors.New("connection refused") // not a 404: fail fast
	}
	if _, err := resolveEloSnapshot(fetch); err == nil {
		t.Error("network errors must abort the walkback, not walk 31 days")
	}
}

func TestMergeBTR(t *testing.T) {
	raw := []byte(`{"rows": [
		{"row": {"model_name": "Muse Spark 1.3 (Max)", "category": "overall", "rating": 0.025936}},
		{"row": {"model_name": "Muse Spark 1.2 (xHigh)", "category": "overall", "rating": -0.07538}},
		{"row": {"model_name": "Kimi K3 (Max)", "category": "overall", "score": 0.010787}},
		{"row": {"model_name": "Code Only", "category": "coding", "rating": 0.5}},
		{"row": {"model_name": "No Value", "category": "overall"}},
		{"row": {"model_name": "", "category": "overall", "rating": 0.1}}
	]}`)
	out := BTR{}
	if n, overall := mergeBTR(out, raw); n != 6 || overall != 4 {
		t.Errorf("merge = (%d, %d), want (6, 4)", n, overall)
	}
	if len(out) != 3 {
		t.Fatalf("got %d families, want 3: %v", len(out), out)
	}
	if out["musespark13"] != 0.025936 {
		t.Errorf("musespark13 = %v", out["musespark13"])
	}
	if out["musespark12"] != -0.07538 {
		t.Errorf("negative btr is real data, must be kept: %v", out["musespark12"])
	}
	if out["kimik3"] != 0.010787 {
		t.Errorf("score fallback = %v", out["kimik3"])
	}
	if n, _ := mergeBTR(BTR{}, []byte("nope")); n != -1 {
		t.Errorf("bad page = %d, want -1", n)
	}
}

func TestArenaLookup(t *testing.T) {
	elo := Elo{"musespark13": 1500, "gpt56sol": 1484}
	if v, ok := elo.Lookup("muse-spark-1.3-contributor"); !ok || v != 1500 {
		t.Errorf("plan suffix must strip: %v, %v", v, ok)
	}
	if _, ok := elo.Lookup("not-there"); ok {
		t.Error("unknown model must not match")
	}
	btr := BTR{"deepseekv41flash20260910": 0.019122}
	if v, ok := btr.Lookup("deepseek-v4.1-flash"); !ok || v != 0.019122 {
		t.Errorf("dated alias must resolve: %v, %v", v, ok)
	}
	var nilMap BTR
	if _, ok := nilMap.Lookup("anything"); ok {
		t.Error("nil map must miss")
	}
}

func TestArenaNear(t *testing.T) {
	btr := BTR{"glm53": 1480, "gemini4argon": 1520, "musespark13max": 1490, "gpt56sol": 1484}
	for _, tc := range []struct{ id, want string }{
		{"glm-5.3-flash", "glm53"},                       // shortened family name: likely alias
		{"glm-5.3-flash-free", "glm53"},                  // plan suffix stripped before comparing
		{"muse-spark-1.3-contributor", "musespark13max"}, // the leaderboard adds a Max qualifier
		{"gemini-3.8-flash", ""},                         // family alone shares no version digit
		{"kimi-k2.7-code", ""},                           // no leaderboard key shares the prefix
		{"gpt-5.6-luna", ""},                             // another variant (Luna, not Sol) is no alias
	} {
		if got := btr.Near(tc.id); got != tc.want {
			t.Errorf("Near(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
	var nilFeed Elo
	if got := nilFeed.Near("glm-5.3"); got != "" {
		t.Errorf("nil feed suggested %q", got)
	}
}
