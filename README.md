# opencodable

**Find the best compromise of price, speed and quality among the models of your
OpenCode subscription — right now, in your terminal.**

[MIT licensed](LICENSE) · single Go binary · zero dependencies · Go 1.26+

```
name            cost_tok  cost_task  allowance  reqs_mo   p_tok_s  aa_int  swe_b  pareto
Kimi K3.1 Pro   $10       $0.0204    $15        735.3     78.7     55.1    -      yes
Gemini 4 Argon  $10       $0.019     $30        1578.9    85.8     52.6    -      yes
Kimi K3         $15       $0.0306    $15        490.2     799.1    43.6    -      yes
Hy3             $0.58     $0.00279   $60        21506.9   615.8    25.3    -      yes
```

---

## Overview

`opencodable` is a single Go binary (standard library only, zero dependencies)
that joins four data sources — your subscription's catalog, OpenCode's own plan
data, Artificial Analysis benchmarks, and live measurements of *your* endpoints —
into one tab-separated table, one line per model. It ranks models by price,
speed and quality, flags the non-dominated ones (Pareto), and tells you how
many requests per month each model actually buys you under your plan.

## The problem

Choosing a model inside a subscription like OpenCode Go is confusing:

- **Retail prices mislead.** The subscription bills through *per-model monthly
  allowances* (e.g. $15/month for GLM-5.3, $60/month for GLM-5.3-Flash), not
  token prices — so the "cheapest" model is not the one that gives you the most
  usable work.
- **Consumption differs wildly.** Reasoning models burn far more tokens per
  request than flash models; list prices per 1M tokens do not translate into
  requests you can actually make.
- **Benchmark sites compare the wrong set.** Public leaderboards (Artificial
  Analysis, BenchLM, Vellum…) rank the whole market, not the ~40 models your
  subscription exposes, and their speed numbers reflect *their* hosts, not your
  provider's routing.
- **No CLI exists for this.** Everything is a website; nothing outputs plain,
  scriptable text about *your* plan.

## The solution

One command joins everything into a decision table:

1. **Catalog & prices** — `models.dev` `api.json` (the source OpenCode itself
   uses): retail $/1M input/output/cached prices, context limits, reasoning flags.
2. **Subscription reality** — OpenCode's public Go/Go Plus tables, snapshotted
   in `internal/goplan`: per-model monthly allowances, OpenCode's own
   estimated token consumption per coding-agent request, peak-hour price
   overrides, and privacy flags (which models train on your prompts).
3. **Quality & reported speed** — Artificial Analysis data API (free key):
   intelligence/coding/math indexes, LiveCodeBench, Terminal-Bench, IFBench,
   long-context reasoning, plus median output tok/s and time-to-first-token.
4. **Measured speed (opt-in)** — a live probe streams a fixed prompt through
   your actual subscription endpoint and measures tok/s, TTFT and allowance
   burn per model. Measured values take precedence over reported ones.

The join handles name mismatches between sources (alias table: `mimo-v2.5` →
`mimo-v2-5-0424`, etc.) and marks every row with a **Pareto flag**: `yes` when
no other model is simultaneously cheaper-or-equal, faster-or-equal *and*
better-or-equal. That shortlist is the answer to "what is the best compromise?".

## Installation

Requires Go 1.26+.

```sh
git clone https://github.com/ColasNahaboo/opencodable
cd opencodable
go build -o ~/.local/bin/opencodable ./cmd/opencodable
```

## Configuration

No config file. Two optional keys, read from the environment or from
git-ignored dot-files in the working directory:

| Key | Purpose | Required? |
|---|---|---|
| `AA_API_KEY` or `.artificialanalysis.key` | benchmark quality/speed columns | recommended (free at artificialanalysis.ai) |
| `OPENCODE_API_KEY` or `.opencode-go.key` | live speed probe (`-probe`) | only for probing |

Without any key you still get prices, allowances and per-request costs; quality
and speed columns print as `-`.

## Usage

```sh
opencodable                          # all subscription models, ranked by quality
opencodable -pareto -paid            # the non-dominated paid models only
opencodable -sort reqs               # most requests per month first
opencodable -privacy                 # exclude models that train on your prompts
opencodable -plan go-plus            # allowances for the Go Plus plan (default: go)
opencodable -peak                    # DeepSeek peak-hours pricing
opencodable -probe                   # measure live speed of every model (paid, cached 24h)
opencodable -probe-model kimi-k3     # measure one model, full JSON details
opencodable -usage                   # remaining Go allowance: 5h / weekly / monthly windows
opencodable -now                     # requests still affordable per model in each window
opencodable -now -spent pasted.tsv   # same, minus your real per-model spend (console paste)
opencodable -json                    # machine-readable rows, no legend
opencodable -refresh                 # refetch everything, ignore the 24h cache
opencodable -offline                 # cache only, no network
```

