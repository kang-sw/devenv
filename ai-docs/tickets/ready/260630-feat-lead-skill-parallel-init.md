---
title: "Lead skill parallel init: workflow_manual key-return + SKILL.md batch entry"
spec:
  - 260626-workflow-manual-restoration-entry
  - 260610-entry-skill-surface-reduction
sage-review: completed
---

# Lead skill parallel init: workflow_manual key-return + SKILL.md batch entry

## Background

Lead skills that load project context (lead-discuss, lead-sprint) currently
incur a 4–5 serial MCP round-trip chain on each invocation:

```
SKILL.md fired
  → ws/playbook.print          [round 1]
  → ws/workflow_manual         [round 2 — playbook must return first]
  → ws/project_tree            [round 3]
  → ws/git.status              [round 4]
  → user message handling
```

Additionally, fresh-start sessions call workflow_manual twice:
1. `workflow_manual("obsidian-latch")` — bootstrap, instructs calling ws.ferrule
2. `ws.ferrule(root)` — mints session key
3. `workflow_manual(real-key)` — CONTINUE mode, redundant round-trip: the newly-minted
   session has no stored state, so the only difference from FRESH is the stripped
   fresh-only block and an empty Session State section

Both problems share the same root: playbook.print gates all subsequent init
because SKILL.md is a one-line routing stub that defers init sequencing entirely
to the playbook.

## Goal

Reduce the init chain to a fixed 2-round pattern uniform across fresh and
continue paths:

```
Round 1: [ws/playbook.print + ws/workflow_manual] in parallel
Round 2: [ws/project_tree + ws/git.status] in parallel   (per playbook prose)
```

## Phases

### Phase 1: workflow_manual absorbs ferrule for fresh-start

**Change:** Add optional `root` parameter to `ws.workflow_manual`. In FRESH
mode (sentinel key), the handler internally calls the ferrule logic to mint a
session key for the given root, then returns the minted `session_key` in the
response body alongside the rendered manual. The caller receives the key
immediately — no separate `ws.ferrule` call needed.

**Response contract:**

- `workflow_manual("obsidian-latch")` with NO root → unchanged FRESH behavior:
  fresh-only block kept, no `## Session Key`. Caller still sees the ferrule
  instruction and must call ferrule separately.
- `workflow_manual("obsidian-latch", root: "...")` with root → new minting path:
  fresh-only block **stripped** (same as CONTINUE rendering, since key is now
  available), then append `## Session Key\n<minted-key>` and `## Session State`
  (empty for new session). Caller receives the key inline with no contradictory
  ferrule instruction.
- CONTINUE mode: unchanged.

The fresh-only block must be stripped when root is supplied and minting succeeds,
because keeping it would tell the caller they have no key while simultaneously
handing them one — contradictory. The implementer must select
`stripModeGatedRegion(body, false)` (not `true`) in the root-supplied branch.

**Eliminated:** double workflow_manual call pattern (obsidian-latch →
ferrule → second workflow_manual). Fresh-start and continue paths are now both
single workflow_manual calls.

**Constraints:**
- `root` must be an absolute filesystem path (same validation as ws.ferrule).
- FAIL-LOUD and keyless modes are unchanged. A non-sentinel key with a `root`
  argument must still hit FAIL-LOUD before any minting — the mint path is gated
  exclusively on the sentinel branch, consistent with the current FAIL-LOUD guard
  at `workflow_manual.go:136` running before the render step.
- Spec anchor `260626-workflow-manual-restoration-entry` in mcp-tools.md must
  be updated to document the new `root` param and FRESH-mode key return.

**Rejected alternative:** workflow_manual returns key in a structured JSON
response — rejected because the tool returns rendered Markdown prose; a
separate `## Session Key` section fits existing response format without
protocol change.

**Verification:** After change, a fresh-start session calling
`workflow_manual("obsidian-latch", root: "...")` receives a session key in the
response. No second workflow_manual call or separate ferrule call is needed
before calling project_tree or git.status.

