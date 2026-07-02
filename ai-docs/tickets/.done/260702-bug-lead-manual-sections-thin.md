---
title: lead-only workflow_manual sections are empty/thin, missing ferrule discipline
sage-review: completed
completed: 2026-07-02
---

# lead-only workflow_manual sections are empty/thin, missing ferrule discipline

## Context

Found during a v0.31.1 dogfooding pass. `ferrule`'s schema description
("Reserved workflow primitive. See wsflow:workflow-manual before use.") is
deliberately terse — this is intentional capability-gating that keeps
subagents from self-minting session keys. An earlier draft of this finding
proposed filling out ferrule's own schema description, but that was
explicitly withdrawn: the stub is correct as-is, and documenting ferrule
discipline there would leak the procedure to subagents who can see the
schema. The correct place for that discipline is the lead-gated
`workflow_manual` output, which subagents never see.

That is exactly where the documentation is missing:

- In the rendered `workflow_manual`, the `### User preferences` section body
  is entirely empty.
- The `### Session setup` section has odd spacing (blank lines before the
  body) and states "call ferrule once per working root" but never states the
  consequence of calling it a second time for the same root — that a second
  call mints a new session identity with empty state, stranding any prior
  agenda/todo/session-tree state bound to the earlier key.

This gap is not cosmetic: a legitimate lead fell into the redundant-mint trap
precisely because the one authorized documentation channel for this
discipline never states the consequence.

## Suggestion

Fill `Session setup` with the full ferrule discipline: reuse the existing
session key across the working session; a second `ferrule` call for the same
root mints a new identity with empty state, stranding prior agenda/todo state;
preserve the key verbatim across compaction. Also fix the section's spacing
and fill in (or otherwise repair) the empty `User preferences` section body.

## Spec Impact

Target: `ai-docs/spec/mcp-tools.md`. Caller-visible change: `workflow_manual`'s
rendered `Session setup` and `User preferences` sections gain the ferrule
reuse-discipline and are no longer empty/thin. Contract-first spec: no.

## Phases

### Phase 1: Fill Session setup and User preferences prose

Rendered `lead-workflow-manual` playbook prose only; no mode-branching,
override-marker, or handler-contract changes. Editing the always-shown
per-root paragraph plus the fresh-only gated block, and the User-preference
override seed's static fallback text.

### Result (pending) - 2026-07-02

- Edited `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`
  (and synced the identical `agents-plugin-wsflow/` copy):
  - `### Session setup`: removed the stray blank line before the always-shown
    paragraph; extended it with the ferrule redundant-mint consequence — a
    second `ferrule` call for a root you already hold a key for mints a new
    session identity with empty state, stranding prior agenda/todo/session-tree
    state — and the compaction-recovery guidance (restore, don't re-mint).
  - `### User preferences`: added a static default sentence inside the
    `UserPreferenceSection` override seed (before `{{.WorkflowLang}}`) so the
    section is never fully empty in the default render; override and
    `workflow.lang` injection still layer on top unchanged.
  - Regenerated `manifest.json` in both `agents-plugin/rsrc/` and
    `agents-plugin-wsflow/rsrc/` (sha256 rsrc integrity check) via a throwaway
    `wsrsrc.GenerateManifest`/`WriteManifest` invocation; no unrelated hash
    changed.
- No handler code (`workflow_manual.go`), mode-gating markers, or override-marker
  engine changed — this was prose-only inside the existing seed slots.
- Spec (`ai-docs/spec/mcp-tools.md`) left unchanged: the fresh/continue/fail-loud
  contract and the "always-shown per-root rule" wording it documents are
  unaffected; only the rsrc-owned prose gained detail as the spec already
  anticipates ("all manual prose lives in the rsrc").

**Verification:**
- Added `TestShippedManualSessionSetupAndUserPreferenceSectionsAreNotThin` in
  `agents-plugin-tool/internal/mcp/prompt_override_test.go`, asserting the
  Session setup redundant-mint sentence is present and the User preferences
  section body is non-empty/non-whitespace in the default (no override, no
  `workflow.lang`) render.
- Updated the now-inaccurate docstring on
  `TestShippedUserPreferenceSectionEmptySlotAndOverride` (previously said the
  no-override render is empty; it now renders the static default text).
- `cd agents-plugin-tool && go test ./...` — all packages pass, including
  `internal/mcp` (16.7s).
- `python3 -m unittest discover agents-plugin/tests` — 40 tests pass.
- `python3 -m unittest discover agents-plugin-wsflow/tests` — 9 tests pass.
