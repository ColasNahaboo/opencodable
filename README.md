# opencodable

![banner](.github/assets/banner.png)

**Find the best compromise of price, speed and quality among the models of your
OpenCode subscription — right now, in your terminal.**

[MIT licensed](LICENSE) · [releases](https://github.com/ColasNahaboo/opencodable/releases) · single Go program · no dependencies · Go 1.26+

---

## What it does

An OpenCode Go subscription does not work like a pay-per-token account. Each
model gets its own **monthly dollar budget**, and every request spends part of
it. The cheapest-looking model is therefore not always the one that lets you get
the most done.

`opencodable` puts the numbers side by side, in one table:

- what a typical coding request costs on each model, and how many requests your budget covers;
- how smart and how fast each model is, from public benchmarks and from measurements on your own subscription;
- how much of your budget you have used, and which models you can still use right now;
- whether a model may train on your prompts.

In a terminal you get a readable table. If you write scripts, the same data
comes out as tab-separated lines or as JSON (see [Output for scripts](#output-for-scripts)).

## Quick start

**1. Install.** You need Go 1.26 or newer:

```sh
go install github.com/ColasNahaboo/opencodable/cmd/opencodable@master
```

The program lands in `$(go env GOPATH)/bin`, so make sure that folder is on your `PATH`.
Without Go, download the archive for your system from the
[releases page](https://github.com/ColasNahaboo/opencodable/releases) and put the
program somewhere on your `PATH`.

**2. Add your keys.** This is optional, but recommended. [Configuration](#configuration) says where to put them.

- Your **OpenCode Go key** unlocks what you can use right now (`-now`, `-full`), your remaining allowance (`-usage`) and speed measurements (`-probe`).
- A **free Artificial Analysis key** (from artificialanalysis.ai) unlocks the quality and benchmark columns.

Without any key you still get prices, budgets and cost per request.

**3. Run it.**

```sh
opencodable
```

Models are listed best quality first. Here is an excerpt, with a few columns
selected by `-cols` so that it fits on the page:

```sh
opencodable -paid -privacy -cols name,cost_task,r_quota,p_tok_s,aa_int,aa_code,pareto
```

```
#name            cost_task  r_quota   p_tok_s  aa_int  aa_code  pareto
Kimi K3          $0.0306    490.2     595.8    -       76.2     yes
GLM-5.3-Flash    $0.0019    31578.9   113.2    -       71.5     yes
GPT-5.6 Luna     $0.00146   10245.9   126.0    -       71.4     yes
GLM-5.2          $0.01516   3957.8    78.9     -       68.8
Kimi K2.7 Code   $0.01208   4968.3    41.2     -       60.8
MiMo V2.5 Pro    $0.00092   16291.1   -        -       60.2     yes
```

A `-` means there is no data for that cell. Some models have no scores yet in
the Artificial Analysis data.

## Which command answers my question?

| I want to know… | Run |
|---|---|
| which models are the best compromise (cost, speed and quality together) | `opencodable -pareto -paid` |
| which model is the smartest | `opencodable` (the default order) |
| which models cost the least per request | `opencodable -sort price` |
| which models give me the most requests a month | `opencodable -sort reqs` |
| which models I can use right now, and how many requests | `opencodable -now` |
| how much of my allowance is left | `opencodable -usage` |
| everything in one wide table | `opencodable -full` (add `-cols` to choose columns) |
| which models do not train on my prompts | `opencodable -privacy` |
| which models are fast enough for me | `opencodable -min-speed 60` |
| how fast they really are on my subscription | `opencodable -probe` (about a cent per model) |
| the same data, for my own script | `opencodable -json`, or the piped output |

## Reading the table

Each row is one model. Column titles are short. These are the ones most people
look at first; the [Columns](#columns) section lists them all.

- **`cost_task`**: estimated cost of one typical coding-agent request, in dollars, based on OpenCode's own figures.
- **`r_quota`**: roughly how many such requests your monthly budget covers for that model.
- **`aa_int`** and **`aa_code`**: overall intelligence and coding scores, from 0 to 100 (Artificial Analysis). Higher is better.
- **`p_tok_s`**: output speed measured on your own subscription, in tokens per second. Run `-probe` to fill it in. `tok_s` is the speed reported for other hosts.
- **`pareto`**: `yes` when no other visible model is at least as cheap, as fast and as good, and better in at least one way. These are the strong compromises.
- **`privacy`**: `safe` (the provider does not train on your requests), `trains` (it may), or `unknown` (not in our data).

### Key words

- **Token**: a chunk of text that a model reads or writes, roughly a word or part of one. Prices are quoted per million tokens; `cost_tok` is the retail output price.
- **Allowance**: your monthly dollar budget for one model. Each request uses part of it.
- **Window**: a model's budget also comes in a 5-hour window (20% of its monthly budget) and a weekly window (50%). A burst of work can use up a window before the month runs out.
- **Probe**: a short test request, sent through your subscription, that measures real speed. It costs about one cent per model.
- **Peak hours**: DeepSeek models cost more at certain hours. The table shows a second row for them, named `[peak]`, unless you use `-peak`.
- **Arena (`elo`, `btr`)**: the public Arena leaderboard (formerly LMArena), where people compare answers without knowing which model wrote them. A higher rating means people preferred that model more often. Typical values are 1400 to 1550.

## Your allowance

OpenCode Go budgets are **per model**. Each model has its own monthly budget
(for example $15 for Kimi K3.1 Pro on the Go plan, $60 for GLM-5.3-Flash), and
its 5-hour and weekly windows are shares of that budget. Budgets are not pooled
across models.

Four views help:

- **`-now`**: for each model, how many requests fit in each window, given what you have already spent this month. Spending is fetched with your OpenCode key.
- **`-usage`**: the account-wide percentages OpenCode reports for the 5-hour, weekly and monthly windows, with their reset times.
- **`-full`**: the main table and `-now` in one wide table.
- **`-window-estimate`**: also takes out what you have probably used in the current 5-hour and weekly windows. OpenCode only reports the account total, so this is an estimate; the method appears on stderr and, in JSON, as `window_method`. Without this flag, the 5-hour and weekly counts assume fresh windows, which gives an upper bound.

The `-now` columns:

- **`burn_req`**: dollars one request really uses. Measured from your own usage once a model has at least 50 requests this month; before that, the official `cost_task`.
- **`spent_mo`**: dollars spent this month on the model (blank if unknown).
- **`reqs_5h`** and **`reqs_wk`**: requests that fit in the model's 5-hour and weekly windows.
- **`reqs_mo`**: requests left in the month after what you have spent.
- **`usable`**: the smallest of those three, that is, how many requests you can make right now.
- **`limit`**: the binding limit: `5h`, `week`, `month`, `unlimited` (free models) or `unknown` (not in our plan data).

```
#name                 cost_task  burn_req  spent_mo    reqs_5h  reqs_wk  reqs_mo  usable  limit  p_tok_s
MiMo-V2.6-Flash       $0.0004    $0.0006   $0.11189    19841    49602    99020    19841   5h     59.5
MiMo V2.5             $0.0004    $0.00096  $0.15188    12562    31406    62653    12562   5h     89.0
LongCat-2.0           $0.00105   $0.00105  $0.00077    11435    28587    57174    11435   5h     76.9
```

Without the OpenCode key, `-now` and `-full` still run, but they assume you have
spent nothing yet, and they say so on stderr. If you cannot use the key, paste
the "usage by model" table from the OpenCode console into a text file and pass
it with `-spent <file>`. A pasted table has no request counts, so the official
per-request cost is used.

`-usage` needs the network and your key. Live allowance data is cached for 10
minutes by default. `-ttl <seconds>` changes that (`0` turns the cache off), and
`-refresh` forces an update.

```
#window  status  used_pct  remaining_pct  resets_at                  resets_in
5h       ok      19        81             2026-10-27T21:43:22+01:00  53m 0s
week     ok      16        84             2026-11-02T01:00:00+01:00  124h 9m 0s
month    ok      21        79             2026-11-07T14:56:38+01:00  258h 6m 0s
```

## Configuration

### Keys

| Key | What it unlocks | Needed? |
|---|---|---|
| `OPENCODE_API_KEY`: your OpenCode Go key | `-now`, `-full`, `-usage`, `-window-estimate`, `-probe` | for those views |
| `AA_API_KEY`: free Artificial Analysis key | quality and benchmark columns | recommended |

Each key is looked up in this order: an environment variable, then
`opencodable.conf`, then a dot-file in the folder you run the command from
(`.opencode-go.key` or `.artificialanalysis.key`).

Without the OpenCode key, `-now` and `-full` assume zero spending (and say so),
while `-usage` and `-probe` stop with a message. Without the Artificial Analysis
key you get a warning, and the quality columns and the Artificial Analysis speed
columns stay empty.

### The configuration file

`opencodable.conf` is a plain text file with one `name=value` per line. Lines
starting with `#` are comments; a comment must be on its own line.

| OS | Location |
|---|---|
| Linux | `~/.config/opencodable/opencodable.conf` (or `$XDG_CONFIG_HOME/opencodable/`) |
| macOS | `~/Library/Application Support/opencodable/opencodable.conf` |
| Windows | `%AppData%\opencodable\opencodable.conf` |

Names in upper case are keys. Names in lower case are default options, so you
can keep your usual choices there. The command line always wins.

```ini
OPENCODE_API_KEY=your-key
AA_API_KEY=your-key

# Default options
plan=go-plus
paid=yes
privacy=on
probe-max-allowance=1

# Filters and columns
exclude=grok*
max-cost=0.02
min-quality=aa_int=35,aa_code=70
#cols=cost_task,p_tok_s,aa_int,swe_b
```

For a single run, `-config FILE` uses another file instead (it must exist; an
empty file drops every default). To turn a default off for one run, write
`-name=false`, for example `opencodable -paid=false`.

`opencodable -doctor` shows where each key was found (`env`, `config`, `dotfile`
or `none`), and warns about setting names it does not know, which are usually
typos.

## Options

Every option is optional, and `opencodable -h` lists them all with their
defaults. Every option except `-config` can also be set in the configuration
file, under its lower-case name.

### Views

Pick one at a time; combining two is an error. Without a view, the main table is shown.

- `-main`: the main table, with cost, budget and quality. This is the default.
- `-now`: what you can use right now, per model.
- `-full`: the main table and `-now` in one wide table.
- `-usage`: your remaining allowance in the 5-hour, weekly and monthly windows.
- `-doctor`: a coverage check: missing data per model, cache ages, where keys were found, typos in your configuration file. It runs no probes.

### What to show, and in what order

- `-sort <key>`: `quality` (the default: intelligence, falling back to the coding score), `price` (cheapest request first), `speed`, `reqs` (most requests a month first), `usable` (most usable right now, for `-full`), or `name`.
- `-pareto`: keep only the models that no other visible model beats on cost, speed and quality together.
- `-paid`: hide free models.
- `-privacy`: hide models that may train on your prompts, and models whose status is unknown.
- `-plan go|go-plus`: which plan's budgets to use. The default is `go`; Go Plus has larger budgets.
- `-provider <name>`: the default is `opencode-go`, the only provider with plan (budget) data.
- `-peak`: price DeepSeek at its peak-hour rates everywhere, without the separate `[peak]` rows.

### Filters

- `-exclude <patterns>`: hide models whose ID or name matches. Separate patterns with commas. Matching ignores case; `*` stands for any run of characters and `?` for one character. A plain word matches anywhere, so `-exclude grok` hides every Grok model.
- `-include <patterns>`: show only the models that match.
- `-max-cost <usd>`: hide models whose estimated cost per request is higher.
- `-min-speed <tok/s>`: hide slower models (measured speed when available, otherwise the reported one).
- `-min-quality <metric>=<score>,...`: hide models below a score, for example `aa_int=40,swe_b=30`; a model must meet every bound. The metric names are column titles. A bare number means the overall quality score.
- `-missing keep|drop`: what to do with a model that has no value for a filtered metric. `keep` (the default) keeps it; `drop` removes it.

Filters work in every view, with one exception: `-now` loads no benchmark data, so
it ignores the quality bounds and says so on stderr.

### Column flags

- `-cols <titles>`: show only these columns. `name` is always shown.
- `-excols <titles>`: hide these columns.
- `-l`: add a legend that explains each column.

### Output flags

- `-json`: machine-readable output. Keys are the column titles.
- `-nohead`: leave out the header row.
- `-vertical`: one model per block of `name: value` lines. Easier with a screen reader, and easy to search with `grep`.

### Allowance flags

- `-window-estimate`: estimate what you have used in the current windows (see [Your allowance](#your-allowance)).
- `-spent <file>`: use a pasted console table instead of fetching your spending.
- `-ttl <seconds>`: how long live allowance data stays cached (default 600; `0` turns caching off).
- `-refresh`: ignore the caches and fetch everything again.
- `-offline`: use only what is cached, with no network. `-usage` refuses this, and `-now` then assumes no spending unless you pass `-spent`.

### Measurement flags

- `-probe`: measure every model's speed on your subscription. This spends allowance. Results are kept for 24 hours.
- `-probe-max-allowance <usd>`: stop a `-probe` run once this much allowance has been spent.
- `-probe-model <id>`: measure one model and print the details as JSON.

### Settings

- `-config <file>`: use this configuration file for the run.
- `-name=false`: turn off a default that your configuration file sets to yes.

## Columns

`-json` keys are the column titles below, without the leading `#`. JSON adds
`id` (the model's catalog ID) and `unlimited` (true for free models). `-now`
adds `known`, `spent_known`, `window_estimate` and `window_method`. The exact
contract is in [TITLES.md](TITLES.md).

| Column | Meaning |
|---|---|
| `#name` | model name |
| `cost_tok` | retail price per million output tokens |
| `cost_task` | estimated cost of one typical coding-agent request |
| `allowance` | your monthly dollar budget for the model (`unlimited` for free models) |
| `r_quota` | roughly how many requests that budget covers each month |
| `tok_s`, `ttft_s` | output speed (tokens per second) and wait before the first word, as reported by Artificial Analysis for other hosts |
| `p_tok_s`, `p_ttft_s` | the same two, measured on your own subscription with `-probe` |
| `aa_int`, `aa_code`, `aa_math` | Artificial Analysis indexes, 0 to 100: overall intelligence, coding and maths |
| `code_b`, `term_b`, `if_b`, `aa_lcr` | pass rates in percent on LiveCodeBench, Terminal-Bench, IFBench and a long-context reasoning test |
| `swe_b` | percent of real GitHub issues solved in SWE-bench Verified (best recorded run) |
| `lb_agent`, `lb_code` | LiveBench scores, 0 to 100, for long agentic tasks and for coding |
| `elo`, `btr` | Arena ratings: `elo` from a daily snapshot of the Arena text leaderboard, `btr` from the official Arena dataset |
| `pareto` | `yes` when no other visible model beats it on cost, speed and quality together |
| `src` | `aa:<effort>` when the speed comes from a reasoning-effort variant of the model |
| `privacy` | `safe`, `trains` or `unknown` (see [Reading the table](#reading-the-table)) |
| `ctx_k`, `out_k` | context window and maximum output, in thousands of tokens |

`-now` adds `burn_req`, `spent_mo`, `reqs_5h`, `reqs_wk`, `reqs_mo`, `usable`
and `limit` (see [Your allowance](#your-allowance)). `-full` shows the main
columns followed by these, 33 in total, so use `-cols` to narrow it down. In
`-full`, `r_quota` is the full-month figure and `reqs_mo` is what remains.

Choose columns with `-cols` or `-excols` (both repeatable). The order always
follows the view. A column name that exists in another view is silently
ignored, so one preset in your configuration file works in every view. A name
that exists nowhere is an error.

## Output for scripts

- **In a terminal** you get an aligned table. **Piped or redirected**, you get tab-separated values: one row per model, one tab between fields, an empty field for missing data, and a header line that starts with `#name`.
- **JSON** with `-json`, for example `opencodable -json | jq -r '.[] | select(.pareto) | .name'`.
- **Two columns without a header:** `opencodable -pareto -paid -nohead -cols name,r_quota`.
- **Legends** are added only with `-l`, so parsing stays simple.
- Until the `v1.0.0` release, column titles may still change. [TITLES.md](TITLES.md) is the contract that applies after it.

## Measured speed (probes)

Speeds reported by public sites are measured on their own servers. `-probe`
measures your own subscription instead: it sends one short, fixed coding prompt
to each model and times the answer. A longer prompt is used for models that
answer too quickly to time reliably.

- **Cost**: about one cent per model. In our last run, 29 models cost about $0.34 in total. `-probe-max-allowance 0.50` stops a run once half a dollar has been spent.
- **Caching**: results are kept for 24 hours. `-refresh` measures again, which spends allowance again.
- **Trust**: a measurement that is too short to time, or implausibly fast, is marked `suspect` and left out of the speed rankings. `-probe-model` shows the raw numbers.
- **Thinking models** take time before their first visible word. `ttft_s` and `p_ttft_s` include that thinking time, so some models wait tens of seconds and then stream quickly.

`-probe-model` prints the full details of one measurement:

```json
{
  "model_id": "kimi-k3.1-pro",
  "tok_s": 78.67540953897064,
  "ttft_s": 6.602359389,
  "e2e_s": 7.911003628,
  "window_s": 5.567177884,
  "prompt_tokens": 104,
  "output_tokens": 438,
  "reasoning_tokens": 283,
  "cached_tokens": 0,
  "allowance_burned": 0.018352,
  "list_cost": 0.004588
}
```

## Things that surprise people

- **Price per request is not the whole story.** Budgets differ a lot between models. In the same table, `r_quota` ranged from a few hundred to a few hundred thousand requests a month. Look at `r_quota` and `usable` as well as the price.
- **Cheap can mean less private.** Contributor models (`trains`) are often much cheaper per request, because the provider may use your requests for training. `-privacy` hides them, along with models whose status is unknown.
- **Public speed numbers may not match yours.** Routing can differ by model and by time of day. `-probe` is the only way to know your own speed.

## Data sources

| Source | What it provides | Key |
|---|---|---|
| [models.dev](https://models.dev) | the model catalog: prices and context sizes | none |
| [OpenCode Go](https://opencode.ai/v2/docs/console/go/) | plan budgets, cost per request, peak prices, privacy | none (built into the program as a snapshot) |
| [Artificial Analysis](https://artificialanalysis.ai) | quality indexes, benchmark scores, reported speed | free key |
| [LiveBench](https://livebench.ai) | agentic and coding scores (pinned release) | none |
| [SWE-bench Verified](https://www.swebench.com/) | percent of GitHub issues solved | none |
| Arena text leaderboard, [daily snapshot](https://github.com/oolong-tea-2026/arena-ai-leaderboards) (third-party mirror, MIT) | `elo` | none |
| [Arena leaderboard dataset](https://huggingface.co/datasets/lmarena-ai/leaderboard-dataset) (CC-BY-4.0) | `btr` | none |
| OpenCode console and usage endpoints | your spending and allowance windows | your OpenCode Go key |
| Your subscription (probe) | your measured speed and allowance per request | your OpenCode Go key |

The plan data (budgets, peak prices and privacy flags) comes from OpenCode's
public documentation and is a snapshot, dated by `-doctor`. Check it when
OpenCode changes its plans.

## Keys and privacy

- Each key is sent only to the service it belongs to: the OpenCode key to OpenCode, the Artificial Analysis key to artificialanalysis.ai. Keys are never printed; `-doctor` only shows where each one was found.
- Fetched data and results are cached in your user cache folder (`~/.cache/opencodable` on Linux, `~/Library/Caches/opencodable` on macOS). On Linux and macOS, the folder is readable only by you.
- A probe sends one short coding prompt through your subscription, and nothing else of yours.

## How it works

- **Catalog and prices** come from models.dev, the same catalog OpenCode uses.
- **Your plan** comes from OpenCode's public Go plan data, which the program carries as a snapshot.
- **Benchmarks** come from the public sources listed above, matched to your models by name. A built-in alias table handles common naming differences, such as `mimo-v2.5` and `mimo-v2-5-0424`.
- **Caching**: public data is kept for 24 hours, and live allowance data for `-ttl` seconds. When a live refresh of allowance or spending data fails, the last cached copy is used, with a warning that gives its age.
- **Trust**: a speed measurement that is too short to time, or implausibly fast, is flagged rather than ranked. Estimates say which method produced them.
- **Dependencies**: the program uses only Go's standard library.

## Possible enhancements

Ideas under consideration — no promises, no timeline.

- Measured (not estimated) per-model 5h/weekly spend — blocked on the
  console API, which serves period totals only (verified Oct 2026: date
  params are ignored or rejected)
- Multi-provider comparison (`-provider any`)
- Recoding in Rust with same functionalities

## Contributing

The interesting places to contribute: `internal/normalize/normalize.go` (name
matching and the alias table), `internal/goplan/goplan.go` (subscription
snapshot — update when OpenCode changes plans), `internal/bench/` (new
benchmark feeds). Any output change must respect [TITLES.md](TITLES.md).

Before sending a change, `gofmt -l .`, `go vet ./...` and `go test ./... -count=1`
should all come back clean. The architecture and the project rules are in
[AGENTS.md](AGENTS.md).

## License

[MIT](LICENSE) © Colas Nahaboo 2026. https://github.com/ColasNahaboo/opencodable

## AI Co-Design Disclosure

![Co-Designed with AI](https://img.shields.io/badge/Co--Designed%20with-AI-blueviolet?style=flat-square&logo=openai)

This repository is developed by me, a human hobbyist in my personal time, acting as a project manager and tech lead of a team of various AI agents whose contributions are committed under their model name.

## History

Version v0.1.14 2026-10-29.
See [HISTORY.md](HISTORY.md).
