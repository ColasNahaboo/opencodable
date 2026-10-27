# opencodable — plan

> Historical design notes (2026-10-27). For current behavior, flags and
> columns see [README.md](README.md), [TITLES.md](TITLES.md) and
> [AGENTS.md](AGENTS.md); several details below have changed since.

A Go CLI that answers one question: **given the models available in my OpenCode Go
subscription, what is the best compromise of price, speed and quality, right now?**

Output: plain TSV, one line per model. Nothing else. Designed so a later Rust rewrite is
a mechanical translation (stdlib-only, no framework lock-in).

## Prior art research (done 2026-10-27)

Nobody has built exactly this; existing options are websites, not a CLI that knows about
*your* subscription's model list:

| Site | What it gives | Usable programmatically? |
|---|---|---|
| [Artificial Analysis](https://artificialanalysis.ai/models) | The reference: intelligence index, coding index, tok/s, TTFT, price | Yes — REST API (free key required) |
| [WhatLLM.org](https://whatllm.org) | AA data re-served with a quality index | No (HTML) |
| [BenchLM.ai](https://benchlm.ai/llm-pricing) | ~900 models, price + composite score + score/$ | Partial (MCP/API beta, paid reads) |
| [Vellum leaderboard](https://www.vellum.ai/llm-leaderboard) | price, context, speed | No (HTML) |
| [sanand0/llmpricing](https://sanand0.github.io/llmpricing/) | cost-vs-quality scatter, Pareto concept | No (HTML) |
| OpenRouter `/api/v1/models` | prices per model, JSON, no key | Yes — good fallback price source |
| LiteLLM `model_prices.json` | huge community price/limit file | Yes — fallback |

**Decision:** build `opencodable`. Reuse *data*, not code: pricing comes from the source
OpenCode itself uses, quality/speed come from AA + free benchmark feeds, and speed can
additionally be *measured live* against the subscription endpoint.

## Data sources

### 1. Prices + limits (authoritative, already verified)

`https://models.dev/api.json` → provider `opencode-go` (~40 models). Verified shape:

```json
"glm-5.3": { "name": "GLM-5.3", "reasoning": true,
  "cost": { "input": 1.4, "output": 4.4, "cache_read": 0.26 },
  "limit": { "context": 1000000, "output": 131072 } }
```

`cost` is USD per 1M tokens. Fields to handle: `input`, `output`, `cache_read`,
`cache_write`, optional `tiers` / `context_over_200k` (e.g. Grok 4.7 doubles above 200k
context), `input_audio` (ignored for coding), and free models where everything is `0`.

### 2. Quality — multiple benchmark families

Primary: **Artificial Analysis API** (one call, per-model JSON: intelligence index,
coding index, math, instruction-following…). Free key, `AA_API_KEY` env var.

Secondary free feeds, each one column in the output:

- **LiveBench** — CSV/JSON in their GitHub repo (composite + per-category)
- **Aider polyglot coding leaderboard** — YAML data file in the Aider repo
  (`aider/website/_data/edit_leaderboard.yml` — path to verify on first fetch)
- **SWE-bench Verified** — leaderboard JSON from the swe-bench GitHub org
- **LMArena Elo** — scrape only, optional, lowest priority

Model-name normalization is the real work here: `deepseek-v4.1-flash` (models.dev) vs
"DeepSeek V4.1 Flash 1017" (BenchLM) vs AA's slug. Strategy: lowercase alnum key + a
hand-maintained `internal/normalize/aliases.go` table for the ~40 subscription models,
reviewed once. Unmatched models get `-` in that column, never a wrong score.

### 3. Speed — two modes

- **Reported (default, offline):** AA output tok/s + TTFT from the API.
- **Measured (`--probe`):** send a fixed ~500-token probe prompt through the OpenAI-compatible
  subscription endpoint `https://opencode.ai/inference/go/openai/v1` with `OPENCODE_API_KEY`,
  stream the response, record time-to-first-token and output tok/s from streamed usage.
  This is the most honest number for *this subscription* (routing/throttling can differ
  from AA's providers) but costs a small paid request per model — opt-in only.

### 4. Cost per request — and the subscription twist (allowances)

Raw $/1M output is not enough — reasoning models burn wildly different token counts.
So each row also shows an **estimated cost per coding-agent task** using a canonical
token profile, configurable:

```
default profile: 120k input (80% of it cache-read) + 8k output
cost = in_uncached·p_in + in_cached·p_cache_read + out·p_out
```

Tiered models (context >200k) are priced at base tier by default; `--tier high` switches.
Free models show `$0` and are still useful to rank by quality/speed.

**Subscription-specific columns (from the public Go/Go Plus docs, verified 2026-10-27):**

- **Monthly allowance per model** (Go plan: $15–$240 per model/month, e.g. GLM-5.3-Flash
  $60 vs GLM-5.3 $15). The real "value" of a model in this subscription is
  *quality per included monthly allowance*, not price — add an `allowance` column and a
  `value` sort over (quality, allowance).
- **Official per-request token profiles** — the docs publish estimated tokens per request
  per model (e.g. "Grok 4.7: 390 input, 32,500 cached, 120 output"). Use these as the
  built-in cost-per-request profile instead of one generic profile; they are OpenCode's
  own estimate of how each model is actually consumed. Derived **requests/month per model**
  come for free from allowance ÷ cost-per-request.
- **Peak/off-peak pricing** (DeepSeek V4.1 Flash / V4 Pro double in peak hours) and
  time-tiered context pricing (Grok 4.7 >200k, GPT-6 Luna >272k, Claude Haiku 5.5 >100k,
  Qwen3.7 Plus >256k): price at base tier by default, `--tier high` / `--peak` to switch.
- Each response carries `opencode.allowance` (how much the request counted) — the speed
  probe can read it to report *real* allowance burn, not just tokens.

## Output format (TSV via text/tabwriter)

```
name	price_out_1M	cost_task	tok_s	ttft_s	aa_intelligence	aa_coding	livebench	aider	swebench	pareto
```

One line per model. `-` for missing data. Flags:

```
opencodable                       # default table, sorted by value (quality / cost_task)
opencodable --sort price|speed|quality|value|name
opencodable --provider opencode-go   # any models.dev provider
opencodable --pareto                 # only Pareto-optimal rows (no model better AND cheaper)
opencodable --probe                  # live speed measurement (paid, opt-in)
opencodable --refresh|--offline      # cache control
opencodable --json                   # machine-readable dump instead of TSV
```

The **Pareto column** (from sanand0's idea) is the decision core: the user asked for
"the best compromise" — flag models that are not dominated on (price, speed, quality).

## Go architecture (stdlib only)

```
cmd/opencodable/main.go        # flags, orchestration
internal/modelsdev/            # client + types for api.json (price/limits)
internal/bench/                # one file per source: aa.go, livebench.go, aider.go, swebench.go
internal/normalize/            # alias tables, key-based matching
internal/probe/                # streaming speed probe (SSE parse, TTFT, tok/s)
internal/cost/                 # cost model, token profile, tier handling
internal/render/               # text/tabwriter TSV + optional JSON
internal/cache/                # ~/.cache/opencodable/*.json, 24h TTL, --refresh/--offline
```

Deliberate constraints to keep the Rust port trivial: no external Go dependencies, no
goroutine-heavy logic (at most a simple per-source fetch), plain JSON structs with
explicit field tags, no reflection tricks.

## Milestones

1. **M1 — skeleton + prices (½ day):** fetch models.dev, filter provider, print TSV with
   name / price_out / cost_task / context / reasoning. Already testable against live data.
2. **M2 — quality via AA (1 day):** AA API client, alias normalization, join, quality
   columns + `--sort quality|value`, Pareto flag.
3. **M3 — speed probe (1 day):** SSE streaming probe, `--probe` mode, TTFT + tok/s.
4. **M4 — extra benchmarks (1-2 days):** LiveBench, Aider, SWE-bench feeds; cache
   hardening; README.

## Risks / knowns

- **AA API key**: required for the primary quality source; if unavailable, fall back to
  scraping BenchLM (scorer=$ column) or ship M1+M3 only.
- **Free models** (`*-free`, cost 0) dominate value scores → keep a `--paid` filter idea
  and sort free models by quality/speed instead.
- **Benchmark mismatch**: scores dated differently per source; we print the value, source
  name is implicit per column; a `--verbose` that prints per-source fetch dates is enough.
- **Subscription ≠ retail**: subscription pricing may differ from models.dev's retail
  prices; we surface retail-equivalent prices as the comparison basis and note it in the
  README. The speed probe measures the *real* subscription.
- **Rust rewrite**: nothing in the design assumes Go beyond syntax; the port is
  mechanical (JSON structs + HTTP + one writer).
