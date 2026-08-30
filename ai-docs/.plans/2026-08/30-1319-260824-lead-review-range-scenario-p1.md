# Plan: 260824-feat-lead-review-range-scenario — Phase 1: Scenario-kind diff selection (branch vs range)

## Relevant Ticket Contract

- Parameterize `lead-review`'s target selection by scenario kind: existing
  branch scenario, plus a new `range` scenario that reviews
  `git.diff(range: "<base>..<head>")` / `git.log(range:)` instead of a checked-out
  branch. Base/head are supplied by the caller (sweep/gate tickets ③/④); this
  ticket does not own the marker.
- Downstream phase machinery (intent/alignment/risk phases, `judge:
  is-large-diff`/Deep Review, verdict routing LGTM/NEEDS FIX/OPEN) is
  diff-content-agnostic and stays unchanged — only how the target diff is
  determined becomes scenario-parameterized.
- Config-load behavior is scenario-scoped (2026-08-30 discuss decision):
  - Branch scenario: absent `ai-docs/_review.local.md` still forces `On: setup`
    (unchanged from today).
  - Range scenario: absent config runs on built-in review-substance defaults
    (intent/alignment/risk phase text, Deep Review threshold 20 files/500
    lines) and **never** enters `On: setup`. When present, the range scenario
    still honors the config's review-substance sections.
  - Range scenario touches no checkout/remote/merge, so the
    collaboration/remote config half (Remote, Branch Naming, Comment/Merge
    Approval/Notification Method, Contributor Workflow) is meaningless to it
    and must not be required.
- Verification boundary (from ticket): a range scenario over a known
  `base..head` produces the same phase/verdict flow as a branch scenario over
  the equivalent branch diff; `is-large-diff`/Deep Review threshold still trips
  on a large range; a range scenario with no config present runs on built-in
  defaults and never enters `On: setup`; the branch/PR scenario with no config
  still forces setup as today; a present config's review-substance sections are
  honored by both.
- Spec impact target: the `lead-review` behavior area in
  `ai-docs/spec/workflow-skills.md` (anchor `#260513-review-workflow-skill`,
  currently at lines 1124-1151) must gain the range/watermark scenario
  description and the scenario-scoped config-load rule. No change to verdict
  vocabulary or the existing branch scenario's own behavior.

## Out of Scope

- Phase 2 (landing lens as a review-config required-check) — do not add the
  landing lens, do not touch the `## Review Config Template` block's phase
  list or add a required-check section in this phase.
- Owning or minting the `base..head` marker itself — the caller (③/④) supplies
  it; this phase only consumes a caller-supplied range.
- Changing verdict vocabulary (BLOCKED/LGTM/NEEDS FIX/OPEN) or the
  intent/alignment/risk phase bodies.
- Any change to the branch scenario's own runtime behavior beyond making the
  existing "run setup if absent" rule explicitly branch-scoped in wording.
- `agents-plugin/skills/lead-review/SKILL.md` and
  `agents-plugin-wsflow/skills/lead-review/SKILL.md` bodies — both are already
  pure `playbook.print` shims with no scenario-specific text; no edit needed
  there.

## Codebase Findings

- `agents-plugin/rsrc/lead-review/lead-review.md#L9-L16` — `Invariants`
  section, line 11: `Load ai-docs/_review.local.md before any review step; run
  setup if absent.` This is the exact rule the ticket's scenario-scoping
  decision overrides; must become scenario-conditional (branch: force setup;
  range: built-in defaults, never setup).
