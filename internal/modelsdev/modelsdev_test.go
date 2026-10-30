package modelsdev

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchFiltersAndSortsModels(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		fmt.Fprint(w, `{
			"opencode-go": {
				"models": {
					"z-model": {"name": "Z Model", "cost": {"input": 1, "output": 2}},
					"a-free":  {"name": "A Free", "cost": {"input": 0, "output": 0}}
				}
			},
			"other": {
				"models": {
					"other-m": {"name": "Other"}
				}
			}
		}`)
	}))
	t.Cleanup(srv.Close)
	old := catalogURL
	catalogURL = srv.URL
	t.Cleanup(func() { catalogURL = old })

	models, err := Fetch("opencode-go", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2", len(models))
	}
	if models[0].ID != "a-free" || models[1].ID != "z-model" {
		t.Errorf("models not sorted by ID: %+v", models)
	}
	if !models[0].Free() || models[1].Free() {
		t.Errorf("Free() = %v/%v, want true/false", models[0].Free(), models[1].Free())
	}

	// Second call hits the 24h disk cache.
	if _, err := Fetch("opencode-go", false, true); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Errorf("hits = %d, want 1", hits)
	}
}
