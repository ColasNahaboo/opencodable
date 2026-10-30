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
5. The piped output of every view starts with the `#name` header row,
   which repeats the column titles below — scripts can thus also read them
   dynamically from there instead of hardcoding column names.

## View: main table (`opencodable`, `-main`)

Piped header: `#name	cost_tok	cost_task	allowance	r_quota	tok_s	ttft_s	p_tok_s	p_ttft_s	aa_int	aa_code	aa_math	code_b	term_b	if_b	aa_lcr	swe_b	pareto	src	privacy	ctx_k	out_k	lb_agent	lb_code	elo	btr`

| # | Column | Meaning |
|---|---|---|
| 1 | `#name` | model display name (models.dev catalog) |
| 2 | `cost_tok` | retail USD per 1M output tokens |
| 3 | `cost_task` | estimated USD per coding-agent request (OpenCode official token profile, base tier) |
| 4 | `allowance` | monthly usage budget of this model in the plan; `unlimited` for free models |
| 5 | `r_quota` | allowance ÷ cost_task ≈ requests per month (theoretical, full reset) |
| 6 | `tok_s` | Artificial Analysis median output tok/s (V2 `performance`, reported, other hosts) |
| 7 | `ttft_s` | AA median seconds to first token, thinking included (V2 `performance`) |
| 8 | `p_tok_s` | **measured** output tok/s on your subscription (from `-probe`; too-short or suspect measurements are omitted) |
| 9 | `p_ttft_s` | **measured** seconds to first content token (from `-probe`; thinking time is included in the wait) |
| 10 | `aa_int` | Artificial Analysis Intelligence Index (composite; Free and Pro) |
| 11 | `aa_code` | AA Coding Index (no longer published; always empty) |
| 12 | `aa_math` | AA Math Index (no longer published; always empty) |
| 13 | `code_b` | LiveCodeBench pass rate % (no longer published; always empty) |
| 14 | `term_b` | Terminal-Bench pass rate % (Pro key with `-aa-pro`) |
| 15 | `if_b` | IFBench instruction-following pass rate % (Pro key with `-aa-pro`) |
| 16 | `aa_lcr` | AA Long-Context Reasoning pass rate % (Pro key with `-aa-pro`) |
| 17 | `swe_b` | SWE-bench Verified % resolved, best recorded agent run |
| 18 | `pareto` | `yes` when no other visible model is cheaper-or-equal, faster-or-equal AND better-or-equal; empty otherwise. Flags are computed after every row filter (`-exclude`, `-include`, `-max-cost`, `-min-speed`, `-min-quality`, `-paid`, `-privacy`), so a hidden model never dominates a visible one. |
| 19 | `src` | `aa:<effort>` when the speed came from that AA reasoning-effort variant |
| 20 | `privacy` | `safe` (no training on request data), `trains` (contributor), or `unknown` (no plan-snapshot verdict) |
| 21 | `ctx_k` | context window in thousands of tokens (models.dev) |
| 22 | `out_k` | maximum output tokens in thousands (models.dev) |
| 23 | `lb_agent` | LiveBench 2.0 Long Agentic score, 0–100 (single run) |
| 24 | `lb_code` | LiveBench 2.0 Coding score, 0–100 (single run) |
| 25 | `elo` | Arena Elo, daily snapshot of the arena.ai text leaderboard (third-party mirror; integer, missing = not in snapshot) |
| 26 | `btr` | Arena Bradley–Terry rating (log-odds scale, ~±0.2; 0 is average, >0 above average), text with style control, overall category (official mirror, fresh; missing = no match) |

On a terminal: same columns, padded for readability, missing values shown as
`-`. Models with peak-hours pricing add a `Name [peak]` twin row (ID
suffixed `-peak`) with `cost_task`/`r_quota` recomputed at peak rates. The `#`-prefixed legend block explaining each column is printed only
with `-l` (in any output mode). `-vertical` emits one model per block of
`field: value` lines (same keys, empty fields skipped, blank line between
records); the field names are the legend, and the prose block again needs
`-l`.

