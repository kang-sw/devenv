---
name: lead-write-ticket
description: Create or update repository workflow tickets. Use when the user asks to write, create, edit, promote, drop, or close a ticket, or when a discussion needs to be captured as a durable ticket.
---

# Write Ticket

Target: user request

## Invariants

- Ticket conventions: call `ws/convention.read(name: "ticket-conventions")` - path format, status flow, phase rules, stem rules, templates.
- Never `read` a ticket file other than the current target - use `ws/subquery(question: "<focused ticket-inspection question>")`, then `ws/agents.result(name: <subquery-key>, timeout_seconds: 600)`, for any other ticket inspection.

## On: invoke

0. Apply **judge: spec-gate** (CREATE path only).
1. If `user request` references an existing ticket, read it.
2. **Create** (new ticket):
   a. Determine category from the topic.
   b. Choose initial status directory (`idea/` for vague, `todo/` for actionable - see `judge: initial-status`).
   c. Write the ticket using the **frontmatter template** and a clear problem/goal statement. Populate `related-mental-model` with the mental-model stems (filename without `.md`) that were consulted or arose during the current session - recovery hint for future sessions, not a validated link. Omit if no mental-model docs were relevant.
   d. If category is `epic`: body defines scope and decomposition (not implementation spec); list child ticket stems; completion means child work is done.
   e. If multiple phases are warranted (see `judge: phase-need`), structure as `### Phase N: <title>` sections. Note inter-phase dependencies explicitly.
   f. After drafting, verify scope - see `judge: ticket-scope`.
   g. If status is `todo/`: add an entry to the `## Ticket Queue` section in `ai-docs/_index.md`. Format: `` `stem` - one-line purpose and dependency notes ``.
3. **Edit** (existing ticket):
   a. Read the ticket first.
   b. Apply the requested changes (update phase, move status).
   c. For moves, use native `git mv` and add `completed:` date in frontmatter (-> `.done/`).
4. **Phase content** - capture goals, constraints, rationale, rejected alternatives, and suggested approaches. Leave codebase-derived details (paths, type reuse, integration patterns, signatures, testing classifications) to the plan.
5. **Intent review** - re-read the written/edited ticket against the preceding conversation:
   - Are decisions, constraints, rejected alternatives, and suggested approaches captured?
   - Does the ticket distort or omit any discussed intent?
   - Fix gaps in-place; present a brief summary of corrections (or confirm nothing was missed).
6. **Spec-stem check** - confirm ticket↔spec linkage:
   a. Use `ws/specs.find` or `ws/specs.status` to confirm canonical stems.
   b. Ensure the ticket frontmatter `spec:` field lists every stem the phases implement. Add missing stems. If a phase implements behavior with no spec entry, see `judge: missing-spec-entry`.
   c. Remind: commits implementing this ticket should include a `## Spec` section with those stems.
7. **Commit** - call `ws/git.commit(paths: ["<ticket-path>"], title: "<title>", ai_context: ["<bullet>"])`; include `ai-docs/_index.md` when the queue changed.
8. **Proceed prompt** - suggest `ws:lead-proceed` as the next step after ticket authoring, unless `judge: missing-spec-entry` fired in step 6. Proceed routes to skeleton, plan, or implementation based on artifacts and session warmth.

   Emit the created ticket path on its own final line: `Ticket: ai-docs/tickets/<status>/<stem>.md`. Callers such as `ws:lead-proceed` capture this path from prefix-stage output.

## Judgments

### judge: spec-gate

Fires on any action that results in `todo/`-or-higher status: direct `todo/` creation and `idea/` -> `todo/` promotion moves. `idea/` creation is ungated.

Identify the relevant spec file for the topic.
Use `ws/specs.find` or `ws/specs.status` if a relevant spec file or stem is identifiable.
If no relevant spec file exists, or no entry covers this behavior -> stop. Name the uncovered behavior; suggest `ws:lead-write-spec` before continuing.

### judge: initial-status

Place in `idea/` when the topic is exploratory or underspecified; place in `todo/` when the scope and goal are actionable. When uncertain, prefer `idea/` - promotion is cheap.

### judge: ticket-scope

Over ~200 lines is a soft signal; over 300 lines, act. First, prune plan-level detail (file paths, function signatures, integration specifics) - that belongs in a plan document. If still large, the scope is too wide: introduce an epic and split into child tickets, each covering one independently reviewable unit of work.

### judge: phase-need

Prefer more phases over fewer. An overly granular ticket is cheaper to merge than an oversized phase that stalls mid-implementation. Single-component, single-concern work may be one phase.

### judge: missing-spec-entry

Fires when a phase implements caller-visible behavior with no entry in any spec file. Stop the authoring flow, tell the user which phase surfaces un-specced behavior, and suggest `ws:lead-write-spec` before continuing. Skipping this loses traceability for the new behavior and bypasses the canonical chain's spec-impact gate.

## Doctrine

A ticket is the primary context-recovery artifact. Every choice optimizes for
**recoverability of intent**: capture decisions, constraints, and rejected
alternatives when writing so downstream skills never re-derive settled context.
When ambiguous, preserve recoverability.
