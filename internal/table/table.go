// Package table joins the catalog, subscription and benchmark data into the
// output rows and renders them as TSV or JSON.
package table

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/ColasNahaboo/opencodable/internal/aa"
	"github.com/ColasNahaboo/opencodable/internal/bench"
	"github.com/ColasNahaboo/opencodable/internal/cost"
	"github.com/ColasNahaboo/opencodable/internal/goplan"
	"github.com/ColasNahaboo/opencodable/internal/modelsdev"
	"github.com/ColasNahaboo/opencodable/internal/normalize"
	"github.com/ColasNahaboo/opencodable/internal/probe"
)

// Row is one output line: one subscription model with everything joined.
// JSON keys match the TSV column titles (see TITLES.md), plus the `id` and
// `unlimited` extras.
type Row struct {
	Name          string  `json:"name"`
	ID            string  `json:"id"`
	PriceOut1M    float64 `json:"cost_tok"`
	CostTask      float64 `json:"cost_task"`
	AllowanceUSD  float64 `json:"allowance"`
	Unlimited     bool    `json:"unlimited"`
	RQuota        float64 `json:"r_quota"`
	TokPerSec     float64 `json:"tok_s"`
	TTFTSeconds   float64 `json:"ttft_s"`
	ProbeTokS     float64 `json:"p_tok_s"`
	ProbeTTFT     float64 `json:"p_ttft_s"`
	AAIntel       float64 `json:"aa_int"`
	AACoding      float64 `json:"aa_code"`
	AAMath        float64 `json:"aa_math"`
	LiveCodeBench float64 `json:"code_b"`
	TerminalBench float64 `json:"term_b"`
	IFBench       float64 `json:"if_b"`
	LCR           float64 `json:"aa_lcr"`
	SWEBench      float64 `json:"swe_b"`
	Pareto        bool    `json:"pareto"`
	Src           string  `json:"src,omitempty"`
	Privacy       string  `json:"privacy"`
	CtxK          float64 `json:"ctx_k"`
	OutK          float64 `json:"out_k"`
	LBAgentic     float64 `json:"lb_agent"`
	LBCoding      float64 `json:"lb_code"`
	Elo           float64 `json:"elo"`
	BTRating      float64 `json:"btr"`
}

