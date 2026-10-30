package goplan

import (
	"fmt"
	"io"
	"strings"

	"github.com/ColasNahaboo/opencodable/internal/modelsdev"
)

// Sanity gates: a scraped page must pass every check before its numbers are
// trusted over the compiled-in snapshot. A mis-parse that shifts one column
// should trip at least one of these, not silently corrupt cost_task.

func errJoin(errs []string) string { return strings.Join(errs, "; ") }

func jsonReadAll(r io.Reader) ([]byte, error) { return io.ReadAll(r) }

// Validate checks a merged snapshot against itself, the models.dev catalog
// and the compiled-in table. It returns every violation; an empty slice means
// the snapshot may be used.
func Validate(snap *LiveSnapshot, catalog []modelsdev.Model) []string {
	var errs []string
	add := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }

	if len(snap.Rows) < 10 {
		add("rows: snapshot has only %d models", len(snap.Rows))
		return errs
	}
	cat := map[string]modelsdev.Model{}
	for _, m := range catalog {
		cat[m.ID] = m
	}

	for id, r := range snap.Rows {
		if r.GoAllowance <= 0 && !r.Unlimited {
			add("completeness: %s has no monthly allowance", id)
		}
		if !r.Unlimited && (r.GoAllowance < 5 || r.GoAllowance > 1000) {
			add("domain: %s allowance %v outside [5,1000]", id, r.GoAllowance)
		}
		if !r.Unlimited && (r.In < 0 || r.In > 50 || r.Out <= 0 || r.Out > 50 || r.CacheRead < 0 || r.CacheRead > 50) {
			add("domain: %s prices in=%v out=%v cr=%v outside (0,50]", id, r.In, r.Out, r.CacheRead)
		}
		if r.Profile != nil {
			p := *r.Profile
			if p.Input < 100 || p.Input > 10000 || p.Cached > 200000 || p.Output < 50 || p.Output > 2000 {
				add("domain: %s profile %+v outside token bounds", id, p)
			}
		}
		// Cross-source: docs prices must agree with models.dev (same rates,
		// published twice).
		if m, ok := cat[id]; ok {
			if !closeEnough(m.Cost.Input, r.In) || !closeEnough(m.Cost.Output, r.Out) {
				add("catalog: %s docs in/out (%v,%v) != models.dev (%v,%v)", id, r.In, r.Out, m.Cost.Input, m.Cost.Output)
			}
		} else if !r.Unlimited {
			add("unknown model id: %s not in the models.dev catalog", id)
		}
		// Internal arithmetic: allowance ÷ cost_task must land near the
		// docs' requests-per-month figure. The published figures are
		// rounded estimates (up to ~30% off exact), so this gate only
		// catches gross mis-parses such as shifted columns.
		if r.ReqsMo > 0 && !r.Unlimited && r.Profile != nil {
			cost := (float64(r.Profile.Input)*r.In + float64(r.Profile.Cached)*r.CacheRead + float64(r.Profile.Output)*r.Out) / 1e6
			if cost > 0 {
				ratio := (r.GoAllowance / cost) / r.ReqsMo
				if ratio < 0.67 || ratio > 1.5 {
					add("requests: %s allowance/cost %.0f vs docs figure %.0f", id, r.GoAllowance/cost, r.ReqsMo)
				}
			}
		}
		// Drift caps against the compiled-in snapshot (real plan changes
		// are stepwise; a shifted scrape row is not).
		if e, ok := table[id]; ok {
			if e.GoAllowance > 0 && r.GoAllowance > 0 && (r.GoAllowance > e.GoAllowance*4 || r.GoAllowance*4 < e.GoAllowance) {
				add("drift: %s allowance %v vs builtin %v", id, r.GoAllowance, e.GoAllowance)
			}
			if e.Profile != nil && r.Profile != nil && *e.Profile != *r.Profile {
				if driftProfile(*e.Profile, *r.Profile) {
					add("drift: %s profile %+v vs builtin %+v", id, *r.Profile, *e.Profile)
				}
			}
		}
		// Peak consistency: docs peak prices must equal base × endpoint factor.
		if r.Peak != nil && r.Factor > 1 {
			if !closeEnough(r.Peak[0], r.In*r.Factor) || !closeEnough(r.Peak[1], r.Out*r.Factor) {
				add("peak: %s docs peak %v != base×factor %v", id, *r.Peak, r.Factor)
			}
		}
	}

	changed := 0
	for id := range table {
		if _, ok := snap.Rows[id]; !ok {
			changed++
		}
	}
	for id := range snap.Rows {
		if _, ok := table[id]; !ok {
			changed++
		}
	}
	if changed*10 > len(table)*3 { // >30% turnover
		add("drift: %d of %d models added/removed vs builtin", changed, len(table))
	}
	return errs
}

func closeEnough(a, b float64) bool {
	d := absF(a - b)
	m := a
	if b > m {
		m = b
	}
	if m == 0 {
		return d < 1e-9
	}
	return d/m <= 0.01
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// driftProfile flags a profile that moved more than ~2× on any axis.
func driftProfile(a, b Profile) bool {
	ratio := func(x, y int64) bool {
		if x == 0 || y == 0 {
			return x != y
		}
		hi, lo := float64(x), float64(y)
		if lo > hi {
			hi, lo = lo, hi
		}
		return hi/lo > 2
	}
	return ratio(a.Input, b.Input) || ratio(a.Cached, b.Cached) || ratio(a.Output, b.Output)
}
