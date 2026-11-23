package goplan

import (
	"fmt"
	"io"
	"strings"

	"github.com/ColasNahaboo/opencodable/internal/modelsdev"
)

// Sanity gates: a scraped snapshot must pass every check before its numbers
// are trusted over the compiled-in snapshot. A mis-parse that shifts one
// column should trip at least one of these, not silently corrupt cost_task.

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
		if !r.Unlimited {
			if r.GoAllowance <= 0 {
				add("completeness: %s has no monthly allowance", id)
			}
			if r.CostReq <= 0 || r.CostReq > 1 {
				add("domain: %s cost per request %v outside (0,1]", id, r.CostReq)
			}
			if r.Reqs5h <= 0 || r.ReqsMo <= 0 {
				add("completeness: %s has no estimated requests", id)
			}
		}
		if !r.Unlimited && (r.GoAllowance < 5 || r.GoAllowance > 1000) {
			add("domain: %s allowance %v outside [5,1000]", id, r.GoAllowance)
		}
		if !r.Unlimited && (r.In < 0 || r.In > 50 || r.Out <= 0 || r.Out > 50 || r.CacheRead < 0 || r.CacheRead > 50) {
			add("domain: %s prices in=%v out=%v cr=%v outside (0,50]", id, r.In, r.Out, r.CacheRead)
		}
		// Internal arithmetic: the Go and Plus plans must agree on the
		// per-request cost implied by their own allowance and request
		// estimates (a shifted console column changes only one of them).
		if r.CostReq > 0 && r.Reqs5hPlus > 0 && r.PlusAllowance > 0 {
			plusCost := 0.2 * r.PlusAllowance / r.Reqs5hPlus
			ratio := plusCost / r.CostReq
			if ratio < 0.95 || ratio > 1.05 {
				add("requests: %s plus-plan cost %.6f vs go %.6f", id, plusCost, r.CostReq)
			}
		}
		// Cross-source: endpoint prices must agree with models.dev (same
		// rates, published twice).
		if m, ok := cat[id]; ok {
			if !closeEnough(m.Cost.Input, r.In) || !closeEnough(m.Cost.Output, r.Out) {
				add("catalog: %s endpoint in/out (%v,%v) != models.dev (%v,%v)", id, r.In, r.Out, m.Cost.Input, m.Cost.Output)
			}
		} else if !r.Unlimited {
			add("unknown model id: %s not in the models.dev catalog", id)
		}
		// Drift caps against the compiled-in snapshot (real plan changes
		// are stepwise; a shifted scrape row is not).
		if e, ok := table[id]; ok {
			if e.GoAllowance > 0 && r.GoAllowance > 0 && (r.GoAllowance > e.GoAllowance*4 || r.GoAllowance*4 < e.GoAllowance) {
				add("drift: %s allowance %v vs builtin %v", id, r.GoAllowance, e.GoAllowance)
			}
		}
		// Peak consistency: peak prices must equal base × endpoint factor.
		if r.Peak != nil && r.Factor > 1 {
			if !closeEnough(r.Peak[0], r.In*r.Factor) || !closeEnough(r.Peak[1], r.Out*r.Factor) {
				add("peak: %s peak %v != base×factor %v", id, *r.Peak, r.Factor)
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
