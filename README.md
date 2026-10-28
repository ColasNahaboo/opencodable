# opencodable

![banner](.github/assets/banner.png)

**Find the best compromise of price, speed and quality among the models of your
OpenCode subscription — right now, in your terminal.**

[MIT licensed](LICENSE) · [releases](https://github.com/ColasNahaboo/opencodable/releases) · single Go binary · zero dependencies · Go 1.26+

---

## Overview

`opencodable` is a single Go binary (standard library only, zero dependencies)
that joins four data sources — your subscription's catalog, OpenCode's own plan
data, Artificial Analysis benchmarks, and live measurements of *your* endpoints.

It outputs a simple text table on the terminal, but is designed to be scripted either by its JSON output or unix tab-separated lines if its output is piped into a program. Basically it gathers data so that you can present it as you like.

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
5. **Live allowance state** — the Go usage endpoint (5h/week/month windows)
   and the console usage API (per-model period spend, same key) make `-usage`
   and `-now` answer "what can I use right now and how much of it".

The join handles name mismatches between sources (alias table: `mimo-v2.5` →
`mimo-v2-5-0424`, etc.) and marks every row with a **Pareto flag**: `yes` when
no other model is simultaneously cheaper-or-equal, faster-or-equal *and*
better-or-equal. That shortlist is the answer to "what is the best compromise?".

## Installation

Requires Go 1.26+.

```sh
go install github.com/ColasNahaboo/opencodable/cmd/opencodable@latest
```

Or build from a clone:

```sh
git clone https://github.com/ColasNahaboo/opencodable
cd opencodable
go build -o ~/.local/bin/opencodable ./cmd/opencodable
```

Prebuilt archives for linux/darwin/windows on amd64/arm64 are attached to
GitHub releases; `scripts/build-release.sh [version]` rebuilds them locally
and writes checksums.

## Configuration

Two optional keys, each resolved in order: **environment variable →
`opencodable.conf` → git-ignored dot-file in the working directory**:

| Key | Purpose | Required? |
|---|---|---|
| `AA_API_KEY` or `.artificialanalysis.key` | benchmark quality/speed columns | recommended (free at artificialanalysis.ai) |
| `OPENCODE_API_KEY` or `.opencode-go.key` | live speed probe (`-probe`) | only for probing |

`opencodable.conf` is a simple `key=value` file (one setting per line, `#`
comments; keys mirror the environment variables) in the per-user config
directory:

| OS | Path |
|---|---|
| Linux | `$XDG_CONFIG_HOME/opencodable/opencodable.conf` (default `~/.config/...`) |
| macOS | `~/Library/Application Support/opencodable/opencodable.conf` |
| Windows | `%AppData%\opencodable\opencodable.conf` |

```ini
# ~/.config/opencodable/opencodable.conf
OPENCODE_API_KEY=your-key
AA_API_KEY=your-key
```

`-doctor` prints where each key was found (`env`, `config`, `dotfile`,
`none`) and warns about unknown keys in the file (typos). Without any key you
still get prices, allowances and per-request costs; quality and speed columns
print as `-`.

## Usage

```sh
opencodable                          # all subscription models, ranked by quality
opencodable -pareto -paid            # the non-dominated paid models only
opencodable -sort reqs               # most requests per month first
opencodable -privacy                 # exclude models that train on your prompts, or whose status is unknown
opencodable -plan go-plus            # allowances for the Go Plus plan (default: go)
opencodable -peak                    # DeepSeek peak-hours pricing
opencodable -probe                   # measure live speed of every model (paid, cached 24h)
opencodable -probe -probe-max-allowance 1  # stop after $1 of allowance burn
opencodable -probe-model kimi-k3     # measure one model, full JSON details
opencodable -usage                   # remaining Go allowance: 5h / weekly / monthly windows
opencodable -now                     # requests still affordable per model, per window (spend auto-fetched)
opencodable -now -window-estimate    # also scale 5h/week caps by account-wide usage
opencodable -spent pasted.tsv -now   # offline fallback: subtract spend from a pasted console table
opencodable -vertical                # one model per 'field: value' block (screen readers)
opencodable -json                    # machine-readable output for every view
opencodable -doctor                  # audit plan/AA/SWE/probe coverage
opencodable -refresh                 # refetch everything, ignore the 24h cache
opencodable -offline                 # cache only for catalog/benchmarks/probes
```

### Options

Every flag is optional; the bare command needs none of them.

**What to rank**

- `-provider <name>` — which models.dev provider to rank. Default
  `opencode-go` (your subscription); other values are for experimenting with
  public catalogs, e.g. `opencodable -provider openrouter`. Only
  `opencode-go` has allowance/plan data.
- `-plan go|go-plus` — which subscription tier's per-model monthly limits to
  use. Go Plus gives every paid model a larger limit, so `reqs_mo` and the
  `-now` counts grow accordingly. Default `go`.
- `-sort name|price|speed|quality|reqs` — column to rank rows by:
  alphabetical, cheapest request first, fastest first, smartest first
  (Artificial Analysis intelligence index), or most requests per month first.
  Default `quality`. Example: `opencodable -sort price` to find the budget
  option.
- `-pareto` — keep only the models that are not dominated: no other model is
  simultaneously cheaper, faster and better. What remains is the honest
  "best compromise" shortlist. Combine with `-paid` to ignore free models.
- `-paid` — hide the free models (those costing $0 per request). Useful when
  you want to decide among the models that actually consume your allowance.
- `-privacy` — hide every model that trains on your prompts, plus any whose
  privacy status the plan snapshot does not confirm (fail-closed). Example:
  `opencodable -pareto -paid -privacy` for the best privacy-safe paid pick.
- `-peak` — price DeepSeek models at their peak-hours rates instead of the
  off-peak ones, so cost columns reflect what you pay if you only code during
  business hours.

**Measuring live speed (costs a few cents)**

- `-probe` — send one small paid request to every model through your
  subscription and measure real tokens/s and time-to-first-token on your
  account (the `p_tok_s` / `p_ttft_s` columns). Results are cached 24h, so a
  daily run is cheap. Progress prints on stderr; the table gains the measured
  columns.
- `-probe-max-allowance <usd>` — with `-probe`, stop after burning this many
  allowance dollars in total: `opencodable -probe -probe-max-allowance 0.5`
  caps the whole run at 50¢.
- `-probe-model <id>` — probe a single model and print its full JSON
  measurement (window length, token counts, burn):
  `opencodable -probe-model kimi-k3`. Use it to re-measure one suspicious
  result; add `-refresh` to ignore the cache.

**Your allowance**

- `-usage` — asks OpenCode how much of your allowance is left and prints the
  three windows (5h rolling, weekly, monthly): percent used, percent
  remaining, reset time. Needs network and a key; it is one free API call.
- `-now` — the "what can I use right now" table: per model, how many requests
  still fit in each window given your real spend this period (fetched from
  the console usage API automatically). See
  [Choosing a model right now](#choosing-a-model-right-now--now) below.
- `-window-estimate` — with `-now`, scale each model's 5h/weekly caps by the
  account-wide remaining percentages from `-usage`, instead of assuming fresh
  windows. An explicit approximation (the API has no per-model window
  breakdown), clearly labelled on stderr.
- `-spent <file>` — offline fallback for `-now`: subtract per-model spend
  read from a pasted console "usage by model" table instead of calling the
  API. Note: a pasted file carries no request counts, so measured `burn_req`
  never kicks in and the official estimate is used.

**Output shape**

- `-vertical` — one model per block of `field: value` lines instead of a wide
  table; much easier for screen readers and for `grep`. Applies to the main
  table and `-now`.
- `-json` — machine-readable output for every view, keys equal to the column
  titles (see [TITLES.md](TITLES.md)). Example:
  `opencodable -json | jq '.[] | select(.pareto)'`.
- Output also adapts automatically: on a terminal you get a padded table with
  a `#` legend; piped or redirected, strict TAB-separated values with no
  legend — see [Output adapts to stdout](#output-adapts-to-stdout).

**Data freshness**

- `-refresh` — ignore the 24h cache and refetch catalog, benchmarks and
  probe results now (prices and scores move; usually unnecessary since each
  run reuses the cache).
- `-offline` — never touch the network for catalog/benchmarks/probes; use
  whatever the cache holds, however old. Handy on a plane or to re-render
  cached data. `-usage` is inherently live and rejects `-offline`.

**Diagnosis**

- `-doctor` — coverage audit: which models lack a plan entry, an Artificial
  Analysis match or a SWE-bench score, how fresh each cache is, where your
  API keys were found, and warnings for unknown keys in `opencodable.conf`.
  Runs no probes; safe to run anytime. Experimental: its columns are not yet
  part of the TITLES.md contract.

### Choosing a model right now: `-now`

Go usage limits are **per model**: each model has its own monthly dollar
limit, and its 5h and weekly caps are 20% and 50% of that limit. There is no
shared pot and no `-pot` flag. `-now` turns those caps into requests:

- `cost_task` — official allowance cost per request (OpenCode token profile).
- `burn_req` — allowance dollars one request actually burns: measured from
  your console token counts when the model has at least 50 requests this
  period, otherwise the official `cost_task`. Real coding-agent requests are
  often larger than the official profile (DeepSeek V4.1 Flash measured about
  5× in our sample), so this can differ substantially.
- `spent_mo` — allowance dollars spent this period, computed from console
  token counts at Go prices, not from the console's list-cost total.
  Subtracted from the model's monthly limit.
- `reqs_5h` / `reqs_wk` — requests in the model's 5h / weekly cap. These are
  fresh-window upper bounds: the API does not expose per-model window spend.
  With `-window-estimate`, they are scaled by the account-wide remaining
  percentages from the usage endpoint (an explicit estimate, not per-model).
- `reqs_mo` — requests left in the monthly limit after `spent_mo`.
- `usable` / `limit` — the minimum and which constraint binds: `5h`, `week`,
  `month`, `unlimited`, or `unknown`.

The console API is **filtered to `opencode-go` rows**; pay-as-you-go rows are
ignored. The same Go key authorizes `console.opencode.ai/api/usage/models`, so
spend is fetched automatically. A pasted console table via `-spent file.tsv`
is the offline fallback; it carries no request counts, so with `-spent` the
measured burn never kicks in and `burn_req` stays at the official `cost_task`.
Free models (zero prices) show `limit=unlimited` with
empty counts, even when the snapshot lacks their entry; paid models missing
from the plan snapshot show `limit=unknown` rather than a fake zero.

### Coverage audit: `-doctor`

`-doctor` joins the same sources and prints one row per model: plan status
(`yes` / `free` / `missing`), AA slug, SWE-bench score, privacy verdict,
cached probe status, and warnings. It also prints the plan snapshot date,
cache ages, key presence, and coverage counts (on stderr). It never runs a
probe; `-json` returns the same report as an object. `-doctor` is
experimental: its columns are not yet part of the TITLES.md contract, but it
follows the same conventions — `# `-prefixed legend lines and a `#id…` header.

### Output adapts to stdout

- **Interactive terminal**: a `#`-prefixed legend block explains each column,
  columns are padded into a visually aligned table, missing values show as `-`.
- **Piped or redirected**: no legend; strict machine format — every row is
  values separated by exactly one TAB character, missing fields are empty
  strings, no padding. The `#name…` header row keeps its `#` prefix, so it can
  never be confused with data or with the `# ` legend lines (terminal mode):
```sh
opencodable | grep -v '^#' | awk -F'\t' '{print $1, $6}'
```

`-vertical` renders one model per block of `field: value` lines with blank
lines between records and empty fields skipped — much easier to read with a
screen reader than a wide table, and applies to both the model table and
`-now`. No prose legend block is printed: the field names are the legend.

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

### Sample output

Main view (`opencodable -pareto -paid -privacy`, columns excerpt — 3 first lines):

```sh
#name           cost_tok  cost_task  allowance  reqs_mo   p_tok_s  aa_int  swe_b  pareto
Kimi K3.1 Pro   $10       $0.0204    $15        735.3     78.7     55.1    -      yes
Gemini 4 Argon  $10       $0.019     $30        1578.9    85.8     52.6    -      yes
Grok 4.7        $6        $0.01775   $15        845.1     172.3    46.4    -      yes
```

`-now -paid -privacy` (3 first lines):

```sh
#name                 cost_task  burn_req  spent_mo    reqs_5h  reqs_wk  reqs_mo  usable  limit  p_tok_s
MiMo-V2.6-Flash       $0.0004    $0.0006   $0.11189    19841    49602    99020    19841   5h     59.5
MiMo V2.5             $0.0004    $0.00096  $0.15188    12562    31406    62653    12562   5h     89.0
LongCat-2.0           $0.00105   $0.00105  $0.00077    11435    28587    57174    11435   5h     76.9
```

`-usage` (all three windows):

```sh
#window  status  used_pct  remaining_pct  resets_at                resets_in
5h       ok      19        81             2026-10-27T21:43:22+01:00  53m 0s
week     ok      16        84             2026-11-02T01:00:00+01:00  124h 9m 0s
month    ok      21        79             2026-11-07T14:56:38+01:00  258h 6m 0s
```

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
plus JSON-only extras: `id` (models.dev model ID) and `unlimited` (boolean,
free models). `-now` also carries `known`, `spent_known` and
`window_estimate` so its zero/scaled counts can be interpreted. See
[TITLES.md](TITLES.md) for the full contract.

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
| `pareto` | `yes` = not dominated on (cost, speed, quality) after `-paid`/`-privacy` filtering |
| `src` | `aa:<effort>` when speed came from that AA reasoning-effort variant |
| `privacy` | `safe`, `trains`, or `unknown` (no plan-snapshot verdict) |
| `ctx_k`, `out_k` | context window / maximum output in thousands of tokens (models.dev) |

Missing data prints as `-` (terminal) or empty (piped) — never a wrong score.

## Data sources

| Source | Provides | Auth |
|---|---|---|
| [models.dev](https://models.dev) | prices, context limits, reasoning flags | none |
| [OpenCode Go docs](https://opencode.ai/v2/docs/console/go/) | allowances, token profiles, peak prices, privacy | none |
| [Artificial Analysis](https://artificialanalysis.ai) | benchmark indexes, reported speed | free key |
| [SWE-bench Verified](https://www.swebench.com/) leaderboard JSON | agentic repo-fixing % resolved | none |
| [Console usage API](https://console.opencode.ai/api/usage) (`/usage/models`) | per-model tokens and spend for the current period; Go allowance spend recomputed from tokens | same Go key |
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
  hides both training models and models whose privacy status is unknown.
- **Free models are real quality.** Time-limited free tiers on the plan
  sometimes benchmark in the same range as paid mid-tier models — worth
  probing before paying attention to price at all.

## Design notes

- **Standard library only.** No Go dependencies, no frameworks — a future Rust
  rewrite is a mechanical translation.
- **Caching.** Catalog, benchmark and probe fetches are cached 24h in
  `~/.cache/opencodable/`; `-refresh` forces refetch, `-offline` forbids
  network for those. `-usage` is live and still needs network; `-now -offline`
  uses `-spent` when given and otherwise assumes zero spend. Probe failures
  are retried on the next run rather than served stale for 24h.
- **Probe robustness.** Thinking models stream `reasoning_content` first and
  can exhaust a small token budget before answering, so the probe escalates the
  budget on failure. Ultra-fast models finish inside a window too short to
  divide by, so the probe retries with a long-output prompt and only trusts
  tok/s from windows ≥ 1 s. A short window (< 2 s) above 500 tok/s, or any rate
  above 1000 tok/s, is flagged suspect and kept out of ranking/`-now`; the raw
  probe JSON still shows it for inspection. Probes send a unique
  `x-opencode-session` per run and an `opencodable/<version>` user agent;
  `-probe-max-allowance` caps the total allowance a run may burn.
- **Allowance math.** Limits are per model: 5h = 20%, week = 50%, month = 100%
  of that model's monthly dollar limit. `-now` uses actual console token counts
  at Go prices when a model has enough traffic, otherwise the official profile.
  The usage endpoint's percentages are aggregate; `-window-estimate` optionally
  applies them as a uniform scaling factor to every model's 5h/weekly caps.
- **Known artifact.** AA's `ttft_s` for reasoning models includes thinking time
  (e.g. Claude Haiku 5.5 ~247 s median) — treat it as time to first *answer* token.
- **Allowances are the comparison basis.** Your subscription bills per-model
  allowances, not retail; that is why `reqs_mo` matters more than `cost_tok`
  for choosing a daily driver, and why Pareto flags are computed after the
  `-paid`/`-privacy` filters.

## Roadmap

- True per-model 5h/weekly spend (the API exposes only aggregate percentages;
  `-window-estimate` is a uniform approximation)
- Peak/off-peak *scheduling advice* (when DeepSeek is cheapest)
- Optional LMArena Elo column
- Multi-provider comparison (`-provider any`)
- Rust port of the same pipeline

## Contributing

The interesting places to contribute: `internal/normalize/normalize.go` (name
matching and the alias table), `internal/goplan/goplan.go` (subscription
snapshot — update when OpenCode changes plans), `internal/bench/` (new
benchmark feeds). Any output change must respect [TITLES.md](TITLES.md).

## License

[MIT](LICENSE) © Colas Nahaboo

## AI Co-Design Disclosure

![Co-Designed with AI](https://img.shields.io/badge/Co--Designed%20with-AI-blueviolet?style=flat-square&logo=openai)

This repository is developed by me, a human hobbyist in my personal time in close collaboration with various AI programming assistants. System architecture, logic design, and final reviews are handled by me, while code scaffolding, repetitive tasks, and standard styling are co-piloted by AI. AI contributions are commited under their model name, and can be quite verbose for tracability.

## History

- v0.1.3 2026-10-27 API keys also read from `opencodable.conf`
  (`key=value`) in the per-user config directory; `-doctor` shows each key's
  source and warns on unknown config keys.
- v0.1.2 2026-10-27 `-doctor` follows the output conventions (`# ` legend,
  `#id…` header, strict TABs when piped; still experimental, not in
  TITLES.md), tag-triggered GitHub releases, single-source probe trust
  thresholds, `cache.Peek`, shared number formatting, clearer spend warnings.
- v0.1.1 2026-10-27 `-vertical` no longer prints the prose legend (the field
  names are the legend), better for screen readers. TITLES.md still changes
  until v1.0.0.
- v0.1.0 2026-10-27 Canonical module path, privacy tri-state with fail-closed
  `-privacy`, context/output columns, `-doctor`, `-window-estimate`,
  cache/parser hardening, CI, cross-platform release builds. TITLES.md still
  changes until v1.0.0.
- v0.0.2 2026-10-27 Per-model `-now` limits (no shared pot), allowance spend
  from console token counts, Pareto after filters, deterministic ordering,
  probe sanity checks. TITLES.md still changes until v1.0.0.
- v0.0.1 2026-10-27 Data gathering, no effort (yet?) on UI or synthetic advice.
  Still in alpha test to gather feedback. TITLES.md may change until v1.0.0.
