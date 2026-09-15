---
title: "Move worker-tier selection to a dispatch-time qualitative lead judgment against a shared risk rubric"
related:
  260915-research-lead-run-tier-selection-qualitative: context; the research record whose Confirmed Decisions this ticket implements (evidence, not a landing dependency)
  260909-epic-ws-worker-interpreter-refoundation: context; owns the lead/worker interpreter surface this routing lives on
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 60c546a7435fe478
sage-review-completeness-reviewed: 60c546a7435fe478
completed: 2026-09-15
---

# Move worker-tier selection to a dispatch-time qualitative lead judgment against a shared risk rubric

## Background

`lead-run` currently picks the worker tier with a mechanical gate: any one of
four pre-graded (model-authored) risk axes being `high` maps the ticket to
`ticket-worker-elevated` @ `large` (`lead-run.md:53,59`). The grades are
produced by a model (`ticket-fact-populator`) at ready-promotion time
(`agents-plugin/rsrc/lead-ticket/lead-ticket.md:64`: "Run at actionable
promotion to `ready/`"), frozen in the ticket's `## Route Facts` table, and
stripped of their evidence column
by the projection parser (`wsdoc/tickets.go:560-561`), so the dispatch-time
resourcing decision runs on a reason-stripped, disjunctive, binary gate over a
lossy enum with a documented anti-low skew. The full analysis and its evidence
live in the research record `260915-research-lead-run-tier-selection-qualitative`.

This ticket implements the confirmed direction: the tier decision is a
resourcing judgment, not a safety gate, so it moves to the lead at dispatch —
the lead reads the selected ticket, analyzes its risk against a shared rubric,
and qualitatively picks the tier — while the mechanical dispatch facts stay
computed in MCP.

## Decisions

- **Tier is a dispatch-time qualitative lead judgment.** The lead reads the
  one selector-chosen ticket body at dispatch, grades its risk against the
  shared rubric, and picks `medium` / `large` / `xlarge`. This reopens the
  lead read-body ban (`lead-run.md:42`) for the selected ticket only.
  - Rejected: keep the mechanical OR-gate (structurally elevation-biased).
  - Rejected: the lighter "expose the evidence column, lead still reads only
    the projection enums" variant — kept in the research record as a fallback,
    not adopted, because holding the ticket's intent first-hand is the point.
- **The risk rubric ("the scale") is a bundled rsrc document** served through
  the same rsrc channel `lead-run` itself loads through, so lead, populator,
  and reviewers grade against one ruler.
  - Rejected: an inline rubric block in `lead-run.md` — no shared single ruler,
    and it bloats the shipped playbook.
- **`ticket-fact-populator` keeps grading the `risk.*` rows but demoted to
  advisory** (a first-pass hint); its tier authority is removed. The lead
  forms its own view from the ticket and the rubric.
  - Rejected: removing the risk rows entirely — larger blast radius on any
    consumer and it discards a useful hint.
- **Mechanical Route Facts stay computed in MCP** (`dispatch_blocked` /
  prerequisite edges, phase/result state, scope). Only the judgment moves out;
  the projection keeps its mechanical role.
- **The context-accumulation tradeoff is accepted.** Reading one ticket body
  per dispatched cycle is bounded per cycle but cumulative across a long goal
  drain; the durable record is the assignment note, and long runs are handled
  by compaction. Accepted by the user.
- **Decoupling model-tier from review-breadth is out of scope** and deferred to
  a separate follow-up ticket; the three worker playbooks stay a 1:1 tier
  ladder here.

## Constraints

Implementation-convention manuals that bind the touched paths (read before
editing; obligations are live, not pointers):

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)

