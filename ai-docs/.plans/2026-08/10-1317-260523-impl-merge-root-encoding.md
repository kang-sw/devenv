# Plan: 260523-bug-implement-merge-target-discovery — Phase 1: Encode impl/<merge-root>/<stem> in the resolver, convention, and mirrors

## Relevant Ticket Contract

- Chosen shape (settled): `impl/<merge-root>/<stem>`. `impl/` prefix kept;
  `<stem>` stays single-segment; parse rule is "strip `impl/`, split on the
  LAST `/`" — everything before is `<merge-root>` (may itself contain `/`),
  final segment is `<stem>`.
- **Resolved Decision (settled, no-behavior-change for rootless/legacy):** a
  branch with no merge-root segment — rootless `impl/<stem>` or any legacy
  `implement/<stem>` — keeps exactly today's behavior: `plan.MergeTarget` comes
  solely from caller `policy.branch.merge_target`; empty ⇒ stop-and-ask (never
  a silent `main`). This ticket's new encoding work applies only to (a) fresh
  `create` (merge-root = current branch) and (b) re-entry onto an
  already-name-rooted `impl/<root>/<stem>` branch (merge-root parsed from the
  branch name).
- **Name-root precedence (load-bearing):** on a name-rooted branch, the
  name-derived `<merge-root>` is authoritative. A diverging caller
  `merge_target` must never be silently honored — reconcile to the name-root
  (with a warning) or stop-and-ask; ticket explicitly permits either choice.
- `ScopeSlug` must be sanitized/rejected of slashes at
  `implementTargetBranchName` build time (ticket names this exact function/line
  range) so the single-segment `<stem>` invariant the parser depends on can
  never be violated by a caller-supplied `target.scope_slug`.
- D/F ref conflict: on `create`, detect a legacy single-segment ref
  (`impl/<merge-root>`, or any ancestor path segment of the new nested target)
  that already exists as a branch and would block creating the nested target;
  non-destructive warn/stop (no auto-delete), exercised by a test.
- Verification boundary (from ticket): non-`main` merge root encodes correctly
  on create; re-entry re-derives root from the branch **name**, not the
  current-branch heuristic; a divergent caller `merge_target` on a name-rooted
  branch does not silently win; rootless/legacy still stops-and-asks with no
  caller target; slashed `ScopeSlug` is sanitized; D/F path is warn/stop and
  tested.
- Coupling to update: impl branch-name test literals, `spec/mcp-tools.md`,
  `spec/workflow-skills.md`, `mental-model/workflow-skills.md`, the two
  goal-staging/fan-out anchors, and the mirrored `agents-plugin/` +
  `agents-plugin-wsflow/` trees.

## Out of Scope

- The original `## Proposed Direction` (generic merge-base-discovery API) is
  explicitly superseded — do not build it.
- Any change to `lead-implement.md`'s `impl/*` autodelete glob text
  (`agents-plugin/rsrc/lead-implement/lead-implement.md:96-97`, mirrored
  byte-identical in `agents-plugin-wsflow/`) — prose `impl/*` already reads as
  "anything under the `impl/` prefix" and needs no edit; confirmed the two
  copies are identical via `diff`.
- `goal/<parent>/<slug>`'s own single-segment fallback-to-`main` behavior
  (`spec/workflow-skills.md:563`, `mental-model/workflow-skills.md:73`) is a
  **different, already-shipped convention** for a different branch namespace —
  do not port its main-fallback into the impl resolver; the two conventions
  intentionally diverge (goal falls back to `main`, impl stops-and-asks).
- Any change to `implementBranchPlan`/`implementAgenda` JSON field names or the
  `enter.implement` public argument shape — only the derived branch-name
  string and internal resolver logic change.
- CHANGELOG.md and historical `.plans/` / `.dropped`/`todo/` ticket mentions of
  `impl/<stem>` — not caller-visible spec/behavior surfaces, out of scope for
  this phase.
