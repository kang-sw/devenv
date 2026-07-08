# Plan: 260707-feat-drain-goal-branch-staging — Phase 1: Add policy.branch.merge_confirm to enter.implement

## Relevant Ticket Contract
- Add the caller-suppliable `policy.branch.merge_confirm` fact to
  `enter.implement` (`implement_resolver.go` and related input parsing),
  defaulting to today's unconditional ask-before-merge behavior when
  absent/unset, and skipping the ask only when the caller explicitly passes
  `skip`.
- Update `lead-implement`'s Branch invariant text ("Wait for user approval
  before merge or another implementation slice") to describe the new
  caller-opt-out condition without introducing any goal-specific language.
- Add or update resolver tests: default (absent) still stops for approval;
  explicit `skip` proceeds without asking; explicit non-`skip` value (or
  anything else) still asks.
- Decision 6: `policy.branch.merge_confirm` is a plain policy fact, shaped
  analogously to `policy.branch.allow_rename` — `lead-implement` stays
  goal-unaware and only consumes the fact its caller supplies.
- Spec Impact: extends existing documented `enter.implement` policy-fact
  behavior in the same shape as `allow_rename` (no new contract kind);
  `ai-docs/spec/mcp-tools.md`'s `enter.implement` entry needs the same kind of
  one-clause addition the `allow_rename` default got.

## Out of Scope
- Phase 2 (`lead-drain-ready-queue` goal-awareness, `goal/<slug>` branch
  creation/checkout, final merge into `main`) — not touched in this phase.
- Any actual "ask vs. skip" enforcement logic beyond exposing the normalized
  fact for playbook prose to read — the current ask-before-merge behavior is
  pure `lead-implement.md` prose today (verified: no Go code implements a
  stop/ask branch action tied to merge confirmation; `deriveImplementBranchPlan`
  only has `create`/`rename`/`continue`/`stop` actions gated by branch-safety
  concerns, not by merge confirmation).
- `260703-chore-implement-branch-rename-default-allow`'s own scope (already
  landed; used here only as the structural precedent).

## Codebase Findings
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L65-68` —
  `implementBranchPolicyInput` struct holds `MergeTarget factString` and
  `AllowRename factString`; add `MergeConfirm factString
  \`json:"merge_confirm,omitempty"\`` alongside them.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L144-167` —
  `normalizedImplementFacts` struct holds `MergeTargetPolicy string` and
  `AllowRename string`; add `MergeConfirmPolicy string`.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L370-376` —
  `parseImplementPolicy`'s `branch` block parses `merge_target` via
  `parseObjectString` and `allow_rename` via `parseEnumFact(gm, "allow_rename",
  []string{"yes", "no", "unknown"})`. Add: `out.Branch.MergeConfirm, err =
  parseEnumFact(gm, "merge_confirm", []string{"skip", "ask", "unknown"})`
  (mirrors the allow_rename enum-fact call exactly; `parseEnumFact` in
  `proceed_resolver.go#L282-297` errors on values outside the allowed set, so
  "anything else" in the ticket's test bullet means any allowed-but-non-skip
  value, i.e. `ask`/`unknown`/absent — all fall through to "ask" behavior).
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L483-512` —
  `normalizeImplementFacts` builds `n` via `factOr(...)` calls; `AllowRename:
  factOr(policy.Branch.AllowRename, "yes")` is the fallback-default pattern.
  Add `MergeConfirmPolicy: factOr(policy.Branch.MergeConfirm, "ask")` (default
  is `"ask"`, not `"yes"`, since this fact's semantics are skip/ask not
  yes/no — pick whichever token reads clearest; `"ask"` matches the enum
  member used for the explicit non-skip case).
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L110-118,614-621` —
  `implementBranchPlan` struct and `deriveImplementBranchPlan`'s initial `plan
  := implementBranchPlan{...}` construction. `MergeConfirm` is not part of
  branch-safety derivation (it never changes `Action`), so add a
  `MergeConfirm string \`json:"merge_confirm,omitempty"\`` field to
  `implementBranchPlan` and set it once, unconditionally, in the initial
  `plan := implementBranchPlan{...}` literal (same treatment as the initial
  `MergeTarget: n.MergeTargetPolicy` assignment on that same struct literal) so
  it survives every early-return branch (`create`/`stop`/`continue`/`rename`).
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L711-724` —
  `renderImplementRaw` prints `Merge Target: %s` right after the `Branch
  Action` line; add a `Merge Confirm: %s` line directly after it using
  `v.BranchPlan.MergeConfirm` so the raw text output surfaces the fact for the
  calling lead to read.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L741-745` —
  `renderImplementRaw`'s Agenda block prints `- merge_target: %s`; add `-
  merge_confirm: %s` right after it, reading `result.Agenda.BranchPlan.MergeConfirm`
  (already present via the `BranchPlan` field added above — no separate Agenda
  struct change needed).
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L681-704` —
  `implementConditions` builds a flat condition list (`"review-override=" +
  n.ReviewOverride`, etc.) consumed by both JSON `Conditions` and the raw
  Conditions block. Add `"merge-confirm=" + n.MergeConfirmPolicy` to this list
  for parity with how other policy facts are surfaced as conditions.
