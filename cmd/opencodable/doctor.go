package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ColasNahaboo/opencodable/internal/aa"
	"github.com/ColasNahaboo/opencodable/internal/bench"
	"github.com/ColasNahaboo/opencodable/internal/cache"
	"github.com/ColasNahaboo/opencodable/internal/config"
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
	ProbeOK  bool     `json:"probe_ok"`
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
	Keys          map[string]string `json:"keys"` // env | config | dotfile | none
	ConfigUnknown []string          `json:"config_unknown,omitempty"`
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
				r.ProbeOK = true
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

// keySource reports where a key resolves from: env | config | dotfile | none.
func keySource(envName, dotfile string) string {
	if s := config.Source(envName); s != "" {
		return s
	}
	if hasFile(dotfile) {
		return "dotfile"
	}
	return "none"
}

// runDoctor audits plan/AA/SWE/probe coverage and prints where the joins are
// missing. It never runs a probe; it only reads cached measurements.
func runDoctor(refresh, offline, jsonOut, withLegend bool) error {
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
		Keys: map[string]string{
			"artificial_analysis": keySource("AA_API_KEY", ".artificialanalysis.key"),
			"opencode_go":         keySource("OPENCODE_API_KEY", ".opencode-go.key"),
		},
		ConfigUnknown: config.Unknown(),
		Rows:          rows,
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
		if r.ProbeOK {
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

	fmt.Fprintf(os.Stderr, "opencodable doctor %s — plan snapshot %s (%d entries)\n",
		report.Version, report.PlanSnapshot, report.PlanEntries)
	fmt.Fprintf(os.Stderr, "caches: models.dev %s | AA %s | SWE-bench %s | trusted probes %d\n",
		report.Caches["models.dev"], report.Caches["aa"], report.Caches["swe-bench"], report.TrustedProbes)
	fmt.Fprintf(os.Stderr, "keys: AA %s | OpenCode Go %s\n",
		report.Keys["artificial_analysis"], report.Keys["opencode_go"])
	for _, k := range report.ConfigUnknown {
		fmt.Fprintf(os.Stderr, "warning: opencodable.conf has unknown key %q (typo?)\n", k)
	}
	fmt.Fprintf(os.Stderr, "coverage: %d models | plan missing %d | free not in plan %d | AA missing %d | SWE missing %d\n",
		report.Models, report.MissingPlan, report.FreeNotInPlan, report.MissingAA, report.MissingSWE)
	if report.AAError != "" {
		fmt.Fprintf(os.Stderr, "warning: AA: %s\n", report.AAError)
	}
	if report.SWEError != "" {
		fmt.Fprintf(os.Stderr, "warning: SWE-bench: %s\n", report.SWEError)
	}

	return writeDoctor(os.Stdout, rows, isTTY(os.Stdout), withLegend)
}

// doctorLegend explains the columns; every line starts with "# " so parsers
// can skip it. It is printed only when -l is given.
var doctorLegend = []string{
	"# opencodable doctor — coverage audit of plan snapshot / AA / SWE-bench / probes",
	"# id        models.dev model ID",
	"# plan      yes = in plan snapshot | free = zero prices, no snapshot entry | missing = no entry, paid",
	"# aa        Artificial Analysis slug (- = no match)",
	"# swe_b     SWE-bench Verified % (- = none found)",
	"# privacy   safe | trains | unknown",
	"# probe     - | ok <age> | suspect | untrusted | error",
	"# warnings  ;-separated issues (- = none)",
}

// writeDoctor renders the audit table: an optional legend ("# "-prefixed,
// only with -l) then a "#id"-prefixed header. tty pads columns and shows "-"
// for missing values; piped output is strict single-TAB rows with empty
// fields.
func writeDoctor(w io.Writer, rows []doctorRow, tty, withLegend bool) error {
	if withLegend {
		for _, l := range doctorLegend {
			if _, err := fmt.Fprintln(w, l); err != nil {
				return err
			}
		}
	}
	missing := ""
	if tty {
		missing = "-"
	}
	out := io.Writer(w)
	var tw *tabwriter.Writer
	if tty {
		tw = tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		out = tw
	}
	if _, err := fmt.Fprintln(out, "#id\tplan\taa\tswe_b\tprivacy\tprobe\twarnings"); err != nil {
		return err
	}
	for _, r := range rows {
		swe := missing
		if r.SWE > 0 {
			swe = fmt.Sprintf("%.1f", r.SWE)
		}
		warn := missing
		if len(r.Warnings) > 0 {
			warn = strings.Join(r.Warnings, "; ")
		}
		if _, err := fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.ID, r.Plan, orMissing(r.AA, missing), swe,
			orMissing(r.Privacy, missing), orMissing(r.Probe, missing), warn); err != nil {
			return err
		}
	}
	if tw != nil {
		return tw.Flush()
	}
	return nil
}

func orMissing(v, missing string) string {
	if v == "" {
		return missing
	}
	return v
}
