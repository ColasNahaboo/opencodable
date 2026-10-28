# History

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
