package usage

import (
	"math"
	"os"
	"strings"
	"testing"
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

func TestRemainingBudgets(t *testing.T) {
	ws := []Window{
		{Name: "5h", Percent: 1},
		{Name: "week", Percent: 8},
		{Name: "month", Percent: 17},
	}
	b := RemainingBudgets(ws, 10)
	cases := []struct {
		name      string
		got, want float64
	}{{"five", b.Five, 1.98}, {"week", b.Week, 4.6}, {"month", b.Month, 8.3}}
	for _, c := range cases {
		if diff := math.Abs(c.got - c.want); diff > 1e-9 {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if b.ModelMonth != nil {
		t.Error("ModelMonth must be nil (unknown) without a spent file")
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
	// per-model bucket binds the month
	mm := 0.5
	b2 := Budgets{Five: 1.98, Week: 4.6, Month: 8.3, ModelMonth: &mm}
	_, _, _, usable, binding = Requests(b2, 0.02)
	if usable != 25 || binding != "allowance" {
		t.Errorf("usable=%d binding=%s, want 25 allowance", usable, binding)
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
