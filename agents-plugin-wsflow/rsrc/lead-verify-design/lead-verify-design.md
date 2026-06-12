---
kind: print
delegates: true
---
# Verify Design

Topic: design proposal under review

## Invariants

Premise
- Verify discussion premises before reviewing design quality.
- Do not build a design brief from materially false or blocker premises.
- Treat user preference as input, not evidence.

Isolation
- Review from a neutral brief file, not raw conversation.
- Separate evidence, constraints, preferences, unknowns, and rejected alternatives in the brief.
- Give the design reviewer only the brief path, review path, and review instructions.
- Ask for calibrated judgment, not generic criticism.
- Clean up temporary artifacts during Return.

Scope
- Review design quality, not post-implementation correctness.
- Do not edit source, specs, tickets, or durable docs unless the user requests persistence.
- State which instruction authorizes persistence before writing any persistent artifact.
- Write all generated artifacts in English.

## On: invoke

### 1. Premise Gate

1. Run discussion verification first by calling `ws/playbook.print(name: "lead-verify-discussion")` and executing the returned procedure inline; if unavailable, locally identify corrected premises, evidence, constraints, reuse opportunities, over-alignment risks, countercases, and unresolved unknowns.
2. If a material premise fails, return premise status, corrected premises, blocking reason, and next action; stop before design review.
3. If multiple plausible premises depend on an unmade user decision, ask one focused question, include the competing premise options, and stop before creating artifacts.
4. Treat a premise as blocking when the design review would rely on it and it is contradicted or unsupported.
5. If uncertainty remains but does not block review, carry it into `Unknowns`.

### 2. Neutral Brief

1. Call `ws/path.generate(kind: "review", stems: ["verify-design-brief", "verify-design-review"])`; capture `<brief-path>` and `<review-path>`; if path generation fails, report failure and stop.
2. Write `<brief-path>` using **Design Brief template**.
3. Audit the brief against corrected premises; remove persuasion, lead preference, and unsupported claims.
4. Keep user preferences only under `Preferences To Treat As Preferences`.

### 3. Fresh Review

1. Register a fresh unique reviewer through `ws.mercenary.register(name: "design-reviewer-<unique-suffix>", model: "deep")`; do not reuse an existing reviewer session.
2. Call `ws.mercenary.call(name: "design-reviewer-<unique-suffix>", prompt: <Design Reviewer prompt>)`.
3. Read `ws.mercenary.result(name: "design-reviewer-<unique-suffix>", timeout_seconds: 600)`; if the call fails or times out, delete artifacts and return review failure with premise status, failure reason, and next action.
4. If the reviewer wrote `<review-path>`, read it; otherwise use the final result text.
5. If neither `<review-path>` nor usable final text exists, delete artifacts and return review failure with premise status, failure reason, and next action.
6. Classify findings as fatal issue, important risk, minor polish, acceptable trade-off, reviewer-overreach, or out of scope; list simpler alternatives only under the separate Simpler Alternatives section.

### 4. Return

1. Evaluate `judge: persistence-needed` and report whether persistence is recommended.
2. Do not persist unless explicitly authorized or required by governing instructions.
3. Delete `<brief-path>` and `<review-path>` unless persistence is authorized.
4. Return **Output format**.

## Judgments

### judge: persistence-needed

Recommend persistence when any condition applies:

- The review changes a future workflow, API, architecture boundary, or cross-skill contract.
- Another session, agent, ticket, or spec will need the review result.
- The design decision rejects a plausible alternative that future work is likely to reopen.
- The review exposes a durable bug, dogfood surprise, or follow-up.

Do not persist when the result only helps the current conversation choose a local implementation shape.

## Templates

### Output format

```markdown
## Verdict
<keep | revise | reject | defer>

## Premise Status
<verified | corrected | blocked | degraded>

## Design Findings
### Fatal Issues
### Important Risks
### Minor Polish
### Acceptable Trade-Offs
### Reviewer Overreach
### Out Of Scope

## Simpler Alternatives
## Decision Blockers
## Persistence Recommendation
## Next Action
```

Use `None` for empty sections; do not omit headings.

### Design Brief template

```markdown
# Design Verification Brief

## Problem
<problem being solved>

## Corrected Premises
<facts and assumptions after premise verification>

## Proposed Design
<neutral description of the design under review>

## Intended User Experience
<caller-visible or workflow-visible outcome>

## Existing Mechanisms And Evidence
<project mechanisms, files, specs, or patterns that support or constrain the design>

## Constraints
<hard boundaries>

## Preferences To Treat As Preferences
<user or maintainer preferences that are not evidence>

## Unknowns
<uncertainties the reviewer must not treat as facts>

## Alternatives Considered
<known alternatives and why they are not the current proposal>

## Non-Goals
<explicitly excluded work>

## Review Questions
- Does the design fit the problem?
- Does it use the right ownership boundary and layer?
- Is the data flow, control flow, and state model simple enough?
- What hidden contracts or operational failure modes does it introduce?
- Can it be implemented and tested without special machinery?
- Is there a simpler existing mechanism that satisfies the same need?
```

### Design Reviewer prompt

```text
Brief path: <brief-path>
Review path: <review-path>

Read only the brief. Do not read other files, docs, tickets, specs, or prior
conversation. Treat preferences as preferences, not evidence. Treat unknowns as
unknowns, not facts.

Evaluate whether the design should be kept, revised, rejected, or deferred.
Do not force findings. Separate fatal issues, important risks, minor polish,
acceptable trade-offs, reviewer-overreach, simpler alternatives, and out-of-scope
concerns.

Write the review to the review path if possible, then report completion.
```

## Doctrine

Design verification optimizes for **judgment isolation**: verify premises first,
remove conversational pressure from the review input, then calibrate the fresh
reviewer so criticism improves the decision instead of manufacturing objections.
