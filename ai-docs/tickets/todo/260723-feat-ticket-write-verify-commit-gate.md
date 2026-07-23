---
title: "ticket.verify + commit-gate mechanical backstop, then must-not-forget mutation-tool collapse"
related:
  260627-bug-write-ticket-bypasses-tickets-create: closes the exact bypass hole its prose-only fix left open
  260624-feat-tickets-template-tool-and-convention-diet: prior offload of ticket text into MCP tools; this continues by moving validation to a gate
parent: 260723-epic-ticket-write-reshape
sage-review-design: required
sage-review-completeness: required
---

# ticket.verify + commit-gate mechanical backstop, then must-not-forget mutation-tool collapse

## Background

The ticket-write path has three structural problems:

1. **No mechanical backstop.** Mechanical guardrails (stem format, status/dir
   consistency, frontmatter integrity, sage-posture presence/value, spec-address
   presence, phase/Result structure) are scattered across `ticket_create.go`,
   `tickets.go`, and `tickets_mutate.go`, enforced only *before* commit by
   playbook discipline. A lead that hand-edits a ticket file bypasses all of it —
   the `260627` fix for this was prose-only, adding no enforcement.
2. **Round-trip weight.** The write path makes ~8-10 MCP calls
   (`convention.read`, `template`, `checklist`×2, `create`, `sage_gate`,
   `sage_record`, `git.commit`), each with schema + response overhead; agents
   already bypass the mutation tools and edit files directly.
3. **Front-loaded procedure.** The playbook restates each action's follow-on
   obligations up front rather than surfacing them at the moment of action.

`git.commit` already runs a validation step (`validateCommitStatus`) before
`git commit -m`, but it does **zero** ticket-content validation. That slot is the
natural host for a deterministic verify gate — the one chokepoint every path
(tool-mediated or hand-edited) must pass.

## Decisions

- **verify = mechanical floor; sage = semantic ceiling.** `ticket.verify` checks
  only file-state-deterministic guardrails and confirms the sage *result* stamp
  is present/valid; it never judges prose quality or design soundness.
- **Commit-gate makes verify non-optional.** Hosting verify at `git.commit`'s
  validation slot means an invalid ticket cannot be *landed*, even if the agent
  skipped a voluntary verify during editing. verify is also callable directly for
  red-green feedback mid-edit.
- **Must-not-forget filter governs tool survival.** Keep a thin tool only where it
  bundles a catastrophic-to-forget follow-on; free the rest to free-edit under the
  verify floor. Seed classification matches today's hard/soft choices (ready→sage
  is hard; close→phases-resolved is soft).
- **sage stamp is lead single-writer.** Reviewers return verdict prose only (no
  frontmatter write — avoids the concurrent design/completeness write race); a
  thin lead-only `sage.stamp` writes `completed`/`blocked` and renders the
  `## Blocked` companion.

### Rejected alternatives

- *Delete all ticket mutation tools, keep only verify* — rejected: something must
  still write frontmatter / move files; deletion targets *validation*, not
  *mutation*, and high-stakes fiddly writes (sage stamp) keep a thin tool.
- *Reviewers raw-edit the sage posture* — rejected: concurrent reviewers race on
  the same frontmatter; the lead is the single writer instead.

## Phases

### Phase 1: verify() + commit-gate backstop (pure addition)

Add a deterministic `ticket.verify(paths)` that runs the mechanical guardrail set
(stem regex, status/dir consistency, frontmatter fence integrity, ready-landing
sage-posture presence/value, ready-landing spec-address presence, phase/Result
structure, close date-field presence). Host it at `wsgit.Client.Commit` after
staging and before `git commit -m`, as a sibling of `validateCommitStatus`, so
every ticket-touching commit is gated; keep it callable standalone for mid-edit
feedback. This phase is purely additive — existing tools stay — and immediately
closes the `260627` direct-file-edit bypass hole. Returns actionable prose on
failure (which guardrail, which file, what to fix), with a bounded
retry/escalation contract so the red-green loop cannot thrash indefinitely.

### Phase 2: must-not-forget mutation-tool collapse + action-time obligation prose

With the verify floor in place, collapse the mutation surface:

- Rename `tickets.create` → an attention-salient name (e.g. `create_empty` /
  `create_template`) whose return prose states it yields a *valid empty skeleton +
  initial posture*, not a full mutation orchestrator.
- Free `tickets.close` to free-edit + soft verify-warn on unresolved phases (no
  hard block) — matches the flexible end of the filter.
- Keep the ready-move hard enforcement (sage posture) — the catastrophic end.
- Add the thin lead-only `sage.stamp` tool; reviewers stop writing frontmatter.
- Replace front-loaded playbook procedure with **action-time obligation prose**:
  each tool/gate return surfaces the must-know follow-on for that action.

## Spec Impact

- Target spec area: `mcp-tools` — new `ticket.verify` contract, the commit-gate
  behavior, the `tickets.create` rename, the freed `tickets.close` semantics, and
  the new `sage.stamp` tool with reviewers-return-prose-only.
- Expected caller-visible change: playbooks call a smaller mutation surface + a
  verify gate; direct file edits are now caught at commit; sage posture is written
  by a lead-only stamp.
- Contract-first spec: no — the exact tool signatures and prose are still
  design-level and will be refined during implementation; the ticket phases carry
  the behavioral intent and the spec update follows as closeout.
