package bench

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/ColasNahaboo/opencodable/internal/cache"
	"github.com/ColasNahaboo/opencodable/internal/normalize"
)

// LiveBenchRelease pins the LiveBench 2.0 leaderboard release. Bump it when
// LiveBench publishes a new one; the cache key follows, so old rows are not
// silently reused.
const LiveBenchRelease = "20261025"

const livebenchURL = "https://livebench.ai/data/" + LiveBenchRelease + "/table.csv"

// Score is one model's LiveBench 2.0 suites. Both are 0-100, single run.
type Score struct {
	Agentic float64 // long-horizon agentic work
	Coding  float64 // bug-fix tasks from real merged pull requests
}

// LB is the LiveBench 2.0 leaderboard keyed by normalize.Key of the model
// slug, so dots and dashes resolve alike.
type LB map[string]Score

// liveBenchAliases maps models.dev IDs to the LiveBench slug when the plain
// key does not match: dated snapshots and plan variants of the same weights.
var liveBenchAliases = map[string]string{
	"deepseek-v4.1-flash":        "deepseek-v4.1-flash-1017", // dated snapshot = measured one
	"gemini-4-argon-contributor": "gemini-4-argon",
	"minimax-m3.1-flash-free":    "minimax-m3.1-flash",
	"muse-spark-1.3-contributor": "muse-spark-1.3", // same weights, plan privacy verdict still applies
	"step-5-preview-free":        "step-5-preview",
}

// Lookup resolves a models.dev model ID to its LiveBench scores.
func (lb LB) Lookup(id string) (Score, bool) {
	if s, ok := lb[normalize.Key(id)]; ok {
		return s, true
	}
	if slug, ok := liveBenchAliases[id]; ok {
		s, ok := lb[normalize.Key(slug)]
		return s, ok
	}
	return Score{}, false
}

// LiveBenchCacheKey names the disk cache entry for the pinned LiveBench release.
func LiveBenchCacheKey() string {
	return "livebench-" + LiveBenchRelease + ".json"
}

// LiveBench fetches the pinned LiveBench 2.0 table (static CSV, no key) with
// a 24h disk cache. The CSV is parsed once and the scores are cached as JSON,
// because cache entries are JSON envelopes.
func LiveBench(refresh, offline bool) (LB, error) {
	b, err := cache.Load(LiveBenchCacheKey(), 24*time.Hour, refresh, offline, func() ([]byte, error) {
		raw, err := get(livebenchURL)
		if err != nil {
			return nil, err
		}
		lb := parseLiveBench(raw)
		if lb == nil {
			return nil, errors.New("livebench: no usable rows in " + livebenchURL)
		}
		return json.Marshal(lb)
	})
	if err != nil {
		return nil, err
	}
	var lb LB
	if err := json.Unmarshal(b, &lb); err != nil {
		return nil, err
	}
	return lb, nil
}

// parseLiveBench reads the release CSV. Columns are selected by name, so a
// renamed or dropped suite degrades to a missing score instead of a bad one.
func parseLiveBench(b []byte) LB {
	rd := csv.NewReader(bytes.NewReader(b))
	rd.FieldsPerRecord = -1
	recs, err := rd.ReadAll()
	if err != nil || len(recs) < 2 {
		return nil
	}
	idx := map[string]int{}
	for i, h := range recs[0] {
		idx[strings.TrimSpace(h)] = i
	}
	mi, ok := idx["model"]
	if !ok {
		return nil
	}
	ai, aok := idx["long_agentic"]
	ci, cok := idx["coding"]
	if !aok && !cok {
		return nil
	}
	out := LB{}
	for _, rec := range recs[1:] {
		if mi >= len(rec) {
			continue
		}
		model := strings.TrimSpace(rec[mi])
		if model == "" {
			continue
		}
		var s Score
		if aok && ai < len(rec) {
			s.Agentic = parseScore(rec[ai])
		}
		if cok && ci < len(rec) {
			s.Coding = parseScore(rec[ci])
		}
		out[normalize.Key(model)] = s
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseScore accepts a 0-100 cell, treating blanks and "n/a" as missing.
func parseScore(cell string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(cell), 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}
