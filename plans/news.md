# Plan: `-news` — what changed in the OpenCode Go plan

Detect what is new in the OpenCode Go subscription between invocations: new
models, removed models, price moves, allowance changes, anything worth noting.
One line per change, designed to run from cron and mail a user list.

## User-visible behavior

- `opencodable -news` prints one line per change since the previous `-news`
  run; the **first run prints nothing** and only records the baseline.
- `opencodable -news -email a@x.com,b@y.com` additionally mails the lines
  (comma-separated list; also settable as `email=` in `opencodable.conf`).
- `ERROR <source>: <message>` lines signal a broken source — the site or API
  changed shape and opencodable's parsing needs a tweak.
- `-news` is a view like `-doctor`: mutually exclusive with the other views;
  `-json`/`-vertical` are ignored (warning on stderr); only meaningful with
  `-provider opencode-go`.

## Line format

`KEYWORD <id> key=value ... "Display Name"` — space-separated fields, the
display name last and quoted (names contain spaces; parsers split on spaces
up to the name). Every line carries the quality tier so the reader can judge
the change's importance.

```
MODEL_ADD glm-5.2 cost_task=$0.01516 plan=paid tier=unknown quality=- "GLM-5.2"
COST_DOWN kimi-k3.1-pro cost_task=$0.0204->$0.018 (-11.8%) price_out=$15->$12 (-20.0%) tier=frontier quality=46.5 "Kimi K3.1 Pro"
ALLOWANCE_UP qwen3.8-max plan=go allowance=$10->$20 (+100.0%) tier=high quality=38.2 "Qwen3.8 Max"
ERROR plan: plan snapshot rejected: docs price mismatch for glm-5.2
```

Keywords:

| Keyword | Meaning |
|---|---|
| `MODEL_ADD`, `MODEL_DEL` | model joined / left the models.dev `opencode-go` catalog |
| `PLAN_ADD`, `PLAN_DEL` | model joined / left the plan's budget table (goplan) |
| `COST_UP`, `COST_DOWN` | `cost_task` (official cost per coding-agent request) moved; includes `price_out` when the retail output price moved too |
| `PRICE_UP`, `PRICE_DOWN` | a raw token price moved without changing `cost_task` |
| `PEAK_ON`, `PEAK_OFF`, `PEAK_COST_UP`, `PEAK_COST_DOWN` | peak-hour premium appeared / disappeared / moved |
| `ALLOWANCE_UP`, `ALLOWANCE_DOWN` | monthly budget moved, `plan=go` or `plan=go-plus` |
| `UNLIMITED_ON`, `UNLIMITED_OFF` | model became free/unlimited, or stopped being |
| `CONTEXT_UP`, `CONTEXT_DOWN` | context window or max output moved (`ctx_k=`, `out_k=`) |
| `PRIVACY_CHANGE` | privacy verdict moved (`safe` / `trains` / `unknown`) |
| `NAME_CHANGE` | display name changed |
| `TIER_UP`, `TIER_DOWN` | coarse AA quality tier moved |

Quality tier: `tier=` is a coarse bucket of the AA Intelligence Index —
`frontier` (≥45), `high` (≥35), `mid` (≥25), `low` below, `unknown` when AA
has no score; `quality=` carries the raw index (`-` when unknown). AA data
comes from the normal 24h cache; an AA outage never blocks `-news` (scores
carry over from the baseline; the warning prints only on a TTY).

## Data sources

- `modelsdev.Fetch("opencode-go")` — catalog membership, token prices, limits
  (24h cache).
- `goplan.Fetch` — live plan snapshot (docs page + Go models endpoint,
  weekly TTL, Validate-gated) overlaid via `goplan.Use`; budgets, profiles,
  peak prices, privacy.
- `aa.Fetch` — Intelligence Index for the tier (24h cache, optional).
- No probes, no console spend, no benchmark feeds: `-news` is free and
  network-light (normal cached fetches; `-refresh` forces them).

