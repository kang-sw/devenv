# Plan: 260825-feat-impl-branch-single-ticket-scope-merge-timing — Phase 2: default no-merge per phase + tickets.close merge-review trigger + ticket-declared stop-gate exception

## Relevant Ticket Contract

- Per-phase final action defaults to continue-on-branch with **no** merge; the
  gate's explicit merge option stays available; `merge_confirm` (ask/skip)
  applies only when a merge is actually chosen. Do **not** add a
  proceed→implement completion signal.
- Add a merge-review trigger to `tickets.close`: closing a ticket while the
  current branch is an `impl/*` branch with `AheadOfMergeRoot > 0` returns a
  `next_instruction` nudging review-and-merge of `impl/<root>/<stem>` into
  `<root>`, sequenced after the close move commit. `tickets.close` performs no
  merge itself.
- Ticket-declared per-phase user stop gate is an **override** that stops the
  run at that phase regardless of the no-merge default — this is ordinary lead
  judgment over free-form phase prose; **no new marker format, schema field, or
  code path**.
- Reuse Phase 1's `AheadOfMergeRoot` observation (`implementBranchObservation`,
  `observeImplementBranch`, `parseImplBranchRoot`, `implementMergeRootFor` —
  all already landed in `agents-plugin-tool/internal/mcp/implement_resolver.go`
  at commit `3db94261`). No new git-observation code is needed for the
  detection itself.
- Host-neutral: no `develop`/`main` hardcoding — the merge-review nudge names
  the branches derived from `impl/<root>/<stem>` encoding only.
- Coordinate (do not implement) with epic ③ (`260824-feat-review-watermark-ledger`,
  currently **unimplemented**, in `todo/`): structure the `tickets.close` hook
  to return guidance only, keyed on the impl-branch observation, so a later
  hook composes without rework. Do not write code for ③.

## Out of Scope

- Phase 1's resolver stop-gate logic (`finishImplementBranchPlanTail`) — already
  landed, reused as-is via `AheadOfMergeRoot`.
- Any code for the ticket-declared stop-gate exception — it is lead judgment
  over existing phase prose, not a code path.
- Any code for epic ③'s marker/sweep recompute — it does not exist yet.
- Skill-prompt changes to `agents-plugin/skills/lead-drain-ready-queue/SKILL.md`
  or `agents-plugin/rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md` —
  read and confirmed both already treat "merge" as a single lead-driven,
  once-per-ticket action gated on approval (fan-out: "Stop at lead-proceed's
  merge gate — do not merge" / "Collect and merge, one at a time"; drain-queue:
  goal-branch terminal is a single explicit-approval merge into PARENT). The
  Phase 2 default flip composes with this existing posture without doc changes.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/session_state.go#L403-L428` —
  `deriveImplementTodosFromVerdict` always appends `final-action-gate` + `merge`
  todo items (keys, in that order) for every non-`current`, non-error verdict,
  regardless of which phase this is. There is **no** intermediate/final phase
  signal anywhere in this package — `implementTodoVerdict` carries no phase
  index/position field. Phase 2's "intermediate and final phases both default
  to no-merge" requirement is satisfied automatically by construction: the same
  instruction-generation functions run on every `lead-implement`/`enter.implement`
  call, so there is nothing phase-position-specific to branch on — verification
  exercises varied `BranchPlan.Action` (`continue`/`create`/`rename`) and
  `MergeConfirm` values instead of a nonexistent phase field.
