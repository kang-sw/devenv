---
title: "lead-backfill-docs: entry skill for retroactive spec and mental-model reconciliation"
sage-review-design: completed
sage-review-completeness: completed
related:
  260726-chore-retire-sprint-salvage-relocate-skill-authoring: removed the last ad-hoc door to the doc pass
  260726-bug-inline-playbook-invocation-commit-ownership: this ticket adds lead-update-spec's second caller
related-mental-model:
  - workflow-skills
completed: 2026-07-28
---

# lead-backfill-docs: entry skill for retroactive spec and mental-model reconciliation

## Background

Work sometimes lands without going through `lead-implement` — a direct edit, a
quick fix, a session that never routed through `lead-proceed`. Those commits are
real behavior changes but no doc pass ever sees them.

Two procedures already exist to fix that: `lead-update-spec` audits a commit
range for caller-visible changes and writes spec entries, and
`mental-model-updater` sweeps a range for domain knowledge and writes
mental-model docs. Neither is reachable without a caller.

Until 2026-07-28 there were two callers. `lead-implement`'s `{doc-pre-pass}`
requires having gone through implement. `lead-sprint`'s wrap-episode was the
other, and it refused without a `Sprint-Edit:` marker — which is precisely the
ad-hoc case it would have needed to cover. That skill is now retired, so one
door remains and it is the wrong one.

The gap is an entry point, not a capability.

## Decisions

**Entry skill, not an internal procedure.** The user must be able to type
`/ws:lead-backfill-docs` with no prior session, because the triggering situation
is "a pile of commits nobody documented" and there is no in-flight workflow to
hang it off. Entry-skill count 12 -> 13.

**The unit is a group list, not one range.** Undocumented commits are not
contiguous — chunk 2 and chunk 7 can be bare while 3-6 are covered. A single
`A..B` range either drags in already-documented commits or forces an arbitrary
cut. Discovery returns N groups, each a coherent behavior change, each mapping to
a range `lead-update-spec` already accepts. This also dissolves the "range too
wide, run multiple rounds" problem: one delegate per group, and freshness falls
out of grouping rather than needing a round-management rule.

**Do not author a new "is this documented enough" heuristic.** Two criteria
already exist — `judge: spec-impact` in `lead-update-spec`, and the inclusion
test in `mental-model-conventions`. A third definition in a discovery playbook
would drift from both. "Under-documented" is also two questions with different
answers: an internal refactor fails spec-impact but can strongly pass the
mental-model inclusion test.

**Discovery is delegated; spec authoring is not.** `lead-update-spec` Invariant 1
is `Lead-driven - no subagent delegation` and its Doctrine says "no delegation,
no suggestion mode". `mental-model-updater` already respects the same boundary
from the other side: it audits spec coverage but "never author or edit spec
content; surface suspected omissions as flags only. The lead owns
caller-visibility and spec-impact judgment." The architecture exists; this
ticket wires an entry point into it rather than inventing a parallel one.

**The delegate returns candidates, not verdicts, and never eliminates a group.**
Following from the rule above: if the discovery agent's spec judgment were
authoritative, spec-impact judgment would have moved into a delegate. So the
delegate reports what changed and what the current docs already say about it,
marks candidacy, and the lead applies `judge: spec-impact` when it runs
`lead-update-spec` on each group. A group it judges already covered is still
reported, as `none` naming the covering stem — dropping it would make the
delegate's screen load-bearing. Only empty-diff and doc-only groups are excluded.

**The delegate is not handed a lead playbook stem.** The obvious way to avoid a
duplicated criterion is to have the delegate call
`playbook.print(name: "lead-update-spec")`. Rejected:
`idea/260626-research-playbook-print-lead-surface-leak` records that
`playbook.print` has no role gate and that the standing defense is obscurity — a
delegate must not already know lead stems. Writing one into a delegate prompt
converts that defense into a handout. The delegate reads
`convention.read(name: "spec-conventions")` instead, which is delegate-reachable
by design. That convention carries no qualifies/does-not-qualify triage table —
only its external-perspective Definition and Refactor test — so what the delegate
applies is narrower than `judge: spec-impact`: is this change observable from
outside the project, and does an existing stem already describe it. That is the
convention applied as a filter, not a third criterion. The delegate also reads
`mental-model-conventions` for the mental-model half; the two questions have
different answers and neither convention answers both.

