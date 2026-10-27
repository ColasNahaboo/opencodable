// Package consoleapi reads the OpenCode Console usage API. The Go
// subscription key authorizes it, so per-model spend is fetchable directly —
// no console browsing needed.
package consoleapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

var usageModelsURL = "https://console.opencode.ai/api/usage/models"

// ModelSpend is one model's usage over the current billing period.
type ModelSpend struct {
	Model              string  // models.dev model ID
	Provider           string  // "opencode-go", "opencode", ...
	Requests           int64   // requests this period
	InputTokens        int64   // uncached input tokens
	OutputTokens       int64   // output tokens (reasoning included)
	CacheReadTokens    int64   // cache-read tokens
	CacheWrite5mTokens int64   // 5-minute cache-write tokens
	CacheWrite1hTokens int64   // 1-hour cache-write tokens
	CostUSD            float64 // list cost this period, as reported
}

// CacheWriteTokens is the total cache-write tokens for the period.
func (m ModelSpend) CacheWriteTokens() int64 {
	return m.CacheWrite5mTokens + m.CacheWrite1hTokens
}

type page struct {
	Items []struct {
		Model                  string `json:"model"`
		Provider               string `json:"provider"`
		TotalRequests          string `json:"totalRequests"`
		TotalInputTokens       string `json:"totalInputTokens"`
		TotalOutputTokens      string `json:"totalOutputTokens"`
		TotalCacheReadTokens   string `json:"totalCacheReadTokens"`
		TotalCacheWrite5mToken string `json:"totalCacheWrite5mTokens"`
		TotalCacheWrite1hToken string `json:"totalCacheWrite1hTokens"`
		TotalCostMicroCent     string `json:"totalCostMicroCents"`
	} `json:"items"`
	PageInfo struct {
		Page      int64 `json:"page"`
		PageSize  int64 `json:"pageSize"`
		Total     int64 `json:"total"`
		PageCount int64 `json:"pageCount"`
	} `json:"pageInfo"`
}

// maxPages guards against a broken pagination response.
const maxPages = 1000

// FetchModelUsage returns every model's spend for the current period,
// following pagination.
func FetchModelUsage(key string) ([]ModelSpend, error) {
	var out []ModelSpend
	for pageN := 1; ; pageN++ {
		if pageN > maxPages {
			return nil, fmt.Errorf("usage pagination exceeded %d pages", maxPages)
		}
		var p page
		if err := get(fmt.Sprintf("%s?page=%d", usageModelsURL, pageN), key, &p); err != nil {
			return nil, err
		}
		for _, it := range p.Items {
			out = append(out, ModelSpend{
				Model:              it.Model,
				Provider:           it.Provider,
				Requests:           parseInt(it.TotalRequests),
				InputTokens:        parseInt(it.TotalInputTokens),
				OutputTokens:       parseInt(it.TotalOutputTokens),
				CacheReadTokens:    parseInt(it.TotalCacheReadTokens),
				CacheWrite5mTokens: parseInt(it.TotalCacheWrite5mToken),
				CacheWrite1hTokens: parseInt(it.TotalCacheWrite1hToken),
				CostUSD:            float64(parseInt(it.TotalCostMicroCent)) / 1e8, // microcents → dollars
			})
		}
		if int64(pageN) >= p.PageInfo.PageCount || len(p.Items) == 0 {
			return out, nil
		}
	}
}

func parseInt(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func get(url, key string, v any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
