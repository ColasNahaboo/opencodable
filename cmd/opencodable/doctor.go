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
	LBMatch  bool     `json:"lb_match"`
	EloMatch bool     `json:"elo_match"`
	BTRMatch bool     `json:"btr_match"`
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
	MissingLB     int               `json:"missing_lb"`
	MissingElo    int               `json:"missing_elo"`
	MissingBTR    int               `json:"missing_btr"`
	TrustedProbes int               `json:"trusted_probes"`
	Caches        map[string]string `json:"caches"`
	Keys          map[string]string `json:"keys"` // env | config | dotfile | none
	ConfigUnknown []string          `json:"config_unknown,omitempty"`
	AAError       string            `json:"aa_error,omitempty"`
	SWEError      string            `json:"swe_error,omitempty"`
	LBError       string            `json:"lb_error,omitempty"`
	EloError      string            `json:"elo_error,omitempty"`
	BTRError      string            `json:"btr_error,omitempty"`
	CacheWarnings []string          `json:"cache_warnings,omitempty"`
	Rows          []doctorRow       `json:"rows"`
}

// buildDoctorRows is the testable core of -doctor.
func buildDoctorRows(models []modelsdev.Model, aas []aa.Model, swe map[string]float64, lb bench.LB, elo bench.Elo, btr bench.BTR, probes map[string]probe.Result, probeAges map[string]time.Duration) []doctorRow {
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
		slug := normalize.Match(m.ID, aaSlugs)
		if slug != "" {
			r.AA = slug
		} else {
			r.Warnings = append(r.Warnings, "no AA match")
		}
		r.SWE = bench.LookupSWE(swe, m.ID, m.Name, slug)
		if _, ok := lb.Lookup(m.ID); ok {
			r.LBMatch = true
		}
		if _, ok := elo.Lookup(m.ID); ok {
			r.EloMatch = true
		} else if near := elo.Near(m.ID); near != "" {
			r.Warnings = append(r.Warnings, "no Elo match (alias? nearest "+near+")")
		}
		if _, ok := btr.Lookup(m.ID); ok {
			r.BTRMatch = true
		} else if near := btr.Near(m.ID); near != "" {
			r.Warnings = append(r.Warnings, "no BTR match (alias? nearest "+near+")")
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
func runDoctor(refresh, offline, aaPro, jsonOut, withLegend, nohead bool) error {
	models, err := modelsdev.Fetch("opencode-go", refresh, offline)
	if err != nil {
		return err
	}
	applyLivePlan(refresh, offline, models)
	aas, aerr := aa.Fetch(refresh, offline, aaPro)
	if aerr != nil {
		aas = nil
	}
	swe, serr := bench.SWEBench(refresh, offline)
	if serr != nil {
		swe = nil
	}
	lb, lerr := bench.LiveBench(refresh, offline)
	if lerr != nil {
		lb = nil
	}
	elo, eerr := bench.FetchElo(refresh, offline)
	if eerr != nil {
		elo = nil
	}
	btr, berr := bench.FetchBTRating(refresh, offline)
	if berr != nil {
		btr = nil
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
	rows := buildDoctorRows(models, aas, swe, lb, elo, btr, probes, ages)

	report := doctorReport{
		Version:      version,
		PlanSnapshot: goplan.Source(),
		PlanEntries:  goplan.Count(),
		Models:       len(rows),
		Caches: map[string]string{
			"models.dev": cacheAgeLabel("modelsdev-opencode-go.json"),
			"plan":       cacheAgeLabel("goplan-live.json"),
			"aa":         cacheAgeLabel(aa.CacheKey(aaPro)),
			"swe-bench":  cacheAgeLabel("swebench.json"),
			"livebench":  cacheAgeLabel(bench.LiveBenchCacheKey()),
			"elo":        cacheAgeLabel("lmarena-elo.json"),
			"btr":        cacheAgeLabel("lmarena-btr.json"),
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
		if r.SWE == 0 {
			report.MissingSWE++
		}
		if !r.LBMatch {
			report.MissingLB++
		}
		if !r.EloMatch {
			report.MissingElo++
		}
		if !r.BTRMatch {
			report.MissingBTR++
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
	if lerr != nil {
		report.LBError = lerr.Error()
	}
	if eerr != nil {
		report.EloError = eerr.Error()
	}
	if berr != nil {
		report.BTRError = berr.Error()
	}
	report.CacheWarnings = staleCacheWarnings(catalogAges(aaPro), aaPro)

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	fmt.Fprintf(os.Stderr, "opencodable doctor %s — plan snapshot %s (%d entries)\n",
		report.Version, report.PlanSnapshot, report.PlanEntries)
	fmt.Fprintf(os.Stderr, "caches: models.dev %s | AA %s | SWE-bench %s | LiveBench %s | Elo %s | BTR %s | trusted probes %d\n",
		report.Caches["models.dev"], report.Caches["aa"], report.Caches["swe-bench"],
		report.Caches["livebench"], report.Caches["elo"], report.Caches["btr"], report.TrustedProbes)
	for _, w := range report.CacheWarnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	fmt.Fprintf(os.Stderr, "keys: AA %s | OpenCode Go %s\n",
		report.Keys["artificial_analysis"], report.Keys["opencode_go"])
	for _, k := range report.ConfigUnknown {
		fmt.Fprintf(os.Stderr, "warning: opencodable.conf has unknown key %q (typo?)\n", k)
	}
	fmt.Fprintf(os.Stderr, "coverage: %d models | plan missing %d | free not in plan %d | AA missing %d | SWE missing %d | LiveBench missing %d | Elo missing %d | BTR missing %d\n",
		report.Models, report.MissingPlan, report.FreeNotInPlan, report.MissingAA, report.MissingSWE,
		report.MissingLB, report.MissingElo, report.MissingBTR)
	if report.AAError != "" {
		fmt.Fprintf(os.Stderr, "warning: AA: %s\n", report.AAError)
	}
	if report.SWEError != "" {
		fmt.Fprintf(os.Stderr, "warning: SWE-bench: %s\n", report.SWEError)
	}
	if report.LBError != "" {
		fmt.Fprintf(os.Stderr, "warning: LiveBench: %s\n", report.LBError)
	}
	if report.EloError != "" {
		fmt.Fprintf(os.Stderr, "warning: Elo: %s\n", report.EloError)
	}
	if report.BTRError != "" {
		fmt.Fprintf(os.Stderr, "warning: BTR: %s\n", report.BTRError)
	}

	return writeDoctor(os.Stdout, rows, isTTY(os.Stdout), withLegend, nohead)
}

// catalogCaches are the 24h caches doctor refreshes itself. The AA cache
// depends on the endpoint: Free and Pro responses have different shapes.
func catalogCaches(aaPro bool) []struct{ label, file string } {
	return []struct{ label, file string }{
		{"models.dev", "modelsdev-opencode-go.json"},
		{"aa", aa.CacheKey(aaPro)},
		{"swe-bench", "swebench.json"},
		{"livebench", bench.LiveBenchCacheKey()},
		{"elo", "lmarena-elo.json"},
		{"btr", "lmarena-btr.json"},
	}
}

// catalogTTL is the lifetime of those caches (modelsdev, aa and bench all use it).
const catalogTTL = 24 * time.Hour

// catalogAges returns the age of each catalog cache that exists.
func catalogAges(aaPro bool) map[string]time.Duration {
	out := map[string]time.Duration{}
	for _, c := range catalogCaches(aaPro) {
		if age, ok := cache.Age(c.file); ok {
			out[c.label] = age
		}
	}
	return out
}

// staleCacheWarnings names catalog caches that are missing or older than
// catalogTTL: the refresh failed, or -offline is in effect. Normal runs stay
// silent on these and serve the old copy; -doctor is where that is said out loud.
func staleCacheWarnings(ages map[string]time.Duration, aaPro bool) []string {
	var out []string
	for _, c := range catalogCaches(aaPro) {
		age, ok := ages[c.label]
		switch {
		case !ok:
			out = append(out, c.label+" cache missing: the fetch failed or -offline is in effect")
		case age > catalogTTL:
			out = append(out, fmt.Sprintf("%s cache is %s old (older than 24h): refresh failed or -offline", c.label, compactAge(age)))
		}
	}
	return out
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
// only with -l) then a "#id"-prefixed header (dropped by -nohead). tty pads
// columns and shows "-" for missing values; piped output is strict
// single-TAB rows with empty fields.
func writeDoctor(w io.Writer, rows []doctorRow, tty, withLegend, nohead bool) error {
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
	if !nohead {
		if _, err := fmt.Fprintln(out, "#id\tplan\taa\tswe_b\tprivacy\tprobe\twarnings"); err != nil {
			return err
		}
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