- The git-level integration test for the D/F conflict path (real repo,
  `observeImplementBranch`) — a pure unit test on `deriveImplementBranchPlan`
  satisfies the ticket's "exercised by a test" requirement; add the git-level
  test only if the reviewer asks for stronger coverage.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/implement_resolver.go#L672-680` —
  `implementTargetBranchName(scopeSlug string) string` is the single shared
  branch-name builder; only two non-test call sites exist in the whole repo
  (verified via grep): here and `session_state.go:1020`. Must become
  `implementTargetBranchName(mergeRoot, scopeSlug string) string`: sanitize
  `scopeSlug` (strip/replace `/`), trim trailing `-` (existing behavior), then
  return `"impl/" + stem` when `mergeRoot == ""` (preserves the legacy/rootless
  shape byte-for-byte) or `"impl/" + mergeRoot + "/" + stem` otherwise.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L682-730` —
  `deriveImplementBranchPlan(n, obs)` is pure (no git access), which is why its
  existing tests fabricate `implementBranchObservation` directly. Restructure:
  1. Not `impl/`- or `implement/`-prefixed current branch → **create**:
     `mergeRoot = obs.CurrentBranch` (may itself contain `/`, e.g.
     `ws-dashboard-dev` or `feature/foo` — both fine, split-on-last-slash still
     unambiguous). `targetBranch = implementTargetBranchName(mergeRoot, n.ScopeSlug)`.
     If `obs.MergeRootRefConflict != ""` (new field, see below), action =
     `"stop"`, reason names the conflicting ref — do not fall through to
     `git branch` creation.
  2. `impl/`-prefixed current branch that parses a root (see new helper below)
     → **name-rooted**: `mergeRoot` = parsed root (name-authoritative). If
     `n.MergeTargetPolicy != ""` and differs from `mergeRoot`, reconcile:
     `plan.MergeTarget = mergeRoot` (override, never the diverging caller
     value) and append a warning, e.g. `policy.branch.merge_target %q ignored
     (implementation branch name encodes merge root %q)`. If empty or equal,
     `plan.MergeTarget = mergeRoot` silently.
     `targetBranch = implementTargetBranchName(mergeRoot, n.ScopeSlug)` (same
     root, requested stem) — feeds the existing continue/rename/stop
     comparison logic unchanged.
  3. Rootless `impl/<stem>` or any `implement/<stem>` → **unchanged legacy
     path**: `plan.MergeTarget = n.MergeTargetPolicy` exactly as today (empty ⇒
     existing stop-and-ask gate at `L702-706`), `targetBranch =
     implementTargetBranchName("", n.ScopeSlug)` (byte-identical legacy shape).
  This 3-way split is exactly what the Resolved Decision requires: rootless
  behavior is provably untouched (verified against existing pure-unit tests
  below), only create/name-rooted branches change.
- **New helper needed** (no existing Go implementation to reuse — confirmed
  via repo-wide grep: the `goal/<parent>/<slug>` split-on-last-slash parse
  cited as prior art in the ticket exists only as skill prose in
  `spec/workflow-skills.md`/`mental-model/workflow-skills.md`, never as Go
  code): `parseImplBranchRoot(branch string) (root, stem string, ok bool)` —
  `ok` only when `branch` has an `impl/` prefix **and** the remainder contains
  at least one `/` (root present). Strip `impl/`, split on the LAST `/`.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L138-145` —
  `implementBranchObservation` struct: add `MergeRootRefConflict string`
  (empty = no conflict; otherwise the blocking existing ref name). Populated
  only by `observeImplementBranch`, read only by `deriveImplementBranchPlan`'s
  create branch. Existing tests construct this struct with named fields, so
  adding a field is additive/non-breaking.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L414-433` —
  `observeImplementBranch(root, targetBranch)` already does the `TargetExists`
  ref-check via `(wsgit.ExecRunner{}).RunGit(ctx, root, "rev-parse",
  "--verify", "--quiet", "refs/heads/"+targetBranch)`. Add a second check in
  the same `if targetBranch != ""` block: split `targetBranch` into path
  segments, and for each **strict-prefix ancestor path** (e.g. for
  `impl/ws-dashboard-dev/foo`, check `impl/ws-dashboard-dev`; do not check the
  full target itself, that's `TargetExists`'s job), run the same
  rev-parse-verify pattern; the first existing ref found is the D/F conflict,
  stored in `obs.MergeRootRefConflict`. Only exercised for `create` (only path
  that mints a brand-new nested ref: name-rooted continue/rename reuse an
  existing root, rootless/legacy stays unrooted).
- `agents-plugin-tool/internal/mcp/session_state.go#L1004-1024` — the
  `enter.implement` preflight (`handleEnterImplement`) computes `targetBranch
  := implementTargetBranchName(normalized.ScopeSlug)` **before**
  `observeImplementBranch` (which is the only call that fetches
  `status.Branch.Head`, i.e. the current branch) — a real ordering bug for the
  new scheme, since building the correct `targetBranch` now requires knowing
  the current branch first. Fix by calling `observeImplementBranch(record.Root,
  "")` once first (empty `targetBranch` short-circuits the
  `TargetExists`/D-F block, so this is a cheap peek that only returns
  `CurrentBranch`/`StartCommit`/etc.), deriving `mergeRoot` from that
  `CurrentBranch` with the same 3-way rule as `deriveImplementBranchPlan`
  (fresh branch → current branch; name-rooted → `parseImplBranchRoot`;
  rootless/legacy → `""`), building `targetBranch` via the updated
  `implementTargetBranchName(mergeRoot, normalized.ScopeSlug)`, then calling
  `observeImplementBranch(record.Root, targetBranch)` again for the real
  observation used by `resolveImplement`. Two git-status round trips instead of
  one; simplest correct fix, no signature change to `observeImplementBranch`.
