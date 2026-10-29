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

// modelsFreeURL and modelsProURL are vars so tests can point them at a fake
// server. The legacy /api/v2/data/llms/models endpoint retires on
// 2026-11-04; the Free route works with any key and the Pro route needs a
// Pro (or Commercial) subscription.
var (
	modelsFreeURL = "https://artificialanalysis.ai/api/v2/language/models/free"
	modelsProURL  = "https://artificialanalysis.ai/api/v2/language/models"
)

// maxPages guards against a broken pagination response. The Free tier allows
// 100 requests per 24h; 20 pages (4,000 models) is far above the current
// catalog and leaves quota for retries.
const maxPages = 20

// Model is one AA entry. Null scores decode as 0. Evaluations carries the
// Free headline indices plus the Pro per-benchmark fields; Coding, Math and
// LiveCodeBench are no longer published by Artificial Analysis.
type Model struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Creator struct {
		Name string `json:"name"`
	} `json:"model_creator"`
	Evaluations struct {
		Intel    float64  `json:"artificial_analysis_intelligence_index"`
		Coding   *float64 `json:"artificial_analysis_coding_index"`
		Math     *float64 `json:"artificial_analysis_math_index"` // legacy: no longer published
		LiveCode *float64 `json:"livecodebench"`                  // legacy: no longer published
		Terminal *float64 `json:"terminalbench_v4_0"`             // Pro tier only
		Term2    *float64 `json:"terminalbench_v2_1"`             // Pro tier only
		IFBench  *float64 `json:"ifbench"`                        // Pro tier only
		LCR      *float64 `json:"aa_lcr"`                         // Pro tier only
	} `json:"evaluations"`
	Pricing struct {
		Input      float64  `json:"price_1m_input_tokens"`
		Output     float64  `json:"price_1m_output_tokens"`
		CacheHit   float64  `json:"price_1m_cache_hit_tokens"`
		CacheWrite *float64 `json:"price_1m_cache_write_tokens"`
	} `json:"pricing"`
	Performance struct {
		TokPerSec   float64 `json:"median_output_tokens_per_second"`
		TTFTSeconds float64 `json:"median_time_to_first_token_seconds"`
		TTATSeconds float64 `json:"median_time_to_first_answer_token_seconds"`
		E2ESeconds  float64 `json:"median_end_to_end_response_time_seconds"`
	} `json:"performance"`
}

// CacheKey names the disk cache entry for the active endpoint. Free and Pro
// responses have different shapes, so they must not share a cache entry.
func CacheKey(pro bool) string {
	if pro {
		return "aa-models-v2-pro.json"
	}
	return "aa-models-v2-free.json"
}

// page is one paginated list response. The V2 API returns 200 models per
// page; follow has_more (and total_pages) until the last page.
type page struct {
	Tier       string `json:"tier"`
	Pagination struct {
		Page       int  `json:"page"`
		PageSize   int  `json:"page_size"`
		TotalPages int  `json:"total_pages"`
		HasMore    bool `json:"has_more"`
	} `json:"pagination"`
	Data []Model `json:"data"`
}

// Fetch loads all AA LLM rows, using a 24h disk cache. The key comes from
// AA_API_KEY (environment or opencodable.conf) or the legacy
// .artificialanalysis.key dot-file. The key is only needed for a network
// fetch: -offline can use an existing cache without one. pro selects the Pro
// endpoint, whose per-benchmark scores the Free endpoint omits.
func Fetch(refresh, offline, pro bool) ([]Model, error) {
	key, _ := config.Value("AA_API_KEY")
	if key == "" {
		if b, err := os.ReadFile(".artificialanalysis.key"); err == nil {
			key = strings.TrimSpace(string(b))
		}
	}
	b, err := cache.Load(CacheKey(pro), 24*time.Hour, refresh, offline, func() ([]byte, error) {
		if key == "" {
			return nil, fmt.Errorf("no Artificial Analysis key: set AA_API_KEY (env or opencodable.conf) or create .artificialanalysis.key")
		}
		return fetchAll(key, pro)
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

// fetchAll walks every page and caches one merged {"data": [...]} payload,
// so a later cache hit needs no network and no pagination state.
func fetchAll(key string, pro bool) ([]byte, error) {
	base := modelsFreeURL
	if pro {
		base = modelsProURL
	}
	var all []Model
	for pageN := 1; ; pageN++ {
		if pageN > maxPages {
			return nil, fmt.Errorf("artificialanalysis: pagination exceeded %d pages", maxPages)
		}
		var p page
		if err := get(fmt.Sprintf("%s?page=%d", base, pageN), key, &p); err != nil {
			return nil, err
		}
		all = append(all, p.Data...)
		if !p.Pagination.HasMore || len(p.Data) == 0 ||
			(p.Pagination.TotalPages > 0 && pageN >= p.Pagination.TotalPages) {
			break
		}
	}
	return json.Marshal(struct {
		Data []Model `json:"data"`
	}{Data: all})
}

func get(url, key string, v any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("artificialanalysis: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
