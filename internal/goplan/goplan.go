// Package goplan holds the OpenCode Go subscription data that models.dev does
// not carry: per-model monthly allowances (USD-equivalent usage budget), the
// official estimated token profile per request, and peak-hour price overrides.
//
// Source: https://opencode.ai/v2/docs/console/go/ (public pricing tables,
// verified 2026-10-27).
package goplan

import (
	"fmt"
	"strings"
)

// Profile is OpenCode's estimated token consumption of one coding-agent
// request for a given model.
type Profile struct {
	Input  int64
	Cached int64
	Output int64
}

type entry struct {
	// Monthly usage allowance per model, USD-equivalent. 0 = unlimited.
	GoAllowance     float64
	GoPlusAllowance float64
	// Official per-request token estimate; nil = use the generic profile.
	Profile *Profile
	// Peak-price override (USD/1M), for models billed by time of day.
	Peak *[3]float64 // input, output, cache_read
	// TrainsOnData: the model trains on request data (OpenCode privacy table).
	TrainsOnData bool
}

// SnapshotDate is the date the plan data below was verified against the
// public docs; -doctor prints it.
const SnapshotDate = "2026-10-27"

// allowances by models.dev model ID.
var table = map[string]entry{
	"glm-5.3-flash":              {60, 180, &Profile{1000, 55000, 200}, nil, false},
	"glm-5.3":                    {15, 120, &Profile{700, 52000, 150}, nil, false},
	"glm-5.2":                    {60, 180, &Profile{700, 52000, 150}, nil, false},
	"kimi-k3.1-pro":              {15, 60, &Profile{1050, 76500, 300}, nil, false},
	"kimi-k3":                    {15, 60, &Profile{1050, 76500, 300}, nil, false},
	"kimi-k2.7-code":             {60, 180, &Profile{870, 55000, 200}, nil, false},
	"longcat-2.0":                {60, 240, &Profile{920, 88900, 200}, nil, false},
	"step-5-preview":             {15, 60, &Profile{1000, 55000, 200}, nil, false},
	"beam-free":                  {0, 0, nil, nil, false},
	"iceman-contributor-free":    {0, 0, nil, nil, true},
	"mimo-v2.6-flash":            {60, 120, &Profile{830, 71500, 295}, nil, false},
	"mimo-v2.6-pro":              {15, 60, &Profile{790, 86000, 305}, nil, false},
	"mimo-v2.5":                  {60, 120, &Profile{830, 71500, 295}, nil, false},
	"mimo-v2.5-pro":              {15, 60, &Profile{790, 86000, 305}, nil, false},
	"minimax-m3.1-flash":         {20, 80, &Profile{500, 49400, 360}, nil, false},
	"minimax-m3":                 {60, 180, &Profile{510, 56000, 190}, nil, false},
	"muse-spark-1.3-contributor": {60, 120, &Profile{620, 71400, 300}, nil, true},
	"muse-spark-1.2-contributor": {60, 120, &Profile{620, 71400, 300}, nil, true},
	"qwen3.8-max":                {15, 60, &Profile{420, 66000, 200}, nil, false},
	"qwen3.8-flash":              {30, 90, &Profile{600, 58000, 200}, nil, false},
	"qwen3.7-plus":               {60, 180, &Profile{500, 57000, 190}, nil, false},
	"deepseek-v4.1-flash":        {80, 160, &Profile{410, 71300, 310}, &[3]float64{0.30, 1.20, 0.006}, false},
	"deepseek-v4-pro":            {15, 60, &Profile{750, 82000, 290}, &[3]float64{1.32, 3.96, 0.044}, false},
	"hy4-preview":                {30, 120, &Profile{830, 71500, 295}, nil, false},
	"hy3":                        {60, 240, &Profile{830, 71500, 295}, nil, false},
	"grok-4.7":                   {15, 60, &Profile{390, 32500, 120}, nil, false},
	"gpt-6-luna":                 {15, 60, &Profile{1000, 50000, 220}, nil, false},
	"gpt-5.6-luna":               {15, 60, &Profile{1000, 50000, 220}, nil, false},
	"claude-haiku-5-5":           {15, 60, &Profile{1100, 55000, 240}, nil, false},
	"gemini-4-argon":             {30, 120, nil, nil, false},
}