- `agents-plugin-tool/internal/mcp/implement_resolver_test.go#L553-652`
  (`TestResolveImplementBranchPlanRules`) — **only 3 of 10 cases use a
  non-`impl/`/`implement/`-prefixed `CurrentBranch` (the `create` cases)** and
  need their `wantTargetBranch` literal updated:
  - `"create outside implement branch"` (L563-570,
    `CurrentBranch: "feature/base"`): `impl/target` → `impl/feature/base/target`.
  - `"target branch name is not truncated..."` (L621-628, same
    `CurrentBranch`): `impl/a-very-long-scope-slug-name` →
    `impl/feature/base/a-very-long-scope-slug-name`.
  - `"target branch name trims a trailing dash..."` (L629-636, same
    `CurrentBranch`): `impl/abc-defghijklm-nop` → `impl/feature/base/abc-defghijklm-nop`.
  **The other 7 cases all use a rootless `CurrentBranch` (`impl/target`,
  `impl/old`, `implement/old`)** — confirmed via inspection, these are
  provably unaffected by the 3-way split (they land in the unchanged legacy
  branch) and need no literal changes; verify with `go test` rather than
  hand-editing them.
- `agents-plugin-tool/internal/mcp/implement_resolver_test.go#L489-551`
  (`TestResolveImplementMergeTargetPolicyIgnoredOutsideImplementBranchWarns` /
  `...HonoredOnImplementBranchNoWarning`) — first uses `CurrentBranch:
  "test/wsflow-smoke"` (create path; only asserts `MergeTarget`, not
  `TargetBranch`, so no literal change needed, but re-verify it still passes).
  Second uses `CurrentBranch: "impl/tiny-edit"` — rootless, unaffected.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L1855-1932`
  (`TestEnterImplementNewSchemaReturnsVerdictAndStoresAgenda`) — calls
  `initGit(t, root)` with **no explicit branch checkout**, so today's assertion
  `"Branch Action: create impl/enter-implement"` / `"Next: Create
  impl/enter-implement"` (L1868, L1871) implicitly relies on whatever the
  test-machine's default unborn-branch name is. Under the new scheme the
  branch name is now baked into the assertion, so this must become
  deterministic: add an explicit `runGit(t, root, "switch", "-c",
  "feature/base")` right after `initGit` (mirrors the existing
  pre-commit-checkout precedent at `session_state_test.go:2226`,
  `runGit(t, root, "switch", "-c", "implement/old-scope")` with no prior
  commit — confirmed `git switch -c` works on an unborn HEAD), then update both
  literals to `impl/feature/base/enter-implement`.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L2265-2301`
  (`TestEnterImplementNewImplPrefixBranchTargetExists`) — final `CurrentBranch`
  is `impl/old-scope`, which is **rootless** (single segment after strip) —
  confirmed this test needs **no changes**; it already exercises the
  `TargetExists` shared-helper contract this ticket's D/F check extends
  alongside, and stays in the legacy/rootless bucket untouched by Phase 1.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L220` — a fabricated
  `implementBranchPlan{...TargetBranch: "impl/demo"...}` literal used for
  todo-derivation testing (`deriveImplementTodosFromVerdict`), independent of
  the naming resolver — no change needed.
