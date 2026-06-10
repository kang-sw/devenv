---
name: lead-salvage
description: Use when a large implementation, sprint, branch, or agent run may need rollback, recovery, or salvage after a wrong premise; interactively preserve reusable work before destructive cleanup.
---

# Salvage

Target: user request

## Project Map

Call `ws/project_tree()`.

## Invariants

- No source edits; this skill freezes, audits, classifies, and routes recovery work.
- No destructive action before explicit immediate user approval.
- Preserve evidence before cleanup: branch, commit range, diff, logs, tickets, specs, and reviewer outputs.
- User confirms the failure claim before invalidated premises become workflow truth.
- User confirms invalidated premises before recovery tickets are created.
- Agent-only findings may mark `Unknown` or `Rework`; user approval is required for `Discard`.
- Classify affected tickets before moving, dropping, or rewriting them.
- Keep salvage reports in research tickets and recovery execution in epic or child tickets.
- Use the user's active conversation language for discussion responses.
- All written artifacts are English.

## On: invoke

1. Invoke `ws:lead-workflow-manual`.
2. Call `ws/git.status()`.
3. Identify target kind: branch, sprint, commit range, ticket graph, worktree diff, agent run, or user-described failure.
4. Enter **Containment**.

## On: Containment

1. Capture current branch, cleanliness, upstream, and obvious target range.
2. If the worktree or branch may be lost, ask before creating a rescue branch or tag.
3. Ask the user for the **Failure Claim**: what is wrong, what must not be trusted, and what must not be lost.
4. Restate the failure claim and ask the user to confirm or amend it.
5. Stop if the user cannot confirm a failure claim; suggest `ws:lead-discuss` for exploratory diagnosis.
6. Enter **Survey Fanout**.

## On: Survey Fanout

1. Select independent surveys:
   - **Code blast radius** - files, packages, tests, commits, and generated artifacts touched by the suspect work.
   - **Ticket graph** - active `idea/`, `todo/`, and `ready/` tickets that depend on suspect premises.
   - **Spec and mental-model impact** - planned or implemented behavior, documented invariants, and stale guidance affected by the failure.
   - **Evidence inventory** - logs, reviewer reports, plans, skeletons, screenshots, or external notes worth preserving.
2. Dispatch all independent survey calls first; store agent ids or names before collecting results.
3. Spawn a native Explore-style subagent via the `explore` playbook (see `lead-workflow-manual`) for one-turn bounded surveys; collect the result when it returns.
4. Use named agents for broad or stateful surveys:
   a. Register one agent per independent survey, such as `salvage-blast-radius`, `salvage-ticket-graph`, `salvage-doc-impact`, or `salvage-evidence`.
   b. Call each agent with the **Survey Prompt** for its assigned question.
   c. Collect each result through `ws/agents.result(name: "<agent-name>", timeout_seconds: 600)`.
5. Summarize survey outputs into the **Salvage Report** before treating them as durable evidence.
6. Do not convert survey outputs into decisions without user confirmation.
7. Enter **Premise Interview**.

## On: Premise Interview

1. Present the failure claim and candidate invalidated premises.
2. Ask the highest-level unresolved premise question first.
3. Descend only after the parent premise is accepted, rejected, or marked unknown.
4. Separate low-level bugs from premise collapse; do not patch while the parent premise is unresolved.
5. Move unconfirmed premise candidates to `Unresolved Premise Questions`; do not use them to justify recovery tickets.
6. Stop interviewing when the salvage report can classify evidence without inventing user intent.
7. Enter **Classification**.

## On: Classification

1. Classify artifacts into `Keep`, `Rework`, `Discard`, and `Unknown`.
2. Classify affected tickets into `Keep`, `Rewrite`, `Drop`, `Absorb`, and `Unknown`.
3. List `ready/` tickets that depend on invalidated premises under `At-Risk Ready Tickets` until disposition is approved.
4. Prefer `Unknown` over false certainty when evidence is incomplete.
5. Draft the **Salvage Report** and **Recovery Ticket Plan**.
6. Ask the user to approve or amend the report before any ticket creation or cleanup.

## On: Capture

Trigger: user approves the salvage report and ticket plan.

1. Invoke `ws:lead-write-ticket` to create or update one research ticket containing the salvage report.
2. Apply **judge: needs-recovery-epic**; if it fires, invoke `ws:lead-write-ticket` to create or update one recovery epic.
3. For concrete execution slices, invoke `ws:lead-write-ticket` separately for each child ticket.
4. Apply **judge: destructive-action** immediately before any destructive ticket, spec, or source cleanup.
5. For affected existing tickets, invoke `ws:lead-write-ticket` separately for each approved rewrite, drop, absorb, or status move.
6. If destructive source cleanup is still needed, route to the approved manual git action or implementation skill.
7. Report created or updated ticket paths, remaining unknowns, and the next user decision point.

## Judgments

### judge: needs-recovery-epic

Create a recovery epic when any condition holds:
- More than one active ticket needs disposition.
- Recovery spans multiple components, phases, or reviewable slices.
- Cross-child invariants are needed to prevent the failed premise from returning.
- Some recovery work is deferred while other work proceeds.

Skip the epic when one child ticket can capture the entire recovery and no active ticket graph is contaminated.

### judge: destructive-action

Treat these as destructive: `git reset`, branch deletion, file deletion, ticket drop, phase deletion, spec removal, and any rewrite that erases evidence. Ask for explicit approval immediately before execution, even if the report was already approved.

## Templates

### Survey Prompt

```text
Target: <branch/range/ticket/run>
Failure claim: <user-confirmed claim>
Question: <specific blast-radius, ticket-graph, spec, or evidence question>

Return:
- Findings with file, ticket, spec, or commit references
- Confidence per finding: high / medium / low
- Unknowns that need lead or user judgment
- Suggested classification only when evidence is concrete
```

### Salvage Report

```text
Failure Claim
- <user-confirmed failure>

Frozen Evidence
- <branch/range/diff/log/ticket/spec/reviewer refs>

Invalidated Premises
- <user-confirmed premise> - <evidence>

Unresolved Premise Questions
- <candidate premise> - <evidence needed before it can drive recovery work>

Blast Radius
- Code:
- Tickets:
- Specs:
- Mental Models:
- Tests/Artifacts:

At-Risk Ready Tickets
- <stem> - <invalidated premise or unknown dependency>

Salvage Classification
- Keep:
- Rework:
- Discard:
- Unknown:

Affected Ticket Disposition
- Keep:
- Rewrite:
- Drop:
- Absorb:
- Unknown:

Recovery Ticket Plan
- Research report: <new/update>
- Epic: <new/update/none>
- Child tickets:
- Existing ticket moves:
```

### Recovery Epic

```text
Scope
- <restoration, replacement, or re-decision boundary>

Non-Scope
- <explicit exclusions>

Invalidated Premises
- <short pointers to the research salvage report>

Affected Tickets
- Keep:
- Rewrite:
- Drop:
- Absorb:
- Unknown:

Child Tickets
- `<stem>` - <repair slice/status/dependency note>
- Planned: <child ticket description>

Cross-Child Decisions
- <invariants all recovery children must preserve>

Completion Criteria
- Done: affected executable work no longer depends on invalidated premises.
- Dropped: recovery direction abandoned with affected work disposed.
- Deferred: unresolved remnants are explicitly moved to accepted backlog.
```

## Doctrine

Salvage optimizes for **evidence-preserving loss containment**. The scarce
resource is not code volume but trustworthy judgment after premise collapse:
freeze evidence, distribute surveys, force user confirmation at irreversible
boundaries, and convert contaminated work graphs into explicit recovery tickets.
