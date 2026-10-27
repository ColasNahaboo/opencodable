package usage

import (
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
