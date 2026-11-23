package goplan

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ColasNahaboo/opencodable/internal/cache"
	"github.com/ColasNahaboo/opencodable/internal/modelsdev"
)

// Live plan snapshots: the Go models endpoint and the console Go page,
// parsed and validated at fetch time. The compiled-in table stays as the
// offline fallback; Use installs a live snapshot over it.
//
// The models endpoint carries every model's token prices, peak-hour
// multipliers, per-plan monthly usage limits and data-training flag. The
// console page carries the per-model estimated requests per 5 hours, which
// grounds the official allowance cost per request (a 5-hour window covers
// 20% of the monthly allowance).

const (
	endpointURL = "https://opencode.ai/inference/go/v1/models"
	consoleURL  = "https://opencode.ai/console/go"
	cacheKey    = "goplan-live.json"
)

// LiveRow is one model's plan data from the live sources.
type LiveRow struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	GoAllowance   float64     `json:"go_allowance"` // USD; 0 when free
	PlusAllowance float64     `json:"plus_allowance"`
	Unlimited     bool        `json:"unlimited"` // free models
	In            float64     `json:"in"`        // off-peak USD/1M
	Out           float64     `json:"out"`
	CacheRead     float64     `json:"cache_read"`
	Factor        float64     `json:"factor"`         // endpoint peak multiplier (0 = flat pricing)
	Peak          *[3]float64 `json:"peak,omitempty"` // input, output, cache_read = base × factor
	Training      bool        `json:"training"`       // trains on request data
	Reqs5h        float64     `json:"reqs_5h"`        // console estimated requests per 5 hours, Go plan
	Reqs5hPlus    float64     `json:"reqs_5h_plus"`   // same, Go Plus plan
	ReqsMo        float64     `json:"reqs_mo"`        // Reqs5h scaled to a month (×5)
	CostReq       float64     `json:"cost_req"`       // official allowance cost per request, USD
}

// LiveSnapshot is a validated live plan snapshot.
type LiveSnapshot struct {
	FetchedAt time.Time           `json:"fetched_at"`
	Source    string              `json:"source"`
	Rows      map[string]*LiveRow `json:"rows"`
}

var (
	trRe     = regexp.MustCompile(`(?s)<tr[^>]*>(.*?)</tr>`)
	planRe   = regexp.MustCompile(`data-plan="([\w-]+)"[^>]*>\s*<span[^>]*data-layer="current-value">([^<]*)<`)
	nameRe   = regexp.MustCompile(`data-layer="model-name"[^>]*title="([^"]*)"`)
	tableRe  = regexp.MustCompile(`(?s)<table.*?</table>`)
	nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)
)

// normKey lower-cases a display name and drops everything but letters and
// digits, so "LongCat-2.5" (endpoint) matches "LongCat 2.5" (console).
func normKey(s string) string { return nonAlnum.ReplaceAllString(strings.ToLower(s), "") }

func parseCount(s string) (float64, error) {
	return strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 64)
}

func parseMoney(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0, fmt.Errorf("empty money value")
	}
	s = strings.NewReplacer("$", "", ",", "", " ", "").Replace(s)
	return strconv.ParseFloat(strings.TrimPrefix(s, "~"), 64)
}

// planValues extracts each plan's value from one console cell of the
// requests/usage kind. Unparseable values (empty, "∞") are skipped.
func planValues(seg string) map[string]float64 {
	out := map[string]float64{}
	for _, m := range planRe.FindAllStringSubmatch(seg, -1) {
		v, err := parseCount(m[2])
		if err != nil {
			if mv, merr := parseMoney(m[2]); merr == nil {
				v = mv
			} else {
				continue
			}
		}
		out[m[1]] = v
	}
	return out
}

// ConsoleRow is one console Go page model row.
type ConsoleRow struct {
	Name  string
	Reqs  map[string]float64 // plan → estimated requests per 5 hours
	Usage map[string]float64 // plan → monthly limit, USD
}

