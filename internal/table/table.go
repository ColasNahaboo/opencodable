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

	"opencodable/internal/aa"
	"opencodable/internal/cost"
	"opencodable/internal/goplan"
	"opencodable/internal/modelsdev"
	"opencodable/internal/normalize"
	"opencodable/internal/probe"
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
	ReqsMonth     float64 `json:"reqs_mo"`
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
func Build(models []modelsdev.Model, aas []aa.Model, probes map[string]probe.Result, swe map[string]float64, plan string, peak bool) []Row {
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
		r := Row{Name: m.Name, ID: m.ID, PriceOut1M: m.Cost.Output}
		r.CostTask = cost.PerRequest(m, peak)
		allow, unlim := goplan.Allowance(m.ID, plan)
		if !goplan.Known(m.ID) && m.Free() {
			// Free variants may be missing from the plan snapshot; zero
			// prices still mean no per-model spend cap.
			allow, unlim = 0, true
		}
		r.AllowanceUSD, r.Unlimited = allow, unlim
		if !unlim && r.CostTask > 0 {
			r.ReqsMonth = allow / r.CostTask
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

func qual(r Row) float64 {
	if r.AAIntel > 0 {
		return r.AAIntel
	}
	return r.AACoding
}

func speed(r Row) float64 {
	if r.ProbeTokS > 0 {
		return r.ProbeTokS
	}
	return r.TokPerSec
}

// MarkPareto flags rows not dominated on (cost, speed, quality). It must be
// called after -paid/-privacy filtering: a hidden model should not dominate
// a visible one.
func MarkPareto(rows []Row) { markPareto(rows) }

// markPareto flags rows not dominated on (cost, speed, quality).
func markPareto(rows []Row) {
	for i := range rows {
		a := rows[i]
		if a.CostTask == 0 && speed(a) == 0 && qual(a) == 0 {
			continue // no data at all: never claim pareto
		}
		dominated := false
		for j := range rows {
			if i == j {
				continue
			}
			b := rows[j]
			leCost := b.CostTask <= a.CostTask
			geSpeed := speed(b) >= speed(a)
			geQual := qual(b) >= qual(a)
			strict := b.CostTask < a.CostTask || speed(b) > speed(a) || qual(b) > qual(a)
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
		"speed":   func(a, b Row) bool { return speed(a) > speed(b) },
		"quality": func(a, b Row) bool { return qual(a) > qual(b) },
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

// reqsRank makes unlimited models sort first on -sort reqs; ReqsMonth is 0
// for them, which would otherwise look like the worst value.
func reqsRank(r Row) float64 {
	if r.Unlimited {
		return math.Inf(1)
	}
	return r.ReqsMonth
}

// WriteTSV renders the table. On a terminal the columns are padded for
// readability, missing values show as "-" and a #-prefixed legend block
// explains each column. When stdout is piped, the legend is omitted and
// every row — header included — is strict single-TAB separated values with
// empty fields for missing data, for easy machine parsing. The "#name"
// header stays prefixed so it can never be confused with the "# " legend.
func WriteTSV(w io.Writer, rows []Row, tty bool) error {
	missing := ""
	if tty {
		missing = "-"
	}
	lines := make([]string, 0, len(rows)+1)
	lines = append(lines, strings.Join(headerCells(), "\t"))
	for _, r := range rows {
		lines = append(lines, strings.Join(rowCells(r, missing), "\t"))
	}
	if tty {
		// Legend prose is for humans only; keep piped output lean.
		for _, l := range legend {
			if _, err := fmt.Fprintln(w, l); err != nil {
				return err
			}
		}
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

// WriteVertical renders one model per block as "field: value" lines, blank
// line between records, empty fields skipped. Easier for screen readers and
// greppers than a wide table.
func WriteVertical(w io.Writer, rows []Row, tty bool) error {
	if tty {
		for _, l := range legend {
			if _, err := fmt.Fprintln(w, l); err != nil {
				return err
			}
		}
	}
	head := headerCells()
	for i, r := range rows {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		for j, v := range rowCells(r, "") {
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

func headerCells() []string {
	return []string{
		"#name", "cost_tok", "cost_task", "allowance", "reqs_mo",
		"tok_s", "ttft_s", "p_tok_s", "p_ttft_s",
		"aa_int", "aa_code", "aa_math",
		"code_b", "term_b", "if_b", "aa_lcr",
		"swe_b", "pareto", "src",
	}
}

// rowCells returns the field values of one row; missing becomes "-" on a
// terminal and the empty string when piped.
func rowCells(r Row, missing string) []string {
	pareto := ""
	if r.Pareto {
		pareto = "yes"
	}
	return []string{
		r.Name,
		numOr(r.PriceOut1M, true, missing), numOr(r.CostTask, true, missing),
		allowanceOr(r, missing), numOr(r.ReqsMonth, false, missing),
		numOr(r.TokPerSec, false, missing), numOr(r.TTFTSeconds, false, missing),
		numOr(r.ProbeTokS, false, missing), numOr(r.ProbeTTFT, false, missing),
		numOr(r.AAIntel, false, missing), numOr(r.AACoding, false, missing), numOr(r.AAMath, false, missing),
		numOr(r.LiveCodeBench, false, missing), numOr(r.TerminalBench, false, missing),
		numOr(r.IFBench, false, missing), numOr(r.LCR, false, missing), numOr(r.SWEBench, false, missing),
		pareto, r.Src,
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
	"# reqs_mo     allowance / cost_task = approx. requests per month",
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
	"# pareto      yes = no other model is cheaper-or-equal, faster-or-equal AND better-or-equal",
	"# src         aa:<effort> = speed from that AA reasoning-effort variant",
}

// WriteJSON dumps the rows machine-readably.
func WriteJSON(w io.Writer, rows []Row) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}
