# OpenCode Go: are model allowances per model or shared?

**Status:** evidence file, 2026-11-01.
**Question:** does exhausting one Go model's 5-hour / weekly / monthly allowance
block every other Go model for the same period, or is each model's allowance
independent?

**Answer in one line:** the current (V2) documentation describes per-model
dollar limits and does not say whether the windows are pooled; the docs were
rewritten on 2026-09-28 and 2026-10-12 from an explicit "across models" pool
wording; a logged natural experiment on 2026-10-29 shows one model's 5-hour
allowance being exhausted while another model kept serving requests inside the
same 5-hour window, which rules out the "weighted single pool" scheme the old
docs described.

Everything below is reproducible from public pages, public commit history, the
local OpenCode store, and the console usage API. Commands and rate tables are in
[Reproduction](#9-reproduction).

---

## 1. Current documentation (fetched 2026-11-01)

Two URLs serve the same V2 text:

- <https://opencode.ai/v2/docs/console/go/>
- <https://opencode.ai/docs/go/>

The complete normative text of the "Usage limits" section is three sentences
(quoted verbatim):

> Usage limits are defined as monthly dollar amounts. The table below shows the
> monthly limit for each plan and the token costs for each model. Token pricing is
> the same for Go, Go Plus, and Go Teams.
> 
> Each model has the following usage limits: 5-hour — 20% of the monthly limit;
> weekly — 50%; and monthly — 100%.
> 
> Each model's monthly limit below determines how its usage counts toward those
> allowances. Each response reports how much the request counted, as
> [`opencode.allowance`](/console/api/inference#cost).

The table that follows has one `Monthly limit` per model, for each plan. Values
relevant to this note:

| Model                            | Go   | Go Plus | Go Teams |
| -------------------------------- | ---- | ------- | -------- |
| Claude Haiku 5.5 (≤ 100K tokens) | $15  | $60     | $20      |
| DeepSeek V4.1 Flash (Off-Peak)   | $100 | $200    | $105     |
| DeepSeek V4.1 Flash (Peak)       | $100 | $200    | $105     |
| DeepSeek V4 Pro                  | $15  | $60     | $20      |
| GPT 6 Luna                       | $15  | $60     | $20      |
| GLM-5.3-Flash                    | $60  | $180    | $65      |
| GLM-5.3                          | $15  | $120    | $20      |
| LongCat-2.0                      | $60  | $240    | $65      |
| MiMo-V2.6-Flash                  | $60  | $120    | $65      |
| MiniMax M3.1 Flash               | $20  | $80     | $25      |
| Qwen3.8 Flash                    | $30  | $90     | $35      |

Three structural observations that are easy to check on the live page:

1. **Go Plus is not a uniform multiplier.** Per model it is 2×
   (DeepSeek V4.1 Flash, MiMo-V2.6-Flash, Muse Spark), 3× (GLM-5.3-Flash,
   Qwen3.8 Flash), 4× (Claude, LongCat, GPT 6 Luna, MiniMax M3.1 Flash) or 8×
   (GLM-5.3). A single normalised pool would produce one pool number, not
   per-model multipliers that differ by 4× between models.
2. **Go Teams is defined per model, per member:** "Every member gets their own
   limits, $5/month higher than Go on every model" — and the table honours it
   exactly ($15→$20, $60→$65, $100→$105).
3. **The request tables agree with per-model windows.** For every model the
   weekly estimate is half the monthly one and the 5-hour estimate is a fifth of
   it (to rounding): e.g. GLM-5.3-Flash 6,320 / 15,790 / 31,580; DeepSeek V4.1
   Flash 43,330 / 108,330 / 216,670.

What the page does **not** contain: any sentence saying the budgets are
independent, and any sentence saying they are pooled. The phrase "those
allowances" in the third sentence has no antecedent other than the per-model
limits just mentioned, which makes it circular as written — it is the surviving
half of wording written for a pool (see §3).

---

## 2. What the docs establish and what they do not

**Establish (unambiguous):**

- Each model has a monthly dollar limit (the table).
- That model's windows are 20% / 50% / 100% of its own monthly limit.
- Go Plus raises each model's limit; Go Teams adds $5 per model per member.
- Each response reports the amount it counted (`opencode.allowance`).

**Do not establish:**

- Whether one model's window exhaustion affects other models. Neither reading is
  stated or excluded by the current text.

Two arguments that look decisive but are not:

- *"The console displays per-model allowances, so there is no pool."* The old
  shared-pool documentation coexisted with an identical per-model table. Display
  is not enforcement.
- *"Estimated requests are per model."* Under the old normalised pool, a single
  model used alone still hits 20/50/100% of **its own** limit at exactly the same
  request counts, because the pool conversion is defined from those limits. The
  tables are consistent with both schemes.

The only clean discriminator is behaviour: exhaust one model's window and call
another in the same window.

---

## 3. Documentation history (public commits)

The docs lived in the public repository `anomalyco/opencode` under
`packages/web/src/content/docs/go.mdx` until 2026-10-12.

### 3.1 V1: an explicit pool (until 2026-09-28)

File at `083ed266e~1`, section "Usage limits" (verbatim):

> For example, if a model has a $60 monthly limit on Go and a $120 monthly limit
> on Go Plus, you can spend up to:
> 
> - **5-hour limit** — $12 on Go or $24 on Go Plus
> - **Weekly limit** — $30 on Go or $60 on Go Plus
> - **Monthly limit** — $60 on Go or $120 on Go Plus
> 
> Across models, Go has $12 five-hour, $30 weekly, and $60 monthly allowances;
> Go Plus has $48 five-hour, $120 weekly, and $240 monthly allowances. Each
> model's monthly limit below determines how its usage counts toward those
> allowances.

Read literally, this is a **single pool sized $12 / $30 / $60** whose depletion
rate depends on which model you use. Because a model's own windows are 20/50/100%
of its own limit, using any one model to its own 5-hour limit consumes the whole
pool: $3 of a $15 model weighs 4×, $12 of a $60 model weighs 1×, both equal the
$12 pool. That is precisely the "you finish one model's quota, you are finished
for all models" behaviour reported by users of that period.

### 3.2 2026-09-28 — commit `083ed266e`, "docs(go): simplify usage limit explanation"

The commit removes the worked example and the pool sentence, and keeps the
determinative sentence as a standalone line. English diff (abridged):

```diff
-For example, if a model has a $60 monthly limit on Go and a $120 monthly limit
-on Go Plus, you can spend up to:
-
-- **5-hour limit** — $12 on Go or $24 on Go Plus
-- **Weekly limit** — $30 on Go or $60 on Go Plus
-- **Monthly limit** — $60 on Go or $120 on Go Plus
-
-Across models, Go has $12 five-hour, $30 weekly, and $60 monthly allowances;
-Go Plus has $48 five-hour, $120 weekly, and $240 monthly allowances. Each
-model's monthly limit below determines how its usage counts toward those
-allowances.
+Each model's monthly limit below determines how its usage counts toward those allowances.
```

Before this commit the claimed pool was explicit; after it, only the circular
sentence remains. No commit message or follow-up states that enforcement changed
— only that the explanation was simplified.

### 3.3 2026-10-12 — commits `337e933d4` and `25053bbd`

- `337e933d4` (08:26, "docs(go): raise DeepSeek V4.1 Flash allowances"): the
  DeepSeek V4.1 Flash limit changes $60 → $80 on Go and $120 → $160 on Plus
  in the Markdown page.
- `25053bbd` (09:30, "docs: replace old Zen and Go pages with V2 console docs
  (#54761)", 110 files): `packages/web/src/content/docs/go.mdx` is **removed**
  (status `removed`, 519 deletions, 0 additions). The Go page is from then on
  rendered by the V2 console docs. The live page carries $100 / $200 / $105,
  i.e. both the $80 bump and a further raise happened in or after the V2
  rewrite.

### 3.4 What survives today

The third sentence of §1 is the only remnant of §3.1. Everything that made the
pool explicit was deleted; nothing that would make independence explicit was
added. The current page therefore **cannot be cited** to prove either scheme.

---

## 4. The logged experiment (2026-10-29)

### 4.1 Data source and method

- Store: `~/.local/share/opencode/opencode.db`, table `session_message` (the V2
  store; the older `message`/`part` tables stop on 2026-09-27). Assistant rows
  carry `model.id`, `model.providerID`, `tokens`, `cost`, `error` and
  `time_created` (epoch ms).
- Clock: local time, Europe/Paris (UTC+1 on 2026-10-29).
- No network and no API key are needed to read the local store.

Rate model used to recompute costs from tokens (Go prices, from the docs):

| Model                                | Input   | Output  | Reasoning | Cache read | Cache write |
| ------------------------------------ | ------- | ------- | --------- | ---------- | ----------- |
| Claude Haiku 5.5, > 100K tokens tier | $0.50/M | $2.50/M | $2.50/M   | $0.05/M    | $0.625/M    |
| DeepSeek V4.1 Flash, off-peak        | $0.15/M | $0.60/M | $0.60/M   | $0.003/M   | —           |

Verification that the recorded `cost` field uses exactly these rates: a
least-squares fit of the 34 priced Claude messages against
`(input, output, reasoning, cache read, cache write)` returns
`0.5175, 2.50, 2.50, 0.05, 0.625` with zero residual (the input coefficient is
noisy because those messages carry 1,160 input tokens in total). The same fit on
the 113 priced DeepSeek messages returns `0.15, 0.60, 0.60, 0.003, 0`. The cost
field is therefore the Go tariff at the >100K tier / off-peak rate, not a
different accounting unit.

### 4.2 What happened, minute by minute

| Local time   | Model                      | Events                                                                                                |
| ------------ | -------------------------- | ----------------------------------------------------------------------------------------------------- |
| 07:12–14:25  | Muse Spark 1.3 Contributor | 388 responses earlier in the day, 25 of them between 14:21 and 14:25, i.e. inside the window below    |
| 13:53:28     | Claude Haiku 5.5           | first response of the block (session switched from Muse at 13:53:28)                                  |
| 13:53–14:49  | Claude                     | 38 responses, recomputed cost **$2.9871**, no `cost` recorded                                         |
| **14:49:21** | Claude                     | cumulative cost **crosses $3.00** ($2.9871 → $3.0260); from this response on, `cost` is recorded      |
| 14:49–15:25  | Claude                     | 34 responses, recorded cost **$3.0111**                                                               |
| 15:25:34     | Claude                     | last response; day total **$5.9982**                                                                  |
| **15:37:00** | **DeepSeek V4.1 Flash**    | first response after the switch — 11m26s after the last Claude response, 47m39s after the $3 crossing |
| 15:37–18:50  | DeepSeek                   | **239 responses, $0.4114** recomputed at off-peak rates, provider `opencode-go`, **no errors**        |
| 18:53:28     | —                          | end of the 5-hour window that opened with the 13:53:28 response                                       |
| 18:54–22:32  | DeepSeek                   | 148 further responses, $0.3816, no errors                                                             |

Allowance arithmetic (docs rule, 20% of the monthly limit):

- Claude Haiku 5.5: monthly $15 → **5-hour allowance $3.00**. Crossed at
  14:49:21; the session then continued to ~2× that amount. That is consistent
  with (but not proof of) Extra Usage — the fallback to the pay-as-you-go
  balance — being enabled, and the local `cost` field is populated for exactly
  the post-crossing responses.
- DeepSeek V4.1 Flash: monthly $100 → **5-hour allowance $20.00**. The 239
  responses cost $0.41, i.e. ~2% of it.
- Muse Spark 1.3 Contributor: monthly $60 → 5-hour allowance $12.00.

### 4.3 What this rules out, and what it does not

**Rules out the weighted single pool** described by the V1 docs (§3.1). Under
that scheme, $3.00 of Claude usage is 100% of the shared $12 five-hour
allowance, and every Go model must be refused until 18:53:28. Instead 239
DeepSeek responses were served from 15:37:00 to 18:50:00 inside that window,
with no error records anywhere in the day.

**Does not rule out** a hypothetical unweighted pool (plain dollar sum). If such
a pool of unknown size existed, $6.00 of Claude spend might still leave room.
That hypothesis has no support in any version of the docs, but this single
observation cannot exclude it.

**Conclusion:** on 2026-10-29, OpenCode Go enforced the 5-hour limit **per
model**. The docs alone do not say so; the log does.

### 4.4 Caveats

1. The local store has **72** Claude responses that day; the console reports
   **75 requests** for the same model and period. Three requests are
   unaccounted for — retries, or another client using the same Go key (the docs
   list Claude Code as a validated client). Nothing here depends on those three.
2. Only 34 of the 72 Claude responses carry a `cost` field. All totals in §4.2
   were recomputed from tokens at the documented rates; the recomputed Claude
   total ($5.9982) matches the console's `CostUSD` ($6.00) to 0.03%.
3. Whether `cost` means "allowance counted" or "amount billed" is not
   documented. The totals do not depend on that distinction. The fact that the
   field switches on exactly at the $3.00 crossing favours the "billed outside
   the allowance" reading, but a client-side pricing quirk cannot be fully
   excluded.
4. Window semantics (fixed vs rolling) are unknown. The conclusion holds either
   way: 15:37:00 is less than 5 hours after both the 13:53:28 start and the
   14:49:21 crossing.
5. One account, one day, one model pair — a natural experiment, not a controlled
   one. §10 lists the observations that would falsify it.

---

## 5. Console cross-check (2026-10-30 snapshot)

Source: the tool's own cache of `console.opencode.ai/api/usage/models`
(`~/.cache/opencodable/console-usage.json`, fetched 2026-10-30T20:09+01:00).
Rows are per model and provider; the API has no time slicing, so it corroborates
totals only, not windows.

| Provider    | Model                        | Requests | CostUSD |
| ----------- | ---------------------------- | -------- | ------- |
| opencode-go | DeepSeek V4.1 Flash          | 7,892    | $19.90  |
| opencode-go | GLM-5.3-Flash                | 2,173    | $12.86  |
| opencode-go | GLM-5.2                      | 192      | $10.26  |
| opencode-go | GPT 5.6 Luna                 | 719      | $7.43   |
| opencode-go | Claude Haiku 5.5             | 75       | $6.00   |
| opencode-go | Gemini 4 Argon               | 41       | $3.61   |
| opencode-go | DeepSeek V4 Flash            | 1,426    | $3.58   |
| opencode    | GPT 5.6 Luna (pay-as-you-go) | 124      | $5.04   |
| opencode    | free models (several)        | —        | $0.00   |

Two useful facts: (a) the Claude row's $6.00 equals the token-recomputed total
for the 2026-10-29 session, which validates the rate table; (b) the only
pay-as-you-go spend in the period is GPT 5.6 Luna, which the local store does not
place on 2026-10-29 (last local use 2026-09-12) — so it is not evidence about
this experiment either way.

---

## 6. Claims in circulation, and their status

| Claim                                                                                              | Source                                       | Status                                                                                                                                 |
| -------------------------------------------------------------------------------------------------- | -------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| "You only have one quota; once you finish a model, you are finished for all models in the period." | r/opencode discussion, Oct 2026              | Matches the **pre-2026-09-28 docs** (§3.1); contradicted by the 2026-10-29 record for the current regime                               |
| "You have a total to spend across all models, with multipliers depending on the model."            | same thread                                  | Literal description of the V1 working; the V1 wording supported it                                                                     |
| Windows are per model, "not a shared $12/$30/$60 for every model"                                  | Medium guide, Sep 2026                       | Correct in direction; its request figures are stale (V4.1 Flash 6,500/5h vs 43,330 in the current docs), so cite it for direction only |
| Per-model allowance table, "Do not sum across models"                                              | StackReplay plan summary, checked 2026-09-29 | Consistent with per-model; third party, not authoritative                                                                              |

The recurring confusion has a concrete cause: the current page and the
pre-September page carry the **same three-sentence core**, and only the pool
clarification was deleted. Readers who saw the old page, and readers who see the
new one, can both quote "the docs" and mean different things.

---

## 7. Things this document deliberately does not claim

- It does not claim the current docs prove independence.
- It does not claim the 2026-10-29 result generalises to other models, other
  plans (Plus, Teams) or later server versions. It is one observation, dated.
- It does not claim the pay-as-you-go fallback was active; that is an inference
  from the ~2× allowance spend, stated as such in §4.2.

---

## 8. Implications for this tool

`opencodable` computes `-now` windows per model: 5-hour = 20% of the model's
monthly allowance, weekly = 50% (`internal/goplan` × `internal/usage`). The
2026-10-29 record supports that model for the Go plan.

Because the docs alone are ambiguous (§2), the README sentence "Budgets are not
pooled across models" should be read as "per the current docs and the logged
2026-10-29 test", not as a quotation of the docs. README and TITLES.md now point
here for that evidence, and README notes that console spend (`spent_mo`) can
include extra-usage traffic when the pay-as-you-go fallback is enabled.

The console's `CostUSD` matched the local token-based total for Claude to 0.03%,
which confirms that pricing console tokens at Go rates (as `-now` does) is the
right conversion.

---

## 9. Reproduction

### 9.1 Docs and commits

```sh
# current text
curl -s https://opencode.ai/v2/docs/console/go/ | sed -n '/Usage limits/,/Token prices/p'

# pre-2026-09-28 wording (the pool); acb6859 is the parent of 083ed266e
curl -s https://raw.githubusercontent.com/anomalyco/opencode/acb6859b06e421c6852a3ce19b0ef76443d258b5/packages/web/src/content/docs/go.mdx \
  | sed -n '/^## Usage limits/,/Token prices/p'

# the commit that removed it
curl -s https://api.github.com/repos/anomalyco/opencode/commits/083ed266e \
  | jq -r '.files[] | select(.filename|endswith("go.mdx")) | .patch' | head -40

# the removal of the page
curl -s https://api.github.com/repos/anomalyco/opencode/commits/25053bbd \
  | jq '.files[] | select(.filename|endswith("go.mdx")) | {status, additions, deletions}'
```

### 9.2 The session data

```sql
-- replace the window with the day of interest (epoch milliseconds)
select datetime(time_created/1000, 'unixepoch', 'localtime') as t,
       json_extract(data,'$.model.providerID')   as provider,
       json_extract(data,'$.model.id')           as model,
       json_extract(data,'$.tokens.input')       as inp,
       json_extract(data,'$.tokens.output')      as outp,
       json_extract(data,'$.tokens.reasoning')   as reasoning,
       json_extract(data,'$.tokens.cache.read')  as cache_read,
       json_extract(data,'$.tokens.cache.write') as cache_write,
       json_extract(data,'$.cost')               as cost,
       json_extract(data,'$.error')              as error
from session_message
where type = 'assistant'
  and time_created between 1793228400000 and 1793314799999   -- 2026-10-29 00:00-24:00 local
order by time_created;
```

```python
# recompute cost from tokens at the documented Go rates
rates = {  # $/1M: input, output(+reasoning), cache read, cache write
    'claude-haiku-5-5':   (0.50, 2.50, 0.05, 0.625),
    'deepseek-v4.1-flash': (0.15, 0.60, 0.003, 0.0),
}
def cost(row):
    i, o, cr, cw = rates[row['model']['id']]
    t = row['tokens']; c = t.get('cache', {})
    return (t.get('input',0)*i + (t.get('output',0)+t.get('reasoning',0))*o
            + c.get('read',0)*cr + c.get('write',0)*cw) / 1e6
```

Expected numbers for 2026-10-29: Claude 72 responses, $5.9982; the $3.00 crossing
at 14:49:21; DeepSeek 239 responses inside 13:53:28–18:53:28 costing $0.4114;
zero error rows.

---

## 10. Falsification

The conclusion in §4.3 fails if any of the following is observed:

1. **Live test:** with one $15 model's 5-hour allowance exhausted (its own 20% =
   $3), a request to a different model in the same 5-hour window returns a
   limit/429 error instead of being served.
2. **Log evidence:** any session where exhausting model A's window is followed,
   inside the same window, by a refused request to model B.
3. **Documentation:** an OpenCode statement that the windows are pooled, or a
   later doc revision that restores the removed "Across models …" sentence.

Conversely, the per-model reading is confirmed by repeating the 2026-10-29
pattern: cross the 20% line on one model, then serve another model inside the
same window.

---

## 11. Sources

**Documentation**

- Current Go docs: <https://opencode.ai/v2/docs/console/go/> and
  <https://opencode.ai/docs/go/> (fetched 2026-11-01).
- Frozen snapshot used by this tool's weekly live refresh:
  `internal/goplan/testdata/go-docs.html` (compare its "Usage limits" section
  with the live page).

**Public commit history** — `anomalyco/opencode`,
`packages/web/src/content/docs/go.mdx`

- `083ed266e` — 2026-09-28, "docs(go): simplify usage limit explanation"
  (removes the "Across models … $12 / $30 / $60" paragraph).
- `337e933d4` — 2026-10-12 08:26, "docs(go): raise DeepSeek V4.1 Flash
  allowances" ($60 → $80, Plus $120 → $160).
- `25053bbd` — 2026-10-12 09:30, "docs: replace old Zen and Go pages with V2
  console docs (#54761)" (`go.mdx` removed, 519 deletions).

**Local data**

- `~/.local/share/opencode/opencode.db`, table `session_message`
  (2026-07-07 → 2026-11-01).
- `~/.cache/opencodable/console-usage.json` (console snapshot,
  2026-10-30T20:09+01:00).

**Third-party, directional only**

- Medium, "OpenCode Go … complete guide to SOTA model routing", Sep 2026.
- StackReplay, OpenCode Go plan summary, checked 2026-09-29.
