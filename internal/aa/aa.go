// Package aa fetches the Artificial Analysis data API: benchmark scores,
// pricing and measured speed for every model they track.
package aa

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ColasNahaboo/opencodable/internal/cache"
	"github.com/ColasNahaboo/opencodable/internal/config"
)

const modelsURL = "https://artificialanalysis.ai/api/v2/data/llms/models"

// Model is one AA entry. Null scores decode as 0.
type Model struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Creator struct {
		Name string `json:"name"`
	} `json:"model_creator"`
	Evaluations struct {
		Intel    float64  `json:"artificial_analysis_intelligence_index"`
		Coding   *float64 `json:"artificial_analysis_coding_index"`
		Math     *float64 `json:"artificial_analysis_math_index"`
		LiveCode *float64 `json:"livecodebench"`
		Terminal *float64 `json:"terminalbench_v4_0"`
		Term2    *float64 `json:"terminalbench_v2_1"`
		IFBench  *float64 `json:"ifbench"`
		LCR      *float64 `json:"lcr"`
	} `json:"evaluations"`
	PriceOut    float64 `json:"price_1m_output_tokens"`
	TokPerSec   float64 `json:"median_output_tokens_per_second"`
	TTFTSeconds float64 `json:"median_time_to_first_token_seconds"`
}

// Fetch loads all AA LLM rows, using a 24h disk cache. The key comes from
// AA_API_KEY (environment or opencodable.conf) or the legacy
// .artificialanalysis.key dot-file. The key is only needed for a network
// fetch: -offline can use an existing cache without one.
func Fetch(refresh, offline bool) ([]Model, error) {
	key, _ := config.Value("AA_API_KEY")
	if key == "" {
		if b, err := os.ReadFile(".artificialanalysis.key"); err == nil {
			key = strings.TrimSpace(string(b))
		}
	}
	b, err := cache.Load("aa-models.json", 24*time.Hour, refresh, offline, func() ([]byte, error) {
		if key == "" {
			return nil, fmt.Errorf("no Artificial Analysis key: set AA_API_KEY (env or opencodable.conf) or create .artificialanalysis.key")
		}
		req, err := http.NewRequest(http.MethodGet, modelsURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-api-key", key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("artificialanalysis: HTTP %d", resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	})
	if err != nil {
		return nil, err
	}
	var out struct {
		Data []Model `json:"data"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}
