package main

import (
	"encoding/json"
	"time"

	"github.com/ColasNahaboo/opencodable/internal/cache"
	"github.com/ColasNahaboo/opencodable/internal/cost"
	"github.com/ColasNahaboo/opencodable/internal/goplan"
	"github.com/ColasNahaboo/opencodable/internal/modelsdev"
)

// windowSpend is estimated allowance dollars already spent in the 5h and
// weekly windows. Zero means a fresh window (or no data).
type windowSpend struct {
	five float64
	week float64
}

// ledgerProbe is one fresh probe's allowance burn: our own telemetry,
// timestamped, so it attributes exactly.
type ledgerProbe struct {
	TS    string  `json:"ts"`
	Model string  `json:"model"`
	USD   float64 `json:"usd"`
}

// ledgerModel is one model's period totals at snapshot time.
type ledgerModel struct {
	USD  float64 `json:"usd"`
	Reqs int64   `json:"reqs"`
}

// ledgerSnapshot is the previous run's per-model period totals. Deltas
// against it attribute spend to the elapsed interval.
type ledgerSnapshot struct {
	TS     string                 `json:"ts"`
	Models map[string]ledgerModel `json:"models"`
}

// spendLedger is the local spend history. Snapshots keep exactly one
// previous run (deltas need no more); probes prune past retention.
type spendLedger struct {
	Prev   *ledgerSnapshot `json:"prev,omitempty"`
	Probes []ledgerProbe   `json:"probes,omitempty"`
}

// ledgerRetention bounds probe history. Snapshots never accumulate: each
// run replaces the previous one.
const ledgerRetention = 35 * 24 * time.Hour

// weekWindow is the assumed weekly window length. The endpoint reports only
// resets_at, so a rolling 7d is assumed and documented; erring inclusive
// (see attributeLedger) keeps the bias conservative.
const weekWindow = 7 * 24 * time.Hour

const fiveWindow = 5 * time.Hour

// loadLedger reads the local spend history; a missing or corrupt file is an
// empty ledger, never an error.
func loadLedger() spendLedger {
	b, ok := cache.Peek("spend-ledger.json")
	if !ok {
		return spendLedger{}
	}
	var led spendLedger
	if err := json.Unmarshal(b, &led); err != nil {
		return spendLedger{}
	}
	return led
}

// saveLedger persists the history. A write failure is silently dropped:
// the ledger sharpens estimates but must never break a run.
func saveLedger(led spendLedger) {
	b, err := json.Marshal(led)
	if err != nil {
		return
	}
	_ = cache.Store("spend-ledger.json", b)
}

// recordSnapshot replaces the previous totals with the current run's.
// Only real data (never assumed-$0 runs): snapshotting zeros would forge
// deltas on the next run.
func recordSnapshot(led spendLedger, spend map[string]spendInfo, now time.Time) spendLedger {
	snap := &ledgerSnapshot{TS: now.UTC().Format(time.RFC3339), Models: map[string]ledgerModel{}}
	for id, si := range spend {
		snap.Models[id] = ledgerModel{USD: si.ledgerUSD(), Reqs: si.reqs}
	}
	led.Prev = snap
	led.Probes = pruneProbes(led.Probes, now)
	return led
}

// recordProbeBurn appends one fresh probe's allowance burn. Cached reads
// never reach here (callers gate on fresh), so nothing double-counts.
func recordProbeBurn(led spendLedger, model string, usd float64, now time.Time) spendLedger {
	led.Probes = append(led.Probes, ledgerProbe{TS: now.UTC().Format(time.RFC3339), Model: model, USD: usd})
	led.Probes = pruneProbes(led.Probes, now)
	return led
}

