// Package normalize maps models.dev model IDs to Artificial Analysis slugs.
package normalize

import "strings"

// aliases: models.dev ID → AA slug. Only needed where the plain
// lowercase-alnum key does not match.
var aliases = map[string]string{
	"step-5-preview":             "step-5", // AA tracks the base snapshot
	"mimo-v2.5":                  "mimo-v2-5-0424",
	"muse-spark-1.2-contributor": "muse-spark-1-2",
	"muse-spark-1.3-contributor": "muse-spark-1-3",
	"beam-free":                  "reflection-beam",
	"gemini-4-argon-contributor": "gemini-4-argon",
	"longcat-2.5-preview-free":   "longcat-2-0",
	"minimax-m3.1-flash-free":    "minimax-m3.1-flash",
	"step-5-preview-free":        "step-5",
	"deepseek-v4.1-flash":        "deepseek-v4-1-flash-1017", // dated snapshot = measured one
}

// effortSuffixes are AA reasoning-effort variants we may fall back to.
var effortSuffixes = []string{"-high", "-medium", "-low", "-xhigh"}

// Key reduces a model name to its comparable form.
func Key(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Match resolves a models.dev ID to an AA slug. Returns "" when unknown.
func Match(id string, aaSlugs map[string]bool) string {
	if aaSlugs[id] {
		return id
	}
	if s, ok := aliases[id]; ok && aaSlugs[s] {
		return s
	}
	// AA stores dots as dashes (qwen3.8-max → qwen3-8-max).
	dashed := strings.ReplaceAll(id, ".", "-")
	if aaSlugs[dashed] {
		return dashed
	}
	return ""
}

// VariantWithSpeed, when the base slug has no measured speed, picks the
// effort variant with the best tok/s (returned with its suffix label).
func VariantWithSpeed(base string, speeds map[string]float64) (string, float64) {
	if speeds[base] > 0 {
		return "", 0
	}
	best, bestT := "", 0.0
	for _, suf := range effortSuffixes {
		if t := speeds[base+suf]; t > bestT {
			best, bestT = suf, t
		}
	}
	return best, bestT
}
