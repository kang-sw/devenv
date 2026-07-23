# Plan: 260723-feat-ticket-system-concept-doc — Phase 1: Author the concept doc and de-duplicate the glosses

## Relevant Ticket Contract

- Write one concept doc covering six areas: status-dir meaning, type-prefix
  distinction (categorization guidance only — `feat`/`bug`/`refactor`/`chore`
  are mechanically identical per `judge: ticket-category`, so wording must not
  imply behavioral divergence), sage-review rationale + posture meaning,
  spec-addressing purpose, the phase model, epic-vs-workset distinction.
- Home: the `workflow_manual` bundle (`rsrc/lead-workflow-manual/`), dual-tree
  (`agents-plugin/` canonical + `agents-plugin-wsflow/` mirror).
- **Concepts only, never guardrails** — any invariant `ticket.verify`
  mechanically enforces stays where it is enforced; the doc must not soften a
  hard guardrail into prose.
- Strip duplicated *explanatory* prose from two files, replacing with a
  concept-doc reference; keep mechanical rules/hard invariants in place:
  - `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md`
  - `lead-write-ticket.md` `judge:` tables (`ticket-category`,
    `initial-status`, `spec-address-gate`) — keep decision criteria, drop
    concept re-explanation. (Both `agents-plugin/rsrc/lead-write-ticket/` and
    `agents-plugin-wsflow/rsrc/lead-write-ticket/` copies.)
- Gloss removal is the **highest-risk step**: `260702-research-destructive-dedup-methodology`
  documents this exact failure mode (silent guardrail loss, under- vs.
  over-capture, flow-position drift) and states the per-merge audit is **not
  yet codified**. Treat every removed line with close, individual review — do
  not assume a methodology exists to lean on.
- Inline-vs-referenced payload knob: **measure, then pick**, defaulting to
  tight inline; degrade to referenced only if the manual-payload delta is
  unacceptable.
- Acceptance check must (a) confirm all six areas are covered, (b) compare
  token count of the write-ticket base path (`lead-write-ticket.md` +
  `ticket-conventions.md`) before vs. after, stating the amortization boundary
  explicitly (per-write savings vs. once-per-session manual baseline), and (c)
  confirm no hard invariant was softened.
- Go constants (`tickets_template.go`, `tickets_checklist.go`,
  `tickets_sage.go`) are **out of scope** — read only, never edited; they
  remain the mechanical source of truth, and verify owns enforcement.

## Out of Scope

- Editing Go source (`tickets_template.go`, `tickets_checklist.go`,
  `tickets_sage.go`, `tickets_mutate.go`, `tickets_verify.go`).
- Spec authoring — the ticket states the spec entry is a post-implementation
  closeout (`Spec Impact`), not contract-first; do not write spec text this
  phase.
- Codifying the 260702 audit methodology itself — apply close per-line review
  using the approach below, do not attempt to generalize or formalize it.
- Any phase-2+ scope not named in Phase 1 (none currently defined on this
  ticket).

## Codebase Findings

- `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md#L1-8` —
  bundle frontmatter pattern: `kind: print`, `includes: [native-spawn-binding]`,
  `variables: [...]`. The concept doc's home is this same bundle; either add
  content inline into this file or as a new sibling file wired through
  `includes:`.
- `agents-plugin/rsrc/lead-workflow-manual/native-spawn-binding.md` (0 lines,
  empty) vs. `native-spawn-binding.codex.md` (10 lines) — **existing
  precedent for the referenced-file pattern** inside this same bundle
  (per-harness variant selection via `includes:`). Confirms `includes:` can
  pull in a sibling file; use this as the mechanical template if the
  "referenced" fallback is chosen over inline.
- `agents-plugin-wsflow/rsrc/lead-workflow-manual/lead-workflow-manual.md` —
  byte-identical to the canonical copy today (verified via diff). **Do not
  hand-edit this file.** It is a generated mirror.
- `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L47-81` —
  `TestWsflowRsrcMirrorUpToDate` fails the build if `agents-plugin-wsflow/rsrc/`
  drifts byte-for-byte from `agents-plugin/rsrc/`. The correct edit sequence is:
  edit canonical `agents-plugin/rsrc/` only, then regenerate the mirror with
  `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
  (documented at `wsflow_mirror_test.go#L83-114`), then commit both trees. This
  supersedes a literal reading of the ticket's "edit both trees" — the
  mechanism is generate-then-commit, not parallel hand-edits.
