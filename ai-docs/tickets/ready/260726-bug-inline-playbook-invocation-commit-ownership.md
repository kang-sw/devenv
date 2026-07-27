---
title: A playbook invoked inline by another playbook commits on its own, splitting one logical unit across commits
related:
  260726-refactor-retire-spec-planned-marker-mechanism: extracted from the ticket that retirement drops; this defect survives that decision
  260726-bug-spec-planned-marker-ready-ticket-cycle: where this was found, as the blocking sage finding; dropped by the retirement
  260723-feat-ticket-write-verify-commit-gate: established ws/git.commit as the ticket-write chokepoint this defect routes around
sage-review-design: completed
sage-review-completeness: completed
---

# A playbook invoked inline by another playbook commits on its own, splitting one logical unit across commits

## Background

Found by a sage design reviewer as the blocking finding on
`260726-bug-spec-planned-marker-ready-ticket-cycle`, and extracted here because it
is a real defect independent of the `🚧` question that ticket was about.

`lead-write-spec` step 7 is an unconditional commit:

```text
7. **Commit** - call `{{.McpNamespace}}/git.commit(paths: ["<file>"], title: "<title>", ai_context: ["<bullet>"])`; include `ai-docs/_index.md` when the listing changed.
```

Note the trailing clause: the step already commits **two** paths, not one.

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
- **This ticket lands before `260726-refactor-retire-spec-planned-marker-mechanism`.**
  That ticket's step 2.4 removes the contract-first branch at
  `lead-write-ticket.md:106` — this ticket's *only known inline caller*. If the
  retirement lands first, the survey finds zero instances and the bullet about
  updating the Spec-address Check has no target, so the general rule would be
  written with its motivating case already deleted. Landing this first is also
  cheap: the fix is small and independent of the marker question. If the
  retirement does land first anyway, this ticket must **re-run the survey** rather
  than assume its instance survives.
- **The handoff shape is part of the fix, not an afterthought.**
  `lead-write-spec` step 3d also writes `ai-docs/_index.md`, and step 7 already
  commits it conditionally — so "returns its changed paths" is a *set*, while the
  current Output handoff template reports a single `Path:` line. Transferring
  ownership without widening the handoff would make the caller's commit silently
  drop the index update.

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

- **Survey, with a stated boundary.** Corpus: `agents-plugin/rsrc/**/*.md`.
  "Invoked inline" means a playbook that calls `playbook.print` on another
  playbook and *continues its own procedure* afterward, as opposed to handing off
  and ending. Record the result in this phase's `### Result`. If the survey finds
  only `lead-write-spec`, proceed; if it finds several with differing commit
  shapes, stop and re-plan rather than generalizing a mechanism inside this phase.
- **Make the commit conditional on invocation mode.** Leading candidate:
  `playbook.print` already takes a `context` object ("Optional caller-supplied
  substitution values for variables declared in the playbook's frontmatter"), so
  the caller sets a variable the callee's step 7 branches on. This gives the
  fail-safe for free — an unset variable renders as direct mode, i.e. commit as
  today — so a caller that forgets to signal reproduces the current behavior
  rather than silently dropping a commit. The survey may override this choice.
- **Widen the Output handoff to carry a path set.** The current template reports
  a single `Path:` line; inline mode must enumerate every changed path, including
  `ai-docs/_index.md` when step 3d touched it.
- **State the ownership rule in the callee's own text.** That is the diagnosed
  defect — the rule exists only caller-side, where the callee never reads it — so
  the requirement is specifically that `lead-write-spec`'s own Invariants (or
  step 7 itself) carry it. Not `agents-plugin/rsrc/subagent-rules.md`: its header
  states "delegates do not read this file directly" and it scopes to spawning
  general-purpose workers, not playbook-to-playbook invocation. A shared
  auto-include is acceptable *in addition*, if the survey finds multiple callees.
  Then update `lead-write-ticket`'s Spec-address Check to stage the returned paths
  into its own commit.
- Regenerate both rsrc artifacts (`WSRSRC_REGEN=1`, `WS_REGEN_WSFLOW_RSRC=1`).

Rejected alternatives: dropping the inline invocation (removes the feature, not
the defect); having the caller amend the callee's commit (`git.commit` owns
validation, and amending across a validated boundary hides what was checked);
leaving it and documenting the two-commit result as intended (the caller's own
Commit step already contradicts that).

Verification boundary:

1. `lead-write-ticket` running its spec-address branch produces exactly one commit
   containing the ticket, the spec file, and `ai-docs/_index.md` when the listing
   changed — no path silently dropped.
2. A direct `lead-write-spec` invocation still produces its own commit.
3. An invocation with no mode signal commits as it does today (fail-safe).
4. The survey result and its corpus are recorded in `### Result`.
5. `lead-write-spec`'s own text states the ownership rule.

## Blocked (2026-07-27)

