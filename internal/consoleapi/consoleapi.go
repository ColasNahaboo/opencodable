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

const usageModelsURL = "https://console.opencode.ai/api/usage/models"

// ModelSpend is one model's usage over the current billing period.
type ModelSpend struct {
	Model    string  // models.dev model ID
	Requests int64   // requests this period
	CostUSD  float64 // list cost this period
}

type page struct {
	Items []struct {
		Model              string `json:"model"`
		Provider           string `json:"provider"`
		TotalRequests      string `json:"totalRequests"`
		TotalCostMicroCent string `json:"totalCostMicroCents"`
	} `json:"items"`
	PageInfo struct {
		Page      int64 `json:"page"`
		PageSize  int64 `json:"pageSize"`
		Total     int64 `json:"total"`
		PageCount int64 `json:"pageCount"`
	} `json:"pageInfo"`
}

// FetchModelUsage returns every model's spend for the current period,
// following pagination.
func FetchModelUsage(key string) ([]ModelSpend, error) {
	var out []ModelSpend
	for pageN := 1; ; pageN++ {
		var p page
		if err := get(fmt.Sprintf("%s?page=%d", usageModelsURL, pageN), key, &p); err != nil {
			return nil, err
		}
		for _, it := range p.Items {
			reqs, _ := strconv.ParseInt(it.TotalRequests, 10, 64)
			mc, _ := strconv.ParseInt(it.TotalCostMicroCent, 10, 64)
			out = append(out, ModelSpend{
				Model:    it.Model,
				Requests: reqs,
				CostUSD:  float64(mc) / 1e8, // microcents → dollars
			})
		}
		if int64(pageN) >= p.PageInfo.PageCount || len(p.Items) == 0 {
			return out, nil
		}
	}
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