**Spec is per group; mental model is one sweep.** The two halves do not share a
unit and forcing them to was the original design error. Spec entries map to
discrete behaviors, so they group and each group earns its own commit. Mental-model
domains do not: `mental-model-updater` step 4 reads `ai-docs/mental-model.md` and
*every* file under `ai-docs/mental-model/` regardless of range, so a per-group
dispatch would re-read the whole corpus once per group and let two delegates edit
the same domain document from partial views. One sweep over the audit window,
after all spec passes, is both cheaper and safer.

Spec still runs first, matching `lead-implement`'s `{doc-pre-pass}` and satisfying
`mental-model-updater` step 3, which inspects "the scoped spec diff to identify
spec headings that add assessment targets" — with a single trailing sweep those
spec commits are inside the window by construction.

**One `docs(spec):` commit per group.** Not a batch. Beyond producing more useful
history, this is what keeps `260726-bug-inline-playbook-invocation-commit-ownership`
unblocked — see Constraints.

## Constraints

**Commit ownership must not regress.** This ticket adds `lead-update-spec`'s
second caller. That bug ticket's `## Blocked` argues a callee-side commit rule
cannot work because "the same callee, `lead-update-spec`, needs opposite behavior
from two different callers", and the sprint/salvage retirement emptied its
Category C by leaving one caller. Both surviving callers must want the same
thing. `{doc-pre-pass}` wants `lead-update-spec` to commit; backfill wants a
commit per group, so it wants that too. Category C stays empty. A design that
batched groups into one commit would need `lead-update-spec` to *not* commit, and
would silently reopen what the retirement closed.

**The single mental-model sweep dissolves two hazards rather than mitigating
them.** Sage review found both: widening a group's range to `<group-start>..<spec-commit>`
swallows every intervening commit, because the spec commit sits at HEAD while a
group's commits sit arbitrarily deep; and `mental-model-updater` step 1 resolves
scope "from the last `mental-model-updated` checkpoint; if absent, use the
caller-provided base", so caller scope is the fallback, not the authority — after
the first group's delegate committed, every later group would have scoped to
`<previous-commit>..HEAD` and silently seen none of its own source commits.

Both existed only because the design dispatched per group. With one trailing
sweep over `<base>..HEAD`, the range needs no widening (the spec commits are
inside it) and the checkpoint-first rule becomes *correct* rather than hazardous:
the checkpoint is the `(mental-model-updated)` marker the audit floor was derived
from, so the delegate resolves to exactly the intended window. No override
instruction, no separate backfill delegate, and no change to a callee
`lead-implement` also calls.

**The marker floor is a high-water mark and is reported as one.** The default
window runs from the newest `(mental-model-updated)` commit and the newest
`ai-docs/spec/` commit — whichever is older — to HEAD. Both are high-water marks,
so the default window finds only drift newer than the last documentation pass.
Gaps *below* it, which the grouping rationale itself cites, are invisible unless
the caller supplies an explicit `..` range. The skill reports this bound with its
result rather than implying full coverage. The floor is also resolved once, before
the first dispatch, because each group's mental-model commit advances it; a run
interrupted mid-way cannot resume from a re-derived floor and must be given the
original range.

**Discovery is read-only.** It never edits spec, mental-model, or source files.

## Prior Art

- `mental-model-updater` — existing medium-tier delegate; writes mental-model
  docs, commits with `(mental-model-updated)` in the body, and reports
  `## Spec Coverage Gaps` flags. Reused unchanged, including its checkpoint-first
  range resolution, which the single-sweep dispatch makes correct rather than
  hazardous.
