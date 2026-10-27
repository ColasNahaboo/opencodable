package probe

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// sseServer builds chunks: any delay markers "W<ms>" between events.
func sseServer(t *testing.T, chunks []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		for _, c := range chunks {
			if strings.HasPrefix(c, "W") {
				var ms int
				fmt.Sscanf(c, "W%d", &ms)
				time.Sleep(time.Duration(ms) * time.Millisecond)
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", c)
			f.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		f.Flush()
	}))
}

func chunk(content string) string {
	return fmt.Sprintf(`{"choices":[{"delta":{"content":%q}}]}`, content)
}

func reasoningChunk(s string) string {
	return fmt.Sprintf(`{"choices":[{"delta":{"reasoning_content":%q}}]}`, s)
}

func usageChunk(out, reasoning int64) string {
	return fmt.Sprintf(`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":%d,`+
		`"prompt_tokens_details":{"cached_tokens":5},`+
		`"completion_tokens_details":{"reasoning_tokens":%d},`+
		`"opencode":{"list_cost":0.001,"allowance":0.002,`+
		`"tokens":{"reasoning":%d,"cache_read":5}}}}`, out, reasoning, reasoning)
}

func TestRunMeasuresThinkingModel(t *testing.T) {
	// Reasoning model: reasoning_content first, content later, usage last.
	srv := sseServer(t, []string{
		"W300", reasoningChunk("think"), "W300", reasoningChunk(" more"),
		"W300", chunk("Hello"), "W400", chunk(" world"), usageChunk(200, 150),
	})
	t.Cleanup(srv.Close)
	old := endpoint
	endpoint = srv.URL
	defer func() { endpoint = old }()

	res, err := Run("test-model", "fake-key", 1024)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.TTFTSeconds < 0.8 || res.TTFTSeconds > 1.6 {
		t.Errorf("ttft = %v, want ~0.9-1.5s (thinking then content)", res.TTFTSeconds)
	}
	if res.WindowSeconds < 0.8 || res.WindowSeconds > 1.3 {
		t.Errorf("window = %v, want ~1.0s (first token at 300ms, last at 1300ms)", res.WindowSeconds)
	}
	if res.TokPerSec <= 0 || res.TokPerSec > 400 {
		t.Errorf("tok/s = %v, want plausible 2-token burst rate", res.TokPerSec)
	}
	if res.OutputTok != 200 || res.Reasoning != 150 {
		t.Errorf("tokens = out %d reasoning %d", res.OutputTok, res.Reasoning)
	}
	if res.CachedTok != 5 || res.Allowance != 0.002 || res.ListCost != 0.001 {
		t.Errorf("usage decode wrong: %+v", res)
	}
}

func TestRunRejectsNoContent(t *testing.T) {
	srv := sseServer(t, []string{"W500", usageChunk(0, 0)})
	t.Cleanup(srv.Close)
	old := endpoint
	endpoint = srv.URL
	defer func() { endpoint = old }()

	if _, err := Run("m", "k", 1024); err == nil {
		t.Error("want error when no tokens generated")
	}
}

func TestNewSessionID(t *testing.T) {
	a, b := newSessionID(), newSessionID()
	if a == b {
		t.Fatalf("session IDs collided: %q", a)
	}
	if !strings.HasPrefix(a, "opencodable-") {
		t.Errorf("session ID %q lacks prefix", a)
	}
}

func TestRunFlagsImplausibleBurst(t *testing.T) {
	srv := sseServer(t, []string{
		"W10", chunk("Hello"), "W10", chunk(" world"), usageChunk(600, 0),
	})
	t.Cleanup(srv.Close)
	old := endpoint
	endpoint = srv.URL
	defer func() { endpoint = old }()

	res, err := Run("m", "k", 1024)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Suspect {
		t.Errorf("tok/s = %v in a %v s window must be flagged suspect", res.TokPerSec, res.WindowSeconds)
	}
}

func TestRunRejectsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"model unavailable"}`, http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	old := endpoint
	endpoint = srv.URL
	defer func() { endpoint = old }()

	_, err := Run("m", "k", 1024)
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Errorf("want HTTP 400 error, got %v", err)
	}
}
