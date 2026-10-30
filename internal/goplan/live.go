package goplan

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ColasNahaboo/opencodable/internal/cache"
	"github.com/ColasNahaboo/opencodable/internal/modelsdev"
)

// Live plan snapshots: the docs page and the Go models endpoint, parsed and
// validated at fetch time. The compiled-in table above stays as the offline
// fallback; Use installs a live snapshot over it.

const (
	docsURL     = "https://opencode.ai/v2/docs/console/go/"
	endpointURL = "https://opencode.ai/inference/go/v1/models"
	cacheKey    = "goplan-live.json"
)

// LiveRow is one model's plan data from the live sources.
type LiveRow struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	GoAllowance   float64     `json:"go_allowance"` // USD; 0 when Unlimited
	PlusAllowance float64     `json:"plus_allowance"`
	Unlimited     bool        `json:"unlimited"`
	Profile       *Profile    `json:"profile,omitempty"`
	Peak          *[3]float64 `json:"peak,omitempty"` // input, output, cache_read
	Factor        float64     `json:"factor"`         // endpoint peak multiplier (0 = none)
	In            float64     `json:"in"`             // off-peak USD/1M
	Out           float64     `json:"out"`
	CacheRead     float64     `json:"cache_read"`
	ReqsMo        float64     `json:"reqs_mo"` // docs "requests per month" figure
}

// LiveSnapshot is a validated live plan snapshot.
type LiveSnapshot struct {
	FetchedAt time.Time           `json:"fetched_at"`
	Source    string              `json:"source"`
	Rows      map[string]*LiveRow `json:"rows"`
}

// PricingRow is one pricing-table row before ID resolution.
type PricingRow struct {
	Name        string // base name, parenthesized suffixes stripped
	Peak        bool   // "(Peak)" row
	BaseName    string // for peak rows: the off-peak model's base name
	In          float64
	Out         float64
	CacheRead   float64
	Unlimited   bool
	GoAllowance float64
}

// DocsPlan is the parsed Go docs page.
type DocsPlan struct {
	Pricing  map[string][]PricingRow // "go" | "go-plus"
	Requests map[string]float64      // base name → requests per month
	Profiles map[string]*Profile     // base name → official token profile
	NameToID map[string]string       // lower-cased display name → model ID
}

var (
	trRe    = regexp.MustCompile(`(?s)<tr[^>]*>(.*?)</tr>`)
	tdRe    = regexp.MustCompile(`(?s)<t[dh][^>]*>(.*?)</t[dh]>`)
	delRe   = regexp.MustCompile(`(?s)<del[^>]*>.*?</del>`)
	tagRe   = regexp.MustCompile(`<[^>]*>`)
	wsRe    = regexp.MustCompile(`\s+`)
	parenRe = regexp.MustCompile(`\s*\([^()]*\)\s*$`)
)

func cellText(s string) string {
	s = delRe.ReplaceAllString(s, "") // strikethrough discount: keep the live price
	s = tagRe.ReplaceAllString(s, "")
	return strings.TrimSpace(html.UnescapeString(s))
}

func parseMoney(s string) (float64, error) {
	s = strings.TrimSpace(s)
	switch {
	case s == "" || s == "-" || strings.EqualFold(s, "free"):
		return 0, nil
	}
	s = strings.NewReplacer("$", "", ",", "", " ", "").Replace(s)
	return strconv.ParseFloat(strings.TrimPrefix(s, "~"), 64)
}

func parseCount(s string) (float64, error) {
	return strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 64)
}

// normName lower-cases and collapses whitespace for cross-table matching.
func normName(s string) string {
	return strings.ToLower(wsRe.ReplaceAllString(strings.TrimSpace(s), " "))
}

// baseName strips trailing parenthesized qualifiers like "(Peak)" or
// "(≤ 256K tokens)".
func baseName(s string) string { return strings.TrimSpace(parenRe.ReplaceAllString(s, "")) }

// panel returns the HTML between two element IDs (endID may be "").
func panel(h, startID, endID string) (string, error) {
	i := strings.Index(h, `id="`+startID+`"`)
	if i < 0 {
		return "", fmt.Errorf("docs page missing section %q", startID)
	}
	if endID == "" {
		return h[i:], nil
	}
	j := strings.Index(h[i:], `id="`+endID+`"`)
	if j < 0 {
		return "", fmt.Errorf("docs page section %q unterminated", startID)
	}
	return h[i : i+j], nil
}

// parseTableRows returns the cell texts of every row in the first table of seg.
func parseTableRows(seg string) ([][]string, error) {
	t := regexp.MustCompile(`(?s)<table.*?</table>`).FindString(seg)
	if t == "" {
		return nil, fmt.Errorf("no table found")
	}
	var out [][]string
	for _, r := range trRe.FindAllStringSubmatch(t, -1) {
		var cells []string
		for _, c := range tdRe.FindAllStringSubmatch(r[1], -1) {
			cells = append(cells, cellText(c[1]))
		}
		out = append(out, cells)
	}
	if len(out) < 2 {
		return nil, fmt.Errorf("table has %d rows", len(out))
	}
	return out, nil
}

