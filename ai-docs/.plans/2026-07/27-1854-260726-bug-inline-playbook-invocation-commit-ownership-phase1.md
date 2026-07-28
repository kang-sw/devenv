# Plan: 260726-bug-inline-playbook-invocation-commit-ownership — Phase 1: Transfer commit ownership to the outermost invocation

## Relevant Ticket Contract

- **Stated stop condition (this is what fired).** Phase 1 bullet 1: "Corpus:
  `agents-plugin/rsrc/**/*.md`. 'Invoked inline' means a playbook that calls
  `playbook.print` on another playbook and *continues its own procedure*
  afterward... If the survey finds only `lead-write-spec`, proceed; if it finds
  several with differing commit shapes, stop and re-plan rather than
  generalizing a mechanism inside this phase."
- Commit ownership belongs to the outermost invocation; an inline-invoked
  playbook returns changed paths and does not commit.
- The rule must be stated where the callee can see it — `lead-write-spec`'s own
  Invariants or its step 7. Explicitly not `agents-plugin/rsrc/subagent-rules.md`.
- Direct invocation is unchanged: `lead-write-spec` run on its own still commits.
- Fail-safe is mandatory: an unset/forgotten mode signal must render as direct
  mode, never as a silently-dropped commit. Leading candidate is
  `playbook.print`'s `context` object with a frontmatter-declared variable —
  "The survey may override this choice."
- Output handoff must widen from a single `Path:` line to a path set, because
  step 3d also writes `ai-docs/_index.md`.
- Both rsrc regen commands must run from `agents-plugin-tool/`, both with
  mandatory `-count=1`.
- Constraint: do not solve this by removing the inline invocation. Do not
  introduce a caller-passed parameter without a fail-safe fallback.

## Out of Scope

- `260726-refactor-retire-spec-planned-marker-mechanism` step 2.4, which deletes
  the `lead-write-ticket.md:106` call site. This ticket lands first by decision.
- The `🚧` marker question itself (belongs to the retirement ticket).
- Downstream ws-consumer projects; only `agents-plugin/rsrc/` and its generated
  `agents-plugin-wsflow/rsrc/` mirror are in scope.

## Codebase Findings

### Survey result — corpus `agents-plugin/rsrc/**/*.md`, 46 `.md` files, 15 `playbook.print` occurrences

Classification applies the ticket's literal test: caller invokes another
playbook via `playbook.print` **and continues its own procedure afterward**.
The material sub-question is whether the callee commits unconditionally.

