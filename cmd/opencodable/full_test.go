package main

import (
	"testing"

	"github.com/ColasNahaboo/opencodable/internal/table"
)

func TestJoinFull(t *testing.T) {
	mrows := []table.Row{
		{ID: "paid", Name: "Paid", CostTask: 0.01, ProbeTokS: 100},
		{ID: "free", Name: "Free", Unlimited: true},
		{ID: "priceless", Name: "Priceless"},
	}
	nrows := []nowRow{
		{id: "paid", name: "Paid", costTask: 0.01, burn: 0.02,
			spentMo: 1, spentOK: true, five: 10, week: 20, month: 30,
			usable: 10, binding: "5h", known: true, probeTokS: 100},
		{id: "free", name: "Free", binding: "unlimited", unlimited: true},
	}
	got := joinFull(mrows, nrows, false)
	if len(got) != 3 {
		t.Fatalf("join = %d rows, want 3", len(got))
	}
	paid := got[0]
	if paid.Burn != 0.02 || paid.Usable != 10 || paid.Limit != "5h" || !paid.Known {
		t.Errorf("paid join wrong: %+v", paid)
	}
	if paid.Row.ProbeTokS != 100 {
		t.Errorf("paid probe speed = %v, want 100", paid.Row.ProbeTokS)
	}
	if got[1].Limit != "unlimited" || !got[1].Row.Unlimited {
		t.Errorf("free join wrong: %+v", got[1])
	}
	// Priceless models have no -now row: static columns kept, limit unknown.
	if got[2].Known || got[2].Limit != "unknown" {
		t.Errorf("priceless join wrong: %+v", got[2])
	}
	if got[2].WindowEstimate {
		t.Error("window_estimate must follow the flag")
	}
	for _, r := range joinFull(mrows, nrows, true) {
		if !r.WindowEstimate {
			t.Error("window_estimate must follow the flag")
		}
	}
}

func TestFullLegendFilters(t *testing.T) {
	sel := []string{"#name", "cost_task", "usable"}
	lines := fullLegend(sel)
	joined := ""
	for _, l := range lines {
		joined += l + "\n"
	}
	for _, want := range []string{"# name", "# cost_task", "# usable"} {
		found := false
		for _, l := range lines {
			if len(l) >= len(want) && l[:len(want)] == want {
				found = true
			}
		}
		if !found {
			t.Errorf("legend missing %q:\n%s", want, joined)
		}
	}
	for _, l := range lines {
		if len(l) > 7 && l[:7] == "# swe_b" {
			t.Errorf("legend must not document hidden columns: %q", l)
		}
	}
}

func TestSelectedViews(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{nil, nil},
		{[]string{"-main"}, []string{"-main"}},
		{[]string{"-now"}, []string{"-now"}},
		{[]string{"-now", "-full"}, []string{"-now", "-full"}},
	} {
		writeConf(t, "")
		fs := registerOptions(t)
		if err := fs.Parse(tc.args); err != nil {
			t.Fatal(err)
		}
		if err := applyConfig(fs); err != nil {
			t.Fatal(err)
		}
		got := selectedViews()
		if len(got) != len(tc.want) {
			t.Errorf("%v: views = %v, want %v", tc.args, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%v: views = %v, want %v", tc.args, got, tc.want)
			}
		}
	}
}