// ParseDocsHTML extracts pricing tables, request estimates, token profiles
// and the model-ID map from the Go docs page.
func ParseDocsHTML(h []byte) (*DocsPlan, error) {
	page := string(h)
	docs := &DocsPlan{
		Pricing:  map[string][]PricingRow{},
		Requests: map[string]float64{},
		Profiles: map[string]*Profile{},
		NameToID: map[string]string{},
	}

	// Model ID table: display name → models.dev ID.
	for _, rows := range allTables(page) {
		if len(rows[0]) < 2 || rows[0][1] != "Model ID" {
			continue
		}
		for _, r := range rows[1:] {
			if len(r) >= 2 && r[1] != "" {
				docs.NameToID[normName(r[0])] = r[1]
			}
		}
	}
	if len(docs.NameToID) < 10 {
		return nil, fmt.Errorf("docs page: model ID table not found (%d entries)", len(docs.NameToID))
	}

	for _, plan := range []struct{ id, end, key string }{
		{"go-pricing-panel-go", "go-pricing-panel-go-plus", "go"},
		{"go-pricing-panel-go-plus", "go-pricing-panel-go-teams", "go-plus"},
	} {
		seg, err := panel(page, plan.id, plan.end)
		if err != nil {
			return nil, err
		}
		rows, err := parseTableRows(seg)
		if err != nil {
			return nil, fmt.Errorf("pricing %s: %w", plan.key, err)
		}
		pricing, err := parsePricingRows(rows)
		if err != nil {
			return nil, fmt.Errorf("pricing %s: %w", plan.key, err)
		}
		docs.Pricing[plan.key] = pricing
	}

	seg, err := panel(page, "go-requests-panel-go", "go-requests-panel-go-plus")
	if err != nil {
		return nil, err
	}
	rows, err := parseTableRows(seg)
	if err != nil {
		return nil, fmt.Errorf("requests table: %w", err)
	}
	for _, r := range rows[1:] {
		if len(r) < 4 {
			continue
		}
		v, err := parseCount(r[3]) // requests per month
		if err == nil {
			docs.Requests[normName(baseName(r[0]))] = v
		}
	}

	if err := parseProfileBullets(page, docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func allTables(page string) [][][]string {
	var out [][][]string
	for _, t := range regexp.MustCompile(`(?s)<table.*?</table>`).FindAllString(page, -1) {
		var rows [][]string
		for _, r := range trRe.FindAllStringSubmatch(t, -1) {
			var cells []string
			for _, c := range tdRe.FindAllStringSubmatch(r[1], -1) {
				cells = append(cells, cellText(c[1]))
			}
			rows = append(rows, cells)
		}
		out = append(out, rows)
	}
	return out
}

func parsePricingRows(rows [][]string) ([]PricingRow, error) {
	var out []PricingRow
	seen := map[string]bool{}
	for _, r := range rows[1:] {
		if len(r) < 6 {
			continue
		}
		name := baseName(r[0])
		isPeak := strings.EqualFold(strings.TrimSpace(parenRe.FindString(r[0])), "(peak)")
		if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(r[0])), "(peak)") {
			isPeak = false // parenRe may have matched a version qualifier, not the regime
		}
		key := normName(name)
		if isPeak {
			key += "|peak"
		} else if seen[key] {
			continue // context-tier duplicate: keep the base tier
		}
		seen[key] = true
		pr := PricingRow{Name: name, Peak: isPeak, BaseName: name}
		if isPeak {
			pr.BaseName = name
		}
		var err error
		if pr.In, err = parseMoney(r[1]); err != nil {
			return nil, fmt.Errorf("row %q input: %w", r[0], err)
		}
		if pr.Out, err = parseMoney(r[2]); err != nil {
			return nil, fmt.Errorf("row %q output: %w", r[0], err)
		}
		if pr.CacheRead, err = parseMoney(r[3]); err != nil {
			return nil, fmt.Errorf("row %q cache read: %w", r[0], err)
		}
		if strings.HasPrefix(r[5], "Unlimited") {
			pr.Unlimited = true
		} else if pr.GoAllowance, err = parseMoney(r[5]); err != nil {
			return nil, fmt.Errorf("row %q allowance: %w", r[0], err)
		}
		out = append(out, pr)
	}
	if len(out) < 10 {
		return nil, fmt.Errorf("pricing table has only %d rows", len(out))
	}
	return out, nil
}

var bulletRe = regexp.MustCompile(`^(.*?) — ([\d,]+) input, ([\d,]+) cached, ([\d,]+) output tokens per request$`)