- `agents-plugin/rsrc/lead-review/lead-review.md#L17-L45` — `## On: invoke
  [branch?]` handler, four H3 sub-steps: `1. Load config`, `2. Identify
  branch`, `3. Prepare`, `4. Review`. This is the single entry handler that
  must branch by scenario kind:
  - `1. Load config` (L19-23): step 2 (`If absent → go to On: setup`) is
    branch-scenario-only; range scenario needs a parallel absent-config branch
    that falls through to built-in review-substance defaults instead.
  - `2. Identify branch` (L25-28) is branch-scenario-only (branch discovery is
    meaningless for a range).
  - `3. Prepare` (L30-35): steps 1-3 (record current branch, fetch, checkout)
    are branch-scenario-only per the ticket ("touches no checkout/remote/merge").
    Step 4 (`judge: has-blocked-paths`) is diff-content-based, not
    checkout-based, so it plausibly still applies to the range scenario
    (working off the range diff) — flagging as a design point the executor
    should decide inline (no ticket text either forbids or requires it, and it
    follows directly from "existing phase machinery... stays as-is").
  - `4. Review` (L37-44) step 1 (`git.diff(mode: "stat")`, no `range:` passed)
    is the branch-scenario diff call; range scenario must call
    `git.diff(range: "<base>..<head>", mode: "stat")` and use `git.log(range:
    "<base>..<head>")` for commit enumeration instead. Steps 2-6 (judges,
    phase execution, checklist, verdict) are unchanged per ticket.