### Result (0b67515d) - 2026-06-30

`workflow_manual` now accepts optional `root` in FRESH mode. When `root` is
supplied with the sentinel key, the handler mints a lead session key inline via
`canonicalSetupRoot` + `sessions.mint`, strips the fresh-only block with
`stripModeGatedRegion(body, false)`, and returns `## Session Key` + empty
`## Session State`. Separate `ws.ferrule` call eliminated for fresh-start flows.

Spec anchor `260626-workflow-manual-restoration-entry` updated with fresh-with-root
sub-case. Mental models `mcp-runtime.md` and `workflow-skills.md` updated.

Review: partitioned (correctness, fit) — both clean (fit: 1 minor gofmt alignment,
recorded and accepted).

### Phase 2: SKILL.md parallel entry + playbook On: invoke simplification

**Change A — SKILL.md for lead-discuss and lead-sprint:**

Replace the current one-line stub with a parallel entry declaration:

```
Call in parallel:
- ws/playbook.print(name: "<skill-name>", session_key: <your key>)
- ws/workflow_manual(session_key: <your key or obsidian-latch>, root: <path if fresh>)

After both return, execute the procedure returned by playbook.print.
```

**Change B — lead-discuss and lead-sprint On: invoke:**

Remove the sequential init steps (workflow_manual, project_tree, git.status).
Replace with:

```
## On: invoke

1. Call ws/project_tree(session_key) and ws/git.status(session_key) in parallel.
2. [continue to user message handling]

Post-compaction reload: if session compaction occurred, call
ws/workflow_manual(session_key) immediately before step 1; if the key is lost,
run ws:lead-revive first to recover it.
```

The workflow_manual step is removed from the normal init sequence because
SKILL.md already called it before the playbook was loaded. However, the
post-compaction reload instruction must be preserved — SKILL.md fires only at
initial invocation, not after in-session compaction, so the playbook must carry
this guidance for the lead to restore session state mid-session.

**Scope:** lead-discuss and lead-sprint only. lead-proceed and lead-implement
have no project-context init in their On: invoke and are not modified.

**Inter-phase dependency:** Phase 2 SKILL.md prose references workflow_manual's
key-return behavior (fresh-start path). Phase 1 must land before Phase 2 is
correct end-to-end, though Phase 2 docs can be written independently.

**Constraints:**
- SKILL.md may use the literal `"obsidian-latch"` sentinel string. The earlier
  no-expose rule was intended to prevent subagent discovery via auto-exposed
  surfaces (playbook.print), but SKILL.md requires direct file access and is a
  narrower exposure path than the 6 playbooks that already contain the literal
  (known-residual: `260626-research-playbook-print-lead-surface-leak`). The
  per-file rule is stale and removed.
- Spec anchor `260610-entry-skill-surface-reduction` in workflow-skills.md must
  be updated to reflect that entry skills for context-heavy skills carry a
  parallel init declaration rather than a pure routing stub.
- agents-plugin and agents-plugin-wsflow rsrc mirrors must both be updated.

**Rejected alternative — full Option A (skill declares all four calls
including project_tree and git.status):** project_tree and git.status are
playbook-specific, not universal to all lead skills. Encoding them in SKILL.md
creates coupling: a future change to discuss-specific init requirements would
require a SKILL.md update. Keeping them in the playbook's On: invoke preserves
a single change point.

**Rejected alternative — Option C only (prose-only parallel hint in playbook
On: invoke for workflow_manual + playbook.print):** LLM batching of
independently-described tool calls is unreliable without an explicit parallel
declaration at the skill level. The skill-level declaration for the
playbook.print + workflow_manual pair is the reliable path.

**Verification:** After both phases, a continue-path lead-discuss invocation
completes init in 2 MCP rounds. Fresh-start completes in 2 rounds (round 1
includes obsidian-latch workflow_manual; round 2 is project_tree + git.status
after key is available). Confirm parallel call declaration is present in updated
lead-discuss/SKILL.md and lead-sprint/SKILL.md.
