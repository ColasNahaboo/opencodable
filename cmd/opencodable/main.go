// Command opencodable ranks the models available in an OpenCode Go
// subscription by price, speed and quality: what is the best compromise
// right now.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"opencodable/internal/aa"
	"opencodable/internal/bench"
	"opencodable/internal/cache"
	"opencodable/internal/consoleapi"
	"opencodable/internal/cost"
	"opencodable/internal/goplan"
	"opencodable/internal/modelsdev"
	"opencodable/internal/probe"
	"opencodable/internal/table"
	"opencodable/internal/usage"
)

// version is displayed at the bottom of the -h help text.
var version = "v0.0.1"

func main() {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(out, "\nopencodable %s — output spec: TITLES.md\n", version)
	}
	var (
		provider = flag.String("provider", "opencode-go", "models.dev provider to rank")
		plan     = flag.String("plan", "go", "subscription plan: go|go-plus (allowances)")
		sortKey  = flag.String("sort", "quality", "sort: name|price|speed|quality|reqs")
		pareto   = flag.Bool("pareto", false, "only Pareto-optimal rows")
		paid     = flag.Bool("paid", false, "hide free models (cost_task = 0)")
		privacy  = flag.Bool("privacy", false, "hide models that train on request data (contributor models)")
		peak     = flag.Bool("peak", false, "use peak-hours prices (DeepSeek)")
		refresh  = flag.Bool("refresh", false, "bypass the 24h cache and refetch")
		offline  = flag.Bool("offline", false, "use cache only, no network")
		probeAll = flag.Bool("probe", false, "measure live speed of every model (paid requests, ~1-2 min each, cached 24h)")
		probeOne = flag.String("probe-model", "", "measure live speed of one model ID and print the result")
		showUse  = flag.Bool("usage", false, "show remaining Go allowance for the 5h/week/month windows")
		now      = flag.Bool("now", false, "per model: requests still affordable in each remaining window")
		spent    = flag.String("spent", "", "path to a pasted console 'usage by model' TSV; subtracts real spend from monthly allowances")
		pot      = flag.Float64("pot", 0, "plan allowance pot in dollars (default: 10 for go, 40 for go-plus)")
		vertical = flag.Bool("vertical", false, "one model per block of 'field: value' lines (screen-reader friendly); applies to the table and -now")
		jsonOut  = flag.Bool("json", false, "JSON output instead of TSV")
	)
	flag.Parse()

	if *probeOne != "" {
		if err := runOneProbe(*probeOne, *refresh); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if *showUse {
		if err := runUsage(*jsonOut, isTTY(os.Stdout)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if *now {
		p := *pot
		if p == 0 {
			p = 10
			if *plan == "go-plus" {
				p = 40
			}
		}
		var spentByModel map[string]float64
		if *spent != "" {
			var err error
			if spentByModel, err = usage.ParseSpentFile(*spent); err != nil {
				fmt.Fprintln(os.Stderr, "spent file:", err)
				os.Exit(1)
			}
		}
		if err := runNow(*plan, p, spentByModel, *spent != "", *paid, *privacy, *peak, *vertical, *jsonOut, isTTY(os.Stdout)); err != nil {
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

	probes := map[string]probe.Result{}
	for _, m := range models {
		if r, ok := cachedProbe(m.ID); ok {
			probes[m.ID] = r
		}
	}
	if *probeAll {
		key, kerr := probe.Key()
		if kerr != nil {
			fmt.Fprintln(os.Stderr, "probe:", kerr)
			os.Exit(1)
		}
		for _, m := range models {
			if *privacy && goplan.TrainsOnData(m.ID) {
				continue
			}
			r := loadOrProbe(m.ID, key, *refresh)
			probes[m.ID] = r
			fmt.Fprintf(os.Stderr, "probe %-32s tok/s=%-7.1f ttft=%-6.2fs allowance=$%.6f %s\n",
				m.ID, r.TokPerSec, r.TTFTSeconds, r.Allowance, r.Error)
		}
	}

	rows := table.Build(models, aas, probes, swe, *plan, *peak)
	if *paid {
		rows = slices.DeleteFunc(rows, func(r table.Row) bool { return r.CostTask == 0 })
	}
	if *privacy {
		rows = slices.DeleteFunc(rows, func(r table.Row) bool {
			return goplan.TrainsOnData(r.ID)
		})
	}
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
		if err := table.WriteVertical(os.Stdout, rows, isTTY(os.Stdout)); err != nil {
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
	b, err := cache.Load("probe-"+modelID+".json", time.Hour, false, true,
		func() ([]byte, error) { return nil, errors.New("never fetch in cachedProbe") })
	if err != nil || json.Unmarshal(b, &r) != nil || r.ModelID == "" {
		return probe.Result{}, false
	}
	return r, true
}

// loadOrProbe returns the cached measurement or runs a fresh one. Escalation:
// thinking models that never reach content get a bigger budget; fast models
// with a too-short window get the long prompt.
func loadOrProbe(modelID, key string, refresh bool) probe.Result {
	run := func(long bool, maxTokens int64) probe.Result {
		runOne := probe.Run
		if long {
			runOne = probe.RunLong
		}
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
	}
	if cached.Error == "" && cached.WindowSeconds >= probe.MinWindow {
		return cached
	}
	r := cached
	if r.Error != "" {
		// Failure (often "no content": thinking ate the budget) → retry
		// with a much bigger budget before giving up for this run.
		if r2 := run(false, RetryMaxTokens); r2.Error == "" {
			r = r2
		}
	}
	if r.Error == "" && r.WindowSeconds < probe.MinWindow {
		if r2 := run(true, RetryMaxTokens); r2.Error == "" && r2.WindowSeconds > r.WindowSeconds {
			r = r2
		}
	}
	if r.Error == "" {
		_ = cache.Store("probe-"+modelID+".json", mustJSON(r))
	}
	return r
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
	if !tty {
		fmt.Fprintln(os.Stdout, strings.Join(
			[]string{"#window", "status", "used_pct", "remaining_pct", "resets_at", "resets_in"}, "\t"))
		for _, w := range ws {
			fmt.Fprintln(os.Stdout, strings.Join([]string{
				w.Name, w.Status,
				strconv.FormatFloat(w.Percent, 'f', 0, 64),
				strconv.FormatFloat(w.Remaining(), 'f', 0, 64),
				w.ResetsAt.Local().Format(time.RFC3339),
				resetIn(now, w.ResetsAt),
			}, "\t"))
		}
		return nil
	}
	fmt.Fprintln(os.Stdout, "# opencodable -usage — remaining Go allowance per usage window")
	for _, l := range useLegend {
		fmt.Fprintln(os.Stdout, l)
	}
	fmt.Fprintf(os.Stdout, "%-8s  %-8s  %5s  %5s  %-25s  %s\n", "window", "status", "used", "left", "resets_at", "resets_in")
	for _, w := range ws {
		fmt.Fprintf(os.Stdout, "%-8s  %-8s  %4.0f%%  %4.0f%%  %-25s  %s\n",
			w.Name, w.Status, w.Percent, w.Remaining(),
			w.ResetsAt.Local().Format("2006-01-02 15:04"), resetIn(now, w.ResetsAt))
	}
	return nil
}

// useLegend documents the -usage columns, tty only.
var useLegend = []string{
	"# window        usage window: 5h rolling = 20% of the plan limit, week = 50%, month = 100%",
	"# status        ok | throttled | exhausted (as reported by the endpoint)",
	"# used_pct      percent of the window allowance already consumed",
	"# remaining_pct 100 - used_pct",
	"# resets_at     local time when the window resets",
	"# resets_in     humanized time until reset",
}

// resetIn humanizes the duration until a window resets.
func resetIn(from, at time.Time) string {
	d := at.Sub(from)
	if d <= 0 {
		return "now"
	}
	return usage.HumanizeDuration(d.Round(time.Minute))
}

// nowRow is one line of the -now table: what this model still affords.
type nowRow struct {
	name      string
	costTask  float64
	burn      float64 // allowance dollars per request (measured ratio x profile cost)
	spentMo   float64 // real spend this period (console api or -spent file)
	five      int
	week      int
	month     int
	usable    int
	binding   string
	probeTokS float64
}

// nowLegend documents the -now columns, tty only.
var nowLegend = []string{
	"# opencodable -now — requests still affordable per model in each remaining window",
	"# name       model display name (models.dev catalog)",
	"# cost_task  estimated USD per coding-agent request (OpenCode official token profile, base tier)",
	"# burn_req   allowance dollars one request burns (measured ratio x cost_task)",
	"# spent_mo   real spend this period (console api), subtracted from the model's allowance",
	"# reqs_5h    requests affordable with the remaining 5h rolling window budget",
	"# reqs_wk    requests affordable with the remaining weekly window budget",
	"# reqs_mo    requests affordable with the remaining monthly window budget",
	"# usable     min of reqs_5h / reqs_wk / reqs_mo / model allowance — what you can spend now",
	"# limit      which constraint binds: 5h | week | month | allowance",
	"# p_tok_s    measured output tok/s on your subscription (from -probe)",
}

// runNow answers "which model can I use right now": for each model, how many
// requests fit in each remaining usage window. The usage endpoint meters one
// shared pot (assumed = plan price, override with -pot); measured probe burn
// ratios make the per-request allowance cost model-specific.
func runNow(planName string, pot float64, spentFile map[string]float64, spentGiven, paid, privacy, peak, vertical, jsonOut, tty bool) error {
	models, err := modelsdev.Fetch("opencode-go", false, false)
	if err != nil {
		return err
	}
	key, err := probe.Key()
	if err != nil {
		return err
	}
	ws, err := usage.Fetch(key)
	if err != nil {
		return err
	}
	budgets := usage.RemainingBudgets(ws, pot)

	// Real per-model spend for the current period, straight from the console
	// API with the same key; the -spent paste file overrides when given.
	spent := map[string]float64{}
	var liveErr error
	if !spentGiven {
		var mu []consoleapi.ModelSpend
		if mu, liveErr = consoleapi.FetchModelUsage(key); liveErr == nil {
			for _, s := range mu {
				spent[s.Model] = s.CostUSD
			}
		}
	} else {
		spent = spentFile
	}
	src := "console api"
	if spentGiven {
		src = "-spent file"
	} else if liveErr != nil {
		src = "unavailable: " + liveErr.Error()
	}

	fmt.Fprintf(os.Stderr, "# pot=$%.0f, remaining: 5h=$%.2f week=$%.2f month=$%.2f (per-model spend: %s)\n",
		pot, budgets.Five, budgets.Week, budgets.Month, src)

	var rows []nowRow
	for _, m := range models {
		c := cost.PerRequest(m, peak)
		if paid && c == 0 {
			continue
		}
		if privacy && goplan.TrainsOnData(m.ID) {
			continue
		}
		if c <= 0 {
			continue
		}
		mult := 1.0
		speed := 0.0
		if p, ok := cachedProbe(m.ID); ok && p.Error == "" {
			speed = p.TokPerSec
			if p.ListCost > 0 {
				mult = p.Allowance / p.ListCost
			}
		}
		b := budgets
		if allow, unlim := goplan.Allowance(m.ID, planName); !unlim {
			if _, known := spent[m.ID]; known || !spentGiven || liveErr == nil {
				mm := allow - spent[m.ID]
				b.ModelMonth = &mm
			}
		}
		five, week, month, usable, binding := usage.Requests(b, mult*c)
		rows = append(rows, nowRow{
			name: m.Name, costTask: c, burn: mult * c, spentMo: spent[m.ID],
			five: five, week: week, month: month, usable: usable,
			binding: binding, probeTokS: speed,
		})
	}
	slices.SortStableFunc(rows, func(a, b nowRow) int {
		if a.usable != b.usable {
			return b.usable - a.usable
		}
		return strings.Compare(a.name, b.name)
	})

	cells := func(r nowRow) []string {
		return []string{
			r.name, numMoney(r.costTask), numMoney(r.burn), numMoney(r.spentMo),
			strconv.Itoa(r.five), strconv.Itoa(r.week), strconv.Itoa(r.month),
			strconv.Itoa(r.usable), r.binding,
			numSpeed(r.probeTokS),
		}
	}
	head := []string{"#name", "cost_task", "burn_req", "spent_mo", "reqs_5h", "reqs_wk", "reqs_mo", "usable", "limit", "p_tok_s"}
	if jsonOut {
		type jsonNow struct {
			Name      string  `json:"name"`
			CostTask  float64 `json:"cost_task"`
			BurnReq   float64 `json:"burn_req"`
			SpentMo   float64 `json:"spent_mo"`
			Reqs5h    int     `json:"reqs_5h"`
			ReqsWk    int     `json:"reqs_wk"`
			ReqsMo    int     `json:"reqs_mo"`
			Usable    int     `json:"usable"`
			Limit     string  `json:"limit"`
			ProbeTokS float64 `json:"p_tok_s"`
		}
		out := make([]jsonNow, 0, len(rows))
		for _, r := range rows {
			out = append(out, jsonNow{
				Name: r.name, CostTask: r.costTask, BurnReq: r.burn, SpentMo: r.spentMo,
				Reqs5h: r.five, ReqsWk: r.week, ReqsMo: r.month,
				Usable: r.usable, Limit: r.binding, ProbeTokS: r.probeTokS,
			})
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	if vertical {
		if tty {
			for _, l := range nowLegend {
				fmt.Fprintln(os.Stdout, l)
			}
		}
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

func numMoney(v float64) string {
	if v == 0 {
		return ""
	}
	s := strconv.FormatFloat(v, 'f', 5, 64)
	s = strings.TrimRight(s, "0")
	return "$" + strings.TrimSuffix(s, ".")
}

func numSpeed(v float64) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

func runOneProbe(modelID string, refresh bool) error {
	key, err := probe.Key()
	if err != nil {
		return err
	}
	r := loadOrProbe(modelID, key, refresh)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
