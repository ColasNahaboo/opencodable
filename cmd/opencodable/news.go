package main

// -news: what changed in the OpenCode Go plan since the previous -news run.
//
// The state (one snapshot per model + the previous run's output) lives in the
// cache directory, so deleting the cache resets the baseline: the next run is
// a first run and prints nothing. News is deliberately limited to the plan's
// public data (catalog, budgets, token profiles, peak prices, privacy) plus
// the AA Intelligence Index as a coarse quality tier; it loads no probes, no
// console spend and no benchmark feeds.
//
// Errors are printed as "ERROR <scope>: <message>" lines so a cron wrapper can
// tell "the site/API changed shape" from an ordinary change. When two runs in
// a row produce only ERROR lines, the mail is skipped: a persistent breakage
// must not page the operator forever.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/mail"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ColasNahaboo/opencodable/internal/aa"
	"github.com/ColasNahaboo/opencodable/internal/cache"
	"github.com/ColasNahaboo/opencodable/internal/cost"
	"github.com/ColasNahaboo/opencodable/internal/goplan"
	"github.com/ColasNahaboo/opencodable/internal/modelsdev"
	"github.com/ColasNahaboo/opencodable/internal/table"
)

const (
	newsStateKey     = "news-state.json"
	newsStateVersion = 1
)

// newsModel is one model's stable, plan-relevant state. CostTask is the
// official per-request cost (base tier); PeakTask is the peak-regime cost when
// the model has peak pricing. Prices are USD per 1M tokens.
type newsModel struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	CostTask      float64 `json:"cost_task"`
	PeakTask      float64 `json:"peak_cost_task,omitempty"`
	HasPeak       bool    `json:"has_peak,omitempty"`
	PriceIn       float64 `json:"price_in"`
	PriceOut      float64 `json:"price_out"`
	CacheRead     float64 `json:"cache_read"`
	CacheWrite    float64 `json:"cache_write"`
	GoAllow       float64 `json:"allowance_go"`
	PlusAllow     float64 `json:"allowance_go_plus"`
	GoUnlimited   bool    `json:"go_unlimited,omitempty"`
	PlusUnlimited bool    `json:"go_plus_unlimited,omitempty"`
	Known         bool    `json:"plan_known"`
	Ctx           int64   `json:"context"`
	Out           int64   `json:"output"`
	Privacy       string  `json:"privacy"`
	Quality       float64 `json:"quality,omitempty"`
}

// newsState is the persisted baseline: one previous snapshot plus the last
// run's output (used to suppress repeated ERROR mails).
type newsState struct {
	Version    int                  `json:"version"`
	UpdatedAt  string               `json:"updated_at"`
	PlanSource string               `json:"plan_source,omitempty"` // live | builtin
	Baseline   bool                 `json:"baseline"`              // a successful snapshot was taken
	Models     map[string]newsModel `json:"models,omitempty"`
	Output     []string             `json:"output,omitempty"`
}

// loadNewsState reads the baseline; a missing or incompatible file means
// "first run", never an error.
func loadNewsState() (newsState, bool) {
	b, ok := cache.Peek(newsStateKey)
	if !ok {
		return newsState{}, false
	}
	var st newsState
	if err := json.Unmarshal(b, &st); err != nil || st.Version != newsStateVersion {
		return newsState{}, false
	}
	return st, true
}

// saveNewsState persists the baseline. A write failure is reported by the
// caller as a warning, never fatal: the next run just reports the same news.
func saveNewsState(st newsState) error {
	st.Version = newsStateVersion
	st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return cache.Store(newsStateKey, b)
}

