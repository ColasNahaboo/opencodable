// Package usage reads the Go subscription's remaining allowance windows.
package usage

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