**Phase 1's own stop condition fired on its first step.** The survey ran over the
stated corpus (`agents-plugin/rsrc/**/*.md`, 46 files, 15 `playbook.print`
occurrences) applying the ticket's literal test. It found not one inline-invoked
callee but **four, across 8 continuing call sites, in three incompatible commit
shapes** — which is exactly the case Phase 1 says to stop and re-plan on rather
than generalize a mechanism inside the phase. Survey output is preserved at
`ai-docs/.plans/2026-07/27-1854-260726-bug-inline-playbook-invocation-commit-ownership-phase1.md`.

- **Category A — the diagnosed defect.** `lead-write-ticket.md:106` ->
  `lead-write-spec`, the known instance, **plus a previously unrecorded second
  site**: `lead-discuss.md:62`, the ticket-Drop branch, which calls
  `lead-write-spec` inline and then continues to `tickets.close` and its own
  `git.commit` at line 65. One logical drop, two commits.
- **Category B — the callee committing is the caller's explicit intent.**
  `lead-implement.md:76` -> `lead-update-spec`, where line 78 literally says
  "Commit spec and mental-model changes separately"; `lead-salvage.md:90/91/92/94`
  -> `lead-write-ticket` four times, deliberately one commit per ticket, with the
  caller having no commit step of its own; `lead-forge-spec.md:262` ->
  `lead-forge-mental-model`, which commits per domain.
- **Category C — genuinely contested.** `lead-sprint.md:97` -> `lead-update-spec`,
  whose step-7 commit pre-empts lead-sprint's own Doc Commit Gate
  (`executor-wrapup.md:23-32`, invoked at sprint steps 9-10).

Category C is the decisive one: **the same callee, `lead-update-spec`, needs
opposite behavior from two different callers.** A rule stated callee-side cannot
express that, which undercuts this ticket's binding decision "State the rule where
the callee can see it." The re-plan question is whether commit ownership is a
property of the **call site** (caller-declared) rather than of the callee.

**Second, independent blocker: the ticket's named fail-safe does not exist.**
Decisions assume "an unset variable renders as direct mode, i.e. commit as today".
The runtime does the opposite — `internal/wsrsrc/loader.go:351-361` returns
`ErrUnprovidedVar` when a declared variable's placeholder appears in the body and
the caller supplied no value, `variables:` is a plain `[]string` with no default
mechanism, and `buildPlaybookVars` always returns a non-nil map so substitution is
never skipped. Adding `{{.InvocationMode}}` to `lead-write-spec` would therefore
hard-fail **every** context-less invocation, including the user-facing wsflow shim
at `agents-plugin-wsflow/skills/lead-write-spec/SKILL.md:8` — violating the binding
decision "Direct invocation is unchanged." It fails loud, not safe.

Both blockers need a decision this ticket did not settle, so implementation is not
resumable as written. Two follow-ups also surfaced and should be routed when the
re-plan happens: whether `lead-discuss.md:62` joins Phase 1's targets, and whether
`lead-sprint.md:97` is a separate doc-gate pre-emption bug in its own right.

## Landing-order inversion (2026-07-28)

`260726-refactor-retire-spec-planned-marker-mechanism` Phase 2 landed **first**,
against the `## Decisions` bullet that reserved that order for this ticket. It was
not preempted by choice: this ticket is blocked on its own Phase 1 stop condition
and needs an owner re-plan, so waiting on it would have stalled the retirement
indefinitely. The same bullet supplies the sanctioned fallback — "If the
retirement does land first anyway, this ticket must re-run the survey."

What the retirement removed:

- `lead-write-ticket.md:106`'s inline `lead-write-spec` invocation — the
  Spec-address Check's contract-first branch, this ticket's *named* Category A
  instance — no longer exists. Step 3 of **On: Spec-address Check** now ends at
  writing or updating `## Spec Impact`. `judge: contract-first-spec` was deleted
  from both `lead-write-ticket` and `lead-write-spec`.

What survives:

- `lead-discuss.md:62` (the ticket-Drop branch) still calls `lead-write-spec`
  inline and still continues to `tickets.close` and its own `git.commit` — so
  Category A is not empty, and the defect still reproduces. Categories B and C
  are untouched by the retirement; `lead-sprint.md:97` -> `lead-update-spec`
  remains the decisive Category C case.

Required on re-plan: **re-run the survey** over `agents-plugin/rsrc/**/*.md`
rather than reusing the preserved output at
`ai-docs/.plans/2026-07/27-1854-260726-bug-inline-playbook-invocation-commit-ownership-phase1.md`,
whose call-site inventory now overstates Category A by one site. The re-plan
question stated in `## Blocked` — whether commit ownership is a property of the
call site rather than of the callee — is unaffected and still open.

Renumbered since: `lead-write-spec`'s unconditional `git.commit` is now **step
6**, not step 7 (its former step 6 accuracy check was a verbatim restatement of
Invariants line 14 and was deleted); `## Background` and `## Prior Art` above
still name it by the old number.
