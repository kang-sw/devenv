# Plan: 260810-feat-idea-ticket-attention-policy — Phase 1: Bring idea/ into worktree topic-scope with capture preserved

## Relevant Ticket Contract

- `lead-scope-worktree` playbook (canonical + wsflow mirror): remove the "idea/
  always stays visible" carve-out; the derived pattern additionally excludes
  `/ai-docs/tickets/idea/*` and re-includes a tracked
  `/ai-docs/tickets/idea/.gitkeep`; extend verify-by-listing and the
  widen-then-move remedy to idea/.
- Forced tracked `ai-docs/tickets/idea/.gitkeep` (create+commit if absent)
  before applying a pattern that scopes idea/ out.
- Staging fix locus is `ws/git.commit` (not `TicketCreate`/`tickets.create_empty`,
  which keep their "does not stage or commit" contract untouched): under an
  active sparse-checkout, `git.commit` stages its explicit caller-named `paths`
  with `git add --sparse` so an off-topic idea capture is committable and then
  self-hides (skip-worktree).
- Guardrail (binding, non-negotiable): the `--sparse` staging must never stage
  a deletion of an absent skip-worktree path — distinguish
  absent-because-sparse-hidden (skip) from absent-because-genuinely-deleted
  (stage the deletion, per existing `#260513` behavior).
- Do not auto-widen the sparse pattern on capture. Do not relax staging for
  non-idea statuses. Do not touch `tickets_mutate.go`'s existing cross-scope
  refusals (ready/todo moves, idea→todo promotion widen-then-retry) — those
  already work status-agnostically today (see Codebase Findings) and need no
  code change, only playbook wording.
- `scope_announcement.go`: add `idea` to the counted status list (currently
  `["ready","todo"]`), update banner text to include idea/, append a one-line
  pointer that `git sparse-checkout list` reveals the active topic.
- Spec Impact (Phase 1 surfaces): three `spec/mcp-tools.md` amendments
  (workflow_manual scope-announcement block, `git.commit` staging, `project_tree`
  ticket inventory — the last is Phase 2, out of scope here) plus one
  `spec/workflow-skills.md` amendment (`lead-scope-worktree`). Phase 1 only
  touches the workflow_manual-announcement and git.commit-staging amendments,
  plus the workflow-skills amendment.
- Verification (5 steps, ticket text): `ls idea/` shows only `.gitkeep`;
  `tickets.create_empty(initial_state:"idea")` + `git.commit` on that path
  stages/commits via `--sparse` with no manual widen; the committed idea file
  self-hides after a fresh sparse re-apply yet is present in the index/other
  worktrees; `workflow_manual` reports hidden idea count + the
  `git sparse-checkout list` pointer; a cross-scope ready/todo move still
  refuses with the widen tip.

## Out of Scope

- Phase 2 (`project_tree` orphan-idea fold in `internal/wsdoc/project_tree.go`
  `renderTickets`) — separate later cycle, per task instructions.
- Any change to `tickets_mutate.go`'s `scopeBlockedMoveError` refusal paths —
  confirmed already status-agnostic (works for idea/ today with zero code
  change).
- Any change to `TicketCreate`/`tickets.create_empty` — stays stage-free by
  contract.
- The active-glob display in the `workflow_manual` banner (ticket explicitly
  defers this; banner carries only the `git sparse-checkout list` pointer).
- `git rm --cached` staging path — analysis below shows `--sparse` should be
  scoped to the `add` command only (see Implementation Plan step 4).

## Codebase Findings

- `agents-plugin/rsrc/lead-scope-worktree/lead-scope-worktree.md#L9-L39` —
  canonical playbook. Invariant at `#L13` is the carve-out to delete
  ("idea/ always stays visible..."). `On: invoke` steps 2 (`#L21-L25`, pattern
  derivation), 4 (`#L27`, verify-by-listing), 5 (`#L28`, remedy explanation)
  all need idea/ added.