// GenericProfile is used when a model has no official estimate.
var GenericProfile = Profile{2000, 100000, 500}

// live holds a validated live snapshot overriding the compiled-in table;
// nil means "use the built-in snapshot only".
var live *LiveSnapshot

// Use installs (or with nil, removes) the live plan snapshot overlay.
func Use(snap *LiveSnapshot) { live = snap }

// Source describes which plan snapshot is active, for -doctor.
func Source() string {
	if live != nil {
		return fmt.Sprintf("live (%s, %d models)", live.FetchedAt.Format("2006-01-02"), len(live.Rows))
	}
	return "builtin (" + SnapshotDate + ")"
}

// liveRow returns the overlay row for id, if any.
func liveRow(id string) *LiveRow {
	if live != nil {
		return live.Rows[id]
	}
	return nil
}

// Known reports whether the model has an entry in the plan snapshot.
// Unknown models are not the same as free models: their allowance is
// unknown, not zero.
func Known(id string) bool {
	if r := liveRow(id); r != nil {
		return true
	}
	_, ok := table[id]
	return ok
}

// Count returns the number of models in the active plan snapshot.
func Count() int {
	if live != nil && len(live.Rows) > 0 {
		return len(live.Rows)
	}
	return len(table)
}

// Allowance returns the monthly usage budget for the plan ("go"|"go-plus").
// unlimited=true when the model has no per-model cap.
func Allowance(id, plan string) (usd float64, unlimited bool) {
	if r := liveRow(id); r != nil {
		a := r.GoAllowance
		if plan == "go-plus" {
			a = r.PlusAllowance
		}
		if r.Unlimited || a == 0 {
			return 0, true
		}
		return a, false
	}
	e, ok := table[id]
	if !ok {
		return 0, false
	}
	a := e.GoAllowance
	if plan == "go-plus" {
		a = e.GoPlusAllowance
	}
	if a == 0 {
		return 0, true
	}
	return a, false
}

// ProfileFor returns the official per-request token profile.
func ProfileFor(id string) Profile {
	if r := liveRow(id); r != nil && r.Profile != nil {
		return *r.Profile
	}
	if e, ok := table[id]; ok && e.Profile != nil {
		return *e.Profile
	}
	return GenericProfile
}

// PeakPrice returns the peak-hours [input, output, cache_read] prices, if any.
func PeakPrice(id string) ([3]float64, bool) {
	if r := liveRow(id); r != nil && r.Peak != nil {
		return *r.Peak, true
	}
	if e, ok := table[id]; ok && e.Peak != nil {
		return *e.Peak, true
	}
	return [3]float64{}, false
}

// Privacy reports whether the model trains on request data and whether the
// snapshot knows the answer. Unknown "contributor" models count as training
// by convention; other unknown IDs have no privacy verdict.
func Privacy(id string) (trains, known bool) {
	if e, ok := table[id]; ok {
		return e.TrainsOnData, true
	}
	if strings.Contains(id, "contributor") {
		return true, true
	}
	return false, false
}

// PrivacyLabel is the human/TSV form of Privacy: safe | trains | unknown.
func PrivacyLabel(id string) string {
	trains, known := Privacy(id)
	switch {
	case !known:
		return "unknown"
	case trains:
		return "trains"
	default:
		return "safe"
	}
}

// ExcludedByPrivacy reports whether -privacy should hide the model: it
// trains on request data, or its privacy status is unknown (fail closed).
func ExcludedByPrivacy(id string) bool {
	trains, known := Privacy(id)
	return trains || !known
}

// TrainsOnData reports whether the model trains on request data. It is the
// non-fail-closed predicate; use ExcludedByPrivacy for the -privacy filter.
func TrainsOnData(id string) bool {
	trains, _ := Privacy(id)
	return trains
}