func pruneProbes(in []ledgerProbe, now time.Time) []ledgerProbe {
	cut := now.Add(-ledgerRetention)
	out := in[:0]
	for _, p := range in {
		ts, err := time.Parse(time.RFC3339, p.TS)
		if err != nil || ts.Before(cut) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// overlapFrac is the fraction of (a0,a1] covered by (b0,b1]: uniform-rate
// proration of spend deltas onto window overlaps. Zero on clock skew.
func overlapFrac(a0, a1, b0, b1 time.Time) float64 {
	lo := maxTime(a0, b0)
	hi := minTime(a1, b1)
	den := a1.Sub(a0)
	if den <= 0 || !hi.After(lo) {
		return 0
	}
	return float64(hi.Sub(lo)) / float64(den)
}

// attributeLedger converts history into per-model window spend (allowance
// dollars). Probes are point events attributed in full when inside the
// window; snapshot deltas prorate by overlap, assuming a uniform spend rate
// over the interval. A negative delta means the period totals reset between
// runs: that model is skipped and the snapshot rebase (on save) heals it.
// Bounds err inclusive (over-attribute), so remaining counts err
// conservative, never optimistic.
func attributeLedger(led spendLedger, cur map[string]spendInfo, now, reset5h, resetWk time.Time) map[string]windowSpend {
	out := map[string]windowSpend{}
	add := func(id string, five, week float64) {
		v := out[id]
		v.five += five
		v.week += week
		out[id] = v
	}
	for _, p := range led.Probes {
		ts, err := time.Parse(time.RFC3339, p.TS)
		if err != nil || ts.After(now) {
			continue
		}
		if ts.After(reset5h.Add(-fiveWindow)) {
			add(p.Model, p.USD, 0)
		}
		if ts.After(resetWk.Add(-weekWindow)) {
			add(p.Model, 0, p.USD)
		}
	}
	if led.Prev == nil {
		return out
	}
	prevTS, err := time.Parse(time.RFC3339, led.Prev.TS)
	if err != nil || !now.After(prevTS) {
		return out
	}
	f5 := overlapFrac(prevTS, now, reset5h.Add(-fiveWindow), now)
	fW := overlapFrac(prevTS, now, resetWk.Add(-weekWindow), now)
	if f5 == 0 && fW == 0 {
		return out
	}
	for id, si := range cur {
		prev := led.Prev.Models[id].USD // absent = $0 then (console rows exist iff spend exists)
		if d := si.ledgerUSD() - prev; d > 0 {
			add(id, d*f5, d*fW)
		}
	}
	return out
}

// estimateWindowSpend attributes 5h/week window spend per model, in allowance
// dollars. Ledger-attributed dollars come first; the unattributed remainder
// (implied by the aggregate endpoint) splits monthly-proportionally over the
// capped, spend-known models. The universe is known-limit models only:
// unknown-limit rows show no counts anyway, and every remainder clamps at
// zero. usedFive/usedWeek are aggregate used fractions, clamped to [0,1].
// It returns the spends and whether any ledger dollar contributed
// ("ledger" vs "proportional").
func estimateWindowSpend(models []modelsdev.Model, spend map[string]spendInfo, spendKnown bool, planName string, usedFive, usedWeek float64, attr map[string]windowSpend) (map[string]windowSpend, string) {
	type caps struct {
		five, week float64
	}
	universe := map[string]caps{}
	var baseFive, baseWeek float64
	for _, m := range models {
		if !goplan.Known(m.ID) {
			continue
		}
		allow, unlimited := goplan.Allowance(m.ID, planName)
		if unlimited || cost.PerRequest(m, false) <= 0 {
			continue
		}
		c := caps{five: 0.20 * allow, week: 0.50 * allow}
		universe[m.ID] = c
		baseFive += c.five
		baseWeek += c.week
	}
	if usedFive < 0 {
		usedFive = 0
	}
	if usedFive > 1 {
		usedFive = 1
	}
	if usedWeek < 0 {
		usedWeek = 0
	}
	if usedWeek > 1 {
		usedWeek = 1
	}
	impliedFive, impliedWeek := usedFive*baseFive, usedWeek*baseWeek
	weights := map[string]float64{}
	var totalWeight, attrSum float64
	for id := range universe {
		si := spend[id]
		if si.known || spendKnown {
			w := si.ledgerUSD()
			weights[id] = w
			totalWeight += w
		}
	}
	remFive, remWeek := impliedFive, impliedWeek
	for id, a := range attr {
		if _, ok := universe[id]; ok {
			remFive -= a.five
			remWeek -= a.week
			attrSum += a.five + a.week
		}
	}
	if remFive < 0 {
		remFive = 0
	}
	if remWeek < 0 {
		remWeek = 0
	}
	out := make(map[string]windowSpend, len(universe))
	for id := range universe {
		v := windowSpend{five: attr[id].five, week: attr[id].week}
		if totalWeight > 0 {
			v.five += remFive * weights[id] / totalWeight
			v.week += remWeek * weights[id] / totalWeight
		}
		out[id] = v
	}
	if attrSum > 0 {
		return out, "ledger"
	}
	return out, "proportional"
}
