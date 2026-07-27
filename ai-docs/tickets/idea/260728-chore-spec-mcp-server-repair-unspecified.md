---
title: the mcp-server-repair fallback route has no spec coverage at all, and a
  ticket already assumed it did
related:
  260726-chore-mcp-repair-pointer-mid-procedure-skills: its Spec Impact section
    asserted coverage that does not exist, which is how the gap surfaced
  260624-epic-pre-release-cleanup: item 8 is the repair-pointer coverage line this
    belongs under
---

# `mcp-server-repair` is unspecified

## Topic

`grep -rn "mcp-server-repair" ai-docs/spec/` returns nothing. The mechanism is
referenced across `ai-docs/ref/wsflow-mirroring.md`, four tickets, four plan
files, and `ai-docs/mental-model/workflow-skills.md` — but no spec entry defines
it.

This is not a case of a spec entry being thin or stale. There is no entry.

## Why it matters

`260726-chore-mcp-repair-pointer-mid-procedure-skills` opened its `## Spec Impact`
with "`ai-docs/spec/workflow-skills.md` already documents the `mcp-server-repair`
fallback as the MCP-down route", and concluded from that premise that the phase
needed no spec work. The premise is false, so the conclusion was reached for a
reason that does not hold — the phase happened to need no spec change anyway,
because it only propagated an existing convention, but the reasoning that got it
there was unsound.

That is the sharper problem. A `## Spec Impact` section is where a phase decides
whether it owes the spec anything, and it decided by citing a document that does
not exist. Nothing in the authoring or review path caught it; the phase's own
sage-review-completeness posture is `completed`.

The mechanism is now load-bearing for over twenty SKILL.md files in two trees,
each of which tells the reader to run `/ws:mcp-server-repair` or
`/wsflow:mcp-server-repair` when a `playbook.print` call cannot reach the server.
What that command must do, what it may assume about the failure, and what it
guarantees on return are defined only by its own implementation.

## Direction

Two separable pieces, worth deciding whether they are one ticket or two:

- **Specify the mechanism.** What the repair skill is for, what failure classes it
  addresses, what the pointer convention is (tree-namespaced command, appended to
  the `playbook.print` step), and which skills are exempt and why. The exemption
  rule matters — `lead-goal-step`, `lead-prefer-subagent`, and
  `lead-verify-discussion` carry inline bodies and make no `playbook.print` call,
  so they are legitimately pointer-free.
- **The false-premise class.** A `## Spec Impact` claim that names a spec document
  or anchor is checkable mechanically. `ws/spec_index.verify` already reads the
  spec index; a ticket-side check that every spec reference in a ticket body
  resolves would have caught this at authoring time. Worth scoping against how
  often such references appear and how many currently resolve.

`adbf5ec3` established the pointer wording and is the natural origin point for
whatever spec text gets written.

## Prior art

- `adbf5ec3` — introduced the pointer to four front-door skills, unspecified.
- `260726-chore-mcp-repair-pointer-mid-procedure-skills` — swept the pointer to
  every remaining `playbook.print` caller; its Phase 1 `### Result` records this
  gap and the reasoning for deferring rather than authoring spec text at the end
  of an already-reviewed phase.
- `260728-chore-ws-tree-skill-pointer-guard` — the enforcement-asymmetry sibling
  from the same phase.
