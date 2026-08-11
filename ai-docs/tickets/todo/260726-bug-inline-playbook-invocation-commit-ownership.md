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

> Superseded 2026-08-11 by `## Reframe` below (owner-settled in-session).
> Retained as the rejected ownership-transfer approach; do not implement from
> this section.

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

> Revised 2026-08-11 for the caller-local squash approach (see `## Reframe`).

- Target spec area: `ai-docs/spec/workflow-skills.md` — the playbook invocation
  and doc-commit granularity contract.
- Expected caller-visible change: a spawning playbook that treats an inline
  invocation as part of its own logical unit squashes the callee's intervening
  commit(s) into its own doc-commit (soft-reset to a captured base, then a single
  `git.commit`), distinct from the implement-merge squash. Callee commit behavior
  and direct invocation are both unchanged.
- Contract-first spec: no. The behavioral rule is stated here; the squash
  mechanism (a `git.commit` option vs. a small helper) settles during
  implementation.

## Phases

### Phase 1: Transfer commit ownership to the outermost invocation [dropped]

> Dropped 2026-08-11 — the ownership-transfer approach is superseded by the
> caller-local squash in `## Reframe`. Plan text below is retained as the
> rejected approach; implement from Phase 2.

- **Survey, with a stated boundary.** Corpus: `agents-plugin/rsrc/**/*.md`.
  "Invoked inline" means a playbook that calls `playbook.print` on another
  playbook and *continues its own procedure* afterward, as opposed to handing off
  and ending. Record the result in this phase's `### Result`. If the survey finds
  only `lead-write-spec`, proceed; if it finds several with differing commit
  shapes, stop and re-plan rather than generalizing a mechanism inside this phase.
- **Make the commit conditional on invocation mode.** Leading candidate:
  `playbook.print` already takes a `context` object ("Optional caller-supplied
  substitution values for variables declared in the playbook's frontmatter"), so
  the caller sets a variable the callee's step 6 branches on. This gives the
  fail-safe for free — an unset variable renders as direct mode, i.e. commit as
  today — so a caller that forgets to signal reproduces the current behavior
  rather than silently dropping a commit. The survey may override this choice.
- **Widen the Output handoff to carry a path set.** The current template reports
  a single `Path:` line; inline mode must enumerate every changed path, including
  `ai-docs/_index.md` when step 3d touched it.
- **State the ownership rule in the callee's own text.** That is the diagnosed
  defect — the rule exists only caller-side, where the callee never reads it — so
  the requirement is specifically that `lead-write-spec`'s own Invariants (or
  step 6 itself) carry it. Not `agents-plugin/rsrc/subagent-rules.md`: its header
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

### Phase 2: Squash inline-callee doc commits at the spawning call site

Supersedes Phase 1. Callee commit behavior is unchanged; the fix lives entirely
in spawning playbooks that treat an inline invocation as part of their own
logical unit.

- **Re-run the survey, with the same boundary.** Corpus:
  `agents-plugin/rsrc/**/*.md`. "Invoked inline" means a playbook that calls
  `playbook.print` on another playbook and *continues its own procedure*
  afterward. Enumerate the opt-in call sites — those where the caller intends the
  inline invocation as part of one logical unit (the former Category A). Record
  the result in this phase's `### Result`. Do not reuse the preserved 2026-07-27
  survey output: it overstates the opt-in set by one site
  (`lead-write-ticket.md:106`, removed by the marker-mechanism retirement);
  `lead-discuss.md:62` (the ticket-Drop branch) is the known surviving instance.
- **Add the squash mechanism.** A spawning playbook captures the base commit
  before the inline invocation (`git rev-parse HEAD`) and, after the callee
  returns having committed, folds the callee's intervening commit(s) into its own
  doc-commit via soft-reset to that base + a single
  `{{.McpNamespace}}/git.commit`. Soft-reset (not rebase or amend) so the
  combined change re-enters the `git.commit` validation gate established by
  `260723-feat-ticket-write-verify-commit-gate`. The base SHA must be threaded
  explicitly — shell state does not persist between tool calls (AGENTS.md
  invariant). Prefer a `git.commit` option (e.g. a `squash_from: <sha>` parameter
  that soft-resets to `<sha>` before committing) or a small helper over
  hand-rolled `git reset` prose in the playbook.
