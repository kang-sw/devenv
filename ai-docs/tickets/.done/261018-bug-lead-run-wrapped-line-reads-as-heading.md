---
title: "lead-run playbook: a wrapped line starting with ### reads as a heading"
related:
  261018-feat-claude-mod-ws-manual-injection: found while dogfooding its repeat-read pointer
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: e81fc648d8cbe482
sage-review-completeness-reviewed: e81fc648d8cbe482
completed: 2026-11-15
---

# lead-run playbook: a wrapped line starting with ### reads as a heading

## Background

`agents-plugin/rsrc/lead-run/lead-run.md` wraps an inline code span so that a
continuation line begins with `### `:

The excerpt below has display-only line-number prefixes:

```text
1 | `Resume: branch <branch> at <head>; your last report is in the ticket's
2 | ### Result and the branch log.`
```

A Markdown reader treats that line as an ATX heading (`### Result and the
branch log.``), splitting the paragraph and breaking the code span. It surfaced
live in the Claude mod's repeat-read pointer, whose heading list (the
`#`-`###` lines of the result, the same rule as the pi bridge) carried the
stray entry. The model reading the playbook sees the same malformed structure.

## Current Facts

- The quoted wrap remains at `agents-plugin/rsrc/lead-run/lead-run.md#L164-L167`; the wsflow copy at `agents-plugin-wsflow/rsrc/lead-run/lead-run.md` is byte-identical (checked with `cmp`).
- Both pointer extractors match `^#{1,3}\s+.+$` with multiline matching, so they include the continuation line: `agents-plugin/hooks/claude-mod/manual.ts#L133-L136`, `agents-plugin-pi/src/playbook-read-dedupe.ts#L72-L75`. The historical dogfood observation is recorded in the related ticket's Result (8d7382f), not a new live reproduction.
- Existing rsrc checks include `TestShippedManifestUpToDate` (`agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go#L18-L44`), `TestWsflowRsrcMirrorUpToDate` (`agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L46-L79`), and `TestPiMirrorUpToDate` (`agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go#L20-L82`). The last also covers `agents-plugin-pi/rsrc/lead-run/lead-run.md`, currently byte-identical to canonical (checked with `cmp`); maintaining that copy is existing mirror maintenance, not a new playbook contract. These checks cover hashes and mirror drift, not malformed inline-code wrapping (search: `heading|Heading|inline` in `agents-plugin-tool/internal/wsrsrc/*_test.go`). No tests were run during fact population; the phase's cheap regression check remains conditional.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Decisions

- 261018-feat-claude-mod-ws-manual-injection (2026-10-18, Result (8d7382f)): "Found while dogfooding: a wrapped line in the lead-run playbook starts with `### ` and shows up in the pointer's heading list" — bearing: supports
- 260915-refactor-lead-run-playbook-diet-drop-assignment-note (2026-09-15, commit 0a1d5b5b): "Pins were kept only for structural fragments" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Canonical agents-plugin/rsrc/lead-run/lead-run.md and its wsflow/Pi mirrors; existing manifest and mirror maintenance |
| scope.surface | public-interface | Shipped lead-run playbook text; no tool API changes |
| scope.new_public_symbol | no | None; rewrap the existing resume sentence |
| scope.new_type_contract | no | None; same resume wording and workflow contract |
| scope.test_surface | existing | agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go, wsflow_mirror_test.go, pi_mirror_test.go; optional cheap check only if it fits |
| complexity.reuse_points | confirmed | Read existing manifest and byte-identity drift checks in internal/wsrsrc; wsflow regeneration entry point already exists |
| complexity.side_effect_risk | low | Physical rewrap only; sentence semantics and dispatch behavior remain unchanged |
| risk.correctness | low | Defect is the exact continuation at lead-run.md#L166-L167, repair preserves the sentence |
| risk.fit | low | Canonical text plus existing byte-identical mirror maintenance, no new workflow mechanism |
| risk.test | moderate | Existing checks detect hash/mirror drift but not the inline-code heading defect; regression check remains conditional |
| risk.security_or_contract | low | No credentials, permissions, tool signatures, or workflow semantics change |

## Phases

### Phase 1: Rewrap the line and guard against recurrence

Rewrap the sentence in `lead-run.md` (and its wsflow mirror) so no line starts
with `#`. Consider a cheap check in the existing rsrc tests that flags a line
starting with `#` inside a paragraph or inline code span, if one fits the
current test layout; otherwise the rewrap alone closes this.

### Result (72f722649) - 2026-11-15

- Rewrapped the resume example without changing any words or workflow semantics;
  its continuation no longer becomes an ATX heading. Regenerated the canonical
  manifest and maintained byte-identical wsflow/Pi resource copies.
- Added a focused shipped-resource regression for heading-like continuation lines
  inside that example; no general Markdown parser or extractor logic changed.
- Offline scoped verification passed: `TestLeadRunResumeExampleHasNoHeading`,
  `TestShippedManifestUpToDate`, `TestWsflowRsrcMirrorUpToDate`, and
  `TestPiMirrorUpToDate`. Whitespace-token comparison confirmed unchanged wording;
  actual heading extraction removed only the accidental heading. Diff checks passed.
- The wsflow package run failed at the unrelated pre-existing runtime contract:
  `test_wsflow_runtime_contract.py:95` reports capabilities containing
  `session.request_hire` and `session.roster` absent from `runtime.json`.
  It was not fixed or rerun. Full failure output and command/environment evidence:
  `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@128075b7/review-paths/07e5b2d4-01-lead-run-heading-verification.md`.
- The already-dispatched one-pass Lite review found no findings; no further review
  ran. Lead approved closure after conflict-free develop synchronization at
  `54372070adf557c92274ec7967f2a8a0d1d5bd93`. Incoming changes did not touch
  resource, test, manifest, dependency, or build inputs covered by the scoped
  checks, so their passing evidence was retained without a redundant rerun.
- Omitted: full-suite/build, further package/runtime checks, browser/provider
  execution, and broader Markdown policy; none is needed for this physical rewrap.
