# Plan: 260726-bug-lead-implement-lost-review-relay-cycle-cap — Phase 2: Adjudicator delegate

## Relevant Ticket Contract

- Depends on Phase 1 (landed, `d911f70d`/`f928ad60`): `implementReviewInstruction`
  now states the per-branch review-cycle budget and the shared
  `implementReviewFinalCycleClause` const; the spec anchor states the cap in
  review cycles and drops "caller escalation at cycle 3" but still says "lead
  adjudication at cycle 2" — Phase 2 owns that remaining correction.
- **New rsrc delegate `review-adjudicator`**, modeled on `ticket-reviewer-design`:
  `kind: render`, `delegates: true`, `role: delegate`, `tier: large`. Inputs are
  file paths only: plan, review findings paths, disposition record, commit
  range, authority (ticket/inline). Budget accounting stays lead-owned — the
  delegate is told the current cycle number but never enforces/tracks budget.
- **Aperture (settled, do not re-derive):** the adjudicator's question is "is
  the implementer's won't-fix reason true", not "is the code correct" — it does
  **not** re-review the diff for correctness. The reviewer's factual claims
  about the diff stand unless the implementer supplied specific disproving
  evidence. Read table:

  | Implementer's defense | Evidence the adjudicator weighs |
  |---|---|
  | style suggestion conflicting with local patterns | codebase-wide patterns, convention docs, commit precedent — **not** the diff |
  | requires scope expansion beyond the plan | the plan and the ticket/inline authority |
  | disproven by specific evidence | the offered evidence itself |

- **Three-verdict output, one line per dispute (settled vocabulary):**
  - `[accept]` — the defense holds; the won't-fix stands.
  - `[override: <reason>]` — the defense fails; relay it as a required fix.
  - `[out-of-scope: <reason>]` — valid finding, outside the plan; recorded
    deferral. It does three things only: leaves the relay list, costs no relay,
    carried into completion output as unresolved-by-decision. The adjudicator
    never edits the plan.
  - One adjudicator spawn handles every maintained dispute in a cycle (output
    is a per-dispute line list, not a per-dispute spawn).
- **Two-armed trigger (settled):**
  1. `[maintained]` from a contested won't-fix (already flows via the
     Re-review prompt's existing per-item ask).
  2. An implementer plan-update escalation — currently invisible because
     `implementer-relay` has no token for it. This phase adds
     `[escalate: <reason>]` to `implementer-relay`'s output vocabulary, in
     **both** enumeration sites (Output section and Process step 4) — the
     ticket names this exact double-enumeration failure mode as the reason
     both sites must change together.
- **Adjudication happens between reviews and never adds one (settled):** it
  runs mid-relay, before any review has occurred, so it spends no budget. An
  override ships as the **next relay** (consumes that relay, does not add
  one). Bounded at **one adjudication per relay slot** — a second escalation
  in the same slot is not re-adjudicated; it reaches the next review as-is.
- **`single` (2-cycle) gets no adjudication slot (settled).** No remaining
  relay exists to ship an override into. The `single` branch instead gains one
  sentence naming the `[escalate]` degradation: with no adjudication slot, the
  lead treats `[escalate]` there as its own accept-or-defer call.
