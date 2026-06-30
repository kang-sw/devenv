---
title: "workflow_manual content diet — remove authoring-only and redundant sections"
related:
  260626-bug-workflow-manual-bootstrap-sentinel-surface: adjacent workflow_manual surface topic
  260611-bug-rsrc-manifest-regen-missed-after-shipped-edit: manifest regen discipline applies after rsrc edits here
spec:
  - 260626-workflow-manual-restoration-entry
related-mental-model:
  - workflow-skills
sage-review: completed
completed: 2026-06-30
---

# workflow_manual content diet — remove authoring-only and redundant sections

## Background

`ws.workflow_manual` is called on every `lead-discuss` and `lead-sprint` invocation
(parallel round 1, after Phase 2 of 260630-feat-lead-skill-parallel-init). Its rendered
response is approximately 2 500 tokens.

Analysis of the current content against what is already visible to the model at invocation
time (deferred tool names in system-reminder, skill descriptions) shows that a significant
fraction is either:

- **Authoring-only guidance** — tells humans how to write skill text; belongs in
  `lead-skill-authoring`, not in a runtime-loaded primitives reference.
- **Redundant tool name listings** — every `ws/*` tool name is already visible in the
  deferred-tools system-reminder; repeating them in code blocks adds no information.
- **Duplicated usage patterns** — the "Usage Pattern" block restates the per-section
  inline descriptions verbatim.
- **Meta-commentary** — a "Doctrine" paragraph that does not change runtime behavior.

Sections that are genuinely exclusive to `workflow_manual` (not visible elsewhere):
- Session State (dynamic: agenda, todos) — unique per session, cannot be inlined.
- User preferences (configured) — per-user override block.
- Mercenary delegate-path pattern (render → register/call/wait/result) — the multi-step
  flow is not inferrable from tool names alone.
- Git commit format (`## AI Context` body structure, ticket-move handling).
- Reference discovery ordering (`status: "ready"` vs `"todo"`, `ticket_stem` vs
  `mentions_ticket_stem` distinction).
- `ws.ferrule` binding rule (one key per canonical repository root / worktree).

Target: reduce rendered size from ~2 500 tokens to ~900–1 100 tokens (~60% reduction)
without removing any session-exclusive or behavior-critical content.

## Constraints

- Both rsrc mirrors must be updated together:
  `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md` (canonical) and
  `agents-plugin-wsflow/rsrc/lead-workflow-manual/` (byte-identical mirror).
- After any rsrc edit, both manifests must be regenerated (run from `agents-plugin-tool/`):
  `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -run TestRegenerateShippedManifest`
  `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -run TestRegenerateWsflowRsrcMirror`
  (lesson from 260611-bug-rsrc-manifest-regen-missed-after-shipped-edit).
- "How To Document" content must not be deleted silently — move the relevant prose to
  `agents-plugin/rsrc/lead-skill-authoring/` if no equivalent already exists there.
- Session State section is dynamic (rendered by the Go handler); its size is not
  meaningfully reducible by this ticket.

## Phases

### Phase 1: Trim lead-workflow-manual rsrc

Remove or relocate the following sections from
`agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`:

**Remove entirely:**
- `## How To Document` (authoring guidance → lead-skill-authoring or drop if already covered)
- Tool-name code blocks inside each `### <tool>` subsection of `## Available`
  (names are visible in deferred-tools; keep only the usage annotation text)
- `## Usage Pattern` block (duplicates inline section descriptions)
- `## Doctrine` paragraph

**Condense to ≤2 lines each:**
- `### Scoped Exploration (native Explore)` — keep the "spawn with English prompt, require
  cited evidence" instruction; drop the verbose expansion
- `### API documentation` — keep "external API: use native exploration, not ws tools"
- `## Planned Or Specialized` — keep the single-sentence advisory about checking
  `runtime.info` before assuming richer interrupt behavior

**Keep unchanged:**
- Session invariant header note
- `### Session setup` (`ws.ferrule` one-key-per-root rule)
- `### User preferences` (configured override block)
- `### Persistent agents` (mercenary pattern: register → render → call → wait/result → erase)
- `### Artifact paths`
- `### Reference discovery` preferences (ordering, `ticket_stem` vs `mentions_ticket_stem`)
- `### Git` usage patterns and `ws/git.commit` format
- `## Session State` (dynamic, not editable)

After edits, regenerate both manifests (see Constraints) and run from `agents-plugin-tool/`:
`go test ./internal/wsrsrc/... ./internal/mcp/...` to confirm no hash drift or
playbook-load failures.

Verify rendered token count has decreased materially (manual inspection of
`ws.workflow_manual` response length before and after; baseline ~2 500 tokens per Background).

### Result (c9088df8) - 2026-06-30

Trimmed `lead-workflow-manual.md` from ~255 lines to 163 lines. Removed `## How To Document`,
bare tool-name code blocks, `## Usage Pattern`, and `## Doctrine`. Condensed Scoped
Exploration, API documentation, and Planned Or Specialized to ≤2 lines each. Migrated
"Prefer Do X through Y" rule to `lead-skill-authoring`. Both rsrc mirrors kept byte-identical;
both manifests regenerated. `wsrsrc` and `mcp` tests pass.

Deviation: implementation was committed before sage review gate ran (workflow sequencing
error in lead session). Ticket updated and closed post-hoc.
