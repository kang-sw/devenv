---
title: Add a required expected_branch guard to ws/git.commit
related:
  260916-feat-ws-git-merge-relax-worktree-gate: related
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 718c241efb8b7146
sage-review-completeness-reviewed: 718c241efb8b7146
completed: 2026-09-17
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
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go (git.commit schema and handler), agents-plugin-tool/internal/wsgit/git.go (CommitOptions/Commit), agents-plugin-tool/cmd/ws-mcp/main.go (CLI gitCommit caller), plus a caller sweep across agents-plugin-tool/internal/mcp/*_test.go, agents-plugin-tool/internal/wsdoc/*.go and *_test.go, agents-plugin/rsrc/lead-ticket/lead-ticket.md, agents-plugin/rsrc/lead-scope-worktree/lead-scope-worktree.md, agents-plugin-wsflow/rsrc/lead-ticket/lead-ticket.md, agents-plugin-wsflow/rsrc/lead-scope-worktree/lead-scope-worktree.md |
| scope.surface | public-interface | git.commit MCP tool inputSchema (agents-plugin-tool/internal/mcp/server.go#L3578-L3593), an agent-facing shipped tool contract, gains a required field |
| scope.new_public_symbol | no | extends the existing CommitOptions struct and git.commit inputSchema; no wholly new exported Go symbol |
| scope.new_type_contract | yes | git.commit inputSchema required list, currently paths, title, ai_context (agents-plugin-tool/internal/mcp/server.go#L3592), gains expected_branch |
| scope.test_surface | existing | git.commit already has coverage in agents-plugin-tool/internal/mcp/server_test.go and related *_test.go files (session_auth_test.go, tickets_verify_test.go, tickets_sage_test.go, note_tools_test.go) |
| complexity.reuse_points | confirmed | mirrors git.merge's expected_source_oid/expected_target_oid compare-and-swap and must_resolve diagnostic shape (agents-plugin-tool/internal/mcp/git_merge.go#L121, #L138, #L200) and its current-branch resolution via git symbolic-ref --quiet --short HEAD (git_merge.go#L97) |
| complexity.side_effect_risk | moderate | making the parameter required ripples through every existing git.commit caller across agents-plugin-tool tests plus the mirrored agents-plugin and agents-plugin-wsflow skill docs |
| risk.correctness | moderate | detached-HEAD and believed-vs-actual mismatch detection are new branch conditions to get right without false positives or negatives |
| risk.fit | low | the change explicitly mirrors the existing git.merge compare-and-swap pattern already established in the same package |
| risk.test | moderate | the caller sweep spans two shipped skill packages plus a wide *_test.go set found in this pass; an incomplete sweep would surface only at go test time |
| risk.security_or_contract | high | the ticket's own Constraints section calls this a shipped tool-contract (breaking) change to a widely used MCP tool, with the schema description's meta wording explicitly load-bearing |

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

### Result (31eedb1) - 2026-09-17

Landed the guard. `git.commit` now requires `expected_branch`; before any staging
mutation `wsgit.Commit` resolves the actual branch via `git symbolic-ref --quiet
--short HEAD` and hard-refuses (no mutation) on a believed-vs-actual mismatch or a
detached HEAD, reporting both branch names. Enforcement is at both surfaces: the
MCP `git.commit` inputSchema `required` list and `normalizeCommitOptions` (so the
CLI and any direct caller are guarded too). The schema description is the
meta/intent wording from `## Decisions`, kept downstream-neutral.

Caller sweep: MCP handler wiring (`server.go`), CLI `--expected-branch` flag
(`cmd/ws-mcp/main.go`), and every test that commits — wsgit `sequenceRunner`
fixtures gained the leading `symbolic-ref` output with call-index assertions
shifted by one, and MCP/CLI integration tests resolve the temp repo's branch via a
new `headBranch` helper (`git init` picks main/master from `init.defaultBranch`).
`lead-ticket.md` commit-call signature lists `expected_branch`; the wsflow rsrc
mirror + both rsrc manifests were regenerated via `WSRSRC_REGEN` /
`WS_REGEN_WSFLOW_RSRC`, and the `agents-plugin-pi/` byte-identical mirror was
resynced for the same two files (review round-1 Critical: the pi mirror was missed
initially; fixed in 8a0200b). `git.merge` is intentionally untouched (it uses OID
compare-and-swap and never calls `wsgit.Commit`).

Verification evidence: `go test ./... -count=1` in `agents-plugin-tool` — all 15
packages green (the `-count=1` matters: the disk-reading mirror tests were served
from a stale cache on an earlier plain `go test`, masking the pi drift);
`go vet ./...` clean; `scripts/smoke-ws-mcp.sh ..` exit 0; wsflow package tests
(`python3 -m unittest discover agents-plugin-wsflow/tests`) OK. New tests:
`TestCommitRefusesBranchMismatch`, `TestCommitRefusesDetachedHead`, the missing-
field `normalizeCommitOptions` subtest, and `TestServeStdioGitCommitRefusesBranchMismatch`
(proves no commit lands on mismatch).

Decisions taken: used a dedicated `symbolic-ref` resolution (the ticket's stated
mechanism) placed as the first git op rather than reading the branch already
present in the pre-commit `git status` output — faithful to the ticket and keeps
the guard independent of the staging-status parse. Recorded (non-blocking) review
Minors, left as-is: a `symbolic-ref` failure for a non-repo/other cause reports the
"detached HEAD" refusal (fail-closed, diagnostic-quality only); and no separate
MCP-layer missing-field test (the Go-level `normalizeCommitOptions` subtest is
functionally equivalent since dispatch has no separate schema-validation layer).