Applied to this ticket's paths: `skill-authoring.md` governs `lead-run/`, the
new rubric doc, and `ticket-fact-populator/` (apply its invariant checklist to
every changed Invariants/Constraints line); `wsflow-mirroring.md` governs the
byte-identical `agents-plugin-wsflow/` (and pi) mirrors, guarded by
`TestWsflowRsrcMirrorUpToDate` / `TestPiMirrorUpToDate`;
`shipped-surface-boundary.md` requires the rubric and reworded Spawn to be
downstream-first — generic risk grading, no dependency on anything this repo
alone has, never leaking an AGENTS.md rule into a shipped playbook (Architecture
Rules 4-5). The `ws-mcp.md` row does not apply: no
`agents-plugin-tool/internal/mcp/` path is touched.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-run/lead-run.md, a new rsrc risk-rubric document, agents-plugin/rsrc/ticket-fact-populator/ticket-fact-populator.md, their agents-plugin-wsflow/ and agents-plugin-pi/ mirrors, and each package's rsrc manifest.json |
| scope.surface | public-interface | rewrites the shipped, downstream-mirrored lead-run Spawn procedure and adds a new shipped rubric doc (agents-plugin/rsrc/lead-run/lead-run.md, mirrored to agents-plugin-wsflow/rsrc/lead-run/lead-run.md and agents-plugin-pi/rsrc/lead-run/lead-run.md) |
| scope.new_public_symbol | no | prose/rsrc-doc changes only; no new exported Go symbol |
| scope.new_type_contract | no | no new Go type or function signature; mechanical Route Facts parsing (wsdoc/tickets.go) is explicitly left untouched |
| scope.test_surface | existing | TestWsflowRsrcMirrorUpToDate (agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go), TestPiMirrorUpToDate (agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go), plus the existing Go rsrc suite and wsflow python tests named in Phase 1's verification boundary; no new test file named |
| complexity.reuse_points | confirmed | reuses the existing rsrc channel lead-run already loads through (agents-plugin/skills/lead-run/SKILL.md: ws/playbook.read(name: "lead-run", ...)) and the existing free-form session.note tool (lead-run.md Spawn step 4), with no MCP tool change |
| complexity.side_effect_risk | moderate | rewrites the dispatch procedure every future ticket run reads, but changes are prose-only and guarded by mirror/manifest tests, not runtime code |
| risk.correctness | moderate | the ticket's own citation for the escalate-ladder safety net was wrong before this pass (corrected lead-run.md:241-242 to lead-run.md:243-252), showing the rewritten Spawn section's cross-references need careful verification |
| risk.fit | low | implements the research ticket's Confirmed Decisions with documented Rejected alternatives (260915-research-lead-run-tier-selection-qualitative); no open architecture question remains for Phase 1 |
| risk.test | moderate | verification boundary covers mirror/manifest and existing Go/python suites only; the qualitative dispatch-time judgment itself has no automated test, only later dogfood observation |
| risk.security_or_contract | moderate | no MCP schema change, but new shipped rubric/Spawn prose must not leak an AGENTS.md-specific rule into a downstream playbook (Architecture Rules 4-5, ai-docs/manuals/shipped-surface-boundary.md), a live authoring risk for this new text |

## Phases

### Phase 1: Shared risk rubric + dispatch-time tier judgment in lead-run

Add a bundled rsrc **risk rubric** document: the risk axes, what distinguishes
`low` / `moderate` / `high` on each, and holistic guidance mapping a read to a
`medium` / `large` / `xlarge` tier (enough to keep leads consistent without
re-becoming a de-facto table). Generic and downstream-first.

Rewrite the `lead-run` Spawn section:
- the lead reads the selector-chosen ticket body at dispatch;
- it grades risk against the rubric (loaded through the rsrc channel) and
  picks the worker tier qualitatively;
- it records the chosen tier and the driving risk in the assignment note
  (extend the free-form `session.note` text — no tool change);
- remove the OR-table and the `:53` author-tier prose;
- reconcile the OR-table's other consumer: the "Parallel route (opt-in)"
  step 4 (`lead-run.md:144`) reads "worker playbook chosen from the Spawn tier
  table for that ticket's risks" — repoint it at the qualitative judgment so
  the parallel batch path selects tiers the same way, not at a removed table;
- keep consuming `dispatch_blocked` and the mechanical facts from the
  projection unchanged;
- keep the reactive escalate ladder (`lead-run.md:243-252`) as a safety net;
  note that `xlarge` is now also selectable proactively at dispatch.

Mirror `lead-run.md` and the new rubric doc byte-identical to
`agents-plugin-wsflow/` and `agents-plugin-pi/`, and regenerate the embedded
`manifest.json` hashes.

Deferred to Phase 2: narrowing the populator and its risk-authority consumers.

Verification boundary: mirror guards (`TestWsflowRsrcMirrorUpToDate`,
`TestPiMirrorUpToDate`) and the manifest-regeneration / release-contract
`diff -rq` over `rsrc` pass; the relevant Go rsrc suite and the wsflow python
tests pass. No Go behavior change is expected (the lead reads the body; the
projection contract is untouched).

### Result (5b332cdb) - 2026-09-15

Landed across three commits: `83c60e42` (Phase 1 implementation), `36edc74f`
(round-1 review fixes), `5b332cdb` (round-2 non-blocking polish).

- Added `agents-plugin/rsrc/risk-rubric.md`: a bundled rsrc doc (Axes / Scale /
  Tier guidance for `medium`/`large`/`xlarge`, holistic, not a table to total),
  included into `lead-run.md` via its existing `includes:` frontmatter
  mechanism rather than inlined, per the ticket's rejected-inline-block
  decision.
