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
	"time"

	"opencodable/internal/aa"
	"opencodable/internal/bench"
	"opencodable/internal/cache"
	"opencodable/internal/goplan"
	"opencodable/internal/modelsdev"
	"opencodable/internal/probe"
	"opencodable/internal/table"
)

func main() {
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
	if *jsonOut {
		if err := table.WriteJSON(os.Stdout, rows); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := table.WriteTSV(os.Stdout, rows); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
