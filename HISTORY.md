# History

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
