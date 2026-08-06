# Plan: 260806-feat-worktree-ticket-scope — Phase 2: ws:lead-scope-worktree skill and reference manual

## Relevant Ticket Contract

- Deliverable 1: `ws:lead-scope-worktree`, a thin entry skill that always opens
  by discussing with the user what the worktree targets *before touching
  anything*, derives the pattern set from that conversation, applies it,
  verifies by listing the affected directories, and offers
  `git sparse-checkout disable` as the restore path. Naming follows the
  verb-first convention of `lead-add-rule`, `lead-drain-ready-queue`,
  `lead-forge-spec`, `lead-write-ticket`.
- The skill must state that promoting an out-of-scope ticket into a hidden
  status directory (the `idea/` -> hidden `todo/` dogfood-capture hot path)
  requires widening the pattern first (`git sparse-checkout add <path>`).
- Deliverable 2: `ai-docs/ref/worktree-ticket-scope.md` — the manual body
  carrying verified properties, failure modes, the cross-scope `git mv`
  behavior and its widen-then-retry remedy, and the unreproduced hazard plus
  the mandatory verification-by-listing step it forces. The skill carries
  procedure only; the manual carries the reference material.
- Deliverable 3: `ws/workflow_manual` renders the active sparse-checkout scope
  only when `core.sparseCheckout` is set, alongside the environment warnings
  it already carries; byte-unchanged when unset. Handler:
  `agents-plugin-tool/internal/mcp/workflow_manual.go`
  (`handleWorkflowManual`).
- Scope covers `ready/` + `todo/`; `idea/` stays visible (ticket `##
  Decisions`, "Scope covers `ready/` and `todo/`; `idea/` stays visible").
  `--no-cone` is required (cone mode cannot express a per-file scope).
- Phase 1 (already landed, `Result (0daa9b74)`) is the only reusable index-
  aware primitive; Phase 2 must not re-derive it. `wsdoc.TicketScope(root,
  statuses)` is the exact reusable call: it degrades to
  `{Active:false, Hidden:0}` after at most two `os.Stat` calls when
  `core.sparseCheckout` is unset — no git subprocess spawned in the common
  case — which is what makes the workflow_manual gate cheap to render on every
  session start.