- `agents-plugin-tool/internal/mcp/server.go#L2914-2918` — the `policy.branch`
  JSON-schema object declares `merge_target` and `allow_rename` via
  `nullableStringProperty`/`nullableEnumStringProperty`. Add: `"merge_confirm":
  nullableEnumStringProperty("Whether lead-implement may skip the
  ask-before-merge confirmation for this merge (defaults to ask).",
  []string{"skip", "ask", "unknown"})`.
- `ai-docs/spec/mcp-tools.md#L233-277` — the `enter.implement` spec entry
  documents `policy.branch.allow_rename`'s default-allowed posture inline
  ("branch rename defaults to allowed unless the caller explicitly withholds
  consent (`policy.branch.allow_rename: no`)"). Add an equivalent clause for
  `merge_confirm`, e.g. "...and whether the caller's own merge-approval ask may
  be skipped for this merge; merge confirmation defaults to asking unless the
  caller explicitly passes `policy.branch.merge_confirm: skip`."
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L15-17` — the Branch
  invariant block: `Wait for user approval before merge or another
  implementation slice.` / `Merge commits follow repository commit rules and
  include \`## AI Context\`.` Update the first line to describe the
  caller-opt-out without goal language, e.g.: "Wait for user approval before
  merge or another implementation slice, unless the resolved verdict's merge
  confirm is `skip`, in which case proceed with that merge without asking."
  This file is the canonical source; `agents-plugin-wsflow/rsrc/lead-implement/lead-implement.md`
  is a byte-identical generated mirror per `ai-docs/ref/wsflow-mirroring.md`
  (`lead-implement` is in the shipped wsflow skill set) and must be
  regenerated, not hand-edited, along with both `manifest.json` files —
  precedent: commit `5f442651` did this same regen dance for the allow_rename
  prose change (`WSRSRC_REGEN` / `WS_REGEN_WSFLOW_RSRC`, `-count=1`).
- `agents-plugin-tool/internal/mcp/implement_resolver_test.go#L151-258` —
  precedent tests for `AllowRename`: `TestResolveImplementBranchStopOmitsPlannerInstructions`
  (explicit `no` still reachable), `TestResolveImplementBranchRenameDefaultsToAllowedWhenUnset`
  (absent defaults to allowed). Mirror this shape for `MergeConfirm` by
  asserting on `result.Verdict.BranchPlan.MergeConfirm` (or
  `result.Agenda.BranchPlan.MergeConfirm`) directly, since there is no branch
  `Action` outcome tied to this fact (unlike `AllowRename`, which does gate
  `Action`).
- `agents-plugin-tool/internal/mcp/implement_resolver_test.go#L260-330ish` —
  `TestResolveImplementBranchPlanRules` table-drives `deriveImplementBranchPlan`
  directly against `normalizedImplementFacts` literals; adding a
  `MergeConfirmPolicy` field there requires no case changes since existing
  cases will just carry the Go zero value `""` for that field (harmless —
  `MergeConfirm` is a pure passthrough, not read by `deriveImplementBranchPlan`'s
  action logic), but a small dedicated test asserting `plan.MergeConfirm ==
  n.MergeConfirmPolicy` verbatim (regardless of branch action taken) is worth
  adding since this is the only place independently exercising
  `deriveImplementBranchPlan`.

## Implementation Plan
1. `implement_resolver.go`: add `MergeConfirm factString
   \`json:"merge_confirm,omitempty"\`` to `implementBranchPolicyInput`
   (near L67).
2. `implement_resolver.go`: add `MergeConfirmPolicy string` to
   `normalizedImplementFacts` (near L165).
3. `implement_resolver.go`: add `MergeConfirm string
   \`json:"merge_confirm,omitempty"\`` to `implementBranchPlan` (near L114).
4. `implement_resolver.go` `parseImplementPolicy`: parse
   `out.Branch.MergeConfirm` via `parseEnumFact(gm, "merge_confirm",
   []string{"skip", "ask", "unknown"})`, mirroring the `allow_rename` call
   immediately above it (near L373-375).
5. `implement_resolver.go` `normalizeImplementFacts`: add
   `MergeConfirmPolicy: factOr(policy.Branch.MergeConfirm, "ask")` to the `n :=
   normalizedImplementFacts{...}` literal, mirroring the `AllowRename` line
   (near L510).
6. `implement_resolver.go` `deriveImplementBranchPlan`: set `MergeConfirm:
   n.MergeConfirmPolicy` in the initial `plan := implementBranchPlan{...}`
   literal (near L616-621) so it is present on every returned plan regardless
   of `Action`.
7. `implement_resolver.go` `implementConditions`: append `"merge-confirm=" +
   n.MergeConfirmPolicy` to the conditions slice (near L699).
8. `implement_resolver.go` `renderImplementRaw`: add `fmt.Fprintf(&b, "Merge
   Confirm: %s\n", firstNonEmpty(v.BranchPlan.MergeConfirm, "ask"))` right
   after the existing `Merge Target:` line (near L721), and add
   `fmt.Fprintf(&b, "- merge_confirm: %s\n",
   firstNonEmpty(result.Agenda.BranchPlan.MergeConfirm, "ask"))` right after
   the existing `- merge_target:` Agenda line (near L745).
9. `server.go`: add the `merge_confirm` schema property to the `policy.branch`
   object next to `allow_rename` (near L2918).
10. `ai-docs/spec/mcp-tools.md`: extend the `enter.implement` bullet
    (L233-277 range) with a clause documenting `merge_confirm`'s default-ask
    posture, following the existing `allow_rename` default-allow clause's
    phrasing pattern.
11. `agents-plugin/rsrc/lead-implement/lead-implement.md`: update the Branch
    invariant's first bullet to describe the caller-opt-out, without any
    "goal" wording (line ~16).
12. Regenerate the wsflow mirror and both manifests per
    `ai-docs/ref/wsflow-mirroring.md` and the precedent in commit `5f442651`,
    from `agents-plugin-tool/`:
    - `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -run TestGenerateRealManifest -v`
      (regenerates `agents-plugin/rsrc/manifest.json`)
    - `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
      (regenerates `agents-plugin-wsflow/rsrc/lead-implement/lead-implement.md`
      and `agents-plugin-wsflow/rsrc/manifest.json`)
    (source: `agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go#L895-900` and
    `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L78-90`).
