---
title: "Prevent failed tickets.move promotion from mutating frontmatter"
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-07-13
---

# Prevent failed tickets.move promotion from mutating frontmatter

## Observation

On 2026-07-13, `tickets.move(..., to: "ready")` returned a blocking
`sage-review-completeness: required` error but first wrote that posture into the
todo ticket's working-tree frontmatter. A retry after an external completeness
review then produced a duplicate key when the caller reasonably added
`sage-review-completeness: completed` near the other Sage field.

## Expected

A mutation tool that reports failure should either leave the ticket unchanged or
return an explicit partial-mutation result that identifies the written posture.
Investigate whether validation can precede persistence, or whether the MCP result
and playbook must make the intentional self-healing write contract explicit and
duplicate-safe.

## Decision

Keep the self-healing legacy-schema-migration write on a blocked/failed
`tickets.move`; do not switch to validate-before-persist (that would drop the
auto-migration for tickets stuck on the old single-field schema). Instead,
make the tool's returned result surface an explicit, hard-to-miss
partial-mutation notice whenever it wrote frontmatter on a call that then
blocked or failed — the caller is normally an agent, not a human, so the
notice needs to be loud in the structured/text result, not just in a doc
comment, so a retry never assumes the file is unchanged.

Implementation notes (design review, resolved autonomously):
- `TicketsMove` currently discards the computed `sageReviewPostures` on error
  and returns a bare result plus a Go error; the MCP layer's shared
  `toolTextResponse(req.ID, "", err)` generic-error path then drops any
  payload. Surfacing the notice requires (1) not discarding the partial
  result inside `TicketsMove` on error, and (2) special-casing the
  `tickets.move` error branch in `server.go` to merge that partial info into
  the response — the shared generic error helper cannot carry it as-is.
- `TicketsClose` has the same write-then-possibly-fail shape (frontmatter
  date field written before `atomicGitMove` can still fail). Out of scope for
  this ticket; a future ticket should generalize if this pattern recurs there.
- Once the notice contract is designed, `ai-docs/spec/mcp-tools.md` (MCP
  result contracts) likely needs a matching update — anticipate a `spec:`
  entry at ready-promotion time rather than treating it as optional.

## Phases

### Phase 1: Surface a loud partial-mutation notice on blocked tickets.move

In `agents-plugin-tool/internal/wsdoc/tickets_mutate.go`, stop discarding the
computed `sageReviewPostures` when `TicketsMove` returns an error after
`prepareSageReviewForUpwardMove` already wrote frontmatter; carry that
partial-mutation info in the returned result. In
`agents-plugin-tool/internal/mcp/server.go`, special-case the `tickets.move`
error branch (it cannot reuse the shared generic `toolTextResponse(req.ID,
"", err)` path as-is) to merge the partial-mutation notice into the response
text/structure so a retrying caller (typically an agent) cannot mistake a
blocked move for a fully unchanged file. `TicketsClose`'s analogous
write-then-possibly-fail shape stays explicitly out of scope. Verification:
a targeted regression reproducing the 2026-07-13 scenario (blocked `to:
"ready"` move on a legacy-schema ticket) asserts the notice appears in the
tool result; `go test ./...` passes.

### Result (35a52688) - 2026-07-13

Implemented as designed: `TicketMutateResult` gained a `PartialMutationNotice`
string field, populated in `TicketsMove`'s error branch whenever
`prepareSageReviewForUpwardMove` already wrote frontmatter before the move
blocked/failed (reusing the existing `sageReviewPostureTip` formatter).
`server.go`'s `tickets.move` error branch special-cases this via
`toolErrorTextResponse` instead of the shared generic-error path, so the
notice survives into the MCP tool result text while preserving `isError:
true`.

Deviation: used a plain `string` field rather than the plan's illustrative
`*sageReviewPostures` pointer, since `sageReviewPostures` is unexported in
`wsdoc` — a pre-rendered string avoids leaking an internal type across the
package boundary. Judged a legitimate implementation-detail choice by fit
review, not a scope change.

`TicketsClose`'s analogous write-then-possibly-fail shape stays explicitly
out of scope, confirmed untouched by fit review.

Verification: new regression tests at both the `wsdoc` and MCP layers
reproduce the 2026-07-13 legacy-schema blocked-move scenario and assert the
notice's presence; both fail against pre-fix code (non-vacuous) and pass
after the fix. `go test ./...` passes across all packages. Correctness and
fit reviews both returned clean.

`ai-docs/spec/mcp-tools.md`'s `{#260620-ticket-move-tool}` anchor was updated
in the same change to document the actual notice shape/text.

## Spec Impact

`ai-docs/spec/mcp-tools.md` documents `tickets.move`'s result contract and
needs a matching addition once the notice's exact field/text shape is
settled during implementation. Deferred to doc closeout rather than
contract-first, since the shape is explicitly open per the design review
(see Implementation notes above) and this is failure-path text, not a new
externally-relied-on schema.

Addressed: see Result above; `{#260620-ticket-move-tool}` now documents the
notice.
