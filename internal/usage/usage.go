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

// Budgets is the remaining allowance dollars per window. ModelMonth nil =
// per-model bucket unknown; otherwise it constrains the month window.
type Budgets struct {
	Five       float64 // 5h rolling window
	Week       float64
	Month      float64
	ModelMonth *float64 // per-model bucket remaining, nil = unknown
}

// RemainingBudgets converts usage windows into remaining allowance dollars,
// given the plan pot (the monthly dollar-equivalent the percentages meter)
// and the documented window caps (5h = 20%, weekly = 50%, monthly = 100%).
func RemainingBudgets(ws []Window, pot float64) Budgets {
	caps := map[string]float64{"5h": 0.2, "week": 0.5, "month": 1.0}
	b := Budgets{}
	for _, w := range ws {
		rem := pot * caps[w.Name] * (1 - w.Percent/100)
		switch w.Name {
		case "5h":
			b.Five = rem
		case "week":
			b.Week = rem
		case "month":
			b.Month = rem
		}
	}
	return b
}

// Requests returns how many requests of the given allowance burn each window
// can still afford, the usable (binding) count, and which window binds. The
// per-model bucket, when known, constrains the month window.
func Requests(b Budgets, burnPerReq float64) (five, week, month, usable int, binding string) {
	if burnPerReq <= 0 {
		return 0, 0, 0, 0, "-"
	}
	floor := func(v float64) int { return int(v/burnPerReq + 1e-9) }
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
	if b.ModelMonth != nil {
		if mm := floor(*b.ModelMonth); mm < usable {
			usable, binding = mm, "allowance"
		}
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
		v, err := strconv.ParseFloat(cost, 64)
		if err != nil || v < 0 {
			continue
		}
		out[fields[0]] = v
	}
	return out, nil
}

// splitFields splits on tabs or runs of two or more spaces.
func splitFields(line string) []string {
	return strings.FieldsFunc(line, func(r rune) bool {
		return r == '\t' || r == ' '
	})
}
