package bench

import "testing"

func TestParseLiveBench(t *testing.T) {
	raw := []byte(`model,long_agentic,reasoning,coding,browser,data_analysis,research,design
claude-haiku-5.5,40.0,82.4,91.4,69.5,53.1,81.9,83.1
deepseek-v4.1-flash-1017,40.9,89.1,96.0,67.2,79.5,79.5,83.4
muse-spark-1.3,25.5,89.7,90.2,30.9,79.6,68.6,74.8
blank,12.0,1.0,n/a,0.0,0.0,0.0,0.0
`)
	lb := parseLiveBench(raw)
	if len(lb) != 4 {
		t.Fatalf("got %d rows, want 4", len(lb))
	}
	if s, ok := lb.Lookup("claude-haiku-5-5"); !ok || s.Agentic != 40.0 || s.Coding != 91.4 {
		t.Errorf("claude-haiku-5-5 = %+v, %v", s, ok)
	}
	if s, ok := lb.Lookup("deepseek-v4.1-flash"); !ok || s.Coding != 96.0 {
		t.Errorf("deepseek snapshot alias = %+v, %v", s, ok)
	}
	if s, ok := lb.Lookup("muse-spark-1.3-contributor"); !ok || s.Agentic != 25.5 {
		t.Errorf("muse contributor alias = %+v, %v", s, ok)
	}
	if s, ok := lb.Lookup("blank"); !ok || s.Agentic != 12.0 || s.Coding != 0 {
		t.Errorf("n/a coding must be missing, got %+v, %v", s, ok)
	}
	if _, ok := lb.Lookup("not-there"); ok {
		t.Error("unknown model must not match")
	}
}

func TestParseLiveBenchBadInput(t *testing.T) {
	for _, raw := range []string{"", "not,a,csv\n1,2,3", "model,other\nm1,1"} {
		if got := parseLiveBench([]byte(raw)); got != nil {
			t.Errorf("parseLiveBench(%q) = %v, want nil", raw, got)
		}
	}
}
