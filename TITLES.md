# TITLES.md — output column spec

This file is the **contract** for parsing `opencodable` output. Every view's
columns, in order, with their meaning. Scripts can rely on it.

All `$` figures are **OpenCode Go credits**: the plan's internal allowance
unit, priced at Go rates (your subscription buys a fixed credit budget; the
real money behind it never enters any computation).

## Stability rules

1. **Column order is fixed.** New columns are always **appended at the end**.
2. Renaming, removing, or reordering a column is a **breaking change** and
   bumps the **major version**.
3. Since the **v1.0.0 tag** the contract is **frozen**: titles never change,
   and rule 2 applies in full. (Before v1.0.0, titles could change freely;
   pin a pre-v1.0.0 commit if you built on them.)
4. Values never contain TAB characters. In piped (non-tty) output, every data
   row has exactly one value per column, separated by single TABs; missing
   values are empty strings. Lines starting with `#` are comments or headers.
5. The piped output of every column view starts with a `#`-prefixed header
   row (`#name` for the model tables, `#window` for `-usage`, `#id` for
   `-doctor`), which repeats the column titles below — scripts can thus also
   read them dynamically from there instead of hardcoding column names. `-news`
   is a line log, not a column view (see its section).

## View: main table (`opencodable`, `-main`)

Piped header: `#name	cost_tok	cost_task	allowance	r_quota	tok_s	ttft_s	p_tok_s	p_ttft_s	aa_int	aa_code	aa_math	code_b	term_b	if_b	aa_lcr	swe_b	pareto	src	privacy	ctx_k	out_k	lb_agent	lb_code	elo	btr	avail`

| # | Column | Meaning |
|---|---|---|
| 1 | `#name` | model display name (models.dev catalog) |
| 2 | `cost_tok` | retail USD per 1M output tokens |
| 3 | `cost_task` | estimated USD per coding-agent request (OpenCode official token profile, base tier) |
| 4 | `allowance` | monthly usage budget of this model in the plan; `unlimited` for free models |
| 5 | `r_quota` | allowance ÷ cost_task ≈ requests per month (theoretical, full reset) |
| 6 | `tok_s` | Artificial Analysis output tok/s (V3 `performance_v3` mean; legacy median fallback) |
| 7 | `ttft_s` | AA seconds to first token, thinking included (V3 `performance_v3` mean; legacy median fallback) |
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
| 18 | `pareto` | `yes` when no other visible model is cheaper-or-equal, faster-or-equal AND better-or-equal; empty otherwise. Flags are computed after every row filter (`-exclude`, `-include`, `-max-cost`, `-min-speed`, `-min-quality`, `-paid`, `-privacy`, `-avail`), so a hidden model never dominates a visible one. |
| 19 | `src` | `aa:<effort>` when the speed came from that AA reasoning-effort variant |
| 20 | `privacy` | `safe` (no training on request data), `trains` (contributor), or `unknown` (no plan-snapshot verdict) |
| 21 | `ctx_k` | context window in thousands of tokens (models.dev) |
| 22 | `out_k` | maximum output tokens in thousands (models.dev) |
| 23 | `lb_agent` | LiveBench 2.0 Long Agentic score, 0–100 (single run) |
| 24 | `lb_code` | LiveBench 2.0 Coding score, 0–100 (single run) |
| 25 | `elo` | Arena Elo, daily snapshot of the arena.ai text leaderboard (third-party mirror; integer, missing = not in snapshot) |
| 26 | `btr` | Arena Bradley–Terry rating (log-odds scale, ~±0.2; 0 is average, >0 above average), text with style control, overall category (official mirror, fresh; missing = no match) |
| 27 | `avail` | `no` when the user marked the model as not available in their country (`-unavail` option or the config's `unavail` key; the API answered "this model is not available in your country"); `yes` otherwise. `-avail` masks the `no` rows. This is user-maintained state, never scraped. |

On a terminal: same columns, padded for readability, missing values shown as
`-`. Models with peak-hours pricing add a `Name [peak]` twin row (ID
suffixed `-peak`) with `cost_task`/`r_quota` recomputed at peak rates. The `#`-prefixed legend block explaining each column is printed only
with `-l` (in any output mode). `-vertical` emits one model per block of
`field: value` lines (same keys, empty fields skipped, blank line between
records); the field names are the legend, and the prose block again needs
`-l`.

## View: `-news` (change log)

`-news` is a line log, not a column table: one line per change, no header row,
and no `-json`/`-vertical`/`-cols` output (`-json` and `-vertical` print a
warning on stderr and are ignored). The first run records the baseline and
prints nothing; the baseline lives in the cache directory (`news-state.json`).

Line shape:

```
KEYWORD <model-id> <key>=<value> ... "<display name>"
```

Fields are single-space separated and contain no spaces; the display name is
last and quoted, so `awk` or a split-on-space parser can read everything before
it positionally. Numeric money fields use the same `$` credit convention as the
tables. `cost_task` has the main table's meaning (official per-request cost,
base tier); `cost_tok` is the retail USD price per 1M output tokens;
`allowance` is a monthly budget; `ctx_k`/`out_k` are token limits in thousands;
`tier` and `quality` come from the AA Intelligence Index (`frontier` >= 45,
`high` >= 35, `mid` >= 25, `low` below, `unknown` when AA has no score).
`plan=go` or `plan=go-plus` names the budget a line refers to.

