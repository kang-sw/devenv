---
title: "Survey plan is a route, not the contract: quote the ticket verbatim, implement to the ticket, review the plan against it"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260908-feat-ws-pi-agent-session-disk-retention: the Pi-track incident that exposed this (child session files landed in tmpdir although the ticket said otherwise)
---

# Survey plan is a route, not the contract: quote the ticket verbatim, implement to the ticket, review the plan against it

## Background

Incident (Pi track, postmortem 2026-09-08). Ticket
`260902-feat-ws-pi-native-mvp` Phase 2 said child sessions use
"`--session <ws-owned-path>` (sibling of `~/.pi/agent/sessions/`, hidden
from the `/resume` picker)". The survey plan (`acc421c7`) restated this as
"ws-owned `sessionPath` (fresh temp path outside `~/.pi/agent/sessions/`)",
keeping the purpose clause and dropping the location. The implementer
followed the plan exactly, three partitioned reviewers found plan and diff
consistent, the ticket Result listed no deviation, and five later tickets
built durability (resume, sidecar revival, approval dir, cap eviction) on a
file living in the one directory the OS may delete.

Two sage reviewers on the ticket and three code reviewers on the diff did
not catch it because none of them is asked to compare the plan with the
ticket, and the current playbooks make the plan the contract:

- `agents-plugin/rsrc/plan-populator-survey/plan-populator-survey.md:94`
  and `:116`: `## Relevant Ticket Contract` is a "clipped authority
  requirement", i.e. the survey paraphrases the contract in its own words.
  Paraphrase is where the location was lost.
- `agents-plugin/rsrc/implementer/implementer.md:27` and `:31`: "The plan
  and its listed references are the task contract" and "Do not read
  ticket files directly unless the plan's `Escalations` section explicitly
  authorizes ticket-file reading". The implementer cannot see the ticket,
  so a plan-level rewording is law. Same lines in `implementer-relay` and
  `implementer-elevated`.
- `agents-plugin/rsrc/lead-implement/lead-implement.md` reviewer prompt
  frame: "Review the supplied authority, plan contract, and diff together"
  plus "Plan guardrails were not bypassed" and "Each specified authority
  requirement is implemented". Nothing says the ticket wins where the plan
  differs, or that a difference is itself a finding; with a plan in hand,
  plan-versus-diff is what a reviewer actually does.

Owner direction (2026-09-08): adding a fourth reviewer on the plan is the
wrong shape; the pipeline already stacks reviewers and still misses. The
survey should be a compressor and a guide, sweeping the codebase for what
a code reviewer would miss (reuse points, hidden constraints, patterns,
shortcut risks), and nothing more. The ticket stays the contract. The
implementer implements along the route the survey proposes but achieves
the ticket contract. Code reviewers read ticket and survey together and
judge the diff against the ticket.

## Direction

1. **Survey quotes, never paraphrases.** `## Relevant Ticket Contract`
   carries the selected phase's requirement text verbatim (with the
   ticket path and line range), plus verbatim Decisions/Constraints lines
   that govern the phase. Any survey judgment about the contract goes to
   `## Codebase Findings` or `## Escalations`, never into the contract
   section. The plan template's "clipped" wording changes to "quoted".
2. **Plan steps are the route; the quoted contract governs.** In
   `implementer` (and `-relay`, `-elevated`): the quoted contract section
   is the task contract; `## Implementation Plan` is the recommended path.
   Where a step and the quoted contract disagree, the contract wins and
   the implementer reports the disagreement instead of following the
   step. The "do not read ticket files" rule can stay, because the
   contract now travels verbatim inside the plan, which keeps the context
   budget the rule exists for.
3. **Reviewer checks the plan against the ticket, as one existing check.**
   The reviewer prompt frame's required checks gain one line: the plan's
   quoted contract matches the ticket phase text, and any implementation
   step that contradicts the ticket is a finding even when the diff
   matches the step. No new reviewer, no new partition; the full-scope
   reviewer and the Correctness partition own it.
4. **Result "Deviations" is plan-diffed.** `executor-wrapup` asks for
   deviations as the difference between the ticket phase text and what
   landed, not as what the implementer recalls.

Not in scope: sage reviewers on tickets, review allocation counts,
research-populator behavior beyond inheriting rule 1 when it refines a
survey plan.

## Open questions

- Whether `## Relevant Ticket Contract` should quote the whole phase or
  only the requirement sentences; a whole-phase quote is simplest and the
  phases this repo writes are short.
- Whether the same verbatim rule applies to inline contracts (probably
  yes: the accepted inline text is pasted, not restated).

## Phases

### Phase 1: Survey quotes and implementer follows the quoted contract

Rules 1 and 2. Edit `plan-populator-survey`, `implementer`,
`implementer-relay`, `implementer-elevated`; run the plugin tests that pin
playbook text; update `ai-docs/manuals/skill-authoring.md` invariants if
the contract wording is listed there.

### Phase 2: Reviewer frame and wrap-up deviations

Rules 3 and 4. Edit the reviewer prompt frame in `lead-implement`,
`code-reviewer`, and `executor-wrapup`; update `ai-docs/spec/workflow-skills.md`
where the review frame is specified.