// ParseConsoleGo reads the model table of the console Go page: one row per
// model, with per-plan estimated requests per 5 hours and monthly limits.
func ParseConsoleGo(b []byte) (map[string]ConsoleRow, error) {
	var tbl string
	for _, t := range tableRe.FindAllString(string(b), -1) {
		if strings.Contains(t, `data-layer="model-name"`) {
			tbl = t
			break
		}
	}
	if tbl == "" {
		return nil, fmt.Errorf("console page: models table not found")
	}
	out := map[string]ConsoleRow{}
	for _, r := range trRe.FindAllStringSubmatch(tbl, -1) {
		row := r[1]
		nm := nameRe.FindStringSubmatch(row)
		if nm == nil {
			continue // header or spacer row
		}
		c := ConsoleRow{Name: nm[1], Reqs: map[string]float64{}, Usage: map[string]float64{}}
		if i := strings.Index(row, `data-layer="requests"`); i >= 0 {
			seg := row[i:]
			if j := strings.Index(seg, `data-layer="usage"`); j > 0 {
				seg = seg[:j]
			}
			c.Reqs = planValues(seg)
		}
		if i := strings.Index(row, `data-layer="usage"`); i >= 0 {
			c.Usage = planValues(row[i:])
		}
		if len(c.Reqs) == 0 && len(c.Usage) == 0 {
			continue
		}
		out[normKey(c.Name)] = c
	}
	if len(out) < 10 {
		return nil, fmt.Errorf("console page: parsed only %d model rows", len(out))
	}
	return out, nil
}

// EndpointModel is one model's live data from the Go models endpoint.
type EndpointModel struct {
	ID            string
	Name          string
	GoAllowance   float64
	PlusAllowance float64
	Unlimited     bool
	In            float64
	Out           float64
	CacheRead     float64
	Factor        float64
	Training      bool
}

type endpointDoc struct {
	Data []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Cost struct {
			Text struct {
				Input struct {
					Price     float64 `json:"price"`
					CacheRead struct {
						Read float64 `json:"read"`
					} `json:"cache"`
				} `json:"input"`
				Output struct {
					Price float64 `json:"price"`
				} `json:"output"`
			} `json:"text"`
			Multipliers []struct {
				Factor float64 `json:"factor"`
			} `json:"multipliers"`
		} `json:"cost"`
		Go *struct {
			MonthlyUsage map[string]float64 `json:"monthly_usage"`
			Training     bool               `json:"training"`
		} `json:"go"`
	} `json:"data"`
}

// ParseEndpointModels reads the Go models endpoint JSON.
func ParseEndpointModels(b []byte) (map[string]*EndpointModel, error) {
	var d endpointDoc
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("models endpoint: %w", err)
	}
	if len(d.Data) < 10 {
		return nil, fmt.Errorf("models endpoint: only %d models", len(d.Data))
	}
	out := map[string]*EndpointModel{}
	for _, m := range d.Data {
		e := &EndpointModel{
			ID:        m.ID,
			Name:      m.Name,
			In:        m.Cost.Text.Input.Price,
			CacheRead: m.Cost.Text.Input.CacheRead.Read,
			Out:       m.Cost.Text.Output.Price,
			Training:  m.Go != nil && m.Go.Training,
		}
		for _, mu := range m.Cost.Multipliers {
			if mu.Factor > e.Factor {
				e.Factor = mu.Factor
			}
		}
		if g := m.Go; g != nil {
			e.GoAllowance = g.MonthlyUsage["go"]
			e.PlusAllowance = g.MonthlyUsage["go-plus"]
		}
		// Free models have no usage block and zero prices.
		e.Unlimited = m.Go == nil ||
			(e.GoAllowance == 0 && e.PlusAllowance == 0 && e.In == 0 && e.Out == 0)
		out[m.ID] = e
	}
	return out, nil
}

