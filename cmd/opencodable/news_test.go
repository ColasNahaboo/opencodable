package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ColasNahaboo/opencodable/internal/cache"
	"github.com/ColasNahaboo/opencodable/internal/goplan"
	"github.com/ColasNahaboo/opencodable/internal/modelsdev"
)

func newsTestModel(id, name string, cost float64) newsModel {
	return newsModel{
		ID: id, Name: name, CostTask: cost,
		PriceIn: 2, PriceOut: 10, CacheRead: 0.2,
		GoAllow: 60, PlusAllow: 180, Known: true,
		Ctx: 200000, Out: 32000, Privacy: "safe",
	}
}

func TestBuildNewsSnapshotFreeInference(t *testing.T) {
	goplan.Use(nil)
	t.Cleanup(func() { goplan.Use(nil) })
	free := testModel("cat-jam-free", "Cat Jam Free", 0, 0, 0)
	paid := testModel("space-bunny", "Space Bunny", 1, 2, 0.1)
	snap := buildNewsSnapshot([]modelsdev.Model{free, paid}, nil, nil)

	f := snap["cat-jam-free"]
	if !f.GoUnlimited || !f.PlusUnlimited || f.Known {
		t.Errorf("free model = %+v, want unlimited on both plans, not plan-known", f)
	}
	if got := newsPlanState(f); got != "free" {
		t.Errorf("free plan state = %q, want free", got)
	}
	u := snap["space-bunny"]
	if u.GoUnlimited || u.PlusUnlimited {
		t.Errorf("unknown paid model must not be unlimited: %+v", u)
	}
	if got := newsPlanState(u); got != "unknown" {
		t.Errorf("unknown plan state = %q, want unknown", got)
	}
}

func TestNewsUnavailTag(t *testing.T) {
	m := newsTestModel("crouton-alpha-free", "Crouton Alpha Free", 0)
	if i := strings.Index(newsTag(m), "unavail"); i != -1 {
		t.Errorf("unmarked tag carries unavail: %q", newsTag(m))
	}
	m.Unavail = true
	tag := newsTag(m)
	if !strings.HasSuffix(tag, `quality=- unavail=yes "Crouton Alpha Free"`) {
		t.Errorf("tag = %q, want unavail=yes before the quoted name", tag)
	}
}

func TestDiffNewsUnavailTransitions(t *testing.T) {
	prev := map[string]newsModel{"m": newsTestModel("m", "M", 0.01)}
	cur := map[string]newsModel{"m": newsTestModel("m", "M", 0.01)}
	if lines := diffNews(prev, cur); len(lines) != 0 {
		t.Errorf("unchanged state = %d lines, want 0: %v", len(lines), lines)
	}
	on := newsTestModel("m", "M", 0.01)
	on.Unavail = true
	cur["m"] = on
	lines := diffNews(prev, cur)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "UNAVAIL_ON m ") {
		t.Errorf("marking = %v, want one UNAVAIL_ON line", lines)
	}
	lines = diffNews(cur, prev)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "UNAVAIL_OFF m ") {
		t.Errorf("unmarking = %v, want one UNAVAIL_OFF line", lines)
	}
}

func TestBuildNewsSnapshotUnavail(t *testing.T) {
	goplan.Use(nil)
	t.Cleanup(func() { goplan.Use(nil) })
	m := testModel("crouton-alpha-free", "Crouton Alpha Free", 0, 0, 0)
	snap := buildNewsSnapshot([]modelsdev.Model{m}, nil, []string{"crouton-*"})
	if !snap[m.ID].Unavail {
		t.Errorf("glob-marked model = %+v, want Unavail", snap[m.ID])
	}
	snap = buildNewsSnapshot([]modelsdev.Model{m}, nil, nil)
	if snap[m.ID].Unavail {
		t.Errorf("unmarked model = %+v, want not Unavail", snap[m.ID])
	}
}

func TestDiffNewsAddRemove(t *testing.T) {
	prev := map[string]newsModel{"old": newsTestModel("old", "Old Model", 0.004)}
	cur := map[string]newsModel{"new": newsTestModel("new", "New Model", 0.002)}
	lines := diffNews(prev, cur)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want add + del: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "MODEL_ADD new cost_task=$0.002 ") {
		t.Errorf("add line = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "MODEL_DEL old cost_task=$0.004 ") {
		t.Errorf("del line = %q", lines[1])
	}
	if !strings.Contains(lines[0], `"New Model"`) {
		t.Errorf("add line must end with the quoted display name: %q", lines[0])
	}
}