- `agents-plugin-wsflow/rsrc/lead-scope-worktree/lead-scope-worktree.md` —
  byte-identical mirror (confirmed by direct read: identical content to
  canonical). Per the rsrc "generated-sameness carve-out"
  (`ai-docs/manuals/wsflow-mirroring.md` "Rsrc Tree Provisioning"), edit only
  the canonical copy, then regenerate — never hand-edit the wsflow copy.
- `agents-plugin/skills/lead-scope-worktree/SKILL.md#L3` and
  `agents-plugin-wsflow/skills/lead-scope-worktree/SKILL.md#L3` — both
  frontmatter `description:` fields say "...while idea/ stays visible", the
  same claim being removed from the playbook body. Both need the phrase
  dropped/updated; these are curated (not generated) shim files, edited by
  hand in both trees.
- `ai-docs/manuals/wsflow-mirroring.md` "Shipped wsflow Skills" enumeration
  (`Included:` list) omits `lead-scope-worktree` entirely, yet
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py:38,74` has it in
  `EXPECTED_SKILLS`/title map and the wsflow rsrc/skill files exist on disk —
  the doc's roster is stale (drift, not a Phase-1 blocker; AGENTS.md says
  "update drifted docs on contact" but this list is descriptive prose, not a
  gate). Note it in the executor's PR but do not block on it; not part of this
  ticket's contract.
- **Regen commands** (`ai-docs/manuals/wsflow-mirroring.md` "Rsrc Tree
  Provisioning" + "Static Verification"): after editing the canonical
  playbook, run in order:
  ```bash
  WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest
  WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
  python3 -m unittest discover agents-plugin-wsflow/tests
  ```
  (`SKILL.md` frontmatter descriptions are hand-edited in both trees, not
  regenerated — `lead-scope-worktree` is not in the "Substitution-Mirrored
  Skill Generation" bounded list, so its `SKILL.md` is curated, not generated.)
- `ai-docs/ref/worktree-ticket-scope.md#L14-L21,#L48-L76` — the widen-then-move
  remedy this phase extends to idea→todo triage; the ticket said this file
  stayed under `ref/` (not moved to `manuals/`), confirmed present at that
  path. `#L16-L21` states the *current* "idea/ always visible" rationale that
  Phase 1 supersedes — this reference file's "What This Covers" section text
  should be checked/updated for consistency in the same change (not explicitly
  named in ticket Spec Impact, but leaving it stating the removed rationale
  would be a stale doc left uncorrected on contact).
- `agents-plugin-tool/internal/wsgit/git.go#L568-L596` (`stagingCommandsForCommit`)
  — the `#260513-git-commit-result-edition-detection` staging locus (confirmed
  by mental-model citation at `ai-docs/mental-model/git-workflow-tools.md#L33`).
  Classifies each caller path into `addPaths` (staged `git add -A -- <paths>`)
  or `rmPaths` (staged `git rm --cached --ignore-unmatch -- <paths>`, only when
  `deletedPathsUnderCommitRoot` found a real `D`/rename-old status for that root
  **and** `commitRootHasAddableStatus` is false). This is the single function
  to extend for `--sparse`.
- `agents-plugin-tool/internal/wsgit/git.go#L598-L619` (`deletedPathsUnderCommitRoot`)
  and `#L621-L636` (`commitRootHasAddableStatus`) — both read only from the
  **pre-staging** `git status --porcelain=v2` snapshot (`Client.Commit` line
  467-471, before any staging command runs). Empirically verified (git 2.43,
  throwaway repo): a tracked, skip-worktree, on-disk-absent, *unmodified* path
  **never appears** in `git status --porcelain=v2` output at all — skip-worktree
  suppresses worktree-vs-index comparison entirely. So `deletedPathsUnderCommitRoot`
  can never route a merely-hidden (not actually deleted) path into `rmPaths`
  today, with or without `--sparse` involved — the existing #260513 logic is
  already immune to the sparse-hidden-vs-deleted confusion on the read side.