- `lead-update-spec` — existing lead-inline rsrc playbook; resolves a range,
  applies `judge: spec-impact`, writes entries, verifies the index, commits.
  Reused unchanged.
- `reference-discovery` — shape reference for a read-only discovery delegate that
  returns annotated candidates rather than decisions.
- `lead-discuss` — shape reference for the parallel-init entry shim, the form used
  by skills that may be invoked with no session key.

## Spec Impact

Target area: `ai-docs/spec/workflow-skills.md`.

Caller-visible change: a new directly-invocable entry skill `/ws:lead-backfill-docs`
that reconciles spec and mental-model coverage for commits that never went
through an implementation doc pass. The entry-skill enumeration and count move
from 12 to 13, and `lead-update-spec` gains a second documented caller. A new
delegate playbook `doc-gap-discovery` joins the render-only surface.
`lead-discuss` gains an outbound route to the new skill (Phase 3), recorded as an
inbound-route clause on the same entry.

## Phases

### Phase 1: Author the discovery delegate and the entry skill

Two new playbooks plus the shim and mirror surfaces.

- `agents-plugin/rsrc/doc-gap-discovery/doc-gap-discovery.md` — `kind: render`,
  `role: delegate`, `tier: medium`. Agent Layout per the authoring manual.
  Groups a commit window into coherent behavior changes and reports, per group,
  what changed and what the current docs already say. Applies `spec-conventions`
  and `mental-model-conventions` as its only criteria and states none of its own.
  Read-only, and never drops a group for lacking candidacy.
- `agents-plugin/rsrc/lead-backfill-docs/lead-backfill-docs.md` — `kind: print`,
  choreography shape (`Invariants` -> `On: invoke` -> `Judgments` -> `Templates`
  -> `Doctrine`). No `enter.*` tool exists for this flow, so Layer 2 does not
  apply and the routing-coverage audit does not run.
- `agents-plugin/skills/lead-backfill-docs/SKILL.md` plus `agents/openai.yaml`,
  in the parallel-init form.
- wsflow mirrors: `agents-plugin-wsflow/skills/lead-backfill-docs/SKILL.md` and
  the regenerated `rsrc/` mirror. The `rsrc/` tree is generated — regenerate,
  never hand-edit.
- Manifests via the three env-gated regen tests.
- Test surfaces: `EXPECTED_SKILLS` and `EXPECTED_PARALLEL_INIT_SKILLS` plus
  `PARALLEL_INIT_TITLES` in `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`;
  a golden print test in `agents-plugin-tool/internal/mcp/playbook_tools_test.go`.
- `ai-docs/ref/wsflow-mirroring.md` Included list.

The audit floor is resolved deterministically before any agent runs: the last
commit whose body carries `(mental-model-updated)`, and the last commit touching
`ai-docs/spec/`. An explicit user-supplied `..` range overrides both. Only the
residue needs judgment, which is what the delegate is for — a medium-tier agent
should not be spending its window rediscovering a marker scan.

Left to the playbook to decide and therefore stated here: what to do when
neither floor exists (a project that has never run a doc pass). This is a soft
call — the honest audit window is the whole history and that is usually not what
the user wants — so it belongs in `Judgments`, not in a hardcoded default.

### Result (707236ae) - 2026-07-28

Phase 1 complete across two commits: `707236ae` authored the surface and
`efe795eb` applied the sage-driven single-sweep revision.

Both playbooks landed as planned. `doc-gap-discovery` reads `spec-conventions`
and `mental-model-conventions` as its only criteria, with step 1 stating
explicitly that it must not supplement them with a qualifies/does-not-qualify
list of its own — the convention carries the Definition and Refactor test but no
triage table, and without that sentence a medium-tier delegate would have
improvised the third criterion the design forbids. Group boundaries are a
top-down first-match table (merge commit / differing `## Ticket Updates` stem /
disjoint paths / doc-only) rather than prose, and `## Excluded` is explicitly
barred from carrying "already covered" so a screened group cannot vanish.