// buildNewsSnapshot captures every catalog model's plan-relevant state.
// aas may be nil (no Artificial Analysis data): tiers then stay unknown.
func buildNewsSnapshot(models []modelsdev.Model, aas []aa.Model) map[string]newsModel {
	byID := make(map[string]modelsdev.Model, len(models))
	for _, m := range models {
		byID[m.ID] = m
	}
	snapshot := map[string]newsModel{}
	for _, r := range table.Build(models, aas, nil, nil, nil, nil, nil, "go", false) {
		m := byID[r.ID]
		goAllow, goUnlimited := goplan.Allowance(r.ID, "go")
		plusAllow, plusUnlimited := goplan.Allowance(r.ID, "go-plus")
		known := goplan.Known(r.ID)
		if !known && m.Free() {
			// Like the table: zero prices mean no per-model cap even when
			// the plan snapshot has no entry for the free variant.
			goUnlimited, plusUnlimited = true, true
		}
		_, hasPeak := goplan.PeakPrice(r.ID)
		nm := newsModel{
			ID: r.ID, Name: r.Name,
			CostTask: r.CostTask,
			HasPeak:  hasPeak,
			PriceIn:  m.Cost.Input, PriceOut: m.Cost.Output,
			CacheRead: m.Cost.CacheRead, CacheWrite: m.Cost.CacheWrite,
			GoAllow: goAllow, PlusAllow: plusAllow,
			GoUnlimited: goUnlimited, PlusUnlimited: plusUnlimited,
			Known:   known,
			Ctx:     m.Limit.Context,
			Out:     m.Limit.Output,
			Privacy: goplan.PrivacyLabel(r.ID),
			Quality: r.AAIntel,
		}
		if hasPeak {
			nm.PeakTask = cost.PerRequest(m, true)
		}
		snapshot[r.ID] = nm
	}
	return snapshot
}

// newsChanged reports whether two numbers differ materially, ignoring float
// round-trip noise: 0.1% relative or 1e-9 absolute.
func newsChanged(a, b float64) bool {
	tol := math.Max(1e-9, 0.001*math.Max(math.Abs(a), math.Abs(b)))
	return math.Abs(a-b) > tol
}

// newsTier buckets the AA Intelligence Index into a coarse quality tier.
// Thresholds are documented in README.md and TITLES.md; unknown means AA has
// no score for the model (not "bad").
func newsTier(q float64) string {
	switch {
	case q <= 0:
		return "unknown"
	case q >= 45:
		return "frontier"
	case q >= 35:
		return "high"
	case q >= 25:
		return "mid"
	default:
		return "low"
	}
}

func newsTierRank(t string) int {
	switch t {
	case "frontier":
		return 4
	case "high":
		return 3
	case "mid":
		return 2
	case "low":
		return 1
	}
	return 0
}