- Verification the ticket requires for this phase (all three deliverables,
  not just existence):
  - a test for `workflow_manual`'s scope-rendering gate (code-level gate, so
    it "takes a test rather than an inspection");
  - an end-to-end real-worktree run of the skill: discuss-first, apply,
    confirm-by-listing, promote-an-out-of-scope-ticket to observe
    widen-then-retry, restore with `git sparse-checkout disable`, and the
    restored worktree byte-identical to its pre-scope state;
  - with a scope active, `ws/git.commit` on an unrelated ticket emits no
    `FIX:` advisory (checks the Phase 1 guarantee from the skill's side);
  - the skill text passes the `ai-docs/ref/skill-authoring.md` invariant
    checklist.
- Spec Impact (ticket `## Spec Impact`, "`workflow-skills` and `mcp-tools`,
  for phase 2"): `workflow-skills.md` gains `ws:lead-scope-worktree` in the
  entry-skill inventory; `mcp-tools.md` anchor
  `#260626-workflow-manual-restoration-entry` documents the new scope
  rendering, gated on `core.sparseCheckout`.

## Out of Scope

- Phase 1's index-aware resolution code (`internal/wsdoc/tickets_scope.go`
  and its call-site wiring) — already landed and must only be *read*, not
  changed, by this phase.
- Automatic pattern derivation from an epic stem or graph closure (ticket
  `## Non-Scope`) — the skill's conversation plus explicit add/remove is the
  full mechanism.
- `260523-bug-worktree-local-index-missing` (worktree-local workflow-context
  propagation) — explicitly out of scope per ticket `## Non-Scope`.
- `260728-research-ticket-graph-load-cost-commit-path`'s graph-load cost
  question — unrelated to this phase.

## Codebase Findings

- `agents-plugin-tool/internal/wsdoc/tickets_scope.go#L408-L449` —
  `TicketScopeInfo{Active bool; Hidden int; HiddenStems []string}` and
  `TicketScope(root string, statuses []string) (TicketScopeInfo, error)` are
  the exact reusable primitive for the manual render: statuses `nil` defaults
  to `ready/todo/idea` (`ticketScopeStatuses`, L451-L466), so pass
  `[]string{"ready", "todo"}` explicitly to match the ticket's "scope covers
  `ready/`+`todo/`" framing, or pass `nil` if the render should also surface
  hidden `idea/` counts (none expected, since `idea/` is never hidden by
  design — worth a one-line note in the manual, not a code branch). No git
  subprocess fires when unscoped (comment at L45-L57 states the two-`os.Stat`
  bound explicitly).
- `agents-plugin-tool/internal/wsdoc/tickets_scope.go` exposes **no raw
  sparse-checkout glob patterns** — only `HiddenStems` (ticket stems, not
  glob text). A render that wants to show the literal active patterns (e.g.
  `!/ai-docs/tickets/todo/*`) would need a new read of
  `$GIT_DIR/info/sparse-checkout`, which Phase 1 does not expose. The
  reusable, zero-new-git-call content is therefore "N ticket(s) hidden
  (stems: ...)", not literal pattern text.
- `agents-plugin-tool/internal/mcp/doc_coverage_alarm.go#L1-L47` and
  `agents-plugin-tool/internal/mcp/bootstrap_alarm.go#L53-L96` — the exact
  reusable shape for a `ws/workflow_manual` conditional-render addition: a
  `compute...(root, ...) string` returning `""` when nothing should render,
  and a generic `injectBootstrapStalenessWarning(body, warning string) string`
  that is a no-op passthrough when `warning == ""` (L88-L96) and otherwise
  prepends `warning + "\n\n" + body`. `docCoverageWarning` already delegates
  to this generic injector rather than duplicating it (L45-L47) — the new
  scope announcement should do the same: `injectBootstrapStalenessWarning(body,
  scopeAnnouncement(root))`, no new injector function needed.
- `agents-plugin-tool/internal/mcp/workflow_manual.go#L251-L313` — the two
  call sites where root is known and the two existing warnings are already
  injected: FRESH-with-root (`canonical`, L271-L282) and CONTINUE
  (`rec.Root`, L299-L311), both gated on the root being non-empty. The new
  scope announcement must be injected at the same two sites, using the same
  root variables, so FRESH-without-root correctly renders no scope block (no
  root to check). Import needed: `"github.com/kang-sw/devenv/internal/wsdoc"`
  — already imported elsewhere in package `mcp` (e.g.
  `doc_coverage_alarm.go`), so no import-cycle risk.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L3399-L3442`
  (`TestWorkflowManualContinueMode`) and
  `agents-plugin-tool/internal/mcp/doc_coverage_alarm_test.go` (whole file) —
  the exact test fixture pattern to copy: `initGit(t, root)` /
  `runGit(t, root, ...)` helpers already exist in package `mcp`
  (`server_test.go:698`), and `doc_coverage_alarm_test.go` already
  demonstrates the "fires in FRESH-with-root, fires in CONTINUE, silent when
  the gate condition is false" three-test shape to mirror for the scope
  announcement.
- `agents-plugin-tool/internal/wsdoc/tickets_scope_test.go#L560-L660`
  (`TestTicketScopeGateIsInertWithoutSparseCheckout`,
  `requireGateSpawnsNoGit`) — Phase 1's own byte-identical/zero-subprocess
  test technique (PATH-shimmed `git` that fails on invocation). This
  guarantee is already proven at the `wsdoc` layer; the new `mcp`-layer test
  does not need to re-prove zero-subprocess, only that
  `handleWorkflowManual`'s output is unchanged when `scopeAnnouncement`
  returns `""` (same "no-op passthrough" property the existing two warnings
  already rely on).
- `agents-plugin/skills/lead-add-rule/SKILL.md` (4 lines) and
  `agents-plugin/rsrc/lead-add-rule/lead-add-rule.md` — the exact shim +
  Choreography-skill shape to copy for `lead-scope-worktree`: `SKILL.md` is
  `name:`/`description:` frontmatter, an H1, one line calling
  `ws/playbook.print(name: "lead-add-rule")` and executing inline, closed by
  `If this call fails to connect, run \`/ws:mcp-server-repair\`.`. The rsrc
  playbook is `kind: print` frontmatter, `Invariants` -> `On: invoke`
  (numbered sub-steps) -> `Judgments` (`judge:` tables) -> `Templates` ->
  `Doctrine`. No `delegates: true` needed for `lead-scope-worktree` — the
  skill applies `git sparse-checkout` directly, spawning no subagent.
- `agents-plugin-wsflow/skills/lead-add-rule/SKILL.md` — the wsflow mirror
  shim shape: identical body, `wsflow/playbook.print(name: "lead-add-rule")`,
  closed by `/wsflow:mcp-server-repair`.
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L16-L73` —
  `EXPECTED_SKILLS` (set of all shipped wsflow skill dirnames) and
  `POINTER_TAIL_TITLES` (dict mapping single-call shim skills to their H1
  title, e.g. `"lead-add-rule": "Add Rule"`) both need a
  `"lead-scope-worktree": "Scope Worktree"` entry. `lead-scope-worktree` is a
  plain `playbook.print` shim, not inline-bodied, so it does **not** go in
  `EXPECTED_INLINE_SKILLS` or `EXPECTED_PARALLEL_INIT_SKILLS`.
- `ai-docs/mental-model/workflow-skills.md#L111-L119` — the "Add a Codex
  workflow skill" change recipe is the authoritative step list (creates both
  trees, wires wsflow, regenerates three manifests). It names env var
  `WS_REGEN_MANIFEST` for the rsrc-manifest regen test; confirmed by direct
  read of `internal/wsrsrc/manifest_shipped_test.go#L93-L106`
  (`TestRegenerateShippedManifest`, gated on `WS_REGEN_MANIFEST=1`, writes
  `agents-plugin/rsrc/manifest.json`) that this is correct (a second,
  functionally-overlapping test `TestGenerateRealManifest` in
  `wsrsrc_test.go:967-982` is gated on `WSRSRC_REGEN` instead — either
  regenerates the same file; the mental model's `WS_REGEN_MANIFEST` name is
  accurate and should be used to match the documented recipe verbatim).
