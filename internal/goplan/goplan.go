// Package goplan holds the OpenCode Go subscription data that models.dev does
// not carry: per-model monthly allowances (USD-equivalent usage budget), the
// official estimated token profile per request, and peak-hour price overrides.
//
// Source: https://opencode.ai/v2/docs/console/go/ (public pricing tables,
// verified 2026-10-27).
package goplan

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
}

// allowances by models.dev model ID.
var table = map[string]entry{
	"glm-5.3-flash":              {60, 180, &Profile{1000, 55000, 200}, nil},
	"glm-5.3":                    {15, 120, &Profile{700, 52000, 150}, nil},
	"glm-5.2":                    {60, 180, &Profile{700, 52000, 150}, nil},
	"kimi-k3.1-pro":              {15, 60, &Profile{1050, 76500, 300}, nil},
	"kimi-k3":                    {15, 60, &Profile{1050, 76500, 300}, nil},
	"kimi-k2.7-code":             {60, 180, &Profile{870, 55000, 200}, nil},
	"longcat-2.0":                {60, 240, &Profile{920, 88900, 200}, nil},
	"step-5-preview":             {15, 60, &Profile{1000, 55000, 200}, nil},
	"beam-free":                  {0, 0, nil, nil},
	"iceman-contributor-free":    {0, 0, nil, nil},
	"mimo-v2.6-flash":            {60, 120, &Profile{830, 71500, 295}, nil},
	"mimo-v2.6-pro":              {15, 60, &Profile{790, 86000, 305}, nil},
	"mimo-v2.5":                  {60, 120, &Profile{830, 71500, 295}, nil},
	"mimo-v2.5-pro":              {15, 60, &Profile{790, 86000, 305}, nil},
	"minimax-m3.1-flash":         {20, 80, &Profile{500, 49400, 360}, nil},
	"minimax-m3":                 {60, 180, &Profile{510, 56000, 190}, nil},
	"muse-spark-1.3-contributor": {60, 120, &Profile{620, 71400, 300}, nil},
	"muse-spark-1.2-contributor": {60, 120, &Profile{620, 71400, 300}, nil},
	"qwen3.8-max":                {15, 60, &Profile{420, 66000, 200}, nil},
	"qwen3.8-flash":              {30, 90, &Profile{600, 58000, 200}, nil},
	"qwen3.7-plus":               {60, 180, &Profile{500, 57000, 190}, nil},
	"deepseek-v4.1-flash":        {80, 160, &Profile{410, 71300, 310}, &[3]float64{0.30, 1.20, 0.006}},
	"deepseek-v4-pro":            {15, 60, &Profile{750, 82000, 290}, &[3]float64{1.32, 3.96, 0.044}},
	"hy4-preview":                {30, 120, &Profile{830, 71500, 295}, nil},
	"hy3":                        {60, 240, &Profile{830, 71500, 295}, nil},
	"grok-4.7":                   {15, 60, &Profile{390, 32500, 120}, nil},
	"gpt-6-luna":                 {15, 60, &Profile{1000, 50000, 220}, nil},
	"gpt-5.6-luna":               {15, 60, &Profile{1000, 50000, 220}, nil},
	"claude-haiku-5-5":           {15, 60, &Profile{1100, 55000, 240}, nil},
	"gemini-4-argon":             {30, 120, nil, nil},
}

// GenericProfile is used when a model has no official estimate.
var GenericProfile = Profile{2000, 100000, 500}

// Allowance returns the monthly usage budget for the plan ("go"|"go-plus").
// unlimited=true when the model has no per-model cap.
func Allowance(id, plan string) (usd float64, unlimited bool) {
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
	if e, ok := table[id]; ok && e.Profile != nil {
		return *e.Profile
	}
	return GenericProfile
}

// PeakPrice returns the peak-hours [input, output, cache_read] prices, if any.
func PeakPrice(id string) ([3]float64, bool) {
	if e, ok := table[id]; ok && e.Peak != nil {
		return *e.Peak, true
	}
	return [3]float64{}, false
}
