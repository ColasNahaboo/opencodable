// Package cost prices one request against a model's rates.
package cost

import (
	"github.com/ColasNahaboo/opencodable/internal/goplan"
	"github.com/ColasNahaboo/opencodable/internal/modelsdev"
)

// PerRequest estimates the USD cost of one coding-agent request for the
// model, using the official Go token profile and base-tier prices
// (or peak prices when requested).
func PerRequest(m modelsdev.Model, peak bool) float64 {
	p := goplan.ProfileFor(m.ID)
	// Context tiers: the official profile stays far below tier thresholds,
	// so base-tier pricing is the right default for per-request cost.
	return FromTokens(m, p.Input, p.Cached, p.Output, 0, peak)
}

// FromTokens prices actual token usage at the model's rates. This is the
// allowance cost when the model's prices are the Go plan prices (the usual
// case in models.dev); cache writes are included when the catalog has a
// cache_write rate. Peak overrides apply to time-of-day models.
func FromTokens(m modelsdev.Model, input, cached, output, cacheWrite int64, peak bool) float64 {
	in, out, cr, cw := m.Cost.Input, m.Cost.Output, m.Cost.CacheRead, m.Cost.CacheWrite
	if peak {
		if pk, ok := goplan.PeakPrice(m.ID); ok {
			in, out, cr = pk[0], pk[1], pk[2]
		}
	}
	return (float64(input)*in + float64(cached)*cr + float64(output)*out + float64(cacheWrite)*cw) / 1e6
}