// Merge joins the endpoint and console data into a snapshot keyed by
// models.dev model ID. A 5-hour window covers 20% of the monthly allowance,
// so the official allowance cost per request is that window's budget divided
// by the console's estimated requests for it.
func Merge(ep map[string]*EndpointModel, console map[string]ConsoleRow) (*LiveSnapshot, error) {
	snap := &LiveSnapshot{FetchedAt: time.Now(), Source: endpointURL, Rows: map[string]*LiveRow{}}
	for id, m := range ep {
		row := &LiveRow{
			ID: id, Name: m.Name,
			GoAllowance: m.GoAllowance, PlusAllowance: m.PlusAllowance,
			Unlimited: m.Unlimited,
			In:        m.In, Out: m.Out, CacheRead: m.CacheRead,
			Factor: m.Factor, Training: m.Training,
		}
		if c, ok := console[normKey(m.Name)]; ok {
			row.Reqs5h = c.Reqs["go"]
			row.Reqs5hPlus = c.Reqs["go-plus"]
			// The console page and the endpoint must agree on the monthly
			// limits: a mismatch means one of the two changed shape.
			if got, want := c.Usage["go"], row.GoAllowance; got > 0 && want > 0 && !closeEnough(want, got) {
				return nil, fmt.Errorf("%s: console go limit %v != endpoint %v", id, got, want)
			}
			if got, want := c.Usage["go-plus"], row.PlusAllowance; got > 0 && want > 0 && !closeEnough(want, got) {
				return nil, fmt.Errorf("%s: console go-plus limit %v != endpoint %v", id, got, want)
			}
		}
		if !row.Unlimited {
			if m.Factor > 1 {
				row.Peak = &[3]float64{m.In * m.Factor, m.Out * m.Factor, m.CacheRead * m.Factor}
			}
			switch {
			case row.Reqs5h > 0 && row.GoAllowance > 0:
				row.CostReq = 0.2 * row.GoAllowance / row.Reqs5h
			case row.Reqs5hPlus > 0 && row.PlusAllowance > 0:
				row.CostReq = 0.2 * row.PlusAllowance / row.Reqs5hPlus
			default:
				return nil, fmt.Errorf("%s: no console estimated requests for a paid model", id)
			}
			row.ReqsMo = 5 * row.Reqs5h
		}
		snap.Rows[id] = row
	}
	if len(snap.Rows) < 10 {
		return nil, fmt.Errorf("models endpoint: only %d usable models", len(snap.Rows))
	}
	return snap, nil
}

// Fetch loads a validated live snapshot: cache first (7-day TTL by default),
// then the network. catalog is the models.dev catalog used for cross-source
// validation. Errors mean "keep using the compiled-in snapshot".
func Fetch(refresh, offline bool, ttl time.Duration, catalog []modelsdev.Model) (*LiveSnapshot, error) {
	payload, err := cache.Load(cacheKey, ttl, refresh, offline, func() ([]byte, error) {
		epB, err := httpGet(endpointURL)
		if err != nil {
			return nil, err
		}
		conB, err := httpGet(consoleURL)
		if err != nil {
			return nil, err
		}
		ep, err := ParseEndpointModels(epB)
		if err != nil {
			return nil, err
		}
		console, err := ParseConsoleGo(conB)
		if err != nil {
			return nil, err
		}
		snap, err := Merge(ep, console)
		if err != nil {
			return nil, err
		}
		if errs := Validate(snap, catalog); len(errs) > 0 {
			return nil, fmt.Errorf("plan snapshot rejected: %s", errJoin(errs))
		}
		snap.FetchedAt = time.Now()
		return json.Marshal(snap)
	})
	if err != nil {
		return nil, err
	}
	var snap LiveSnapshot
	if err := json.Unmarshal(payload, &snap); err != nil {
		return nil, fmt.Errorf("cached plan snapshot: %w", err)
	}
	if len(snap.Rows) == 0 {
		return nil, fmt.Errorf("cached plan snapshot has no rows")
	}
	if !hasCostReq(&snap) {
		return nil, fmt.Errorf("cached plan snapshot predates the console request estimates")
	}
	return &snap, nil
}

// hasCostReq reports whether the snapshot carries derived per-request costs.
// Snapshots from the docs-page era (before the console page became the
// requests source) do not, and must not be used as the live overlay.
func hasCostReq(snap *LiveSnapshot) bool {
	for _, r := range snap.Rows {
		if r.CostReq > 0 {
			return true
		}
	}
	return false
}

func httpGet(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "opencodable/plan-refresh")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	return jsonReadAll(resp.Body)
}