- `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md` (63
  lines, full file read) — mixed mechanical-rule/explanatory-gloss text
  throughout. Concrete per-section classification for the executor:
  - `#L13` "Status is directory-based only... Never duplicate status in
    frontmatter." — **hard invariant, keep verbatim.**
  - `#L14` "`idea/` is rough capture... `ready/` is the spec-addressed
    implementation-ready status." — **pure meaning gloss, prune-candidate**
    (concept doc should own "what does each status dir mean").
  - `#L22` "...require spec addressing through `spec:`... epics decompose
    scope, research captures findings, worksets collect operating context." —
    first clause is a mechanical rule enforced by
    `tickets_mutate.go` (`sageReviewStageRequirement` /
    `exemptReadyGateCategories`, see below) — **keep**; the trailing clause
    ("epics decompose scope...") is pure category-meaning gloss —
    **prune-candidate**.
  - `#L23-24` epic/workset ready-gate exemption statements — **keep**; this is
    the prose mirror of a real Go-enforced exemption (see finding below), and
    removing it would drop the only place a reader sees *why* the exemption
    exists at the convention layer (verify.go does not explain rationale, only
    enforces heading shape).
  - `## Epic Tickets` (`#L29-34`) and `## Workset Tickets` (`#L36-41`) —
    entirely explanatory ("epic bodies preserve board-level context...",
    "workset bodies preserve non-hierarchical operating context...") with
    **no corresponding Go enforcement found** (see finding below) — strong
    prune-candidates, but verify each sentence individually: some phrasing
    ("do not use implementation phases", "do not add/remove/change `parent:`
    based on workset inclusion") reads as an operational rule an agent must
    still obey even though nothing mechanically checks it — these are
    unenforced hard invariants, not restatements, and must stay (per ticket
    text: "keep the mechanical rules and hard invariants").
  - `## Phases` (`#L43-49`) — `#L45` "mark dropped phases `[dropped]`, never
    renumber" is an **unenforced hard invariant** (see verify.go finding
    below: only heading *format* is checked, not renumbering) — **keep**.
    `#L46` "One phase is one complete behavior..." and `#L47-48` are
    definitional/meaning — **prune-candidates**.
  - `## General` (`#L56-63`) — Result/Edition heading format and the
    "frozen after Result" rule: heading *shape* is enforced by
    `tickets_verify.go` (finding below), but the *freeze* semantics
    ("Existing Result and Edition entries are frozen once written; append a
    new Edition instead of editing prior result text") are **not** enforced
    by any Go check found — **keep as hard invariant**, even though it reads
    as prose.
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L177-186,#L252-277` —
  `ticketCategoryRE` and `sageReviewStageRequirement` mechanically enforce
  the epic/workset exemption from ready-gate and sage-review-stage
  requirements (`research`/`workset` exempt from both stages, `epic`
  design-only). This is real Go enforcement — the convention lines that state
  the *existence* of the exemption are safe restatements of an enforced rule;
  the lines that explain *why* worksets/epics differ conceptually are not
  enforced anywhere and are the concept doc's job to own once, not the
  convention doc's job to keep re-explaining per read.
- `agents-plugin-tool/internal/wsdoc/tickets_verify.go#L42-253` (full function
  list read) — `TicketVerify` mechanically checks: frontmatter fence
  well-formedness, ready-posture (sage) consistency, close-date field
  presence, and `### Phase N` / `### Result (<hash>) - YYYY-MM-DD` /
  `#### Edition (<hash>) - YYYY-MM-DD` heading **format** only. It does
  **not** check: phase renumbering, stem immutability, Result/Edition text
  freeze, or any semantic (non-shape) rule. Any convention-doc line describing
  one of these unchecked behaviors is a hard invariant that exists nowhere
  else — do not prune it.
- `agents-plugin-tool/internal/wsdoc/tickets_sage.go#L1-24,#L91-158` — the
  only existing source for sage-review rationale/posture semantics
  (`skipped`/`blocked`/`recommended`/`required`/`completed`/`pending`,
  design-vs-completeness stage gating, combined-mode resolution order). No
  doc currently explains this (confirmed: `ticket-conventions.md` has zero
  sage-review section, `lead-write-ticket.md` only has the mechanical call
  site). This is new content to *add* to the concept doc, not a dedup target
  — there is nothing to prune here since no prose restatement exists yet.
- `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md#L142-193` (full
  Judgments section read; identical in the wsflow mirror) — three tables in
  scope:
  - `judge: ticket-category` (`#L142-148`) — each category line doubles as
    *both* the definition and the classification criterion a judge needs to
    operate; this cannot be deleted outright without breaking the judge's
    function. The safe edit is trimming the definitional flourish while
    keeping a short discriminating trigger phrase, and pointing to the
    concept doc for the "why" — not wholesale removal.
  - `judge: initial-status` (`#L156-162`) — same shape: short criteria
    already, minimal gloss to remove.
  - `judge: spec-address-gate` (`#L150-154`) — already terse and refers out
    to **On: Spec-address Check** and `judge: missing-spec-address`; little to
    prune here beyond possibly tightening wording once the concept doc exists.
  - Type-prefix semantic distinction (`feat`/`bug`/`refactor`/`chore`) is
    **not currently glossed anywhere** (confirmed: `judge: ticket-category`
    already lists them flatly, `ticket-conventions.md#L8` is a bare list) —
    this is new concept-doc content to add, not a prune target.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1905-1960` —
  `TestPlaybookPrintGoldenLeadWriteTicket` is the golden test that will need
  re-running (and possibly light updating if an asserted substring lives
  inside an edited line range) after the `lead-write-ticket.md` edit. Current
  asserted/forbidden substrings do not currently overlap the judge-table gloss
  text targeted for pruning, but re-grep this test after drafting the diff to
  confirm no collision.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1860-1884`
  (`TestPlaybookPrintGoldenLeadWorkflowManual`) and `#L553-590`
  (`TestPlaybookPrintGoldenLeadWorkflowManualScopedExplorationTierModels`) —
  golden tests for the bundle the concept doc lands in; both will need a
  follow-on look (not necessarily edits) once the new content changes the
  rendered body, and both already assert `{{.` placeholder absence, which
  keeps applying to any new variable-bearing content.
- `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go` — the parity
  golden test for the dual tree (see above); this is the test to run, not
  edit, after regenerating the mirror.
- No repo mechanism computes LLM token counts (`grep` for
  `token`/`tiktoken`/`estimateTokens` across `agents-plugin-tool` found no
  tokenizer utility, and no golden test asserts a byte/size budget on any
  playbook body). The ticket's "compare token count... before vs. after"
  acceptance check has **no automated tool to run** — treat it as a manual
  measurement (e.g., word/byte count of the rendered `lead-write-ticket` +
  `ticket-conventions.md` pair via `printPlaybook` output or direct file
  size, reported in the ticket's Result text), not a test assertion.

## Implementation Plan

1. Read `agents-plugin-tool/internal/wsdoc/tickets_sage.go` posture/rationale
   comments (already located above) as the sole source for the sage-review
   concept section; do not infer posture semantics from `lead-write-ticket.md`
   alone (it only has the mechanical call site left after the 260701/260723
   Go relocation).
2. Draft the concept doc content (all six areas) as a candidate section for
   `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`. Default
   to tight inline (append as a new `##`-level section, e.g. "Ticket System
   Concepts", after the existing `## Available` primitives content, before
   `## Planned Or Specialized`). Keep prose concept-only — no invariant
   duplicated from `tickets_verify.go`'s enforced checks stated as if it were
   the concept doc's job to enforce it.
3. Measure the manual-payload delta: rendered body length before vs. after
   (via `printPlaybook` in a scratch test or direct file diff line/byte
   count). If the inline addition looks disproportionate against the existing
   ~180-line manual body, switch to the referenced fallback using the
   `native-spawn-binding.md`/`.codex.md` `includes:` pattern as the mechanical
   template (new sibling file, wired via `includes:` in the frontmatter) and
   state the decision + measured delta in the ticket's `### Result`.
4. Apply the identical content change to
   `agents-plugin-wsflow/rsrc/lead-workflow-manual/lead-workflow-manual.md` —
   **do not hand-edit it**; after finishing step 2-3 in the canonical tree,
   run `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
   to regenerate it, then verify with
   `go test ./internal/wsrsrc -run TestWsflowRsrcMirrorUpToDate`.
5. Gloss removal in `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md`:
   for each candidate line identified in Codebase Findings, apply this
   per-line audit before deleting (per 260702's working hypothesis, since no
   codified checklist exists):
   - Is this line the *only* place the rule/invariant is stated (cross-check
     `tickets_verify.go`, `tickets_mutate.go` category/exemption logic, and
     `tickets_checklist.go`)? If yes — keep verbatim regardless of how
     "conceptual" it reads.
   - Does deleting it lose an under-capture-only-covered fact (something the
     concept doc's *rewrite* won't independently restate), or does it also
     risk over-capturing (moving something that was actually a rule, not a
     concept, into the "soft prose" doc)? Check both directions explicitly,
     not just "does the concept doc say something similar."
   - Does the line encode a flow-position/ordering fact (e.g., "before
     drafting", "only after X")? If so, confirm the concept doc's replacement
     text — or the reference left in place — does not strand that ordering.
   - After drafting each deletion, diff the ticket-conventions.md before/after
     against this plan's classification table above; do not silently drop a
     line not called out as a prune-candidate here without repeating the
     four checks above for it.
6. Apply the equivalent per-line audit and edit to the `judge: ticket-category`
   / `judge: initial-status` / `judge: spec-address-gate` tables in
   `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md` — trim
   definitional flourish, keep the discriminating criterion each judge needs
   to function, and add a concept-doc reference (e.g., "see the workflow
   manual's ticket-system concepts" or equivalent in-repo phrasing) in place
   of the removed explanation. Do not delete a judge table wholesale — these
   tables are load-bearing decision logic, not pure prose.
7. Mirror step 6's exact diff into
   `agents-plugin-wsflow/rsrc/lead-write-ticket/lead-write-ticket.md` via the
   same regenerate-mirror flow as step 4 (both `lead-write-ticket` and
   `lead-workflow-manual` bundles regenerate in one
   `TestRegenerateWsflowRsrcMirror` run).
8. Add the type-prefix categorization guidance to the concept doc as new
   content (not a dedup) — plain-word guidance only, explicitly stating the
   workflow treats all four prefixes identically per `judge: ticket-category`
   and must not imply behavioral divergence.
9. Grep `agents-plugin-tool/internal/mcp/playbook_tools_test.go` for any
   asserted/forbidden substring that falls inside the edited line ranges in
   `ticket-conventions.md` or `lead-write-ticket.md` before finalizing the
   diff; adjust the test only if an assertion targeted text that was
   intentionally pruned as a gloss (not if it targeted a kept invariant).

## Verification Plan

- `go test ./internal/mcp/... -run TestPlaybookPrintGoldenLeadWriteTicket` —
  confirms the `lead-write-ticket.md` edit still resolves and keeps required
  substrings.
- `go test ./internal/mcp/... -run TestPlaybookPrintGoldenLeadWorkflowManual` and
  `-run TestPlaybookPrintGoldenLeadWorkflowManualScopedExplorationTierModels` —
  confirms the concept doc addition renders cleanly with no unsubstituted
  `{{.` placeholders and preserves the harness-specific tier-model sentence.
- `go test ./internal/wsrsrc/... -run TestWsflowRsrcMirrorUpToDate` — confirms
  the regenerated wsflow mirror is byte-identical to canonical after step 4/7.
- `go test ./internal/wsdoc/...` (or a targeted `-run` covering
  `ticket-conventions.md` loading, e.g. convention-read tests if present) —
  confirms the trimmed convention doc still loads/parses.
- Manual: confirm all six concept areas are present in the drafted section by
  re-reading it against the ticket's `## Decisions` list line-by-line.
- Manual: report the before/after size of the `lead-write-ticket.md` +
  `ticket-conventions.md` pair (word or byte count; no tokenizer utility
  exists in-repo) in the ticket's `### Result`, stated against the documented
  amortization boundary (per-write gloss savings vs. once-per-session manual
  baseline).
- Manual: re-read every pruned line against the four-question audit in
  Implementation Plan step 5 one final time before commit, since this is the
  ticket's explicitly named highest-risk step and no automated check exists
  for guardrail-vs-restatement.

## Escalations

- None.
