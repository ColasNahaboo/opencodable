package consoleapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchModelUsageParsesTokensAndProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
			"items": [
				{"model":"deepseek-v4.1-flash","provider":"opencode-go",
				 "totalRequests":"6806","totalInputTokens":"49129857",
				 "totalOutputTokens":"7966752","totalCacheReadTokens":"1191377841",
				 "totalCacheWrite5mTokens":"10","totalCacheWrite1hTokens":"20",
				 "totalCostMicroCents":"1822189109"},
				{"model":"gpt-5.6-luna","provider":"opencode",
				 "totalRequests":"124","totalInputTokens":"0",
				 "totalOutputTokens":"0","totalCacheReadTokens":"0",
				 "totalCacheWrite5mTokens":"0","totalCacheWrite1hTokens":"0",
				 "totalCostMicroCents":"503793078"}
			],
			"pageInfo":{"page":1,"pageSize":10,"total":2,"pageCount":1}
		}`)
	}))
	t.Cleanup(srv.Close)
	old := usageModelsURL
	usageModelsURL = srv.URL
	defer func() { usageModelsURL = old }()

	rows, err := FetchModelUsage("key")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	got := rows[0]
	if got.Model != "deepseek-v4.1-flash" || got.Provider != "opencode-go" {
		t.Errorf("row identity = %s / %s", got.Model, got.Provider)
	}
	if got.Requests != 6806 || got.InputTokens != 49129857 ||
		got.OutputTokens != 7966752 || got.CacheReadTokens != 1191377841 {
		t.Errorf("token counts = %+v", got)
	}
	if got.CacheWriteTokens() != 30 {
		t.Errorf("cache write = %d, want 30", got.CacheWriteTokens())
	}
	if got.CostUSD < 18.22 || got.CostUSD > 18.23 {
		t.Errorf("cost = %v, want ~18.22", got.CostUSD)
	}
	if rows[1].Provider != "opencode" {
		t.Errorf("second provider = %s, want opencode", rows[1].Provider)
	}
}
