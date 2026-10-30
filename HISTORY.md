# History

- v0.1.20 2026-10-30 Live plan snapshots: allowances, token profiles and peak
  prices are now refreshed from the OpenCode Go docs page and models endpoint
  (weekly by default, `-plan-ttl`, forced by `-refresh`) and layered over the
  compiled-in snapshot. Every scrape passes sanity gates before use — row
  completeness, value domains, prices cross-checked against models.dev,
  budgets against the docs' requests-per-month figures, drift caps against the
  previous snapshot, peak prices against the endpoint's multiplier — and any
  rejection falls back to the built-in table silently. `-doctor` shows the
  active source (`live (date)` / `builtin (date)`) and the plan cache age;
  privacy flags stay built-in.
- v0.1.19 2026-10-30 Review fixes: `-probe` now counts first-try probe burns
  toward `-probe-max-allowance` and the total burned summary, escalates
  thinking models that exhaust the default token budget in reasoning without
  emitting content, and avoids caching failed probes; `swe_b` resolves by
  model ID/name independently of Artificial Analysis (`AA_API_KEY` optional,
  dated AA slugs no longer mask SWE-bench); `-spent` resolves pasted model
  display names to catalog IDs; `-window-estimate` snapshots off-peak base USD
  so toggling `-peak` does not forge ledger deltas; `-doctor` audits LiveBench
  cache age and coverage; `-sort` validates its key upfront and works in `-now`
  too; `opencodable.conf` strips unquoted inline `#` comments and caches the
  parsed file; `btr` docs state the Bradley–Terry log-odds scale (~±0.2).
- v0.1.18 2026-10-29 `-now` peak twins now scale a measured `burn_req` by the
  official peak/base price ratio, so a `[peak]` row never looks cheaper per
  request than its base. Before this, the twin used the official peak profile
  while the base used the measured average; when actual usage was heavier than
  the official profile (DeepSeek V4.1 Flash), peak appeared to afford more
  requests.
- v0.1.17 2026-10-29 Migrate the Artificial Analysis client to the supported
  V2 endpoints before the legacy `/api/v2/data/*` retirement on November 4,
  2026: `/api/v2/language/models/free` by default, the Pro route
  `/api/v2/language/models` with `-aa-pro`, both paginated (200 rows/page,
  `pagination.has_more`) with prices under `pricing` and medians under
  `performance`. Free and Pro cache separately
  (`aa-models-v2-free.json` / `aa-models-v2-pro.json`). AA no longer publishes
  Coding, Math or LiveCodeBench, so `aa_code`, `aa_math` and `code_b` stay
  empty; `term_b`, `if_b` and `aa_lcr` need a Pro key. `-aa-pro` is
  config-settable, and `-doctor` follows the active AA cache.
- v0.1.16 2026-10-29 Elo aliases for `kimi-k3` (Kimi K3 Max) and
  `muse-spark-1.3-contributor` (Muse Spark 1.3 Max): their Elo values now
  show, and the `-doctor` hints for them are gone.
- v0.1.15 2026-10-29 `-doctor` coverage for Arena ratings: `elo` and `btr`
  matched counts on the coverage line (Arena lists only top models, so a
  missing value stays silent) and a hint when a model has no match but a
  leaderboard key is one of its prefixes (a likely alias, such as a Max
  qualifier). Doctor JSON rows gain `elo_match` and `btr_match`; the caches
  line and the cache-age warnings cover the Arena caches too.
- v0.1.14 2026-10-29 `-include`: show only models matching ID/name patterns,
  the pendant of `-exclude` (same wildcard rules; a row must match `-include`
  and dodge `-exclude`; unprobed and unrendered otherwise, so narrow runs
  also save allowance). `-doctor` warns about catalog and benchmark caches
  older than 24 hours; normal runs stay silent on them. README examples use
  illustrative numbers.
- v0.1.13 2026-10-29 `-window-estimate` upgraded from uniform cap scaling
  to spend attribution: ledger-recorded dollars (past runs' console deltas,
  fresh probe burns) first, the implied remainder split
  monthly-proportionally; method on stderr and as `window_method` in JSON
  (`full`/`proportional`/`ledger`). Live usage endpoints (console spend,
  allowance windows) are TTL-cached (`-ttl` seconds, default 600, `0` =
  live); stale fallbacks warn instead of failing silently.
- v0.1.12 2026-10-29 Peak twins in every model view: models with peak-hours
  pricing (DeepSeek) gain a `Name [peak]` row (ID suffixed `-peak`) with
  `cost_task` and quotas/counts recomputed at peak rates. Twins share the
  base's probes, scores, privacy, allowance and spend dollars; `-now` peak
  `burn_req` prices the official profile (measured burn needs regime tags
  the API lacks). Twins inherit `-paid`/`-privacy`, face value filters
  themselves, and vanish under `-peak` (whole table already peak-priced).
- v0.1.11 2026-10-29 `-nohead`: omit the header row in every view (data
  rows only; `-l` still prints, no-op in `-vertical`/`-json`).
- v0.1.10 2026-10-29 Arena columns in the main view (and `-full`): `elo`
  (daily third-party snapshot of the arena.ai text leaderboard: top-20, can
  lag, walks back ≤31 days for the newest non-empty snapshot) and `btr`
  (official Bradley–Terry rating on the Elo scale, text with style control,
  fresh; overall rows lead so the fetch stops after 2–3 pages). Both
  filterable via `-min-quality`; neither joins the composite quality, Pareto
  or `-sort`. A dead feed blanks its column with a warning, never an error.
- v0.1.9 2026-10-29 `-full` view: the main table joined with `-now`
  (main columns first, then the affordability columns). Column titles are
  now unique across views: main `reqs_mo` is renamed `r_quota` (pre-1.0).
  Plus column selection in every model view: repeatable `-cols` (show only)
  and `-excols` (show all but), validated against all views at once so one
  global preset serves main, `-now` and `-full`; `name` is always shown.
  Plus `-main` to name the default view explicitly; combining two views is
  an error.
- v0.1.8 2026-10-28 Row filters: `-exclude` (case-insensitive ID/name
  patterns, `*`/`?` wildcards),
  `-max-cost` (`cost_task` ceiling), `-min-speed` (effective tok/s),
  `-min-quality` (`metric=value` floors over the quality columns, composite
  when bare) and `-missing keep|drop` (default `keep`: no value means the
  bound does not apply). Filters run before Pareto marking; `-now` honours
  exclude/cost/speed and notes that quality needs AA data.
- v0.1.7 2026-10-28 legends are opt-in: every view prints only its header row
  by default, and `-l` adds the `# `-prefixed column legend (terminal, piped,
  `-vertical`, `-now`, `-usage`, `-doctor`).
- v0.1.6 2026-10-28 `-config FILE` reads settings and option defaults from
  another file (the path must exist; an empty file drops every default), and
  boolean options can be overridden per run with `-name=false`, e.g.
  `-paid=false` against `paid=yes` in the file.
- v0.1.5 2026-10-28 LiveBench 2.0 columns (`lb_agent`, `lb_code`) from the
  pinned 2026-10-25 release, fetched as static CSV (no key) and cached as
  JSON; AA remains the only price/speed source.
- v0.1.4 2026-10-28 every command-line option can be set as a default in
  `opencodable.conf` under its lowercase name (command line wins; booleans
  true/on/1/yes | false/off/0/no).
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