func TestDiffNewsNoChange(t *testing.T) {
	m := newsTestModel("m", "M", 0.01)
	if lines := diffNews(map[string]newsModel{"m": m}, map[string]newsModel{"m": m}); len(lines) != 0 {
		t.Errorf("identical snapshots produced %v", lines)
	}
	// Float round-trip noise must not count as a change.
	m2 := m
	m2.CostTask = m.CostTask * (1 + 1e-7)
	if lines := diffNews(map[string]newsModel{"m": m}, map[string]newsModel{"m": m2}); len(lines) != 0 {
		t.Errorf("noise produced %v", lines)
	}
}

func TestDiffNewsCost(t *testing.T) {
	prev := map[string]newsModel{"m": newsTestModel("m", "M", 0.02)}
	cur := map[string]newsModel{"m": newsTestModel("m", "M", 0.03)}
	cur["m"] = func() newsModel {
		m := cur["m"]
		m.PriceOut = 15
		return m
	}()
	lines := diffNews(prev, cur)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want one COST_UP: %v", len(lines), lines)
	}
	want := "COST_UP m cost_task=$0.02->$0.03 (+50.0%) price_out=$10->$15 (+50.0%) tier=unknown"
	if !strings.HasPrefix(lines[0], want) {
		t.Errorf("line = %q, want prefix %q", lines[0], want)
	}

	cur["m"] = func() newsModel {
		m := cur["m"]
		m.CostTask = 0.01
		return m
	}()
	if lines := diffNews(prev, cur); len(lines) != 1 || !strings.HasPrefix(lines[0], "COST_DOWN m ") {
		t.Errorf("cost down = %v", lines)
	}
}

func TestDiffNewsAllowance(t *testing.T) {
	prev := map[string]newsModel{"m": newsTestModel("m", "M", 0.01)}
	cur := map[string]newsModel{"m": newsTestModel("m", "M", 0.01)}
	m := cur["m"]
	m.GoAllow = 80
	m.PlusAllow = 240
	cur["m"] = m
	lines := diffNews(prev, cur)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want one per plan: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "ALLOWANCE_UP m plan=go allowance=$60->$80 (+33.3%)") {
		t.Errorf("go line = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "ALLOWANCE_UP m plan=go-plus allowance=$180->$240 (+33.3%)") {
		t.Errorf("go-plus line = %q", lines[1])
	}
}

func TestDiffNewsUnlimitedAndPlanMembership(t *testing.T) {
	prev := map[string]newsModel{"m": newsTestModel("m", "M", 0.01)}
	cur := map[string]newsModel{"m": newsTestModel("m", "M", 0.01)}
	m := cur["m"]
	m.GoAllow, m.GoUnlimited = 0, true
	m.PlusAllow, m.PlusUnlimited = 0, true
	cur["m"] = m
	lines := diffNews(prev, cur)
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "UNLIMITED_ON m plan=go ") ||
		!strings.HasPrefix(lines[1], "UNLIMITED_ON m plan=go-plus ") {
		t.Errorf("unlimited lines = %v", lines)
	}

	// A model that gained a plan entry reports PLAN_ADD, not budget deltas.
	prev = map[string]newsModel{"m": newsTestModel("m", "M", 0.01)}
	prevM := prev["m"]
	prevM.Known = false
	prevM.GoAllow, prevM.PlusAllow = 0, 0
	prev["m"] = prevM
	cur = map[string]newsModel{"m": newsTestModel("m", "M", 0.01)}
	lines = diffNews(prev, cur)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "PLAN_ADD m allowance=$60/$180 ") {
		t.Errorf("plan add = %v", lines)
	}
}

func TestDiffNewsOutputLimitOnly(t *testing.T) {
	prev := map[string]newsModel{"m": newsTestModel("m", "M", 0.01)}
	cur := map[string]newsModel{"m": newsTestModel("m", "M", 0.01)}
	c := cur["m"]
	c.Out = 16000
	cur["m"] = c
	lines := diffNews(prev, cur)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "CONTEXT_DOWN m out_k=32->16 ") {
		t.Errorf("output-only change = %v", lines)
	}
}

func TestDiffNewsPriceOnly(t *testing.T) {
	// cache_write is not part of the token profile, so a cache-write price
	// move leaves cost_task untouched and surfaces as PRICE_*.
	prev := map[string]newsModel{"m": newsTestModel("m", "M", 0.01)}
	cur := map[string]newsModel{"m": newsTestModel("m", "M", 0.01)}
	c := cur["m"]
	c.CacheWrite = 2
	cur["m"] = c
	lines := diffNews(prev, cur)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "PRICE_UP m cache_write=$0->$2 ") {
		t.Errorf("price-only change = %v", lines)
	}
}

