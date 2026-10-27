// Package usage reads the Go subscription's remaining allowance windows.
package usage

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const usageURL = "https://opencode.ai/inference/go/v1/usage"

// Window is one usage-limit period (rolling=5h, weekly, monthly).
type Window struct {
	Name     string
	Status   string
	Percent  float64 // percent of the allowance already used
	ResetsAt time.Time
}

// Remaining returns 100 - used percent.
func (w Window) Remaining() float64 { return 100 - w.Percent }

type payload struct {
	Usage struct {
		Rolling struct {
			Status   string  `json:"status"`
			Percent  float64 `json:"percent"`
			ResetsAt string  `json:"resetsAt"`
		} `json:"rolling"`
		Weekly struct {
			Status   string  `json:"status"`
			Percent  float64 `json:"percent"`
			ResetsAt string  `json:"resetsAt"`
		} `json:"weekly"`
		Monthly struct {
			Status   string  `json:"status"`
			Percent  float64 `json:"percent"`
			ResetsAt string  `json:"resetsAt"`
		} `json:"monthly"`
	} `json:"usage"`
}

// Fetch reads the current allowance usage for the Go plan.
func Fetch(key string) ([]Window, error) {
	req, err := http.NewRequest(http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}
	return parse(resp.Body)
}

func parse(r io.Reader) ([]Window, error) {
	var p payload
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return nil, err
	}
	mk := func(name, status string, pct float64, reset string) Window {
		at, _ := time.Parse(time.RFC3339, reset)
		return Window{Name: name, Status: status, Percent: pct, ResetsAt: at}
	}
	u := p.Usage
	return []Window{
		mk("5h", u.Rolling.Status, u.Rolling.Percent, u.Rolling.ResetsAt),
		mk("week", u.Weekly.Status, u.Weekly.Percent, u.Weekly.ResetsAt),
		mk("month", u.Monthly.Status, u.Monthly.Percent, u.Monthly.ResetsAt),
	}, nil
}

// WindowFactors are the remaining fractions of the aggregate 5h and weekly
// windows. They let -now scale each model's caps by the account-wide usage
// reported by the endpoint; this is an estimate, because the endpoint does
// not expose per-model window spend.
type WindowFactors struct {
	Five float64
	Week float64
}

// RemainingFactors converts the endpoint percentages into remaining
// fractions, defaulting to 1 (full window) when a window is absent.
func RemainingFactors(ws []Window) WindowFactors {
	f := WindowFactors{Five: 1, Week: 1}
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 1 {
			return 1
		}
		return v
	}
	for _, w := range ws {
		rem := clamp(1 - w.Percent/100)
		switch w.Name {
		case "5h":
			f.Five = rem
		case "week":
			f.Week = rem
		}
	}
	return f
}

// Budgets is the remaining allowance dollars per window. Go limits are
// per model: a model's 5h and weekly caps are 20% and 50% of its own
// monthly limit, and Month is what remains of that monthly limit.
type Budgets struct {
	Five  float64 // 5h rolling window cap
	Week  float64 // weekly cap
	Month float64 // remaining monthly allowance
}

// Requests returns how many requests of the given allowance burn each window
// can still afford, the usable (binding) count, and which window binds.
// Negative budgets are clamped to zero: an over-spent model affords nothing.
func Requests(b Budgets, burnPerReq float64) (five, week, month, usable int, binding string) {
	if burnPerReq <= 0 {
		return 0, 0, 0, 0, "-"
	}
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		return v
	}
	floor := func(v float64) int { return int(clamp(v)/burnPerReq + 1e-9) }
	five = floor(b.Five)
	week = floor(b.Week)
	month = floor(b.Month)
	usable, binding = five, "5h"
	if week < usable {
		usable, binding = week, "week"
	}
	if month < usable {
		usable, binding = month, "month"
	}
	return
}

// HumanizeDuration formats a Go duration with spaces between components:
// "4h23m0s" → "4h 23m 0s".
func HumanizeDuration(d time.Duration) string {
	s := d.String()
	var b strings.Builder
	for i, r := range s {
		b.WriteRune(r)
		if (r == 'h' || r == 'm' || r == 's') &&
			i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// ParseSpentFile reads a console "usage by model" paste (tab or 2+-space
// separated lines: model id, counts..., last field a $cost, "<$0.01" allowed)
// and returns spent allowance dollars per model ID.
func ParseSpentFile(path string) (map[string]float64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]float64{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := splitFields(line)
		if len(fields) < 2 {
			continue
		}
		cost := strings.TrimPrefix(strings.TrimPrefix(fields[len(fields)-1], "<"), "$")
		cost = strings.ReplaceAll(cost, ",", "")
		v, err := strconv.ParseFloat(strings.TrimSpace(cost), 64)
		if err != nil || v < 0 {
			continue
		}
		out[fields[0]] = v
	}
	return out, nil
}

// splitFields splits on tabs or runs of two or more spaces. A single space
// stays inside a field, so display names such as "Kimi K3.1 Pro" survive a
// console paste.
func splitFields(line string) []string {
	var fields []string
	var b strings.Builder
	spaces := 0
	flush := func() {
		if b.Len() > 0 {
			fields = append(fields, b.String())
			b.Reset()
		}
	}
	for _, r := range line {
		switch {
		case r == '\t':
			flush()
			spaces = 0
		case r == ' ':
			spaces++
			if spaces >= 2 {
				flush()
			}
		default:
			if spaces == 1 && b.Len() > 0 {
				b.WriteByte(' ')
			}
			spaces = 0
			b.WriteRune(r)
		}
	}
	flush()
	return fields
}
