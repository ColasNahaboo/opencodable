// Package bench joins external, non-AA benchmark feeds.
package bench

import (
	"encoding/json"
	"time"

	"opencodable/internal/cache"
	"opencodable/internal/normalize"
)

const swebenchURL = "https://raw.githubusercontent.com/SWE-bench/swe-bench.github.io/master/data/leaderboards.json"

type sweResult struct {
	ModelDisplay string  `json:"model_display"`
	Agent        string  `json:"agent"`
	Resolved     float64 `json:"resolved"`
	Date         string  `json:"date"`
}

type sweBoard struct {
	Leaderboards []struct {
		Name    string      `json:"name"`
		Results []sweResult `json:"results"`
	} `json:"leaderboards"`
}

// SWEBench fetches SWE-bench Verified % resolved, keyed by
// normalize.Key of the display name. When a model has several runs with
// different agent scaffolds, the best recorded result wins (documented bias).
func SWEBench(refresh, offline bool) (map[string]float64, error) {
	b, err := cache.Load("swebench.json", 24*time.Hour, refresh, offline, func() ([]byte, error) {
		return get(swebenchURL)
	})
	if err != nil {
		return nil, err
	}
	return parseSWEBench(b), nil
}

// parseSWEBench extracts best-per-model Verified scores from the raw JSON.
func parseSWEBench(b []byte) map[string]float64 {
	var board sweBoard
	if err := json.Unmarshal(b, &board); err != nil {
		return nil
	}
	out := map[string]float64{}
	for _, lb := range board.Leaderboards {
		if lb.Name != "Verified" {
			continue
		}
		for _, r := range lb.Results {
			if r.ModelDisplay == "" || r.Resolved <= 0 {
				continue
			}
			k := normalize.Key(r.ModelDisplay)
			if r.Resolved > out[k] {
				out[k] = r.Resolved
			}
		}
	}
	return out
}