func TestDiffNewsContextPrivacyNameTier(t *testing.T) {
	prev := map[string]newsModel{"m": newsTestModel("m", "Old Name", 0.01)}
	p := prev["m"]
	p.Quality = 31.7
	p.Privacy = "safe"
	prev["m"] = p

	cur := map[string]newsModel{"m": newsTestModel("m", "New Name", 0.01)}
	c := cur["m"]
	c.Quality = 46
	c.Ctx = 256000
	c.Out = 64000
	c.Privacy = "trains"
	cur["m"] = c

	lines := diffNews(prev, cur)
	var kinds []string
	for _, l := range lines {
		kinds = append(kinds, strings.SplitN(l, " ", 2)[0])
	}
	want := []string{"NAME_CHANGE", "CONTEXT_UP", "PRIVACY_CHANGE", "TIER_UP"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("keywords = %v, want %v (lines %v)", kinds, want, lines)
	}
	if !strings.Contains(lines[1], "ctx_k=200->256 out_k=32->64") {
		t.Errorf("context line = %q", lines[1])
	}
	if !strings.Contains(lines[3], "tier=mid->frontier quality=31.7->46.0") {
		t.Errorf("tier line = %q", lines[3])
	}
}

func TestIsErrorOutput(t *testing.T) {
	cases := []struct {
		lines []string
		want  bool
	}{
		{nil, false},
		{[]string{"ERROR plan: x"}, true},
		{[]string{"ERROR plan: x", "ERROR models.dev: y"}, true},
		{[]string{"MODEL_ADD m x", "ERROR plan: x"}, false},
		{[]string{"COST_UP m x"}, false},
	}
	for _, c := range cases {
		if got := isErrorOutput(c.lines); got != c.want {
			t.Errorf("isErrorOutput(%v) = %v, want %v", c.lines, got, c.want)
		}
	}
}

func TestParseEmails(t *testing.T) {
	got, err := parseEmails("a@example.com, Bob <b@example.com>,")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "a@example.com" || got[1] != "b@example.com" {
		t.Errorf("addresses = %v", got)
	}
	if _, err := parseEmails("not-an-address"); err == nil {
		t.Error("invalid address must fail")
	}
}