## View: `-usage` (allowance windows)

Piped header: `#window	status	used_pct	remaining_pct	resets_at	resets_in	remaining_usd`

| # | Column | Meaning |
|---|---|---|
| 1 | `#window` | usage window: `5h` = 20%, `week` = 50%, `month` = 100% of each model's monthly limit (the endpoint reports one aggregate set) |
| 2 | `status` | `ok` / `throttled` / `exhausted`, as reported by the endpoint |
| 3 | `used_pct` | percent of the window allowance already consumed |
| 4 | `remaining_pct` | `100 - used_pct` |
| 5 | `resets_at` | local time (RFC3339) when the window resets |
| 6 | `resets_in` | humanized time until reset, e.g. `4h 20m 0s` |
| 7 | `remaining_usd` | estimated dollars left in this window before it resets: remaining percent × summed window caps (20%/50%/100% of each capped model's monthly limit). Account-wide estimate — the endpoint reports one aggregate set, not per-model window spend; 0 when plan budgets are unavailable |

## View: `-now` (what is affordable right now)

Piped header: `#name	cost_task	burn_req	spent_mo	reqs_5h	reqs_wk	reqs_mo	usable	limit	p_tok_s`

| # | Column | Meaning |
|---|---|---|
| 1 | `#name` | model display name (models.dev catalog) |
| 2 | `cost_task` | official allowance cost per coding-agent request (OpenCode token profile, base tier) |
| 3 | `burn_req` | allowance dollars one request burns: measured from console tokens when the model has at least 50 requests this period, otherwise `cost_task`; `[peak]` twins scale the measured burn by the official peak/base price ratio |
| 4 | `spent_mo` | allowance dollars spent this period (console tokens priced at Go rates, or `-spent`), subtracted from the model's monthly limit; blank when spend is unknown |
| 5 | `reqs_5h` | requests in the model's 5h cap (20% of its monthly limit) after estimated window spend |
| 6 | `reqs_wk` | requests in the model's weekly cap (50% of its monthly limit) after estimated window spend |
| 7 | `reqs_mo` | requests left in the model's monthly limit after `spent_mo` |
| 8 | `usable` | min of `reqs_5h` / `reqs_wk` / `reqs_mo` — what you can use now |
| 9 | `limit` | binding constraint: `5h` \| `week` \| `month` \| `unlimited` \| `unknown` |
| 10 | `p_tok_s` | **measured** output tok/s on your subscription (from `-probe`) |

`-now` assumptions (printed on stderr, never in the piped stream): Go usage
limits are **per model**. Each model's monthly limit is the dollar allowance
in `internal/goplan`; its 5h and weekly caps are 20% and 50% of that limit.
There is no shared pot. The usage endpoint reports one aggregate set of
percentages. `reqs_5h` and `reqs_wk` assume fresh windows unless
`-window-estimate` attributes spend: ledger-recorded dollars first
(console deltas between runs, fresh probe burns), the implied remainder
split monthly-proportionally (`full`/`proportional`/`ledger` in
`window_method`). The console API serves period totals only, so window
spend is estimated, never measured.
Free models (zero
prices) show `limit=unlimited` with empty counts, even when the snapshot lacks
their entry; paid models missing from the plan snapshot show `limit=unknown`.
Spend uses only `opencode-go` console rows and prices their token counts at Go
rates; `-spent` is the offline fallback. Peak-priced models add a `[peak]`
twin row sharing the base's spend dollars, with `burn_req` and the request
counts at peak prices (a measured average is scaled by the official peak/base
ratio; the console does not tag tokens by regime).

## View: `-full` (static quality plus affordable requests)

Piped header: `#name	cost_tok	cost_task	allowance	r_quota	tok_s	ttft_s	p_tok_s	p_ttft_s	aa_int	aa_code	aa_math	code_b	term_b	if_b	aa_lcr	swe_b	pareto	src	privacy	ctx_k	out_k	lb_agent	lb_code	elo	btr	burn_req	spent_mo	reqs_5h	reqs_wk	reqs_mo	usable	limit`

Column order is the main order first, then the `-now`-only columns in `-now`
order. Shared titles (`name`, `cost_task`, `p_tok_s`) carry the same value in
both views and appear once. Titles are unique across views: the theoretical
monthly count is `r_quota` (main), the remaining count is `reqs_mo` (`-now`).

| # | Column | Meaning |
|---|---|---|
| 1–24 | main columns, in main order | identical values and meanings to the main table above |
| 25 | `burn_req` | as in `-now` |
| 26 | `spent_mo` | as in `-now` |
| 27 | `reqs_5h` | as in `-now` |
| 28 | `reqs_wk` | as in `-now` |
| 29 | `reqs_mo` | as in `-now`: requests left in the monthly limit after `spent_mo` |
| 30 | `usable` | as in `-now` |
| 31 | `limit` | as in `-now` |

`-full` is live like `-now` (console spend, aggregate windows) and loads AA
scores like the main table, so every row filter applies — including
`-min-quality` — and speed filtering uses the effective speed (measured probe
when available, else AA). Rows without `-now` data (no price to turn dollars
into requests) keep their static columns with blank affordability cells and
`limit=unknown`. JSON extras: `id`, `unlimited` (as in main) plus `known`,
`spent_known`, `window_estimate` (as in `-now`). `[peak]` twins join like
any other row, carrying their own peak-priced affordability.

## Column selection (`-cols`, `-excols`; model views only)

`-cols a,b,c` shows only those columns; `-excols e,f` shows all but those.
Both take comma-separated titles, both are repeatable (values union), both
are config-file defaults refined per run (`-cols` union plus `-excols` union,
then subtract). `name` is always shown. A name unknown in **every** view is an
error; a name known only in another view is silently ignored, so one global
set works for main, `-now` and `-full` alike (e.g. `-cols name,burn_req`
renders just `name` in the main table). Selection applies after sorting,
Pareto flags and filters; `-l` documents only visible columns; `-json` emits
only visible columns plus the view's JSON-only extras. `-usage` and `-doctor`
ignore the selection (unknown names still fail fast).

## Header (`-nohead`; every view)

`-nohead` drops the header row, leaving data rows only; scripts that already
know the column positions (or read them from an earlier headed run) can skip
parsing it. The `-l` legend still prints. No-op in `-vertical` and `-json`,
which have no header row.

## Versioning

- Pre-release (until the `v1.0.0` tag): no guarantee, titles may change (see
  rule 3).
- From `v1.0.0` on: rules 1–2 apply; bump the major version on any rename,
  removal, or reorder of the columns above.

## JSON output (`-json`, all views)

`-json` works for every view and emits an array of objects whose keys are
**the column titles above, without the `#` prefix** — same names, so one
spec covers both formats. Numbers stay numbers, `pareto` stays boolean, and
`resets_at`/`resets_in` keep their meanings. JSON-only extras:

- main table: `id` (models.dev model ID, e.g. `kimi-k3.1-pro`) and
  `unlimited` (true for free models; their `allowance` reads 0).
- `-now`: `id`, `unlimited`, `known` (false when the plan snapshot has no
  entry), `spent_known` (false when spend was assumed to be zero),
  `window_estimate` (true when window spend was estimated) and
  `window_method` (`full` fresh caps, `proportional` monthly split, `ledger`
  with local attribution).
  For free/unknown rows the request counts are 0; use `limit` plus these
  booleans, not the zero, to interpret them.
- `-full`: the main extras plus the `-now` extras (including
  `window_method`); `r_quota` is the
  theoretical full-reset count and `reqs_mo` the remaining count.

Empty/missing values: absent numeric fields mean zero (the table's `-` and
the piped empty string both encode "no data" as 0 in JSON); `src` is omitted
when empty.
