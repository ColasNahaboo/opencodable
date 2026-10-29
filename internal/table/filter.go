package table

import "strings"

// QualityMetricNames lists the -min-quality metrics, in output order. They
// are output column titles, so filters and scripts share one vocabulary.
var QualityMetricNames = []string{
	"aa_int", "aa_code", "aa_math", "code_b", "term_b", "if_b",
	"aa_lcr", "swe_b", "lb_agent", "lb_code", "elo", "btr",
}

// qualityMetrics maps a metric name to its accessor. "quality" is the
// composite used by -sort quality and the Pareto flags: aa_int, else aa_code.
// elo and btr are filterable but join neither the composite nor Pareto,
// like the other benchmark columns.
var qualityMetrics = map[string]func(Row) float64{
	"aa_int":   func(r Row) float64 { return r.AAIntel },
	"aa_code":  func(r Row) float64 { return r.AACoding },
	"aa_math":  func(r Row) float64 { return r.AAMath },
	"code_b":   func(r Row) float64 { return r.LiveCodeBench },
	"term_b":   func(r Row) float64 { return r.TerminalBench },
	"if_b":     func(r Row) float64 { return r.IFBench },
	"aa_lcr":   func(r Row) float64 { return r.LCR },
	"swe_b":    func(r Row) float64 { return r.SWEBench },
	"lb_agent": func(r Row) float64 { return r.LBAgentic },
	"lb_code":  func(r Row) float64 { return r.LBCoding },
	"elo":      func(r Row) float64 { return r.Elo },
	"btr":      func(r Row) float64 { return r.BTRating },
	"quality":  Quality,
}

// IsQualityMetric reports whether name is a -min-quality metric, including
// the composite "quality" alias. Names are case-insensitive.
func IsQualityMetric(name string) bool {
	_, ok := qualityMetrics[strings.ToLower(name)]
	return ok
}

// qualityValue returns one metric's score and whether the name is known.
func qualityValue(r Row, metric string) (float64, bool) {
	fn, ok := qualityMetrics[strings.ToLower(metric)]
	if !ok {
		return 0, false
	}
	return fn(r), true
}

// Filter selects rows. The zero value keeps everything; pointer bounds
// distinguish "not set" from an explicit 0.
type Filter struct {
	Exclude     []string           // -exclude patterns
	MaxCost     *float64           // -max-cost: cost_task ceiling
	MinSpeed    *float64           // -min-speed: effective tok/s floor
	MinQuality  map[string]float64 // -min-quality: metric -> floor
	DropMissing bool               // -missing drop: remove rows with no value
}

// Excluded reports whether id or name matches an -exclude pattern. Patterns
// are case-insensitive: without * or ? they match anywhere in the string,
// otherwise they are globs (* = any run, ? = one character).
func (f Filter) Excluded(id, name string) bool {
	for _, p := range f.Exclude {
		if matchPattern(p, id) || matchPattern(p, name) {
			return true
		}
	}
	return false
}

func matchPattern(pattern, s string) bool {
	p := strings.ToLower(pattern)
	if strings.ContainsAny(p, "*?") {
		return matchGlob(p, strings.ToLower(s))
	}
	return strings.Contains(strings.ToLower(s), p)
}

// matchGlob is a minimal glob matcher: * matches any run of characters
// (including /, unlike path.Match), ? matches one. Everything else is
// literal, so model IDs with provider prefixes behave as users expect.
func matchGlob(pattern, s string) bool {
	p, str := []rune(pattern), []rune(s)
	pi, si := 0, 0
	star, mark := -1, 0
	for si < len(str) {
		switch {
		case pi < len(p) && (p[pi] == '?' || p[pi] == str[si]):
			pi++
			si++
		case pi < len(p) && p[pi] == '*':
			star, mark = pi, si
			pi++
		case star >= 0:
			pi = star + 1
			mark++
			si = mark
		default:
			return false
		}
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}

// CostOK reports whether usd passes -max-cost. cost_task is always known:
// free models have 0, so they pass a non-negative bound.
func (f Filter) CostOK(usd float64) bool {
	return f.MaxCost == nil || usd <= *f.MaxCost
}

// SpeedOK reports whether an effective speed passes -min-speed. A zero speed
// means no measurement, so -missing decides whether the row survives.
func (f Filter) SpeedOK(tokPerSec float64) bool {
	if f.MinSpeed == nil {
		return true
	}
	if tokPerSec == 0 {
		return !f.DropMissing
	}
	return tokPerSec >= *f.MinSpeed
}

// Keep reports whether r passes every numeric filter. Name exclusion is
// separate (Excluded) because -now reuses both halves.
func (f Filter) Keep(r Row) bool {
	if !f.CostOK(r.CostTask) || !f.SpeedOK(Speed(r)) {
		return false
	}
	for metric, min := range f.MinQuality {
		v, ok := qualityValue(r, metric)
		if !ok {
			continue // callers validate metric names; ignore here
		}
		if v == 0 { // no data
			if f.DropMissing {
				return false
			}
			continue
		}
		if v < min {
			return false
		}
	}
	return true
}
