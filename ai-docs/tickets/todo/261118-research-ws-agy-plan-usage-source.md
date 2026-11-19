---
title: "agy plan-usage source for the dashboard usage gutter"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-hosted-antigravity-adapter: "mapped agy's context usage (usage_update) but no plan or rate-limit usage"
  261118-feat-ws-agy-tool-names-and-worker-role: "agy worker role; more agy usage makes the plan quota visible-worthy"
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

Superseded by `261119-feat-ws-agy-plan-usage-display` (`todo/`), which captures
the live `agy -p /usage --output-format json` probe, Orca prior art, and
owner-confirmed decisions (D1–D5) resolving `## Questions`.

## Outcome Ledger

### Verified Findings

- agy's ACP stream carries `usage_update {used, size}` for the context window
  only (`261117-feat-ws-hosted-antigravity-adapter`, capability `usage_query`).

### Confirmed Decisions

- (161)=a, 2026-11-18: research first, run after the agy worker role lands,
  preferring a Pi agent.

### Proposals

### Open Questions

- Questions 1-3 above.

### Rejected Alternatives