func f(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// Build joins everything. Call MarkPareto after the caller has applied its
// row filters, so the flags describe the set the user actually sees.
// Measured probe results, when present, take precedence over reported AA
// speed.
func Build(models []modelsdev.Model, aas []aa.Model, probes map[string]probe.Result, swe map[string]float64, lb bench.LB, elo bench.Elo, btr bench.BTR, plan string, peak bool) []Row {
	slugs := map[string]bool{}
	speeds := map[string]float64{}
	bySlug := map[string]aa.Model{}
	for _, a := range aas {
		slugs[a.Slug] = true
		if a.TokPerSec > speeds[a.Slug] {
			speeds[a.Slug] = a.TokPerSec
		}
		bySlug[a.Slug] = a
	}

	var rows []Row
	for _, m := range models {
		r := Row{
			Name: m.Name, ID: m.ID, PriceOut1M: m.Cost.Output,
			Privacy: goplan.PrivacyLabel(m.ID),
			CtxK:    float64(m.Limit.Context) / 1000,
			OutK:    float64(m.Limit.Output) / 1000,
		}
		r.CostTask = cost.PerRequest(m, peak)
		allow, unlim := goplan.Allowance(m.ID, plan)
		if !goplan.Known(m.ID) && m.Free() {
			// Free variants may be missing from the plan snapshot; zero
			// prices still mean no per-model spend cap.
			allow, unlim = 0, true
		}
		r.AllowanceUSD, r.Unlimited = allow, unlim
		if !unlim && r.CostTask > 0 {
			r.RQuota = allow / r.CostTask
		}
		if s, ok := lb.Lookup(m.ID); ok {
			r.LBAgentic, r.LBCoding = s.Agentic, s.Coding
		}
		if v, ok := elo.Lookup(m.ID); ok {
			r.Elo = v
		}
		if v, ok := btr.Lookup(m.ID); ok {
			r.BTRating = v
		}
		if slug := normalize.Match(m.ID, slugs); slug != "" {
			a := bySlug[slug]
			r.TokPerSec = speeds[slug]
			r.TTFTSeconds = a.TTFTSeconds
			r.AAIntel = a.Evaluations.Intel
			r.AACoding = f(a.Evaluations.Coding)
			r.AAMath = f(a.Evaluations.Math)
			r.LiveCodeBench = f(a.Evaluations.LiveCode) * 100
			if a.Evaluations.Terminal != nil {
				r.TerminalBench = f(a.Evaluations.Terminal) * 100
			} else {
				r.TerminalBench = f(a.Evaluations.Term2) * 100
			}
			r.IFBench = f(a.Evaluations.IFBench) * 100
			r.LCR = f(a.Evaluations.LCR) * 100
			r.SWEBench = swe[normalize.Key(a.Slug)]
			if suf, t := normalize.VariantWithSpeed(slug, speeds); suf != "" {
				r.TokPerSec = t
				r.Src = "aa:" + suf[1:]
			}
		}
		rows = append(rows, r)
	}
	for i := range rows {
		if p, ok := probes[rows[i].ID]; ok && p.Error == "" {
			// Distrust throughput from too-short or implausible measurements.
			if p.Trusted() {
				rows[i].ProbeTokS = p.TokPerSec
			}
			rows[i].ProbeTTFT = p.TTFTSeconds
		}
	}
	return rows
}

func Quality(r Row) float64 {
	if r.AAIntel > 0 {
		return r.AAIntel
	}
	return r.AACoding
}

func Speed(r Row) float64 {
	if r.ProbeTokS > 0 {
		return r.ProbeTokS
	}
	return r.TokPerSec
}

// MarkPareto flags rows not dominated on (cost, speed, quality). It must be
// called after every row filter: a hidden model should not dominate
// a visible one.
func MarkPareto(rows []Row) { markPareto(rows) }

// markPareto flags rows not dominated on (cost, speed, quality).
func markPareto(rows []Row) {
	for i := range rows {
		a := rows[i]
		if a.CostTask == 0 && Speed(a) == 0 && Quality(a) == 0 {
			continue // no data at all: never claim pareto
		}
		dominated := false
		for j := range rows {
			if i == j {
				continue
			}
			b := rows[j]
			leCost := b.CostTask <= a.CostTask
			geSpeed := Speed(b) >= Speed(a)
			geQual := Quality(b) >= Quality(a)
			strict := b.CostTask < a.CostTask || Speed(b) > Speed(a) || Quality(b) > Quality(a)
			if leCost && geSpeed && geQual && strict {
				dominated = true
				break
			}
		}
		a.Pareto = !dominated
		rows[i] = a
	}
}

// Sort orders rows in place. key: name|price|speed|quality|reqs. The model
// ID is the final tie-breaker so identical inputs always produce identical
// output (models.dev is a map, so its natural order is random).
func Sort(rows []Row, key string) {
	less := map[string]func(a, b Row) bool{
		"name":    func(a, b Row) bool { return a.Name < b.Name },
		"price":   func(a, b Row) bool { return a.CostTask < b.CostTask },
		"speed":   func(a, b Row) bool { return Speed(a) > Speed(b) },
		"quality": func(a, b Row) bool { return Quality(a) > Quality(b) },
		"reqs":    func(a, b Row) bool { return reqsRank(a) > reqsRank(b) },
	}[key]
	if less == nil {
		less = func(a, b Row) bool { return a.Name < b.Name }
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if less(rows[i], rows[j]) {
			return true
		}
		if less(rows[j], rows[i]) {
			return false
		}
		return rows[i].ID < rows[j].ID
	})
}

