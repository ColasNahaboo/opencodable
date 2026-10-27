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
	AAIntel       float64 `json:"aa_intelligence"`
	AACoding      float64 `json:"aa_coding"`
	LiveCodeBench float64 `json:"livecodebench"`
	TerminalBench float64 `json:"terminalbench"`
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

// Build joins everything and computes the Pareto flags.
func Build(models []modelsdev.Model, aas []aa.Model, plan string, peak bool) []Row {
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
			r.LiveCodeBench = f(a.Evaluations.LiveCode) * 100
			if a.Evaluations.Terminal != nil {
				r.TerminalBench = f(a.Evaluations.Terminal) * 100
			} else {
				r.TerminalBench = f(a.Evaluations.Term2) * 100
			}
			if suf, t := normalize.VariantWithSpeed(slug, speeds); suf != "" {
				r.TokPerSec = t
				r.variant = suf[1:]
			}
		}
		rows = append(rows, r)
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

func speed(r Row) float64 { return r.TokPerSec }

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

// WriteTSV renders the table; missing values print as "-".
func WriteTSV(w io.Writer, rows []Row) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "name\tprice_out_1M\tcost_task\tallowance_usd\treqs_month\ttok_s\tttft_s\taa_intelligence\taa_coding\tlivecodebench\tterminalbench\tpareto\tsource"); err != nil {
		return err
	}
	for _, r := range rows {
		src := ""
		if r.variant != "" {
			src = "aa:" + r.variant
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Name,
			num(r.PriceOut1M, true), num(r.CostTask, true),
			allowance(r), num(r.ReqsMonth, false),
			num(r.TokPerSec, false), num(r.TTFTSeconds, false),
			num(r.AAIntel, false), num(r.AACoding, false),
			num(r.LiveCodeBench, false), num(r.TerminalBench, false),
			yesno(r.Pareto), src)
	}
	return tw.Flush()
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
