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
