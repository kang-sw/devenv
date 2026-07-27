---
title: "reviewer worktree isolation — parallel review subagents can contaminate the shared working tree"
related:
  260723-feat-ticket-write-verify-commit-gate: surfaced during this ticket's Phase 1 review as a near-miss false critical
---

# reviewer worktree isolation — parallel review subagents can contaminate the shared working tree

## Background

During the partitioned review of `260723-feat-ticket-write-verify-commit-gate`
Phase 1, the correctness reviewer reported a **critical** finding: the stem
guardrail was disabled (`if false { addFinding("stem", ...) }`), letting a
malformed stem land, with two tests failing to confirm it.

Investigation showed the committed branch tip was **correct** — the defect
existed only as an **uncommitted, unstaged edit in the shared working tree**.
A review subagent (which runs as a general-purpose agent with `Edit`/`Write`
access) had apparently modified `tickets_verify.go` to `if false` — presumably
to experiment with making the guardrail fire — and never reverted it. A sibling
reviewer then ran the test suite against that dirty tree and observed genuine
failures, producing a plausible-but-wrong critical that nearly triggered a
wasted fix-relay cycle. The lead caught it only by cross-checking the committed
tip against the working tree.

## Why this matters

- **False criticals waste cycles.** A contaminating edit from one agent makes
  another agent's evidence-based finding wrong-but-convincing.
- **Contamination can escape.** Had the lead committed with a broad `git add`
  (rather than explicit-path `git.commit`), the stray edit could have shipped
  into the branch and silently regressed a guardrail — the existing test would
  have caught it in CI, but only by luck of that test existing.
- **Shared mutable state across parallel agents** violates the isolation the
  review fan-out assumes; reviewers are meant to be read-mostly observers of a
  fixed diff, not writers to a live tree.

## Open questions

- **Should review subagents run read-only?** Reviewers analyze a fixed diff
  range; do they ever legitimately need `Edit`/`Write`? If not, dispatch them
  with a read-only tool profile so contamination is impossible by construction.
- **Or isolate per-agent worktrees?** The Agent tool supports `isolation:
  "worktree"`. Should parallel reviewers (and possibly implementers) each get a
  throwaway worktree so no agent observes another's uncommitted state? Trade-off:
  worktree setup cost vs. contamination safety.
- **Lead-side guard.** Should the implement pipeline assert a clean working tree
  (modulo known artifacts like the plan file) before dispatching reviewers, and
  again before merge, failing loud on unexpected dirt? This is a cheap backstop
  independent of the isolation decision.
- **Scope.** Is this a ws-runtime concern (review-dispatch playbook mandates a
  profile/isolation), a workflow-skill-text concern (lead-implement review step),
  or both? Where does the fix live?
- **Generality.** Does the same hazard apply to any parallel-agent stage that
  shares one working tree (e.g. concurrent implementers)? The reshape toward
  fresh/isolated subagents (`260605` pivot) may already point at the answer.

## Recurrence evidence — 260726-refactor-retire-spec-planned-marker-mechanism

The hazard recurred across **both phases** of the same ticket, which raises it
from a one-off to a reproducible property of partitioned review in one checkout.

Phase 2, cycle 1 (range `d8d03fdb..4722ceea`, three concurrent partitions):

- The **test partition** ran seven mutation-test iterations, each applying an
  uncommitted single-hunk edit and reverting it by hand. This is legitimate work
  for that partition and it left the tree clean at the end.
- The **correctness partition**, running concurrently in the same checkout,
  observed those mutations appear and disappear under it in
  `internal/wsdoc/spec_discovery.go`, `internal/wsdoc/legacy_marker.go`,
  `internal/mcp/server.go`, and `internal/wsdoc/tickets_template.go`. Its first
  targeted `go test -run …` runs reported failures caused by the injected
  mutations rather than by the diff under review.
- It needed **7 attempts** to obtain a single verification window with
  `git status --porcelain` verified empty immediately before *and* immediately
  after the run.
- One mutation (an unused `fence` local in `legacy_marker.go`) was a **build
  break**, so during that window a sibling partition's targeted runs could not
  compile at all — a failure mode with no relation to the reviewed diff.

Two things this adds to the open questions above:

- The contaminating agent was **not** misbehaving. Mutation testing is the test
  partition's assigned method, and it restored every edit. Read-only reviewer
  profiles would therefore not fix this case; only per-agent isolation or
  serialization would.
- Recurrence across two phases of one ticket suggests the cost is not rare
  enough to absorb: the visible price here was seven wasted verification
  attempts plus one round of investigating failures that did not exist.
