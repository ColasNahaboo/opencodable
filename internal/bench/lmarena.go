package bench

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ColasNahaboo/opencodable/internal/cache"
	"github.com/ColasNahaboo/opencodable/internal/normalize"
)

// eloBaseURL serves daily snapshots of every Arena leaderboard as static
// JSON (third-party mirror, MIT): data/latest.json points at the newest
// snapshot, each dated directory holds one file per leaderboard.
const eloBaseURL = "https://raw.githubusercontent.com/oolong-tea-2026/arena-ai-leaderboards/main/data"

// btrBaseURL serves the official Arena leaderboard history (CC-BY-4.0) as
// keyless JSON. text_style_control is the site default; latest is the
// current leaderboard. Bradley-Terry scale (~±0.2), not Elo.
const btrBaseURL = "https://datasets-server.huggingface.co/rows?dataset=lmarena-ai/leaderboard-dataset&config=text_style_control&split=latest"

// eloWalkbackDays caps the snapshot search: the mirror has gaps (empty
// snapshots), so the newest non-empty text.json wins, at most this far back.
const eloWalkbackDays = 31

// btrPageSize is the datasets-server page length for the offline walk.
// btrMaxPages bounds it: overall rows lead the split (verified Oct 2026),
// so the walk stops at the first overall-free page; the cap is the backstop
// if that ordering ever changes.
const btrPageSize = 100
const btrMaxPages = 10

// Elo is Arena Elo per model base key (text leaderboard).
type Elo map[string]float64

// BTR is the official Bradley-Terry rating per model base key (text with
// style control, overall category). Negatives are real (below average).
type BTR map[string]float64

// eloAliases maps models.dev IDs to arena base keys where plain matching
// fails. A blank elo cell means no match: extend here as gaps surface.
var eloAliases = map[string]string{}

// btrAliases maps models.dev IDs to HF base keys. Dated snapshots rot:
// bump the value when Arena renames the measured row.
var btrAliases = map[string]string{
	"deepseek-v4.1-flash": "deepseekv41flash20260910", // dated snapshot = measured one
}

// arenaEffortSuffixes are leaderboard effort variants, longest first so
// -xhigh strips before -high. Matched pre-Key with dashes intact, so plain
// words ending in low/high (flow, …) never strip.
var arenaEffortSuffixes = []string{"-xhigh", "-high", "-medium", "-low"}

// planSuffixes are subscription variants of the same weights.
var planSuffixes = []string{"-contributor", "-free"}

// arenaBaseKey reduces an arena model name to its comparable base:
// parenthetical tags ("Kimi K3 (Max)") and dash effort suffixes
// ("gpt-5.6-sol-xhigh") go away; the rest is normalize.Key. The best score
// among a family's variants wins (documented bias, as with SWE-bench).
func arenaBaseKey(name string) string {
	if i := strings.Index(name, "("); i >= 0 {
		name = name[:i]
	}
	n := strings.ToLower(strings.TrimSpace(name))
	for _, suf := range arenaEffortSuffixes {
		if strings.HasSuffix(n, suf) {
			n = strings.TrimSuffix(n, suf)
			break
		}
	}
	return normalize.Key(n)
}

// stripPlan removes subscription suffixes before matching.
func stripPlan(id string) string {
	for _, suf := range planSuffixes {
		id = strings.TrimSuffix(id, suf)
	}
	return id
}

// Lookup resolves a models.dev ID to its Arena Elo.
func (m Elo) Lookup(id string) (float64, bool) {
	if v, ok := m[normalize.Key(stripPlan(id))]; ok {
		return v, true
	}
	if base, ok := eloAliases[id]; ok {
		if v, ok := m[base]; ok {
			return v, true
		}
	}
	return 0, false
}

// Lookup resolves a models.dev ID to its Bradley-Terry rating.
func (m BTR) Lookup(id string) (float64, bool) {
	if v, ok := m[normalize.Key(stripPlan(id))]; ok {
		return v, true
	}
	if base, ok := btrAliases[id]; ok {
		if v, ok := m[base]; ok {
			return v, true
		}
	}
	return 0, false
}

// minNearPrefix is the shortest shared prefix that counts as a near miss.
const minNearPrefix = 5

// Near suggests the leaderboard key that most likely belongs to id when id has
// no match: one key is a prefix of the other, as with a Max variant or a
// shortened family name (glm53 for GLM-5.3-Flash). The shorter key must have
// at least minNearPrefix characters and a digit. It is a hint for a likely
// alias gap to check, never a match; "" when none.
func (m Elo) Near(id string) string { return nearKey(m, id) }

// Near is the BTR counterpart of Elo.Near.
func (m BTR) Near(id string) string { return nearKey(m, id) }

func nearKey(m map[string]float64, id string) string {
	k := normalize.Key(stripPlan(id))
	best := ""
	for lk := range m {
		short, long := k, lk
		if len(lk) < len(k) {
			short, long = lk, k
		}
		if lk == k || len(short) < minNearPrefix || !hasDigit(short) || !strings.HasPrefix(long, short) {
			continue
		}
		if best == "" || lenGap(lk, k) < lenGap(best, k) || (lenGap(lk, k) == lenGap(best, k) && lk < best) {
			best = lk
		}
	}
	return best
}

