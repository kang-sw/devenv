---
title: Render implementer delegate prompts from file-first context
sage-review: completed
related:
  260627-feat-enter-implement-deterministic-verdict-engine: follows the completed lead-implement compression work by moving long delegate prompt templates out of the always-rendered playbook body
  260627-feat-todo-item-instructions: prerequisite for keeping post-verdict execution guidance in focused todo instructions
spec:
  - 260505-implementation-workflow-skills
  - 260619-stateless-implement-review-continuity
  - 260505-workflow-delegate-prompt-boundaries
  - 260609-playbook-tools
related-mental-model:
  - workflow-skills
  - mcp-runtime
---

# Render implementer delegate prompts from file-first context

## Background

`lead-implement` has already been compressed so the always-rendered playbook
follows the MCP-authored Implementation Verdict and the derived todo runbook
instead of repeating every reachable execution branch. The next visible source
of bloat is delegate prompt delivery: `lead-implement` still carries long
implementer and review-relay prompt templates even though delegate playbooks are
already rendered through `ws/playbook.render` and implementation contracts are
already written to brief and plan files before dispatch.

This ticket optimizes that boundary. The lead should prepare file-backed task
artifacts, render the implementer-facing prompt with only path and verification
metadata, then send a minimal host prompt that tells the worker to read and
execute the rendered playbook. Contracts stay in the brief and plan files; render
context should not duplicate the ticket, plan, acceptance contract, or long
review findings.

## Decisions

- Use a file-first delegate contract. The brief is the primary implementation
  contract; an optional plan file refines that contract when plan depth requires
  one.
- Keep render context small and mechanical: paths, commit ranges, verification
  metadata, and short lead notes. Do not pass large prose contracts through
  render context when the same content belongs in a brief, plan, or review file.
- Split the delegate surfaces when it avoids optional-variable or mode
  complexity. The expected shape is `implementer` for initial implementation and
  `implementer-relay` for review-fix loops.
- Keep the lead-owned spawn prompt short. It should identify the rendered
  playbook path and instruct the worker to follow it; the rendered playbook owns
  the detailed task input.
- Use review findings paths for reviewer-to-implementer relay. The lead adds
  only disposition notes such as fix, reject, defer, or won't-fix rationale.
- Preserve stateless fix-loop correctness. Reusing a host worker is allowed when
  available, but every required fact must still be present in rendered prompt
  inputs and file paths.

## Spec Impact

Contract-first spec: no. Existing specs already define `lead-implement` as the
implementation harness, `ws/playbook.render` as the delegate prompt
materialization surface, and delegate prompts as rendered rsrc playbooks rather
than register-time stems. This ticket changes how the existing `lead-implement`
delegation contract is expressed and should update these entries after
implementation:

- `260505-implementation-workflow-skills`: record that delegated implementation
  prompt delivery is file-first and render-parameterized.
- `260619-stateless-implement-review-continuity`: record that relay cycles pass
  review paths and lead disposition notes into a rendered relay prompt rather
  than embedding long relay prose in `lead-implement`.
- `260505-workflow-delegate-prompt-boundaries`: record that lead-owned spawn
  prompts stay minimal because role-specific task input is rendered into the
  delegate playbook.
- `260609-playbook-tools`: no new MCP surface is required; confirm that declared
  playbook variables are sufficient, and document any newly required variable
  convention only if implementation makes it caller-visible.

## Implementation Notes

Relevant current constraints:

- `playbook.render` accepts caller context only for variables declared by the
  target playbook. Undeclared context fails, and there is no conditional
  template language.
- Optional values should not force a single bloated playbook shape. Prefer a
  separate `implementer-relay` playbook over mode flags when relay input differs
  materially from initial implementation input.
- wsflow does not use a freeform context bridge for `implementer`, so this work
  should use declared variables and keep ws/wsflow resource mirrors consistent.
- The just-finished compression commit `bb1d1e2e` reduced the lead playbook body
  but intentionally left delegate templates in place. This ticket is the next
  phase that removes or sharply compresses those templates.

## Sage Review

Status: completed.

Result: design reviewer `pass`; completeness reviewer `pass`.

## Phases

### Phase 1: Parameterize initial implementer dispatch

Convert initial delegated implementation dispatch to a rendered,
file-first `implementer` prompt:

- Add declared render variables for the minimal initial context, expected to be
  brief path, optional plan path or empty-string convention, verification
  command/hint, result expectations, and any required commit-range reporting
  metadata.
- Move detailed initial implementation task input into the rendered
  `implementer` playbook, using brief and plan paths as the source of contract
  truth.