- **Opt-in per call site.** Only spawning playbooks that assert the inline
  invocation is part of their own unit adopt the squash. Category B
  (callee-commit is the caller's intent: `lead-implement.md:76` -> `lead-update-spec`,
  `lead-forge-spec.md:262` -> `lead-forge-mental-model`) does not adopt it and is
  left unchanged. Nothing is stated callee-side; no invocation-mode variable is
  introduced.
- Regenerate both rsrc artifacts if any `agents-plugin/rsrc/**/*.md` changed
  (`WSRSRC_REGEN=1`, `WS_REGEN_WSFLOW_RSRC=1`).

Fail-safe posture (explicit trade-off): if a spawning caller omits the squash,
the result is today's two-commit granularity — both commits valid and
individually revertable — not a hard error or a dropped commit. This is a
strictly better failure floor than the retired invocation-mode variable, which
`internal/wsrsrc/loader.go`'s `ErrUnprovidedVar` would have turned into a
hard-fail of every context-less invocation. The Phase-1 constraint "no parameter
without a fail-safe fallback" is met differently: there is no callee parameter at
all, and a caller-side omission degrades to the benign floor.

Verification boundary:

1. A spawning playbook that adopts the squash (e.g. `lead-discuss`'s ticket-Drop
   branch) produces exactly one commit for its logical unit, containing both the
   callee's changes and the caller's own — no path dropped.
2. The combined commit passes through the `git.commit` validation gate (the
   squash is soft-reset + re-commit, not a gate-bypassing history edit).
3. Callee playbooks are byte-unchanged; a direct invocation of any callee still
   commits exactly as today.
4. Category B call sites are unchanged and still produce their intended separate
   commits.
5. The re-run survey result and its corpus are recorded in `### Result`.

## Blocked (2026-07-27) — RESOLVED 2026-08-11 (superseded by `## Reframe`)

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

Renumbered since. Two playbooks this ticket cites by step number moved:

- `lead-write-spec`'s unconditional `git.commit` is now **step 6**, not step 7.
  Its former step 6 accuracy check was a verbatim restatement of Invariants line
  14 and was deleted.
- `lead-update-spec`'s commit, in its `### 6. Finalize` step, is now **step 6**,
  not step 7. The retirement deleted that playbook's former §5 marker-strip step
  and renumbered §6 and §7 down.

The two Phase 1 bullets that carried the old `lead-write-spec` number were
corrected in place — "the callee's step 7 branches on" and "(or step 7 itself)
carry it" now say step 6. Phase 1 has no `### Result`, so its plan text is still
editable, and both are implementation instructions: left uncorrected they would
send an implementer to `**Output Handoff**`, a step that commits nothing, while
the real unconditional commit kept firing.

The remaining old numbers are left as written, as records of what each passage
said when it was written:

- `## Background` `:19`, "`lead-write-spec` step 7 is an unconditional commit",
  and the fenced block under it, which reproduces the pre-renumber playbook line
  verbatim as evidence and is retained as a quotation of what the file said then.
- `## Decisions`, "step 7 already commits it conditionally".
- `## Blocked`, "`lead-sprint.md:97` -> `lead-update-spec`, whose step-7 commit
  pre-empts lead-sprint's own Doc Commit Gate" — that one names
  `lead-update-spec`'s old number, not `lead-write-spec`'s.

`## Prior Art` carries no step number at all.

## Category C dissolved by the sprint/salvage retirement (2026-07-28)

`260726-chore-retire-sprint-salvage-relocate-skill-authoring` Phase 1 deleted
`lead-sprint` and `lead-salvage`. That removes both of this ticket's non-Category-A
call sites:

