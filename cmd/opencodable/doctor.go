package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/ColasNahaboo/opencodable/internal/aa"
	"github.com/ColasNahaboo/opencodable/internal/bench"
	"github.com/ColasNahaboo/opencodable/internal/cache"
	"github.com/ColasNahaboo/opencodable/internal/goplan"
	"github.com/ColasNahaboo/opencodable/internal/modelsdev"
	"github.com/ColasNahaboo/opencodable/internal/normalize"
	"github.com/ColasNahaboo/opencodable/internal/probe"
)

// doctorRow is one model's join/coverage audit.
type doctorRow struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Plan     string   `json:"plan"` // yes | free | missing
	AA       string   `json:"aa,omitempty"`
	SWE      float64  `json:"swe_b"`
	Privacy  string   `json:"privacy"`
	CtxK     float64  `json:"ctx_k"`
	OutK     float64  `json:"out_k"`
	Probe    string   `json:"probe"` // - | ok <age> | suspect | untrusted | error
	Warnings []string `json:"warnings,omitempty"`
}

// doctorReport is the machine-readable -doctor output.
type doctorReport struct {
	Version       string            `json:"version"`
	PlanSnapshot  string            `json:"plan_snapshot"`
	PlanEntries   int               `json:"plan_entries"`
	Models        int               `json:"models"`
	MissingPlan   int               `json:"missing_plan"`
	FreeNotInPlan int               `json:"free_not_in_plan"`
	MissingAA     int               `json:"missing_aa"`
	MissingSWE    int               `json:"missing_swe"`
	TrustedProbes int               `json:"trusted_probes"`
	Caches        map[string]string `json:"caches"`
	Keys          map[string]bool   `json:"keys"`
	AAError       string            `json:"aa_error,omitempty"`
	SWEError      string            `json:"swe_error,omitempty"`
	Rows          []doctorRow       `json:"rows"`
}

// buildDoctorRows is the testable core of -doctor.
func buildDoctorRows(models []modelsdev.Model, aas []aa.Model, swe map[string]float64, probes map[string]probe.Result, probeAges map[string]time.Duration) []doctorRow {
	aaSlugs := map[string]bool{}
	for _, a := range aas {
		aaSlugs[a.Slug] = true
	}
	rows := make([]doctorRow, 0, len(models))
	for _, m := range models {
		r := doctorRow{
			ID: m.ID, Name: m.Name, Privacy: goplan.PrivacyLabel(m.ID),
			CtxK: float64(m.Limit.Context) / 1000, OutK: float64(m.Limit.Output) / 1000,
			Probe: "-",
		}
		switch {
		case goplan.Known(m.ID):
			r.Plan = "yes"
		case m.Free():
			r.Plan = "free"
			r.Warnings = append(r.Warnings, "free variant not in plan snapshot")
		default:
			r.Plan = "missing"
			r.Warnings = append(r.Warnings, "no plan entry")
		}
		if slug := normalize.Match(m.ID, aaSlugs); slug != "" {
			r.AA = slug
			r.SWE = swe[normalize.Key(slug)]
		} else {
			r.Warnings = append(r.Warnings, "no AA match")
		}
		if p, ok := probes[m.ID]; ok {
			age := compactAge(probeAges[m.ID])
			switch {
			case p.Error != "":
				r.Probe = "error"
				r.Warnings = append(r.Warnings, "probe error")
			case p.Suspect:
				r.Probe = "suspect"
				r.Warnings = append(r.Warnings, "suspect probe")
			case !p.Trusted():
				r.Probe = "untrusted"
				r.Warnings = append(r.Warnings, "untrusted probe")
			default:
				r.Probe = "ok " + age
			}
		}
		rows = append(rows, r)
	}
	return rows
}

