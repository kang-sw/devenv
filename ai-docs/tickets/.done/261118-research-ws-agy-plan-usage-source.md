---
title: "agy plan-usage source for the dashboard usage gutter"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-hosted-antigravity-adapter: "mapped agy's context usage (usage_update) but no plan or rate-limit usage"
  261118-feat-ws-agy-tool-names-and-worker-role: "agy worker role; more agy usage makes the plan quota visible-worthy"
completed: 2026-11-20
---

# agy plan-usage source for the dashboard usage gutter

## Background

The dashboard's plan-usage gutter (`agents-plugin-tool/internal/wsdashboard/plan_usage.go`)
tracks Claude and GPT (Codex/Pi) windows only; its store holds `claude` and
`gpt` entries. For agy, ws maps only the context window: `usage_update
{used, size}` becomes the status context percentage
(`agents-plugin-tool/internal/wsagent/agy.go`, the `usage_update` case near
L901). Whether agy exposes plan quota or rate-limit state at all has not been
checked, and no ticket covered it. Owner (161)=a, 2026-11-18: capture this as
a research ticket and run it after the agy worker role lands, through Pi.

## Questions

1. Does agy expose plan quota or rate-limit usage through any surface: an
   `agy` CLI subcommand, an ACP notification or `_meta` field, or a local
   state file under the user's agy home? Record the shape (window kinds,
   percent or counts, reset time) and the agy version.
2. Is that source cheap enough for the gutter's refresh path under
   `ai-docs/manuals/performance.md` (no per-render file I/O or process spawn)?
3. Which gutter entry does it map to: a new `agy` harness entry or an
   existing one, and what does an old daemon do with it (compatibility tier
   per `ai-docs/manuals/release-compatibility.md`)?

## Constraints

- Do not log in, log out, switch accounts, or change agy credentials; keep
  account details (plan type, real usage values) out of tracked files and
  fixtures.

## Outcome

Superseded by `261119-feat-ws-agy-plan-usage-display` (now `.done/`), which
captures the live `agy -p /usage --output-format json` probe, Orca prior art,
and owner-confirmed decisions resolving `## Questions`; it shipped to
`develop`.

- Q1: agy exposes plan quota through the CLI `/usage` prompt (JSON output: two
  model groups, each with 5-hour and weekly buckets carrying
  `remaining_fraction` and `reset_time`); ACP carries only the context window.
- Q2: not cheap enough for a refresh or render path; the shipped design spawns
  the probe only on user action (popover open or refresh) with a 10-minute
  debounce and billed/auth latches.
- Q3: two new `agy` gutter entries, one per model group, persisted in separate
  `plan-usage-agy.json` / `plan-usage-agy-history.json` files (T0: an old
  daemon never reads them).

## Outcome Ledger

### Verified Findings

- agy's ACP stream carries `usage_update {used, size}` for the context window
  only (`261117-feat-ws-hosted-antigravity-adapter`, capability `usage_query`).

### Confirmed Decisions

- (161)=a, 2026-11-18: research first, run after the agy worker role lands,
  preferring a Pi agent.

### Proposals

### Open Questions

- None: Questions 1-3 are answered in `## Outcome`.

### Rejected Alternatives


## Resolution (2026-11-20)

Answered by 261119-feat-ws-agy-plan-usage-display, merged to develop; see `## Outcome`.
