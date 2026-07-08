# Plan: 260707-feat-impl-branch-convention-autodelete — Phase 1: Rename branch convention and add naming-gated auto-delete

## Relevant Ticket Contract

- Rename branch-creation convention from `implement/<scope-slug>` to
  `impl/<stem>`, with `<stem>` capped at a maximum of 15 characters.
- Branch Cleanup (lead-implement step 8) gains a naming precondition: if the
  branch name matches `impl/*` **and** all existing structural guardrails
  (strict-ancestor, not checked out, not worktree-linked, merge target not
  ambiguous, no commits unreachable from merge target) already pass, delete
  without asking. Non-`impl/*` branches (including legacy `implement/*`) keep
  today's ask-first flow verbatim.
- `deriveImplementBranchPlan`'s "already on an implementation branch" check
  must accept **either** `implement/` or `impl/` prefix so an in-progress
  legacy branch is still `continue`/`rename`-eligible, not misidentified as
  fresh-start (`create`).
- Out of scope: no change to guardrail logic itself; no change to
  fresh-branch creation logic beyond the naming convention.
- Deferred/settled by this plan: truncation scheme for stems over 15 chars —
  hard-truncate the slug to 15 characters and trim a trailing `-` (simple,
  pragmatic; collisions are already caught by the existing `TargetExists`
  guardrail, which still applies unchanged).

## Out of Scope

- `260707-feat-drain-goal-branch-staging` (dependent ticket) — not touched.
- Guardrail predicate logic (ancestor/checked-out/worktree/ambiguous-target/
  unreachable-commits checks) — unchanged, reused as-is.
- Retroactive renaming of in-flight `implement/*` branches — none performed.
- Spec doc (`ai-docs/spec/mcp-tools.md`) — updated later in lead-implement's
  own Documentation step (step 5, `lead-update-spec` playbook), not part of
  this code-focused survey plan.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/implement_resolver.go#L600-L613` — `deriveImplementBranchPlan` builds `targetBranch := "implement/" + n.ScopeSlug` and gates "already on an implementation branch" via `strings.HasPrefix(obs.CurrentBranch, "implement/")`. Both need updating: target construction → `impl/<stem-capped-at-15>`; prefix check → accept `implement/` OR `impl/`.