- `agents-plugin-tool/internal/wsgit/git.go#L461-L522` (`Client.Commit`) — call
  site: line 472 expands ticket-move paths, line 473 iterates
  `stagingCommandsForCommit(opts.Paths, preStatus)`. `opts` is `CommitOptions`
  (`#L429-L438`), which has no scope-awareness field today — needs a new field
  (see Implementation Plan step 2).
  Verified experiment risk finding: `git add -A --sparse -- <path>` on an
  **already-tracked, skip-worktree, on-disk-absent, unmodified** path fails
  loudly with `fatal: pathspec '<path>' did not match any files` (exit 128) —
  it does **not** silently stage a deletion. Batching such a path together with
  a genuine new capture in one `git add -A --sparse -- <p1> <p2>` call aborts
  the *whole* command (exit 128, nothing staged, not even the good path) — a
  safe-fail, not silent corruption, but a real caller-visible failure mode if
  the executor batches unrelated paths. `git add -A --sparse -- <new-untracked-path>`
  (a genuine new capture under a hidden dir) succeeds (exit 0), stages `A.`,
  and — confirmed via `git sparse-checkout reapply` — self-hides (returns to
  skip-worktree, vanishes from disk) only on the **next** sparse re-apply, not
  immediately after commit; `git show HEAD:<path>` and other worktrees see it
  throughout. This matches the ticket's Decision text exactly.
- `agents-plugin-tool/internal/wsdoc/tickets_scope.go#L58-L82` (`newTicketScope`)
  — the existing cheapest-first "is a sparse-checkout scope active" gate
  (pattern-file stat, then config-file agreement, then `git config --get
  core.sparseCheckout` only when the files leave it open). `#L423-L449`
  (exported `TicketScope(root, statuses)`) already builds on this gate and is
  the function `scope_announcement.go` and `project_tree`'s
  `ticketScopeAnnotation` both call today. Reusable for the git.commit-side
  "is scope active" check — see Implementation Plan step 2 for the two
  concrete options (reuse `TicketScope` vs. add a lighter dedicated export).
  `#L326` comment: "wsdoc must not import wsgit" — confirmed one-directional;
  nothing bans `internal/mcp` (which already imports both) from bridging, and
  nothing bans a plain bool field being computed by `internal/mcp` and passed
  into `wsgit.CommitOptions` — this mirrors the existing `Verifier` /
  `verifyAdapter` injection shape already used for the commit-time guardrail
  gate (`ai-docs/mental-model/git-workflow-tools.md#L29`, `{#260720-wsdoc-commit-boundary}`).
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L100,#L122,#L180,#L200,#L758`
  — `scopeBlockedMoveError` call sites operate on `newTicketScope`/`.includes(relPath)`
  generically for any ticket path, not filtered by status. Confirms idea→todo
  widen-then-retry already works mechanically today with **zero code change**;
  only playbook prose needs to state it governs idea/ too.
- `agents-plugin-tool/internal/wsdoc/ticket_create.go#L23,#L103` — `TicketCreate`
  writes via plain `os.WriteFile`, no git call, confirming the "does not stage
  or commit" contract the ticket says stays untouched.
- `agents-plugin-tool/internal/mcp/scope_announcement.go#L19-L37` — the whole
  function. `#L20`: `wsdoc.TicketScope(root, []string{"ready", "todo"})` is the
  one-line change target (add `"idea"`). `#L26-L35`: banner text
  ("ai-docs/tickets/ready/ and ai-docs/tickets/todo/...") needs idea/ added in
  both the zero-hidden and hidden-N branches, plus a new one-line pointer
  sentence for `git sparse-checkout list` (append after `#L35`'s existing
  "See ai-docs/ref/worktree-ticket-scope.md..." sentence, or fold into it).
  Existing asymmetry confirmed: `server.go#L1111`
  (`ticketScopeAnnotation(root, []string{"ready", "todo", "idea"})`, the
  `project_tree` case) already includes idea — only `scopeAnnouncement`
  (workflow_manual path) is stuck on 2 statuses.
- `agents-plugin-tool/internal/mcp/scope_announcement_test.go` (full file) —
  existing test patterns to mirror: `enableSparseCheckout` (`#L12-L21`, the
  exact `git sparse-checkout set --no-cone` shape `lead-scope-worktree`
  derives — needs an idea/ exclude+reinclude added once the playbook changes),
  `mustWriteAndCommitTicket` (`#L25-L30`, helper reused for idea fixtures),
  `TestScopeAnnouncementFiresOnWorkflowManual` (`#L35-L70`),
  `TestWorkflowManualScopeAnnouncementByteUnchangedWhenUnscoped` (`#L78-L100`),
  `TestScopeAnnouncementFiresWithNoTicketHidden` (`#L106-L126`). All three
  need either updates (to also cover a hidden idea stem + assert the new
  `git sparse-checkout list` pointer text) or a new sibling test.
