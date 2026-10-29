// Command opencodable ranks the models available in an OpenCode Go
// subscription by price, speed and quality: what is the best compromise
// right now.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ColasNahaboo/opencodable/internal/aa"
	"github.com/ColasNahaboo/opencodable/internal/bench"
	"github.com/ColasNahaboo/opencodable/internal/cache"
	"github.com/ColasNahaboo/opencodable/internal/config"
	"github.com/ColasNahaboo/opencodable/internal/consoleapi"
	"github.com/ColasNahaboo/opencodable/internal/cost"
	"github.com/ColasNahaboo/opencodable/internal/goplan"
	"github.com/ColasNahaboo/opencodable/internal/modelsdev"
	"github.com/ColasNahaboo/opencodable/internal/probe"
	"github.com/ColasNahaboo/opencodable/internal/table"
	"github.com/ColasNahaboo/opencodable/internal/usage"
)

// version is displayed at the bottom of the -h help text.
var version = "v0.1.18"

// colList accumulates comma-separated -cols/-excols values. The flag may be
// repeated and unions with the config-file default; output order always
// follows the view, never the flag order.
type colList []string

func (c *colList) String() string { return strings.Join(*c, ",") }
func (c *colList) Set(s string) error {
	*c = append(*c, s)
	return nil
}

// split flattens every occurrence into trimmed, non-blank column names.
func (c colList) split() []string {
	var out []string
	for _, s := range c {
		out = append(out, splitList(s)...)
	}
	return out
}

// Parsed options; package-level so applyConfig can override them from
// opencodable.conf. Registered in main.
var (
	configPath *string
	provider   *string
	plan       *string
	sortKey    *string
	pareto     *bool
	paid       *bool
	privacy    *bool
	exclude    *string
	include    *string
	maxCost    *float64
	minSpeed   *float64
	minQuality *string
	missing    *string
	colsFlag   colList
	excolsFlag colList
	peak       *bool
	refresh    *bool
	offline    *bool
	aaPro      *bool
	probeAll   *bool
	probeOne   *string
	probeMax   *float64
	showUse    *bool
	now        *bool
	fullView   *bool
	mainView   *bool
	nohead     *bool
	ttlSecs    *int
	winEst     *bool
	doctor     *bool
	spent      *string
	vertical   *bool
	jsonOut    *bool
	showLegend *bool
)