- `agents-plugin/rsrc/lead-review/lead-review.md#L67-L106` — `### Review
  Config Template` fenced block lists sections in write order: Remote, Branch
  Naming, Review Phases, Checklist, Blocked Paths, Comment Method, Merge
  Approval Method, Notification Method, Contributor Workflow, Deep Review.
  `Review Phases` (L78-84) and `Deep Review` (L104-105) are the two sections
  with an explicit built-in default text already inline in this template —
  these are the literal "built-in defaults" the range scenario should read
  from when no config file exists (no new default text needs inventing;
  reuse this block's existing phase/threshold wording as the fallback).
  `Checklist` and `Blocked Paths` are already optional/absent-safe via their
  own judges (`has-checklist`, `has-blocked-paths`; see below) — they need no
  separate "built-in default" handling since an absent config for either
  simply means the judge does not fire, whether branch or range scenario.
- `agents-plugin/rsrc/lead-review/lead-review.md#L158-L202` — `## Judgments`
  section: `judge: has-blocked-paths`, `judge: follows-ws-workflow`, `judge:
  is-large-diff`, `judge: has-checklist`, `judge: has-comment-method`, `judge:
  has-merge-approval-method`, `judge: has-notification-method`. Per ticket,
  these stay diff-content-agnostic and unchanged; `has-comment-method`,
  `has-merge-approval-method`, `has-notification-method`, and
  `follows-ws-workflow`'s `Contributor Workflow` config all belong to the
  branch-scenario collaboration/remote half and are naturally inert for range
  reviews (their triggering sections won't exist in a range-scenario
  built-in-defaults run) — no judge text edit needed, only the config-load
  Invariant and `On: invoke`/`On: setup` entry logic need to change.
- `agents-plugin/rsrc/lead-review/lead-review.md#L206-L212` — `## Doctrine`
  section, exact sentence `Review optimizes for **maintainer decision quality
  with minimum friction**.` is asserted verbatim by
  `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L2749-L2765`
  (`TestPlaybookPrintGoldenLeadReview`, checks doctrine text is a substring and
  that no "Continuity tip" leaks in). Do not alter or remove this sentence in
  this phase; if Doctrine needs new content for the scenario split, append,
  don't replace.
- `agents-plugin-tool/internal/mcp/server.go#L879-L901` — `git.diff` accepts
  `range` (string) and `mode` params; `git.log` accepts `range`, `limit`,
  `include_body`. Confirms the MCP tool surface the ticket cites
  (`wsgit.DiffOptions.Range`) already exists and needs no tool-side change —
  this is Layer 1 (MCP schema), so the playbook should reference the call
  shape (`git.diff(range: "<base>..<head>")`) without restating full param
  semantics per `ai-docs/manuals/skill-authoring.md`'s Layer 1 rule.
- `agents-plugin-tool/internal/wsgit/git.go#L250-L276` (`DiffArgs`) — when
  `Range` is empty, `git diff --stat` runs with no revision arg (working tree
  vs index), confirming the branch scenario's current step-4-1 call is
  unrelated to range plumbing and does not need to change; only the new range
  branch adds an explicit `range:` argument.
- `agents-plugin/rsrc/lead-review/lead-review.md` mirrors byte-identically
  into `agents-plugin-wsflow/rsrc/lead-review/lead-review.md` — confirmed via
  `find` diff of both trees (same relative paths present). Per
  `ai-docs/manuals/wsflow-mirroring.md` "Generated-sameness carve-out", only
  edit the canonical `agents-plugin/rsrc/lead-review/lead-review.md`; the
  wsflow copy is regenerated, never hand-edited.
- `agents-plugin/skills/lead-review/SKILL.md` and
  `agents-plugin-wsflow/skills/lead-review/SKILL.md` — both already pure
  `playbook.print`/`playbook.print` shims with no scenario text (`lead-review`
  is in the wsflow shipped skill list and is not one of the
  substitution-mirrored-inline exceptions), confirming no skill-entry edit is
  needed; the entire change is confined to the shared rsrc playbook body.
- `ai-docs/spec/workflow-skills.md#L1124-L1151` — current `lead-review`
  behavior-area prose under anchor `#260513-review-workflow-skill`. Describes
  only the branch/PR scenario and today's unconditional "run setup if absent."
  Needs a new paragraph (or amended paragraph) describing the range scenario
  and the scenario-scoped config-load rule, keeping the anchor id.
- `agents-plugin/rsrc/manifest.json#L28` — current hash for
  `lead-review/lead-review.md` is
  `8fec80ea720c248774b20188a647d1e9eb2a8f56956c7dd7191772e96c2f8e33`; per
  `agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go`
  (`TestShippedManifestUpToDate`) and `wsrsrc_test.go#L967-L973`
  (`TestGenerateRealManifest`), this hash goes stale the moment the file body
  changes and must be regenerated (see Implementation Plan step 10). No
  code-level test asserts specific `lead-review.md` prose beyond the doctrine
  substring checked by `TestPlaybookPrintGoldenLeadReview`.

## Implementation Plan

1. Edit `agents-plugin/rsrc/lead-review/lead-review.md` `## Invariants`
   (around L11): replace the single unconditional config-load line with a
   scenario-scoped rule (e.g. a grouped invariant per
   `ai-docs/manuals/skill-authoring.md`'s "Grouped invariant lists" format) —
   branch scenario forces `On: setup` when `ai-docs/_review.local.md` is
   absent; range scenario runs on built-in review-substance defaults and never
   enters `On: setup` when absent; a present config's review-substance
   sections are honored by both scenarios.
2. Edit the `## On: invoke [branch?]` heading and its entry step to also
   accept a range invocation shape (e.g. `## On: invoke [branch?] [range:
   <base>..<head>?]`), with a new first sub-step that determines scenario kind
   from the caller's supplied argument (branch arg or default → branch
   scenario; `range:` arg → range scenario) before dispatching into the
   existing four H3 sub-blocks.
3. In `1. Load config`: branch the absent-config step by scenario kind per
   step 1 above — branch scenario keeps `go to On: setup`; range scenario adds
   a parallel branch that proceeds directly into review using the built-in
   Review Phases/Deep Review defaults already written in the `### Review
   Config Template` block (L78-84, L104-105), and when a config file *is*
   present, both scenarios load its review-substance sections (Review Phases,
   Checklist, Deep Review) while range ignores the collaboration/remote
   sections.
4. In `2. Identify branch`: scope this sub-block to the branch scenario only
   (skip for range — the caller-supplied `base`/`head` is the identified
   target, no branch discovery needed).
5. In `3. Prepare`: scope steps 1-3 (record current branch, fetch, checkout)
   to the branch scenario only. Decide and document whether step 4
   (`judge: has-blocked-paths`) still runs for the range scenario against the
   range diff (recommended: yes, since the judge is diff-content-based and the
   ticket says diff-content-agnostic machinery stays as-is) — if applied to
   range, reword the step to not assume a checkout has happened.
6. In `4. Review` step 1: branch the diff call by scenario — branch scenario
   keeps `git.diff(mode: "stat")` unchanged; range scenario calls
   `{{.McpNamespace}}/git.diff(range: "<base>..<head>", mode: "stat")` and use
   `{{.McpNamespace}}/git.log(range: "<base>..<head>")` for commit
   enumeration (needed by `judge: follows-ws-workflow`'s commit-log
   inspection, which stays otherwise unchanged). Steps 2-6 stay as written —
   confirm the judges under `## Judgments` need no wording change, only the
   diff/commit source feeding into them differs.
7. Leave `## On: setup`, `## On: branch discovery`, `## On: verdict`, and
   `## Judgments` bodies unchanged in this phase (Phase 2 owns the landing
   lens / template additions); only add the minimum cross-references needed
   so the new range branch in step 3 above can name where its built-in
   defaults come from.
8. Update `ai-docs/spec/workflow-skills.md` around L1124-L1151 (anchor
   `#260513-review-workflow-skill`) to describe the range scenario and the
   scenario-scoped config-load rule, matching the ticket's "Spec Impact"
   wording; keep the anchor id unchanged (no heading text rename).
9. Run `ai-docs/manuals/skill-authoring.md`'s **On: Fresh-Reader Audit** on
   `agents-plugin/rsrc/lead-review/lead-review.md` after edits (required by
   that manual's "After edits" rule for doctrine/routing/layout edits), and
   the **On: Downstream Consistency Sweep** since this is a shared rsrc
   playbook edit — its step 2 requires reading
   `ai-docs/manuals/wsflow-mirroring.md` first (already done for this plan)
   and following the regen steps below.
10. Regenerate generated artifacts per `ai-docs/manuals/wsflow-mirroring.md`
    "Rsrc Tree Provisioning" → "After-edit checklist" (both steps, in order):
    - `cd agents-plugin-tool && WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
    - `cd agents-plugin-tool && WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
    Do not hand-edit `agents-plugin-wsflow/rsrc/lead-review/lead-review.md` —
    it must come out byte-identical to the canonical file via this regen.

## Verification Plan

- `cd agents-plugin-tool && go test ./internal/mcp/... -run TestPlaybookPrintGoldenLeadReview` —
  confirms the doctrine substring still resolves and no continuity-tip leak,
  after the Invariants/On:invoke edits.
- `cd agents-plugin-tool && go test ./internal/wsrsrc/...` — confirms
  `TestShippedManifestUpToDate` and `TestWsflowRsrcMirrorUpToDate` (and any
  sibling drift tests) are green after the regen steps in Implementation Plan
  step 10; run before regen once to see them fail (expected red), then again
  after regen to confirm green.
- `python3 -m unittest discover agents-plugin-wsflow/tests` — confirms the
  wsflow distributed skill bundle checks (thin-shim shape, forbidden
  references, shared playbook stem coverage) still pass; `lead-review`'s
  SKILL.md is untouched so this should be unaffected, but the manual requires
  running it whenever a shared lead playbook changes.
- Manual-only (no automated runtime harness for playbook prose): walk the
  edited `On: invoke` handler by hand for both scenarios against the ticket's
  stated verification boundary —
  1. Range scenario, no `ai-docs/_review.local.md` present → confirm the
     walked procedure never reaches `On: setup` and proceeds on the built-in
     Review Phases/Deep Review text.
  2. Branch scenario, no config present → confirm the walked procedure still
     reaches `On: setup` (unchanged from today).
  3. Either scenario with a config file present → confirm review-substance
     sections (Review Phases, Checklist, Deep Review) are read from it.
  4. A large range diff → confirm `judge: is-large-diff` still fires from the
     range-scoped `git.diff`/`git.log` output.
- `ws/spec_index_verify` (or `wsflow/spec_index_verify`) after the
  `ai-docs/spec/workflow-skills.md` edit, to confirm the anchor
  `#260513-review-workflow-skill` stays indexed and well-formed.

## Escalations

- None.
