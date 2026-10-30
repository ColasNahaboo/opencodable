# opencodable — agent guide

Go CLI ranking the models of an [OpenCode Go](https://opencode.ai/v2/docs/console/go/)
subscription by price / speed / quality, with live allowance tracking.
User-facing documentation lives in [README.md](README.md); the **output
contract** is [TITLES.md](TITLES.md).

## Build & test

```sh
go build -o TMP/opencodable ./cmd/opencodable   # TMP/ is git-ignored
go test ./... -count=1
go vet ./... && gofmt -l .                      # both must stay clean
```

Run against live data (network + possibly a few cents of allowance for
probes): `TMP/opencodable`, `... -usage`, `... -now`. The
`.artificialanalysis.key` and `.opencode-go.key` dot-files in the repo root
are git-ignored credentials — never print, commit, or copy them.

## Layout

```
cmd/opencodable/       flags and view orchestration (-main, -now, -full, -usage,
                       -probe, -doctor); ledger.go: spend history for
                       -window-estimate; doctor.go: coverage audit
internal/modelsdev/    models.dev catalog: prices, limits (authoritative)
internal/goplan/       OpenCode Go plan SNAPSHOT: allowances, token profiles,
                       peak prices, privacy flags — built-in fallback plus
                       live.go/validate.go weekly docs+endpoint refresh with
                       sanity gates (live.Use overlays the table)
internal/aa/           Artificial Analysis V2 data API client (Free/Pro, paginated)
internal/bench/        external feeds: SWE-bench Verified, LiveBench 2.0,
                       Arena Elo (third-party snapshot), Arena BTR (official)
internal/consoleapi/   console usage API (per-model period spend, TTL-cached)
internal/usage/        allowance windows (TTL-cached), budgets/requests math
internal/probe/        live speed probe (SSE streaming, allowance-burn reporting)
internal/normalize/    name matching + alias table (models.dev ↔ AA slugs)
internal/cost/         per-request cost model
internal/table/        join, Pareto, row filters (filter.go), column selection
                       and -full titles (columns.go), peak twins, TSV/vertical/
                       JSON rendering
internal/cache/        disk cache (os.UserCacheDir()/opencodable/): 24h for
                       catalogs, benchmarks and probes; per-call TTL for usage
internal/config/       opencodable.conf (key=value: API keys + option
                       defaults) in UserConfigDir; case-coded, see gotchas
```

## Hard rules

- **Standard library only.** No external Go dependencies, ever — a Rust port
  of the same pipeline is planned and must stay mechanical.
- **[TITLES.md](TITLES.md) is the output contract.** New columns are appended
  at the end only; renames, removals and reorders bump the major version, but
  titles may change freely until the `v1.0.0` tag (TITLES.md rule 3). Every TSV
  title is a JSON key (`TestJSONKeysMatchTitles`); JSON-only extras (`id`,
  `unlimited`, `window_method`, …) are listed in TITLES.md's JSON section.
- **Output modes.** tty = padded table, missing = `-`; piped = strict
  single-TAB, missing = empty string. Legends (`# `-prefixed) are opt-in with
  `-l` in every view and every mode; never print notes or assumptions into
  stdout — put them on stderr. Exception: `-doctor`'s header row starts with
  `#`, and its legend prints only with `-l`.
- All column titles contain no spaces (underscores only).
- **Version history lives in [HISTORY.md](HISTORY.md)**, not README.md: on
  every version bump, set `version` in `cmd/opencodable/main.go`, prepend the
  new entry to HISTORY.md, and update the one-line `Version vX.Y.Z <date>`
  marker at the end of README.md's History section.
- Keep functions small and focused; idiomatic Go 1.26; no refactors outside
  the scope of a change.
- **All CLI options** must also be settable in the config file (`-config`
  excepted, see gotchas).
- **Boolean CLI options** can be unset by suffixing them by `=false`
- **Commit, version and push each change** once it builds and passes the checks
  in Build & test: bump the last number of the version (v0.1.15 → v0.1.16), tag
  that commit, and push the commit and the tag. Author the commit with the
  auto-commit skill's convention: your active model name, not a model name from
  the catalog data.
- **Doc-only changes** (no code touched): just commit them — no version bump,
  no tag, no push.
- **No polling.** Never add loops, daemons or repeated requests against web
  endpoints: repeated requests may trigger a ban. Every network fetch goes
  through `cache.Load` with a TTL (24h for catalogs, benchmarks and probes;
  `-ttl` for the usage endpoints) and honours `-refresh`; `-offline` never
  reaches the network. Probes are paid: they run only on explicit `-probe` or
  `-probe-model`.
- **Private and scratch files.** `TODO.md` is the maintainer's git-ignored
  scratchpad: don't edit, delete or publish it unless asked. `TMP/` is
  git-ignored scratch, but `go vet ./...`, `go test ./...` and `gofmt -l .`
  still compile and scan it: keep scratch programs compiling, or remove them.

## Data-source gotchas (learned the hard way)

- **AA API v2**: the legacy `/api/v2/data/llms/models` endpoint retires on
  2026-11-04. `internal/aa` uses `/api/v2/language/models/free` by default and
  the Pro route `/api/v2/language/models` under `-aa-pro`. Both are paginated
  (`?page=N`, 200 rows/page, follow `pagination.has_more`) and nest prices
  under `pricing` and medians under `performance`. Free carries the
  Intelligence Index and capability indices only; `term_b`, `if_b` and
  `aa_lcr` need Pro. AA no longer publishes Coding, Math or LiveCodeBench, so
  `aa_code`, `aa_math` and `code_b` stay empty. Free and Pro use separate cache
  keys (`aa-models-v2-free.json` / `aa-models-v2-pro.json`); the Free tier
  allows 100 requests per 24h, so a full refresh costs 4.
- **AA slug matching**: `normalize.Match` resolves a models.dev ID by exact
  slug, then the alias table in `internal/normalize` (`mimo-v2.5` →
  `mimo-v2-5-0424`, `step-5-preview` → `step-5`, …), then dots-as-dashes
  (`qwen3.8-max` → `qwen3-8-max`). `-doctor` separates the two causes of
  missing AA data: `no AA match` is an alias gap (fix `normalize`); a matched
  model with empty scores means Artificial Analysis has not published that
  score yet, or the score is Pro-only (see the previous bullet). AA has
  reasoning-effort variants (`-low/-high/…`); the base slug may exist with
  tok/s = 0 — `VariantWithSpeed` picks the fastest measured variant.
- **AA ttft_s artifact**: includes reasoning time (Haiku ~247 s). Reported,
  not a bug.
- **Go limits are per model, not a shared pot**: docs define each model's
  5h/weekly/monthly windows as 20%/50%/100% of *that model's* monthly limit.
  `-now` builds `Budgets` from `goplan.Allowance`; there is no `-pot` flag.
  `-window-estimate` then subtracts estimated window spend from the 5h and
  weekly caps.
- **Allowance burn ≠ list cost**: a probe's `opencode.allowance` can exceed
  `list_cost` (Kimi K3.1 Pro 4×, MiniMax M3.1 Flash 3×), but that ratio is a
  property of the probe's token mix and must **not** be multiplied into the
  official profile's `cost_task`. `-now` instead uses the console's actual
  token counts to derive the measured burn when a model has ≥50 requests.
- **Probe reliability**: thinking models stream `reasoning_content` first and
  can exhaust small token budgets before emitting content (escalate budget);
  very fast models finish in < 1 s windows (retry with the long prompt;
  `probe.MinWindow` guards the tok/s math). Short windows above
  `probe.HighRateTokS`, or rates above `probe.MaxPlausibleTokS`, are marked
  `Suspect` and excluded from ranking. Probes send a unique
  `x-opencode-session` and `opencodable/<version>` UA; `-probe-max-allowance`
  caps a full run's allowance burn. Probe failures are cheap — retry live
  instead of serving a stale error.
- **Usage endpoint** (`/inference/go/v1/usage`) reports one aggregate set of
  percentages, not per-model spend. Per-model spend comes from
  `console.opencode.ai/api/usage/models`; filter to `provider ==
  "opencode-go"` (the API also returns pay-as-you-go `opencode` rows) and
  price the token counts at Go rates. `totalCostMicroCents ÷ 1e8` is the
  console's list cost, not the allowance cost. `-window-estimate` turns the
  endpoint's aggregate percentages into implied window dollars and attributes
  them (see Spend ledger); it is an estimate, never per-model window spend.
- **goplan snapshot**: allowances, profiles and privacy flags are hardcoded
  from the public docs; new OpenCode models need new entries (unknown models
  fall back to a generic profile). A live refresh (`goplan.Fetch`, weekly via
  `-plan-ttl`) scrapes the docs page + Go models endpoint and overlays the
  table via `goplan.Use` — only after `Validate` passes every gate (domains,
  models.dev price agreement, requests arithmetic ±33%, drift caps, peak
  factor); any failure keeps the built-in table silently. Privacy flags are
  never scraped. Tests use the HTML/JSON fixtures in `testdata/`.
  `goplan.Privacy` is tri-state
  (`safe`/`trains`/`unknown`); `-privacy` fails closed, hiding both training
  and unknown models. Free variants missing from the snapshot are inferred
  from zero prices (`modelsdev.Model.Free`).
- **Cache hardening**: keys are sanitized (no traversal), writes are
  temp-file + rename, permissions are 0700/0600. `cache.Age` feeds `-doctor`.
  `cache.Load` serves a stale entry when a fetch fails. Live usage data reports
  that on stderr (`staleWarn`). The 24h catalog and benchmark caches stay silent
  in normal runs; `-doctor` warns about them (`staleCacheWarnings`). `-ttl 0`
  always refetches; `-refresh` bypasses the TTL.
- **Key resolution order** (both APIs): environment variable →
  `opencodable.conf` (`key=value`, in `os.UserConfigDir()/opencodable/`) →
  legacy cwd dot-file. `internal/config` owns the file; never print values,
  and `-doctor` reports only the source and unknown key names.
- **Config keys are case-coded**: uppercase/mixed = environment-style
  settings (API keys); all-lowercase = command-line option defaults
  (command line beats file beats flag default; booleans are
  true/on/1/yes | false/off/0/no). **When adding a flag**, add its name to
  the `config.AddOptions` call and to the `applyConfig` map matching its type
  (str, boolean, float, int or list) in `cmd/opencodable/main.go`, or
  `-doctor` will flag it as unknown and the file default is ignored. The one
  exception is `-config`, which selects the file itself and therefore cannot
  come from it.
- **Doctor**: `-doctor` audits plan/AA/SWE/Arena/probe coverage without running
  a probe; keep `buildDoctorRows` pure and tested. Arena (`elo`, `btr`) lists
  only top models, so an absent value is silent; `Near` hints only when one key
  is a prefix of the other (a likely alias, such as a Max qualifier).
- **Console API has no time slicing** (verified Oct 2026: startDate/days/
  period params ignored, from/to 400s): per-model window spend is estimated
  (ledger attribution + monthly-proportional split, `window_method`), never
  measured. `reqs_5h`/`reqs_wk` are fresh caps without `-window-estimate`.
- **Live usage endpoints are TTL-cached** (`-ttl` seconds, default 600):
  console spend and allowance windows. `-refresh` bypasses the TTL; stale
  fallbacks warn with their age. Probes stay 24h.
- **Spend ledger** (`spend-ledger.json`, in the cache dir): one previous
  per-model spend snapshot, written only by `-window-estimate` runs with real
  spend data, plus each successful live probe's allowance burn; probes older
  than 35 days are pruned. It sharpens window attribution, and deleting the
  cache resets it to the proportional fallback. Never snapshot assumed-$0
  spend: it would forge the next delta. Non-positive deltas are skipped (a
  period reset shows as a drop).
- **Peak twins** are synthetic rows (`Name [peak]`, ID suffixed `-peak`) added
  after paid/privacy pruning and before the value filters, Pareto and sort
  (`table.WithPeakTwins`; `buildNowRows` for `-now`). They share the base's
  probes, scores, privacy and spend, so look models up by the base ID (`m.ID`),
  never the twin's. `-now` twins scale the base's measured `burn_req` by the
  official peak/base price ratio; without a measured average they use the
  official peak profile. `-peak` disables twinning.
- **Row pipeline order is load-bearing**: `Build`, paid/privacy prune, peak
  twins (not under `-peak`), `Exclude`/`Include`/`Keep`, `MarkPareto` (hidden
  rows must never dominate), `Sort`, `-pareto`. The main flow and `runFull`
  each spell this out; keep them in step.
- **Titles are unique across views.** `FullTitles` dedups by name, so a `-now`
  title equal to a main title is silently dropped from `-full`. `-cols` and
  `-excols` validate against `FullTitles`, so a new title needs no registration
  there. Legend lines are `# <title>  description`; `FilterLegend` keys on the
  second word.

## Testing

Table-driven tests next to the code (`*_test.go`). Unit tests never touch the
network. Logic lives in pure functions over data (`buildNowRows`, `joinFull`,
`estimateWindowSpend`, `attributeLedger`, `table.WithPeakTwins`, `Filter.Keep`)
that tests call directly; the `cmd/` wrappers only fetch and print. Fetchers
are tested through seams: fixtures (`usage_test.go`, `swebench_test.go`),
`httptest` servers (`consoleapi_test.go`, and the fake SSE server in
`probe_test.go`) and injected getters (`resolveEloSnapshot`). Tests that touch
the disk cache or config point `XDG_CACHE_HOME` / `XDG_CONFIG_HOME` at a temp
dir (honoured on Linux; macOS and Windows resolve their own dirs). Run with
`-count=1`.
