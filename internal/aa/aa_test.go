package aa

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const freePage1 = `{"tier":"free","pagination":{"page":1,"page_size":200,"total_pages":2,"has_more":true},"data":[{
	"id":"id-1","name":"Model One","slug":"model-one","release_date":"2026-10-01",
	"model_creator":{"id":"c-1","name":"Creator"},
	"evaluations":{"artificial_analysis_intelligence_index":42.5,"artificial_analysis_coding_index":null},
	"pricing":{"price_1m_input_tokens":1.4,"price_1m_output_tokens":4.4,"price_1m_cache_hit_tokens":0.26},
	"performance":{"median_output_tokens_per_second":80.07,"median_time_to_first_token_seconds":2.22,
		"median_time_to_first_answer_token_seconds":27.19,"median_end_to_end_response_time_seconds":33.44}
}]}`

const freePage2 = `{"tier":"free","pagination":{"page":2,"page_size":200,"total_pages":2,"has_more":false},"data":[{
	"id":"id-2","name":"Model Two","slug":"model-two",
	"evaluations":{"artificial_analysis_intelligence_index":31.7,
		"terminalbench_v4_0":0.419,"terminalbench_v2_1":0.839,"ifbench":0.7,"aa_lcr":0.797},
	"pricing":{"price_1m_input_tokens":2,"price_1m_output_tokens":10},
	"performance":{"median_output_tokens_per_second":61.2,"median_time_to_first_token_seconds":3.1}
}]}`

// aaServer serves two pages on both the Free and Pro paths and records every
// path it receives.
func aaServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Errorf("x-api-key = %q, want test-key", got)
		}
		switch r.URL.Query().Get("page") {
		case "1":
			fmt.Fprint(w, freePage1)
		case "2":
			fmt.Fprint(w, freePage2)
		default:
			http.Error(w, "bad page", http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), paths...)
	}
}

func TestFetchPaginatesAndParsesV2(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AA_API_KEY", "test-key")
	srv, paths := aaServer(t)
	oldFree, oldPro := modelsFreeURL, modelsProURL
	modelsFreeURL, modelsProURL = srv.URL+"/free", srv.URL+"/pro"
	t.Cleanup(func() { modelsFreeURL, modelsProURL = oldFree, oldPro })

	models, err := Fetch(false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2 (both pages)", len(models))
	}
	m1 := models[0]
	if m1.Slug != "model-one" || m1.Evaluations.Intel != 42.5 {
		t.Errorf("page 1 model = %+v", m1)
	}
	if m1.Evaluations.Coding != nil {
		t.Errorf("null coding index must stay nil: %v", *m1.Evaluations.Coding)
	}
	if m1.Pricing.Output != 4.4 {
		t.Errorf("nested pricing = %v, want 4.4", m1.Pricing.Output)
	}
	if m1.Performance.TokPerSec != 80.07 || m1.Performance.TTFTSeconds != 2.22 {
		t.Errorf("nested performance = %+v", m1.Performance)
	}
	m2 := models[1]
	if m2.Performance.TokPerSec != 61.2 {
		t.Errorf("page 2 speed = %v, want 61.2", m2.Performance.TokPerSec)
	}
	if m2.Evaluations.Terminal == nil || *m2.Evaluations.Terminal != 0.419 {
		t.Errorf("terminalbench = %v", m2.Evaluations.Terminal)
	}
	if m2.Evaluations.Term2 == nil || *m2.Evaluations.Term2 != 0.839 {
		t.Errorf("terminalbench v2.1 = %v", m2.Evaluations.Term2)
	}
	if m2.Evaluations.IFBench == nil || *m2.Evaluations.IFBench != 0.7 {
		t.Errorf("ifbench = %v", m2.Evaluations.IFBench)
	}
	if m2.Evaluations.LCR == nil || *m2.Evaluations.LCR != 0.797 {
		t.Errorf("aa_lcr = %v", m2.Evaluations.LCR)
	}
	if got := paths(); len(got) != 2 || got[0] != "/free" || got[1] != "/free" {
		t.Errorf("fetched paths = %v, want two /free pages", got)
	}

	// A second load within the TTL is served from disk: no new requests.
	if _, err := Fetch(false, false, false); err != nil {
		t.Fatal(err)
	}
	if got := paths(); len(got) != 2 {
		t.Errorf("cache hit fetched again: %v", got)
	}
	// Offline uses the same cache.
	if _, err := Fetch(false, true, false); err != nil {
		t.Fatal(err)
	}
	if got := paths(); len(got) != 2 {
		t.Errorf("offline fetched again: %v", got)
	}
}

func TestFetchProAndFreeCachesAreSeparate(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AA_API_KEY", "test-key")
	srv, paths := aaServer(t)
	oldFree, oldPro := modelsFreeURL, modelsProURL
	modelsFreeURL, modelsProURL = srv.URL+"/free", srv.URL+"/pro"
	t.Cleanup(func() { modelsFreeURL, modelsProURL = oldFree, oldPro })

	if CacheKey(true) == CacheKey(false) {
		t.Fatal("Free and Pro must not share a cache entry")
	}
	if _, err := Fetch(false, false, true); err != nil {
		t.Fatal(err)
	}
	if got := paths(); len(got) != 2 || got[0] != "/pro" || got[1] != "/pro" {
		t.Errorf("Pro paths = %v, want two /pro pages", got)
	}
	if _, err := Fetch(false, false, false); err != nil {
		t.Fatal(err)
	}
	if got := paths(); len(got) != 4 || got[2] != "/free" || got[3] != "/free" {
		t.Errorf("Free after Pro paths = %v, want two /free pages appended", got)
	}
}

func TestFetchHTTPError(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AA_API_KEY", "test-key")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"subscription required"}`, http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	oldFree, oldPro := modelsFreeURL, modelsProURL
	modelsFreeURL, modelsProURL = srv.URL, srv.URL
	t.Cleanup(func() { modelsFreeURL, modelsProURL = oldFree, oldPro })

	_, err := Fetch(false, false, true)
	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "subscription required") {
		t.Errorf("Fetch error = %v, want HTTP 403 with the body", err)
	}
}

func TestFetchNoKey(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AA_API_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("missing key must not reach the network: %s", r.URL)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	oldFree, oldPro := modelsFreeURL, modelsProURL
	modelsFreeURL, modelsProURL = srv.URL, srv.URL
	t.Cleanup(func() { modelsFreeURL, modelsProURL = oldFree, oldPro })

	_, err := Fetch(false, false, false)
	if err == nil || !strings.Contains(err.Error(), "no Artificial Analysis key") {
		t.Errorf("Fetch error = %v, want a missing-key error", err)
	}
}
