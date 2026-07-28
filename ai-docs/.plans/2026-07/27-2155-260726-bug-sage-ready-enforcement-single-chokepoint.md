# Plan: 260726-bug-sage-ready-enforcement-single-chokepoint — Phase 1: Relocate ready-sage enforcement to the commit gate

## Relevant Ticket Contract

- **Single chokepoint at the commit gate.** `tickets.verify`/`ws/git.commit` stays the one HARD
  enforcement point. `tickets.move` and `tickets.create_empty` stop rejecting on ready sage
  posture and emit a loud warning instead. Owner's explicit direction: do not block at tool level.
- **No `waive` action, no `waived_by_owner` verdict.** Not in scope; do not add either.
- **The warning must state the consequence and a reachable escape**, naming `ws/tickets.sage_gate(stem, landing: "ready")`
  as the resolving call and `ws/config.show` as the config-surface pointer, plus the review-scope
  line (design checks coherence/right-problem/executability; completeness checks structure/fields/clarity;
  neither judges whether the underlying research is settled). Target shape given verbatim in the
  ticket's `## Decisions` section — treat it as the message skeleton, not literal text to copy
  unmodified (field name / exact wording can adapt to code structure).
- **The message must ride the ordinary path an agent actually takes**: the non-waivable statement
  and review-scope line belong on the move/create warning AND on `tickets.sage_gate`'s ordinary
  `required` → `run` result — not only on an `answer: "no"` decline path, which no agent reaches at
  `required` (posture `required` never asks; `answer` is documented as a follow-up to a prior ask).
  Keep them on the `recommended` ask prompt too.
