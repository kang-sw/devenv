# Plan: 260725-bug-sage-stamp-swallows-unrelated-ticket-edits — Phase 1: make sage_stamp stage-only, and commit explicitly in the playbook

## Relevant Ticket Contract

- Remove the `wsgit.NewClient().Commit(...)` call from the `tickets.sage_stamp`
  MCP dispatch. Do **not** stage the posture write in its place: `wsgit.Commit`
  already stages its own `paths`, so a pre-stage buys nothing and creates a new
  failure mode (a leftover staged ticket file breaks the next unrelated
  `ws/git.commit` via `validateCommitStatus`'s "refusing to commit unrelated
  staged path"). Match `tickets.create_empty`: write, return, leave the tree
  alone.
- Drop the commit ref from `sage_stamp`'s response and its schema description.
  Update `next_instruction` in **both** verdict branches (`concern` and the
  default/pass branch) so neither claims a commit happened or routes the lead
  past one — this string is the lead's actual control surface per the
  playbook's "follow its returned next_instruction" step.
- `SageRecordResult`'s `CommitTitle`/`CommitPaths`/`AIContext` become unused;
  remove them rather than leaving an unreferenced commit-shaped struct behind.
- Decide `tickets.sage_gate`'s ask-decline commit explicitly rather than by
  omission (it does not swallow anything today — it runs before any reviewer
  output exists — so the swallow criterion does not force a change; ticket
  recommends aligning it with `sage_stamp` for one shared no-commit
  convention, but permits leaving it committing if the ticket records why).
- Update `lead-write-ticket`'s Sage Review Gate step with the one thing not
  derivable from the rest of the fix: an explicit commit after stamping, so
  the posture change lands with whatever else the lead is holding, under real
  `## AI Context`.
- Explicitly out of scope (ticket-stated): do not add a playbook step
  instructing the lead to apply reviewer findings to the ticket body — the
  `resolution: autonomous|missing` disposition contract already carries that
  data per-issue, and a step would be pure restatement.
- Spec closeout: `ai-docs/spec/mcp-tools.md` anchor
  `{#260720-sage-gate-record-tools}` — drop `sage_stamp`'s commit/commit-ref
  contract, and correct the pre-existing drift where the anchor claims a
  config-fallback resolution also commits (it does not — `resolveConcretePosture`
  only writes the field, never a commit). Also touch
  `ai-docs/spec/workflow-skills.md`'s Sage Review Gate mention if it describes
  commit behavior (survey found it currently does not — see Codebase Findings).
- Verification boundary: `sage_stamp` produces no commit and leaves the
  working tree otherwise untouched (no staging); a subsequent caller
  `ws/git.commit` carries the posture change plus whatever body edits the lead
  had uncommitted, under caller-supplied `## AI Context`; the tool's response,
  schema description, and `next_instruction` in both verdict branches no
  longer advertise or route past a commit; the playbook's Sage Review Gate
  step issues an explicit commit after stamping; `sage_gate`'s ask-decline
  path matches whichever outcome this plan settles on, with rationale on
  record if it still commits.

## Out of Scope

- Applying reviewer findings to the ticket body from the playbook — ticket
  explicitly rejects adding this step (the per-issue `resolution:` field
  already carries the disposition contract at finer granularity).
- Any refactor of `SageGate`'s internal ask-decline commit-computation
  machinery (`stageOutcome.commitTitle/commitPaths/aiContext`,
  `mergeGateCommit`, `gateResultFromStage`) beyond what this plan's Decision
  below requires — see Codebase Findings for why a full "align sage_gate to
  stage-only" refactor is a materially larger, differently-shaped change than
  ticket text assumes.
- `260725-idea-ws-git-commit-rename-and-payload-rejections` — already landed
  (`.done/`); no action needed here beyond the ticket's own note that an
  un-reinstalled binary needs the native `git` fallback for a staged status
  transition until the plugin is reinstalled.
- Bulk-editing historic `sage_stamp`-produced commits (`b9c72975`, `27b3b599`,
  etc.) — history is not rewritten by this fix.
- Any change to `tickets.move`/`tickets.close`/`tickets.create_empty` — cited
  only as the existing stage-only/no-commit precedent pattern to match.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/server.go:1410-1441` (`tickets.sage_stamp`
  dispatch case) — the actual commit call is at **lines 1433-1440**
  (`wsgit.NewClient().Commit(context.Background(), root, wsgit.CommitOptions{Paths: result.CommitPaths, Title: result.CommitTitle, AIContext: result.AIContext})`),
  followed by `return toolTextResponse(req.ID, formatSageRecord(result, commitRes.Hash), nil)`
  at line 1441. **Line drift from the ticket**: the ticket cites line 1398 for
  this call, but line 1398 today falls inside the *`sage_gate`* case's own
  commit block (`server.go:1397-1408`, the ask-decline commit path) — confirm
  by case label, not raw line number, when editing.
- `agents-plugin-tool/internal/mcp/server.go:1342-1364` (`tickets.create_empty`
  dispatch) — the write-return-no-touch pattern to match: calls `wsdoc.TicketCreate`,
  returns `formatTicketCreate(result)` directly, no staging, no commit.
- `agents-plugin-tool/internal/mcp/server.go:2786-2806` (`formatSageRecord`) —
  takes `(result wsdoc.SageRecordResult, commitHash string)`; the `if commitHash != "" { fmt.Fprintf(&b, "commit: %s (%s)\n", commitHash, result.CommitTitle) }`
  block is lines 2797-2799; the two `next_instruction` branches are lines
  2800-2804 (`concern` → line 2801 "Recorded as completed; ... escalate to
  block manually ..."; default/pass → line 2803 "Sage review recorded and
  committed; follow this confirmation and proceed to handoff."). Both must
  drop the "committed" claim and instead direct the lead to `ws/git.commit`.
- `agents-plugin-tool/internal/mcp/server.go:4147-4158` — `tickets.sage_stamp`
  tool schema; its `description` (line 4148) says "...commits with the
  canonical title, and returns the applied posture and commit ref." — must be
  rewritten to describe a write-only, no-commit contract.
- `agents-plugin-tool/internal/wsdoc/tickets_sage.go:70-79` — `SageRecordResult`
  struct: `CommitTitle`, `CommitPaths`, `AIContext` (lines 76-78) to remove,
  plus the doc comment (lines 70-71, "The MCP layer commits CommitPaths under
  CommitTitle/AIContext.") to rewrite.
- `agents-plugin-tool/internal/wsdoc/tickets_sage.go` — these fields are
  **populated internally**, not just passed through, so removal touches:
  - `sageRecordSingle` (lines 384-424): `res.CommitTitle`/`res.AIContext`
    assignments at lines 397 (init via struct literal, `CommitPaths` only —
    fine to keep, see below), 411-412 (block case), 421-422 (pass case).
  - `sageRecordCombined` (lines 428-479): assignments at lines 450 (struct
    literal `CommitPaths`), 466-467 (block case), 476-477 (pass case).
  - `CommitPaths` is still meaningful data (`[]string{ticketRel}` — "which file
    was written") even after commit removal, but the ticket's instruction is
    unqualified ("remove them"); since nothing will consume `CommitPaths`
    once no commit happens and the caller already knows which ticket it
    passed in, remove all three fields together for consistency with the
    "unreferenced commit-shaped struct" rationale — do not selectively keep
    `CommitPaths`.
- `agents-plugin-tool/internal/wsgit/git.go:552` (`stagingCommandsForCommit`)
  and `git.go:639-651` (`validateCommitStatus`, the "refusing to commit
  unrelated staged path" error at line 649) — confirms the ticket's
  pre-staging-hazard reasoning (drift from ticket's cited 528/615, same
  functions). No change needed here; cited only as evidence for why "do not
  pre-stage" is correct.
- **`tickets.sage_gate`'s decision — the sage_gate side is NOT a simple mirror
  of the sage_stamp fix; recommend leaving it committing, with rationale
  recorded, rather than aligning it in this phase:**
  - `agents-plugin-tool/internal/wsdoc/tickets_sage.go:34-46` (`SageGateResult`)
    and `:81-89` (`stageOutcome`) carry the same-shaped
    `CommitTitle/CommitPaths/AIContext` fields, but they are woven through
    `resolveStage` (lines 176-210, decline branch 190-199),
    `gateResultFromStage` (lines 221-232), `sageGateCombined`'s two independent
    decline branches (lines 262-289 design-decline, 307-320
    completeness-decline), and `mergeGateCommit` (lines 335-351) — a
    materially larger internal surface than `sage_stamp`'s single dispatch-site
    commit call.
  - Removing this machinery would also require a **new, ticket-unspecified
    design decision**: `SageGateResult.Action == "skip"` is returned both for
    an ordinary skip (nothing to commit — landing exempt, category exempt,
    posture already terminal) and for an ask-decline (posture just written to
    `skipped`, a commit is now pending). Today `CommitTitle != ""` is exactly
    what `sageGateNextInstruction` (`server.go:2761-2774`) and
    `formatSageGate` (`server.go:2742-2759`) use to know a commit occurred and
    show it — removing the field with no replacement would silently drop the
    lead's only signal that an ask-decline write needs a caller commit, with
    no ticket-specified replacement signal.
  - Test surface tied to the current committing behavior:
    `agents-plugin-tool/internal/wsdoc/tickets_sage_test.go:266-268`
    (`TestSageGateDeclinePersistsSkipped`), `:489-491`/`:515-517`/`:571-573`
    (three decline branches inside `TestSageGateCombinedSeparateAsks`), and
    `:598-630` (`TestMergeGateCommitDualDecline`, a dedicated test for the
    merge machinery) — none of these are called out or updated by the ticket
    text, whereas the ticket is explicit about exactly which `sage_stamp`
    tests/lines need touching.
  - **Decision for this plan: leave `tickets.sage_gate`'s ask-decline commit
    as-is (no source change to `server.go`'s `sage_gate` case or to
    `SageGateResult`/`stageOutcome`/`mergeGateCommit`)**, and record the
    rationale in the ticket per its own permitted fallback ("if it is left
    committing, record why in the ticket"). This keeps Phase 1's actual diff
    matching what the ticket's Phase 1 prose details in full, and defers the
    open next_instruction-signaling design question rather than inventing an
    unspecified mechanism mid-phase.
- `ai-docs/spec/mcp-tools.md:1044-1073` (anchor
  `{#260720-sage-gate-record-tools}`) — lines 1054-1055 currently read "A
  declined `ask` and a config-fallback resolution each persist the resolved
  posture and commit." This is the drift the ticket names: `resolveConcretePosture`
  (`tickets_sage.go:162-171`) only calls `writeFrontmatterField`, never
  commits. Split this sentence: decline commits (kept, per the Decision
  above), config-fallback does not. Lines 1057-1064 describe `sage_stamp` as
  committing and returning a commit reference — rewrite to a write-only, no-commit
  contract, matching the `tickets.move`/`tickets.close` no-commit phrasing
  style already used earlier in this same document.
- `ai-docs/spec/workflow-skills.md:580-587` (anchor
  `{#260524-design-verification-skill}`, the only "Sage Review Gate" mention
  found) — describes the gate at a conceptual level only ("dispatches
  `ticket-reviewer-design` automatically at `todo/`→`ready/` promotion");
  makes no claim about commit behavior. No edit is strictly required here;
  confirm on contact that no other passage in this file describes `sage_stamp`
  as committing before treating this as a no-op.
- `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md:66-69`
  (`### 6. Sage Review Gate`) — exactly two numbered steps today: (1) call
  `sage_gate` and follow `next_instruction`; (2) call `sage_stamp` and follow
  `next_instruction`. Step `5. Commit` (lines 61-64) is the existing sibling
  pattern for an explicit `{{.McpNamespace}}/git.commit(paths, title, ai_context)`
  call — reuse its phrasing for the new step 3 added here.
- `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go:47-81`
  (`TestWsflowRsrcMirrorUpToDate`) and `:83-97`
  (`TestRegenerateWsflowRsrcMirror`, gated by `WS_REGEN_WSFLOW_RSRC=1`) — the
  byte-identical canonical-vs-mirror guard and its regen entry point; edit
  only the canonical `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`,
  never `agents-plugin-wsflow/rsrc/...` directly.
- `agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go:24`
  (`TestShippedManifestUpToDate`) and `:93` (`TestRegenerateShippedManifest`,
  gated by `WS_REGEN_MANIFEST=1`) — editing the playbook body changes its
  content hash, so the manifest also needs regeneration.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go:2011-2057`
  (`TestPlaybookPrintGoldenLeadWriteTicket`) — asserts (via `strings.Contains`,
  not exact structure) that the rendered playbook still contains
  `"tickets.sage_gate(stem, landing)"`, `"tickets.sage_stamp(stem, stage, verdicts)"`,
  and `"follow its returned next_instruction"`; adding a third numbered step
  should not break these substring checks, but re-run this test after editing
  the playbook to confirm.
- **Test impact of the `SageRecordResult` field removal** —
  `agents-plugin-tool/internal/wsdoc/tickets_sage_test.go` has six `SageRecord`-path
  assertions on the now-removed `CommitTitle` field that must be updated (drop
  the `CommitTitle` check, keep the surrounding posture/`BlockedSection`
  assertions which are unaffected): lines 303-305 (`TestSageRecordDesignStandalone`,
  block case), 327-328 (same test, pass case), 347-348
  (`TestSageRecordCompletenessStandalone`), 373-375, 410-412, 427-428 (three
  cases inside `TestSageRecordCombinedAggregation`). (Line 266-268 is a
  `SageGate` test, `TestSageGateDeclinePersistsSkipped` — unaffected by this
  plan's Decision to leave `sage_gate` unchanged.)
- `agents-plugin-tool/internal/mcp/tickets_sage_test.go`:
  - `TestFormatSageRecordRoundTrip` (lines 69-91) — both `formatSageRecord(...)`
    calls pass a `wsdoc.SageRecordResult{... CommitTitle: "..."}` literal
    (fields 74, 86) and a trailing `commitHash` argument (`"deadbeef"`, `"cafe"`
    at lines 75, 87); asserts `"commit: deadbeef"` (line 76). Needs: drop the
    `CommitTitle` struct field (no longer exists), drop the `commitHash`
    parameter from the call (signature changes to `formatSageRecord(result wsdoc.SageRecordResult) string`),
    drop the `"commit: ..."` substring assertion, and update the wanted
    substrings to match the new no-commit `next_instruction` wording.
  - `TestServeStdioSageStampDispatch` (lines 114-147) — asserts
    `strings.Contains(resp, "commit:")` at line 133; must invert to assert no
    `"commit:"` substring, and add a positive check that no commit landed
    (e.g. `runGitOutput(t, root, "log", "--oneline")` is empty, or
    `git status --porcelain` shows the ticket path modified-but-unstaged,
    matching "no commit and no staging" from the verification boundary). Test
    helpers `runGit`/`runGitOutput` already exist at
    `agents-plugin-tool/internal/mcp/server_test.go:934-943`.
  - `TestFormatSageGateRoundTrip` (lines 48-67) — unaffected by this plan's
    Decision (sage_gate keeps committing); no change needed.

## Implementation Plan

1. **`agents-plugin-tool/internal/mcp/server.go`, `tickets.sage_stamp` case
   (currently lines 1410-1441)**: delete the `wsgit.NewClient().Commit(...)`
   block (lines 1433-1440) entirely; change the final line to
   `return toolTextResponse(req.ID, formatSageRecord(result), nil)`.
2. **Same file, `formatSageRecord`** (lines 2786-2806): drop the `commitHash string`
   parameter; delete the `if commitHash != "" { ... }` block (lines 2797-2799);
   rewrite both `next_instruction` strings (lines 2800-2804) to state the
   posture was recorded but **not** committed and to route the lead to its own
   `ws/git.commit` (e.g. concern branch: "Recorded but not committed; commit
   this posture change via ws/git.commit before proceeding. The concern with a
   missing decision is surfaced — escalate to block manually only if the
   missing decision is judged critical." / default branch: "Sage review
   posture recorded but not committed; commit this change via ws/git.commit,
   then proceed to handoff.").
3. **Same file, `tickets.sage_stamp` schema `description`** (line 4148):
   remove "commits with the canonical title, and returns the applied posture
   and commit ref"; replace with a write-only, no-commit description
   consistent with `tickets.create_empty`'s "does not stage or commit"
   phrasing (line 4100).
4. **`agents-plugin-tool/internal/wsdoc/tickets_sage.go`**: remove
   `CommitTitle`/`CommitPaths`/`AIContext` from `SageRecordResult` (lines
   72-79) and rewrite its doc comment (lines 70-71). Remove the corresponding
   assignments in `sageRecordSingle` (lines 397, 411-412, 421-422) and
   `sageRecordCombined` (lines 450, 466-467, 476-477), keeping every other
   field (`Verdict`, `Posture`, `BlockedSection`) and all frontmatter-write/
   Blocked-section-render behavior unchanged. Leave `SageGateResult`,
   `stageOutcome`, `mergeGateCommit`, `resolveStage`, `sageGateCombined`
   untouched (see Decision in Codebase Findings).
5. **Record the `sage_gate` decision in the ticket**: append a short rationale
   sentence to `ai-docs/tickets/ready/260725-bug-sage-stamp-swallows-unrelated-ticket-edits.md`'s
   existing "Decide `tickets.sage_gate`'s ask-decline commit explicitly..."
   paragraph (Phase 1 section), stating that alignment is deferred because it
   requires a new, unspecified `next_instruction`/`SageGateResult` signaling
   mechanism to distinguish an ask-decline write (needs a caller commit) from
   an ordinary skip (nothing to commit) — not a same-shape mirror of the
   `sage_stamp` fix. No Phase 1 `### Result` section exists yet, so this is a
   direct edit, not a post-Result `#### Edition` addendum.
6. **`ai-docs/spec/mcp-tools.md`, anchor `{#260720-sage-gate-record-tools}`
   (lines 1044-1073)**: split line 1054-1055's "A declined `ask` and a
   config-fallback resolution each persist the resolved posture and commit"
   into two accurate sentences (decline commits; config-fallback only
   persists, no commit). Rewrite lines 1057-1064's `sage_stamp` contract to
   drop "commits with the canonical title" / "the commit reference", stating
   instead that it writes the frontmatter field(s) and any `## Blocked`
   section and returns the applied posture only — the caller commits via its
   own `ws/git.commit` with caller-supplied `## AI Context`.
7. **`ai-docs/spec/workflow-skills.md`**: re-check anchor `{#260524-design-verification-skill}`
   (lines 580-587) and the rest of the file for any prose asserting
   `sage_stamp` commits; survey found none, so this step may be a no-op —
   confirm and skip if still true at implementation time.
8. **`agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`, `### 6. Sage
   Review Gate` (lines 66-69)**: add a new numbered step 3 after the existing
   two, instructing an explicit commit after stamping — reuse step 5's
   `{{.McpNamespace}}/git.commit(paths: [...], title: "...", ai_context: [...])`
   phrasing, directing the lead to carry the posture change together with any
   other uncommitted edits it is holding, under its own rationale. Do **not**
   add a step instructing the lead to apply reviewer findings (explicitly out
   of scope per the ticket).
9. **Regenerate the wsflow mirror and manifest** after step 8: run
   `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`
   then `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
   from `agents-plugin-tool/` (`-count=1` mandatory for both). Never hand-edit
   `agents-plugin-wsflow/rsrc/...` directly.
10. **Update tests** per the Codebase Findings test-impact notes above:
    - `agents-plugin-tool/internal/wsdoc/tickets_sage_test.go`: drop the six
      `CommitTitle` assertions on `SageRecord`-path results (lines 303-305,
      327-328, 347-348, 373-375, 410-412, 427-428); leave every `SageGate`-path
      `CommitTitle` assertion untouched.
    - `agents-plugin-tool/internal/mcp/tickets_sage_test.go`:
      `TestFormatSageRecordRoundTrip` — drop `CommitTitle` from both
      `wsdoc.SageRecordResult{...}` literals, drop the `commitHash` call
      argument, drop the `"commit: ..."` substring assertions, assert the new
      no-commit `next_instruction` wording instead.
      `TestServeStdioSageStampDispatch` — invert the `"commit:"` substring
      check to assert its absence; add a check that no new commit exists
      (`runGitOutput(t, root, "log", "--oneline")` before/after comparison, or
      equivalent) and that the ticket file is left modified-but-unstaged
      (`git status --porcelain` via `runGitOutput`) after the dispatch call.
11. **Version bump**: per `AGENTS.md`'s dev-merge rule, bump the plugin patch
    version via `agents-plugin-tool/scripts/bump-ws-version.sh <X.Y.Z>` when
    this lands on an integration branch or `main` — not part of this
    implementation commit itself unless that merge happens in the same step.

## Verification Plan

- `cd agents-plugin-tool && go build ./...` — confirms no dangling references
  to the removed `SageRecordResult` fields or the old `formatSageRecord`
  signature.
- `cd agents-plugin-tool && go test ./internal/wsdoc/... -run TestSageRecord -v`
  — updated `SageRecord`-path assertions pass; posture/`BlockedSection`
  behavior is unchanged.
- `cd agents-plugin-tool && go test ./internal/wsdoc/... -run TestSageGate -v`
  and `-run TestMergeGateCommitDualDecline` — confirm the untouched
  `sage_gate` decline-commit behavior still passes as-is.
- `cd agents-plugin-tool && go test ./internal/mcp/... -run TestServeStdioSageStamp -v`
  and `-run TestFormatSageRecordRoundTrip -v` and `-run TestServeStdioSageGateDispatch -v`
  — confirm `sage_stamp` produces no commit and no staging, `sage_gate`'s
  decline-commit path is unaffected, and response/`next_instruction` text
  matches the new no-commit contract.
- `cd agents-plugin-tool && go test ./internal/mcp/... -run TestPlaybookPrintGoldenLeadWriteTicket -v`
  — confirms the rendered playbook still contains the expected sage-gate
  call-site substrings after adding the new commit step.
- `cd agents-plugin-tool && go test ./internal/wsrsrc/... -run TestWsflowRsrcMirrorUpToDate -v`
  and `-run TestShippedManifestUpToDate -v` — must fail before regeneration
  (drift detected) and pass after running the two `WS_REGEN_*` commands from
  step 9.
- Manual/scenario check: run a real `sage_stamp` call against a ticket with
  uncommitted body edits already present; confirm `git status` shows the
  ticket still modified-and-unstaged afterward (no commit, no staging), then
  confirm a follow-up `ws/git.commit` on that path succeeds and carries both
  the posture change and the pre-existing body edits under one commit.

## Escalations

- None.