13. Tests (`implement_resolver_test.go`): add
    `TestResolveImplementMergeConfirmDefaultsToAskWhenUnset` (absent →
    `MergeConfirm == "ask"`), `TestResolveImplementMergeConfirmSkipHonored`
    (explicit `skip` → `MergeConfirm == "skip"`), and
    `TestResolveImplementMergeConfirmNonSkipStillAsks` (explicit `ask` →
    `MergeConfirm == "ask"`), asserting on
    `result.Verdict.BranchPlan.MergeConfirm` (or equivalently
    `result.Agenda.BranchPlan.MergeConfirm`); reuse the existing
    `TestResolveImplementBranchRenameDefaultsToAllowedWhenUnset`-style input
    shape (ticket target + minimal scope facts + `Policy.Branch` literal).
    Optionally extend `TestResolveImplementBranchPlanRules`'s table with a case
    asserting `MergeConfirm` passthrough independent of `Action`, per the
    Codebase Findings note.

## Verification Plan
- `go test ./agents-plugin-tool/internal/mcp/... -run TestResolveImplement -count=1`
  (or the package's full `go test ./... -count=1` if the repo's convention is
  full-package runs) to exercise the new and existing resolver tests.
- Manual/spec check: confirm `ai-docs/spec/mcp-tools.md`'s updated
  `enter.implement` bullet reads consistently with the `allow_rename` clause
  it sits beside.
- Confirm wsflow mirror regen leaves `git diff` showing only the expected
  mirrored file + manifest hash changes (no unrelated drift), matching commit
  `5f442651`'s diff shape.

## Escalations
- None.
