---
title: "lead-backfill-docs: entry skill for retroactive spec and mental-model reconciliation"
sage-review-design: completed
sage-review-completeness: completed
related:
  260726-chore-retire-sprint-salvage-relocate-skill-authoring: removed the last ad-hoc door to the doc pass
  260726-bug-inline-playbook-invocation-commit-ownership: this ticket adds lead-update-spec's second caller
related-mental-model:
  - workflow-skills
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

**Order is spec, then mental model, per group.** This matches `lead-implement`,
whose `{doc-pre-pass}` runs `lead-update-spec` first, and it is what
`mental-model-updater` step 3 expects: it inspects "the scoped spec diff to
identify spec headings that add assessment targets".

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

**The group range and the group's spec commit are two separate dispatch inputs.**
In backfill the spec commit lands *after* the code it documents, and
`mental-model-updater` step 3 wants a spec diff in scope. The obvious fix —
widening the range to `<group-start>..<spec-commit>` — is wrong: the spec commit
is at HEAD while a group's commits sit arbitrarily deep, so one contiguous range
spanning both swallows every intervening commit, including groups already
processed. That is precisely the over-wide range the group-list decision exists
to avoid. So the range is passed unmodified and the spec commit is named
separately, by hash, for the delegate to read directly. When a group produced no
spec commit — `lead-update-spec` reports `Spec: no changes.` and commits nothing
whenever `judge: spec-impact` rejects the whole group, which is the expected
outcome for a pure internal refactor — the input is `none`.

**Every dispatch must declare its range authoritative.**
`mental-model-updater` Process step 1 resolves scope "from the last
`mental-model-updated` checkpoint; if absent, use the caller-provided base" —
caller scope is the fallback, not the authority. Benign for `lead-implement`,
where the checkpoint precedes the branch. Inverted for backfill: once group 1's
delegate commits, the checkpoint becomes HEAD-adjacent, so group 2 would scope to
`<group-1-commit>..HEAD`, see none of its own source commits, and report no
changes without erroring. The dispatch prompt therefore states the range is
authoritative and forbids checkpoint rescoping. This is a caller-side mitigation
of a callee contract that does not accept an authoritative range; see Out of
Scope for why the clean fix is deferred.

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
  range resolution, which this ticket works around caller-side rather than edits.
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

## Out of Scope

- **MCP-ifying the grouping.** Floor resolution and merge-chunk grouping are
  deterministic and could become `enter.backfill_docs` (Lever B, per
  `260630-epic-skill-playbook-diet`). Premature before the playbook has run
  against real drift.
- **Changing `lead-update-spec` or `mental-model-updater`.** Both are reused as
  they are. If backfill needs behavior neither provides, that is a separate
  ticket, not a quiet edit to a shared callee with another caller. Specifically
  deferred: giving `mental-model-updater` an authoritative caller range instead of
  checkpoint-first resolution. That is the clean fix for the rescoping hazard
  above, but it is a cross-skill interface change on a callee `lead-implement` also
  calls, so it needs its own ticket and approval rather than riding this one.
- **A `(spec-updated)` checkpoint marker** symmetric to `(mental-model-updated)`.
  It would make the spec floor exact instead of approximate, but it is a
  commit-body convention change affecting every downstream project.