- `agents-plugin-tool/internal/mcp/session_state.go#L969-L986` — `handleEnterImplement` independently rebuilds `targetBranch := "implement/" + normalized.ScopeSlug` (duplicated logic) before calling `observeImplementBranch` to check `TargetExists`. **Risk signal**: this duplicate construction must stay byte-identical to `deriveImplementBranchPlan`'s target-branch naming or `TargetExists` will check the wrong branch name and silently break the `stop`-on-exists guardrail. Recommend extracting a single helper (e.g. `implementTargetBranchName(scopeSlug string) string`) in `implement_resolver.go` and calling it from both sites instead of duplicating the truncation logic twice.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L750-L771` — `slugifyImplementScope` already produces the lowercase-dash-normalized scope slug; the new 15-char stem cap should apply to this slug's output (truncate after slugify, then trim trailing `-`), not change slugify itself.
- `agents-plugin-tool/internal/mcp/implement_resolver_test.go#L240-L303` — `TestResolveImplementBranchPlanRules` table-drives `deriveImplementBranchPlan` directly with `normalizedImplementFacts{ScopeSlug: ...}` and asserts on `implementBranchPlan.Action`/`.Reason`; extend this table (or add a sibling test) for: legacy `implement/*` current branch still recognized (continue/rename, not create); new `impl/*` target branch naming with truncation.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L1748` — existing test does `runGit(t, root, "switch", "-c", "implement/old-scope")`; useful pattern for a new test asserting legacy-branch continuation still resolves correctly post-rename, and for asserting `impl/*` new-branch construction end-to-end through `enter.implement`.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L48` — prose: "Set `policy.branch.merge_target` only when already on `implement/*` or the user names it." Needs updating to describe the current-branch check as accepting either prefix (or use a prefix-agnostic phrase like "already on an implementation branch").
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L87-L96` — Branch Cleanup step 8 is the ask-first flow to be split: keep guardrail check (steps 1-2) as-is, add naming gate — `impl/*` + guardrails clear ⇒ delete without asking; otherwise (non-`impl/*`, including legacy `implement/*`, or any guardrail fails) ⇒ keep current ask-then-delete-on-approval flow. Step 5 (report retained branches with skip reason) still applies to the non-auto-delete path.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1757-L1872` — golden-render tests (`TestPlaybookPrintGoldenLeadImplement` and a wsflow-render counterpart) assert on exact substrings of the rendered `lead-implement` playbook body for both the canonical (`agents-plugin`) and wsflow-mirrored render paths; updating step 8 prose will require adding/adjusting `want`/`forbidden` substring assertions here.
- **Mirroring constraint (no separate edit needed, but a required regen step)**: `agents-plugin-wsflow/rsrc/lead-implement/lead-implement.md` is a byte-identical generated mirror of `agents-plugin/rsrc/lead-implement/lead-implement.md`, guarded by `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L47-L114` (`TestWsflowRsrcMirrorUpToDate`). Only edit the canonical file under `agents-plugin/rsrc/`; regenerate the mirror via `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -run TestRegenerateWsflowRsrcMirror`. Do not hand-edit the wsflow copy.
- **Manifest hash constraint**: `agents-plugin/rsrc/manifest.json#L22` and `agents-plugin-wsflow/rsrc/manifest.json#L22` pin a sha256 for `lead-implement/lead-implement.md`; guarded by `agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go#L23-L44` (`TestShippedManifestUpToDate`). After editing the canonical markdown, regenerate with `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest` (run the wsflow mirror regen first so both trees are in sync before the manifest hash is recomputed against `agents-plugin/rsrc`).
- `ai-docs/tickets/.done/260521-refactor-wsflow-lead-implement-mirroring-gap.md` — background on why the canonical/mirror split and manifest-hash guard exist; informative only, no action beyond the regen commands above.

## Implementation Plan

1. In `implement_resolver.go`, add a small helper (e.g. `implementTargetBranchName(scopeSlug string) string`) that returns `"impl/" + truncated-and-trimmed(scopeSlug, 15)`. Use it in `deriveImplementBranchPlan` (replacing the `"implement/" + n.ScopeSlug` line) and in `handleEnterImplement` in `session_state.go` (replacing its duplicate `"implement/" + normalized.ScopeSlug` line), so both target-branch constructions can never drift apart.
2. In `deriveImplementBranchPlan`, change the "already on an implementation branch" gate from `strings.HasPrefix(obs.CurrentBranch, "implement/")` to accept either prefix, e.g. `strings.HasPrefix(obs.CurrentBranch, "implement/") || strings.HasPrefix(obs.CurrentBranch, "impl/")`.
3. Update `agents-plugin/rsrc/lead-implement/lead-implement.md`:
   - Line 48 policy-rule prose to reflect the prefix-agnostic "already on an implementation branch" check (mention both `impl/*` as the new convention and `implement/*` as still-recognized legacy).
   - Step 8 (Branch Cleanup): after the existing ancestor check and skip-condition check, branch the flow — when the branch name matches `impl/*` and no skip condition held, delete without asking (`git branch -d <branch>`); when the branch does not match `impl/*` (including legacy `implement/*`) or any skip condition held, keep the current ask-first flow (ask → delete only on explicit approval → report retained branches with skip reason).
4. Regenerate the wsflow mirror (`WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`) and then the manifest (`WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`) from repo root `agents-plugin-tool`.
5. Update/extend tests:
   - `implement_resolver_test.go`: extend `TestResolveImplementBranchPlanRules` (or add new cases) covering: `impl/<stem>` construction with 15-char truncation; legacy `implement/*` current branch still resolves `continue`/`rename` (not `create`); new `create` targets use `impl/` prefix.
   - `session_state_test.go`: add/adjust a case mirroring the `implement/old-scope` pattern (`~L1748`) for an `impl/*`-prefixed pre-existing branch, and verify `TargetExists`/branch-plan derivation is consistent with the new helper.
   - `playbook_tools_test.go`: update the golden `want`/`forbidden` substring lists in `TestPlaybookPrintGoldenLeadImplement` (and the wsflow-render counterpart) to match the new Branch Cleanup step 8 prose and the updated policy-rule line.

## Verification Plan

- `cd agents-plugin-tool && go test ./internal/mcp/... ./internal/wsrsrc/...`
- Manual: read the regenerated `agents-plugin/rsrc/lead-implement/lead-implement.md` and confirm step 8 reads correctly for both the auto-delete and ask-first branches; confirm `agents-plugin-wsflow/rsrc/lead-implement/lead-implement.md` is byte-identical afterward.
- No user-facing runtime surface beyond the MCP tool and printed playbook text; the Go test suite plus a manual read of the regenerated playbook is the full verification boundary for this phase.

## Escalations

- None.