**Category A — inline + callee commits + caller then commits its own unit (true defect, matches the ticket's diagnosis):**

- `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md#L106` — Spec-address
  Check step 3 calls `lead-write-spec` inline, then continues: writes `spec:`,
  drops `## Spec Impact`, runs §4 Verify, §5 Commit (`#L61-L64`), §6 Sage Review
  Gate. Callee `lead-write-spec` commits unconditionally at
  `agents-plugin/rsrc/lead-write-spec/lead-write-spec.md#L37`. **This is the
  ticket's known instance.**
- `agents-plugin/rsrc/lead-discuss/lead-discuss.md#L62` — **second, previously
  unrecorded instance of the same shape.** Ticket Status Transition, Drop branch
  step 3b calls `lead-write-spec` inline "to close the linked spec entry", then
  continues to 3c/3d (`tickets.close`) and step 4 (`#L65`) "Commit through
  `git.commit`". Identical defect: the callee commits the spec removal, then the
  caller commits the ticket move — one logical drop, two commits.

**Category B — inline + callee commits, and separate commits are the caller's explicit intent (a general "callee never commits inline" rule would break these):**

- `agents-plugin/rsrc/lead-implement/lead-implement.md#L76` — `{doc-pre-pass}`
  prints and executes `lead-update-spec` inline, then continues to
  `{doc-commit-gate}` and Closeout. Callee `lead-update-spec` commits
  unconditionally — and does so as a **hard Invariant**:
  `agents-plugin/rsrc/lead-update-spec/lead-update-spec.md#L13` "Commit all spec
  changes in a single `docs(spec): ...` commit", plus step 7 at `#L59-L62`.
  The caller *wants* this: `lead-implement.md#L78` states "Commit spec and
  mental-model changes separately when both changed."
- `agents-plugin/rsrc/lead-salvage/lead-salvage.md#L90,#L91,#L92,#L94` — four
  inline `lead-write-ticket` invocations, three of them explicitly "separately
  for each child ticket" / "for each approved rewrite, drop, absorb, or status
  move". Caller continues to step 7 report and has **no commit step of its own**.
  Callee `lead-write-ticket` commits at `#L61-L64`. One commit per ticket is the
  intended granularity; transferring ownership here would leave nobody committing.
- `agents-plugin/rsrc/lead-forge-spec/lead-forge-spec.md#L262` — chains into
  `lead-forge-mental-model` inline on a yes answer, then "continue to step 4".
  Callee commits at `agents-plugin/rsrc/lead-forge-mental-model/lead-forge-mental-model.md#L177`
  and `#L193` ("Commit with `(mental-model-updated)` in the message body") —
  per-domain commits, deliberately granular.

**Category C — inline + callee commits, and the correct owner is genuinely contested:**

- `agents-plugin/rsrc/lead-sprint/lead-sprint.md#L97` — wrap-episode step 5 calls
  `lead-update-spec` inline, then continues to step 9 (`infra.read`
  `executor-wrapup`, "follow Doc Pipeline and **Doc Commit Gate**") and step 10
  "Commit documentation changes only after the Doc Commit Gate passes." The
  callee's step 7 commit lands **before** the caller's gate at
  `agents-plugin/rsrc/executor-wrapup.md#L23-L32` ever runs. This is worse than
  the ticket's instance — a validation gate is pre-empted, not just granularity —
  yet the same callee under `lead-implement` (Category B) is supposed to commit
  separately. **The same callee needs opposite behavior from two callers.**

**Excluded by the ticket's own definition (hand off and end, or not playbook-to-playbook inline):**

- `lead-discuss.md#L59` → `lead-write-ticket` — "Stop after it returns." Ends.
- `lead-discuss.md#L71` → `lead-write-ticket` — caller's remaining steps 4-5 are
  clarification/reporting only, with no commit of its own. Benign.
- `lead-goal-fan-out-step.md#L18` → `lead-proceed` with a **worker** session_key —
  subagent dispatch, not inline execution.
- `lead-proceed.md#L14` — routing invariant, not an inline call.
- `lead-workflow-manual.md#L12` — self-reload instruction; the manual commits nothing.
- `lead-forge-spec.md#L270` — post-wrap-up suggestion text, not a continuation.

**Count: 4 distinct callees invoked inline while committing unconditionally
(`lead-write-spec`, `lead-update-spec`, `lead-write-ticket`,
`lead-forge-mental-model`), across 8 continuing call sites, in 3 mutually
incompatible commit shapes.** The ticket's stop condition — "several with
differing commit shapes" — is met.

### Independent finding: the ticket's leading fail-safe mechanism does not behave as assumed

The ticket proposes a frontmatter-declared variable set through
`playbook.print`'s `context`, asserting "an unset variable renders as direct
mode, i.e. commit as today". **The runtime does the opposite — it hard-errors.**

- `agents-plugin-tool/internal/wsrsrc/loader.go#L351-L361` — for each declared
  variable whose `{{.Name}}` placeholder appears in the body, if the caller did
  not supply it: `return "", ErrUnprovidedVar{Name: name}`. There is no default
  value mechanism (frontmatter `variables:` is a plain `[]string` —
  `agents-plugin-tool/internal/wsrsrc/loader.go#L259-L262`).
- Substitution is not skippable: `buildPlaybookVars`
  (`agents-plugin-tool/internal/mcp/playbook_tools.go#L208-L263`) always returns
  a non-nil map, so `loader.go#L134`'s `if vars != nil` guard is always true.
- Consequence: the moment `lead-write-spec.md` contains a `{{.InvocationMode}}`
  placeholder, **every** invocation that does not pass `context` fails to render.
  That includes the user-facing shim
  `agents-plugin-wsflow/skills/lead-write-spec/SKILL.md#L8`
  (`playbook.print(name: "lead-write-spec")`, no context) and
  `lead-forge-spec.md#L270`. This violates the binding decision "Direct
  invocation is unchanged".
- The inverse guard also exists: an *undeclared* caller context key is a loud
  error (`playbook_tools.go#L214-L219`, `ErrUndeclaredVar`), so the caller side
  cannot pass a signal opportunistically either.

So the mechanism is fail-loud-on-direct-invocation, not fail-safe-to-direct-mode.
Making it behave as the ticket assumes requires either a runtime change
(optional/defaulted declared variables in `wsrsrc`) — which is Go code, outside
this phase's stated `agents-plugin/rsrc/**/*.md` corpus and unbudgeted by the
phase — or a different mechanism entirely.

### Constraints the re-plan must respect

- `agents-plugin/rsrc/` is mirrored into `agents-plugin-wsflow/rsrc/`;
  `ai-docs/ref/wsflow-mirroring.md#L36-L58` lists `lead-write-spec`,
  `lead-write-ticket`, `lead-update-spec`, `lead-discuss`, `lead-implement`,
  `lead-sprint`, `lead-forge-spec`, `lead-forge-mental-model` as **shipped**
  wsflow skills — every Category A/B/C participant except `lead-salvage`, which
  is explicitly excluded. Any rule change is user-visible in both distributions.
- Per `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md`, the
  Invariant checklist (Falsifiable · Actionable · One line · Context-free ·
  Non-redundant · Doctrine-aligned) applies to the new ownership line, and
  "State each rule once: if an Invariant already captures a constraint, remove it
  from handler steps" — so the rule goes in Invariants **or** step 7, not both.

## Implementation Plan

- Escalate to research before execution.

The phase's own stop condition fired. Do not generalize a commit-ownership
mechanism inside this phase. Specifically, do **not**:

- patch only `lead-write-spec` and call the rule general — `lead-discuss.md#L62`
  is a second Category A site the ticket never recorded, and Categories B/C would
  silently inherit a rule that breaks them;
- write the frontmatter-`context` mechanism as specified — it fails loud on every
  direct invocation instead of falling back to direct mode.

## Verification Plan

The ticket's five-point boundary remains the target once research settles the
mechanism. These are prompt/playbook documents, so no point is machine-provable
end-to-end; each is a documentary check plus a regeneration/consistency test.

1. *One commit for the spec-address branch* — not executable as a test. Verified
   by reading the final `lead-write-ticket.md` Spec-address Check step 3 and §5
   Commit together and confirming the returned path set (spec file **and**
   `ai-docs/_index.md`) is named in the §5 `git.commit(paths: [...])` argument.
   Reviewer artifact: the diff of `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`.
2. *Direct `lead-write-spec` still commits* — render it with no context and
   confirm success plus an unconditional commit step:
   `ws/playbook.print(name: "lead-write-spec")` and inspect the returned body.
   A render error here is an outright failure (see the `ErrUnprovidedVar` finding).
3. *No mode signal commits as today (fail-safe)* — same render as point 2; the
   rendered step 7 must read as an unconditional commit when no signal is
   present. If the settled mechanism is a declared variable, add a Go test beside
   `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L687` asserting that
   printing `lead-write-spec` with an empty `context` succeeds.
4. *Survey result recorded* — `### Result` under Phase 1 of
   `ai-docs/tickets/ready/260726-bug-inline-playbook-invocation-commit-ownership.md`
   names the corpus (`agents-plugin/rsrc/**/*.md`) and the per-call-site
   classification above. Checked by reading the ticket.
5. *Callee states the rule* — `grep` the ownership sentence in
   `agents-plugin/rsrc/lead-write-spec/lead-write-spec.md`, and confirm it is not
   duplicated into `agents-plugin/rsrc/subagent-rules.md`.

Mechanical checks that must pass regardless of mechanism, from `agents-plugin-tool/`, in order:

```
WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest
WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
```

Both `-count=1` flags are mandatory; `agents-plugin-wsflow/rsrc/` is never
hand-edited. Then, from the repo root:

```
python3 -m unittest discover agents-plugin-wsflow/tests
go test ./... -count=1        # from agents-plugin-tool/
```

The wsflow package test is required because every Category A/B/C playbook except
`lead-salvage` is a shipped wsflow skill (`ai-docs/ref/wsflow-mirroring.md#L36-L58`).

## Escalations

- Confidence: **high** (in the escalation itself; low that light planning could
  land this safely).
- Reason: the phase's own stated stop condition fired. The survey found 4 distinct
  callees invoked inline while committing unconditionally, across 8 continuing
  call sites, in 3 mutually incompatible commit shapes — including a case
  (`lead-update-spec`) where the same callee must commit under one caller
  (`lead-implement.md#L78`, separate docs commit is the documented intent) and
  must not under another (`lead-sprint.md#L97`, whose Doc Commit Gate at
  `executor-wrapup.md#L23-L32` is pre-empted by the inner commit). Independently,
  the ticket's leading mechanism is contradicted by the runtime: an unprovided
  declared variable returns `ErrUnprovidedVar` (`loader.go#L351-L361`) rather than
  rendering as direct mode, so it breaks direct invocation instead of failing safe.
- Research should decide:
  1. Whether "inline callee does not commit" is a general rule with an opt-out,
     or a per-call-site contract the **caller** declares. Categories B and C
     suggest ownership is a property of the call site, not of the callee.
  2. How invocation mode is conveyed given that `context` variables are
     mandatory-once-declared. Options: (a) add optional/defaulted declared
     variables to `wsrsrc` (Go change, new phase or sibling ticket); (b) convey
     mode in the `Target: user request` free text the caller already supplies,
     with absence meaning direct mode — no frontmatter change, fail-safe by
     construction; (c) a shared auto-include the callee reads, stating that a
     caller who will commit says so.
  3. Whether `lead-sprint.md#L97` is a separate bug (gate pre-emption) that
     should be split out rather than folded into a commit-granularity fix.
  4. Whether `lead-discuss.md#L62` joins this ticket's Phase 1 scope as a second
     Category A site, or becomes its own phase.
