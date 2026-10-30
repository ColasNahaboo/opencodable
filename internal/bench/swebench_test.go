package bench

import "testing"

func TestParseSWEBench(t *testing.T) {
	raw := []byte(`{"leaderboards":[
		{"name":"Test","results":[{"model_display":"Kimi K3","resolved":99.0}]},
		{"name":"Verified","results":[
			{"model_display":"MiniMax M2.5","agent":"mini-SWE-agent","resolved":70.0},
			{"model_display":"MiniMax M2.5","agent":"other-agent","resolved":75.8},
			{"model_display":"","resolved":90.0},
			{"model_display":"Ghost","resolved":0}
		]},
		{"name":"Lite","results":[{"model_display":"MiniMax M2.5","resolved":88.8}]}
	]}`)
	got := parseSWEBench(raw)
	if v := got["minimaxm25"]; v != 75.8 {
		t.Errorf("minimaxm25 = %v, want 75.8 (best Verified run)", v)
	}
	if _, ok := got["kimik3"]; ok {
		t.Error("Test board must be ignored")
	}
	if _, ok := got["lite-board-minimaxm25"]; ok {
		t.Error("Lite board must be ignored")
	}
	if _, ok := got["ghost"]; ok {
		t.Error("zero-resolved rows must be skipped")
	}
}

func TestParseSWEBenchBadJSON(t *testing.T) {
	if got := parseSWEBench([]byte("not json")); got != nil {
		t.Errorf("want nil map on garbage, got %v", got)
	}
}

func TestLookupSWE(t *testing.T) {
	swe := map[string]float64{
		"deepseekv41flash": 68.5,
		"musespark13":      71.2,
		"reflectionbeam":   54.0,
	}
	// Resolves without AA slug (e.g. no AA_API_KEY) and when AA uses a dated slug.
	if got := LookupSWE(swe, "deepseek-v4.1-flash", "DeepSeek V4.1 Flash", ""); got != 68.5 {
		t.Errorf("LookupSWE without AA = %v, want 68.5", got)
	}
	if got := LookupSWE(swe, "deepseek-v4.1-flash", "DeepSeek V4.1 Flash", "deepseek-v4-1-flash-1017"); got != 68.5 {
		t.Errorf("LookupSWE with dated AA slug = %v, want 68.5", got)
	}
	// Plan suffix stripped.
	if got := LookupSWE(swe, "muse-spark-1.3-contributor", "Muse Spark 1.3", ""); got != 71.2 {
		t.Errorf("LookupSWE contributor = %v, want 71.2", got)
	}
	// Falls back to AA slug when ID/name differ.
	if got := LookupSWE(swe, "beam-free", "Beam Free", "reflection-beam"); got != 54.0 {
		t.Errorf("LookupSWE via AA slug = %v, want 54.0", got)
	}
	if got := LookupSWE(nil, "deepseek-v4.1-flash", "DeepSeek V4.1 Flash", ""); got != 0 {
		t.Errorf("LookupSWE nil map = %v, want 0", got)
	}
}