`lead-backfill-docs` runs six steps: resolve the window, discover groups,
reconcile spec group by group, sweep mental model once over `<base>..HEAD`,
reconcile residual `## Spec Coverage Gaps` flags, report. `judge: absent-floor`
covers the never-documented project — merge-base when HEAD differs from `main`,
ask the user when it does not.

Shims took the parallel-init form (`playbook.print` + `workflow_manual` in
parallel), matching `lead-discuss` and `lead-goal-fan-out-step`; the wsflow
`SKILL.md` is hand-maintained because `lead-backfill-docs` is not in
`substitutionMirroredSkills`, which is the established pattern for entry skills.
All three manifests plus the wsflow `rsrc/` mirror came from the env-gated regen
tests; nothing was hand-edited.

`test_wsflow_skill_bundle.py` needed a third edit beyond the two planned: the
`pointer_tail` dict inside `test_parallel_init_skill_files_are_playbook_shims`
is keyed per-skill deliberately, so adding to `EXPECTED_PARALLEL_INIT_SKILLS`
alone raised `KeyError`. Its "Both parallel-init skills" comment is now "Every".

`TestPlaybookPrintGoldenLeadBackfillDocs` asserts the doctrine plus the two
invariants that encode contested decisions (`never delegate spec authoring`,
`never once per group`), and separately prints `doc-gap-discovery` to assert it
contains no `lead-update-spec` string — the delegate must not learn a lead stem,
per `idea/260626-research-playbook-print-lead-surface-leak`. Both content
assertions were mutation-checked; because `printPlaybook` validates the manifest
hash before content, the mutated body had to be regenerated into the manifest
first to reach them, then restored by manual edit and regenerated again.

### Phase 2: Spec and mental-model updates

- `ai-docs/spec/workflow-skills.md` — entry enumeration and count 12 -> 13; a
  behavior entry for the skill; `lead-update-spec`'s second caller.
- `ai-docs/spec/documentation-system.md` — the doc-pass surface description
  currently names `lead-write-spec`, `lead-update-spec`, and `lead-write-ticket`;
  add the retroactive entry point.
- `ai-docs/mental-model/workflow-skills.md` — entry-skill surface and the
  caller relationship.

Verification: `ws/spec_index_verify` after the spec edits, `ws/tickets.verify` on
this ticket path, and the full Go and wsflow suites again — the entry-skill count
in `workflow-skills.md` and the roster in `test_wsflow_skill_bundle.py` are two
hand-maintained copies of the same fact, so a spec-only edit can still break the
bundle test. Re-run the three regen tests if any `rsrc/` body changed during this
phase.

### Result (707236ae) - 2026-07-28

Phase 2 complete, in the same two commits as Phase 1 (`707236ae`, `efe795eb`).

`spec/workflow-skills.md` gained the namespace-block entry, the count move 12 ->
13 with its enumeration, the wsflow shipped list, and the anchored behavior entry
`{#260728-retroactive-doc-backfill-entry}`. The `lead-update-spec` paragraph now
names both callers. The render-eligible "five" clause was scoped rather than
renumbered, with a note that `doc-gap-discovery` joins the render-only surface.
`spec/documentation-system.md` gained a paragraph placing the skill as the
retroactive entry point for both document kinds.
`mental-model/workflow-skills.md` gained the bullet under the same anchor.

Two drift items left by the sprint/salvage retirement were fixed on contact: the
`spec/workflow-skills.md` frontmatter summary still said "sprint work", and the
mental model still said "sprint-edit episode closure". A pre-existing gap was
also filled — three parallel-init enumerations named only two of the three
skills, having never been updated when `lead-goal-fan-out-step` was added.

Verification: `spec_index_verify: ok`, `tickets.verify` clean, full Go suite
green across 12 packages, 10 wsflow tests OK. The count-and-roster coupling the
phase predicted did hold — the bundle test is what caught the missing
`pointer_tail` entry.