- `ai-docs/ref/wsflow-mirroring.md#L239-L266` ("Rsrc Tree Provisioning") —
  confirms the exact regen order and commands:
  `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run
  TestGenerateRealManifest` (or the `WS_REGEN_MANIFEST` variant), then
  `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run
  TestRegenerateWsflowRsrcMirror`. `-count=1` is called out twice as
  mandatory (go test cache silently no-ops the write side effect otherwise).
- `agents-plugin-tool/internal/wsrsrc/skills_manifest_test.go#L49-L68`
  (`TestGenerateRealSkillsManifest`, gated `WSRSRC_REGEN_SKILLS`) — the
  independent skills-tree manifest regen, distinct from the two rsrc-tree
  regens above; must run separately since a new `agents-plugin/skills/`
  directory is added.
- `ai-docs/ref/skill-authoring.md` (whole file) — the invariant checklist
  (`Falsifiable`/`Actionable`/`One line`/`Context-free`/`Non-redundant`/
  `Doctrine-aligned`, `## Authoring Rules` -> `### Invariant checklist`) and
  the Choreography-skill layout (`## Skill Layout`, "Choreography skill"
  block) apply directly, since `lead-scope-worktree` calls no `enter.*`
  routing tool (Layer 2 does not apply) and is sequential-step choreography.
  `## On: Fresh-Reader Audit` (L107-L116) is required after authoring both
  `SKILL.md` and the rsrc playbook.
- `ai-docs/spec/workflow-skills.md#L14-L82` — the entry-skill roster: a
  fenced fifteen-skill code list at L24-L42 (needs
  `lead-scope-worktree` inserted alphabetically) and a prose sentence at
  L63-L68 naming "13 entry skills" (must become 14 and list
  `lead-scope-worktree`).
- `ai-docs/spec/mcp-tools.md#L407-L451`
  (`### Workflow Manual Entry And Restoration {#260626-workflow-manual-restoration-entry}`)
  — the exact anchor the ticket's Spec Impact section names for documenting
  the new scope-rendering branch, gated on `core.sparseCheckout`.
- `ai-docs/ref/ws-mcp.md` (whole file, short) — closest stylistic precedent
  for `ai-docs/ref/worktree-ticket-scope.md`: opens with a "what this file is
  / is not, where the real contract lives" pointer block, then operational
  detail sections.
- Ticket `## Verified behavior` and `### Unreproduced hazard`
  (`ai-docs/tickets/ready/260806-feat-worktree-ticket-scope.md#L164-L199`)
  is the exact verified-property and failure-mode content the manual body
  must carry (patterns worktree-local, per-file hiding, `git ls-files -v`
  `S` marker, exit-1 staging refusal, atomic no-op cross-scope `git mv`,
  `git sparse-checkout disable` full restore, and the unreproduced
  Windows/git-2.48.1 "whole directory vanishes" hazard with its mandated
  verify-by-listing consequence).