- Replace the long `lead-implement` Implementer spawn prompt with a minimal
  instruction that names the rendered playbook path and says to execute it.
- Preserve recommended tier handling from `playbook.render`.
- Update tests that assert `lead-implement`, `implementer`, and wsflow mirrored
  rendering behavior.

Verification boundary: render `lead-implement` and `implementer` successfully
with representative context, run Go tests covering playbook rendering, run
wsflow package tests when mirrored resources change, and verify no always-rendered
lead text still embeds the old long initial implementer prompt.

### Result (af1cebf) - 2026-06-27

Implemented Phase 1. Initial delegated implementation dispatch now renders the
`implementer` playbook with declared file-first inputs for the brief path,
optional plan path, verification hint, result expectations, and commit-range
hint. `lead-implement` now keeps only the rendered prompt path handoff for the
initial implementer path instead of embedding the long worker prompt in the
always-rendered lead playbook.

The `implementer` playbook now treats the brief, optional plan, and listed
references as the task contract, blocks direct ticket-file reads unless the
caller explicitly overrides the rule, and reports final commit/hash range as
part of normal completion. `delegates:false` suppresses the generic continuation
tip for this direct-execution prompt while preserving `role: implementer` and
`tier: medium` behavior.

Verification passed for focused MCP playbook-render tests, manifest
regeneration, wsflow rsrc mirror regeneration, `go test ./internal/wsrsrc
-count=1`, `python3 -m unittest discover agents-plugin-wsflow/tests`, `go test
./...` in `agents-plugin-tool`, and `git diff --check`. Partitioned review
completed: correctness `clean`, test `clean`, fit `clean` after the
fresh-reader audit findings were fixed.

### Phase 2: Add review-fix relay render surface

Introduce a review-fix relay surface, expected to be `implementer-relay`, when it
keeps relay inputs simpler than overloading `implementer`:

- Render relay prompts from review findings paths, current commit range, lead
  disposition notes, and verification metadata.
- Keep reviewer findings in files and pass paths to the implementer; do not copy
  full findings into lead-owned prompt templates.
- Keep lead adjudication explicit for rejected, deferred, or won't-fix findings.
- Replace the long `lead-implement` Review relay prompt with a minimal render
  and dispatch instruction.
- Preserve stateless loop semantics and the existing option to reuse an
  implementer or reviewer when the host supports it.

Verification boundary: render the relay prompt with representative non-clean
review paths and disposition notes, run playbook-render tests, run wsflow package
tests when mirrored resources change, and run a focused text audit that confirms
`lead-implement` no longer carries long relay prose.

### Result (089a0e0) - 2026-06-27

Implemented Phase 2. Delegated review-fix relay now renders a dedicated
`implementer-relay` playbook with declared file-first inputs for brief path,
optional plan path, review cycle, current commit range, non-clean review paths,
lead disposition notes, verification instructions, and result expectations.
`lead-implement` now keeps a short rendered-prompt dispatch for review relay
instead of carrying the long relay prompt body in its always-rendered text.

The relay prompt is a direct-execution implementer surface with
`delegates:false`, `role: implementer`, and `tier: medium`, preserving
render-minted delegate credentials and recommended-tier metadata without adding
a nested-delegation continuation cue. Reviewer findings remain file inputs; the
lead supplies disposition notes and owns triage, verification, re-review
orchestration, and final clean judgment.

Verification passed for focused MCP playbook-render tests, manifest
regeneration, wsflow rsrc mirror regeneration, `go test ./internal/wsrsrc
-count=1`, `python3 -m unittest discover agents-plugin-wsflow/tests`, `go test
./...` in `agents-plugin-tool`, and `git diff --check`. Fresh-reader audit
findings accepted for this phase were fixed in `089a0e0`. Partitioned review
completed with correctness `clean`, fit `clean`, and test `clean`.

### Phase 3: Update workflow documentation

Update spec and mental-model documentation after the implementation shape is
proven:

- Record the file-first delegate prompt contract in `workflow-skills`.
- Record any caller-visible `playbook.render` variable convention in
  `mcp-tools` only if implementation exposes one beyond existing declared
  variable behavior.
- Update the workflow-skills and mcp-runtime mental models so future edits know
  that delegate task contracts live in brief/plan/review files and rendered
  prompt context only carries pointers and metadata.
- Close this ticket only after sage review has either passed or its execution
  blocker has been explicitly accepted for the closeout.

Verification boundary: run spec index verification, documentation diff checks,
and the implementation test suite relevant to any source/resource changes.
