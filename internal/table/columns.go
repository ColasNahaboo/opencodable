package table

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// NowTitles lists the -now view columns in output order. The first title
// keeps its # prefix so it is never confused with a legend line.
func NowTitles() []string {
	return []string{
		"#name", "cost_task", "burn_req", "spent_mo", "reqs_5h",
		"reqs_wk", "reqs_mo", "usable", "limit", "p_tok_s",
	}
}

// FullTitles lists the -full view columns: every main column first, then
// every -now column not already present, in -now order. Derived, not
// hardcoded, so the order follows automatically when a view gains a column.
// Titles are unique across views: main reqs_mo became r_quota, so -now
// reqs_mo needs no alias.
func FullTitles() []string {
	out := append([]string{}, HeaderCells()...)
	seen := map[string]bool{}
	for _, t := range out {
		seen[normTitle(t)] = true
	}
	for _, t := range NowTitles() {
		if !seen[normTitle(t)] {
			out = append(out, t)
		}
	}
	return out
}

// normTitle canonicalizes a column name for comparison: case-insensitive,
// without the # prefix, untrimmed input tolerated.
func normTitle(t string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(t), "#"))
}

// KnownColumn reports whether name is a column title in any model view:
// main, -now, or -full-only aliases. Used to validate -cols/-excols so a
// name valid in another view is silently ignored instead of erroring.
func KnownColumn(name string) bool {
	n := normTitle(name)
	if n == "" {
		return false
	}
	for _, t := range FullTitles() {
		if normTitle(t) == n {
			return true
		}
	}
	return false
}

// SelectTitles resolves -cols (includes) and -excols (excludes) against one
// view's titles, preserving view order: intersect-then-subtract. A name
// unknown in every view is an error; a name known only in another view is
// silently ignored, so one global set works for every view. The name column
// is always shown.
func SelectTitles(view, includes, excludes []string) ([]string, error) {
	for _, n := range includes {
		if !KnownColumn(n) {
			return nil, fmt.Errorf("-cols: unknown column %q", n)
		}
	}
	for _, n := range excludes {
		if !KnownColumn(n) {
			return nil, fmt.Errorf("-excols: unknown column %q", n)
		}
	}
	inc := map[string]bool{}
	for _, n := range includes {
		inc[normTitle(n)] = true
	}
	exc := map[string]bool{}
	for _, n := range excludes {
		exc[normTitle(n)] = true
	}
	var out []string
	for _, t := range view {
		k := normTitle(t)
		if k == "name" {
			out = append(out, t) // join key and header anchor: always shown
			continue
		}
		if len(inc) > 0 {
			if inc[k] && !exc[k] {
				out = append(out, t)
			}
		} else if !exc[k] {
			out = append(out, t)
		}
	}
	return out, nil
}

// ProjectCells picks from a full-order cell row the cells named by sel.
func ProjectCells(full, row, sel []string) []string {
	idx := make(map[string]int, len(full))
	for i, t := range full {
		idx[t] = i
	}
	out := make([]string, 0, len(sel))
	for _, t := range sel {
		if i, ok := idx[t]; ok && i < len(row) {
			out = append(out, row[i])
		}
	}
	return out
}

// FilterLegend keeps prose lines plus the legend lines of selected columns.
// A line counts as a column line when its second field names a known column.
func FilterLegend(lines, sel []string) []string {
	keep := map[string]bool{}
	for _, t := range sel {
		keep[normTitle(t)] = true
	}
	var out []string
	for _, l := range lines {
		f := strings.Fields(l)
		if len(f) < 2 || !KnownColumn(f[1]) {
			out = append(out, l) // prose header, not a column line
			continue
		}
		if keep[strings.ToLower(f[1])] {
			out = append(out, l)
		}
	}
	return out
}

// FullRow joins one main Row with its -now affordability data for the -full
// view. JSON keys equal the -full column titles; the extras below travel
// alongside every selection.
type FullRow struct {
	Row
	Burn           float64 `json:"burn_req"`
	SpentMo        float64 `json:"spent_mo"`
	SpentKnown     bool    `json:"spent_known"`
	Reqs5h         int     `json:"reqs_5h"`
	ReqsWk         int     `json:"reqs_wk"`
	ReqsMo         int     `json:"reqs_mo"`
	Usable         int     `json:"usable"`
	Limit          string  `json:"limit"`
	Known          bool    `json:"known"`
	WindowEstimate bool    `json:"window_estimate"`
	WindowMethod   string  `json:"window_method"`
}

// SpentCell formats spent_mo: blank when spend is unknown, $0 when known
// zero. The -now view shares it so all views format identically.
func SpentCell(v float64, ok bool, missing string) string {
	if !ok {
		return missing
	}
	if v == 0 {
		return "$0"
	}
	return numOr(v, true, missing)
}

// FullCells returns one -full data row: the main cells in main order,
// then the -now-only cells in -now order. Free models show no counts
// (limit=unlimited); models without plan data show no counts either.
func FullCells(r FullRow, missing string) []string {
	out := RowCells(r.Row, missing)
	b, s, c5, cw, cm, u := missing, missing, missing, missing, missing, missing
	if !r.Row.Unlimited {
		b = numOr(r.Burn, true, missing)
		s = SpentCell(r.SpentMo, r.SpentKnown, missing)
		if r.Known {
			c5, cw, cm, u = strconv.Itoa(r.Reqs5h), strconv.Itoa(r.ReqsWk),
				strconv.Itoa(r.ReqsMo), strconv.Itoa(r.Usable)
		}
	}
	return append(out, b, s, c5, cw, cm, u, r.Limit)
}

// fullRank orders usable-first: paid models, then unlimited, then unknown.
func fullRank(r FullRow) int {
	if r.Known && !r.Row.Unlimited {
		return 0
	}
	if r.Row.Unlimited {
		return 1
	}
	return 2
}

// SortFull orders -full rows in place. Keys mirror Sort, plus usable for the
// affordable-requests ranking; unknown keys fall back to name, like Sort.
func SortFull(rows []FullRow, key string) {
	var less func(a, b FullRow) bool
	switch key {
	case "price":
		less = func(a, b FullRow) bool { return a.Row.CostTask < b.Row.CostTask }
	case "speed":
		less = func(a, b FullRow) bool { return Speed(a.Row) > Speed(b.Row) }
	case "quality":
		less = func(a, b FullRow) bool { return Quality(a.Row) > Quality(b.Row) }
	case "reqs":
		less = func(a, b FullRow) bool { return reqsRank(a.Row) > reqsRank(b.Row) }
	case "usable":
		less = func(a, b FullRow) bool {
			if fullRank(a) != fullRank(b) {
				return fullRank(a) < fullRank(b)
			}
			return a.Usable > b.Usable
		}
	default: // name
		less = func(a, b FullRow) bool { return a.Row.Name < b.Row.Name }
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if less(rows[i], rows[j]) {
			return true
		}
		if less(rows[j], rows[i]) {
			return false
		}
		if rows[i].Row.Name != rows[j].Row.Name {
			return rows[i].Row.Name < rows[j].Row.Name
		}
		return rows[i].Row.ID < rows[j].Row.ID
	})
}
