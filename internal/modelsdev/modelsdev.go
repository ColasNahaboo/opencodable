// Package modelsdev fetches and caches the models.dev catalog: per-provider
// model metadata with USD prices per 1M tokens and context limits.
package modelsdev

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"opencodable/internal/cache"
)

const catalogURL = "https://models.dev/api.json"

// Cost is USD per 1M tokens.
type Cost struct {
	Input      float64    `json:"input"`
	Output     float64    `json:"output"`
	CacheRead  float64    `json:"cache_read"`
	CacheWrite float64    `json:"cache_write"`
	Tiers      []TierCost `json:"tiers"`
}

// TierCost holds prices applying above a context-size threshold.
type TierCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
	Tier       struct {
		Type string `json:"type"`
		Size int64  `json:"size"`
	} `json:"tier"`
}

// Model is one entry of the models.dev catalog.
type Model struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Reasoning bool    `json:"reasoning"`
	Cost      Cost    `json:"cost"`
	Limit     Context `json:"limit"`
}

// Context is the token-limit block of a model.
type Context struct {
	Context int64 `json:"context"`
	Input   int64 `json:"input"`
	Output  int64 `json:"output"`
}

type catalog map[string]struct {
	Models map[string]Model `json:"models"`
}

// Fetch returns the models of one provider, using a 24h disk cache.
func Fetch(provider string, refresh, offline bool) ([]Model, error) {
	b, err := cache.Load("modelsdev-"+provider+".json", 24*time.Hour, refresh, offline,
		func() ([]byte, error) {
			resp, err := http.Get(catalogURL)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("models.dev: HTTP %d", resp.StatusCode)
			}
			return io.ReadAll(resp.Body)
		})
	if err != nil {
		return nil, err
	}
	var cat catalog
	if err := json.Unmarshal(b, &cat); err != nil {
		return nil, err
	}
	var out []Model
	for id, m := range cat[provider].Models {
		m.ID = id
		out = append(out, m)
	}
	return out, nil
}
