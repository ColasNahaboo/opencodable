// Command opencodable ranks the models available in an OpenCode Go
// subscription by price, speed and quality: what is the best compromise
// right now.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
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
var version = "v0.1.5"

// Parsed options; package-level so applyConfig can override them from
// opencodable.conf. Registered in main.
var (
	provider *string
	plan     *string
	sortKey  *string
	pareto   *bool
	paid     *bool
	privacy  *bool
	peak     *bool
	refresh  *bool
	offline  *bool
	probeAll *bool
	probeOne *string
	probeMax *float64
	showUse  *bool
	now      *bool
	winEst   *bool
	doctor   *bool
	spent    *string
	vertical *bool
	jsonOut  *bool
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
		fmt.Fprintf(out, "\nopencodable %s — https://github.com/ColasNahaboo/opencodable\n", version)
	}
	provider = flag.String("provider", "opencode-go", "models.dev provider to rank")
	plan = flag.String("plan", "go", "subscription plan: go|go-plus (allowances)")
	sortKey = flag.String("sort", "quality", "sort: name|price|speed|quality|reqs")
	pareto = flag.Bool("pareto", false, "only Pareto-optimal rows")
	paid = flag.Bool("paid", false, "hide free models (cost_task = 0)")
	privacy = flag.Bool("privacy", false, "hide models that train on request data or have unknown privacy status")
	peak = flag.Bool("peak", false, "use peak-hours prices (DeepSeek)")
	refresh = flag.Bool("refresh", false, "bypass the 24h cache and refetch")
	offline = flag.Bool("offline", false, "cache only for catalog/benchmarks/probes; -usage still needs network")
	probeAll = flag.Bool("probe", false, "measure live speed of every model (paid requests, cached 24h)")
	probeOne = flag.String("probe-model", "", "measure live speed of one model ID and print the result")
	probeMax = flag.Float64("probe-max-allowance", 0, "stop -probe after this many allowance dollars (0 = no limit)")
	showUse = flag.Bool("usage", false, "show remaining Go allowance for the 5h/week/month windows")
	now = flag.Bool("now", false, "per model: requests affordable in each Go window, using the model's own monthly limit")
	winEst = flag.Bool("window-estimate", false, "scale -now 5h/week caps by the account-wide usage percentages (estimate)")
	doctor = flag.Bool("doctor", false, "audit plan/AA/SWE/probe coverage and exit")
	spent = flag.String("spent", "", "path to a pasted console 'usage by model' TSV; offline fallback for per-model spend")
	vertical = flag.Bool("vertical", false, "one model per block of 'field: value' lines (screen-reader friendly); applies to the table and -now")
	jsonOut = flag.Bool("json", false, "JSON output instead of TSV")
	config.AddOptions("provider", "plan", "sort", "pareto", "paid", "privacy",
		"peak", "refresh", "offline", "probe", "probe-model", "probe-max-allowance",
		"usage", "now", "window-estimate", "doctor", "spent", "vertical", "json")
	flag.Parse()
	if err := applyConfig(); err != nil {
		fmt.Fprintln(os.Stderr, "opencodable.conf:", err)
		os.Exit(2)
	}

	if *plan != "go" && *plan != "go-plus" {
		fmt.Fprintf(os.Stderr, "invalid -plan %q: want go or go-plus\n", *plan)
		os.Exit(2)
	}

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
		if err := runUsage(*jsonOut, isTTY(os.Stdout)); err != nil {
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
		var spentByModel map[string]float64
		if *spent != "" {
			var err error
			if spentByModel, err = usage.ParseSpentFile(*spent); err != nil {
				fmt.Fprintln(os.Stderr, "spent file:", err)
				os.Exit(1)
			}
		}
		if err := runNow(*plan, spentByModel, *spent != "", *paid, *privacy, *peak, *winEst, *vertical, *jsonOut, *refresh, *offline, isTTY(os.Stdout)); err != nil {
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
		if err := runDoctor(*refresh, *offline, *jsonOut); err != nil {
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
	aas, aerr := aa.Fetch(*refresh, *offline)
	if aerr != nil {
		fmt.Fprintln(os.Stderr, "warning: benchmark data unavailable:", aerr)
		aas = nil
	}
	swe, serr := bench.SWEBench(*refresh, *offline)
	if serr != nil {
		fmt.Fprintln(os.Stderr, "warning: SWE-bench unavailable:", serr)
		swe = nil
	}
	lb, lerr := bench.LiveBench(*refresh, *offline)
	if lerr != nil {
		fmt.Fprintln(os.Stderr, "warning: LiveBench unavailable:", lerr)
		lb = nil
	}

	probes := map[string]probe.Result{}
	for _, m := range models {
		if r, ok := cachedProbe(m.ID); ok {
			probes[m.ID] = r
		}
	}
	if *probeAll {
		key := ""
		if !*offline {
			var kerr error
			if key, kerr = probe.Key(); kerr != nil {
				fmt.Fprintln(os.Stderr, "probe:", kerr)
				os.Exit(1)
			}
		}
		burned := 0.0
		for _, m := range models {
			if *privacy && goplan.ExcludedByPrivacy(m.ID) {
				continue
			}
			r, fresh := loadOrProbe(m.ID, key, *refresh, *offline)
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
			if *probeMax > 0 && burned >= *probeMax {
				fmt.Fprintf(os.Stderr, "probe: stopping after $%.6f allowance burned (limit $%.6f)\n", burned, *probeMax)
				break
			}
		}
		if !*offline {
			fmt.Fprintf(os.Stderr, "probe total allowance burned: $%.6f\n", burned)
		}
	}

	rows := table.Build(models, aas, probes, swe, lb, *plan, *peak)
	if *paid {
		rows = slices.DeleteFunc(rows, func(r table.Row) bool { return r.CostTask == 0 })
	}
	if *privacy {
		rows = slices.DeleteFunc(rows, func(r table.Row) bool {
			return goplan.ExcludedByPrivacy(r.ID)
		})
	}
	// Pareto is relative to the set the user actually sees: a model hidden
	// by -paid/-privacy must not dominate a visible one.
	table.MarkPareto(rows)
	table.Sort(rows, *sortKey)
	if *pareto {
		rows = slices.DeleteFunc(rows, func(r table.Row) bool { return !r.Pareto })
	}
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "no models matched; check -provider, -plan and cache state")
		os.Exit(1)
	}
	if *vertical && *jsonOut {
		fmt.Fprintln(os.Stderr, "warning: -vertical is ignored with -json")
	}
	if *jsonOut {
		if err := table.WriteJSON(os.Stdout, rows); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *vertical {
		if err := table.WriteVertical(os.Stdout, rows); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := table.WriteTSV(os.Stdout, rows, isTTY(os.Stdout)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// applyConfig fills options set in opencodable.conf (lowercase keys) for
// every flag the command line did not set: command line beats config file,
// config file beats the flag default. Booleans accept true/on/1/yes and
// false/off/0/no.
func applyConfig() error {
	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })
	str := map[string]*string{
		"provider": provider, "plan": plan, "sort": sortKey,
		"probe-model": probeOne, "spent": spent,
	}
	boolean := map[string]*bool{
		"pareto": pareto, "paid": paid, "privacy": privacy, "peak": peak,
		"refresh": refresh, "offline": offline, "probe": probeAll, "usage": showUse,
		"now": now, "window-estimate": winEst, "doctor": doctor,
		"vertical": vertical, "json": jsonOut,
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
	if v, ok := config.Option("probe-max-allowance"); ok && !set["probe-max-allowance"] {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("option probe-max-allowance: %w", err)
		}
		*probeMax = f
	}
	return nil
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

// runUsage prints the remaining Go allowance windows, as JSON, tty-padded
// or piped as strict TABs — same style rules as the model table.
func runUsage(jsonOut, tty bool) error {
	key, err := probe.Key()
	if err != nil {
		return err
	}
	ws, err := usage.Fetch(key)
	if err != nil {
		return err
	}
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
	if !tty {
		fmt.Fprintln(os.Stdout, strings.Join(head, "\t"))
		for _, w := range ws {
			fmt.Fprintln(os.Stdout, strings.Join(useCells(w, false), "\t"))
		}
		return nil
	}
	fmt.Fprintln(os.Stdout, "# opencodable -usage — remaining Go allowance per usage window")
	for _, l := range useLegend {
		fmt.Fprintln(os.Stdout, l)
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(head, "\t"))
	for _, w := range ws {
		fmt.Fprintln(tw, strings.Join(useCells(w, true), "\t"))
	}
	return tw.Flush()
}

// useLegend documents the -usage columns, tty only.
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

// nowLegend documents the -now columns, tty only.
var nowLegend = []string{
	"# opencodable -now — requests affordable per model in each Go window",
	"# name       model display name (models.dev catalog)",
	"# cost_task  official allowance cost per request (OpenCode token profile, base tier)",
	"# burn_req   allowance dollars one request burns: measured from console tokens when the model has >=50 requests this period, else cost_task",
	"# spent_mo   allowance dollars spent this period (console tokens at Go prices, or -spent); subtracted from the model's monthly limit",
	"# reqs_5h    requests in the model's 5h cap (20% of its monthly limit; fresh-window upper bound, scaled by -window-estimate)",
	"# reqs_wk    requests in the model's weekly cap (50% of its monthly limit; fresh-window upper bound, scaled by -window-estimate)",
	"# reqs_mo    requests left in the model's monthly limit after spent_mo",
	"# usable     min of reqs_5h / reqs_wk / reqs_mo — what you can use now",
	"# limit      binding constraint: 5h | week | month | unlimited | unknown",
	"# p_tok_s    measured output tok/s on your subscription (trusted probe windows only)",
}

// buildNowRows is the testable core of -now. Go limits are per model: the
// 5h and weekly windows are 20% and 50% of that model's monthly limit, and
// the monthly window is what remains of it. Spend is in allowance dollars.
func buildNowRows(models []modelsdev.Model, spend map[string]spendInfo, spendKnown bool, planName string, peak, paid, privacy bool, fiveFactor, weekFactor float64, speeds map[string]float64) []nowRow {
	if fiveFactor <= 0 {
		fiveFactor = 1
	}
	if weekFactor <= 0 {
		weekFactor = 1
	}
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
		burn := costTask
		if si.known && si.reqs >= minActualRequests && si.usd > 0 {
			if avg := si.usd / float64(si.reqs); avg > 0 {
				burn = avg
			}
		}
		spent := si.usd
		if !si.known {
			spent = 0 // explicit assumption, reported on stderr
		}
		b := usage.Budgets{
			Five:  0.20 * allow * fiveFactor,
			Week:  0.50 * allow * weekFactor,
			Month: allow - spent,
		}
		five, week, month, usable, binding := usage.Requests(b, burn)
		rows = append(rows, nowRow{
			id: m.ID, name: m.Name, costTask: costTask, burn: burn,
			spentMo: spent, spentOK: si.known,
			five: five, week: week, month: month, usable: usable,
			binding: binding, known: true, probeTokS: speeds[m.ID],
		})
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
func runNow(planName string, spentFile map[string]float64, spentGiven, paid, privacy, peak, windowEstimate, vertical, jsonOut, refresh, offline, tty bool) error {
	models, err := modelsdev.Fetch("opencode-go", refresh, offline)
	if err != nil {
		return err
	}
	byID := make(map[string]modelsdev.Model, len(models))
	for _, m := range models {
		byID[m.ID] = m
	}

	// Per-model spend for the current period, straight from the console API
	// with the same key; the -spent paste file overrides when given.
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
		mu, merr := consoleapi.FetchModelUsage(key)
		if merr != nil {
			src = "unavailable: " + merr.Error() + "; assuming $0 spent"
			break
		}
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

	var fiveFactor, weekFactor float64
	if windowEstimate {
		switch {
		case offline:
			fmt.Fprintln(os.Stderr, "# window estimate: offline; using full 5h/week caps")
		default:
			key, kerr := probe.Key()
			if kerr != nil {
				fmt.Fprintf(os.Stderr, "# window estimate: %v; using full 5h/week caps\n", kerr)
				break
			}
			ws, uerr := usage.Fetch(key)
			if uerr != nil {
				fmt.Fprintf(os.Stderr, "# window estimate: %v; using full 5h/week caps\n", uerr)
				break
			}
			f := usage.RemainingFactors(ws)
			fiveFactor, weekFactor = f.Five, f.Week
			fmt.Fprintf(os.Stderr, "# window estimate: 5h %.0f%% left, week %.0f%% left (account-wide; applied to every model)\n",
				f.Five*100, f.Week*100)
		}
	}

	speeds := cachedProbeSpeeds(models)
	rows := buildNowRows(models, spend, spendKnown, planName, peak, paid, privacy, fiveFactor, weekFactor, speeds)

	cells := func(r nowRow) []string {
		switch {
		case r.unlimited:
			return []string{r.name, "", "", "", "", "", "", "", "unlimited", table.Num(r.probeTokS, false)}
		case !r.known:
			return []string{r.name, table.Num(r.costTask, true), table.Num(r.burn, true), moneyKnown(r.spentMo, r.spentOK), "", "", "", "", "unknown", table.Num(r.probeTokS, false)}
		default:
			return []string{
				r.name, table.Num(r.costTask, true), table.Num(r.burn, true), moneyKnown(r.spentMo, r.spentOK),
				strconv.Itoa(r.five), strconv.Itoa(r.week), strconv.Itoa(r.month),
				strconv.Itoa(r.usable), r.binding, table.Num(r.probeTokS, false),
			}
		}
	}
	head := []string{"#name", "cost_task", "burn_req", "spent_mo", "reqs_5h", "reqs_wk", "reqs_mo", "usable", "limit", "p_tok_s"}
	if jsonOut {
		type jsonNow struct {
			ID             string  `json:"id"`
			Name           string  `json:"name"`
			CostTask       float64 `json:"cost_task"`
			BurnReq        float64 `json:"burn_req"`
			SpentMo        float64 `json:"spent_mo"`
			SpentKnown     bool    `json:"spent_known"`
			Reqs5h         int     `json:"reqs_5h"`
			ReqsWk         int     `json:"reqs_wk"`
			ReqsMo         int     `json:"reqs_mo"`
			Usable         int     `json:"usable"`
			Limit          string  `json:"limit"`
			Unlimited      bool    `json:"unlimited"`
			Known          bool    `json:"known"`
			WindowEstimate bool    `json:"window_estimate"`
			ProbeTokS      float64 `json:"p_tok_s"`
		}
		out := make([]jsonNow, 0, len(rows))
		for _, r := range rows {
			out = append(out, jsonNow{
				ID: r.id, Name: r.name, CostTask: r.costTask, BurnReq: r.burn,
				SpentMo: r.spentMo, SpentKnown: r.spentOK,
				Reqs5h: r.five, ReqsWk: r.week, ReqsMo: r.month,
				Usable: r.usable, Limit: r.binding, Unlimited: r.unlimited,
				Known: r.known, WindowEstimate: windowEstimate, ProbeTokS: r.probeTokS,
			})
		}
		return json.NewEncoder(os.Stdout).Encode(out)
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
		fmt.Fprintln(os.Stdout, strings.Join(head, "\t"))
		for _, r := range rows {
			fmt.Fprintln(os.Stdout, strings.Join(cells(r), "\t"))
		}
		return nil
	}
	for _, l := range nowLegend {
		fmt.Fprintln(os.Stdout, l)
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(head, "\t"))
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

func moneyKnown(v float64, ok bool) string {
	if !ok {
		return ""
	}
	if v == 0 {
		return "$0"
	}
	return table.Num(v, true)
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
