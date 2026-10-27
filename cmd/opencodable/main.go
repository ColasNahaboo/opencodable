// Command opencodable ranks the models available in an OpenCode Go
// subscription by price, speed and quality: what is the best compromise
// right now.
package main

import (
	"flag"
	"fmt"
	"os"
	"slices"

	"opencodable/internal/aa"
	"opencodable/internal/modelsdev"
	"opencodable/internal/table"
)

func main() {
	var (
		provider = flag.String("provider", "opencode-go", "models.dev provider to rank")
		plan     = flag.String("plan", "go", "subscription plan: go|go-plus (allowances)")
		sortKey  = flag.String("sort", "quality", "sort: name|price|speed|quality|reqs")
		pareto   = flag.Bool("pareto", false, "only Pareto-optimal rows")
		paid     = flag.Bool("paid", false, "hide free models (cost_task = 0)")
		peak     = flag.Bool("peak", false, "use peak-hours prices (DeepSeek)")
		refresh  = flag.Bool("refresh", false, "bypass the 24h cache and refetch")
		offline  = flag.Bool("offline", false, "use cache only, no network")
		jsonOut  = flag.Bool("json", false, "JSON output instead of TSV")
	)
	flag.Parse()

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

	rows := table.Build(models, aas, *plan, *peak)
	if *paid {
		rows = slices.DeleteFunc(rows, func(r table.Row) bool { return r.CostTask == 0 })
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
