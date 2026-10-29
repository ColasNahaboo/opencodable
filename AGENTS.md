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
cmd/opencodable/       flags, view orchestration (-now, -usage, -probe, -doctor)
internal/modelsdev/    models.dev catalog: prices, limits (authoritative)
internal/goplan/       OpenCode Go plan SNAPSHOT: allowances, token profiles,
                       peak prices, privacy flags — update when plans change
internal/aa/           Artificial Analysis data API client
internal/bench/        external feeds (SWE-bench Verified)
internal/consoleapi/   console usage API (per-model period spend)
internal/usage/        allowance windows, budgets/requests math
internal/probe/        live speed probe (SSE streaming, allowance-burn reporting)
internal/normalize/    name matching + alias table (models.dev ↔ AA slugs)
internal/cost/         per-request cost model
internal/table/        join, Pareto, TSV/vertical/JSON rendering
internal/cache/        24h disk cache (~/.cache/opencodable/)
internal/config/       opencodable.conf (key=value: API keys + option
                       defaults) in UserConfigDir; case-coded, see gotchas
```

## Hard rules

- **Standard library only.** No external Go dependencies, ever — a Rust port
  of the same pipeline is planned and must stay mechanical.
- **[TITLES.md](TITLES.md) is the output contract.** New columns are appended
  at the end only; renames/removals/reorders bump the major version. JSON
  keys must equal the TSV column titles (enforced by
  `TestJSONKeysMatchTitles`).
- **Output modes.** tty = padded table, missing = `-`; piped = strict
  single-TAB, missing = empty string. Legends (`# `-prefixed) are opt-in with
  `-l` in every view and every mode; never print notes or assumptions into
  stdout — put them on stderr. Exception: `-doctor`'s header row starts with
  `#`, and its legend prints only with `-l`.
- All column titles contain no spaces (underscores only).
- **Version history lives in [HISTORY.md](HISTORY.md)**, not README.md: on
  every version bump, prepend the new entry there (as before) and update the
  one-line `Version vX.Y.Z <date>` marker at the end of README.md's History
  section to the current version and date.
- Keep functions small and focused; idiomatic Go 1.26; no refactors outside
  the scope of a change.
- **All CLI Options** must also be settable in the config file**.
- **Boolean CLI options** can be unset by suffixing them by `=false`

## Data-source gotchas (learned the hard way)

- **AA slug matching**: `normalize.Match` resolves a models.dev ID by exact
  slug, then the alias table in `internal/normalize` (`mimo-v2.5` →
  `mimo-v2-5-0424`, `step-5-preview` → `step-5`, …), then dots-as-dashes
  (`qwen3.8-max` → `qwen3-8-max`). When a model shows `-` scores, suspect a
  missing alias first. AA has reasoning-effort variants (`-low/-high/…`); the
  base slug may exist with tok/s = 0 — `VariantWithSpeed` picks the fastest
  measured variant.
- **AA ttft_s artifact**: includes reasoning time (Haiku ~247 s). Reported,
  not a bug.
- **Go limits are per model, not a shared pot**: docs define each model's
  5h/weekly/monthly windows as 20%/50%/100% of *that model's* monthly limit.
  `-now` builds `Budgets` from `goplan.Allowance`; there is no `-pot` flag.
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
  console's list cost, not the allowance cost. `-window-estimate` scales the
  per-model 5h/week caps by the endpoint's aggregate remaining percentages;
  it is an approximation, not per-model window spend.
- **goplan snapshot**: allowances, profiles and privacy flags are hardcoded
  from the public docs; new OpenCode models need new entries (unknown models
  fall back to a generic profile). `goplan.Privacy` is tri-state
  (`safe`/`trains`/`unknown`); `-privacy` fails closed, hiding both training
  and unknown models. Free variants missing from the snapshot are inferred
  from zero prices (`modelsdev.Model.Free`).
- **Cache hardening**: keys are sanitized (no traversal), writes are
  temp-file + rename, permissions are 0700/0600. `cache.Age` feeds `-doctor`.
- **Key resolution order** (both APIs): environment variable →
  `opencodable.conf` (`key=value`, in `os.UserConfigDir()/opencodable/`) →
  legacy cwd dot-file. `internal/config` owns the file; never print values,
  and `-doctor` reports only the source and unknown key names.
- **Config keys are case-coded**: uppercase/mixed = environment-style
  settings (API keys); all-lowercase = command-line option defaults
  (command line beats file beats flag default; booleans are
  true/on/1/yes | false/off/0/no). **When adding a flag**, add its name to
  the `config.AddOptions` call and to `applyConfig`'s str/boolean maps in
  `cmd/opencodable/main.go`, or `-doctor` will flag it as unknown and the
  file default is ignored. The one exception is `-config`, which selects the
  file itself and therefore cannot come from it.
- **Doctor**: `-doctor` audits plan/AA/SWE/probe coverage without running a
  probe; keep `buildDoctorRows` pure and tested.
- **Console API has no time slicing** (verified Oct 2026: startDate/days/
  period params ignored, from/to 400s): per-model window spend is estimated
  (ledger attribution + monthly-proportional split, `window_method`), never
  measured. `reqs_5h`/`reqs_wk` are fresh caps without `-window-estimate`.
- **Live usage endpoints are TTL-cached** (`-ttl` seconds, default 600):
  console spend + allowance windows; `-refresh` bypasses, stale fallbacks
  warn with their age. Probes stay 24h; the spend ledger (`spend-ledger.json`)
  is append-only local history, never polled.

## Testing

Table-driven tests next to the code (`*_test.go`). Network is never touched
by unit tests: parse/normalize/render logic is separated from fetching
precisely so tests feed fixtures (see `usage_test.go`, `swebench_test.go`,
`probe_test.go` with a fake SSE server). Run with `-count=1`.