// reqsRank makes unlimited models sort first on -sort reqs; RQuota is 0
// for them, which would otherwise look like the worst value.
func reqsRank(r Row) float64 {
	if r.Unlimited {
		return math.Inf(1)
	}
	return r.RQuota
}

// WriteTSV renders the table. sel names the visible columns in view order
// (nil selects all); sorting, Pareto flags and filters are computed on full
// rows before projection. nohead drops the header row (data rows only;
// the -l legend still prints). On a terminal the columns are padded for
// readability and missing values show as "-". The #-prefixed legend block is
// printed only when withLegend is set (-l). When stdout is piped, every row —
// header included — is strict single-TAB separated values with empty fields
// for missing data. The "#name" header stays prefixed so it can never be
// confused with the "# " legend.
func WriteTSV(w io.Writer, rows []Row, sel []string, tty, withLegend, nohead bool) error {
	if sel == nil {
		sel = HeaderCells()
	}
	missing := ""
	if tty {
		missing = "-"
	}
	full := HeaderCells()
	recs := make([][]string, 0, len(rows))
	for _, r := range rows {
		recs = append(recs, ProjectCells(full, RowCells(r, missing), sel))
	}
	return writeTabbed(w, sel, recs, tty, FilterLegend(legend, sel), withLegend, nohead)
}

// WriteVertical renders one model per block as "field: value" lines, blank
// line between records, empty fields skipped. The field names are the legend,
// so the prose block is only printed when withLegend is set (-l).
func WriteVertical(w io.Writer, rows []Row, sel []string, withLegend bool) error {
	if sel == nil {
		sel = HeaderCells()
	}
	full := HeaderCells()
	recs := make([][]string, 0, len(rows))
	for _, r := range rows {
		recs = append(recs, ProjectCells(full, RowCells(r, ""), sel))
	}
	return writeVertical(w, sel, recs, FilterLegend(legend, sel), withLegend)
}

// writeTabbed renders a header plus records, padded on a terminal and strict
// single-TAB when piped. The legend block is opt-in via withLegend.
func writeTabbed(w io.Writer, head []string, recs [][]string, tty bool, legendLines []string, withLegend, nohead bool) error {
	if withLegend {
		for _, l := range legendLines {
			if _, err := fmt.Fprintln(w, l); err != nil {
				return err
			}
		}
	}
	lines := make([]string, 0, len(recs)+1)
	if !nohead {
		lines = append(lines, strings.Join(head, "\t"))
	}
	for _, r := range recs {
		lines = append(lines, strings.Join(r, "\t"))
	}
	if tty {
		tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		for _, l := range lines {
			if _, err := fmt.Fprintln(tw, l); err != nil {
				return err
			}
		}
		return tw.Flush()
	}
	for _, l := range lines {
		if _, err := fmt.Fprintln(w, l); err != nil {
			return err
		}
	}
	return nil
}

// writeVertical renders one record per block as "field: value" lines.
func writeVertical(w io.Writer, head []string, recs [][]string, legendLines []string, withLegend bool) error {
	if withLegend {
		for _, l := range legendLines {
			if _, err := fmt.Fprintln(w, l); err != nil {
				return err
			}
		}
	}
	for i, r := range recs {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		for j, v := range r {
			if v == "" {
				continue
			}
			if _, err := fmt.Fprintf(w, "%s: %s\n", strings.TrimPrefix(head[j], "#"), v); err != nil {
				return err
			}
		}
	}
	return nil
}

// WriteFullTSV renders -full rows; legendLines is the caller-built legend
// (main legend plus -now-only lines), filtered to the visible columns.
// nohead drops the header row.
func WriteFullTSV(w io.Writer, rows []FullRow, sel []string, tty, withLegend bool, legendLines []string, nohead bool) error {
	if sel == nil {
		sel = FullTitles()
	}
	missing := ""
	if tty {
		missing = "-"
	}
	full := FullTitles()
	recs := make([][]string, 0, len(rows))
	for _, r := range rows {
		recs = append(recs, ProjectCells(full, FullCells(r, missing), sel))
	}
	return writeTabbed(w, sel, recs, tty, FilterLegend(legendLines, sel), withLegend, nohead)
}

