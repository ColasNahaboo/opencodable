package table

import "testing"

func TestMarkPareto(t *testing.T) {
	rows := []Row{
		{Name: "cheap-fast", CostTask: 0.001, TokPerSec: 100, AAIntel: 40},
		{Name: "cheap-slow", CostTask: 0.001, TokPerSec: 10, AAIntel: 40},
		{Name: "pricey-best", CostTask: 0.02, TokPerSec: 90, AAIntel: 55},
		{Name: "dominated", CostTask: 0.03, TokPerSec: 50, AAIntel: 30},
		{Name: "nodata", CostTask: 0},
	}
	markPareto(rows)
	want := map[string]bool{
		"cheap-fast": true, "pricey-best": true,
		"cheap-slow": false, "dominated": false, "nodata": false,
	}
	for _, r := range rows {
		if r.Pareto != want[r.Name] {
			t.Errorf("%s: pareto=%v, want %v", r.Name, r.Pareto, want[r.Name])
		}
	}
}

func TestSort(t *testing.T) {
	rows := []Row{
		{Name: "b", TokPerSec: 10, AAIntel: 20, CostTask: 0.5, ReqsMonth: 10},
		{Name: "a", TokPerSec: 50, AAIntel: 10, CostTask: 0.1, ReqsMonth: 100},
		{Name: "c", TokPerSec: 20, AAIntel: 30, CostTask: 1, ReqsMonth: 1},
	}
	Sort(rows, "speed")
	if rows[0].Name != "a" || rows[2].Name != "b" {
		t.Errorf("speed sort wrong: %v %v %v", rows[0].Name, rows[1].Name, rows[2].Name)
	}
	Sort(rows, "quality")
	if rows[0].Name != "c" {
		t.Errorf("quality sort wrong: first=%s", rows[0].Name)
	}
	Sort(rows, "price")
	if rows[0].Name != "a" {
		t.Errorf("price sort wrong: first=%s", rows[0].Name)
	}
	Sort(rows, "reqs")
	if rows[0].Name != "a" || rows[2].Name != "c" {
		t.Errorf("reqs sort wrong: %v %v %v", rows[0].Name, rows[1].Name, rows[2].Name)
	}
	Sort(rows, "bogus") // falls back to name
	if rows[0].Name != "a" {
		t.Errorf("fallback sort wrong: first=%s", rows[0].Name)
	}
}

func TestNum(t *testing.T) {
	cases := []struct {
		v     float64
		money bool
		want  string
	}{
		{0, true, "-"},
		{10, true, "$10"},
		{0.0027898000000000003, true, "$0.00279"},
		{0.00026, true, "$0.00026"},
		{1578.9, false, "1578.9"},
	}
	for _, c := range cases {
		if got := num(c.v, c.money); got != c.want {
			t.Errorf("num(%v,%v)=%q want %q", c.v, c.money, got, c.want)
		}
	}
}

func TestQualFallsBackToCoding(t *testing.T) {
	r := Row{AACoding: 70}
	if qual(r) != 70 {
		t.Errorf("qual fallback = %v", qual(r))
	}
	r2 := Row{AAIntel: 55, AACoding: 70}
	if qual(r2) != 55 {
		t.Errorf("qual prefer intel = %v", qual(r2))
	}
}