- `agents-plugin-tool/internal/wsgit/git_test.go` — existing `stagingCommandsForCommit`-
  adjacent tests to mirror for the new `--sparse` branch:
  `TestCommitStagesExplicitPathsAndBuildsMessage` (`#L225`),
  `TestCommitStagesDeletedTicketMoveByParentDirectory` (`#L439`),
  `TestCommitStagesDeletedTodoToReadyTicketMove` (`#L451`),
  `TestCommitStagesDeletedTicketMoveWhenOldStatusDirectoryIsGone` (`#L463`),
  `TestCommitStagesRenamedDirectoryWithoutAddingMissingOldRoot` (`#L475`),
  `TestCommitStagesDeletedDirectoryRootByConcreteChildren` (`#L487`),
  `TestCommitStillAddsRootWithLiveChangesAndDeletedChildren` (`#L498`). None of
  these exercise sparse-checkout; all should stay green unmodified (byte-identical
  staging when the new scope-active field defaults false/zero-value), proving
  the change is additive-only for the unscoped path.
- `ai-docs/spec/mcp-tools.md#L523-L532` (`{#260626-workflow-manual-restoration-entry}`)
  — exact spec prose to amend: "a short block naming the hidden ticket count and
  stems under `ai-docs/tickets/ready/` and `ai-docs/tickets/todo/`" needs idea/
  added, plus the new `git sparse-checkout list` pointer line.
  `ai-docs/spec/mcp-tools.md#L1535` area (`{#260513-git-commit-result-edition-detection}`)
  — exact anchor to amend with the new `--sparse` staging behavior under active
  scope, per the ticket's Spec Impact bullet.
  `ai-docs/spec/workflow-skills.md#L36,#L69-L72` — exact `lead-scope-worktree`
  spec prose ("idea/ always stays visible" carve-out in the roster description
  at `#L69-L72`) to amend per the ticket's fourth Spec Impact bullet.

## Implementation Plan