- Rewrote `lead-run.md`'s Spawn section: the lead now reads the selected
  ticket's whole body at dispatch (the one exception to the projection-only
  rule, scoped to that one ticket) and grades risk against the rubric,
  replacing the four-axis OR-gate table and the `:53` author-tier prose. The
  projection's `risk.*` rows are now an explicit first-pass hint, not a
  verdict. The chosen tier and driving risk are recorded in the assignment
  note (`session.note` text extended, no tool change). The Parallel route's
  per-ticket worker-playbook selection (step 4) is repointed at the same
  qualitative grading instead of the removed OR-table. The stop-(e) escalate
  ladder is kept as the safety net, with an explicit terminal case added for a
  worker already dispatched at `ticket-worker-escalated` (reachable
  proactively now), and Select's read-body ban is reworded to name Spawn's
  read as its sole, scoped exception instead of contradicting it.
  Mechanical Route Facts (`dispatch_blocked`, phase state, scope) are
  untouched.
- Mirrored byte-identical to `agents-plugin-wsflow/rsrc/` (via
  `WS_REGEN_WSFLOW_RSRC`) and `agents-plugin-pi/rsrc/` (manual copy — no
  dedicated pi regen test exists short of the full version-bump script, which
  was out of scope); `manifest.json` regenerated in all three via
  `WSRSRC_REGEN`.