func parseProfileBullets(page string, docs *DocsPlan) error {
	i := strings.Index(page, "The estimates use the following token counts")
	if i < 0 {
		return fmt.Errorf("docs page: token profile list not found")
	}
	ul := regexp.MustCompile(`(?s)<ul.*?</ul>`).FindString(page[i:])
	if ul == "" {
		return fmt.Errorf("docs page: token profile list has no list")
	}
	n := 0
	for _, li := range regexp.MustCompile(`(?s)<li>(.*?)</li>`).FindAllStringSubmatch(ul, -1) {
		m := bulletRe.FindStringSubmatch(cellText(li[1]))
		if m == nil {
			continue
		}
		in, e1 := parseCount(m[2])
		ca, e2 := parseCount(m[3])
		out, e3 := parseCount(m[4])
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		p := &Profile{int64(in), int64(ca), int64(out)}
		parts := strings.Split(m[1], "/") // "GLM-5.3/5.2" shorthand
		first := strings.TrimSpace(parts[0])
		docs.Profiles[normName(first)] = p
		n++
		if len(parts) > 1 {
			// Trailing parts drop the leading token: "5.2" → "GLM-5.2".
			pre, _, _ := strings.Cut(first, "-")
			for _, rest := range parts[1:] {
				docs.Profiles[normName(pre+"-"+strings.TrimSpace(rest))] = p
				n++
			}
		}
	}
	if n < 10 {
		return fmt.Errorf("docs page: parsed only %d token profiles", n)
	}
	return nil
}

// EndpointModel is one model's live pricing from the Go models endpoint.
type EndpointModel struct {
	In        float64 `json:"in"`
	Out       float64 `json:"out"`
	CacheRead float64 `json:"cache_read"`
	Factor    float64 `json:"factor"` // peak multiplier (0 = flat pricing)
}

type endpointDoc struct {
	Data []struct {
		ID   string `json:"id"`
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
				Multipliers []struct {
					Factor float64 `json:"factor"`
				} `json:"multipliers"`
			} `json:"text"`
			Multipliers []struct {
				Factor float64 `json:"factor"`
			} `json:"multipliers"`
		} `json:"cost"`
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
			In:        m.Cost.Text.Input.Price,
			CacheRead: m.Cost.Text.Input.CacheRead.Read,
			Out:       m.Cost.Text.Output.Price,
		}
		for _, mu := range m.Cost.Multipliers {
			if mu.Factor > e.Factor {
				e.Factor = mu.Factor
			}
		}
		out[m.ID] = e
	}
	return out, nil
}

// Merge joins the docs page and endpoint data into a snapshot keyed by
// models.dev model ID.
func Merge(docs *DocsPlan, ep map[string]*EndpointModel) (*LiveSnapshot, error) {
	snap := &LiveSnapshot{FetchedAt: time.Now(), Source: docsURL, Rows: map[string]*LiveRow{}}
	plus := map[string]PricingRow{}
	for _, r := range docs.Pricing["go-plus"] {
		if !r.Peak {
			plus[normName(r.Name)] = r
		}
	}
	peaks := map[string]PricingRow{}
	for _, r := range docs.Pricing["go"] {
		if r.Peak {
			peaks[normName(r.BaseName)] = r
		}
	}
	for _, r := range docs.Pricing["go"] {
		if r.Peak {
			continue
		}
		id, ok := docs.NameToID[normName(r.Name)]
		if !ok {
			return nil, fmt.Errorf("no model ID for docs model %q", r.Name)
		}
		row := &LiveRow{
			ID: id, Name: r.Name, GoAllowance: r.GoAllowance,
			Unlimited: r.Unlimited, In: r.In, Out: r.Out, CacheRead: r.CacheRead,
		}
		if p, ok := plus[normName(r.Name)]; ok {
			row.PlusAllowance = p.GoAllowance
		}
		if p, ok := docs.Profiles[normName(r.Name)]; ok {
			row.Profile = p
		}
		row.ReqsMo = docs.Requests[normName(r.Name)]
		if pk, ok := peaks[normName(r.Name)]; ok {
			row.Peak = &[3]float64{pk.In, pk.Out, pk.CacheRead}
		}
		if e, ok := ep[id]; ok {
			row.Factor = e.Factor
		}
		snap.Rows[id] = row
	}
	return snap, nil
}

// Fetch loads a validated live snapshot: cache first (7-day TTL by default),
// then the network. catalog is the models.dev catalog used for cross-source
// validation. Errors mean "keep using the compiled-in snapshot".
func Fetch(refresh, offline bool, ttl time.Duration, catalog []modelsdev.Model) (*LiveSnapshot, error) {
	payload, err := cache.Load(cacheKey, ttl, refresh, offline, func() ([]byte, error) {
		docsB, err := httpGet(docsURL)
		if err != nil {
			return nil, err
		}
		epB, err := httpGet(endpointURL)
		if err != nil {
			return nil, err
		}
		docs, err := ParseDocsHTML(docsB)
		if err != nil {
			return nil, err
		}
		ep, err := ParseEndpointModels(epB)
		if err != nil {
			return nil, err
		}
		snap, err := Merge(docs, ep)
		if err != nil {
			return nil, err
		}
		if errs := Validate(snap, catalog); len(errs) > 0 {
			return nil, fmt.Errorf("plan snapshot rejected: %s", errJoin(errs))
		}
		snap.FetchedAt = time.Now()
		b, err := json.Marshal(snap)
		if err != nil {
			return nil, err
		}
		return b, nil
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
	return &snap, nil
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