// WriteFullVertical renders one -full model per block.
func WriteFullVertical(w io.Writer, rows []FullRow, sel []string, withLegend bool, legendLines []string) error {
	if sel == nil {
		sel = FullTitles()
	}
	full := FullTitles()
	recs := make([][]string, 0, len(rows))
	for _, r := range rows {
		recs = append(recs, ProjectCells(full, FullCells(r, ""), sel))
	}
	return writeVertical(w, sel, recs, FilterLegend(legendLines, sel), withLegend)
}

func HeaderCells() []string {
	return []string{
		"#name", "cost_tok", "cost_task", "allowance", "r_quota",
		"tok_s", "ttft_s", "p_tok_s", "p_ttft_s",
		"aa_int", "aa_code", "aa_math",
		"code_b", "term_b", "if_b", "aa_lcr",
		"swe_b", "pareto", "src", "privacy", "ctx_k", "out_k",
		"lb_agent", "lb_code", "elo", "btr",
	}
}

// RowCells returns the field values of one row; missing becomes "-" on a
// terminal and the empty string when piped.
func RowCells(r Row, missing string) []string {
	pareto := ""
	if r.Pareto {
		pareto = "yes"
	}
	privacy := r.Privacy
	if privacy == "" {
		privacy = missing
	}
	return []string{
		r.Name,
		numOr(r.PriceOut1M, true, missing), numOr(r.CostTask, true, missing),
		allowanceOr(r, missing), numOr(r.RQuota, false, missing),
		numOr(r.TokPerSec, false, missing), numOr(r.TTFTSeconds, false, missing),
		numOr(r.ProbeTokS, false, missing), numOr(r.ProbeTTFT, false, missing),
		numOr(r.AAIntel, false, missing), numOr(r.AACoding, false, missing), numOr(r.AAMath, false, missing),
		numOr(r.LiveCodeBench, false, missing), numOr(r.TerminalBench, false, missing),
		numOr(r.IFBench, false, missing), numOr(r.LCR, false, missing), numOr(r.SWEBench, false, missing),
		pareto, r.Src, privacy,
		numOr(r.CtxK, false, missing), numOr(r.OutK, false, missing),
		numOr(r.LBAgentic, false, missing), numOr(r.LBCoding, false, missing),
		arenaOr(r.Elo, missing), arenaOr(r.BTRating, missing),
	}
}