- Two pre-existing pinned tests that asserted the old OR-table/render-call text
  verbatim were updated in the same change:
  `agents-plugin-tool/internal/mcp/playbook_tools_test.go`
  (`TestPlaybookPrintLeadRunWorkerTierPolicy`) and
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`
  (`test_wsflow_run_and_stop_protocol_carry_merge_obligation_text`); the Go
  test's want-list now also pins a distinctive `risk-rubric.md` phrase so a
  broken or dropped `includes:` declaration fails loudly instead of silently
  rendering an empty rubric.

**Round-1 review** (correctness + test partitions, per the route verdict's
`partitioned: correctness, test` allocation) found 3 Important correctness
findings — proactive `xlarge` had no stop-(e) terminal rung; Select's
unqualified read-body ban textually contradicted Spawn's new scoped read; the
new text asserted a tier/review-breadth coupling the runtime does not
have (review allocation is computed from the ticket's frozen Route Facts risk
rows via the worker's own `route.resolve_implement` call, independent of the
lead's dispatch-time tier pick) — and 1 Important test-coverage finding (no
assertion pinned the rubric's own content reaching the rendered body). All
four fixed in `36edc74f`.

**Round 2** re-verified all four fixes as landed correctly (including tracing
`deriveImplementReviewAlloc`/`implementReviewPartitions` in
`agents-plugin-tool/internal/mcp/implement_resolver.go` to confirm the
review-allocation independence claim is a real mechanical fact, not an
assertion taken on faith) and raised no new blocking findings; one non-blocking
observation (residual "heavier review" wording in the rubric's own `medium`
bullet) was applied in `5b332cdb`.

**Decisions taken (cosmetic/adapted, not structural deviations):**
- The rubric's opening line was drafted to name only "picking the worker tier
  to dispatch it at," not implementation-and-review scope, since worker tier
  selects the model only — review breadth is a separate, mechanically
  independent axis (see round-1/round-2 correctness findings above).
- The Parallel route's step 4 render-call wording was repointed to the same
  qualitative grading Spawn performs, rather than left referencing the removed
  OR-table, per the ticket's explicit reconciliation instruction.

**Verification:**
- `go build ./... && go vet ./...` (agents-plugin-tool): clean.
- `go test ./... -count=1` (agents-plugin-tool, all packages): pass.
- `go test ./internal/wsrsrc/... -run 'TestWsflowRsrcMirrorUpToDate|TestPiMirrorUpToDate|TestValidateRealTree'`: pass.
- `python3 -m unittest discover agents-plugin-wsflow/tests`: 12/12 pass.
- Independent review: round 1 (3 Important correctness + 1 Important test,
  all fixed) and round 2 (all fixes verified, no Critical open, one
  non-blocking observation applied).

**Deferred to Phase 2** (unchanged from the plan): narrowing the
fact-populator's `risk.*` rows to advisory and auditing other risk-authority
consumers.

### Phase 2: Narrow fact-populator risk to advisory and adjust risk-authority consumers

Depends on Phase 1 landing (the lead must already be the tier authority before
the populator's authority is removed).

Demote the `risk.*` rows in `ticket-fact-populator` to advisory: keep filling
them as a first-pass hint, drop any wording that makes them tier-authoritative,
and rephrase the anti-low calibration rule (`ticket-fact-populator.md:101-103`)
for an advisory context. Audit and adjust any other consumer that read the
populator's risk grades as authority; leave the mechanical facts untouched.
(A search of `agents-plugin/rsrc` and `agents-plugin/skills` for
`risk.correctness`, `risk.fit`, `risk.test`, and `risk.security_or_contract`
currently matches only `lead-run.md` and `ticket-fact-populator.md` itself —
neither `ticket-reviewer-design.md` nor `ticket-reviewer-completeness.md`
references Route Facts or `risk.*` today, so the audit may find no other
consumer to adjust.) Mirror any shipped changes.

Verification boundary: the fact-populator and reviewer playbooks still satisfy
their skill-authoring invariants; mirror and manifest guards pass; no consumer
still treats `risk.*` as tier authority.

### Result (6b6144a9) - 2026-09-15

Landed across two commits: `e8f6c28b` (Phase 2 implementation), `6b6144a9`
(round-1 review fixes; round 2 raised no new findings).

- Reworded `agents-plugin/rsrc/ticket-fact-populator/ticket-fact-populator.md`'s
  Route Facts grading guidance (the paragraph after the schema table): the four
  `risk.*` rows are now explicitly stated as not driving the worker tier — the
  worker tier is the lead's own dispatch-time read of the ticket (Phase 1),
  and a populator's `risk.*` grade is only that read's first-pass hint. The
  "never guess a low" calibration rule is kept (not deleted) but re-grounded in
  the rows' remaining live mechanical role: they still contribute to the review
  allocation the worker's own `route.resolve_implement` call derives (only
  `moderate`/`high` keeps a partition in play, so an ungrounded `low` *or*
  `unknown` silently drops one).
- Audited `agents-plugin/rsrc` and `agents-plugin/skills` for
  `risk.correctness`/`risk.fit`/`risk.test`/`risk.security_or_contract`: no
  consumer beyond `lead-run.md` (already reworded in Phase 1) and
  `ticket-fact-populator.md` itself exists; neither
  `ticket-reviewer-design.md` nor `ticket-reviewer-completeness.md` references
  Route Facts or `risk.*`, matching the ticket's own prediction — no other
  consumer needed adjustment.
- Mirrored byte-identical to `agents-plugin-wsflow/rsrc/` (via
  `WS_REGEN_WSFLOW_RSRC`) and `agents-plugin-pi/rsrc/` (manual copy, per Phase
  1's precedent — no dedicated pi regen test exists short of the full
  version-bump script); `manifest.json` regenerated via `WSRSRC_REGEN` and
  copied byte-identical into both mirrors, both times (initial edit and the
  round-1 fix).

**Round-1 review** (correctness + test partitions, per the route verdict's
`partitioned: correctness, test` allocation): the test partition reported
clean (prose-only diff, no test-code checklist items applied; re-verified the
phase's stated verification boundary directly). The correctness partition
found 2 Important findings: (1) the anti-low rule's justification claimed only
an ungrounded `low` drops a review partition, but `materialRisk` in
`implement_resolver.go` (`value == "moderate" || value == "high"`) treats
`low` and `unknown` identically as non-material, so the original wording was
inconsistent with both the resolver code and the file's own earlier
"all-unknown table asks for the smallest review" line; (2) the wording named
"the shared Risk Rubric" as a resolvable pointer, but the populator playbook
declares no `includes:` for it, leaving an unresolvable reference. Also 2
Minor findings: "still set the review allocation" overstated sole causation
(other facts also feed the partitions), and "no longer reads this table" was
migration-relative phrasing in a shipped prompt. All four fixed in `6b6144a9`.

**Round 2** re-verified both Important fixes against the cited code itself
(re-read `materialRisk`/`implementReviewPartitions` and grepped for `rubric`
in the populator playbook) and confirmed both Minor fixes, raising no new
blocking findings. One non-blocking observation recorded: "silently drops one"
is conditional at the allocation level (a partition can survive via a
non-risk trigger, e.g. `new_type_contract`/`new_public_symbol`), judged an
adequate hedge for a cheap-tier delegate instruction.

**Decisions taken (cosmetic/adapted, not structural deviations):**
- Did not add an `includes:` for `risk-rubric` to `ticket-fact-populator.md`.
  The populator is a cheap-tier delegate that fills a first-pass hint, not the
  tier-grading reader (that's the lead, per Phase 1); wiring the rubric into
  the populator was not named in this phase's task list, and the round-1
  correctness finding was resolved by dropping the unresolvable proper-noun
  reference rather than by adding a new include. Left as a candidate follow-up
  if dogfooding shows populator/lead grading drift without a shared rubric.

**Verification:**
- `go build ./... && go vet ./...` (agents-plugin-tool): clean.
- `go test ./... -count=1` (agents-plugin-tool, all packages): pass, both
  before and after the round-1 fix.
- `go test ./internal/wsrsrc/... -run 'TestWsflowRsrcMirrorUpToDate|TestPiMirrorUpToDate|TestValidateRealTree'`: pass.
- `python3 -m unittest discover agents-plugin-wsflow/tests`: 12/12 pass.
- Independent review: round 1 (2 Important + 2 Minor, all fixed) and round 2
  (all fixes verified against source, no Critical open, one non-blocking
  observation recorded).

Both phases of this ticket are now complete.
