// Package probe measures real speed of OpenCode Go models by streaming a
// fixed prompt through the subscription endpoint and timing the response.
package probe

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// endpoint is a var so tests can point it at a fake server.
var endpoint = "https://opencode.ai/inference/go/openai/v1/chat/completions"

// Prompt is the fixed probe prompt: long enough to measure throughput,
// short enough to cost a fraction of a cent.
const Prompt = "Write a FizzBuzz function in Python, then briefly explain the modulo logic in two sentences."

// LongPrompt forces a much longer natural output length, used when a model
// is so fast that the default answer finishes inside an untrustworthy window.
const LongPrompt = "Explain in detail (at least 600 words) how a binary search tree works: insertion, search, deletion, balancing, then give a full Python implementation with comments."

// MinWindow is the smallest generation window (seconds) we trust for tok/s.
// Below it, bursts (all tokens in one chunk) produce absurd numbers.
const MinWindow = 1.0

// Result is one measured request.
type Result struct {
	ModelID       string  `json:"model_id"`
	TokPerSec     float64 `json:"tok_s"`
	TTFTSeconds   float64 `json:"ttft_s"`
	E2ESeconds    float64 `json:"e2e_s"`
	WindowSeconds float64 `json:"window_s"`
	PromptTok     int64   `json:"prompt_tokens"`
	OutputTok     int64   `json:"output_tokens"`
	Reasoning     int64   `json:"reasoning_tokens"`
	CachedTok     int64   `json:"cached_tokens"`
	Allowance     float64 `json:"allowance_burned"`
	ListCost      float64 `json:"list_cost"`
	Error         string  `json:"error,omitempty"`
}

// Key returns the subscription key from OPENCODE_API_KEY or .opencode-go.key.
func Key() (string, error) {
	if k := os.Getenv("OPENCODE_API_KEY"); k != "" {
		return k, nil
	}
	if b, err := os.ReadFile(".opencode-go.key"); err == nil {
		return strings.TrimSpace(string(b)), nil
	}
	return "", fmt.Errorf("no OpenCode Go key: set OPENCODE_API_KEY or create .opencode-go.key")
}

type sseChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *usage `json:"usage"`
}

type usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	PromptDetails    *struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionDetails *struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
	Opencode *struct {
		ListCost  float64 `json:"list_cost"`
		Allowance float64 `json:"allowance"`
		Tokens    struct {
			Reasoning int64 `json:"reasoning"`
			CacheRead int64 `json:"cache_read"`
		} `json:"tokens"`
	} `json:"opencode"`
}

// Run streams the default probe prompt.
func Run(modelID, key string, maxTokens int64) (Result, error) {
	return runPrompt(modelID, key, Prompt, maxTokens)
}

// RunLong streams a prompt whose natural answer is much longer, for models
// too fast to fill a trustworthy window with the default prompt.
func RunLong(modelID, key string, maxTokens int64) (Result, error) {
	return runPrompt(modelID, key, LongPrompt, maxTokens)
}

func runPrompt(modelID, key, prompt string, maxTokens int64) (Result, error) {
	res := Result{ModelID: modelID}
	body, _ := json.Marshal(map[string]any{
		"model":          modelID,
		"messages":       []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens":     maxTokens,
		"stream":         true,
		"stream_options": map[string]bool{"include_usage": true},
	})
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return res, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "opencodable/0.1")
	req.Header.Set("x-opencode-session", "opencodable-probe")

	start := time.Now()
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return res, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return res, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var firstGen, firstContent, last time.Time
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}
		var ch sseChunk
		if json.Unmarshal([]byte(payload), &ch) != nil {
			continue
		}
		now := time.Now()
		if firstGen.IsZero() {
			for _, c := range ch.Choices {
				if c.Delta.Content != "" || c.Delta.ReasoningContent != "" {
					firstGen = now
					break
				}
			}
		}
		if firstContent.IsZero() {
			for _, c := range ch.Choices {
				if c.Delta.Content != "" {
					firstContent = now
					break
				}
			}
		}
		last = now
		if ch.Usage != nil {
			u := ch.Usage
			res.PromptTok = u.PromptTokens
			res.OutputTok = u.CompletionTokens
			if u.PromptDetails != nil {
				res.CachedTok = u.PromptDetails.CachedTokens
			}
			if u.CompletionDetails != nil {
				res.Reasoning = u.CompletionDetails.ReasoningTokens
			}
			if u.Opencode != nil {
				res.ListCost = u.Opencode.ListCost
				res.Allowance = u.Opencode.Allowance
			}
		}
	}
	if err := sc.Err(); err != nil {
		return res, err
	}
	if firstGen.IsZero() || res.OutputTok == 0 {
		return res, fmt.Errorf("no content received")
	}
	genStart := firstGen
	if firstContent.IsZero() {
		firstContent = firstGen
	}
	res.TTFTSeconds = firstContent.Sub(start).Seconds()
	res.E2ESeconds = last.Sub(start).Seconds()
	res.WindowSeconds = last.Sub(genStart).Seconds()
	res.TokPerSec = float64(res.OutputTok) / res.WindowSeconds
	return res, nil
}