- `agents-plugin-tool/internal/mcp/session_state.go#L632-L648` —
  `implementFinalActionInstruction(verdict)`: today asks for "final action
  approval" (i.e., approval to merge) by default, dropping the ask only when
  `MergeConfirm == "skip"`. Must be reworded so the default path states the
  outcome is continue-on-branch-without-merge (report branch/commit range,
  consistent with `tickets.close`'s later review), while still describing the
  explicit-merge option a caller can choose.
- `agents-plugin-tool/internal/mcp/session_state.go#L650-L658` —
  `implementMergeInstruction(verdict)`: today unconditionally instructs
  performing "the selected final action against the verdict merge target"
  (i.e., merge always happens; only the approval-ask is skipped/kept per
  `MergeConfirm`). Must be reworded to opt-in: state this step only runs when a
  merge was explicitly chosen (at the gate, or later at `tickets.close`
  review), and that `MergeConfirm` ask/skip governs approval for that chosen
  merge only — never a default trigger.
- `agents-plugin-tool/internal/mcp/session_state.go#L660-L662` —
  `implementCompletionInstruction(verdict)` (used only for the `current` action
  low-ceremony path, precedent from 260711) is the existing model for a
  no-merge completion report ("report the retained branch, commit range,
  ... and no-merge completion. Do not push."). Reuse this phrasing pattern for
  the new default final-action wording rather than inventing new vocabulary.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L140-L149` —
  `implementBranchObservation` already carries `AheadOfMergeRoot int` (Phase 1).
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L441-L474` —
  `observeImplementBranch(root string, targetBranch string) (implementBranchObservation, error)`.
  Calling with `targetBranch=""` (precedented at `session_state.go#L1020`) is
  sufficient to get `CurrentBranch` + `AheadOfMergeRoot`; `TargetExists`/
  `MergeRootRefConflict` stay zero-value and are irrelevant to the nudge.
  `AheadOfMergeRoot` is computed only when the branch is `impl/`-prefixed with a
  parseable merge root differing from itself (`L454-L459`), so a `goal/*` or
  plain feature branch always yields `0` here — this is what prevents the
  nudge from ever firing on a goal-drain's own `goal/*` staging branch (see
  Out of Scope).
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L736-L747` —
  `parseImplBranchRoot(branch string) (root, stem string, ok bool)` — splits
  `impl/<root>/<stem>` on the **last** `/`. Use this to build the nudge's
  `impl/<root>/<stem>` → `<root>` message; zero commit-content parsing,
  consistent with Phase 1's SAFETY/IDENTITY separation.
- `agents-plugin-tool/internal/mcp/server.go#L1214-L1244` — the `tickets.close`
  MCP handler. `root, err := s.resolveToolRoot(...)` is already resolved at
  `L1218` before `wsdoc.TicketsClose` runs, giving the handler session-root git
  access needed to call `observeImplementBranch` directly (same package,
  no new import). The success path returns
  `toolTextResponse(req.ID, formatTicketMutate("closed", result), nil)` at
  `L1244` — this is the sole return point to extend for the merge-review
  nudge. `tickets.move`'s case (`L1245-L1273`) must NOT gain this nudge.
- `agents-plugin-tool/internal/mcp/server.go#L2572-L2598` —
  `formatTicketMutate(verb, result)` and `ticketMutateNextInstruction(verb, result)`
  are the existing `next_instruction:` composition path (used today for the
  "moved to ready/" and "closed with unresolved phase" nudges, 260723 Phase 2
  precedent). `ticketMutateNextInstruction` takes no `root`/git argument, so it
  cannot itself call `observeImplementBranch` without a signature change.
  Cleanest fit: keep `ticketMutateNextInstruction` as-is and append a second,
  independently-computed `next_instruction:` line from the `tickets.close` case
  in `server.go` (two lines is an established shape already — the "closed with
  unresolved phase" tip and this nudge are orthogonal conditions that can both
  be true for the same close call).
- `ai-docs/spec/mcp-tools.md#L1195-L1207` (`{#260620-ticket-close-tool}`) —
  states plainly: **`tickets.close` "is atomic: the frontmatter write, `git
  add`, and `git mv` happen as one staged change set, and the tool never
  commits."** This confirms the ticket's "sequenced after the close move
  commit" wording: the close-move commit is itself a *separate, later* lead
  action (via `ws/git.commit`), not something `tickets.close` performs. The
  nudge computed inside the `tickets.close` handler call therefore observes
  `AheadOfMergeRoot` **before** that commit lands — this is still correct
  evidence (staged-but-uncommitted changes don't affect `rev-list --count`), and
  the nudge's own text is what tells the lead to land the close commit first,
  then merge.
- `ai-docs/spec/mcp-tools.md#L283-L287` — the merge-confirmation sentence to
  update: "...whether the caller's own merge-approval ask may be skipped for
  this merge; merge confirmation defaults to asking unless the caller
  explicitly passes `policy.branch.merge_confirm: skip`." Needs a clause that
  the per-phase default is now continue-without-merge, and `merge_confirm` only
  governs approval for a merge that is explicitly chosen (at the gate or at
  `tickets.close` review).
- `agents-plugin-tool/internal/mcp/session_state_test.go` — existing tests hard-code
  the current default-merge wording and will need rewrites, not just additions:
  `L63-L84` (`TestDeriveImplementTodos`, todo-key shape only — unaffected),
  `L105-L130`, `L392-L490` (`TestDeriveImplementTodoInstructionsMergeConfirmSkip`
  — asserts `"After user approval"` appears by default and `merge_confirm=skip`
  merely drops the *ask*, i.e. asserts today's always-merge default; must
  invert to assert no-merge-by-default plus explicit-merge-still-honors
  `merge_confirm`), `L492-L513` (`TestDeriveImplementTodoInstructionsBranchStop`),
  `L956`, `L1905`, `L2060`, `L2090`, `L2211-L2215`. Treat these as "update in
  place," not additive — the whole point of Phase 2 is that the old default
  wording becomes wrong.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L2486-L2513` —
  `TestServeStdioTicketsCloseUnresolvedPhaseStatesSoftWarnNextInstruction` is
  the exact template to copy for the new `tickets.close` merge-nudge
  integration test: `useLeadProfile(t)`, `t.TempDir()`, `initGit(t, root)`,
  `mustWrite` a ticket, `NewServer`, `callLogin`, `callToolWithKey(..., "tickets.close", ...)`,
  assert on `strings.Contains(closeResp, "next_instruction: ...")`.
- `agents-plugin-tool/internal/mcp/server_test.go#L698-L707` (`initGit`) and
  `#L966-L975` (`runGitOutput`/`runGit`) — helpers for constructing the real
  `impl/<root>/<stem>` branch-with-unmerged-commits fixture the new
  `tickets.close` tests need (Phase 1 left `AheadOfMergeRoot`'s real git
  computation, `aheadOfMergeRootCount` at `implement_resolver.go#L419-L439`,
  covered only by hand-built `implementBranchObservation` unit tests — no
  integration test yet exercises real git branches for it; the new
  `tickets.close` test is also the first integration coverage of the real
  `rev-list --count` path).
- `agents-plugin/skills/lead-drain-ready-queue/SKILL.md` and
  `agents-plugin/rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md` —
  read in full; both already gate merge as a single, lead-approved,
  once-per-unit action (drain-queue: one goal-branch→PARENT merge at the
  `ready/`-empty terminal, on explicit approval; fan-out: one
  `impl/<parent>/<stem>`→parent merge per worker, at the worker's stop-at-merge-gate).
  Neither relies on the old per-phase auto-merge default, so no doc edits are
  required by Phase 2 — cited as **verification evidence**, not an
  implementation target.

## Implementation Plan

1. `agents-plugin-tool/internal/mcp/implement_resolver.go`: add
   `implementCloseMergeReviewNudge(root string) string` near
   `parseImplBranchRoot`/`aheadOfMergeRootCount`. Body: call
   `observeImplementBranch(root, "")`; on error, fail open and return `""`
   (advisory-only, must never block or error the close). Then
   `mergeRoot, stem, ok := parseImplBranchRoot(obs.CurrentBranch)`; if `!ok` or
   `obs.AheadOfMergeRoot <= 0`, return `""`. Otherwise return a nudge string
   naming `impl/<mergeRoot>/<stem>` and `<mergeRoot>`, stating the tool
   performed no merge and that the merge is a lead action to take after landing
   the close-move commit.
2. `agents-plugin-tool/internal/mcp/server.go`: in the `tickets.close` case
   (around `L1244`), replace the direct
   `return toolTextResponse(req.ID, formatTicketMutate("closed", result), nil)`
   with: build `text := formatTicketMutate("closed", result)`, then if
   `nudge := implementCloseMergeReviewNudge(root); nudge != ""`, append a
   second `"next_instruction: " + nudge + "\n"` line, then return
   `toolTextResponse(req.ID, text, nil)`. Do not touch the `tickets.move` case
   or `formatTicketMutate`'s signature.
3. `agents-plugin-tool/internal/mcp/session_state.go`: rewrite
   `implementFinalActionInstruction` (`L632-648`) and `implementMergeInstruction`
   (`L650-658`) so the produced text states continue-on-branch-without-merge as
   the default outcome (model the no-merge phrasing on
   `implementCompletionInstruction`, `L660-662`), while preserving an
   explicit-merge option whose approval behavior still branches on
   `verdict.BranchPlan.MergeConfirm` (`skip` → no approval ask for that chosen
   merge; `ask`/absent → require approval) — i.e. move `MergeConfirm`'s effect
   from "gates whether the ask is skipped before an always-happening merge" to
   "gates whether an explicitly-chosen merge needs approval." Keep the two todo
   keys (`final-action-gate`, `merge`) and their call sites in
   `deriveImplementTodosFromVerdict` unchanged — only the instruction text
   changes. Do not add any phase-index/position field or a proceed→implement
   completion signal.
4. `agents-plugin-tool/internal/mcp/session_state_test.go`: update the tests
   listed in Codebase Findings (`L392-L490` centrally,
   `TestDeriveImplementTodoInstructionsMergeConfirmSkip` in particular) to
   assert the new default: `final-action-gate`/`merge` instructions state
   no-merge by default; a case representing an explicit merge choice still
   honors `MergeConfirm` ask/skip for that chosen merge. Keep
   `TestDeriveImplementTodoInstructionsBranchStop` and
   `TestDeriveImplementTodoInstructionsCurrentBranchCompletion` passing
   (branch-stop and current-branch-completion paths are untouched by this
   phase). Add explicit coverage that both a `continue` and a `create`
   `BranchPlan.Action` (standing in for "any phase, since there is no
   phase-position field") default to no-merge, satisfying the ticket's
   "intermediate and final phases" verification language.
5. `agents-plugin-tool/internal/mcp/session_state_test.go` (or
   `tickets_scope_test.go`, matching the `TestTicketsCloseDeliversPartialMutationNoticeOverMCP`
   fixture style): add `tickets.close` MCP-level tests using the
   `TestServeStdioTicketsCloseUnresolvedPhaseStatesSoftWarnNextInstruction`
   pattern:
   - unmerged case: `initGit`, commit on a root branch, `git checkout -b
     impl/<root>/<stem>`, add a commit ahead of root, write+close the ticket →
     assert `closeResp` contains a `next_instruction:` mentioning merge review
     of `impl/<root>/<stem>` into `<root>`.
   - merged/clean case: same branch but with the impl branch fast-forwarded
     into (or identical to) root before close → assert no merge-review
     `next_instruction` appears.
   - non-impl case: close on a plain branch (e.g. the initial `initGit`
     default branch, never `impl/`-prefixed) → assert no merge-review
     `next_instruction` appears.
6. `ai-docs/spec/mcp-tools.md`: update the merge-confirmation sentence at
   `L283-L287` (per-phase default is continue-without-merge;
   `merge_confirm` governs approval only for an explicitly chosen merge) and
   the `tickets.close` description at `{#260620-ticket-close-tool}`
   (`L1195-L1207`) to document the new merge-review `next_instruction` nudge
   (fires when closing on an unmerged `impl/*` branch; the tool performs no
   merge itself).

## Verification Plan

- `cd agents-plugin-tool && go test ./internal/mcp/...`
- Focused sub-runs while iterating:
  `go test ./internal/mcp/... -run TestDeriveImplementTodoInstructionsMergeConfirmSkip`,
  `-run TestDeriveImplementTodoInstructionsBranchStop`,
  `-run TestDeriveImplementTodoInstructionsCurrentBranchCompletion`,
  `-run TestServeStdioTicketsClose` (new merge-nudge tests + existing
  unresolved-phase test), `-run TestResolveImplementAheadOfMergeRoot` (Phase 1
  pinned tests, must stay green — this phase reuses, not changes, that
  observation).
- `go vet ./internal/mcp/...`
- Manual read-check: confirm the goal-drain skill docs
  (`agents-plugin/skills/lead-drain-ready-queue/SKILL.md`,
  `agents-plugin/rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md`) still
  read consistently with the new default (no code change expected there; this
  is a doc-drift check, not a test).

## Escalations

- None.