## Baseline state (`news-state.json`, cache dir)

Ledger pattern (`cache.Peek`/`cache.Store`, corrupt/missing = empty, never an
error). Contents:

- one snapshot per model: `cost_task`, peak cost, the four token prices, both
  plan allowances + unlimited flags, plan membership, context/output limits,
  privacy, AA quality;
- the plan source (`live` / `builtin`) at snapshot time;
- the **previous run's output lines** (for the ERROR-mail suppression rule).

Rules:

- First successful run: baseline recorded, nothing printed.
- Failures never advance the model baseline — only its `output` field (for
  suppression) — so the recovery run reports everything accumulated since the
  last good run.
- Source transitions are not news: `builtin → live` suppresses the diff (the
  deltas are source artifacts); `live → builtin` keeps the baseline untouched
  so the next online run still reports pending changes.
- Deleting the cache resets the baseline: next run is a silent first run.

## Error detection

- Any fetch/parse failure → `ERROR <scope>: <msg>` on stdout (scopes:
  `models.dev`, `plan`).
- A silently-stale cache (fetch failed, `cache.Load` served the old copy) is
  detected via `cache.Age` exceeding the TTL and reported as an ERROR too —
  otherwise a site change would look like "no news" forever.
- Without a trustworthy plan snapshot, no diff is produced (comparing builtin
  budgets against live ones would invent changes).
- AA failure is never an ERROR (tier is a bonus).

## Mailing

- `-email` / `email=` config key; addresses validated with `net/mail`
  (header injection impossible by construction).
- Non-empty output + recipients → send: `sendmail -t -oi` when available
  (RFC 822 message with `Auto-Submitted: auto-generated`), else
  `mail -s <subject>`. Subject: `opencodable news: N change(s)` or
  `opencodable news: check failed`.
- **Suppression rule:** a run whose output is only ERROR lines is not mailed
  when the previous run's output was also only ERROR lines (persistent
  breakage must not page forever).
- Mail failure returns an error and the baseline is not advanced: the next
  run retries both the mail and the same diff.
- Cron: the tool mails itself, so the crontab drops stdout to avoid a second
  copy:
  `0 8 * * * opencodable -news -email you@example.com >/dev/null`
- stderr diagnostics (suppressed mail, missing AA key) print only on a TTY,
  so cron stays quiet unless something really failed.

## Implementation shape

- `cmd/opencodable/news.go`: `runNews(w, email, refresh, offline, aaPro,
  planTTL)` early-return dispatch block in main + `selectedViews()` entry +
  flags in `registerFlags`, `config.AddOptions`, `applyConfig` (bool `news`,
  str `email`).
- Pure core, table-driven tested: `buildNewsSnapshot`, `diffNews` /
  `newsChangeLines`, `isErrorOutput`, `parseEmails`, `newsTier`.
- Mail behind an injectable seam (`newsMailer`), like `probeRun`; unit tests
  never touch the network or spawn processes.
- Float comparisons use a 0.1% relative tolerance (`newsChanged`) to ignore
  round-trip noise.

## Docs & release

- README: "What's new (`-news`)" section (keywords table, cron example, tier
  thresholds).
- TITLES.md: `-news` is not a table view — document the keyword lines in a
  short "View: `-news`" note (no column contract).
- Version bump + HISTORY.md entry per AGENTS.md; commit with the auto-commit
  skill convention.

## Open points

- The goplan `Validate` drift caps (>30% turnover, 4× allowance) reject big
  *real* plan changes as if they were scrape mis-parses: such a rejection
  surfaces as `ERROR plan: plan snapshot rejected: ...`. Acceptable v1
  behavior (a human tweaks the caps/snapshot), but worth revisiting: `-news`
  could compare pre-Validate rows to report the change instead.
- `cost_task` uses the official token profile; a profile change alone shows
  as `COST_*` without a `price_out` field — fine, it is a plan change.