// lenGap is the difference in length between two keys; the closest length wins.
func lenGap(a, b string) int {
	if len(a) > len(b) {
		return len(a) - len(b)
	}
	return len(b) - len(a)
}

func hasDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

type eloPointer struct {
	Path string `json:"path"`
}

type eloSnapshot struct {
	Models []struct {
		Model string  `json:"model"`
		Score float64 `json:"score"`
	} `json:"models"`
}

// FetchElo returns Arena Elo per base key from the newest non-empty daily
// snapshot, with a 24h disk cache.
func FetchElo(refresh, offline bool) (Elo, error) {
	b, err := cache.Load("lmarena-elo.json", 24*time.Hour, refresh, offline, func() ([]byte, error) {
		raw, err := resolveEloSnapshot(get)
		if err != nil {
			return nil, err
		}
		m := parseElo(raw)
		if len(m) == 0 {
			return nil, errors.New("lmarena: no usable rows")
		}
		return json.Marshal(m)
	})
	if err != nil {
		return nil, err
	}
	var m Elo
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// resolveEloSnapshot finds the newest non-empty text.json at most
// eloWalkbackDays back. A 404 means "try older"; any other error aborts, so
// a dead network fails fast instead of walking days of timeouts.
func resolveEloSnapshot(fetch func(string) ([]byte, error)) ([]byte, error) {
	pb, err := fetch(eloBaseURL + "/latest.json")
	if err != nil {
		return nil, err
	}
	var p eloPointer
	if err := json.Unmarshal(pb, &p); err != nil {
		return nil, err
	}
	day, err := time.Parse("2006-01-02", p.Path)
	if err != nil {
		return nil, fmt.Errorf("lmarena: bad snapshot path %q", p.Path)
	}
	var firstErr error
	for i := 0; i < eloWalkbackDays; i++ {
		d := day.AddDate(0, 0, -i).Format("2006-01-02")
		raw, err := fetch(eloBaseURL + "/" + d + "/text.json")
		if err != nil {
			if strings.Contains(err.Error(), "HTTP 404") {
				continue
			}
			return nil, err
		}
		if len(parseElo(raw)) > 0 {
			return raw, nil
		}
		if firstErr == nil {
			firstErr = fmt.Errorf("lmarena: empty snapshot %s", d)
		}
	}
	if firstErr == nil {
		firstErr = errors.New("lmarena: no snapshot found")
	}
	return nil, firstErr
}

// parseElo extracts the best score per base key; blank names and null or
// zero scores are skipped (Elo is never ≤ 0 when measured).
func parseElo(b []byte) Elo {
	var s eloSnapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil
	}
	out := Elo{}
	for _, m := range s.Models {
		if m.Model == "" || m.Score <= 0 {
			continue
		}
		if k := arenaBaseKey(m.Model); k != "" {
			if cur, ok := out[k]; !ok || m.Score > cur {
				out[k] = m.Score
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type btrRow struct {
	ModelName string   `json:"model_name"`
	Category  string   `json:"category"`
	Rating    *float64 `json:"rating"`
	Score     *float64 `json:"score"`
}

type btrPage struct {
	Rows []struct {
		Row btrRow `json:"row"`
	} `json:"rows"`
}

// FetchBTRating returns the official Bradley-Terry rating per base key,
// overall category only, walking every page. Pointers tell present-but-zero
// (exactly average) apart from absent.
func FetchBTRating(refresh, offline bool) (BTR, error) {
	b, err := cache.Load("lmarena-btr.json", 24*time.Hour, refresh, offline, func() ([]byte, error) {
		out := BTR{}
		seen := 0
		for p := 0; p < btrMaxPages; p++ {
			raw, err := get(fmt.Sprintf("%s&offset=%d&length=%d", btrBaseURL, p*btrPageSize, btrPageSize))
			if err != nil {
				return nil, err
			}
			n, overall := mergeBTR(out, raw)
			if n < 0 {
				return nil, errors.New("lmarena: bad btr page")
			}
			seen += overall
			if n < btrPageSize || (seen > 0 && overall == 0) {
				break
			}
		}
		if len(out) == 0 {
			return nil, errors.New("lmarena: no usable rows")
		}
		return json.Marshal(out)
	})
	if err != nil {
		return nil, err
	}
	var m BTR
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// mergeBTR adds one page's overall rows, best per base wins. Returns the
// page's row count and its overall count, or -1 on unparsable input.
func mergeBTR(out BTR, raw []byte) (int, int) {
	var p btrPage
	if err := json.Unmarshal(raw, &p); err != nil {
		return -1, 0
	}
	overall := 0
	for _, r := range p.Rows {
		if r.Row.Category != "overall" || r.Row.ModelName == "" {
			continue
		}
		overall++
		v, ok := btrValue(r.Row)
		if !ok {
			continue
		}
		if k := arenaBaseKey(r.Row.ModelName); k != "" {
			if cur, ok := out[k]; !ok || v > cur {
				out[k] = v
			}
		}
	}
	return len(p.Rows), overall
}

// btrValue prefers rating, falls back to score; a row with neither is missing.
func btrValue(r btrRow) (float64, bool) {
	if r.Rating != nil {
		return *r.Rating, true
	}
	if r.Score != nil {
		return *r.Score, true
	}
	return 0, false
}