// compactAge keeps doctor output narrow: 42s, 17m, 3h, 2d.
func compactAge(d time.Duration) string {
	switch {
	case d <= 0:
		return "now"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func cacheAgeLabel(key string) string {
	age, ok := cache.Age(key)
	if !ok {
		return "missing"
	}
	return compactAge(age)
}

func hasFile(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func aaKeyPresent() bool {
	return os.Getenv("AA_API_KEY") != "" || hasFile(".artificialanalysis.key")
}

func goKeyPresent() bool {
	_, err := probe.Key()
	return err == nil
}

// runDoctor audits plan/AA/SWE/probe coverage and prints where the joins are
// missing. It never runs a probe; it only reads cached measurements.
func runDoctor(refresh, offline, jsonOut bool) error {
	models, err := modelsdev.Fetch("opencode-go", refresh, offline)
	if err != nil {
		return err
	}
	aas, aerr := aa.Fetch(refresh, offline)
	if aerr != nil {
		aas = nil
	}
	swe, serr := bench.SWEBench(refresh, offline)
	if serr != nil {
		swe = nil
	}

	probes := map[string]probe.Result{}
	ages := map[string]time.Duration{}
	for _, m := range models {
		if p, ok := cachedProbe(m.ID); ok {
			probes[m.ID] = p
			if age, ok := cache.Age("probe-" + m.ID + ".json"); ok {
				ages[m.ID] = age
			}
		}
	}
	rows := buildDoctorRows(models, aas, swe, probes, ages)

	report := doctorReport{
		Version:      version,
		PlanSnapshot: goplan.SnapshotDate,
		PlanEntries:  goplan.Count(),
		Models:       len(rows),
		Caches: map[string]string{
			"models.dev": cacheAgeLabel("modelsdev-opencode-go.json"),
			"aa":         cacheAgeLabel("aa-models.json"),
			"swe-bench":  cacheAgeLabel("swebench.json"),
		},
		Keys: map[string]bool{
			"artificial_analysis": aaKeyPresent(),
			"opencode_go":         goKeyPresent(),
		},
		Rows: rows,
	}
	for _, r := range rows {
		switch r.Plan {
		case "missing":
			report.MissingPlan++
		case "free":
			report.FreeNotInPlan++
		}
		if r.AA == "" {
			report.MissingAA++
		}
		if r.AA != "" && r.SWE == 0 {
			report.MissingSWE++
		}
		if len(r.Probe) > 3 && r.Probe[:3] == "ok " {
			report.TrustedProbes++
		}
	}
	if aerr != nil {
		report.AAError = aerr.Error()
	}
	if serr != nil {
		report.SWEError = serr.Error()
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	fmt.Fprintf(os.Stdout, "opencodable doctor %s — plan snapshot %s (%d entries)\n",
		report.Version, report.PlanSnapshot, report.PlanEntries)
	fmt.Fprintf(os.Stdout, "caches: models.dev %s | AA %s | SWE-bench %s | trusted probes %d\n",
		report.Caches["models.dev"], report.Caches["aa"], report.Caches["swe-bench"], report.TrustedProbes)
	fmt.Fprintf(os.Stdout, "keys: AA %s | OpenCode Go %s\n",
		yesNo(report.Keys["artificial_analysis"]), yesNo(report.Keys["opencode_go"]))
	fmt.Fprintf(os.Stdout, "coverage: %d models | plan missing %d | free not in plan %d | AA missing %d | SWE missing %d\n",
		report.Models, report.MissingPlan, report.FreeNotInPlan, report.MissingAA, report.MissingSWE)
	if report.AAError != "" {
		fmt.Fprintf(os.Stdout, "warning: AA: %s\n", report.AAError)
	}
	if report.SWEError != "" {
		fmt.Fprintf(os.Stdout, "warning: SWE-bench: %s\n", report.SWEError)
	}
	fmt.Fprintln(os.Stdout)

	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "id\tplan\taa\tswe_b\tprivacy\tprobe\twarnings")
	for _, r := range rows {
		swe := "-"
		if r.SWE > 0 {
			swe = fmt.Sprintf("%.1f", r.SWE)
		}
		aaSlug := r.AA
		if aaSlug == "" {
			aaSlug = "-"
		}
		warn := "-"
		if len(r.Warnings) > 0 {
			warn = ""
			for i, w := range r.Warnings {
				if i > 0 {
					warn += "; "
				}
				warn += w
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.ID, r.Plan, aaSlug, swe, r.Privacy, r.Probe, warn)
	}
	return tw.Flush()
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