All flags: `-provider` (models.dev provider, default `opencode-go`), `-plan`,
`-sort` (`name|price|speed|quality|reqs`), `-pareto`, `-paid`, `-privacy`,
`-peak`, `-probe`, `-probe-model`, `-usage`, `-now`, `-spent`, `-pot`,
`-vertical`, `-json`, `-refresh`, `-offline`.

`-usage` reads the live allowance endpoint and prints, per window (5h rolling
= 20% of the plan's monthly limit, weekly = 50%, monthly = 100%), the percent
used, the percent remaining, and when the window resets — in the same
tty-padded / piped-TABs style as the model table.

### Choosing a model right now: `-now`

The usage endpoint meters **one shared pot** (assumed to be the plan price:
$10 for Go, $40 for Go Plus; override with `-pot`), divided by the documented
window caps. `-now` converts the remaining dollars into *requests per model*:

- `burn_req` — allowance dollars one request of this model burns. The probe
  measures each model's burn **ratio** against list price (e.g. Kimi K3.1 Pro
  burns 4× its list cost, MiniMax M3.1 Flash 3× — the endpoint's
  `opencode.allowance` makes this visible); unprobed models assume ratio 1.
- `reqs_5h` / `reqs_wk` / `reqs_mo` — requests affordable with the
  remaining dollars of each window.
- `usable` / `limit` — the minimum and which window binds.

The console's per-model spend page is **fetchable directly**: the same Go key
authorizes `console.opencode.ai/api/usage/models`, so `-now` subtracts your
real period spend automatically (the `spent_mo` column shows it). A pasted
console table still works as an offline fallback via `-spent file.tsv` — its
`model` and last `$cost` columns are parsed and override the live fetch;
whichever per-model bucket is tighter then competes for the `limit`
verdict (`allowance`).

### Output adapts to stdout

- **Interactive terminal**: a `#`-prefixed legend block explains each column,
  columns are padded into a visually aligned table, missing values show as `-`.
- **Piped or redirected**: no legend; strict machine format — every row is
  values separated by exactly one TAB character, missing fields are empty
  strings, no padding. The `#name…` header row keeps its `#` prefix, so it can
  never be confused with data or with the `# ` legend lines (terminal mode):
```sh
opencodable | grep -v '^#' | awk -F'\t' 'NR>1 {print $1, $6}'
```

`-vertical` renders one model per block of `field: value` lines with blank
lines between records and empty fields skipped — much easier to read with a
screen reader than a wide table, and applies to both the model table and
`-now`:

```sh
$ opencodable -pareto -paid -vertical
name: Kimi K3.1 Pro
cost_tok: $10
cost_task: $0.0204
allowance: $15
reqs_mo: 735.3
p_tok_s: 78.7
aa_int: 55.1
pareto: yes
```

The `-json` flag bypasses TSV entirely and dumps the rows as JSON.

**[TITLES.md](TITLES.md) is the parsing contract**: every view's columns in
order, with meanings; new columns are always appended at the end; renames,
removals or reorders bump the major version.

### Sample probe detail

```json
$ opencodable -probe-model kimi-k3.1-pro
{
  "model_id": "kimi-k3.1-pro",
  "tok_s": 78.67,
  "ttft_s": 6.60,
  "window_s": 5.57,
  "output_tokens": 438,
  "reasoning_tokens": 283,
  "allowance_burned": 0.018352
}
```

## Columns

`-json` keys are exactly the column titles below (minus the `#` on `name`),
plus two JSON-only extras: `id` (models.dev model ID) and `unlimited`
(boolean, free models). See [TITLES.md](TITLES.md) for the full contract.

