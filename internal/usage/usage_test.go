package usage

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	ws, err := parse(strings.NewReader(`{"usage":{
		"rolling":{"status":"ok","percent":1,"resetsAt":"2026-10-27T20:43:22.000Z"},
		"weekly":{"status":"throttled","percent":8,"resetsAt":"2026-11-02T00:00:00.000Z"},
		"monthly":{"status":"ok","percent":17,"resetsAt":"2026-11-07T13:56:38.000Z"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 3 {
		t.Fatalf("want 3 windows, got %d", len(ws))
	}
	if ws[0].Name != "5h" || ws[0].Percent != 1 || ws[0].Remaining() != 99 {
		t.Errorf("rolling window wrong: %+v", ws[0])
	}
	if ws[1].Status != "throttled" || ws[1].Remaining() != 92 {
		t.Errorf("weekly window wrong: %+v", ws[1])
	}
	if ws[2].ResetsAt.IsZero() {
		t.Error("monthly resetsAt not parsed")
	}
}

func TestParseGarbage(t *testing.T) {
	if _, err := parse(strings.NewReader("nope")); err == nil {
		t.Error("want error on garbage")
	}
}

func TestHumanizeDuration(t *testing.T) {
	cases := map[time.Duration]string{
		4*time.Hour + 23*time.Minute:                   "4h 23m 0s",
		128*time.Hour + 11*time.Minute + 5*time.Second: "128h 11m 5s",
		45 * time.Second:                               "45s",
		2*time.Minute + 30*time.Second:                 "2m 30s",
	}
	for d, want := range cases {
		if got := HumanizeDuration(d); got != want {
			t.Errorf("HumanizeDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestRequests(t *testing.T) {
	b := Budgets{Five: 1.98, Week: 4.6, Month: 8.3}
	five, week, month, usable, binding := Requests(b, 0.02)
	if five != 99 || week != 230 || month != 415 {
		t.Errorf("counts = %d %d %d, want 99 230 415", five, week, month)
	}
	if usable != 99 || binding != "5h" {
		t.Errorf("usable=%d binding=%s, want 99 5h", usable, binding)
	}
	// the remaining monthly allowance can bind instead of the 5h cap
	b2 := Budgets{Five: 1.98, Week: 4.6, Month: 0.5}
	_, _, _, usable, binding = Requests(b2, 0.02)
	if usable != 25 || binding != "month" {
		t.Errorf("usable=%d binding=%s, want 25 month", usable, binding)
	}
	// over-spent models afford zero, never a negative count
	b3 := Budgets{Five: 1.98, Week: 4.6, Month: -3}
	five, week, month, usable, binding = Requests(b3, 0.02)
	if five != 99 || week != 230 || month != 0 || usable != 0 || binding != "month" {
		t.Errorf("negative budget = %d %d %d %d %s, want 99 230 0 0 month",
			five, week, month, usable, binding)
	}
	// zero burn is refused
	if _, _, _, _, binding = Requests(b, 0); binding != "-" {
		t.Errorf("zero burn binding = %q", binding)
	}
}

func TestParseSpentFile(t *testing.T) {
	f := t.TempDir() + "/spent.tsv"
	content := "glm-5.3-flash\t232\t1.0M\t29.3M\t-\t107.8k\t$1.09\n" +
		"kimi-k3.1-pro\t3\t312\t-\t-\t950\t<$0.01\n" +
		"# comment line\n" +
		"garbage line\n"
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	spent, err := ParseSpentFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if spent["glm-5.3-flash"] != 1.09 {
		t.Errorf("glm spend = %v, want 1.09", spent["glm-5.3-flash"])
	}
	if spent["kimi-k3.1-pro"] != 0.01 {
		t.Errorf("kimi spend = %v, want 0.01 (<$0.01 rounds up)", spent["kimi-k3.1-pro"])
	}
	if v, ok := spent["garbage"]; ok {
		t.Errorf("garbage line parsed as %v", v)
	}
}