## Implementation Plan

1. Write `agents-plugin/rsrc/lead-scope-worktree/lead-scope-worktree.md`
   (`kind: print` frontmatter) as a Choreography skill mirroring
   `lead-add-rule.md`'s shape:
   - `Invariants`: discuss-first before any pattern write; `--no-cone`
     required; scope covers `ready/`+`todo/`, `idea/` always stays visible;
     verify by listing affected status directories after every apply
     (mandatory, not optional — the unreproduced hazard's only defense);
     promoting an out-of-scope ticket into scope requires widening the
     pattern first; restore is `git sparse-checkout disable`.
   - `On: invoke`: (1) discuss what the worktree targets, wait for the
     user's topic/pattern decision; (2) derive the `git sparse-checkout set
     --no-cone /* !/ai-docs/tickets/ready/* !/ai-docs/tickets/todo/*
     <topic re-includes>` command from the conversation; (3) apply it; (4)
     verify by listing `ai-docs/tickets/ready/` and `ai-docs/tickets/todo/`
     contents and reporting exactly what remains visible; (5) offer/explain
     the widen-then-retry remedy and the `git sparse-checkout disable`
     restore path.
   - `Doctrine`: one paragraph naming the finite resource (attention on an
     unrelated work line's tickets) the skill trades against the visibility
     cost of a wrong scope (cheap to correct, per ticket `## Non-Scope`).
   - Apply `ai-docs/ref/skill-authoring.md`'s invariant checklist to every
     Invariants line before finalizing.
2. Write `agents-plugin/skills/lead-scope-worktree/SKILL.md`: frontmatter
   `name: lead-scope-worktree`, a description capturing "scope a worktree's
   ticket board to one work line via sparse-checkout", H1, one line calling
   `ws/playbook.print(name: "lead-scope-worktree")`, closed by
   `If this call fails to connect, run \`/ws:mcp-server-repair\`.` — copy
   `lead-add-rule/SKILL.md`'s exact structure.
3. Write `agents-plugin-wsflow/skills/lead-scope-worktree/SKILL.md`: same
   frontmatter/body as step 2 but `wsflow/playbook.print(name:
   "lead-scope-worktree")` and the `/wsflow:mcp-server-repair` pointer —
   copy `agents-plugin-wsflow/skills/lead-add-rule/SKILL.md`'s shape.
4. Edit `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`: add
   `"lead-scope-worktree"` to `EXPECTED_SKILLS` (L16-L39) and
   `"lead-scope-worktree": "Scope Worktree"` to `POINTER_TAIL_TITLES`
   (L58-L73).
5. Write `ai-docs/ref/worktree-ticket-scope.md`: verified properties,
   failure modes, cross-scope `git mv` behavior + widen-then-retry remedy,
   the unreproduced hazard and its mandatory verify-by-listing consequence —
   content sourced from the ticket's `## Verified behavior` and `###
   Unreproduced hazard` sections (do not re-derive; transcribe/compress).
   Open with a pointer block to the caller-visible contract in
   `ai-docs/spec/mcp-tools.md` (`#260806-worktree-sparse-checkout-ticket-scope`,
   `#260626-workflow-manual-restoration-entry`) per the `ws-mcp.md`
   convention, since this file is reference material, not the contract
   itself.
6. Add `scopeAnnouncement(root string) string` in a new
   `agents-plugin-tool/internal/mcp/scope_announcement.go`, mirroring
   `docCoverageWarning`'s shape: call `wsdoc.TicketScope(root, []string{"ready",
   "todo"})`; return `""` when `!info.Active`; otherwise render a short
   `> **Sparse-checkout scope is active.**` block naming the hidden count and
   stems, plus a pointer to `ai-docs/ref/worktree-ticket-scope.md` and
   `git sparse-checkout disable`.
7. Wire `scopeAnnouncement` into
   `agents-plugin-tool/internal/mcp/workflow_manual.go`'s two root-known
   branches (`handleWorkflowManual` FRESH-with-root L271-L282, CONTINUE
   L299-L311), calling
   `body = injectBootstrapStalenessWarning(body, scopeAnnouncement(<root
   var>))` alongside the two existing warning injections — no new injector
   function, reuse the existing no-op-when-empty one.
8. Add `agents-plugin-tool/internal/mcp/scope_announcement_test.go`: three
   tests mirroring `doc_coverage_alarm_test.go`'s shape — fires in
   FRESH-with-root, fires in CONTINUE, and a
   `TestWorkflowManualScopeAnnouncementByteUnchangedWhenUnscoped` test that
   calls `workflow_manual` in a plain `initGit`-only repo (no
   sparse-checkout) and asserts the scope block's marker text is absent
   (mirrors `TestDocCoverageWarningSilentWhenBothAreasCovered`'s technique;
   Phase 1's `wsdoc` package already separately proves the zero-subprocess
   property via `requireGateSpawnsNoGit`, so this test only needs to prove
   the render-layer no-op, not re-prove the subprocess bound).
9. Run the three-command manifest/mirror regen sequence from
   `agents-plugin-tool/`, in order:
   `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run
   TestRegenerateShippedManifest`;
   `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run
   TestRegenerateWsflowRsrcMirror`;
   `WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run
   TestGenerateRealSkillsManifest -count=1`.
10. Update `ai-docs/spec/workflow-skills.md`: insert `lead-scope-worktree`
    into the fenced roster (L24-L42, alphabetical) and change the "13 entry
    skills" prose sentence (L63-L68) to 14, naming
    `lead-scope-worktree` and its discuss-first contract.
11. Update `ai-docs/spec/mcp-tools.md` anchor
    `#260626-workflow-manual-restoration-entry` (around L407-L451): add a
    sentence documenting that FRESH-with-root and CONTINUE both render an
    active sparse-checkout scope announcement when `core.sparseCheckout` is
    set, with no change to any other branch or to the unscoped case.
12. Run the `ai-docs/ref/skill-authoring.md` `## On: Fresh-Reader Audit` over
    both `agents-plugin/skills/lead-scope-worktree/SKILL.md` and
    `agents-plugin/rsrc/lead-scope-worktree/lead-scope-worktree.md`; edit
    only `fix`-classified findings.
13. End-to-end skill exercise (manual, not unit-testable): in a real worktree
    of this repository, run `ws:lead-scope-worktree`, confirm discuss-first
    ordering, apply a `ready/`+`todo/` scope, list the affected directories to
    confirm exactly the intended tickets remain, promote a captured `idea/`
    ticket into hidden `todo/` to observe the widen-then-retry path, restore
    with `git sparse-checkout disable`, and diff the worktree against its
    pre-scope state to confirm byte-identity. Separately confirm
    `ws/git.commit` on an unrelated ticket edit emits no `FIX:` advisory while
    the scope is active (Phase 1 guarantee, checked from the skill side).

## Verification Plan

- `cd agents-plugin-tool && go build ./... && go vet ./...`
- `cd agents-plugin-tool && go test ./internal/mcp/... -run
  'TestWorkflowManual|TestScopeAnnouncement' -v` (new + existing
  workflow_manual tests)
- `cd agents-plugin-tool && go test ./internal/wsrsrc/... -count=1` (manifest
  drift gates: `TestShippedManifestUpToDate`,
  `TestSkillsManifestDriftIsVisible`, `TestWsflowRsrcMirrorUpToDate`)
- `python3 -m unittest discover agents-plugin-wsflow/tests` (wsflow bundle
  inventory + pointer-tail checks, including the new
  `lead-scope-worktree` entries)
- `cd agents-plugin-tool && go test ./... -count=1` (full package suite,
  clean, per Phase 1's own verification bar)
- Manual: the end-to-end real-worktree skill run described in Implementation
  Plan step 13 — no automated test can exercise git's live sparse-checkout
  hazard-verification step or the discuss-first conversational ordering.
- Manual: apply the `ai-docs/ref/skill-authoring.md` Fresh-Reader Audit to
  both new skill files (step 12 above); record findings inline, fix only
  `fix`-classified ones.

## Escalations

- None. The ticket's Phase 2 text and the current code agree on every point
  surveyed: Phase 1's `TicketScope` primitive exposes exactly what the
  rendering gate needs (hidden count + stems, zero-git-call when unscoped),
  the "Add a Codex workflow skill" change recipe's commands were each
  independently confirmed against the actual gated test functions, and the
  existing `bootstrapStalenessWarning`/`docCoverageWarning` pair is a
  directly-reusable template for the new conditional render with no design
  decision left open. One minor gap worth the lead's attention, not a
  strategy question: Phase 1 does not expose literal sparse-checkout glob
  pattern text (only ticket stems), so the manual-render wording in step 6
  is scoped to "hidden count + stems," not literal pattern strings — flagged
  in Codebase Findings, not blocking.
