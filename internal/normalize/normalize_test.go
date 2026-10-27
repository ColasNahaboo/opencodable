package normalize

import "testing"

func TestKey(t *testing.T) {
	cases := map[string]string{
		"DeepSeek V4.1 Flash": "deepseekv41flash",
		"GLM-5.3-Flash":       "glm53flash",
		"Kimi K3.1 Pro":       "kimik31pro",
	}
	for in, want := range cases {
		if got := Key(in); got != want {
			t.Errorf("Key(%q)=%q want %q", in, got, want)
		}
	}
}

func TestMatch(t *testing.T) {
	slugs := map[string]bool{
		"glm-5-3": true, "step-5": true, "mimo-v2-5-0424": true,
		"muse-spark-1-3": true, "qwen3-8-max": true, "longcat-2-0": true,
	}
	cases := map[string]string{ // models.dev ID → expected AA slug
		"glm-5.3":                    "glm-5-3", // dashed direct hit
		"step-5-preview":             "step-5",
		"mimo-v2.5":                  "mimo-v2-5-0424",
		"muse-spark-1.3-contributor": "muse-spark-1-3",
		"qwen3.8-max":                "qwen3-8-max",
		"longcat-2.5-preview-free":   "longcat-2-0",
	}
	for id, want := range cases {
		if got := Match(id, slugs); got != want {
			t.Errorf("Match(%q)=%q want %q", id, got, want)
		}
	}
	if got := Match("space-bunny", slugs); got != "" {
		t.Errorf("Match(space-bunny)=%q want \"\"", got)
	}
}

func TestVariantWithSpeed(t *testing.T) {
	speeds := map[string]float64{
		"kimi-k3": 0, "kimi-k3-high": 40, "kimi-k3-low": 30,
		"gpt-6-luna": 120,
	}
	if suf, t2 := VariantWithSpeed("kimi-k3", speeds); suf != "-high" || t2 != 40 {
		t.Errorf("variant = %q %v, want -high 40", suf, t2)
	}
	if suf, _ := VariantWithSpeed("gpt-6-luna", speeds); suf != "" {
		t.Errorf("base has speed, want no variant, got %q", suf)
	}
}