// newsMoney formats a USD figure with 5 decimals, trailing zeros trimmed.
func newsMoney(v float64) string {
	if v == 0 {
		return "$0"
	}
	s := strconv.FormatFloat(v, 'f', 5, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	return "$" + s
}

// newsK formats token limits in thousands.
func newsK(v int64) string {
	return strconv.FormatFloat(float64(v)/1000, 'f', -1, 64)
}

// newsPct is the relative change, e.g. "+12.3%".
func newsPct(old, new float64) string {
	if old == 0 {
		return "new"
	}
	return fmt.Sprintf("%+.1f%%", (new-old)/old*100)
}

// newsQuality renders the AA score, or "-" when unknown.
func newsQuality(q float64) string {
	if q <= 0 {
		return "-"
	}
	return strconv.FormatFloat(q, 'f', 1, 64)
}

// newsPlanState labels a model's plan membership for added/removed lines.
func newsPlanState(m newsModel) string {
	switch {
	case m.GoUnlimited:
		return "free"
	case !m.Known:
		return "unknown"
	default:
		return "paid"
	}
}

// newsTag is the common tail of most lines: tier, quality and display name
// (quoted, so the name may contain spaces and parsers can split on fields).
func newsTag(m newsModel) string {
	return fmt.Sprintf("tier=%s quality=%s %s", newsTier(m.Quality), newsQuality(m.Quality), strconv.Quote(m.Name))
}

// newsAllow renders a plan's budget, "unlimited" when the model is free.
func newsAllow(allow float64, unlimited bool) string {
	if unlimited {
		return "unlimited"
	}
	return newsMoney(allow)
}

// newsAddLine renders MODEL_ADD / MODEL_DEL.
func newsAddLine(keyword string, m newsModel) string {
	return fmt.Sprintf("%s %s cost_task=%s plan=%s %s",
		keyword, m.ID, newsMoney(m.CostTask), newsPlanState(m), newsTag(m))
}

// allowanceLines diffs one plan's monthly allowance. Known must hold on both
// sides; callers skip this when the plan membership itself changed.
func allowanceLines(id, plan string, oldAllow float64, oldUnlimited bool, newAllow float64, newUnlimited bool, tag string) []string {
	switch {
	case oldUnlimited && newUnlimited:
		return nil
	case !oldUnlimited && newUnlimited:
		return []string{fmt.Sprintf("UNLIMITED_ON %s plan=%s allowance=%s->unlimited %s",
			id, plan, newsMoney(oldAllow), tag)}
	case oldUnlimited && !newUnlimited:
		return []string{fmt.Sprintf("UNLIMITED_OFF %s plan=%s allowance=unlimited->%s %s",
			id, plan, newsMoney(newAllow), tag)}
	case !newsChanged(oldAllow, newAllow):
		return nil
	}
	keyword := "ALLOWANCE_UP"
	if newAllow < oldAllow {
		keyword = "ALLOWANCE_DOWN"
	}
	return []string{fmt.Sprintf("%s %s plan=%s allowance=%s->%s (%s) %s",
		keyword, id, plan, newsMoney(oldAllow), newsMoney(newAllow),
		newsPct(oldAllow, newAllow), tag)}
}

// newsPriceChanged reports whether any base token price moved.
func newsPriceChanged(p, c newsModel) bool {
	return newsChanged(p.PriceIn, c.PriceIn) || newsChanged(p.PriceOut, c.PriceOut) ||
		newsChanged(p.CacheRead, c.CacheRead) || newsChanged(p.CacheWrite, c.CacheWrite)
}

// newsPriceFields lists the raw prices that moved, so a PRICE_* line never
// prints a misleading unchanged headline.
func newsPriceFields(p, c newsModel) string {
	var parts []string
	add := func(name string, old, new float64) {
		if newsChanged(old, new) {
			parts = append(parts, fmt.Sprintf("%s=%s->%s", name, newsMoney(old), newsMoney(new)))
		}
	}
	add("cost_tok", p.PriceOut, c.PriceOut)
	add("price_in", p.PriceIn, c.PriceIn)
	add("cache_read", p.CacheRead, c.CacheRead)
	add("cache_write", p.CacheWrite, c.CacheWrite)
	return strings.Join(parts, " ")
}

// newsPriceDir picks the direction of a raw price change: output price first
// (the retail headline), then input, cache read and cache write.
func newsPriceDir(p, c newsModel) int {
	pairs := [][2]float64{
		{p.PriceOut, c.PriceOut},
		{p.PriceIn, c.PriceIn},
		{p.CacheRead, c.CacheRead},
		{p.CacheWrite, c.CacheWrite},
	}
	for _, pair := range pairs {
		if newsChanged(pair[0], pair[1]) && pair[0] != pair[1] {
			if pair[1] > pair[0] {
				return 1
			}
			return -1
		}
	}
	return -1
}

// newsChangeLines diffs one model that exists on both sides.
func newsChangeLines(p, c newsModel) []string {
	var out []string
	tag := newsTag(c)

	if p.Name != c.Name {
		out = append(out, fmt.Sprintf("NAME_CHANGE %s was=%s %s",
			c.ID, strconv.Quote(p.Name), tag))
	}

	if p.Known != c.Known {
		if c.Known {
			out = append(out, fmt.Sprintf("PLAN_ADD %s allowance=%s/%s %s",
				c.ID, newsAllow(c.GoAllow, c.GoUnlimited), newsAllow(c.PlusAllow, c.PlusUnlimited), tag))
		} else {
			out = append(out, fmt.Sprintf("PLAN_DEL %s %s", c.ID, tag))
		}
	} else if c.Known {
		// Plan membership unchanged: compare both plans' budgets.
		out = append(out, allowanceLines(c.ID, "go", p.GoAllow, p.GoUnlimited, c.GoAllow, c.GoUnlimited, tag)...)
		out = append(out, allowanceLines(c.ID, "go-plus", p.PlusAllow, p.PlusUnlimited, c.PlusAllow, c.PlusUnlimited, tag)...)
	}

	if newsChanged(p.CostTask, c.CostTask) {
		keyword := "COST_UP"
		if c.CostTask < p.CostTask {
			keyword = "COST_DOWN"
		}
		price := ""
		if newsChanged(p.PriceOut, c.PriceOut) {
			price = fmt.Sprintf(" price_out=%s->%s (%s)",
				newsMoney(p.PriceOut), newsMoney(c.PriceOut), newsPct(p.PriceOut, c.PriceOut))
		}
		out = append(out, fmt.Sprintf("%s %s cost_task=%s->%s (%s)%s %s",
			keyword, c.ID, newsMoney(p.CostTask), newsMoney(c.CostTask),
			newsPct(p.CostTask, c.CostTask), price, tag))
	} else if newsPriceChanged(p, c) {
		keyword := "PRICE_UP"
		if newsPriceDir(p, c) < 0 {
			keyword = "PRICE_DOWN"
		}
		out = append(out, fmt.Sprintf("%s %s %s %s",
			keyword, c.ID, newsPriceFields(p, c), tag))
	}

	switch {
	case !p.HasPeak && c.HasPeak:
		out = append(out, fmt.Sprintf("PEAK_ON %s peak_cost_task=%s %s",
			c.ID, newsMoney(c.PeakTask), tag))
	case p.HasPeak && !c.HasPeak:
		out = append(out, fmt.Sprintf("PEAK_OFF %s %s", c.ID, tag))
	case p.HasPeak && c.HasPeak && newsChanged(p.PeakTask, c.PeakTask):
		keyword := "PEAK_COST_UP"
		if c.PeakTask < p.PeakTask {
			keyword = "PEAK_COST_DOWN"
		}
		out = append(out, fmt.Sprintf("%s %s peak_cost_task=%s->%s (%s) %s",
			keyword, c.ID, newsMoney(p.PeakTask), newsMoney(c.PeakTask),
			newsPct(p.PeakTask, c.PeakTask), tag))
	}

	if p.Ctx != c.Ctx || p.Out != c.Out {
		fields := ""
		if p.Ctx != c.Ctx {
			fields += fmt.Sprintf(" ctx_k=%s->%s", newsK(p.Ctx), newsK(c.Ctx))
		}
		if p.Out != c.Out {
			fields += fmt.Sprintf(" out_k=%s->%s", newsK(p.Out), newsK(c.Out))
		}
		keyword := "CONTEXT_UP"
		if p.Ctx != c.Ctx {
			if c.Ctx < p.Ctx {
				keyword = "CONTEXT_DOWN"
			}
		} else if c.Out < p.Out {
			keyword = "CONTEXT_DOWN" // output limit is the only change
		}
		out = append(out, fmt.Sprintf("%s %s%s %s", keyword, c.ID, fields, tag))
	}

	if p.Privacy != c.Privacy {
		out = append(out, fmt.Sprintf("PRIVACY_CHANGE %s privacy=%s->%s %s",
			c.ID, p.Privacy, c.Privacy, tag))
	}

	pt, ct := newsTier(p.Quality), newsTier(c.Quality)
	if p.Quality > 0 && c.Quality > 0 && pt != ct {
		keyword := "TIER_UP"
		if newsTierRank(ct) < newsTierRank(pt) {
			keyword = "TIER_DOWN"
		}
		out = append(out, fmt.Sprintf("%s %s tier=%s->%s quality=%.1f->%.1f %s",
			keyword, c.ID, pt, ct, p.Quality, c.Quality, strconv.Quote(c.Name)))
	}

	return out
}

// diffNews returns one line per change, ordered by model ID then by the
// field order inside newsChangeLines.
func diffNews(prev, cur map[string]newsModel) []string {
	seen := make(map[string]bool, len(prev)+len(cur))
	ids := make([]string, 0, len(prev)+len(cur))
	for id := range prev {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for id := range cur {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	var out []string
	for _, id := range ids {
		p, was := prev[id]
		c, is := cur[id]
		switch {
		case !was:
			out = append(out, newsAddLine("MODEL_ADD", c))
		case !is:
			out = append(out, newsAddLine("MODEL_DEL", p))
		default:
			out = append(out, newsChangeLines(p, c)...)
		}
	}
	return out
}

// isErrorOutput reports whether every line is an ERROR line. An empty output
// is not an error.
func isErrorOutput(lines []string) bool {
	if len(lines) == 0 {
		return false
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, "ERROR ") {
			return false
		}
	}
	return true
}

// newsError flattens a fetch/parse failure into one output line.
func newsError(scope string, err error) string {
	msg := strings.ReplaceAll(err.Error(), "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	return fmt.Sprintf("ERROR %s: %s", scope, msg)
}

// runNews computes and prints the changes since the previous successful run,
// then mails them when -email is set. It writes the change lines to w so a
// test (or a future caller) can capture them; main passes os.Stdout.
func runNews(w io.Writer, email string, refresh, offline, aaPro bool, planTTL time.Duration) error {
	prev, found := loadNewsState()
	var lines []string
	var cur map[string]newsModel
	snapshotOK := false
	planSource := "builtin"

	models, err := modelsdev.Fetch("opencode-go", refresh, offline)
	switch {
	case err != nil:
		lines = append(lines, newsError("models.dev", err))
	case len(models) == 0:
		lines = append(lines, "ERROR models.dev: provider opencode-go returned no models")
	default:
		if !offline {
			// cache.Load falls back to the old copy when a fetch fails, so a
			// cache older than its TTL means this run's refresh silently
			// failed. Report it: a site/API change is the usual cause.
			if age, ok := cache.Age("modelsdev-opencode-go.json"); ok && age > catalogTTL {
				lines = append(lines, fmt.Sprintf("ERROR models.dev: refresh failed; using a %s-old cached catalog", compactAge(age)))
			}
		}
		snap, perr := goplan.Fetch(refresh, offline, planTTL, models)
		switch {
		case perr != nil && !offline:
			// Without a trustworthy plan snapshot a diff would compare
			// builtin budgets with live ones and invent changes.
			lines = append(lines, newsError("plan", perr))
		default:
			if perr == nil {
				goplan.Use(snap)
			}
			if !offline {
				if age, ok := cache.Age("goplan-live.json"); ok && age > planTTL {
					lines = append(lines, fmt.Sprintf("ERROR plan: refresh failed; using a %s-old cached snapshot", compactAge(age)))
				}
			}
			if goplan.IsLive() {
				planSource = "live"
			}
			var aas []aa.Model
			if a, aerr := aa.Fetch(refresh, offline, aaPro); aerr == nil {
				aas = a
			} else if isTTY(os.Stderr) {
				// Quality tiers are a bonus, never a reason to fail news.
				// Kept off stderr in cron runs: cron mails any output.
				fmt.Fprintln(os.Stderr, "# warning: AA data unavailable, quality tiers unknown:", aerr)
			}
			cur = buildNewsSnapshot(models, aas)
			// An AA outage carries the previous scores forward: a missing
			// score is not a quality change.
			for id, m := range cur {
				if m.Quality == 0 && prev.Models != nil {
					if old, ok := prev.Models[id]; ok && old.Quality > 0 {
						m.Quality = old.Quality
						cur[id] = m
					}
				}
			}
			snapshotOK = true
		}
	}

	switch {
	case !snapshotOK:
		// lines already carries the ERROR.
	case !found || !prev.Baseline:
		// First successful run: record the baseline silently.
	case prev.PlanSource == "" || prev.PlanSource == planSource:
		lines = append(lines, diffNews(prev.Models, cur)...)
	case prev.PlanSource == "builtin" && planSource == "live":
		// Upgraded from the built-in table to the live snapshot: the budget
		// and profile differences are source artifacts, not plan news.
	default:
		// live -> builtin (offline with no cached plan snapshot): keep the
		// baseline so the next online run still reports pending changes.
		if isTTY(os.Stderr) {
			fmt.Fprintln(os.Stderr, "# news: no live plan snapshot (offline?); baseline kept, no diff")
		}
		return nil
	}

	for _, line := range lines {
		fmt.Fprintln(w, line)
	}

	// Mail: only a non-empty change set, and not the same ERROR twice in a row.
	var to []string
	if strings.TrimSpace(email) != "" {
		parsed, perr := parseEmails(email)
		if perr != nil {
			return perr
		}
		to = parsed
	}
	if len(to) > 0 && len(lines) > 0 {
		if isErrorOutput(lines) && isErrorOutput(prev.Output) {
			if isTTY(os.Stderr) {
				fmt.Fprintln(os.Stderr, "# mail skipped: the previous run ended with the same kind of ERROR")
			}
		} else if err := newsMailer(to, newsSubject(lines), strings.Join(lines, "\n")+"\n"); err != nil {
			// Do not advance the baseline: the next run must retry the mail.
			return fmt.Errorf("mail: %w", err)
		}
	}

	if snapshotOK {
		prev.Models = cur
		prev.PlanSource = planSource
		prev.Baseline = true
	}
	prev.Output = lines
	if err := saveNewsState(prev); err != nil {
		fmt.Fprintln(os.Stderr, "# warning: cannot save news state:", err)
	}
	return nil
}

// newsSubject summarizes a change set for the mail subject.
func newsSubject(lines []string) string {
	if isErrorOutput(lines) {
		return "opencodable news: check failed"
	}
	return fmt.Sprintf("opencodable news: %d change(s)", len(lines))
}

// parseEmails splits and validates a comma-separated address list. Header
// injection is impossible by construction: only parsed addresses are used.
func parseEmails(s string) ([]string, error) {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		a, err := mail.ParseAddress(part)
		if err != nil {
			return nil, fmt.Errorf("invalid -email %q: %w", part, err)
		}
		out = append(out, a.Address)
	}
	return out, nil
}

// newsMailer sends a change set; a test seam, like probeRun.
var newsMailer = sendNewsMail

// sendNewsMail hands the message to the local mail transport: sendmail when
// available (recipients in the headers), else mail(1) (recipients as args).
// Credentials and SMTP hosts are the system's business; opencodable only
// composes the message.
func sendNewsMail(to []string, subject, body string) error {
	if path, err := exec.LookPath("sendmail"); err == nil {
		cmd := exec.Command(path, "-t", "-oi")
		cmd.Stdin = bytes.NewReader(newsMessage(to, subject, body))
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("sendmail: %v: %s", err, strings.TrimSpace(stderr.String()))
		}
		return nil
	}
	if path, err := exec.LookPath("mail"); err == nil {
		args := append([]string{"-s", subject}, to...)
		cmd := exec.Command(path, args...)
		cmd.Stdin = strings.NewReader(body)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("mail: %v: %s", err, strings.TrimSpace(stderr.String()))
		}
		return nil
	}
	return errors.New("no sendmail or mail command in PATH")
}

// newsMessage builds an RFC 822 message for sendmail -t.
func newsMessage(to []string, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Auto-Submitted: auto-generated\r\n")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(strings.TrimRight(body, "\r\n"), "\n", "\r\n"))
	b.WriteString("\r\n")
	return []byte(b.String())
}
