# TITLES.md — output column spec

This file is the **contract** for parsing `opencodable` output. Every view's
columns, in order, with their meaning. Scripts can rely on it.

## Stability rules

1. **Column order is fixed.** New columns are always **appended at the end**.
2. Renaming, removing, or reordering a column is a **breaking change** and
   bumps the **major version**.
3. Until the **v1.0.0 tag** is pushed, titles may still change freely without
   notice (pin a commit if you build on it). From v1.0.0 on, rule 2 applies.
4. Values never contain TAB characters. In piped (non-tty) output, every data
   row has exactly one value per column, separated by single TABs; missing
   values are empty strings. Lines starting with `#` are comments or headers.
5. The piped output of every view starts with the `#<viewname>` header row,
   which repeats the column titles below — scripts can also read them
   dynamically from there instead of hardcoding.

## View: main table (`opencodable`)

Piped header: `#name	cost_tok	cost_task	allowance	reqs_mo	tok_s	ttft_s	p_tok_s	p_ttft_s	aa_int	aa_code	aa_math	code_b	term_b	if_b	aa_lcr	swe_b	pareto	src`

| # | Column | Meaning |
|---|---|---|
| 1 | `#name` | model display name (models.dev catalog) |
| 2 | `cost_tok` | retail USD per 1M output tokens |
| 3 | `cost_task` | estimated USD per coding-agent request (OpenCode official token profile, base tier) |
| 4 | `allowance` | monthly usage budget of this model in the plan; `unlimited` for free models |
| 5 | `reqs_mo` | allowance ÷ cost_task ≈ requests per month (theoretical, full reset) |
| 6 | `tok_s` | Artificial Analysis median output tok/s (reported, other hosts) |
| 7 | `ttft_s` | AA median seconds to first token, thinking included |
| 8 | `p_tok_s` | **measured** output tok/s on your subscription (from `-probe`) |
| 9 | `p_ttft_s` | **measured** seconds to first generated token (from `-probe`) |
| 10 | `aa_int` | Artificial Analysis Intelligence Index (composite) |
| 11 | `aa_code` | Artificial Analysis Coding Index |
| 12 | `aa_math` | Artificial Analysis Math Index |
| 13 | `code_b` | LiveCodeBench pass rate % (via AA) |
| 14 | `term_b` | Terminal-Bench pass rate % (via AA) |
| 15 | `if_b` | IFBench instruction-following pass rate % (via AA) |
| 16 | `aa_lcr` | AA Long-Context Reasoning pass rate % |
| 17 | `swe_b` | SWE-bench Verified % resolved, best recorded agent run |
| 18 | `pareto` | `yes` when no other model is cheaper-or-equal, faster-or-equal AND better-or-equal; empty otherwise |
| 19 | `src` | `aa:<effort>` when the speed came from that AA reasoning-effort variant |

On a terminal: same columns, padded for readability, missing values shown as
`-`, preceded by a `#`-prefixed legend block. `-vertical` emits one model per
block of `field: value` lines (same keys, empty fields skipped, blank line
between records).

## View: `-usage` (allowance windows)

Piped header: `#window	status	used_pct	remaining_pct	resets_at	resets_in`

| # | Column | Meaning |
|---|---|---|
| 1 | `#window` | usage window: `5h` rolling = 20% of the plan limit, `week` = 50%, `month` = 100% |
| 2 | `status` | `ok` / `throttled` / `exhausted`, as reported by the endpoint |
| 3 | `used_pct` | percent of the window allowance already consumed |
| 4 | `remaining_pct` | `100 - used_pct` |
| 5 | `resets_at` | local time (RFC3339) when the window resets |
| 6 | `resets_in` | humanized time until reset, e.g. `4h 20m 0s` |

## View: `-now` (what is affordable right now)

Piped header: `#name	cost_task	burn_req	spent_mo	reqs_5h	reqs_wk	reqs_mo	usable	limit	p_tok_s`

| # | Column | Meaning |
|---|---|---|
| 1 | `#name` | model display name (models.dev catalog) |
| 2 | `cost_task` | estimated USD per coding-agent request (official token profile, base tier) |
| 3 | `burn_req` | allowance dollars one request burns (measured ratio × cost_task) |
| 4 | `spent_mo` | real spend this period (console usage API, or `-spent` file), subtracted from the model's allowance |
| 5 | `reqs_5h` | requests affordable with the remaining 5h rolling window budget |
| 6 | `reqs_wk` | requests affordable with the remaining weekly window budget |
| 7 | `reqs_mo` | requests affordable with the remaining monthly window budget |
| 8 | `usable` | min of `reqs_5h` / `reqs_wk` / `reqs_mo` / per-model allowance — what you can spend now |
| 9 | `limit` | which constraint binds: `5h` \| `week` \| `month` \| `allowance` |
| 10 | `p_tok_s` | **measured** output tok/s on your subscription (from `-probe`) |

`-now` assumptions (printed on stderr, never in the piped stream): the usage
endpoint meters one shared pot, assumed equal to the plan price (`-pot`
overrides); window caps are 20% / 50% / 100% of the pot as documented by
OpenCode.

## Versioning

- Pre-release (until the `v1.0.0` tag): no guarantee, titles may change (see
  rule 3).
- From `v1.0.0` on: rules 1–2 apply; bump the major version on any rename,
  removal, or reorder of the columns above.

## JSON output (`-json`, all views)

`-json` works for every view and emits an array of objects whose keys are
**the column titles above, without the `#` prefix** — same names, so one
spec covers both formats. Numbers stay numbers, `pareto` stays boolean, and
`resets_at`/`resets_in` keep their meanings. Two JSON-only extras exist in
the main table view:

- `id` — the models.dev model ID (e.g. `kimi-k3.1-pro`)
- `unlimited` — boolean, true for free models (their `allowance` reads 0)

Empty/missing values: absent numeric fields mean zero (the table's `-` and
the piped empty string both encode "no data" as 0 in JSON); `src` is omitted
when empty.
