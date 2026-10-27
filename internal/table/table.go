// Package table joins the catalog, subscription and benchmark data into the
// output rows and renders them as TSV or JSON.
package table

import (
	"encoding/json"
	"fmt"
	"io"
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
type Row struct {
	Name          string  `json:"name"`
	ID            string  `json:"id"`
	PriceOut1M    float64 `json:"price_out_1M"`
	CostTask      float64 `json:"cost_task"`
	AllowanceUSD  float64 `json:"allowance_usd"`
	Unlimited     bool    `json:"unlimited"`
	ReqsMonth     float64 `json:"reqs_month"`
	TokPerSec     float64 `json:"tok_s"`
	TTFTSeconds   float64 `json:"ttft_s"`
	ProbeTokS     float64 `json:"probe_tok_s"`
	ProbeTTFT     float64 `json:"probe_ttft_s"`
	AAIntel       float64 `json:"aa_intelligence"`
	AACoding      float64 `json:"aa_coding"`
	AAMath        float64 `json:"aa_math"`
	LiveCodeBench float64 `json:"livecodebench"`
	TerminalBench float64 `json:"terminalbench"`
	IFBench       float64 `json:"ifbench"`
	LCR           float64 `json:"aa_lcr"`
	SWEBench      float64 `json:"swebench_verified"`
	Pareto        bool    `json:"pareto"`
	// set when the speed came from an AA effort variant
	variant string
}

func f(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// Build joins everything and computes the Pareto flags. Measured probe
// results, when present, take precedence over reported AA speed.
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
				r.variant = suf[1:]
			}
		}
		rows = append(rows, r)
	}
	for i := range rows {
		if p, ok := probes[rows[i].ID]; ok && p.Error == "" {
			// Distrust throughput from too-short measurement windows.
			if p.WindowSeconds >= probe.MinWindow {
				rows[i].ProbeTokS = p.TokPerSec
			}
			rows[i].ProbeTTFT = p.TTFTSeconds
		}
	}
	markPareto(rows)
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

// Sort orders rows in place. key: name|price|speed|quality|reqs.
func Sort(rows []Row, key string) {
	less := map[string]func(a, b Row) bool{
		"name":    func(a, b Row) bool { return a.Name < b.Name },
		"price":   func(a, b Row) bool { return a.CostTask < b.CostTask },
		"speed":   func(a, b Row) bool { return speed(b) < speed(a) },
		"quality": func(a, b Row) bool { return qual(b) < qual(a) },
		"reqs":    func(a, b Row) bool { return b.ReqsMonth < a.ReqsMonth },
	}[key]
	if less == nil {
		less = func(a, b Row) bool { return a.Name < b.Name }
	}
	sort.SliceStable(rows, func(i, j int) bool { return less(rows[i], rows[j]) })
}

// WriteTSV renders the table; missing values print as "-". A legend block of
// #-prefixed lines explains every column.
func WriteTSV(w io.Writer, rows []Row) error {
	for _, l := range legend {
		if _, err := fmt.Fprintln(w, l); err != nil {
			return err
		}
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "#name\tprice_out_1M\tcost_task\tallowance_usd\treqs_month\ttok_s\tttft_s\tprobe_tok_s\tprobe_ttft_s\taa_intelligence\taa_coding\taa_math\tlivecodebench\tterminalbench\tifbench\taa_lcr\tswebench_verified\tpareto\tsource"); err != nil {
		return err
	}
	for _, r := range rows {
		src := ""
		if r.variant != "" {
			src = "aa:" + r.variant
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Name,
			num(r.PriceOut1M, true), num(r.CostTask, true),
			allowance(r), num(r.ReqsMonth, false),
			num(r.TokPerSec, false), num(r.TTFTSeconds, false),
			num(r.ProbeTokS, false), num(r.ProbeTTFT, false),
			num(r.AAIntel, false), num(r.AACoding, false), num(r.AAMath, false),
			num(r.LiveCodeBench, false), num(r.TerminalBench, false),
			num(r.IFBench, false), num(r.LCR, false), num(r.SWEBench, false),
			yesno(r.Pareto), src)
	}
	return tw.Flush()
}

// legend documents each column, one #-prefixed line per column.
var legend = []string{
	"# opencodable — OpenCode Go models ranked by price / speed / quality",
	"# name             model display name (models.dev catalog)",
	"# price_out_1M     retail USD per 1M output tokens",
	"# cost_task        estimated USD per coding-agent request (OpenCode official token profile, base tier)",
	"# allowance_usd    monthly usage budget of this model in the plan (unlimited = free models)",
	"# reqs_month       allowance_usd / cost_task = approx. requests per month",
	"# tok_s            Artificial Analysis median output tok/s (reported, other hosts)",
	"# ttft_s           AA median seconds to first token, thinking included (same)",
	"# probe_tok_s      MEASURED on your subscription via -probe (tok/s over the generation window)",
	"# probe_ttft_s     MEASURED seconds to first generated token via -probe",
	"# aa_intelligence  Artificial Analysis Intelligence Index (composite)",
	"# aa_coding        Artificial Analysis Coding Index",
	"# aa_math          Artificial Analysis Math Index",
	"# livecodebench    LiveCodeBench pass rate % (via AA)",
	"# terminalbench    Terminal-Bench pass rate % (via AA)",
	"# ifbench          IFBench instruction-following pass rate % (via AA)",
	"# aa_lcr           AA Long-Context Reasoning pass rate %",
	"# swebench_verified SWE-bench Verified % resolved, best recorded agent run (swebench.com)",
	"# pareto           yes = no other model is cheaper-or-equal, faster-or-equal AND better-or-equal",
	"# source           aa:<effort> = speed from that AA reasoning-effort variant",
}

// WriteJSON dumps the rows machine-readably.
func WriteJSON(w io.Writer, rows []Row) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

func allowance(r Row) string {
	if r.Unlimited {
		return "unlimited"
	}
	if r.AllowanceUSD == 0 {
		return "-"
	}
	return num(r.AllowanceUSD, true)
}

func num(v float64, money bool) string {
	if v == 0 {
		return "-"
	}
	if money {
		s := strconv.FormatFloat(v, 'f', 5, 64)
		s = strings.TrimRight(s, "0")
		return "$" + strings.TrimSuffix(s, ".")
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

func yesno(b bool) string {
	if b {
		return "yes"
	}
	return ""
}