- Placement: only the generated review Instruction (`partitioned:` and
  fallback branches) gains the adjudication clause. `lead-implement.md` is not
  edited for this — no new template, no separate todo item (both rejected by
  the ticket for reasons already settled: ordering-contract violation and
  Doctrine's route-facts-to-MCP split).
- **Spec update required.** `#260612-reviewer-allocation-tier-default`
  currently reads "...3 cycles for partitioned with lead adjudication at cycle
  2." This phase makes the adjudicator a delegate with its own verdict
  vocabulary, so "lead adjudication" must change to reflect delegate
  adjudication in the same logical change.
- Verification boundary (from the ticket, verbatim scope): a `partitioned:`
  verdict's Instruction carries the adjudication clause, a single/lead-only
  verdict's does not; the rendered `review-adjudicator` prompt states the
  do-not-re-review-the-diff constraint and the three verdicts; the delegate
  prompt is self-contained under a fresh spawn with no prior conversation;
  **both** of `implementer-relay`'s token enumerations list
  `[escalate: <reason>]`, verified by grepping/counting rather than reading one
  section; the Instruction states adjudication happens within a relay slot
  without consuming a review and is bounded to one per slot, and states the
  `[out-of-scope]` disposition; the `single` Instruction names the
  `[escalate]` degradation; the spec anchor no longer says "lead adjudication";
  manifest and wsflow mirror tests pass without hand-edits.
- Constraint: `role:` must be from `childRoleForPlaybookRole`'s closed set
  (`implementer`, `reviewer`, `delegate`, `leaf`) — anything else silently
  mints no child session key. `review-adjudicator` uses `role: delegate`.
- Constraint: generated Instructions are plain Go strings — no `{{...}}` may
  appear.
- Constraint: touching `agents-plugin-tool/` invokes the dev-merge version
  bump via `agents-plugin-tool/scripts/bump-ws-version.sh` (current version
  `0.36.20` per `agents-plugin/.claude-plugin/plugin.json`, confirmed live).
- Constraint: adding an rsrc file requires
  `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
  before `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
  (both `-count=1` mandatory, in this order). `agents-plugin-wsflow/rsrc/` is a
  generated byte-identical mirror; never hand-edit it.
- Skill/prompt-text edits must pass `agents-plugin/skills/lead-skill-authoring/SKILL.md`'s
  invariant checklist (Code Standards rule 5 / ticket Constraints).

## Out of Scope

- `implementer-elevated`, the capacity condition (`[fixed]` then still
  non-clean), the root-cause detector, the `[resolved]`/`[unresolved]`
  re-review ask, and the three dispatch-template edits (`Review relay
  dispatch`, its task-input mapping, `Re-review prompt`) in
  `lead-implement.md` — all Phase 3.
- The wording-conflict fix to "only for genuinely new non-clean
  Critical/Important findings" (relaying a persisting `[unresolved]` finding)
  — tied to Phase 3's capacity condition, not introduced here.
- Any `lead-implement.md` edit of any kind. Phase 2 touches only the generated
  Instruction (`session_state.go`), the new `review-adjudicator` rsrc file,
  `implementer-relay`'s two token enumerations, and the spec anchor.
- Widening `implementer-relay`'s Process step 3 prose beyond the one token
  addition — the ticket calls this "the one `implementer-relay` edit," additive
  to the output vocabulary, not a new rule.
- Any `{{...}}` render-time syntax in generated Instruction strings (plain Go
  strings only).
- Re-deciding the aperture, the verdict vocabulary, the trigger, or the
  one-adjudication-per-slot bound — these are settled in `## Decisions` and
  must be carried, not re-derived.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/session_state.go#L563-L582` —
  `implementReviewInstruction`, the sole generated-Instruction edit target
  (confirmed current, post-Phase-1):
  - L566: `implementReviewFinalCycleClause` const (completes-not-halts text) —
    reused unchanged, do not duplicate its text inline.
  - L575-576: `partitioned:` branch (`fmt.Sprintf` return) — add the
    adjudication clause here.
  - L578-579: `single` branch — add only the one `[escalate]`-degradation
    sentence.
  - L581: fallback branch (bare `"partitioned"`, reached via legacy
    `enter.implement` with no `target`) — add the same adjudication clause as
    the `partitioned:` branch.
  - L572-574: `lead-only` branch — untouched (dispatches no reviewers, never
    relays).
- `agents-plugin/rsrc/ticket-reviewer-design/ticket-reviewer-design.md` (94
  lines) — prompt-format precedent named by the ticket. Frontmatter:
  `kind: render`, `delegates: true`, `role: reviewer`, `tier: large`,
  `variables: [RoleModel]`. Body shape to mirror: `## Constraints`
  (read-only, no mutation tools, English-only), `## Process` (numbered file
  reads), a domain checklist, `## Output` (structured verdict block with
  explicit per-item schema and emit/omit rules), `## Doctrine` (one-paragraph
  optimization statement). `review-adjudicator` should follow this shape,
  substituting the design-review checklist for the aperture/read-table
  constraint and the pass/concern/block verdict for the
  accept/override/out-of-scope per-dispute lines.
- `agents-plugin/rsrc/plan-populator-survey/plan-populator-survey.md#L1-L11` —
  second precedent for `role: delegate` (not just `role: reviewer`), already
  live and working: `kind: render`, `delegates: false`, `role: delegate`,
  `tier: medium`. Confirms `role: delegate` is an established, exercised
  pattern, not a novel choice for Phase 2 to validate from scratch. Also shows
  the ticket/inline authority input shape (`target_kind`, `ticket_path`,
  `selected_phase`, `inline_contract`) usable as a model for the adjudicator's
  "authority" input.
- `agents-plugin/rsrc/implementer-relay/implementer-relay.md` (77 lines) — the
  file for the token-vocabulary edit. Both enumeration sites confirmed by
  direct read:
  - L51 (Process step 4): `...decide `[fixed]`, `[won't fix: <reason>]`, or
    `[deferred: <reason>]`.` — add `, or `[escalate: <reason>]`` (or restructure
    the four-way list).
  - L58-61 (Output section, per-finding disposition bullets):
    ```
    - `[fixed]` — addressed and committed.
    - `[won't fix: <reason>]` — refused; reason must cite a specific local pattern or scope boundary.
    - `[deferred: <reason>]` — not addressed this cycle; state the resolution condition.
    ```
    add a fourth bullet `- `[escalate: <reason>]` — <describe: relayed to the
    adjudicator for a plan-scope judgment>.`
  - L50 (Process step 3) already contains "escalate for a plan update if a
    required fix needs ticket material or a plan deviation" — this sentence is
    the existing hook the new token makes machine-visible; it does not itself
    need to change (the ticket names only the two-site token addition as the
    one `implementer-relay` edit).
  - Variables declared: `RoleModel`, `PlanPath`, `ReviewCycle`, `CommitRange`,
    `ReviewPaths`, `DispositionNotes`, `VerificationHint`,
    `ResultExpectations` — no variables addition needed for this token-only
    change.
- `agents-plugin-tool/internal/mcp/playbook_tools.go#L338-L347` —
  `childRoleForPlaybookRole`: confirmed the closed set is exactly
  `implementer`/`reviewer`/`delegate` → `roleDelegate`, `leaf` → `roleLeaf`,
  everything else (including a natural-reading `role: adjudicator`) silently
  returns `("", false)` — no child key minted, no error. `role: delegate` is
  in the accepted set.
- `agents-plugin-tool/internal/mcp/playbook_tools.go#L763-L769` — the second
  `role` consumer: `prefer_mercenary` guidance block is appended only for
  `role` in `{"implementer", "reviewer"}`. `review-adjudicator` (`role:
  delegate`) will not receive this block — confirmed this is expected and
  correct (Phase 3's `implementer-elevated`, which replaces an implementer, is
  the one that must stay `role: implementer` to keep this guidance; Phase 2's
  delegate is a genuinely new role, not a drop-in implementer replacement, so
  losing this block is not a regression).
- `agents-plugin-tool/internal/mcp/playbook_tools.go#L798-L834` —
  `substitutePlaybookVars`: any caller-context key not in `declared` (plus
  `wsrsrc.ImplicitVariableNames`) returns `wsrsrc.ErrUndeclaredVar`. Confirms
  the ticket's Constraint literally — every input the new delegate's body
  references via `{{.Name}}` must appear in its `variables:` frontmatter list.
- `agents-plugin-tool/internal/wsrsrc/wsrsrc.go#L61-64` —
  `ImplicitVariableNames` (`McpNamespace`, `SkillNamespace`, four
  `*TierModel` vars) are available to every playbook body without frontmatter
  declaration; `RoleModel` is **not** implicit — it is declared explicitly in
  `variables:` by every examined delegate precedent
  (`ticket-reviewer-design`, `implementer-relay`, `plan-populator-survey`) even
  when unused in the body, apparently as a standing per-delegate convention.
  Declare it for `review-adjudicator` too, for consistency, even if the body
  never references `{{.RoleModel}}`.
- `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md#L76-81` ("On:
  Reviewer Spawn") — the concrete render-then-spawn pattern to describe inline
  in the new Instruction clause (no template exists for the adjudicator, so
  the clause must be self-sufficient prose, following the same style the
  `single` branch already uses to reference "Reviewer prompt frame" by name):
  `playbook.render(name: "review-adjudicator")` returns a path; spawn a native
  subagent with `Read <rendered-path> as your system prompt` plus the
  file-path inputs (plan, review findings paths, disposition record, commit
  range, authority); parse the per-dispute verdict lines from the result.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L181-205` —
  `Review relay dispatch` and `Re-review prompt` templates: confirmed
  untouched by Phase 2 (Phase 3 territory per the ticket's "Dispatch
  surface — three templates, not two"). `Re-review prompt` (L194-205) already
  asks "For each [won't fix], respond [accepted] or [maintained: <short
  reason>]" — this is the existing flow Phase 2 hooks the first trigger arm
  into; not edited this phase.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L233-264` —
  `TestDeriveImplementTodoInstructionsPartitionedReview`: existing pins on the
  `partitioned:` branch (partition-name substitution, budget/final-cycle
  text). Add assertions for the new adjudication clause elements: the two
  trigger tokens (`[maintained]`, `[escalate`), the spawn reference
  (`review-adjudicator`), the budget-neutral override framing ("next relay"
  consumes rather than adds), the one-per-slot bound, and the
  `[out-of-scope]` disposition description. Existing assertions must keep
  passing unchanged.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L266-291` —
  `TestDeriveImplementTodoInstructionsBarePartitionedReviewFallback`: same
  additions as above, mirrored for the fallback branch.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L1865-1909` —
  `TestEnterImplementAllocatesSingleReviewForBoundedPublicExistingTestChange`:
  add an assertion for the new `[escalate]`-degradation sentence on the
  `single` branch. Preserve the existing negative check (`!strings.Contains(review,
  "reviewers")`, L1906-1908) — verify the chosen wording does not introduce
  the plural substring. Consider adding a negative assertion that
  `"review-adjudicator"` does not appear on this branch, mirroring Phase 1's
  precedent of mutation-checked negative assertions
  (`TestEnterImplementFocusedTodosDirectLeadOnlySkippedDocs#L1943-1948`) —
  optional strengthening, not a ticket-mandated boundary item, but consistent
  with "single gets no adjudication slot."
- `agents-plugin-tool/internal/mcp/session_state_test.go#L1911-1948` —
  `TestEnterImplementFocusedTodosDirectLeadOnlySkippedDocs`: `lead-only`
  branch, confirmed unaffected; re-run only, no edit needed.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1203-1260` —
  `TestRenderPlaybookShippedImplementerRelayDeclaredContext`: the existing
  `want` list (L1224-1246) asserts `` `[fixed]` ``, `` `[won't fix: <reason>]` ``,
  and `` `[deferred: <reason>]` `` all render. **Risk signal:** a bare
  `strings.Contains(body, "`[escalate: <reason>]`")` addition to this `want`
  list would pass if the token appeared at only one of the two required
  sites — it does not prove both enumerations changed, which is exactly the
  verification method the ticket's boundary text explicitly rejects
  ("verified by grepping the file for the token set rather than by reading
  one section"). Use a count-based or two-site-anchored assertion instead —
  e.g. `strings.Count(body, "[escalate: <reason>]") == 2`, or two separate
  `strings.Contains` checks each anchored to surrounding context unique to
  Process step 4 vs. the Output section bullet.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L74-84` —
  `shippedImplementerRelayContext()` helper: no change needed (token-only
  edit adds no new variable).
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go` — repo-wide pattern
  for "self-contained under a fresh spawn with no prior conversation" and "no
  leftover template placeholder": `strings.Contains(body, "{{.")` must be
  false after render (used at L243, L351, L515, L548, L584, L798, L1983), and
  the credential-block check `strings.Contains(body, "Your ws session_key")`
  (used in `mercenary_surface_test.go#L568-570` for `ticket-reviewer-design`
  et al.) proves a render-minted child key. Add an equivalent render test for
  `review-adjudicator` — a new `TestRenderPlaybookShippedReviewAdjudicator...`
  test (or an addition to an existing shipped-delegate table test) asserting:
  full variable substitution (no `{{.` remaining), the credential block
  present, `tier == "large"`, and the do-not-re-review-the-diff sentence plus
  all three verdict tokens (`[accept]`, `[override:`, `[out-of-scope:`) appear
  in the rendered body.
- `ai-docs/spec/workflow-skills.md#L704-727` — `#260612-reviewer-allocation-tier-default`
  anchor (current, post-Phase-1): the exact clause to edit is "...3 cycles for
  partitioned with lead adjudication at cycle 2." (L722-723,
  wrapped/hyphenated across L721-723 in the current text). Replace "lead
  adjudication" with delegate-adjudication phrasing (e.g. "adjudicator
  delegate arbitration") without restating the full verdict vocabulary or read
  table inline — the spec states the contract exists and its shape, not the
  full prompt text.
- `agents-plugin-tool/scripts/bump-ws-version.sh` and
  `agents-plugin/.claude-plugin/plugin.json` (current `version: 0.36.20`,
  confirmed live; matches `agents-plugin/runtime.json` and
  `.codex-plugin/plugin.json`) — run as the last step touching
  `agents-plugin-tool/`/`agents-plugin/rsrc/`; confirm the exact next patch
  value against `plugin.json` immediately before running, since intervening
  work on this branch may have already bumped it.
- Confirmed via direct diff: `agents-plugin/rsrc/{ticket-reviewer-design,implementer-relay}/*.md`
  are byte-identical to their `agents-plugin-wsflow/rsrc/` mirror copies today
  (baseline before this phase's edits) and carry no `<!-- ws:full-only -->`
  product-mode markers — the new `review-adjudicator` file needs none either.

## Implementation Plan

1. Create `agents-plugin/rsrc/review-adjudicator/review-adjudicator.md`,
   modeled structurally on
   `agents-plugin/rsrc/ticket-reviewer-design/ticket-reviewer-design.md`:
   - Frontmatter: `kind: render`, `delegates: true`, `role: delegate`,
     `tier: large`, `variables:` listing `RoleModel` plus the file-path inputs
     chosen for plan, review findings paths, disposition record, commit
     range, and authority (name them consistently with existing sibling
     playbooks — e.g. reuse `PlanPath`/`ReviewPaths`/`DispositionNotes`/
     `CommitRange` naming from `implementer-relay` where the concept matches,
     and an authority shape modeled on `plan-populator-survey`'s
     `target_kind`/`ticket_path`/`selected_phase`/`inline_contract`).
   - Body: state the aperture constraint explicitly ("do not re-review the
     diff for correctness... reviewer's factual claims stand unless
     specifically disproven") as its own load-bearing line, mirroring how
     `ticket-reviewer-design` states "Read only the ticket file at the
     provided path" as explicitly. Include the read table from `##
     Decisions` verbatim in substance. State the process: read plan, review
     findings, disposition record, authority; for each `[maintained]` or
     escalated dispute, weigh only the evidence class the read table assigns
     to that defense type. Output: one line per dispute using exactly
     `[accept]`, `[override: <reason>]`, `[out-of-scope: <reason>]` — no
     other verdict tokens. Constraints: read-only (no mutation tools, no plan
     edits), stateless (self-contained inputs, no prior conversation), English
     output.
   - Run `agents-plugin/skills/lead-skill-authoring/SKILL.md`'s invariant
     checklist against every Constraints/Invariants line before finalizing.
2. Edit `agents-plugin-tool/internal/mcp/session_state.go`,
   `implementReviewInstruction`:
   - `partitioned:` branch (L575-576) and fallback branch (L581): append a
     clause covering, in this order: (a) the two-armed trigger
     (`[maintained]` from Re-review, or an implementer `[escalate: <reason>]`)
     and that the step is optional when neither fired; (b) the spawn method
     (render `review-adjudicator`, dispatch with plan/review
     findings/disposition record/commit range/authority, per the
     render-then-spawn pattern in `lead-write-ticket.md`'s "On: Reviewer
     Spawn"); (c) that an `[override]` ships as the next relay and consumes
     that relay's budget rather than adding a new one; (d) the
     `[out-of-scope]` disposition (leaves the relay list, costs no relay,
     carried into completion output); (e) the one-adjudication-per-relay-slot
     bound (a second same-slot escalation is not re-adjudicated; it reaches
     the next review as-is). Keep the existing budget/final-cycle text
     unchanged; append, do not replace.
   - `single` branch (L578-579): append exactly one sentence — with no
     adjudication slot at the 2-cycle budget, `[escalate: <reason>]` there is
     the lead's own accept-or-defer call, not a delegate dispatch. Do not
     introduce the substring "reviewers" (plural) or "review-adjudicator".
   - `lead-only` branch (L572-574): untouched.
   - Plain Go strings only — no `{{...}}`.
3. Edit `agents-plugin/rsrc/implementer-relay/implementer-relay.md`:
   - Process step 4 (L51): extend the three-way decision to four —
     `[fixed]`, `[won't fix: <reason>]`, `[deferred: <reason>]`, or
     `[escalate: <reason>]`.
   - Output section (L58-61): add a fourth bullet —
     `` `[escalate: <reason>]` — <effect: routes to adjudicator review for the
     defense in question> ``, in the same one-line style as the other three.
   - No variables-list change (token-only edit).
   - Run the lead-skill-authoring invariant checklist against the changed
     lines.
4. Edit `ai-docs/spec/workflow-skills.md` at
   `{#260612-reviewer-allocation-tier-default}` (current L704-727): replace
   "lead adjudication at cycle 2" with delegate-adjudication phrasing that
   names the adjudicator as a delegate, without restating the full verdict
   vocabulary inline (keep the spec at contract-shape granularity, consistent
   with how the rest of this anchor already summarizes rather than
   transcribes prompt text).
5. Update `agents-plugin-tool/internal/mcp/session_state_test.go`:
   - `TestDeriveImplementTodoInstructionsPartitionedReview` (L233-264) and
     `TestDeriveImplementTodoInstructionsBarePartitionedReviewFallback`
     (L266-291): add assertions per the Codebase Findings list above (trigger
     tokens, spawn reference, budget-neutral override framing, one-per-slot
     bound, `[out-of-scope]` disposition). Existing assertions must keep
     passing unchanged.
   - `TestEnterImplementAllocatesSingleReviewForBoundedPublicExistingTestChange`
     (L1865-1909): add the `[escalate]`-degradation assertion; keep the
     existing `"reviewers"` negative check passing; optionally add a negative
     check for `"review-adjudicator"` absence.
   - Leave `TestEnterImplementFocusedTodosDirectLeadOnlySkippedDocs`
     (L1911-1948) unchanged; re-run to confirm `lead-only` is unaffected.
6. Update `agents-plugin-tool/internal/mcp/playbook_tools_test.go`:
   - `TestRenderPlaybookShippedImplementerRelayDeclaredContext` (L1203-1260):
     add a count-based or two-site-anchored assertion proving
     `[escalate: <reason>]` appears at **both** the Process step 4 and Output
     section sites (see the risk signal in Codebase Findings — a plain
     `Contains` addition is insufficient).
   - Add a new render test for `review-adjudicator` (new function, or an
     addition to an existing shipped-delegate table test) asserting: full
     variable substitution (no leftover `{{.`), the render-minted credential
     block present, `tier == "large"`, the do-not-re-review-the-diff sentence,
     and all three verdict tokens present in the rendered body.
7. Run manifest and mirror regeneration, in this exact order, from
   `agents-plugin-tool/`:
   - `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
   - `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
   Confirm `agents-plugin-wsflow/rsrc/review-adjudicator/` was created as a
   byte-identical copy and no other file under either `rsrc/` tree changed
   unexpectedly.
8. Run `agents-plugin-tool/scripts/bump-ws-version.sh <next-patch>` for the
   dev-merge version bump; confirm the exact next-patch value against
   `agents-plugin/.claude-plugin/plugin.json` (currently `0.36.20`)
   immediately before running.

## Verification Plan

- `cd agents-plugin-tool && go test ./internal/mcp/... -run 'TestDeriveImplementTodoInstructionsPartitionedReview|TestDeriveImplementTodoInstructionsBarePartitionedReviewFallback|TestEnterImplementAllocatesSingleReviewForBoundedPublicExistingTestChange|TestEnterImplementFocusedTodosDirectLeadOnlySkippedDocs|TestRenderPlaybookShippedImplementerRelayDeclaredContext' -v`
  — focused check on the four Instruction branches plus the `implementer-relay`
  token render.
- `cd agents-plugin-tool && go test ./internal/mcp/... -run 'ReviewAdjudicator' -v`
  — the new delegate's render test.
- `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
  then `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
  (mandatory `-count=1`, in this order) from `agents-plugin-tool/`.
- `cd agents-plugin-tool && go build ./... && go test ./... -count=1` — full
  package regression, confirming `TestValidateRealTree`,
  `TestShippedManifestUpToDate`, and `TestWsflowRsrcMirrorUpToDate` all pass
  without further hand-edits after the regeneration step above.
- Manual check: grep `implementer-relay.md` for `[escalate: <reason>]` and
  confirm exactly 2 matches (Process step 4, Output bullet).
- Manual check: `git diff` scoped to `agents-plugin-wsflow/rsrc/` shows only
  the new `review-adjudicator/` addition and no drift elsewhere; diff the new
  file against its `agents-plugin/rsrc/` source to confirm byte-identity.
- Manual check: re-read the edited spec anchor to confirm it no longer
  contains the string "lead adjudication".
- Manual check: grep all changed/new files for the literal substring `"{{"` to
  confirm no template syntax leaked into a generated Instruction string or a
  spec sentence.

## Escalations

- None.