func numOr(v float64, money bool, missing string) string {
	if v == 0 {
		return missing
	}
	if money {
		s := strconv.FormatFloat(v, 'f', 5, 64)
		s = strings.TrimRight(s, "0")
		return "$" + strings.TrimSuffix(s, ".")
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

// Num formats one numeric cell: money renders $-prefixed with trailing zeros
// trimmed, plain numbers one decimal; zero becomes the missing marker. The
// -now view shares it so all views format identically.
func Num(v float64, money bool) string { return numOr(v, money, "") }

// arenaOr formats leaderboard scores: Elo-scale values as integers (1525),
// small Bradley-Terry-scale values with 3 decimals (0.026); zero is missing,
// like every other metric.
func arenaOr(v float64, missing string) string {
	if v == 0 {
		return missing
	}
	if v > -10 && v < 10 {
		return strconv.FormatFloat(v, 'f', 3, 64)
	}
	return strconv.Itoa(int(math.Round(v)))
}

func allowanceOr(r Row, missing string) string {
	if r.Unlimited {
		return "unlimited"
	}
	if r.AllowanceUSD == 0 {
		return missing
	}
	return numOr(r.AllowanceUSD, true, missing)
}

// legend documents each column, one #-prefixed line per column.
var legend = []string{
	"# opencodable — OpenCode Go models ranked by cost / speed / quality",
	"# name        model display name (models.dev catalog)",
	"# cost_tok    retail USD per 1M output tokens",
	"# cost_task   estimated USD per coding-agent request (OpenCode official token profile, base tier)",
	"# allowance   monthly usage budget of this model in the plan (unlimited = free models)",
	"# r_quota     monthly request quota: allowance / cost_task, approx. requests per month",
	"# tok_s       Artificial Analysis median output tok/s (reported, other hosts)",
	"# ttft_s      AA median seconds to first token, thinking included (same)",
	"# p_tok_s     MEASURED on your subscription via -probe (trusted windows only; suspect/too-short dropped)",
	"# p_ttft_s    MEASURED seconds to first content token via -probe (thinking time included in the wait)",
	"# aa_int      Artificial Analysis Intelligence Index (composite)",
	"# aa_code     Artificial Analysis Coding Index",
	"# aa_math     Artificial Analysis Math Index",
	"# code_b      LiveCodeBench pass rate % (via AA)",
	"# term_b      Terminal-Bench pass rate % (via AA)",
	"# if_b        IFBench instruction-following pass rate % (via AA)",
	"# aa_lcr      AA Long-Context Reasoning pass rate %",
	"# swe_b       SWE-bench Verified % resolved, best recorded agent run (swebench.com)",
	"# pareto      yes = no other visible model is cheaper-or-equal, faster-or-equal AND better-or-equal",
	"# src         aa:<effort> = speed from that AA reasoning-effort variant",
	"# privacy     safe = no training on request data; trains = contributor; unknown = no plan-snapshot verdict",
	"# ctx_k       context window in thousands of tokens (models.dev)",
	"# out_k       maximum output tokens in thousands (models.dev)",
	"# lb_agent    LiveBench 2.0 Long Agentic score (single run, 0-100)",
	"# lb_code     LiveBench 2.0 Coding score (single run, 0-100)",
	"# elo         Arena Elo, daily snapshot of the arena.ai text leaderboard (third-party mirror; top-20, can lag; missing = not in snapshot)",
	"# btr         Arena Bradley-Terry rating on the Elo scale, text with style control, overall category (official mirror, fresh; missing = no match)",
}

// MainLegend returns the main-table legend lines, one #-prefixed line per
// column after the prose header.
func MainLegend() []string { return legend }

// rowMap converts a row struct to its JSON object.
func rowMap(v any) map[string]any {
	b, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

// selectMap keeps the selected columns plus the named JSON-only extras
// (id, unlimited, known, ...), which travel alongside every selection.
func selectMap(m map[string]any, sel []string, extras ...string) map[string]any {
	keep := map[string]bool{}
	for _, t := range sel {
		keep[strings.ToLower(strings.TrimPrefix(t, "#"))] = true
	}
	for _, e := range extras {
		keep[e] = true
	}
	out := make(map[string]any, len(keep))
	for k, v := range m {
		if keep[k] {
			out[k] = v
		}
	}
	return out
}

// WriteJSON dumps the rows machine-readably. sel names the visible columns
// (nil selects all); JSON-only extras (id, unlimited) are always emitted and
// keys equal the column titles, without the # prefix.
func WriteJSON(w io.Writer, rows []Row, sel []string) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if sel == nil {
		return enc.Encode(rows)
	}
	maps := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		maps = append(maps, selectMap(rowMap(r), sel, "id", "unlimited"))
	}
	return enc.Encode(maps)
}

// WriteFullJSON dumps -full rows; extras (id, unlimited, known, spent_known,
// window_estimate) are always emitted alongside the selected columns.
func WriteFullJSON(w io.Writer, rows []FullRow, sel []string) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if sel == nil {
		return enc.Encode(rows)
	}
	maps := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		maps = append(maps, selectMap(rowMap(r), sel,
			"id", "unlimited", "known", "spent_known", "window_estimate"))
	}
	return enc.Encode(maps)
}