- `ai-docs/spec/mcp-tools.md#L316-319` — "Fresh implementation branches are
  created under the `impl/<stem>` convention..." needs the merge-root segment
  added: state the shape as `impl/<merge-root>/<stem>`, merge-root = current
  branch on create or the parsed branch-name root on re-entry, `<stem>` keeps
  the `<=15` recommendation and trailing-`-` trim.
- `ai-docs/spec/workflow-skills.md#L554-558` (`260707-drain-goal-branch-staging`
  anchor) and `#L601` (`lead-goal-fan-out-step` prose) — both state "own
  `impl/<stem>` branch merged into `goal/<parent>/<slug>`"; since the create
  path derives merge-root from the checked-out branch automatically (no skill
  change needed, this is pure resolver behavior), these become `impl/<goal-
  branch>/<stem>` — update prose only, no skill-logic change.
- `ai-docs/spec/workflow-skills.md#L940-950` (`260707-implement-branch-cleanup-
  naming-gate` anchor) — "a branch named `impl/<stem>` ... is deleted without
  asking" should read `impl/<merge-root>/<stem>` (or generalized "any `impl/`-
  prefixed branch") to stay accurate; the actual autodelete gate text in
  `lead-implement.md` already reads as a prefix match and needs no code/prompt
  change (see Out of Scope).
- `ai-docs/mental-model/workflow-skills.md#L48` and `#L73` — same two
  `impl/<stem>` mentions as the spec anchors above; update in lockstep.
- `agents-plugin/rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md` and its
  byte-identical mirror `agents-plugin-wsflow/rsrc/lead-goal-fan-out-step/
  lead-goal-fan-out-step.md` (confirmed identical via `diff`, exit 0) — lines
  13, 16, 19, 28 all say `impl/<stem>` describing per-worker branches created
  inside a goal worktree; update both copies in lockstep to `impl/<parent>/
  <stem>` (or `impl/<goal-branch>/<stem>`) to match the new automatic encoding.

## Implementation Plan

1. `implement_resolver.go`: add `parseImplBranchRoot(branch string) (root,
   stem string, ok bool)` near `implementTargetBranchName`. `ok` is true only
   when `branch` has prefix `impl/` and the remainder (after stripping the
   prefix) contains at least one `/`; split on the LAST `/`.
2. `implement_resolver.go#L677-680`: change
   `implementTargetBranchName(scopeSlug string) string` to
   `implementTargetBranchName(mergeRoot, scopeSlug string) string`. Sanitize
   `scopeSlug` by replacing any `/` with `-` before the existing
   `strings.TrimRight(scopeSlug, "-")` trim. Return `"impl/" + stem` when
   `mergeRoot == ""`, else `"impl/" + mergeRoot + "/" + stem`.
3. `implement_resolver.go#L138-145`: add `MergeRootRefConflict string` to
   `implementBranchObservation`.
4. `implement_resolver.go#L682-730`: restructure `deriveImplementBranchPlan`
   into the 3-way split described in Codebase Findings (create /
   name-rooted / rootless-legacy), each branch computing its own `mergeRoot`
   and calling `implementTargetBranchName(mergeRoot, n.ScopeSlug)`, then
   falling into the existing continue/rename/stop comparison logic unchanged.
   Add the D/F stop check (`obs.MergeRootRefConflict != ""`) inside the create
   branch, before the existing "not an implementation branch" success path.
   Add the name-root-divergence reconcile-and-warn inside the name-rooted
   branch.