The common tail ends with `tier=<tier> quality=<score-or--> "<display name>"`;
lines about a model the user has marked unavailable in their country (`-unavail`)
carry an extra `unavail=yes` field between `quality=` and the quoted name, so a
marked `MODEL_ADD` reads `MODEL_ADD crouton-alpha-free cost_task=$0 plan=free
unavail=yes tier=unknown quality=- "Crouton Alpha Free"`.

| Keyword | Fields | Meaning |
|---|---|---|
| `MODEL_ADD`, `MODEL_DEL` | `cost_task`, `plan=paid/free/unknown`, `tier`, `quality`, optional `unavail=yes`, display name | model joined/left the models.dev catalog |
| `PLAN_ADD`, `PLAN_DEL` | `PLAN_ADD`: `allowance=<go>/<go-plus>`; both: `tier`, `quality`, display name | model joined/left the plan's budget table |
| `COST_UP`, `COST_DOWN` | `cost_task=<old>-><new>`, `(<±pct>)`, optional `price_out=<old>-><new> (<±pct>)`, display name | official cost per request changed |
| `PEAK_COST_UP`, `PEAK_COST_DOWN` | `peak_cost_task=<old>-><new>`, `(<±pct>)`, display name | peak-hour cost per request changed |
| `PEAK_ON`, `PEAK_OFF` | `PEAK_ON`: `peak_cost_task=<new>`; both: display name | the model gained/lost a peak-hours price |
| `PRICE_UP`, `PRICE_DOWN` | the moved field(s): `cost_tok=`, `price_in=`, `cache_read=`, `cache_write=` (`<old>-><new>`), display name | a base token price moved without changing `cost_task` |
| `ALLOWANCE_UP`, `ALLOWANCE_DOWN` | `plan=`, `allowance=<old>-><new>`, `(<±pct>)`, display name | a monthly budget changed |
| `UNLIMITED_ON`, `UNLIMITED_OFF` | `plan=`, `allowance=<old>->unlimited` (or the reverse), display name | the model became/stopped being free |
| `CONTEXT_UP`, `CONTEXT_DOWN` | `ctx_k=<old>-><new>`, optional `out_k=<old>-><new>`, display name | context window or maximum output changed |
| `PRIVACY_CHANGE` | `privacy=<old>-><new>`, display name | the privacy verdict changed |
| `UNAVAIL_ON`, `UNAVAIL_OFF` | `UNAVAIL_ON` carries `unavail=yes`; both: display name | the user added the model to (or removed it from) the `-unavail` country-availability list |
| `NAME_CHANGE` | `was="<old name>"`, then `tier`, `quality`, `"<new name>"` | display name changed |
| `TIER_UP`, `TIER_DOWN` | `tier=<old>-><new>`, `quality=<old>-><new>`, display name | AA quality tier moved |
| `ERROR` | `ERROR <source>: <message>` | a source changed shape, was unreachable, or its refresh failed (an expired cache was used); `-news` does not advance the baseline when the snapshot is unusable |

Common lines end with `tier=<tier> quality=<score-or--> "<display name>"` (plus
an optional `unavail=yes` field behind `quality=`, see above).
When `-email` is set, non-empty output is mailed; two consecutive runs made of
only `ERROR` lines skip the second mail.

## View: `-usage` (allowance windows)

Piped header: `#window	status	used	remain	resets_at	resets_in	forecast`

| # | Column | Meaning |
|---|---|---|
| 1 | `#window` | usage window: `5h` = 20%, `week` = 50%, `month` = 100% of each model's monthly limit (the endpoint reports one aggregate set) |
| 2 | `status` | `ok` / `throttled` / `exhausted`, as reported by the endpoint |
| 3 | `used` | percent of the window allowance already consumed |
| 4 | `remain` | `100 - used` |
| 5 | `resets_at` | local time (RFC3339) when the window resets |
| 6 | `resets_in` | time until reset, days rolled out of hours, zero units omitted, e.g. `4d 10h 15m` |
| 7 | `forecast` | end-of-window outlook at the current average consumption rate: `used=N%` (projected end percent, under 95), `on pace` (100% reached around the reset), `throttle=X` (100% reached `X` before the reset, i.e. throttled from then on), `-` (the window has just started, no elapsed time to pace from, or the reset is past) |

