// Package bench joins external, non-AA benchmark feeds.
package bench

import (
	"encoding/json"
	"time"

	"github.com/ColasNahaboo/opencodable/internal/cache"
	"github.com/ColasNahaboo/opencodable/internal/normalize"
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

// LookupSWE resolves a model's SWE-bench Verified score by checking the
// plan-stripped models.dev ID, full ID, display name, and matched AA slug.
// Keeping the ID/name lookup independent of AA ensures SWE-bench scores still
// resolve when AA_API_KEY is absent or when AA uses a dated snapshot slug.
func LookupSWE(swe map[string]float64, id, name, aaSlug string) float64 {
	if len(swe) == 0 {
		return 0
	}
	for _, s := range []string{stripPlan(id), id, name, aaSlug} {
		if s == "" {
			continue
		}
		if v := swe[normalize.Key(s)]; v > 0 {
			return v
		}
	}
	return 0
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
