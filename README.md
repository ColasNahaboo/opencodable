# opencodable

Rank the models of an [OpenCode](https://opencode.ai) Go subscription by the
best compromise of **price, speed and quality**, right now. Output is plain
tab-separated text, one line per model.

## Data sources

| Source | Gives | Auth |
|---|---|---|
| [models.dev](https://models.dev) `api.json` | retail prices ($/1M in/out/cached), context limits, reasoning flags | none |
| [OpenCode Go docs](https://opencode.ai/v2/docs/console/go/) (hardcoded snapshot in `internal/goplan`) | per-model monthly allowance, official per-request token profile, peak prices | none |
| [Artificial Analysis](https://artificialanalysis.ai) data API | intelligence/coding/math indexes, LiveCodeBench, Terminal-Bench, IFBench, long-context reasoning, tok/s, TTFT | free key |
| [SWE-bench Verified](https://www.swebench.com/) leaderboard JSON | % resolved, best recorded agent run | none |
| Live speed probe against your subscription (`-probe`) | real tok/s, TTFT, allowance burn per request | `OPENCODE_API_KEY` |

Dropped with cause: LiveBench (data files not machine-accessible), Aider
polyglot (public YAML files stale since 2025-10, no 2026 models).

## Setup

```sh
# Artificial Analysis free API key, either form works:
export AA_API_KEY=...            # or: echo KEY > .artificialanalysis.key  (git-ignored)

# Only for the live speed probe:
export OPENCODE_API_KEY=...      # or: echo KEY > .opencode-go.key  (git-ignored)
```

## Usage

```sh
go build -o TMP/opencodable ./cmd/opencodable
TMP/opencodable                       # ranked by quality, all subscription models
TMP/opencodable -pareto -paid         # only the non-dominated paid models
TMP/opencodable -sort speed           # name|price|speed|quality|reqs
TMP/opencodable -plan go-plus         # allowances for the Go Plus plan
TMP/opencodable -peak                 # DeepSeek peak-hours pricing
TMP/opencodable -json                 # machine-readable
TMP/opencodable -refresh              # bypass the 24h cache
TMP/opencodable -offline              # cache only
TMP/opencodable -probe                # measure live speed of all models (paid, cached 24h)
TMP/opencodable -probe-model kimi-k3  # measure one model, print JSON details
TMP/opencodable -privacy              # hide contributor models (they train on your prompts)
TMP/opencodable -peak                 # DeepSeek peak-hours pricing
```

Every TSV output starts with a `#` legend block explaining each column; JSON
output (`-json`) has no legend.

### Columns

- `price_out_1M` — retail price per 1M output tokens (models.dev)
- `cost_task` — estimated USD per coding-agent request, using OpenCode's
  official token profile for that model (input + cached-read + output)
- `allowance_usd` — monthly usage budget of this model in your plan
- `reqs_month` — allowance ÷ cost_task ≈ requests/month you get
- `tok_s`, `ttft_s` — Artificial Analysis median output speed / time to first token
- `probe_tok_s`, `probe_ttft_s` — **measured on your subscription**: one streamed
  FizzBuzz request per model. Thinking models get a bigger token budget when
  reasoning would swallow the answer; ultra-fast models get a long-output
  prompt so the timing window is trustworthy. `-sort speed` prefers measured
  values. Costs a few cents total per full run.
- `aa_intelligence`, `aa_coding`, `aa_math`, `livecodebench`, `terminalbench`,
  `ifbench`, `aa_lcr` — benchmark scores via Artificial Analysis
- `swebench_verified` — SWE-bench Verified % resolved (best recorded agent
  run per model); sparse, the official board covers few current models
- `pareto` — `yes` when no other model is simultaneously cheaper-or-equal,
  faster-or-equal and better-or-equal (the "best compromise" shortlist)
- `source` — AA reasoning-effort variant used when the base slug has no speed

The full legend ships with every table as `#` lines above the header row.

Missing data prints as `-`. Free models show `-` prices; `-paid` hides them.

## Design notes

- Go stdlib only (no dependencies) so a future Rust rewrite is mechanical.
- Responses are cached 24h in `~/.cache/opencodable/`.
- Alias table `internal/normalize/normalize.go` maps models.dev IDs to AA
  slugs (e.g. `mimo-v2.5` → `mimo-v2-5-0424`); unmatched models get `-` scores,
  never wrong ones.
- Retail prices are the comparison basis; your subscription bills through the
  allowance system instead — that's why `reqs_month` matters more than
  `price_out_1M` for choosing a daily driver.
- Known AA artifact: TTFT for reasoning models includes thinking time (e.g.
  Claude Haiku 5.5 shows ~247 s median); prefer `tok_s` for throughput and
  treat `ttft_s` as "time to first answer token".
