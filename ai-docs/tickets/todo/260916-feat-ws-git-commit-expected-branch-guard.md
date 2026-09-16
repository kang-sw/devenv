---
title: Add a required expected_branch guard to ws/git.commit
related:
  260916-feat-ws-git-merge-relax-worktree-gate: related
---

# Add a required expected_branch guard to ws/git.commit

## Background

In a worktree shared by parallel ws sessions, one session can `git switch` the
checkout to another branch (e.g. a lead-run acquiring `impl/develop/<name>`)
while another agent still believes it is on the branch it started on. The second
agent then commits against the *actual* checkout, not the one it remembers, and
the commit silently lands on the wrong branch.

This session brushed the hazard directly: the lead committed a ticket believing
it was on `develop`, and moments later found the shared worktree checked out on a
parallel session's `impl/develop/film-spilt-zap`. The commit happened to land on
`develop` only because the parallel switch occurred just after the commit — a few
seconds' difference would have put it on the wrong branch.

The fix is deliberately simple: let the caller assert the branch it *believes*
it is on, and refuse the commit if reality disagrees. No session-state machinery.

## Decisions

### Contract

- Add a **required** `expected_branch` parameter to `git.commit`. Because it is
  required, a caller that does not know its branch cannot form the call at all —
  which is the point: not knowing forces a deliberate check of whether a commit
  should happen here.
- Before committing, resolve the actual current branch (`git symbolic-ref
  --quiet --short HEAD`). If it does not equal `expected_branch`, **hard-refuse
  the commit** (no mutation) and report both values ("believed: X / actual: Y").
- On a match, commit as today.
- Detached HEAD (no branch name): refuse — there is no branch to confirm, and a
  commit under a detached HEAD in a shared worktree is itself the kind of state
  this guard exists to stop.

### The description carries the intent, not just the field

The `expected_branch` schema description must be written **meta / intent-first**,
not as a mechanical "put the branch name here". A real agent, trying to fill the
field, will otherwise habitually run "check the current branch" and paste that —
which makes `expected == actual` by construction and defeats the guard entirely,
while also missing the real signal ("you may be about to commit where you should
not"). The description must therefore say, in substance:

> Write the branch name you currently *remember/believe* you are on. If you do
> not know it, stop and verify whether you should be committing here at all —
> do not reflexively read HEAD just to fill this field. This is a safety guard
> against committing while your believed branch differs from the actual checkout
> (e.g. a parallel session switched it).

This wording is a load-bearing part of the feature, not decoration: it is what
keeps the guard from being trivially self-satisfied.

### Scope

- `git.commit` only this pass. It is where the near-miss occurred.
- `git.merge` is intentionally excluded: it already performs OID
  compare-and-swap via `expected_source_oid` / `expected_target_oid`, and it
  switches branches by design, so a "current branch" assertion means something
  different there. Revisit as a possible follow-up, not part of this ticket.

### Rejected / not doing

- **Session-tracked "last known branch" auto-guard** — rejected as overengineered
  for this purpose. The required parameter plus a claim-vs-reality check achieves
  the intent without any session state or wiring into sanctioned switchers.
- **OID / tip assertion on commit** — out of scope; the concern here is a
  branch-identity mismatch, not concurrent commits on the same branch.

## Constraints

- Making `expected_branch` required is a **shipped tool-contract change**: every
  existing `git.commit` caller (worker and lead playbooks, tests) must be swept
  to pass it. Account for that blast radius when planning the change.
- The `git.commit` schema/handler lives under
  `agents-plugin-tool/internal/mcp/`; read `ai-docs/manuals/ws-mcp.md` before
  editing.
- The description string is agent-facing shipped-surface text; read
  `ai-docs/manuals/shipped-surface-boundary.md` before writing it and keep it
  downstream-neutral.
- Implementation note (not user-facing): do not auto-derive `expected_branch`
  from a fresh HEAD read inside the tool or its callers — that would always match
  and check nothing. The value must originate from the caller's remembered
  context established earlier in its task.

## Prior Art

- `agents-plugin-tool/internal/mcp/git_merge.go` — the existing
  `expected_source_oid` / `expected_target_oid` acknowledgement is the
  compare-and-swap pattern and diagnostic shape to mirror (a `must_resolve`
  refusal that reports expected-vs-actual).
- The `git.commit` handler and its `tools/list` schema registration in
  `agents-plugin-tool/internal/mcp/server.go`.

## Phases

### Phase 1: Add and enforce the expected_branch guard on git.commit

Add a required `expected_branch` parameter to the `git.commit` schema and handler.
Resolve the actual current branch and hard-refuse the commit on mismatch (and on
detached HEAD), reporting believed-vs-actual. Write the schema description as the
meta/intent wording in `## Decisions`. Sweep every existing `git.commit` caller
(playbooks, tests) to pass the branch it established for its task.

Verification: matching branch → commit proceeds; mismatched branch → refused with
no commit and a believed-vs-actual message; detached HEAD → refused; missing
`expected_branch` → schema-level rejection. `go test ./...` in `agents-plugin-tool`
green, and the caller sweep leaves no playbook/test issuing a `git.commit` without
the parameter.
