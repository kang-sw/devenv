---
title: "Retire lead-goal-fan-out-step and the orphaned session.note surface"
sage-review-design: required
related:
  260724-feat-lead-fan-out-worktree: the ticket that shipped the skill and the session.note tool this retires; its spec anchors must be closed here
  260725-research-goal-loop-restart-starved-by-background-delegation: names the fan-out dispatch model as the worst case for the Stop-hook starvation that drives the goal loop
  260630-epic-skill-playbook-diet: same direction — remove machinery that does not earn its cost
  260730-refactor-drain-ready-queue-rename-and-prefer-subagent-splice: sibling; this ticket must land first so the fan-out transclusion hook is gone before drain's surface is reshaped
---

# Retire lead-goal-fan-out-step and the orphaned session.note surface

## Background

`lead-goal-fan-out-step` landed 2026-07-25 (`260724-feat-lead-fan-out-worktree`)
as a batch-parallel `lead-goal-step` variant: when two or more `ready/` tickets
are mutually independent, it advances them in parallel, one worktree-isolated
mini-lead per ticket. Three facts have accumulated against it since.

1. **It starves the mechanism that drives it.**
   `260725-research-goal-loop-restart-starved-by-background-delegation` observed
   that a lead turn ending with pending background tasks does not produce the
   clean terminal stop the `/goal` Stop-hook needs to re-inject the next cycle.
   The fan-out path dispatches N background mini-leads and juggles them from the
   lead's own context — the research names it explicitly as the worst case for
   this starvation. The feature's dispatch model is in direct tension with the
   loop it is a step of.

2. **It is host-specific in a host-neutral surface.** The playbook body states
   the fan-out path is "known-available on Claude Code, where mini-leads must be
   spawned in an `Agent`-tool-retaining form (not Explore or Plan)"
   (`agents-plugin/rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md:9`).
   AGENTS.md Architecture Rule 3 is host-neutral first; this is a Claude-Code-only
   capability gate sitting in the shared skill surface.

3. **It carries bespoke Go for one caller.** `printPlaybook`
   (`agents-plugin-tool/internal/mcp/playbook_tools.go:923-932`) has a
   name-hardcoded serve-time transclusion that appends `lead-goal-step`'s
   `SKILL.md` body into the fan-out overlay, purely so the overlay need not
   restate goal-step's contract.

Separately, **`session.note` has exactly one consumer in shipped skill text**:
the fan-out playbook's dispatch board (`:26` and `:32`, in both the
`agents-plugin/` and `agents-plugin-wsflow/` mirrors). Its runtime gate is
`">=0.37.3-dev <0.38.0"` in both `runtime.json` files — it has never shipped in a
released version. Deleting the skill without deleting the tool leaves a
zero-caller MCP surface in `api.list` and `runtime.json`, which a later author
reads as a blessed facility.

## Decisions

**Delete the skill, keep the thesis.** The parallelism premise — independent
`ready/` tickets are genuinely parallelizable — is not refuted; what is refuted
is this driver under this harness. Capture the premise as an `idea/` research
ticket so the direction survives the deletion, and let `260725` answer "can a
lead drive N background workers under this harness at all" before anything is
rebuilt.

**Retire `session.note` with it.** A tool with zero callers is worse than no
tool. It never shipped in a release, so there is no downstream compatibility
obligation.

**Keep `session.children`.** Unlike `session.note` it has an independent role
that outlives fan-out: re-finding child sessions after the lead's own context
compaction. It stays.

**Delete the transclusion hook rather than generalizing it.** The sibling ticket
originally planned a second serve-time consumer (drain transcluding
`lead-prefer-subagent`), which would have justified turning the two hardcoded
hooks into a declarative table. That plan changed: the sibling now composes at
build time instead, so no second serve-time consumer appears. Hook #2 (the
fan-out one) is deleted outright. Hook #1 — `lead-workflow-manual` appending
`lead-prefer-subagent` when `workflow.prefer_subagent` is on — is out of scope
and stays exactly as it is.

## Phases

### Phase 1: Delete the skill surface and its transclusion hook

Remove the skill and playbook in both package mirrors, and the Go that serves it.

- `agents-plugin/skills/lead-goal-fan-out-step/`,
  `agents-plugin/rsrc/lead-goal-fan-out-step/`, and the
  `agents-plugin-wsflow/` counterparts.
- `playbook_tools.go`: the `name == goalFanOutStepPlaybookName` branch and the
  now-unused `goalFanOutStepPlaybookName` / `goalStepPlaybookName` /
  `goalStepPlaybookTitle` constants (verify `goalStepPlaybookName` has no other
  reader before removing it). Update the `printPlaybook` doc comment, which
  currently documents two concatenation hooks.
- Regenerate `agents-plugin/skills/manifest.json`
  (`WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest`)
  and any wsflow rsrc mirror (`WS_REGEN_WSFLOW_RSRC=1`).
- Tests referencing the skill: `playbook_tools_test.go`,
  `skills_mirror_test.go`, `agents-plugin/tests/test_skill_dispatch_contracts.py`,
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`.

Verification: `go build ./...`, `go test ./...`, and both python test files pass;
`grep -r lead-goal-fan-out-step` returns nothing outside `ai-docs/` history.

### Phase 2: Retire session.note

Remove the tool and its on-disk half, keeping `session.children` intact.

- `server.go`: the `"session.note"` dispatch case, its handler, and its
  tool-list descriptor entry.
- `session_auth.go`: the note field and its write path (`:62`, `:276-279`);
  confirm `session.children` still renders correctly with the field gone, and
  decide whether existing on-disk records carrying a note need tolerant decode
  (they should — a stale record must not fail to load).
- Both `runtime.json` files: drop the `session.note` gate, keep
  `session.children`.
- CHANGELOG entry for the removal.

Verification: `go test ./...` passes; a session record written before the change
still loads; `api.list` no longer lists `session.note`.

### Phase 3: Documentation closeout

- `ai-docs/spec/workflow-skills.md`: remove the fan-out paragraph (`:543-567`)
  and the `{#260724-goal-fan-out-step-transclusion}` anchor; drop
  `lead-goal-fan-out-step` from the skill inventory (`:30`, `:55-56`).
- `ai-docs/mental-model/workflow-skills.md`: lines 18, 19, and 83 each name
  `lead-goal-fan-out-step` as an example of a context-heavy entry skill —
  replace the example rather than deleting the sentence.
- Create the `idea/` research ticket carrying the preserved parallelism thesis.

## Spec Impact

Removes `{#260724-goal-fan-out-step-transclusion}` and the fan-out skill entry
from `workflow-skills.md`. The `session.note` removal touches the MCP tool
inventory in `mcp-tools.md` and the runtime gate list in `plugin-runtime.md`.
No new caller-visible behavior is introduced.

## Out of Scope

- Hook #1 (`lead-workflow-manual` ← `lead-prefer-subagent`). Untouched.
- `session.children`. Retained.
- Any replacement parallel-execution mechanism. Deferred to the preserved
  `idea/` ticket, gated on `260725`.