| Column | Meaning |
|---|---|
| `#name` | model display name (models.dev) |
| `cost_tok` | retail USD per 1M output tokens |
| `cost_task` | estimated USD per coding-agent request (OpenCode's official token profile, base tier) |
| `allowance` | monthly usage budget of this model in your plan; `unlimited` for free models |
| `reqs_mo` | allowance ÷ cost_task ≈ requests per month |
| `tok_s`, `ttft_s` | Artificial Analysis reported output speed / time to first token |
| `p_tok_s`, `p_ttft_s` | **measured on your subscription** via `-probe` |
| `aa_int`, `aa_code`, `aa_math` | Artificial Analysis composite indexes |
| `code_b`, `term_b`, `if_b`, `aa_lcr` | per-benchmark pass rates % via AA: LiveCodeBench, Terminal-Bench, IFBench, Long-Context Reasoning |
| `swe_b` | SWE-bench Verified % resolved, best recorded agent run |
| `pareto` | `yes` = not dominated on (cost, speed, quality) |
| `src` | `aa:<effort>` when speed came from that AA reasoning-effort variant |

Missing data prints as `-` (terminal) or empty (piped) — never a wrong score.

## Data sources

| Source | Provides | Auth |
|---|---|---|
| [models.dev](https://models.dev) | prices, context limits, reasoning flags | none |
| [OpenCode Go docs](https://opencode.ai/v2/docs/console/go/) | allowances, token profiles, peak prices, privacy | none |
| [Artificial Analysis](https://artificialanalysis.ai) | benchmark indexes, reported speed | free key |
| [SWE-bench Verified](https://www.swebench.com/) leaderboard JSON | agentic repo-fixing % resolved | none |
| [Console usage API](https://console.opencode.ai/api/usage) (`/usage/models`) | per-model spend for the current period | same Go key |
| Your subscription endpoint (probe) | real tok/s, TTFT, allowance burn | `OPENCODE_API_KEY` |

Dropped with cause: LiveBench (data files not machine-accessible), Aider
polyglot (public YAML files stale since 2025-10).

## What live probing tends to reveal

Observations from full runs on the OpenCode Go plan (first: October 2026 —
expect the specific numbers to drift; the patterns below are stable):

- **Allowance ≠ value.** High-quality flagship models often carry the thinnest
  monthly allowances (e.g. $15), while mid-tier models buy orders of magnitude
  more requests — `reqs_mo` flips the usual leaderboard ordering.
- **Your routes can beat public numbers.** Subscription endpoints frequently
  measure *faster* than aggregator averages (reserved capacity), but routing
  varies hour to hour and per model — sometimes 2-3×. Site-reported speeds are
  a starting point; `-probe` is the ground truth for your setup.
- **Thinking costs latency.** Heavy-reasoning models can take tens of seconds
  before their first content token even while streaming fast afterwards.
  Judge responsiveness by `p_ttft_s`, throughput by `p_tok_s`.
- **Privacy has a price.** Contributor models are an order of magnitude
  cheaper per request in exchange for training on your prompts; `-privacy`
  filters them out when that matters.
- **Free models are real quality.** Time-limited free tiers on the plan
  sometimes benchmark in the same range as paid mid-tier models — worth
  probing before paying attention to price at all.

## Design notes

- **Standard library only.** No Go dependencies, no frameworks — a future Rust
  rewrite is a mechanical translation.
- **Caching.** All fetches are cached 24h in `~/.cache/opencodable/`;
  `-refresh` forces refetch, `-offline` forbids network. Probe failures are
  retried on the next run rather than served stale for 24h.
- **Probe robustness.** Thinking models stream `reasoning_content` first and
  can exhaust a small token budget before answering, so the probe escalates the
  budget on failure. Ultra-fast models finish inside a window too short to
  divide by, so the probe retries with a long-output prompt and only trusts
  tok/s from windows ≥ 1 s.
- **Known artifact.** AA's `ttft_s` for reasoning models includes thinking time
  (e.g. Claude Haiku 5.5 ~247 s median) — treat it as time to first *answer* token.
- **Prices are the comparison basis.** Your subscription bills allowances, not
  retail; that is why `reqs_mo` matters more than `cost_tok` for
  choosing a daily driver.

## Roadmap

- `-privacy` awareness in Pareto scoring (currently a pre-filter)
- Peak/off-peak *scheduling advice* (when DeepSeek is cheapest)
- Optional LMArena Elo column
- Multi-provider comparison (`-provider any`)
- Rust port of the same pipeline

## Contributing

The interesting places to contribute: `internal/normalize/aliases.go` (name
matching), `internal/goplan/goplan.go` (subscription snapshot — update when
OpenCode changes plans), `internal/bench/` (new benchmark feeds).

## License

[MIT](LICENSE) © Colas Nahaboo