## View: `-now` (what is affordable right now)

Piped header: `#name	cost_task	burn_req	spent_mo	reqs_5h	reqs_wk	reqs_mo	usable	limit	p_tok_s	avail`

| # | Column | Meaning |
|---|---|---|
| 1 | `#name` | model display name (models.dev catalog); `[peak]` suffix marks the peak-hours twin row |
| 2 | `cost_task` | official allowance cost per coding-agent request (OpenCode token profile, base tier) |
| 3 | `burn_req` | allowance dollars one request burns: measured from console tokens when the model has at least 50 requests this period, otherwise `cost_task`; `[peak]` twins scale the measured burn by the official peak/base price ratio |
| 4 | `spent_mo` | allowance dollars spent this period (console tokens priced at Go rates, or `-spent`), subtracted from the model's monthly limit; blank when spend is unknown. With extra usage enabled, console spend can include balance-billed traffic, so this is an upper bound on allowance consumed |
| 5 | `reqs_5h` | requests in the model's 5h cap (20% of its monthly limit) after estimated window spend |
| 6 | `reqs_wk` | requests in the model's weekly cap (50% of its monthly limit) after estimated window spend |
| 7 | `reqs_mo` | requests left in the model's monthly limit after `spent_mo` |
| 8 | `usable` | min of `reqs_5h` / `reqs_wk` / `reqs_mo` — what you can use now |
| 9 | `limit` | binding constraint: `5h` \| `week` \| `month` \| `unlimited` \| `unknown` |
| 10 | `p_tok_s` | **measured** output tok/s on your subscription (from `-probe`) |
| 11 | `avail` | as in the main table: `no` when the user marked the model unavailable in their country (`-unavail`), `yes` otherwise; `-avail` masks the `no` rows before sorting |

`-now` assumptions (printed on stderr, never in the piped stream): Go usage
limits are **per model**. Each model's monthly limit is the dollar allowance
in `internal/goplan`; its 5h and weekly caps are 20% and 50% of that limit.
There is no shared pot: a logged run (2026-10-29) shows one model's 5-hour
allowance exhausted while another model kept serving inside the same window;
older docs described an "across models" pool
(see `docs/OPENCODE_GO_MODEL_V2.md`). The usage endpoint reports one aggregate
set of percentages. `reqs_5h` and `reqs_wk` assume fresh windows unless
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

Piped header: `#name	cost_tok	cost_task	allowance	r_quota	tok_s	ttft_s	p_tok_s	p_ttft_s	aa_int	aa_code	aa_math	code_b	term_b	if_b	aa_lcr	swe_b	pareto	src	privacy	ctx_k	out_k	lb_agent	lb_code	elo	btr	avail	burn_req	spent_mo	reqs_5h	reqs_wk	reqs_mo	usable	limit`

Column order is the main order first, then the `-now`-only columns in `-now`
order. Shared titles (`name`, `cost_task`, `p_tok_s`) carry the same value in
both views and appear once. Titles are unique across views: the theoretical
monthly count is `r_quota` (main), the remaining count is `reqs_mo` (`-now`).

| # | Column | Meaning |
|---|---|---|
| 1–25 | main columns, in main order | identical values and meanings to the main table above |
| 26 | `burn_req` | as in `-now` |
| 27 | `spent_mo` | as in `-now` |
| 28 | `reqs_5h` | as in `-now` |
| 29 | `reqs_wk` | as in `-now` |
| 30 | `reqs_mo` | as in `-now`: requests left in the monthly limit after `spent_mo` |
| 31 | `usable` | as in `-now` |
| 32 | `limit` | as in `-now` |

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

- From the `v1.0.0` tag on the contract is frozen: new columns may be added
  (appended at the end, rule 1); a rename, removal or reorder is a breaking
  change and bumps the major version (rule 2).
- Before `v1.0.0` there was no guarantee; titles changed freely (old rule 3).

## JSON output (`-json`, all column views)

`-json` works for every column view and emits an array of objects whose keys
are **the column titles above, without the `#` prefix** — same names, so one
spec covers both formats. (`-news` is a line log and ignores `-json`.) Numbers
stay numbers, `pareto` stays boolean, and `resets_at`/`resets_in` keep their
meanings. JSON-only extras:

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