1. **Canonical playbook edit** —
   `agents-plugin/rsrc/lead-scope-worktree/lead-scope-worktree.md`:
   - `#L13` Invariant: replace the "Scope covers `ready/` and `todo/` only;
     `idea/` always stays visible..." bullet with one stating scope now covers
     `ready/`, `todo/`, and `idea/` uniformly, and that a forced tracked
     `/ai-docs/tickets/idea/.gitkeep` keeps the directory materialized when
     idea/ is scoped out (cite the git-2.43-verified directory-vanishes-when-
     empty behavior already documented in `ai-docs/ref/worktree-ticket-scope.md`
     Decisions).
   - `On: invoke` step 2 (Derive, `#L21-L25`): extend the base pattern to
     `!/ai-docs/tickets/idea/*` plus a forced re-include
     `/ai-docs/tickets/idea/.gitkeep`, and extend `<topic re-includes>` guidance
     to optionally include `/ai-docs/tickets/idea/<topic-glob>*`.
   - New step before Apply (or folded into Apply): ensure
     `ai-docs/tickets/idea/.gitkeep` exists as a tracked, committed file before
     applying a pattern that excludes idea/* — create + commit it if absent
     (this is a playbook prose step, not a Go code change; use
     `{{.McpNamespace}}/git.commit` for the commit per skill-authoring Layer 1
     conventions, not a raw `git commit`).
   - Step 4 (Verify by listing, `#L27`): extend to also list
     `ai-docs/tickets/idea/` and report only `.gitkeep` remains (plus any
     topic-matched idea re-includes).
   - Step 5 (Explain remedies, `#L28`): extend the widen-then-move remedy
     explanation to name idea→todo triage explicitly (cite
     `ai-docs/ref/worktree-ticket-scope.md` "Cross-Scope `git mv`" section,
     unchanged mechanism).
   - Apply the skill-authoring invariant checklist
     (`ai-docs/manuals/skill-authoring.md` "Invariant checklist") to every
     changed/added Invariants-section line: falsifiable, actionable, one line,
     context-free, non-redundant, doctrine-aligned.
2. **`ai-docs/ref/worktree-ticket-scope.md` consistency check** — the "What
   This Covers" paragraph (`#L14-L21`) currently states the idea/-always-visible
   rationale as present-tense fact; update it to reflect Phase 1's new scope
   (idea/ now participates, `.gitkeep` keeps the directory alive) so this
   reference file does not contradict the updated playbook. Not explicitly
   named in the ticket's Spec Impact list, but leaving it stale on contact
   conflicts with `AGENTS.md`'s "Update drifted docs on contact" rule.
3. **wsflow mirror regen** — do not hand-edit
   `agents-plugin-wsflow/rsrc/lead-scope-worktree/lead-scope-worktree.md`; after
   step 1, run the two regen commands from Codebase Findings
   (`TestGenerateRealManifest`, `TestRegenerateWsflowRsrcMirror`, both with
   `-count=1`), then run `python3 -m unittest discover agents-plugin-wsflow/tests`
   to confirm the wsflow package bundle stays green.
4. **`SKILL.md` shim descriptions** — hand-edit both
   `agents-plugin/skills/lead-scope-worktree/SKILL.md#L3` and
   `agents-plugin-wsflow/skills/lead-scope-worktree/SKILL.md#L3`: drop "while
   idea/ stays visible" from the `description:` frontmatter (these are curated
   files, not generated — no regen tool covers this text).
5. **`wsgit.CommitOptions` gains a scope-active signal** —
   `agents-plugin-tool/internal/wsgit/git.go#L429-L438`: add a field (e.g.
   `SparseScopeActive bool`) to `CommitOptions`. Recommendation (reversible,
   not ticket-mandated): a plain bool is simplest and matches the "Verifier
   injection" precedent's spirit without needing a full callback — the MCP
   dispatch layer computes it once per call via a cheap wsdoc-side gate before
   invoking `wsgit.Client.Commit`, exactly as it already computes
   `ai_context` debug-event fields inline (`server.go#L1066-L1071` pattern).
6. **Thread the flag into staging** —
   `agents-plugin-tool/internal/wsgit/git.go#L568-L596`
   (`stagingCommandsForCommit`): add a `sparseActive bool` parameter (or read
   it off a small struct if the signature is getting unwieldy); when true,
   change only the `add` branch (`#L589-L591`) to
   `append([]string{"add", "-A", "--sparse", "--"}, addPaths...)`. Leave the
   `rm --cached --ignore-unmatch` branch (`#L592-L594`) untouched — Codebase
   Findings confirms `deletedPathsUnderCommitRoot` only ever sees genuinely
   `D`/renamed paths from pre-staging `git status`, never a merely-hidden one,
   so `--sparse` is not needed there and adding it would be scope creep beyond
   the ticket's guardrail wording ("additions/updates of paths present on disk").
   Update the call site at `#L473` (`Client.Commit`) to pass
   `opts.SparseScopeActive`. When `sparseActive` is false (default, matches
   every existing caller/test), output is byte-identical to today — this is
   what keeps every listed existing `git_test.go` test green unmodified.
7. **Wire the MCP dispatch case** —
   `agents-plugin-tool/internal/mcp/server.go` `case "git.commit":`
   (`#L1032-L1097`): before constructing `wsgit.CommitOptions`, compute
   scope-active with a cheap wsdoc-side check. Two implementation options,
   pick the lighter one:
   - (a) Reuse existing exported `wsdoc.TicketScope(root, nil)` and read only
     `.Active` — simplest, zero new exported surface, but does one extra
     `git ls-files` + a `os.Stat` per open ticket that is wasted work (the
     Hidden/HiddenStems fields go unused here).
   - (b) Add a minimal new exported wsdoc function (e.g.
     `wsdoc.SparseCheckoutActive(root string) bool`) built directly on
     `newTicketScope(root) != nil` (git.go#L58-L82) with no `indexPaths()`
     call — cheaper, purpose-built.
   Recommendation: (b), since it is a small, clearly-scoped addition that
   avoids doing (and then discarding) a full index-hidden-count computation on
   every `git.commit` call, and it composes cleanly with the "cheapest-first,
   filesystem-only-when-possible" doctrine `newTicketScope`'s own comment
   states. This is a reversible implementation detail, not a ticket-settled
   decision — either option satisfies the ticket's contract.
8. **`scope_announcement.go` idea/ inclusion** —
   `agents-plugin-tool/internal/mcp/scope_announcement.go`:
   - `#L20`: change `[]string{"ready", "todo"}` to `[]string{"ready", "todo", "idea"}`.
   - `#L28`: extend the zero-hidden sentence to name idea/ alongside ready/todo.
   - `#L30-L33`: extend the hidden-N sentence's directory list to include
     `ai-docs/tickets/idea/`.
   - `#L35`: append the new one-line pointer, e.g. "Run `git sparse-checkout
     list` to see this worktree's active re-include patterns." — placed after
     the existing "See ai-docs/ref/worktree-ticket-scope.md..." sentence so the
     `strings.Contains(..., "git sparse-checkout list")` assertion in a new/
     updated test has a stable substring to check.
9. **Spec updates** (per ticket's Spec Impact list, Phase 1 subset only):
   - `ai-docs/spec/mcp-tools.md` `{#260626-workflow-manual-restoration-entry}`
     (`#L523-L532`): amend the scope-announcement description to name idea/
     alongside ready/todo, and add the `git sparse-checkout list` pointer.
   - `ai-docs/spec/mcp-tools.md` near `{#260513-git-commit-result-edition-detection}`
     (`#L1535` area — read the surrounding paragraph before editing to match
     existing prose style): add a paragraph describing the new `--sparse`
     staging behavior under an active scope, the guardrail (never stages a
     deletion of an absent skip-worktree path), and that `tickets.create_empty`
     is unaffected.
   - `ai-docs/spec/workflow-skills.md` `{#260610-entry-skill-surface-reduction}`
     area (`#L69-L72`): remove/rewrite the "idea/ always stays visible" clause
     in the `lead-scope-worktree` roster description.
   - Do **not** touch the `project_tree` ticket-inventory Spec Impact bullet —
     that is Phase 2, out of scope here.
10. **Tests** — add/extend, mirroring the patterns found:
    - `agents-plugin-tool/internal/wsgit/git_test.go`: new
      `TestCommitStagesExplicitPathUnderActiveSparseScope`-style test(s)
      asserting `stagingCommandsForCommit`/`Client.Commit` with
      `SparseScopeActive: true` produces `add -A --sparse --` for a plain new
      path, and a companion test asserting `SparseScopeActive: false` (or
      omitted) reproduces today's exact command shape (regression pin).
    - `agents-plugin-tool/internal/mcp/scope_announcement_test.go`: extend
      `enableSparseCheckout` (or add a variant) to also exclude/reinclude an
      idea/ stem, add a hidden idea fixture via `mustWriteAndCommitTicket`, and
      assert the rendered banner names the idea stem and contains
      `git sparse-checkout list`. Extend
      `TestWorkflowManualScopeAnnouncementByteUnchangedWhenUnscoped` is
      unaffected (already scope-inactive, stays as-is).
    - Consider one integration-style Go test (or document as manual-only, see
      Verification Plan) exercising the full lifecycle: sparse-scope excluding
      idea/ with `.gitkeep` forced, a real captured idea file staged via
      `--sparse` through the actual `wsgit.Client.Commit` against a real throwaway
      git repo (the existing test helpers already do this — `runGit`, `initGit`,
      `mustWrite`), then a `git sparse-checkout reapply` to prove self-hiding —
      this exercises real git subprocess behavior end-to-end rather than mocked
      staging-command assembly alone, closest in spirit to
      `scope_announcement_test.go`'s existing real-git-repo tests.

## Verification Plan

Ticket lists 5 concrete steps; mapped to test type:

1. `ls ai-docs/tickets/idea/` shows only `.gitkeep` under an active
   idea-excluding scope — **unit/integration-testable** via a Go test using
   `os.ReadDir` against a throwaway repo with the derived pattern applied
   (same shape as `enableSparseCheckout`), or manual `bash` verification during
   review. Recommend a Go integration test since the fixture helpers already
   exist.
2. `tickets.create_empty(initial_state:"idea")` writes under idea/, and a
   subsequent `ws/git.commit` on that path stages+commits via `--sparse` with
   no manual widen — **unit-testable** at the `wsgit` layer (step 10's new
   `stagingCommandsForCommit`/`Client.Commit` test) for the staging mechanics;
   the `tickets.create_empty` write-through-hidden-dir half needs a **scripted
   integration check** (real repo, real sparse pattern, real `os.WriteFile`
   via `TicketCreate`) since it crosses the wsdoc/wsgit boundary that unit
   tests intentionally keep separate.
2b. The committed idea file self-hides on a fresh sparse re-apply yet remains
    in the index/other worktrees — **needs a scripted integration check**
    (`git sparse-checkout reapply` + `git ls-files -v` + `git show HEAD:<path>`
    in a real repo); not mockable at the Go-struct level since it depends on
    git's own skip-worktree re-evaluation, which this survey directly verified
    by hand (see Codebase Findings) but which should also be pinned by an
    automated test to guard against regression.
3. A fresh `workflow_manual` call reports the hidden idea count and the
   `git sparse-checkout list` pointer — **unit-testable**, extend
   `TestScopeAnnouncementFiresOnWorkflowManual` per step 10.
4. An attempted cross-scope ready/todo move still refuses with the widen tip
   — **already covered** by existing `tickets_mutate.go` behavior and its
   existing tests (no code change here); rerun the existing test suite to
   confirm no regression, do not add a new test for this step since nothing
   changes on that path.

Commands:
```bash
cd agents-plugin-tool
go test ./internal/wsgit/... -run TestCommit -v
go test ./internal/mcp/... -run TestScopeAnnouncement -v
go test ./internal/wsdoc/... -run TestTicketScope -v   # regression: unaffected package
WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest
WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
python3 -m unittest discover agents-plugin-wsflow/tests
go build ./...
go vet ./...
```

## Escalations

- None. The two design points that looked like they might need research
  escalation — (a) how `git.commit` detects "active sparse-checkout", and
  (b) whether the sparse-hidden-vs-deleted disambiguation has a clean signal —
  were both resolved during survey with direct empirical verification (git
  2.43, throwaway repo, matching the ticket's own verified-experiment
  epistemics) plus an existing, reusable in-tree mechanism:
  - (a) `internal/wsdoc/tickets_scope.go`'s `newTicketScope`/`TicketScope`
    already implements a cheap, filesystem-first "is scope active" gate,
    reachable from `internal/mcp` (which already imports both `wsdoc` and
    `wsgit`) without violating the one-directional
    `{#260720-wsdoc-commit-boundary}` rule (wsdoc must not import wsgit; the
    reverse is unconstrained and the MCP layer is the established bridge,
    mirroring the existing `Verifier`/`verifyAdapter` injection shape).
  - (b) Verified directly: a tracked, skip-worktree, on-disk-absent,
    unmodified path never appears in `git status --porcelain=v2` output, so
    the existing `#260513` `deletedPathsUnderCommitRoot` logic (which reads
    only pre-staging `git status`) can never misclassify a merely-hidden path
    as deleted — no disambiguation code is actually needed on the read side.
    On the write side, `git add -A --sparse` on such a path fails loudly
    (exit 128, nothing staged) rather than silently staging a deletion, so the
    worst case if a caller ever batches a stale hidden path into `paths` is a
    hard error, not silent index corruption. The safe design is therefore to
    scope `--sparse` to only the `add` command (never `rm --cached`), which
    the Implementation Plan states explicitly (step 6).
  Both are still genuinely reversible implementation choices (e.g., which of
  the two wsdoc-export options in step 7 to use) rather than open strategy
  forks, so a stated recommendation was given instead of an escalation.