### Phase 3: Route stale-doc observations from `lead-discuss`

Added after Phase 2, on the observation that the new entry skill had zero inbound
routing: it was reachable only by a user who already knew it existed.
`lead-discuss` is where stale documentation is actually noticed, so it is the
natural referrer.

Extend `lead-discuss`'s existing Evidence invariant rather than adding a bullet,
and state the discriminator that separates backfill's case from a plain
spec-authoring gap.

### Result (8c584079) - 2026-07-28

The invariant now reads:

```text
- When docs are stale or insufficient, say so; do not speculate. When the
  staleness traces to commits that never had a doc pass, name
  `{{.SkillNamespace}}:lead-backfill-docs`.
```

Three choices worth recording. Extending the existing bullet rather than adding
one: the "docs are stale" condition is already owned by that line, and splitting
it lets a pressured reader act on "say so" and never reach the route. The
`traces to commits` clause: docs nobody ever wrote are a spec-authoring gap, and
routing those to backfill spawns a discovery delegate that returns no groups.
The verb `name` rather than a hand-off: `lead-discuss` carries `Never proactively
ask to wrap up or persist; wait for the user's explicit signal`, so it surfaces
the route and lets the user choose.

Blast radius was checked before editing. `TestPlaybookPrintGoldenLeadDiscuss`
asserts only the doctrine and the `Continuity tip`, so it was unaffected, and
discuss's routing rules are not generally enumerated in spec — the `lead-tune`
disambiguation line has no spec counterpart — so only the new backfill entry
gained a clause recording the inbound route. Mirror confirmed at
`agents-plugin-wsflow/rsrc/lead-discuss/lead-discuss.md:22`; full Go suite green,
10 wsflow tests OK, `spec_index_verify: ok`.

A `lead-workflow-manual` pointer was considered and not added: the manual's
`lead-tune` line is a topic pointer, not a skill roster, and the manual does not
enumerate entry skills.

## Out of Scope

- **MCP-ifying the grouping.** Floor resolution and merge-chunk grouping are
  deterministic and could become `enter.backfill_docs` (Lever B, per
  `260630-epic-skill-playbook-diet`). Premature before the playbook has run
  against real drift.
- **Changing `lead-update-spec` or `mental-model-updater`.** Both are reused as
  they are. If backfill needs behavior neither provides, that is a separate
  ticket, not a quiet edit to a shared callee with another caller. The single-sweep
  design removes the one place this was under pressure: no change to
  `mental-model-updater`'s range contract is needed, so `lead-implement`'s sweep
  behavior is provably untouched.
- **A `(spec-updated)` checkpoint marker** symmetric to `(mental-model-updated)`.
  It would make the spec floor exact instead of approximate, but it is a
  commit-body convention change affecting every downstream project.


## Resolution (2026-07-28)

Shipped as designed. `/ws:lead-backfill-docs` is the retroactive entry point for both document kinds, reusing `lead-update-spec` and `mental-model-updater` unchanged — no callee contract moved, so `lead-implement`'s doc pass is provably untouched and `260726-bug-inline-playbook-invocation-commit-ownership` Category C stays empty.

Two design corrections came from sage review and are worth carrying: the mental-model half is a single window sweep rather than a per-group dispatch (a per-group dispatch would have re-read the whole corpus per group and let two delegates edit one domain from partial views), and the discovery delegate reads `spec-conventions` instead of a lead playbook stem, which keeps `idea/260626-research-playbook-print-lead-surface-leak`'s obscurity defense intact.

Known bound, accepted: the default audit window starts at a high-water marker, so gaps below the last documentation pass need an explicit `..` range from the caller. The skill reports the bound with its result rather than implying full coverage.

No MCP-layer work was required — the flow has no `enter.*` tool, which is recorded in Out of Scope as a possible Lever B follow-up once the playbook has run against real drift.