func main() {
	// The probe uses its own longer client; all catalog/benchmark/usage
	// fetches share this timeout.
	http.DefaultClient.Timeout = 30 * time.Second
	probe.Version = version

	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintln(out, "\nBoolean options also accept -name=false (for example -paid=false) to override a value from opencodable.conf.")
		fmt.Fprintf(out, "\nopencodable %s — https://github.com/ColasNahaboo/opencodable\n", version)
	}
	configPath = flag.String("config", "", "read settings and option defaults from this file instead of the per-user opencodable.conf")
	provider = flag.String("provider", "opencode-go", "models.dev provider to rank")
	plan = flag.String("plan", "go", "subscription plan: go|go-plus (allowances)")
	sortKey = flag.String("sort", "quality", "sort: name|price|speed|quality|reqs|usable (usable ranks -full by affordable requests)")
	pareto = flag.Bool("pareto", false, "only Pareto-optimal rows")
	paid = flag.Bool("paid", false, "hide free models (cost_task = 0)")
	privacy = flag.Bool("privacy", false, "hide models that train on request data or have unknown privacy status")
	exclude = flag.String("exclude", "", "hide models whose ID or name matches any comma-separated pattern ('*' and '?' are wildcards)")
	include = flag.String("include", "", "show only models whose ID or name matches any comma-separated pattern ('*' and '?' are wildcards)")
	maxCost = flag.Float64("max-cost", 0, "hide models whose cost per task (cost_task, USD) exceeds this value")
	minSpeed = flag.Float64("min-speed", 0, "hide models slower than this many output tokens/s (measured probe when available, else AA)")
	minQuality = flag.String("min-quality", "", "hide models below 'metric=value' pairs (aa_int, swe_b, term_b, if_b, aa_lcr, lb_agent, lb_code, elo, btr; aa_code/aa_math/code_b are no longer published); a bare value means composite quality")
	missing = flag.String("missing", "keep", "numeric filters: keep or drop models with no value for the metric (keep|drop)")
	flag.Var(&colsFlag, "cols", "show only these columns (comma-separated titles, repeatable; name is always shown)")
	flag.Var(&excolsFlag, "excols", "hide these columns (comma-separated titles, repeatable; name is always shown)")
	peak = flag.Bool("peak", false, "use peak-hours prices (DeepSeek); the whole table is peak-priced, so no [peak] twins are added")
	refresh = flag.Bool("refresh", false, "bypass the 24h cache and refetch")
	offline = flag.Bool("offline", false, "cache only for catalog/benchmarks/probes; -usage still needs network")
	aaPro = flag.Bool("aa-pro", false, "use the Artificial Analysis Pro endpoint (per-benchmark scores; requires a Pro key)")
	probeAll = flag.Bool("probe", false, "measure live speed of every model (paid requests, cached 24h)")
	probeOne = flag.String("probe-model", "", "measure live speed of one model ID and print the result")
	probeMax = flag.Float64("probe-max-allowance", 0, "stop -probe after this many allowance dollars (0 = no limit)")
	showUse = flag.Bool("usage", false, "show remaining Go allowance for the 5h/week/month windows")
	now = flag.Bool("now", false, "per model: requests affordable in each Go window, using the model's own monthly limit")
	fullView = flag.Bool("full", false, "join the main table and -now: static cost/speed/quality plus affordable requests per model")
	mainView = flag.Bool("main", false, "show the default ranking table (static cost/speed/quality); the default when no view is given")
	nohead = flag.Bool("nohead", false, "omit the header row (data rows only); no-op in -vertical/-json, which have no header")
	ttlSecs = flag.Int("ttl", 600, "cache lifetime in seconds for live usage endpoints (console + windows); 0 disables caching")
	winEst = flag.Bool("window-estimate", false, "scale -now/-full 5h/week caps by the account-wide usage percentages (estimate)")
	doctor = flag.Bool("doctor", false, "audit plan/AA/SWE/probe coverage and exit")
	spent = flag.String("spent", "", "path to a pasted console 'usage by model' TSV; offline fallback for per-model spend")
	vertical = flag.Bool("vertical", false, "one model per block of 'field: value' lines (screen-reader friendly); applies to the table and -now")
	jsonOut = flag.Bool("json", false, "JSON output instead of TSV")
	showLegend = flag.Bool("l", false, "print the legend block explaining each column")
	config.AddOptions("provider", "plan", "sort", "pareto", "paid", "privacy",
		"exclude", "include", "max-cost", "min-speed", "min-quality", "missing",
		"cols", "excols",
		"peak", "refresh", "offline", "aa-pro", "probe", "probe-model", "probe-max-allowance",
		"usage", "now", "full", "main", "window-estimate", "doctor", "spent", "vertical", "json", "l", "nohead", "ttl")
	flag.Parse()
	// The config path itself cannot come from the config file: it is needed
	// to read it. Hence it is not registered with config.AddOptions.
	if err := config.SetPath(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, "-config:", err)
		os.Exit(2)
	}
	if err := applyConfig(flag.CommandLine); err != nil {
		fmt.Fprintln(os.Stderr, "opencodable.conf:", err)
		os.Exit(2)
	}
	flt, ferr := buildFilter(flag.CommandLine)
	if ferr != nil {
		fmt.Fprintln(os.Stderr, ferr)
		os.Exit(2)
	}
	csel, cerr := buildColumns()
	if cerr != nil {
		fmt.Fprintln(os.Stderr, cerr)
		os.Exit(2)
	}
	if vs := selectedViews(); len(vs) > 1 {
		fmt.Fprintf(os.Stderr, "only one view at a time: %s\n", strings.Join(vs, ", "))
		os.Exit(2)
	}

	if *plan != "go" && *plan != "go-plus" {
		fmt.Fprintf(os.Stderr, "invalid -plan %q: want go or go-plus\n", *plan)
		os.Exit(2)
	}
	if *ttlSecs < 0 {
		fmt.Fprintf(os.Stderr, "invalid -ttl %d: want >= 0 seconds\n", *ttlSecs)
		os.Exit(2)
	}
	ttl := time.Duration(*ttlSecs) * time.Second

	if *probeOne != "" {
		if err := runOneProbe(*probeOne, *refresh, *offline); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if *showUse {
		if *offline {
			fmt.Fprintln(os.Stderr, "-usage reads the live allowance endpoint; remove -offline")
			os.Exit(2)
		}
		if err := runUsage(*jsonOut, *showLegend, isTTY(os.Stdout), *nohead, ttl, *refresh); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if *now {
		if *provider != "opencode-go" {
			fmt.Fprintln(os.Stderr, "-now only supports -provider opencode-go")
			os.Exit(2)
		}
		spentByModel, err := loadSpentFile(*spent)
		if err != nil {
			fmt.Fprintln(os.Stderr, "spent file:", err)
			os.Exit(1)
		}
		if err := runNow(*plan, spentByModel, *spent != "", *paid, *privacy, flt, *peak, *winEst, *vertical, *jsonOut, *showLegend, *refresh, *offline, isTTY(os.Stdout), csel, *nohead, ttl); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if *fullView {
		if *provider != "opencode-go" {
			fmt.Fprintln(os.Stderr, "-full only supports -provider opencode-go")
			os.Exit(2)
		}
		spentByModel, err := loadSpentFile(*spent)
		if err != nil {
			fmt.Fprintln(os.Stderr, "spent file:", err)
			os.Exit(1)
		}
		if err := runFull(*plan, spentByModel, *spent != "", *paid, *privacy, flt, *peak, *winEst, *vertical, *jsonOut, *showLegend, *refresh, *offline, isTTY(os.Stdout), csel, *nohead, ttl); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if *doctor {
		if *provider != "opencode-go" {
			fmt.Fprintln(os.Stderr, "-doctor only supports -provider opencode-go")
			os.Exit(2)
		}
		if err := runDoctor(*refresh, *offline, *aaPro, *jsonOut, *showLegend, *nohead); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	models, err := modelsdev.Fetch(*provider, *refresh, *offline)
	if err != nil {
		fmt.Fprintln(os.Stderr, "models.dev:", err)
		os.Exit(1)
	}
	aas, swe, lb, elo, btr := fetchBenchmarks(*refresh, *offline)

	probes, perr := probeModels(models, flt, *probeAll, *refresh, *offline, *privacy, *probeMax)
	if perr != nil {
		fmt.Fprintln(os.Stderr, "probe:", perr)
		os.Exit(1)
	}

	rows := table.Build(models, aas, probes, swe, lb, elo, btr, *plan, *peak)
	if *paid {
		rows = slices.DeleteFunc(rows, func(r table.Row) bool { return r.CostTask == 0 })
	}
	if *privacy {
		rows = slices.DeleteFunc(rows, func(r table.Row) bool {
			return goplan.ExcludedByPrivacy(r.ID)
		})
	}
	if !*peak {
		// Peak twins inherit the paid/privacy verdicts above; their peak
		// cost still faces the value filters below. Skipped under -peak:
		// the whole table is already peak-priced, so twins would duplicate
		// base rows.
		rows = table.WithPeakTwins(rows, models)
	}
	rows = slices.DeleteFunc(rows, func(r table.Row) bool {
		return flt.Excluded(r.ID, r.Name) || !flt.Included(r.ID, r.Name) || !flt.Keep(r)
	})
	// Pareto is relative to the set the user actually sees: a model hidden
	// by any filter must not dominate a visible one.
	table.MarkPareto(rows)
	table.Sort(rows, *sortKey)
	if *pareto {
		rows = slices.DeleteFunc(rows, func(r table.Row) bool { return !r.Pareto })
	}
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "no models matched; check -provider, -plan, filters and cache state")
		os.Exit(1)
	}
	sel, serr := table.SelectTitles(table.HeaderCells(), csel.includes, csel.excludes)
	if serr != nil {
		fmt.Fprintln(os.Stderr, serr)
		os.Exit(2)
	}
	if *vertical && *jsonOut {
		fmt.Fprintln(os.Stderr, "warning: -vertical is ignored with -json")
	}
	if *jsonOut {
		if err := table.WriteJSON(os.Stdout, rows, sel); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *vertical {
		if err := table.WriteVertical(os.Stdout, rows, sel, *showLegend); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := table.WriteTSV(os.Stdout, rows, sel, isTTY(os.Stdout), *showLegend, *nohead); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// applyConfig fills options set in opencodable.conf (lowercase keys) for
// every flag the command line did not set: command line beats config file,
// config file beats the flag default. Booleans accept true/on/1/yes and
// false/off/0/no. fs is the parsed command line, so -paid=false counts as
// set and overrides paid=yes in the file.
func applyConfig(fs *flag.FlagSet) error {
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	str := map[string]*string{
		"provider": provider, "plan": plan, "sort": sortKey,
		"probe-model": probeOne, "spent": spent,
		"exclude": exclude, "include": include, "min-quality": minQuality, "missing": missing,
	}
	boolean := map[string]*bool{
		"pareto": pareto, "paid": paid, "privacy": privacy, "peak": peak,
		"refresh": refresh, "offline": offline, "aa-pro": aaPro, "probe": probeAll, "usage": showUse,
		"now": now, "full": fullView, "main": mainView, "window-estimate": winEst, "doctor": doctor,
		"vertical": vertical, "json": jsonOut, "l": showLegend, "nohead": nohead,
	}
	for name, p := range str {
		if v, ok := config.Option(name); ok && !set[name] {
			*p = v
		}
	}
	for name, p := range boolean {
		v, ok := config.Option(name)
		if !ok || set[name] {
			continue
		}
		b, err := config.ParseBool(v)
		if err != nil {
			return fmt.Errorf("option %s: %w", name, err)
		}
		*p = b
	}
	floats := map[string]*float64{
		"probe-max-allowance": probeMax,
		"max-cost":            maxCost,
		"min-speed":           minSpeed,
	}
	for name, p := range floats {
		if v, ok := config.Option(name); ok && !set[name] {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return fmt.Errorf("option %s: %w", name, err)
			}
			*p = f
		}
	}
	ints := map[string]*int{
		"ttl": ttlSecs,
	}
	for name, p := range ints {
		if v, ok := config.Option(name); ok && !set[name] {
			n, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("option %s: %w", name, err)
			}
			*p = n
		}
	}
	// Column sets are additive: the command line unions with the config
	// file, so a global preset can be refined per run with -excols.
	lists := map[string]*colList{"cols": &colsFlag, "excols": &excolsFlag}
	for name, p := range lists {
		v, ok := config.Option(name)
		if !ok {
			continue
		}
		if set[name] {
			*p = append(splitList(v), *p...)
		} else {
			*p = splitList(v)
		}
	}
	return nil
}

// colSel is the resolved -cols/-excols selection: raw column names,
// matched case-insensitively when projected onto a view.
type colSel struct {
	includes []string
	excludes []string
}

// buildColumns validates the column selection against every model view.
// Unknown names fail here, so typos surface even for views that ignore the
// selection (-usage, -doctor, -probe-model).
func buildColumns() (colSel, error) {
	s := colSel{includes: colsFlag.split(), excludes: excolsFlag.split()}
	for _, n := range s.includes {
		if !table.KnownColumn(n) {
			return s, fmt.Errorf("-cols: unknown column %q", n)
		}
	}
	for _, n := range s.excludes {
		if !table.KnownColumn(n) {
			return s, fmt.Errorf("-excols: unknown column %q", n)
		}
	}
	return s, nil
}

// buildFilter validates the filter options (command line or config) and
// returns the row selection. -missing defaults to keep: a row without a value
// for a filtered metric is not pruned by that bound.
func buildFilter(fs *flag.FlagSet) (table.Filter, error) {
	f := table.Filter{Exclude: splitList(*exclude), Include: splitList(*include)}
	if optionSet(fs, "max-cost") {
		f.MaxCost = maxCost
	}
	if optionSet(fs, "min-speed") {
		f.MinSpeed = minSpeed
	}
	metrics, err := parseMinQuality(*minQuality)
	if err != nil {
		return f, err
	}
	f.MinQuality = metrics
	switch strings.ToLower(strings.TrimSpace(*missing)) {
	case "", "keep":
		f.DropMissing = false
	case "drop":
		f.DropMissing = true
	default:
		return f, fmt.Errorf("-missing %q: want keep or drop", *missing)
	}
	return f, nil
}

// selectedViews names the requested output views (-usage, -now, -full,
// -main, -doctor). At most one may be set; with none the main table runs.
func selectedViews() []string {
	var out []string
	if *showUse {
		out = append(out, "-usage")
	}
	if *now {
		out = append(out, "-now")
	}
	if *fullView {
		out = append(out, "-full")
	}
	if *mainView {
		out = append(out, "-main")
	}
	if *doctor {
		out = append(out, "-doctor")
	}
	return out
}

// optionSet reports whether name was set on the command line or in the config
// file, so an explicit 0 is distinguishable from an unset bound.
func optionSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	if found {
		return true
	}
	_, ok := config.Option(name)
	return ok
}

// splitList splits a comma-separated option value, dropping blanks.
func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// parseMinQuality parses "aa_int=40,aa_code=35": one or more metric=value
// pairs, or a bare value for the composite quality used by -sort quality.
func parseMinQuality(s string) (map[string]float64, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	out := map[string]float64{}
	for _, part := range splitList(s) {
		metric, value := "quality", part
		if k, v, ok := strings.Cut(part, "="); ok {
			metric, value = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		}
		if !table.IsQualityMetric(metric) {
			return nil, fmt.Errorf("-min-quality: unknown metric %q (want %s, or quality)",
				metric, strings.Join(table.QualityMetricNames, ", "))
		}
		f, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("-min-quality: %s=%q: want a score", metric, value)
		}
		out[metric] = f
	}
	return out, nil
}

// isTTY reports whether f is an interactive terminal.
func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// DefaultMaxTokens is the probe generation budget; wide enough for thinking
// models to reach content. RetryMaxTokens widens the window when a model is
// so fast that the timing burst is too short to trust.
const (
	DefaultMaxTokens int64 = 1024
	RetryMaxTokens   int64 = 4096
)

// cachedProbe reads a stored measurement without any network or probing.
func cachedProbe(modelID string) (probe.Result, bool) {
	var r probe.Result
	b, ok := cache.Peek("probe-" + modelID + ".json")
	if !ok || json.Unmarshal(b, &r) != nil || r.ModelID == "" {
		return probe.Result{}, false
	}
	r.Normalize() // old cache entries predate the current thresholds
	return r, true
}

// loadOrProbe returns the cached measurement or runs a fresh one. Escalation:
// thinking models that never reach content get a bigger budget; fast models
// with a too-short window get the long prompt.
func loadOrProbe(modelID, key string, refresh, offline bool) (probe.Result, bool) {
	if offline {
		if r, ok := cachedProbe(modelID); ok {
			return r, false
		}
		return probe.Result{ModelID: modelID, Error: "offline and no cached measurement"}, false
	}
	fresh := false
	run := func(long bool, maxTokens int64) probe.Result {
		runOne := probe.Run
		if long {
			runOne = probe.RunLong
		}
		fresh = true
		r, err := runOne(modelID, key, maxTokens)
		if err != nil {
			return probe.Result{ModelID: modelID, Error: err.Error()}
		}
		// Ledger: a successful live probe burned real allowance. Cached
		// reads never reach here, failures carry no burn to record.
		saveLedger(recordProbeBurn(loadLedger(), modelID, r.Allowance, time.Now()))
		return r
	}
	var cached probe.Result
	if b, err := cache.Load("probe-"+modelID+".json", 24*time.Hour, refresh, false,
		func() ([]byte, error) { return json.Marshal(run(false, DefaultMaxTokens)) }); err == nil {
		_ = json.Unmarshal(b, &cached)
		cached.Normalize()
	}
	if cached.Trusted() {
		return cached, false
	}
	r := cached
	if r.Error != "" {
		// Failure (often "no content": thinking ate the budget) → retry
		// with a much bigger budget before giving up for this run.
		if r2 := run(false, RetryMaxTokens); r2.Error == "" {
			r = r2
		}
	}
	if r.Error == "" && !r.Trusted() {
		if r2 := run(true, RetryMaxTokens); r2.Error == "" && r2.WindowSeconds > r.WindowSeconds {
			r = r2
		}
	}
	if r.Error == "" {
		_ = cache.Store("probe-"+modelID+".json", mustJSON(r))
	}
	return r, fresh
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// fetchBenchmarks loads the AA scores, SWE-bench, LiveBench, Arena Elo and
// Bradley-Terry snapshots, warning on stderr and continuing without the
// missing source.
func fetchBenchmarks(refresh, offline bool) (aas []aa.Model, swe map[string]float64, lb bench.LB, elo bench.Elo, btr bench.BTR) {
	var aerr error
	if aas, aerr = aa.Fetch(refresh, offline, *aaPro); aerr != nil {
		fmt.Fprintln(os.Stderr, "warning: benchmark data unavailable:", aerr)
		aas = nil
	}
	var serr error
	if swe, serr = bench.SWEBench(refresh, offline); serr != nil {
		fmt.Fprintln(os.Stderr, "warning: SWE-bench unavailable:", serr)
		swe = nil
	}
	var lerr error
	if lb, lerr = bench.LiveBench(refresh, offline); lerr != nil {
		fmt.Fprintln(os.Stderr, "warning: LiveBench unavailable:", lerr)
		lb = nil
	}
	var eerr error
	if elo, eerr = bench.FetchElo(refresh, offline); eerr != nil {
		fmt.Fprintln(os.Stderr, "warning: Arena Elo unavailable:", eerr)
		elo = nil
	}
	var berr error
	if btr, berr = bench.FetchBTRating(refresh, offline); berr != nil {
		fmt.Fprintln(os.Stderr, "warning: Arena BTR unavailable:", berr)
		btr = nil
	}
	return aas, swe, lb, elo, btr
}

// probeModels returns cached probe measurements, running fresh paid probes
// when asked. Hidden models (-privacy, -exclude, or missing -include) are
// skipped: hidden rows are not worth paid probe requests.
func probeModels(models []modelsdev.Model, flt table.Filter, doProbe, doRefresh, doOffline, doPrivacy bool, probeMax float64) (map[string]probe.Result, error) {
	probes := map[string]probe.Result{}
	for _, m := range models {
		if r, ok := cachedProbe(m.ID); ok {
			probes[m.ID] = r
		}
	}
	if !doProbe {
		return probes, nil
	}
	key := ""
	if !doOffline {
		var kerr error
		if key, kerr = probe.Key(); kerr != nil {
			return nil, kerr
		}
	}
	burned := 0.0
	for _, m := range models {
		if doPrivacy && goplan.ExcludedByPrivacy(m.ID) {
			continue
		}
		if flt.Excluded(m.ID, m.Name) || !flt.Included(m.ID, m.Name) {
			continue
		}
		r, fresh := loadOrProbe(m.ID, key, doRefresh, doOffline)
		probes[m.ID] = r
		suspect := ""
		if r.Suspect {
			suspect = " suspect"
		}
		fmt.Fprintf(os.Stderr, "probe %-32s tok/s=%-7.1f ttft=%-6.2fs allowance=$%.6f%s %s\n",
			m.ID, r.TokPerSec, r.TTFTSeconds, r.Allowance, suspect, r.Error)
		if fresh {
			burned += r.Allowance
		}
		if probeMax > 0 && burned >= probeMax {
			fmt.Fprintf(os.Stderr, "probe: stopping after $%.6f allowance burned (limit $%.6f)\n", burned, probeMax)
			break
		}
	}
	if !doOffline {
		fmt.Fprintf(os.Stderr, "probe total allowance burned: $%.6f\n", burned)
	}
	return probes, nil
}

// loadSpentFile parses the pasted console TSV, or returns nil when no path
// was given (spend then comes from the live console API).
func loadSpentFile(path string) (map[string]float64, error) {
	if path == "" {
		return nil, nil
	}
	return usage.ParseSpentFile(path)
}

// staleWarn reports served-stale data: age beyond both the TTL and a grace
// interval (fresh writes always read back younger than that).
func staleWarn(what string, age, ttl time.Duration) {
	if age > ttl && age > 5*time.Second {
		fmt.Fprintf(os.Stderr, "# warning: %s is %s old (live fetch failed)\n", what, age.Round(time.Second))
	}
}

// fetchSpend returns per-model period spend in allowance dollars, straight
// from the console API with the same key (the -spent paste file overrides
// when given). Live fetches are TTL-cached; stale fallbacks are reported,
// never silent. Spend sources and assumptions print on stderr, never stdout.
func fetchSpend(byID map[string]modelsdev.Model, spentFile map[string]float64, spentGiven, peak, offline bool, ttl time.Duration, refresh bool) (map[string]spendInfo, bool) {
	spend := map[string]spendInfo{}
	spendKnown := false
	src := "unavailable: assuming $0 spent"
	listCostRows := 0
	switch {
	case spentGiven:
		spendKnown = true
		src = "-spent file"
		for id, usd := range spentFile {
			spend[id] = spendInfo{usd: usd, known: true}
		}
	case offline:
		src = "offline: assuming $0 spent"
	default:
		key, kerr := probe.Key()
		if kerr != nil {
			src = "unavailable: " + kerr.Error() + "; assuming $0 spent"
			break
		}
		mu, age, merr := consoleapi.FetchModelUsage(key, ttl, refresh)
		if merr != nil {
			src = "unavailable: " + merr.Error() + "; assuming $0 spent"
			break
		}
		staleWarn("console spend", age, ttl)
		spendKnown = true
		src = "console api (opencode-go rows, tokens at Go prices)"
		for _, s := range mu {
			if s.Provider != "opencode-go" && s.Provider != "" {
				continue // pay-as-you-go rows are not Go allowance spend
			}
			usd := s.CostUSD
			m, known := byID[s.Model]
			tokens := s.InputTokens + s.OutputTokens + s.CacheReadTokens + s.CacheWriteTokens()
			if known && tokens > 0 {
				usd = cost.FromTokens(m, s.InputTokens, s.CacheReadTokens, s.OutputTokens, s.CacheWriteTokens(), peak)
			} else {
				listCostRows++ // console list cost, not necessarily allowance cost
			}
			prev := spend[s.Model]
			prev.usd += usd
			prev.reqs += s.Requests
			prev.known = true
			spend[s.Model] = prev
		}
	}

	fmt.Fprintf(os.Stderr, "# per-model spend: %s\n", src)
	fmt.Fprintln(os.Stderr, "# per-model Go limits: 5h=20%, week=50%, month=100% of each model's monthly limit; reqs_5h/reqs_wk are fresh-window caps")
	if listCostRows > 0 {
		fmt.Fprintf(os.Stderr, "# warning: %d console rows priced at their reported list cost (no catalog entry or no token counts); allowance cost may differ\n", listCostRows)
	}
	return spend, spendKnown
}

// windowEst is a resolved window estimate: per-model 5h/week window spend
// plus the derivation method behind window_method.
type windowEst struct {
	spend  map[string]windowSpend
	method string // "full" | "proportional" | "ledger"
}

// resolveWindowSpend estimates per-model 5h/week window spend for
// -window-estimate. Without the flag (or offline, or on fetch failure) it
// returns no spends ("full": fresh caps). Otherwise it fetches the aggregate
// windows (TTL-cached), attributes the local ledger, and splits the
// remainder monthly-proportionally. On success with real spend data it also
// records the snapshot for future runs. Every basis prints on stderr.
func resolveWindowSpend(doEstimate, offline bool, models []modelsdev.Model, spend map[string]spendInfo, spendKnown bool, planName string, ttl time.Duration, refresh bool) windowEst {
	if !doEstimate {
		return windowEst{method: "full"}
	}
	if offline {
		fmt.Fprintln(os.Stderr, "# window estimate: offline; using full 5h/week caps")
		return windowEst{method: "full"}
	}
	key, kerr := probe.Key()
	if kerr != nil {
		fmt.Fprintf(os.Stderr, "# window estimate: %v; using full 5h/week caps\n", kerr)
		return windowEst{method: "full"}
	}
	ws, age, uerr := usage.Fetch(key, ttl, refresh)
	if uerr != nil {
		fmt.Fprintf(os.Stderr, "# window estimate: %v; using full 5h/week caps\n", uerr)
		return windowEst{method: "full"}
	}
	staleWarn("usage windows", age, ttl)
	f := usage.RemainingFactors(ws)
	var reset5h, resetWk time.Time
	for _, w := range ws {
		switch w.Name {
		case "5h":
			reset5h = w.ResetsAt
		case "week":
			resetWk = w.ResetsAt
		}
	}
	now := time.Now()
	led := loadLedger()
	attr := attributeLedger(led, spend, now, reset5h, resetWk)
	est, method := estimateWindowSpend(models, spend, spendKnown, planName, 1-f.Five, 1-f.Week, attr)
	attributed := 0
	for _, a := range attr {
		if a.five+a.week > 0 {
			attributed++
		}
	}
	if method == "ledger" {
		fmt.Fprintf(os.Stderr, "# window method: ledger (%d models with ledger-attributed spend)\n", attributed)
	} else {
		fmt.Fprintln(os.Stderr, "# window method: proportional (monthly-proportional split; no ledger attribution)")
	}
	if spendKnown {
		saveLedger(recordSnapshot(led, spend, now))
	}
	return windowEst{spend: est, method: method}
}

// runUsage prints the remaining Go allowance windows, as JSON, tty-padded
// or piped as strict TABs — same style rules as the model table.
func runUsage(jsonOut, withLegend, tty, nohead bool, ttl time.Duration, refresh bool) error {
	key, err := probe.Key()
	if err != nil {
		return err
	}
	ws, age, err := usage.Fetch(key, ttl, refresh)
	if err != nil {
		return err
	}
	staleWarn("usage windows", age, ttl)
	now := time.Now()
	if jsonOut {
		type jsonWindow struct {
			Window       string  `json:"window"`
			Status       string  `json:"status"`
			UsedPct      float64 `json:"used_pct"`
			RemainingPct float64 `json:"remaining_pct"`
			ResetsAt     string  `json:"resets_at"`
			ResetsIn     string  `json:"resets_in"`
		}
		out := make([]jsonWindow, 0, len(ws))
		for _, w := range ws {
			out = append(out, jsonWindow{
				Window: w.Name, Status: w.Status,
				UsedPct: w.Percent, RemainingPct: w.Remaining(),
				ResetsAt: w.ResetsAt.Local().Format(time.RFC3339),
				ResetsIn: resetIn(now, w.ResetsAt),
			})
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	useCells := func(w usage.Window, tty bool) []string {
		if tty {
			return []string{w.Name, w.Status,
				strconv.FormatFloat(w.Percent, 'f', 0, 64) + "%",
				strconv.FormatFloat(w.Remaining(), 'f', 0, 64) + "%",
				w.ResetsAt.Local().Format("2006-01-02 15:04"),
				resetIn(now, w.ResetsAt)}
		}
		return []string{w.Name, w.Status,
			strconv.FormatFloat(w.Percent, 'f', 0, 64),
			strconv.FormatFloat(w.Remaining(), 'f', 0, 64),
			w.ResetsAt.Local().Format(time.RFC3339),
			resetIn(now, w.ResetsAt)}
	}
	head := []string{"#window", "status", "used_pct", "remaining_pct", "resets_at", "resets_in"}
	if withLegend {
		for _, l := range useLegend {
			fmt.Fprintln(os.Stdout, l)
		}
	}
	if !tty {
		if !nohead {
			fmt.Fprintln(os.Stdout, strings.Join(head, "\t"))
		}
		for _, w := range ws {
			fmt.Fprintln(os.Stdout, strings.Join(useCells(w, false), "\t"))
		}
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	if !nohead {
		fmt.Fprintln(tw, strings.Join(head, "\t"))
	}
	for _, w := range ws {
		fmt.Fprintln(tw, strings.Join(useCells(w, true), "\t"))
	}
	return tw.Flush()
}

// useLegend documents the -usage columns; printed only with -l.
var useLegend = []string{
	"# window        usage window: 5h = 20%, week = 50%, month = 100% of each model's monthly limit",
	"# status        ok | throttled | exhausted (as reported by the endpoint)",
	"# used_pct      percent of the window allowance already consumed",
	"# remaining_pct 100 - used_pct",
	"# resets_at     local time when the window resets",
	"# resets_in     humanized time until reset",
	"# note          the endpoint reports one aggregate set of windows; per-model caps are in -now",
}

// resetIn humanizes the duration until a window resets.
func resetIn(from, at time.Time) string {
	d := at.Sub(from)
	if d <= 0 {
		return "now"
	}
	return usage.HumanizeDuration(d.Round(time.Minute))
}

// minActualRequests is the minimum sample size before the console's measured
// average replaces the official per-request profile; below it the average is
// too noisy.
const minActualRequests = 50

// spendInfo is one model's console/period spend in allowance dollars.
type spendInfo struct {
	usd   float64
	reqs  int64
	known bool
}

// nowRow is one line of the -now table: what this model affords in each
// per-model Go window.
type nowRow struct {
	id        string
	name      string
	costTask  float64 // official allowance cost per request
	burn      float64 // measured or official allowance cost per request
	spentMo   float64 // allowance dollars spent this period
	spentOK   bool    // spend is known, not assumed zero
	five      int     // cap in the 5h window (20% of monthly limit)
	week      int     // cap in the weekly window (50%)
	month     int     // remaining monthly allowance
	usable    int
	binding   string
	unlimited bool // free model: no per-model cap
	known     bool // plan snapshot knows this model
	probeTokS float64
}

// nowLegend documents the -now columns; printed only with -l.
var nowLegend = []string{
	"# opencodable -now — requests affordable per model in each Go window",
	"# name       model display name (models.dev catalog; [peak] = peak-hours twin of the same model)",
	"# cost_task  official allowance cost per request (OpenCode token profile, base tier)",
	"# burn_req   allowance dollars one request burns: measured from console tokens when the model has >=50 requests this period, else cost_task; [peak] twins scale that measured burn by the official peak/base price ratio",
	"# spent_mo   allowance dollars spent this period (console tokens at Go prices, or -spent); subtracted from the model's monthly limit",
	"# reqs_5h    requests in the model's 5h cap (20% of its monthly limit) after estimated window spend; -window-estimate method on stderr",
	"# reqs_wk    requests in the model's weekly cap (50% of its monthly limit) after estimated window spend; -window-estimate method on stderr",
	"# reqs_mo    requests left in the model's monthly limit after spent_mo",
	"# usable     min of reqs_5h / reqs_wk / reqs_mo — what you can use now",
	"# limit      binding constraint: 5h | week | month | unlimited | unknown",
	"# p_tok_s    measured output tok/s on your subscription (trusted probe windows only)",
}

// buildNowRows is the testable core of -now. Go limits are per model: the
// 5h and weekly windows are 20% and 50% of that model's monthly limit, and
// the monthly window is what remains of it. Spend is in allowance dollars.
// wspend carries estimated window spend per model (nil = fresh caps);
// over-spent budgets clamp to zero inside usage.Requests.
func buildNowRows(models []modelsdev.Model, spend map[string]spendInfo, spendKnown bool, planName string, peak, paid, privacy bool, wspend map[string]windowSpend, speeds map[string]float64) []nowRow {
	var rows []nowRow
	for _, m := range models {
		if privacy && goplan.ExcludedByPrivacy(m.ID) {
			continue
		}
		costTask := cost.PerRequest(m, peak)
		if paid && costTask == 0 {
			continue
		}
		planKnown := goplan.Known(m.ID)
		allow, unlimited := goplan.Allowance(m.ID, planName)
		if !planKnown {
			if m.Free() {
				unlimited = true
			} else {
				rows = append(rows, nowRow{
					id: m.ID, name: m.Name, costTask: costTask, burn: costTask,
					spentMo: spend[m.ID].usd, spentOK: spend[m.ID].known || spendKnown,
					binding: "unknown", known: false, probeTokS: speeds[m.ID],
				})
				continue
			}
		}
		if unlimited {
			rows = append(rows, nowRow{
				id: m.ID, name: m.Name, costTask: costTask,
				binding: "unlimited", unlimited: true, known: planKnown,
				probeTokS: speeds[m.ID],
			})
			continue
		}
		if costTask <= 0 {
			continue // no price data: cannot turn dollars into requests
		}

		si := spend[m.ID]
		si.known = si.known || spendKnown
		measured := false
		burn := costTask
		if si.known && si.reqs >= minActualRequests && si.usd > 0 {
			if avg := si.usd / float64(si.reqs); avg > 0 {
				burn = avg
				measured = true
			}
		}
		spent := si.usd
		if !si.known {
			spent = 0 // explicit assumption, reported on stderr
		}
		b := usage.Budgets{
			Five:  0.20*allow - wspend[m.ID].five,
			Week:  0.50*allow - wspend[m.ID].week,
			Month: allow - spent,
		}
		five, week, month, usable, binding := usage.Requests(b, burn)
		rows = append(rows, nowRow{
			id: m.ID, name: m.Name, costTask: costTask, burn: burn,
			spentMo: spent, spentOK: si.known,
			five: five, week: week, month: month, usable: usable,
			binding: binding, known: true, probeTokS: speeds[m.ID],
		})
		if !peak {
			// Peak twin: same spend dollars, peak burn. With a measured
			// average, scale it by the official peak/base price ratio: the
			// console reports no regime-tagged tokens, and the official
			// profile alone can be cheaper than the measured off-peak
			// average, which would make peak look cheaper per request.
			if peakCost := cost.PerRequest(m, true); peakCost > costTask {
				peakBurn := peakCost
				if measured && costTask > 0 {
					peakBurn = burn * peakCost / costTask
				}
				tfive, tweek, tmonth, tusable, tbinding := usage.Requests(b, peakBurn)
				rows = append(rows, nowRow{
					id: m.ID + table.PeakTwinIDSuffix, name: m.Name + table.PeakTwinNameSuffix,
					costTask: peakCost, burn: peakBurn,
					spentMo: spent, spentOK: si.known,
					five: tfive, week: tweek, month: tmonth, usable: tusable,
					binding: tbinding, known: true, probeTokS: speeds[m.ID],
				})
			}
		}
	}
	slices.SortStableFunc(rows, func(a, b nowRow) int {
		if ra, rb := nowRank(a), nowRank(b); ra != rb {
			return ra - rb
		}
		if a.usable != b.usable {
			return b.usable - a.usable
		}
		if a.name != b.name {
			return strings.Compare(a.name, b.name)
		}
		return strings.Compare(a.id, b.id)
	})
	return rows
}

// filterNowRows applies the view-neutral filters to -now rows. Quality bounds
// are not evaluated: this view loads no AA scores.
func filterNowRows(rows []nowRow, flt table.Filter) []nowRow {
	return slices.DeleteFunc(rows, func(r nowRow) bool {
		return flt.Excluded(r.id, r.name) || !flt.Included(r.id, r.name) || !flt.CostOK(r.costTask) || !flt.SpeedOK(r.probeTokS)
	})
}

// nowRank orders paid models first, then unlimited, then unknown.
func nowRank(r nowRow) int {
	if r.known && !r.unlimited {
		return 0
	}
	if r.unlimited {
		return 1
	}
	return 2
}

// runNow answers "which model can I use right now": for each model, how many
// requests fit in its own Go 5h/weekly/monthly windows. There is no shared
// pot: the docs define the windows as percentages of each model's monthly
// limit. Per-model spend is measured from console token counts at Go prices.
func runNow(planName string, spentFile map[string]float64, spentGiven, paid, privacy bool, flt table.Filter, peak, windowEstimate, vertical, jsonOut, withLegend, refresh, offline, tty bool, sel colSel, nohead bool, ttl time.Duration) error {
	models, err := modelsdev.Fetch("opencode-go", refresh, offline)
	if err != nil {
		return err
	}
	byID := make(map[string]modelsdev.Model, len(models))
	for _, m := range models {
		byID[m.ID] = m
	}

	spend, spendKnown := fetchSpend(byID, spentFile, spentGiven, peak, offline, ttl, refresh)
	win := resolveWindowSpend(windowEstimate, offline, models, spend, spendKnown, planName, ttl, refresh)

	speeds := cachedProbeSpeeds(models)
	rows := buildNowRows(models, spend, spendKnown, planName, peak, paid, privacy, win.spend, speeds)
	rows = filterNowRows(rows, flt)
	if len(flt.MinQuality) > 0 {
		fmt.Fprintln(os.Stderr, "# warning: -min-quality is ignored by -now (this view loads no AA quality data)")
	}

	fullHead := table.NowTitles()
	head, herr := table.SelectTitles(fullHead, sel.includes, sel.excludes)
	if herr != nil {
		return herr
	}
	fullCells := func(r nowRow) []string {
		switch {
		case r.unlimited:
			return []string{r.name, "", "", "", "", "", "", "", "unlimited", table.Num(r.probeTokS, false)}
		case !r.known:
			return []string{r.name, table.Num(r.costTask, true), table.Num(r.burn, true), table.SpentCell(r.spentMo, r.spentOK, ""), "", "", "", "", "unknown", table.Num(r.probeTokS, false)}
		default:
			return []string{
				r.name, table.Num(r.costTask, true), table.Num(r.burn, true), table.SpentCell(r.spentMo, r.spentOK, ""),
				strconv.Itoa(r.five), strconv.Itoa(r.week), strconv.Itoa(r.month),
				strconv.Itoa(r.usable), r.binding, table.Num(r.probeTokS, false),
			}
		}
	}
	cells := func(r nowRow) []string {
		return table.ProjectCells(fullHead, fullCells(r), head)
	}
	if jsonOut {
		keep := map[string]bool{
			"id": true, "unlimited": true, "known": true,
			"spent_known": true, "window_estimate": true,
			"window_method": true,
		}
		for _, t := range head {
			keep[strings.ToLower(strings.TrimPrefix(t, "#"))] = true
		}
		out := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			m := map[string]any{
				"id": r.id, "name": r.name,
				"cost_task": r.costTask, "burn_req": r.burn,
				"spent_mo": r.spentMo, "spent_known": r.spentOK,
				"reqs_5h": r.five, "reqs_wk": r.week, "reqs_mo": r.month,
				"usable": r.usable, "limit": r.binding,
				"unlimited": r.unlimited, "known": r.known,
				"window_estimate": windowEstimate, "window_method": win.method,
				"p_tok_s": r.probeTokS,
			}
			for k := range m {
				if !keep[k] {
					delete(m, k)
				}
			}
			out = append(out, m)
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	if withLegend {
		for _, l := range table.FilterLegend(nowLegend, head) {
			fmt.Fprintln(os.Stdout, l)
		}
	}
	if vertical {
		out := os.Stdout
		for i, r := range rows {
			if i > 0 {
				fmt.Fprintln(out)
			}
			for j, v := range cells(r) {
				if v == "" {
					continue
				}
				fmt.Fprintf(out, "%s: %s\n", strings.TrimPrefix(head[j], "#"), v)
			}
		}
		return nil
	}
	if !tty {
		if !nohead {
			fmt.Fprintln(os.Stdout, strings.Join(head, "\t"))
		}
		for _, r := range rows {
			fmt.Fprintln(os.Stdout, strings.Join(cells(r), "\t"))
		}
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	if !nohead {
		fmt.Fprintln(tw, strings.Join(head, "\t"))
	}
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(cells(r), "\t"))
	}
	return tw.Flush()
}

// cachedProbeSpeeds returns trustworthy measured speeds, keyed by model ID.
func cachedProbeSpeeds(models []modelsdev.Model) map[string]float64 {
	out := map[string]float64{}
	for _, m := range models {
		if p, ok := cachedProbe(m.ID); ok && p.Trusted() {
			out[m.ID] = p.TokPerSec
		}
	}
	return out
}

// nowOnlyLegend returns the -now legend lines for columns the main table
// lacks (prose and shared columns excluded: shared values keep the main
// legend's prose). The -full legend is the main legend plus these lines,
// filtered to visible columns.
func nowOnlyLegend() []string {
	main := map[string]bool{}
	for _, t := range table.HeaderCells() {
		main[strings.ToLower(strings.TrimPrefix(t, "#"))] = true
	}
	var out []string
	for _, l := range nowLegend {
		f := strings.Fields(l)
		if len(f) < 2 || !table.KnownColumn(f[1]) || main[strings.ToLower(f[1])] {
			continue
		}
		out = append(out, l)
	}
	return out
}

// fullLegendLine builds the -full legend: shared columns keep the main
// table's prose (the values are identical), then the -now-only columns.
func fullLegend(sel []string) []string {
	lines := append(append([]string{}, table.MainLegend()...), nowOnlyLegend()...)
	return table.FilterLegend(lines, sel)
}

// joinFull outer-joins main rows with -now rows by model ID. Main rows drive:
// a model without -now data (no price to turn dollars into requests) keeps
// its static columns with blank affordability cells.
func joinFull(mrows []table.Row, nrows []nowRow, windowEstimate bool) []table.FullRow {
	byNow := make(map[string]nowRow, len(nrows))
	for _, nr := range nrows {
		byNow[nr.id] = nr
	}
	out := make([]table.FullRow, 0, len(mrows))
	for _, mr := range mrows {
		fr := table.FullRow{Row: mr, Limit: "unknown", WindowEstimate: windowEstimate}
		if nr, ok := byNow[mr.ID]; ok {
			fr.Burn, fr.SpentMo, fr.SpentKnown = nr.burn, nr.spentMo, nr.spentOK
			fr.Reqs5h, fr.ReqsWk, fr.ReqsMo, fr.Usable = nr.five, nr.week, nr.month, nr.usable
			fr.Limit, fr.Known = nr.binding, nr.known
		}
		out = append(out, fr)
	}
	return out
}

// runFull answers "what is the best model I can use right now": every main
// column (static cost/speed/quality) joined with every -now column
// (affordable requests from live spend). Column order is the main order,
// then the -now-only columns. Like -now it needs a key and is live; unlike
// -now, quality filters apply because AA scores are loaded. Speed filtering
// uses the effective speed (measured probe when available, else AA).
func runFull(planName string, spentFile map[string]float64, spentGiven, paid, privacy bool, flt table.Filter, peak, windowEstimate, vertical, jsonOut, withLegend, refresh, offline, tty bool, sel colSel, nohead bool, ttl time.Duration) error {
	models, err := modelsdev.Fetch("opencode-go", refresh, offline)
	if err != nil {
		return err
	}
	byID := make(map[string]modelsdev.Model, len(models))
	for _, m := range models {
		byID[m.ID] = m
	}
	aas, swe, lb, elo, btr := fetchBenchmarks(refresh, offline)
	probes, perr := probeModels(models, flt, *probeAll, refresh, offline, privacy, *probeMax)
	if perr != nil {
		return fmt.Errorf("probe: %w", perr)
	}

	mrows := table.Build(models, aas, probes, swe, lb, elo, btr, planName, peak)
	if paid {
		mrows = slices.DeleteFunc(mrows, func(r table.Row) bool { return r.CostTask == 0 })
	}
	if privacy {
		mrows = slices.DeleteFunc(mrows, func(r table.Row) bool {
			return goplan.ExcludedByPrivacy(r.ID)
		})
	}
	if !peak {
		// Same order as the main flow: twins inherit paid/privacy, then
		// face the value filters below.
		mrows = table.WithPeakTwins(mrows, models)
	}
	mrows = slices.DeleteFunc(mrows, func(r table.Row) bool {
		return flt.Excluded(r.ID, r.Name) || !flt.Included(r.ID, r.Name) || !flt.Keep(r)
	})
	// Pareto is relative to the set the user actually sees.
	table.MarkPareto(mrows)

	spend, spendKnown := fetchSpend(byID, spentFile, spentGiven, peak, offline, ttl, refresh)
	win := resolveWindowSpend(windowEstimate, offline, models, spend, spendKnown, planName, ttl, refresh)
	speeds := cachedProbeSpeeds(models)
	nrows := buildNowRows(models, spend, spendKnown, planName, peak, paid, privacy, win.spend, speeds)

	rows := joinFull(mrows, nrows, windowEstimate)
	for i := range rows {
		rows[i].WindowMethod = win.method
	}
	table.SortFull(rows, *sortKey)
	if *pareto {
		rows = slices.DeleteFunc(rows, func(r table.FullRow) bool { return !r.Row.Pareto })
	}
	if len(rows) == 0 {
		return fmt.Errorf("no models matched; check -provider, -plan, filters and cache state")
	}
	head, herr := table.SelectTitles(table.FullTitles(), sel.includes, sel.excludes)
	if herr != nil {
		return herr
	}
	legend := fullLegend(head)
	if vertical && jsonOut {
		fmt.Fprintln(os.Stderr, "warning: -vertical is ignored with -json")
	}
	if jsonOut {
		return table.WriteFullJSON(os.Stdout, rows, head)
	}
	if vertical {
		return table.WriteFullVertical(os.Stdout, rows, head, withLegend, legend)
	}
	return table.WriteFullTSV(os.Stdout, rows, head, tty, withLegend, legend, nohead)
}

func runOneProbe(modelID string, refresh, offline bool) error {
	key := ""
	if !offline {
		var err error
		if key, err = probe.Key(); err != nil {
			return err
		}
	}
	r, _ := loadOrProbe(modelID, key, refresh, offline)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