- **`blocked` de-blocks at mutation time too, with its own distinct message** ("a prior review
  found unresolved issues" vs. "review has not run"). `tickets.verify` still hard-fails on `blocked`.
- Do not weaken `tickets.verify`'s `ready-sage-posture` guardrail; it stays HARD and becomes the
  sole enforcement point.
- Do not change reviewer criteria or semantic sage judgment — out of scope per the parent epic.
- **Renumber `lead-write-ticket` unconditionally**: Sage Review Gate becomes step 5, Commit
  becomes step 6. Not conditional — the gate is already a no-op for non-`ready/` landings.
- **Resolve the `260713` partial-mutation notice explicitly** — delete it if it becomes
  unreachable, or keep it and name the remaining path that reaches it. This is an acceptance item.
- Confirm `260723`'s already-revised seed-classification bullet (`## Cross-Child Decisions`) still
  matches shipped behavior at close; no fresh epic edit needed (already revised 2026-07-26, already
  reads correctly against this plan — see Codebase Findings).
- Fixtures must exercise the shipped default `sage_review: auto` (never `skipped`).

Verification boundary (carried verbatim, all at `sage_review: auto`):

1. `tickets.create_empty(initial_state: "ready")` succeeds and returns the warning instead of failing.
2. `tickets.move(to: "ready")` succeeds from both an unreviewed posture and a `blocked` posture,
   each returning its own warning text.
3. `ws/git.commit` on either resulting ticket still fails on guardrail `ready-sage-posture`.
4. `tickets.sage_gate` at posture `required` returns the non-waivable statement, the review-scope
   line, and the resolving call on its ordinary `run` result.
5. A `todo/` → `ready/` promotion completes end-to-end through the renumbered rendered playbook
   with no hand edit.

## Out of Scope

- Adding a `waive` action or `waived_by_owner` verdict (rejected alternative — collapses `required`
  into `recommended`).
- Removing `ready/` from `judge: initial-status` (rejected — treats the symptom).
- The `sage_review` config setter (`260626-bug-sage-review-config-setter-missing`) — the warning
  only *names* `ws/config.show` as the resolved-config pointer; it does not add a setter tool.
- `260726-feat-proceed-sage-posture-consumption-guard` (separate ticket; the narrowed `ready/`
  guarantee is accepted, mitigation lives there).
- Spec text authoring in `ai-docs/spec/mcp-tools.md` — Spec Impact says this is a documentation
  pre-pass task, not part of the code edit step. This plan only names the anchors that will need
  updating at closeout.
- Version bump (`agents-plugin-tool/scripts/bump-ws-version.sh`) — not part of this phase.

## Codebase Findings

- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go:341-404` `prepareSageReviewForUpwardMove` —
  the function to change. It currently `return`s an error via `sageReviewBlockedError` (line 382,
  385) or `sageReviewStageError` (line 401) after already persisting the resolved posture (lines
  360-371, the self-healing write). Only caller: `TicketsMove` at `tickets_mutate.go:135-140`.
  De-blocking means: this function stops returning an error for the ready-posture problem cases;
  it must still return `sageReviewPostures` so the caller can build the warning text. The write
  side (lines 354-371) is unaffected — it must keep persisting the resolved posture regardless.
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go:111-163` `TicketsMove` — the call site.
  Lines 135-140 currently: on `prepareSageReviewForUpwardMove` error, return
  `TicketMutateResult{PartialMutationNotice: ...}, err` (never reaches the `atomicGitMove`/success
  path below). After de-blocking, this branch goes away: the function proceeds to
  `atomicGitMove` and the existing post-move tip logic at lines 147-161 (which already calls
  `readyGateWarning` for `to == "ready"` and `sageReviewPostureTip` for any upward move) is the
  natural place to append the new ready-sage warning — reuse `appendTip`/`result.Tip`, do not
  invent a second field.
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go:406-451` `readyPostureProblems` — already
  the single, pure (no I/O) implementation of "which required stage(s) are non-terminal", shared
  by `prepareSageReviewForUpwardMove` (mutation) and `tickets_verify.go:154-166` (guardrail). This
  is the right function to build the new mutation-time warning text from directly (it already
  distinguishes `Blocked: true` vs. an unset/non-terminal posture) — do not duplicate its logic in
  a new warning-only pass.
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go:314-329` `sageReviewStageError` /
  `sageReviewBlockedError` — these two constructors must survive unchanged (or with compatible
  signatures) because `tickets_verify.go:154-166` calls the *same* constructors to build its hard
  finding text. If their wording changes, `tickets.verify`'s FAIL text changes too — that's fine
  (ticket allows richer text there) but do not fork them into separate mutation-only vs.
  verify-only variants; that would let the two enforcement points' language drift, contrary to
  "single chokepoint" intent. Simplest approach: keep these two producing the *hard* wording used
  by verify's FAIL lines, and build a *separate* warning string (new function, e.g.
  `readySagePostureWarning`) for the mutation-time soft path that states the consequence +
  resolving call, reusing `readyPostureProblems` for which-field/blocked-or-not detection.