func TestSendNewsMailSendmail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake sendmail script needs a POSIX shell")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "msg")
	script := "#!/bin/sh\n: > \"$NEWS_TEST_OUT\"\nwhile IFS= read -r line; do printf '%s\\n' \"$line\" >> \"$NEWS_TEST_OUT\"; done\n"
	if err := os.WriteFile(filepath.Join(dir, "sendmail"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("NEWS_TEST_OUT", out)

	if err := sendNewsMail([]string{"a@example.com", "b@example.com"}, "subject", "line1\nline2\n"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	msg := string(b)
	for _, want := range []string{
		"To: a@example.com, b@example.com\r\n",
		"Subject: subject\r\n",
		"line1\r\nline2\r\n",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestSendNewsMailFallsBackToMail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake mail script needs a POSIX shell")
	}
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bodyFile := filepath.Join(dir, "body")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$NEWS_TEST_ARGS\"\n: > \"$NEWS_TEST_BODY\"\nwhile IFS= read -r line; do printf '%s\\n' \"$line\" >> \"$NEWS_TEST_BODY\"; done\n"
	if err := os.WriteFile(filepath.Join(dir, "mail"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("NEWS_TEST_ARGS", argsFile)
	t.Setenv("NEWS_TEST_BODY", bodyFile)

	if err := sendNewsMail([]string{"a@example.com"}, "subj", "hello\n"); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(args)); got != "-s\nsubj\na@example.com" {
		t.Errorf("mail args = %q", got)
	}
	body, err := os.ReadFile(bodyFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello\n" {
		t.Errorf("mail body = %q", body)
	}
}

func TestSendNewsMailNoTransport(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := sendNewsMail([]string{"a@example.com"}, "s", "b"); err == nil {
		t.Error("no transport must fail")
	}
}

// newsCatalog stores a catalog cache entry so -news can run with -offline.
func newsCatalog(t *testing.T, outputPrice float64) {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"opencode-go": map[string]any{
			"models": map[string]any{
				"kimi-k3.1-pro": map[string]any{
					"name":  "Kimi K3.1 Pro",
					"cost":  map[string]any{"input": 2.0, "output": outputPrice, "cache_read": 0.2},
					"limit": map[string]any{"context": 200000, "output": 32000},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Store("modelsdev-opencode-go.json", b); err != nil {
		t.Fatal(err)
	}
}

// newsAA stores an empty AA cache so the offline test never warns about it.
func newsAA(t *testing.T) {
	t.Helper()
	if err := cache.Store("aa-models-v2-free.json", []byte(`{"data":[]}`)); err != nil {
		t.Fatal(err)
	}
}

func TestRunNewsBaselineThenChanges(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	goplan.Use(nil)
	t.Cleanup(func() { goplan.Use(nil) })
	newsCatalog(t, 10)
	newsAA(t)

	oldMailer := newsMailer
	var mails []string
	newsMailer = func(to []string, subject, body string) error {
		mails = append(mails, subject+"\n"+body)
		return nil
	}
	t.Cleanup(func() { newsMailer = oldMailer })

	run := func() string {
		var buf bytes.Buffer
		if err := runNews(&buf, "", false, true, false, time.Hour, nil); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}

	// First run: baseline only, nothing to report.
	if out := run(); out != "" {
		t.Fatalf("first run printed %q", out)
	}
	// Same catalog: still nothing.
	if out := run(); out != "" {
		t.Fatalf("unchanged run printed %q", out)
	}
	// Price up: one COST_UP line.
	newsCatalog(t, 12)
	out := run()
	if !strings.HasPrefix(out, "COST_UP kimi-k3.1-pro cost_task=") {
		t.Fatalf("change run = %q", out)
	}
	if len(mails) != 0 {
		t.Errorf("mail sent without -email: %d", len(mails))
	}
	// State advanced: the same catalog prints nothing.
	if out := run(); out != "" {
		t.Fatalf("repeat run printed %q", out)
	}
}

func TestRunNewsMailsAndSuppressesRepeatedErrors(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	goplan.Use(nil)
	t.Cleanup(func() { goplan.Use(nil) })
	newsCatalog(t, 10)
	newsAA(t)

	oldMailer := newsMailer
	mails := 0
	newsMailer = func(to []string, subject, body string) error {
		mails++
		if len(to) != 1 || to[0] != "ops@example.com" {
			t.Errorf("recipients = %v", to)
		}
		return nil
	}
	t.Cleanup(func() { newsMailer = oldMailer })

	run := func() string {
		var buf bytes.Buffer
		if err := runNews(&buf, "ops@example.com", false, true, false, time.Hour, nil); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}

	if out := run(); out != "" { // baseline
		t.Fatalf("first run printed %q", out)
	}
	if mails != 0 {
		t.Fatalf("first run mailed %d times", mails)
	}

	// A change is mailed once.
	newsCatalog(t, 12)
	if out := run(); !strings.HasPrefix(out, "COST_UP ") {
		t.Fatalf("change run = %q", out)
	}
	if mails != 1 {
		t.Fatalf("change mailed %d times, want 1", mails)
	}

	// Break the catalog fetch: the first ERROR mails, the second does not.
	if err := removeCacheFile("modelsdev-opencode-go.json"); err != nil {
		t.Fatal(err)
	}
	if out := run(); !strings.HasPrefix(out, "ERROR models.dev:") {
		t.Fatalf("broken run = %q", out)
	}
	if mails != 2 {
		t.Fatalf("first ERROR mailed %d times, want 2 total", mails)
	}
	if out := run(); !strings.HasPrefix(out, "ERROR models.dev:") {
		t.Fatalf("second broken run = %q", out)
	}
	if mails != 2 {
		t.Errorf("repeated ERROR mailed again: %d", mails)
	}

	// Recovery reports the change accumulated since the last good run and
	// mails it (the previous output was an ERROR, but this one is not).
	newsCatalog(t, 15)
	if out := run(); !strings.HasPrefix(out, "COST_UP ") {
		t.Fatalf("recovery run = %q", out)
	}
	if mails != 3 {
		t.Errorf("recovery mailed %d times, want 3 total", mails)
	}
}

func TestRunNewsKeepsLiveBaselineWhenOffline(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	goplan.Use(nil)
	t.Cleanup(func() { goplan.Use(nil) })
	newsCatalog(t, 10)
	newsAA(t)

	var buf bytes.Buffer
	if err := runNews(&buf, "", false, true, false, time.Hour, nil); err != nil {
		t.Fatal(err)
	}
	st, ok := loadNewsState()
	if !ok || !st.Baseline {
		t.Fatal("baseline not saved")
	}
	// Pretend the baseline came from a live plan snapshot.
	st.PlanSource = "live"
	if err := saveNewsState(st); err != nil {
		t.Fatal(err)
	}
	before := st.Models["kimi-k3.1-pro"].CostTask

	// A later -offline run cannot fetch the plan and falls back to the
	// built-in table: comparing would invent budget/profile changes.
	newsCatalog(t, 12)
	buf.Reset()
	if err := runNews(&buf, "", false, true, false, time.Hour, nil); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("offline run with a live baseline printed %q", buf.String())
	}
	after, ok := loadNewsState()
	if !ok || after.PlanSource != "live" {
		t.Fatalf("baseline was overwritten: %+v", after)
	}
	if got := after.Models["kimi-k3.1-pro"].CostTask; got != before {
		t.Errorf("baseline advanced offline: cost %v, want %v", got, before)
	}
}

func removeCacheFile(key string) error {
	dir, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(dir, "opencodable", key))
}
