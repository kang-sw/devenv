---
title: "dashboard 설정 패널: 확장된 config.list 스키마 기반 하네스별 설정 편집기"
related:
  261116-feat-ws-dashboard-generalized-mentions: "sibling undesigned dashboard todo from the same day; same decision-queue handling"
---

# Dashboard settings panel: per-harness config editor driven by an enriched config.list schema

## Background

Owner idea (2026-11-16 21:58 KST, verbatim):

> 지금 config.list 자체가 뭔가 스키마를 반환하잖아요? 좌하단 설정 패널에서 이걸 바탕으로 사용자가 직접 각 하네스별로 조정할 수 있는 UI를 만들 여지가 있을까요?

Owner decision (1199)=a: capture this as a todo ticket, link the mockup as the reference, and include schema enrichment (types, defaults, per-scope values, translations) and the open decisions.

Visual reference (approved as a reference direction, not final): `ai-docs/assets/mockups/261116/config-settings-ui/` (`mockup.html`, `mockup-light.png`, `mockup-dark.png`). The mockup is linked, not copied. Its labels and groups are hand-made, and it contains its own findings (G1-G10) and questions (Q1-Q8), summarized below.

Code facts re-verified at drafting time (line numbers drift; re-check before use):

- No daemon config route exists: `Daemon.Handler` in `agents-plugin-tool/internal/wsdashboard/daemon.go` (starts at line 729) registers health, rail, hire, tracker, agents, holders and similar routes only, and no file in `wsdashboard` imports `wsconfig`.
- Config logic lives in the MCP server: `tuningField` (`agents-plugin-tool/internal/mcp/server.go`, around line 2842) carries only name, description, enum, required; `buildTuningCatalog` is at about line 2864; the per-key registry and harness-bucket resolution are in `agents-plugin-tool/internal/mcp/config_registry.go` (`resolveHarnessBucketed`, about lines 160-185).
- `TrustedWrite` (`agents-plugin-tool/internal/wsdashboard/access.go`, about line 168) is the existing gate for writes with execution authority (local browser plus same-origin).

## Constraints

- Read before editing, per AGENTS.md `### Implementation Conventions`:
  - `ai-docs/manuals/release-compatibility.md` (touches `agents-plugin-tool/internal/`).
  - `ai-docs/manuals/shipped-surface-boundary.md` (touches `agents-plugin-tool/`).
  - `ai-docs/manuals/ws-mcp.md` (touches `agents-plugin-tool/internal/mcp/`, where config.list lives).
  - If rsrc, skills, or convention docs change: `ai-docs/manuals/skill-authoring.md` and `ai-docs/manuals/wsflow-mirroring.md`.
- Compatibility tier: new `config.list` schema fields and any new daemon config route are cross-version surfaces, so their tier must be settled and recorded in `## Decisions` before release (AGENTS.md `### Compatibility Tiers`). Non-authoritative proposal: additive optional fields are T0 (an old binary ignores unknown additions); the tier of a new daemon route and of any shared-package move must be verified with cited read/write sites, not assumed. A daemon write path must reuse wsconfig's existing write path; a whole-file rewrite would drop unknown fields, which is not T2 without a recovery path.
- Verification must include a side-by-side comparison of a result capture (light and dark) against the mockup at the path above.

## Open Questions

None of these is decided. Gaps G1-G10 and questions Q1-Q8 are the mockup's own (see its sections for detail).

Schema gaps:

- G1. Value type and range are only prose for built-in knobs; adapter manifests carry type/minimum/maximum/pattern but only the default survives into the catalog.
- G2. Built-in knobs have no default field.
- G3. No short label, group, or translations (the dashboard is localized in 11 languages).
- G4. Per-harness current values are incomplete: bucketed knobs report only the calling session's harness.
- G5. Only the winning value is reported, not the per-scope stack.
- G6. The catalog depends on the caller's package manifest; a harness-agnostic daemon needs the union of installed manifests, and defaults differ per package.
- G7. Selector enums are inconsistent (`*`, `default`, `all`) and `repo_scope` interacts with the scope enum.
- G8. No apply-timing field (next manual load, session restart, daemon restart).
- G9. No seed text for prompt overrides.
- G10. `writer.tool = "config.tune"` is an MCP contract; the browser needs an HTTP route taking the same argument shape.

Product and design questions:

- Q1. Default save scope: global, or the workspace's project scope?
- Q2. Allow writing the committed repo scope from the dashboard?
- Q3. Keep session scope out of this sheet and in the per-session menu?
- Q4. Worktree and agent profiles (hooks, tool permissions): read-only, or allow JSON edit?
- Q5. Who owns label/description translation: the config.list schema or the dashboard i18n table?
- Q6. Show Pi adapter settings without a Claude session (needs the manifest union, G6)?
- Q7. Does the prompt-override editor need seed view and diff (G9)?
- Q8. Entry point: widen the settings popover into a sheet, or a separate `/settings` page?

Also raised by the mockup: a Claude-bucket global value outranks a common-bucket project value, so provenance badges must show the bucket; saving from the dashboard adds a write path outside config.tune's lead-session-key gate, so it must be bound by `TrustedWrite`; and the logic would need to move from MCP server methods into a shared package for the daemon to call (proxying through a live session was judged unsuitable).

## Phases

### Phase 1: undesigned

Not yet designed. Design settles through this ticket's Open Decision Queue before promotion to `ready/`: answer Q1-Q8, choose which of G1-G10 enter the first phase, settle the compatibility tier of each new surface, and then replace this placeholder with a concrete phase plan whose verification includes the side-by-side capture comparison above.