- `agents-plugin-tool/internal/wsdoc/ticket_create.go:50-62` `TicketCreate` — lines 60-62 currently
  `return TicketCreateResult{}, sageReviewStageError(...)` for `state=="ready"` when the freshly
  resolved design posture (`resolved := ResolvedSageReviewPosture(opts.SageReview)`, line 52) is
  not `completed`/`skipped`. Note: `TicketCreate` never checks `blocked` (a brand-new ticket has no
  prior posture to be blocked from — `resolved` only ever comes from `ResolvedSageReviewPosture`,
  whose possible outputs are `recommended`/`required`/`skipped`, never `blocked`). So `TicketCreate`
  only needs the unreviewed-posture warning variant, not the blocked variant. After de-blocking:
  proceed to write the stub (lines 64-76) and append the warning to `TicketCreateResult.Tip`
  (reuse the existing `tip` variable / switch at lines 78-86, which already branches on `state`).
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go:33-46` `TicketMutateResult.Tip` and
  `PartialMutationNotice` — existing non-fatal channel. Reuse `Tip` for the new warning (via
  `appendTip`, lines 165-175). Do NOT invent a new result field for this.
- `agents-plugin-tool/internal/wsdoc/ticket_create.go:18-21` `TicketCreateResult.Tip` — same reuse
  target for `create_empty`.
- **`260713` partial-mutation notice disposition (acceptance item).** `PartialMutationNotice` is
  set in exactly one place: `tickets_mutate.go:136-140`, on the `prepareSageReviewForUpwardMove`
  error branch. Once that function stops erroring for the ready-posture-problem cases (blocked and
  unreviewed), the only remaining way `prepareSageReviewForUpwardMove` returns a non-nil error is
  the `writeFrontmatterField` I/O-error path (line 368-370, e.g. disk full or permission denied) —
  a self-healing-write failure, not a posture rejection. `writeFrontmatterField` (
  `tickets_mutate.go:505-543`) is a single `os.WriteFile` call after building the full new content
  in memory — no partial-write window visible to a caller — so once posture rejection is gone, no
  reachable error path leaves meaningfully-partial frontmatter for a caller to worry about.
  Recommend disposition: **delete `PartialMutationNotice` outright** (the field itself, its
  `tickets_mutate.go:38-45` doc comment, the `server.go:1332-1338` surfacing branch, and the two
  tests that assert on it — see below) rather than keep it pointed at a scenario that no longer
  has meaningful partial-write semantics. State the chosen disposition explicitly in the
  implementation commit (do not leave it implicit).
- `agents-plugin-tool/internal/mcp/server.go:1313-1341` `tickets.move` MCP dispatch — lines
  1332-1338 read `result.PartialMutationNotice` on error and format a `partial-mutation:` line via
  `toolErrorTextResponse`. Per the disposition above, delete this branch and fold back to the plain
  `return toolTextResponse(req.ID, "", err)` — only structural errors (ticket not found, already at
  status, closed-ticket reopen, I/O failure) reach this call site once posture rejection is gone.
- `agents-plugin-tool/internal/mcp/server.go:462-466` `builtinConfigDefaults()` — confirms
  `sage_review: "auto"` ships by default; `wsdoc.ResolvedSageReviewPosture("auto")` (
  `tickets_mutate.go:241-250`) maps to `"required"`. Any new test fixture must set
  `SageReview: "auto"` (Go call) / rely on the builtin default (MCP dispatch), never `"off"`
  (→`skipped`) alone, to exercise the newly-permissive path meaningfully.
- `agents-plugin-tool/internal/mcp/server.go:2677-2688` `formatTicketMutate` and
  `:2665-2675` `formatTicketCreate` — already render `result.Tip` as `"tip: %s\n"` /
  `"Tip: %s\n"` respectively; the new warning text flows through unchanged once it's on `Tip`. No
  format-layer change needed unless the warning is long enough to want its own line — recommend
  keeping it as a `Tip` addition via `appendTip` for consistency with existing tips (spec-address
  warning, sage-review-posture tip) that already stack there.
- `agents-plugin-tool/internal/mcp/server.go:2790-2822` `formatSageGate` / `sageGateNextInstruction`
  — `case "run"` (line 2817-2818) is the ordinary `required` → `run` path where the non-waivable
  statement + review-scope line must also appear (per ticket decision "ride the ordinary path").
  `formatSageGate` currently only prints `action`, `ask_prompt`, `reviewers`, `mode`, `commit`, then
  the single `next_instruction` line. Adding the new prose requires either: (a) a new field on
  `SageGateResult` (e.g. `Advisory string`) populated by `SageGate`/`resolveStage` for the `run` and
  `recommended`-ask cases, rendered by `formatSageGate` before `next_instruction`; or (b) folding
  the text directly into `sageGateNextInstruction`'s `"run"`/`"ask"` case strings. Given the ticket
  wants this text on both the `run` result and the `recommended` ask prompt (`AskPrompt` field,
  already free text), (a) is cleaner: it lets `run` and `ask` share one advisory-building helper
  without overloading `AskPrompt`/`next_instruction` semantics that other callers (tests) already
  match against.
- `agents-plugin-tool/internal/wsdoc/tickets_sage.go:178-212` `resolveStage` — `case "required":
  return stageOutcome{action: "run"}, nil` (lines 205-206) and the `recommended` ask branch (line
  188-204, `askPrompt: "Run " + reviewer + " review for this ticket?"`, line 203) are the two
  places needing the new advisory text (non-waivable statement for `required`; keep it plus the ask
  prompt for `recommended`). `stageOutcome` (lines 84-91) has no field for this text today; add one
  (e.g. `advisory string`) and thread it through `gateResultFromStage` (lines 223-234) into the new
  `SageGateResult` field from the previous bullet. `sageGateCombined` (tickets_sage.go:247-329) has
  three more `run`/`ask` return sites (lines 267, 308, 317, 324, 327) that also need the advisory —
  confirm each is covered, not just the standalone path.
- `agents-plugin-tool/internal/mcp/server.go:4130` (`enumStringProperty("Optional follow-up answer
  to a prior ask action.", ...)`) — confirms `answer`'s schema doc already frames it as
  ask-follow-up only, supporting the ticket's claim that no agent has reason to send `answer` at
  `required` (it never asks).
- `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md:61-70` — current numbering: `### 5.
  Commit` (lines 61-64) then `### 6. Sage Review Gate` (lines 66-70). Swap to `### 5. Sage Review
  Gate` / `### 6. Commit`, preserving each section's internal numbered steps and all
  `{{.McpNamespace}}` placeholders verbatim. Line 70 ("carrying the posture change together with
  any other uncommitted edits already held on the ticket") already reads correctly post-swap — no
  wording change needed there, only the heading order/numbers. No other rsrc file or Go code
  references these step numbers by number (confirmed: `step 6.3` appears nowhere else; `## On:
  Move` step 4 at line 88 references **Spec-address Check**, not a numbered `## On: invoke` step,
  so it is unaffected by the swap).
- `agents-plugin-wsflow/rsrc/lead-write-ticket/lead-write-ticket.md` — byte-identical generated
  mirror; **never hand-edit**. Regenerate per the mandatory command sequence below.
- `agents-plugin-tool/internal/wsdoc/tickets_verify.go:154-166` — the `ready-sage-posture`
  guardrail; calls `sageReviewBlockedError`/`sageReviewStageError` (same constructors as the
  mutation path) to build FAIL finding text. Confirmed: **do not touch this block's control flow**
  (still `addFinding`, still hard). The "stays HARD" comment is at lines 216-218 (
  `ticketUnresolvedPhaseWarning` doc comment, contrasting itself against the sage guardrail) — it's
  a comment on an unrelated function, not on the guardrail block itself; no edit needed there
  either since the contrast statement remains true.
- `agents-plugin/rsrc` regen risk signal: **any edit under `agents-plugin/rsrc/` requires the
  two-command regen + idempotence re-run** before the change is complete (see Implementation Plan
  step 7). Skipping this leaves the wsflow mirror stale and `TestWsflowRsrcMirrorUpToDate` failing.
- `ai-docs/tickets/todo/260723-epic-ticket-write-reshape.md:76-88` — the seed-classification bullet
  already carries a `**Revised 2026-07-26:**` addendum stating the enforcement location moves to
  verify/`git.commit` and is "Owned by `260726-bug-sage-ready-enforcement-single-chokepoint`" —
  this already matches the phase's direction. Confirming this still matches shipped behavior at
  close is a read-only check (no edit expected) unless implementation diverges from what's
  described here.
- Existing tests whose *expectations* (not just fixture values) need to flip from
  error-return-with-message to success-with-warning:
  - `agents-plugin-tool/internal/wsdoc/tickets_mutate_test.go:393-497`
    `TestTicketsMoveUpwardToReadyBlocksUnresolvedSageReviewPosture` (table test, `err == nil` is
    currently the failure branch — invert to assert success + warning content, keeping each
    table case's distinguishing field/posture assertions).
  - `:507-533` `TestTicketsMoveUpwardToReadyFromIdeaBlocksUnresolvedSageReviewPosture` (same
    inversion; keep confirming design-before-completeness field-write order via `after` content
    checks).
  - `:543-580` `TestTicketsMoveBlockedReturnsPartialMutationNotice` — depends on the disposition
    above; if `PartialMutationNotice` is deleted (recommended), delete this test too (its premise,
    a blocking legacy-migration case, no longer exists as an error path).
  - `:608-628` `TestTicketsMoveUpwardToReadyEpicBlocksOnUnresolvedDesign` — invert to success +
    warning.
  - `:678-696` `TestTicketsMoveUpwardToReadyLegacyBlockedStillBlocks` — this is the `blocked`
    variant; invert to success + the **distinct blocked warning text** (ticket requires
    unreviewed vs. `blocked` warnings to differ).
  - `agents-plugin-tool/internal/wsdoc/ticket_create_test.go:108-129`
    `TestTicketCreateReadyBlocksUnresolvedSageReviewDesignPosture` — invert: `TicketCreate` must
    now succeed and the file at `260101-feat-foo.md` must exist (currently asserts
    `os.IsNotExist`), with `Tip` carrying the warning.
  - `agents-plugin-tool/internal/mcp/session_state_test.go:2572-2604`
    `TestServeStdioTicketsMoveBlockedSurfacesPartialMutationNotice` — same disposition dependency
    as the wsdoc-level partial-mutation test above; delete together with it.
- New test coverage needed (none of the above cover it): `tickets.sage_gate` at posture `required`
  returning the non-waivable statement + review-scope line on `run` (verification item 4), and the
  `recommended`-ask variant carrying it too (per ticket decision). No existing test asserts on
  `SageGateResult`'s advisory content since the field doesn't exist yet.

## Implementation Plan

1. **`tickets_mutate.go`**: change `prepareSageReviewForUpwardMove` (lines 341-404) so the
   ready-posture-problem branch (lines 396-402) no longer returns an error for the non-blocked
   case (`sageReviewStageError`) and the blocked case (`sageReviewBlockedError`, both at line
   381-385 for the early-stage blocked check, and 399 for the `readyPostureProblems`-driven check)
   — instead return `(result, nil)` in all cases, keeping the persisted-write behavior (lines
   354-371) untouched. Add a small helper, e.g. `readySagePostureWarning(problems
   []readyPostureProblem) string`, built from the existing `readyPostureProblems` return value,
   producing the two distinct message shapes (unreviewed vs. blocked) per the ticket's target
   message shape — reuse `readyPostureProblems`'s `Blocked` field to pick the variant. Text must
   name: the field, the consequence (`ws/git.commit` will fail on guardrail `ready-sage-posture`),
   the resolving call (`ws/tickets.sage_gate(stem, landing: "ready")`), the non-waivable statement,
   `ws/config.show` as the config pointer, and the review-scope line (design vs. completeness
   distinction).
2. **`tickets_mutate.go`**: update `TicketsMove` (lines 111-163) — since
   `prepareSageReviewForUpwardMove` no longer errors on posture, its call site (lines 135-140)
   simplifies to just propagating any remaining structural/I-O error. Capture the returned
   `sageReviewPostures`/problems and, for `to == "ready"`, append the new warning to `result.Tip`
   via `appendTip` alongside the existing `readyGateWarning` call (lines 157-161) — do not
   overwrite the existing spec-address tip; both must be able to coexist on one move.
3. **`ticket_create.go`**: change `TicketCreate` (lines 50-62) so the `state == "ready"` +
   non-terminal-design-posture branch no longer returns an error; instead proceed to write the stub
   and set `TicketCreateResult.Tip` to the same warning text/helper from step 1 (design-only, since
   `TicketCreate` never has a `blocked` case — confirmed in Codebase Findings). Reuse the tip-switch
   at lines 78-86 or extend it with a new case.
4. **`tickets_sage.go`**: add an advisory-text field (e.g. `Advisory string`) to `SageGateResult`
   (lines 37-49) and `stageOutcome` (lines 84-91, e.g. `advisory string`). Populate it in
   `resolveStage`'s `case "required"` (lines 205-206) with the non-waivable statement + review-scope
   line, and in the `case "recommended"` ask branch (lines 188-204, specifically the `default`
   sub-case building `askPrompt` at line 203) with the same text appended to the ask flow. Thread
   the field through `gateResultFromStage` (lines 223-234) and through all `sageGateCombined`
   run/ask return sites (lines 267, 308, 317, 324, 327) so the combined-mode paths carry the
   advisory too, not just the standalone path.
5. **`server.go`**: update `formatSageGate` (lines 2790-2807) to print the new `Advisory` field
   (when non-empty) before `next_instruction`. Implement the `PartialMutationNotice` disposition
   from Codebase Findings (delete the field, the `tickets_mutate.go:38-45` doc comment, the
   `server.go:1332-1338` call site collapsing to the plain error return, and the two now-obsolete
   tests `tickets_mutate_test.go:543-580` and `session_state_test.go:2572-2604`). State the chosen
   disposition in the commit body since it's an acceptance item.
6. **Update existing tests** per the Codebase Findings list: invert the five remaining
   error-expecting-now-success-expecting tests in `tickets_mutate_test.go` and
   `ticket_create_test.go`, preserving each test's field/write/warning-content assertions in their
   new success-path form; ensure the `blocked` variant (`TestTicketsMoveUpwardToReadyLegacyBlockedStillBlocks`)
   asserts the distinct blocked-warning wording. Add new coverage for `tickets.sage_gate`
   `required` → `run` and `recommended` → ask both carrying the `Advisory`/non-waivable text
   (verification item 4).
7. **`agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`**: swap `### 5. Commit` and `###
   6. Sage Review Gate` (lines 61-70) to `### 5. Sage Review Gate` / `### 6. Commit`, keeping each
   section's internal step numbering and `{{.McpNamespace}}` calls unchanged. After this edit, run
   (from `agents-plugin-tool/`), in order, both with `-count=1`:
   ```
   WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest
   WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
   ```
   then re-run both again to confirm idempotence (no diff on the second run). Never hand-edit
   `agents-plugin-wsflow/rsrc/`.
8. **Confirm (read-only)** that `ai-docs/tickets/todo/260723-epic-ticket-write-reshape.md`'s
   already-revised seed-classification bullet (lines 76-88) still matches shipped behavior after
   the above changes; no edit expected unless something diverges.
9. **Spec closeout is out of scope for this edit step** (per Spec Impact: "Post-implementation
   closeout updates `mcp-tools.md` with the shipped contract"); leave `ai-docs/spec/mcp-tools.md`
   untouched during implementation. Anchors that will need updating at closeout, for the
   documentation pre-pass to pick up: `{#260620-ticket-move-tool}` (lines 872-901, especially the
   "stop with an action-oriented message" line 881-882 and the partial-mutation paragraph
   892-900 — both describe behavior this phase changes), `{#260622-create-ticket-tool}` (lines
   903-923, specifically "the call is rejected with an action-oriented error" at 917-919), and
   `{#260720-sage-gate-record-tools}` (lines 1093-1125, needs the new `Advisory`/non-waivable
   contract on `run`/`ask`).

## Verification Plan

- `cd agents-plugin-tool && go test ./internal/wsdoc/... -run 'TicketsMove|TicketCreate' -v` —
  confirms the inverted tests pass and no other `TicketsMove`/`TicketCreate` test regresses.
- `cd agents-plugin-tool && go test ./internal/mcp/... -run 'TicketsMove|SageGate' -v` — confirms
  MCP dispatch layer (including whatever remains of the partial-mutation surfacing) still passes.
- `cd agents-plugin-tool && go test ./... -count=1` — full package sweep before calling this done.
- Mandatory rsrc regen sequence (see Implementation Plan step 7) — both commands, twice each, to
  confirm idempotence; `TestWsflowRsrcMirrorUpToDate` (part of the `go test ./...` sweep) is the
  gate that catches a stale mirror.
- Manual/tool-level walkthrough of the ticket's 5-item verification boundary: create a `ready/`
  ticket via `tickets.create_empty` under `sage_review: auto` and confirm success+warning; move a
  `todo/` ticket with unresolved posture to `ready/` and confirm success+warning; move a separate
  `todo/` ticket with `sage-review-design: blocked` to `ready/` and confirm success + the distinct
  blocked warning; call `ws/git.commit` on either resulting ticket and confirm it still FAILs on
  `ready-sage-posture`; call `tickets.sage_gate` at `required` and confirm the non-waivable
  statement + review-scope line + resolving call all appear on the `run` result; run a `todo/` →
  `ready/` promotion through the rendered (post-regen) `lead-write-ticket` playbook end-to-end with
  no hand edit, confirming Sage Review Gate now runs before Commit.

## Escalations

- None.