5. `implement_resolver.go#L414-433`: extend `observeImplementBranch` — inside
   the existing `if targetBranch != ""` block, after the `TargetExists` check,
   walk `targetBranch`'s `/`-separated ancestor path segments (excluding the
   full target) and rev-parse-verify each as `refs/heads/<segment-path>`; set
   `obs.MergeRootRefConflict` to the first hit.
6. `session_state.go#L1004-1024`: before building `targetBranch`, call
   `observeImplementBranch(record.Root, "")` to peek `CurrentBranch`, derive
   `mergeRoot` via the same 3-way rule (inline, or a tiny shared helper if it
   reads cleaner — reuse `parseImplBranchRoot` for the name-rooted case),
   build `targetBranch := implementTargetBranchName(mergeRoot,
   normalized.ScopeSlug)`, then call `observeImplementBranch(record.Root,
   targetBranch)` again for the observation passed to `resolveImplement`.
7. Update the 3 pure-unit `wantTargetBranch` literals in
   `implement_resolver_test.go` (`TestResolveImplementBranchPlanRules`, the 3
   `create`-path cases listed in Codebase Findings).
8. Add new pure-unit tests in `implement_resolver_test.go` (function name your
   choice, e.g. `TestResolveImplementMergeRootEncoding`):
   - Fresh create with a non-`main` current branch (`ws-dashboard-dev`) yields
     `TargetBranch = "impl/ws-dashboard-dev/<stem>"` and `MergeTarget =
     "ws-dashboard-dev"`.
   - Re-entry on `impl/ws-dashboard-dev/<stem>` (matching stem) yields action
     `continue`, `MergeTarget = "ws-dashboard-dev"` derived from the name, not
     from any current-branch heuristic.
   - Re-entry on `impl/ws-dashboard-dev/<stem>` with a diverging
     `n.MergeTargetPolicy = "main"` does not honor `"main"`: `MergeTarget`
     stays `"ws-dashboard-dev"` (or action becomes `stop` if you choose the
     stop-and-ask alternative — pick one and assert it) and a warning is
     present.
   - A slashed `ScopeSlug` (e.g. `"feature/evil"`) on create sanitizes to a
     single-segment stem in `TargetBranch` (no accidental extra `/`).
   - `obs.MergeRootRefConflict` set on a create-path input yields action
     `"stop"` with a reason naming the conflict.
9. `session_state_test.go#L1855-1932`
   (`TestEnterImplementNewSchemaReturnsVerdictAndStoresAgenda`): add
   `runGit(t, root, "switch", "-c", "feature/base")` after `initGit`; update
   the two `impl/enter-implement` literals to `impl/feature/base/enter-implement`.
10. Update spec/mental-model prose in lockstep at the 5 locations listed in
    Codebase Findings: `spec/mcp-tools.md#L316-319`,
    `spec/workflow-skills.md#L554-558`, `#L601`, `#L940-950`,
    `mental-model/workflow-skills.md#L48`, `#L73`.
11. Update both mirrored copies of `lead-goal-fan-out-step.md`
    (`agents-plugin/rsrc/...` and `agents-plugin-wsflow/rsrc/...`, currently
    byte-identical) at lines 13, 16, 19, 28: `impl/<stem>` →
    `impl/<parent>/<stem>` (or equivalent phrasing matching the new
    automatic-encoding behavior).

## Verification Plan

- `cd agents-plugin-tool && go test ./internal/mcp/... -run Implement -v` —
  covers `TestResolveImplement*`, `TestDeriveImplementBranchPlan*`, and
  `TestEnterImplement*`.
- `cd agents-plugin-tool && go test ./...` — full package suite, catch any
  other incidental breakage from the `implementTargetBranchName` signature
  change (grep already confirmed only the 2 non-test call sites, but tests may
  reference the old 1-arg signature indirectly through table-driven helpers).
- `diff agents-plugin/rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md agents-plugin-wsflow/rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md` —
  confirm the two mirrors stay byte-identical after edits.
- Manual read-through of the 5 updated spec/mental-model passages to confirm
  the `impl/<merge-root>/<stem>` shape and the "rootless/legacy still
  stops-and-asks" contract are both stated accurately and consistently.

## Escalations

- None.