- **Category C is empty.** `lead-sprint.md:97` -> `lead-update-spec` was the only
  Category C entry, and `## Blocked` calls it "the decisive one" on the grounds
  that "the same callee, `lead-update-spec`, needs opposite behavior from two
  different callers." Only one of those callers survives. `lead-implement`'s
  `{doc-pre-pass}` still wants `lead-update-spec` to commit; nothing now wants the
  opposite. The stated reason a callee-side rule cannot work no longer holds.
- **Category B loses one of three entries.** `lead-salvage.md:90/91/92/94` ->
  `lead-write-ticket` is gone. `lead-implement.md:76` and `lead-forge-spec.md:262`
  are unaffected.
- The follow-up question "whether `lead-sprint.md:97` is a separate doc-gate
  pre-emption bug in its own right" is moot — the file and its Doc Commit Gate
  invocation are both deleted.

This does not by itself unblock the ticket: the second blocker (whether commit
ownership is a property of the call site or of the callee) is a real design
question that outlives the retirement, and the preserved Phase 1 survey output
still needs re-running for the reason already stated above. But the re-plan should
not open by re-deriving a conflict that no longer exists.

## Reframe (2026-08-11): caller-local squash supersedes ownership-transfer

Settled with the owner in-session. Supersedes `## Decisions` and clears both
blockers recorded in `## Blocked (2026-07-27)`. Implement from Phase 2, not
Phase 1.

The original fix rewrote the commit-ownership contract: an inline-invoked callee
would stop committing, return its changed paths, and let the caller stage them.
That had a wide blast radius (every inline callee's commit behavior) and hit two
blockers — a callee-side rule cannot express per-caller intent (blocker 1), and
the "unset variable = direct mode" fail-safe does not exist because
`ErrUnprovidedVar` hard-fails an unset declared variable (blocker 2).

New approach: leave *who commits* untouched. The callee commits as today. A
spawning playbook that treats an inline invocation as part of its own logical
unit squashes the callee's intervening commit(s) into its own doc-commit —
soft-reset to a captured base, then a single `git.commit` — separate from the
implement-merge squash. Why this is better:

- **Blocker 1 (call-site vs callee) resolved as call-site.** Granularity is
  decided by the spawning caller, which is exactly where a squash policy lives.
  Different callers differ freely; Category B simply does not squash. The
  2026-07-28 Category C dissolution already removed the only
  same-callee-opposite-behavior case, so nothing forces a callee-side rule.
- **Blocker 2 (fail-safe variable) moot.** No variable is passed to the callee,
  so `ErrUnprovidedVar` never enters.
- **Failure floor flips favorably.** A forgotten squash degrades to today's two
  valid commits, never a hard-fail or a dropped commit.

Preserved from the original `## Constraints`: the inline invocation is kept (the
defect is commit granularity, not the call). Retired: the invocation-mode
variable and the callee-side ownership rule. The survey re-run over
`agents-plugin/rsrc/**/*.md` is still required (see Phase 2) — the preserved
2026-07-27 survey overstates the opt-in set by one site.

## Downgraded ready -> todo (2026-08-11)

Owner-decided in-session, not dropped. After the reframe, working through the
squash mechanism reduced this ticket's benefit to a single thing: **revert
atomicity** (one commit = one reversible unit; and, only when the split spec
commit omits the ticket stem, `git log --grep` completeness) for the one rare
surviving call site (`lead-discuss.md:62`, the ticket-Drop branch). The message
content is byte-identical whether split or squashed, so message preservation is a
constraint on the fix, not a benefit of it — nothing else is gained.

Given low severity, low frequency, and no independent reuse for a
`git.commit squash_from` primitive (implement-merge owns its own squash), a
`ready/` slot is not justified. The owner also holds that doc-commit
proliferation (thousands accumulating) is acceptable rather than a problem to
engineer around, which further lowers urgency; residual uncertainty about the
squash mechanism itself points the same way.

Not dropped: the design is captured and sound, and Phase 2 is a small,
mechanical, message-preserving `squash_from` when picked up. Revisit if the
split-commit granularity on the drop path ever becomes a felt pain.
