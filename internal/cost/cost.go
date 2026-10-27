// Package cost prices one request against a model's rates.
package cost

import (
	"opencodable/internal/goplan"
	"opencodable/internal/modelsdev"
)

// PerRequest estimates the USD cost of one coding-agent request for the
// model, using the official Go token profile and base-tier prices
// (or peak prices when requested).
func PerRequest(m modelsdev.Model, peak bool) float64 {
	p := goplan.ProfileFor(m.ID)
	in, out, cached := m.Cost.Input, m.Cost.Output, m.Cost.CacheRead
	if peak {
		if pk, ok := goplan.PeakPrice(m.ID); ok {
			in, out, cached = pk[0], pk[1], pk[2]
		}
	}
	// Context tiers: the official profile stays far below tier thresholds,
	// so base-tier pricing is the right default for per-request cost.
	return (float64(p.Input)*in + float64(p.Cached)*cached + float64(p.Output)*out) / 1e6
}
