---
title: A playbook invoked inline by another playbook commits on its own, splitting one logical unit across commits
related:
  260726-refactor-retire-spec-planned-marker-mechanism: extracted from the ticket that retirement drops; this defect survives that decision
  260726-bug-spec-planned-marker-ready-ticket-cycle: where this was found, as the blocking sage finding; dropped by the retirement
  260723-feat-ticket-write-verify-commit-gate: established ws/git.commit as the ticket-write chokepoint this defect routes around
sage-review-design: required
---

# A playbook invoked inline by another playbook commits on its own, splitting one logical unit across commits

## Background

Found by a sage design reviewer as the blocking finding on
`260726-bug-spec-planned-marker-ready-ticket-cycle`, and extracted here because it
is a real defect independent of the `🚧` question that ticket was about.

`lead-write-spec` step 7 is an unconditional commit:

```text
7. **Commit** - call `{{.McpNamespace}}/git.commit(paths: ["<file>"], title: "<title>", ai_context: ["<bullet>"])`
```

`lead-write-ticket`'s Spec-address Check invokes `lead-write-spec` **inline** —
not as a separate user-facing invocation. So a single logical unit of work
("create this ticket, addressing this spec") lands as two commits, with the
inner one committing spec changes before the outer procedure has finished or
validated anything.

**The rule already exists implicitly and is simply not honored.**
`lead-write-ticket`'s own Commit step says "separate follow-up invocations own
their own commits and outputs". The contrapositive — an inline invocation does
not own its commit — is exactly the missing rule, but it is stated only from the
caller's side, where the callee never reads it.

## Decisions

- **Commit ownership belongs to the outermost invocation.** A playbook invoked
  inline returns its changed paths to the caller and does not commit; the caller
  stages them together with its own edits.
- **State the rule where the callee can see it.** The current wording lives in
  `lead-write-ticket` and describes what *callers* do with follow-ups. A callee
  has no reason to read its caller's playbook. The rule needs to be visible from
  the invoked playbook's side, or in shared guidance both include.
- **Direct invocation is unchanged.** `lead-write-spec` run on its own still
  commits exactly as it does today. Only the inline path transfers ownership.
- **Not scoped to `lead-write-spec`.** It is the known instance, but the fix is a
  general rule, so the phase surveys which other playbooks are invoked inline and
  commit unconditionally rather than patching one call site.

## Constraints

- Do not solve this by removing the inline invocation. Producing the spec
  addressing inline is the point of the branch; the defect is the commit, not the
  call.
- Do not introduce a parameter the caller must remember to pass without a
  fallback that fails safe. An invoked playbook that commits when it should not
  is the current bug; a caller that forgets a flag reproduces it.

## Prior Art

- `260723-feat-ticket-write-verify-commit-gate` (done) made `ws/git.commit` the
  validation chokepoint for ticket writes. An inner commit still routes through
  that gate, so this is not a validation bypass — it is a granularity defect:
  the commit boundary stops matching the unit of work, which breaks
  `## Ticket Updates` attribution and makes the pair non-atomic to revert.

## Spec Impact

- Target spec area: `ai-docs/spec/workflow-skills.md` — the playbook invocation
  and commit-ownership contract.
- Expected caller-visible change: an inline-invoked playbook reports changed
  paths instead of committing, so one logical unit of work produces one commit.
  Direct invocation is unaffected.
- Contract-first spec: no. The behavioral rule is stated here; how invocation
  mode is conveyed (parameter, caller-set context, or shared guidance the callee
  reads) settles during implementation.

## Phases

### Phase 1: Transfer commit ownership to the outermost invocation

- Survey which playbooks are invoked inline by another playbook and end in an
  unconditional commit. `lead-write-spec` step 7 is the known instance; establish
  whether it is the only one before choosing a mechanism.
- Make the invoked playbook's commit conditional on invocation mode, returning
  changed paths to the caller otherwise, with a fail-safe default: if invocation
  mode is unknown, behave as today (commit), so a caller that fails to signal
  produces the current behavior rather than silently dropping the commit.
- State the ownership rule in shared guidance both caller and callee reach, and
  update `lead-write-ticket`'s Spec-address Check to stage the returned paths
  into its own commit.
- Regenerate both rsrc artifacts (`WSRSRC_REGEN=1`, `WS_REGEN_WSFLOW_RSRC=1`).

Rejected alternatives: dropping the inline invocation (removes the feature, not
the defect); having the caller amend the callee's commit (`git.commit` owns
validation, and amending across a validated boundary hides what was checked);
leaving it and documenting the two-commit result as intended (the caller's own
Commit step already contradicts that).

Verification boundary: `lead-write-ticket` running its spec-address branch
produces exactly one commit containing both the ticket and the spec change; a
direct `lead-write-spec` invocation still produces its own commit; an invocation
with no mode signal commits as it does today.
