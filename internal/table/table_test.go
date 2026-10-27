package table

import (
	"encoding/json"
	"strings"
	"testing"
)

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

func TestNumOr(t *testing.T) {
	cases := []struct {
		v       float64
		money   bool
		missing string
		want    string
	}{
		{0, true, "-", "-"},
		{0, true, "", ""},
		{10, true, "-", "$10"},
		{0.0027898000000000003, true, "", "$0.00279"},
		{0.00026, true, "", "$0.00026"},
		{1578.9, false, "-", "1578.9"},
	}
	for _, c := range cases {
		if got := numOr(c.v, c.money, c.missing); got != c.want {
			t.Errorf("numOr(%v,%v,%q)=%q want %q", c.v, c.money, c.missing, got, c.want)
		}
	}
}

func TestWriteTSVMachineMode(t *testing.T) {
	rows := []Row{
		{Name: "m1", PriceOut1M: 10, CostTask: 0.02, TokPerSec: 123.45, Pareto: true, Src: "aa:high"},
		{Name: "m2"},
	}
	var b strings.Builder
	if err := WriteTSV(&b, rows, false); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	nCol := len(headerCells())
	headJoin := strings.Join(headerCells(), "\t")
	var row1, row2 []string
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "# "): // legend prose: no TAB constraints
		case l == headJoin:
			// header
		default: // data row
			if got := len(strings.Split(l, "\t")); got != nCol {
				t.Errorf("line %q has %d fields, want %d", l, got, nCol)
			}
			if strings.Contains(l, "  ") {
				t.Errorf("machine mode must not pad: %q", l)
			}
			f := strings.Split(l, "\t")
			if f[0] == "m1" {
				row1 = f
			}
			if f[0] == "m2" {
				row2 = f
			}
		}
	}
	if row1[len(row1)-1] != "aa:high" {
		t.Errorf("source cell = %q, want aa:high", row1[len(row1)-1])
	}
	for i, v := range row2 {
		if i == 0 {
			continue
		}
		if v != "" {
			t.Errorf("m2 empty-model field %q = %q, want empty", headerCells()[i], v)
		}
	}
}

func TestWriteVertical(t *testing.T) {
	rows := []Row{
		{Name: "m1", PriceOut1M: 10, Pareto: true, Src: "aa:high"},
		{Name: "m2", AACoding: 70},
	}
	var b strings.Builder
	if err := WriteVertical(&b, rows, false); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	// every record starts with "name: ", fields are key: value, empties skipped
	if !strings.Contains(out, "name: m1\ncost_tok: $10\npareto: yes\nsrc: aa:high") {
		t.Errorf("m1 block wrong:\n%s", out)
	}
	if strings.Contains(out, "cost_task: ") {
		t.Error("empty fields must be skipped in vertical mode")
	}
	records := strings.Split(strings.TrimRight(out, "\n"), "\n\n")
	if len(records) != 2 {
		t.Errorf("want 2 records separated by a blank line, got %d:\n%s", len(records), out)
	}
	if !strings.HasPrefix(records[1], "name: m2\n") {
		t.Errorf("second record wrong: %q", records[1])
	}
}

func TestJSONKeysMatchTitles(t *testing.T) {
	rows := []Row{{Name: "m1", PriceOut1M: 10, Pareto: true, Src: "aa:high"}}
	b, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var recs []map[string]any
	if err := json.Unmarshal(b, &recs); err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	// every TSV column title (sans #) must exist as a JSON key
	for _, title := range headerCells() {
		key := strings.TrimPrefix(title, "#")
		if _, ok := recs[0][key]; !ok {
			t.Errorf("JSON missing key %q for column %q", key, title)
		}
	}
	if recs[0]["src"] != "aa:high" {
		t.Errorf("src = %v, want aa:high", recs[0]["src"])
	}
}

func TestWriteTSVTTYMode(t *testing.T) {
	rows := []Row{{Name: "m1", PriceOut1M: 10}}
	var b strings.Builder
	if err := WriteTSV(&b, rows, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "-") {
		t.Error("tty mode must print - for missing values")
	}
	// header and data aligned by tabwriter: at least one padded gap
	for _, l := range strings.Split(b.String(), "\n") {
		if strings.HasPrefix(l, "m1") && !strings.Contains(l, "  ") {
			t.Errorf("tty mode must pad columns: %q", l)
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
