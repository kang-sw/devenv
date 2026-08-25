# Plan: 260825-feat-impl-branch-single-ticket-scope-merge-timing — Phase 1: relation-aware start gate — conservative safety stop over silent rename

## Relevant Ticket Contract

- Goal: entering `enter.implement` on an `impl/*` branch whose scope differs
  from the caller's target, while that branch has unmerged commits ahead of
  its merge root, resolves to `stop` with lead-routing guidance instead of a
  silent `rename`. Goal-branch seamless (`create`) and same-scope `continue`
  paths are unchanged.
- Gate the `rename` action on the current branch having **no** unmerged work
  ahead of its merge root; when unmerged work exists and scope mismatches,
  resolve to `stop` regardless of `allow_rename` (not overridable).
- Add the ahead-of-merge-root observation to the branch-observation fact set
  the MCP layer builds, alongside the existing upstream Ahead/Behind
  observation. This is a *new* observation, distinct from Ahead/Behind (which
  are measured against upstream tracking, not the merge root).
- Emit a stop `next_instruction` that routes identity resolution to the lead
  (L2 session context / L3 lead-dispatched explore), naming the suspected
  owning work by branch-name-encoded stem (`parseImplBranchRoot`) only — zero
  commit-content parsing — best-effort and non-load-bearing.
- Consume `target.ticket_stem` (already present in the input schema; no
  schema change) where it sharpens the guidance message; the safety decision
  itself must not depend on stem parsing.
- `allow_rename`'s meaning stays intact for the no-unmerged-work relabel case
  (renaming an empty/just-created branch stays a valid `rename`).
- Host-neutral: no `develop`/`main` hardcoding; merge root is derived from
  branch-name encoding / caller policy, never a literal topology name.
- Verification matrix (ticket Phase 1 Verification):
  (a) `impl/<root>/<A>` with unmerged commits, target scope `<B>` ⇒ `stop`
      even with `allow_rename: yes`.
  (b) same branch, no unmerged commits, target `<B>` ⇒ `rename` still
      allowed.
  (c) same-scope target ⇒ `continue`.
  (d) on a `goal/*` branch ⇒ `create` seamless path unchanged.
  (e) the stop instruction carries the lead-routing guidance.
- Spec Impact: update `ai-docs/spec/mcp-tools.md`'s `enter.implement`
  branch-plan/rename-default sentence (Phase 1 only; the `tickets.close`
  merge-review sentence is Phase 2).

## Out of Scope

- Phase 2 in full: the per-phase no-merge default (`session_state.go`'s
  implement final-action merge instruction), the `tickets.close` merge-review
  trigger, and the ticket-declared stop-gate exception. Do not touch
  `tickets.close` or the final-action gate logic.
- Mechanical commit-content parsing of any kind (rejected by ticket Decisions
  — the resolver only ever sees `parseImplBranchRoot`'s branch-name stem and
  `target.ticket_stem`).
- Any new MCP tool input schema field — `target.ticket_stem` is already
  accepted by `parseImplementTarget`; this phase only threads its existing
  value further into the resolver's internal fact set.
- L2 (lead-context relation) and L3 (lead explore fallback) implementations —
  those are lead-side prompt/skill behavior, not resolver code; only the
  resolver's stop-hint text needs to reference them.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/implement_resolver.go#L138-L146` —
  `implementBranchObservation` struct holds the MCP-layer-observed git facts
  (`Ahead`/`Behind` are upstream-tracking-based). Add a new
  `AheadOfMergeRoot int` field here for the merge-root-based count; this
  struct has no JSON tags (never marshaled directly), so the addition is
  purely internal.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L415-L442` —
  `observeImplementBranch(root, targetBranch string)` is where the MCP layer
  builds the branch-observation fact set (calls `client.Status` for
  `Ahead`/`Behind`, then does `rev-parse --verify` checks for `TargetExists`
  and `MergeRootRefConflict`). Add the ahead-of-merge-root computation here,
  placed alongside the existing `Ahead`/`Behind` assignment (right after the
  `obs := implementBranchObservation{...}` literal, before the
  `if targetBranch != ""` block): derive the merge root from
  `obs.CurrentBranch` via the already-existing `implementMergeRootFor`
  helper (`#L716-L732`, purely string-based, no git call), skip when the
  merge root is empty or equals the current branch itself (fresh/non-impl
  branch — no ahead-of-merge-root concept applies), otherwise compute the
  count via git.
- `agents-plugin-tool/internal/wsgit/git.go#L408-L423` — `Client.MergeBase(ctx,
  root, base, head) (MergeBaseResult, error)` is the existing git-exec helper
  to reuse for the merge-base half of the computation (already exposed as the
  `git.merge_base` MCP tool per `server.go#L913-L917`, so it is a proven,
  tested wrapper). For the "count of commits ahead" half, no existing helper
  exists; follow the same low-level pattern `observeImplementBranch` already
  uses at `#L429` / `#L435` — call `(wsgit.ExecRunner{}).RunGit(ctx, root,
  "rev-list", "--count", mergeBase+".."+currentBranch)` and parse the
  trimmed output as an int (needs a `strconv` import). On any error from
  either git call (unresolvable ref, unrelated histories), default the count
  to `0` (fail-open on the *count itself*, consistent with the existing
  `err == nil` truthy pattern the same function already uses for
  `TargetExists`/`MergeRootRefConflict` — an infra failure here is out of the
  ticket's test matrix, not a normal false-negative risk case).
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L716-L732` —
  `implementMergeRootFor(currentBranch)` already implements the exact 3-way
  merge-root derivation rule (own branch is its own root; name-rooted
  `impl/<root>/<stem>` yields the parsed root via `parseImplBranchRoot`;
  rootless `impl/<stem>`/`implement/<stem>` yields `""`) — reuse this
  directly rather than re-deriving merge-root logic. Its rootless-`""` case
  means the new ahead-of-merge-root fact is only populated for name-rooted
  branches (the realistic scenario in the ticket's Background); the legacy
  rootless/`implement/*` path keeps its pre-existing behavior unchanged
  (`AheadOfMergeRoot` stays `0`, so the new stop branch never fires there —
  consistent with "does not rewrite the branch model").
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L817-L844` —
  `finishImplementBranchPlanTail(plan, n, obs, targetBranch)` is the exact
  `rename` vs `stop` decision point (shared tail for both the name-rooted and
  legacy-rootless callers at `#L797` and `#L814`). Current order: same-target
  ⇒ `continue`; `AllowRename != "yes"` ⇒ `stop`; `TargetExists` ⇒ `stop`;
  `Upstream/Ahead/Behind` (upstream-tracking, not merge-root) ⇒ `stop`; else
  `rename`. Insert the new `obs.AheadOfMergeRoot > 0` check **immediately
  after** the same-target check and **before** the `AllowRename` check, so it
  is unconditional and not gated by (and therefore not overridable by)
  `allow_rename` — this ordering directly encodes the "not overridable"
  invariant.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L703-L714` —
  `parseImplBranchRoot(branch) (root, stem string, ok bool)` is the existing
  branch-name-only parser to reuse for the stop hint's "suspected owning
  work" stem (zero commit parsing, per Decisions).
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L148-L173` /
  `#L506-L550` — `normalizedImplementFacts` / `normalizeImplementFacts` is
  where `target.*` values already get threaded into resolver-internal facts
  (e.g. `ScopeSlug`). Add a `TicketStem string` field, populated from
  `strings.TrimSpace(input.Target.TicketStem)`, so
  `finishImplementBranchPlanTail` (which only receives `n
  normalizedImplementFacts`, not the raw `implementInput`) can reference
  `target.ticket_stem` in the stop reason without changing that function's
  signature or threading `implementTargetInput` through the branch-plan call
  chain.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L112-L121` —
  `implementBranchPlan` (JSON-exposed via `verdict.branch_plan` /
  `agenda.branch_plan`, not an MCP tool *input* schema — additive fields here
  are not the "no schema change" constraint, which is about
  `enter.implement`'s input parameters). Add `SuspectedOwnerStem string
  \`json:"suspected_owner_stem,omitempty"\`` so the new stop branch can carry
  a distinguishing, non-brittle signal into `implementNextInstruction`
  (avoids matching on `Reason` substrings) without touching any other stop
  path's existing message.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L846-L862` —
  `implementNextInstruction(verdict)`'s `"stop"` case currently returns one
  generic sentence for every stop cause (unavailable branch, target exists,
  missing merge target, upstream ambiguity, AND now this new cross-ticket
  case). Special-case on `verdict.BranchPlan.SuspectedOwnerStem != ""` to
  return the lead-routing message; fall through to the existing generic
  sentence for every other stop cause (unchanged behavior, unchanged test
  expectations for `TestResolveImplementBranchStopOmitsPlannerInstructions`
  and the `merge target required` stop test in
  `session_state_test.go#L2230-L2264`).
- `agents-plugin-tool/internal/mcp/implement_resolver_test.go#L500-L543` —
  existing pattern: `resolveImplement(input, implementBranchObservation{...})`
  literal construction, no real git involved. `TestResolveImplementBranchRenameDefaultsToAllowedWhenUnset`
  (`#L525-L543`) uses `CurrentBranch: "impl/old"` (single segment after
  `impl/`, i.e. the **rootless legacy path**, not name-rooted) — this test
  must keep passing unchanged since `AheadOfMergeRoot` stays `0` there by
  construction (zero-value default in the literal). New matrix tests should
  use a name-rooted `CurrentBranch: "impl/<root>/<stem>"` (two segments) to
  exercise the new branch, matching the pattern already used at
  `implement_resolver_test.go#L779-L831` (e.g. `"impl/ws-dashboard-dev/target"`).
- `agents-plugin-tool/internal/mcp/session_state.go#L1004-L1029` —
  `handleEnterImplement` is the MCP-layer call site that builds the fact set
  fed to `resolveImplement`: it calls `observeImplementBranch(record.Root,
  "")` once to peek `CurrentBranch`, derives `targetBranch` via
  `implementTargetBranchName(implementMergeRootFor(peek.CurrentBranch),
  normalized.ScopeSlug)`, then calls `observeImplementBranch(record.Root,
  targetBranch)` again for the real observation passed to `resolveImplement`.
  No change needed here — `AheadOfMergeRoot` is derived purely from
  `obs.CurrentBranch` inside `observeImplementBranch` itself on both calls
  (idempotent; the peek call's result is discarded as today).
- `ai-docs/spec/mcp-tools.md#L276-L279` — the sentence "branch rename
  defaults to allowed unless the caller explicitly withholds consent
  (`policy.branch.allow_rename: no`)" needs a Phase-1-scoped update: rename
  defaults to allowed only when the current implementation branch carries no
  unmerged commits ahead of its merge root; when it does and scope
  mismatches, the resolver stops regardless of `allow_rename`. Do not touch
  the `tickets.close` prose in this same file (Phase 2).

## Implementation Plan

1. `implement_resolver.go` — add `AheadOfMergeRoot int` to
   `implementBranchObservation` (near `Ahead`/`Behind`, `#L138-L146`).
2. `implement_resolver.go` — add `TicketStem string` to
   `normalizedImplementFacts` (`#L148-L173`) and populate it in
   `normalizeImplementFacts` from `strings.TrimSpace(input.Target.TicketStem)`
   (`#L506-L537`).
3. `implement_resolver.go` — add `SuspectedOwnerStem string
   \`json:"suspected_owner_stem,omitempty"\`` to `implementBranchPlan`
   (`#L112-L121`).
4. `implement_resolver.go` — add a small helper (near `observeImplementBranch`,
   `#L415`) `aheadOfMergeRootCount(root, mergeRoot, currentBranch string) int`
   that calls `wsgit.NewClient().MergeBase(ctx, root, mergeRoot,
   currentBranch)`, then `(wsgit.ExecRunner{}).RunGit(ctx, root, "rev-list",
   "--count", mergeBase+".."+currentBranch)`, parses the trimmed output with
   `strconv.Atoi`, and returns `0` on any error from either call. Add
   `"strconv"` to the file's import block.
5. `implement_resolver.go` — in `observeImplementBranch` (`#L421-L427`),
   right after constructing `obs`, add: if `validObservedBranch(obs.CurrentBranch)`,
   compute `mergeRoot := implementMergeRootFor(obs.CurrentBranch)`; when
   `mergeRoot != "" && mergeRoot != obs.CurrentBranch`, set
   `obs.AheadOfMergeRoot = aheadOfMergeRootCount(root, mergeRoot, obs.CurrentBranch)`.
6. `implement_resolver.go` — in `finishImplementBranchPlanTail`
   (`#L820-L844`), insert a new branch immediately after the
   `obs.CurrentBranch == targetBranch` check and before the
   `n.AllowRename != "yes"` check:
   ```go
   if obs.AheadOfMergeRoot > 0 {
       _, suspectedStem, _ := parseImplBranchRoot(obs.CurrentBranch)
       plan.Action = "stop"
       plan.SuspectedOwnerStem = firstNonEmpty(suspectedStem, "unknown")
       plan.Reason = fmt.Sprintf(
           "current implementation branch has %d unmerged commit(s) ahead of merge root %q and target scope %q differs from suspected prior work %q; starting here would mix ticket work (not overridable by allow_rename)",
           obs.AheadOfMergeRoot, plan.MergeTarget, firstNonEmpty(n.TicketStem, "unspecified"), plan.SuspectedOwnerStem)
       return plan
   }
   ```
   This makes the stop unconditional on `allow_rename` (the check runs before
   and returns before that field is ever consulted), matches invariant (1)
   from the guardrails, and leaves the no-unmerged-work `rename` path
   (invariant (2)) and the `TargetExists`/upstream-ambiguity stops untouched.
7. `implement_resolver.go` — in `implementNextInstruction` (`#L846-L862`),
   change the `"stop"` case to:
   ```go
   case "stop":
       if verdict.BranchPlan.SuspectedOwnerStem != "" {
           return fmt.Sprintf(
               "Stop before source edits. Do not rename over unmerged work. Resolve branch identity from session context, or dispatch an explore comparing %s's commit history to the target ticket, then re-invoke enter.implement. Suspected prior owner (branch-name encoded, best-effort): %s.",
               verdict.BranchPlan.CurrentBranch, verdict.BranchPlan.SuspectedOwnerStem)
       }
       return "Stop before source edits. Report the branch safety blocker in Branch Action and ask for the missing policy or branch cleanup."
   ```
8. `ai-docs/spec/mcp-tools.md#L276-L279` — update the rename-default sentence
   to state the new conservative-stop condition (unmerged work ahead of
   merge root overrides `allow_rename`), keeping the rest of the paragraph
   intact. Do not touch the `tickets.close` prose (Phase 2, out of scope).

## Verification Plan

- `cd agents-plugin-tool && go test ./internal/mcp/...` — primary focused
  command; must pass including all pre-existing `implement_resolver_test.go`
  and `session_state_test.go` cases (notably
  `TestResolveImplementBranchRenameDefaultsToAllowedWhenUnset`,
  `TestResolveImplementBranchStopOmitsPlannerInstructions`, and the
  `merge target required` stop test at `session_state_test.go#L2230-L2264`,
  none of which should change behavior).
- Add resolver unit tests in `implement_resolver_test.go` (literal
  `implementBranchObservation{...}` construction, no real git, following the
  existing pattern) covering the ticket's matrix:
  (a) name-rooted branch (e.g. `CurrentBranch: "impl/root-branch/old"`,
      `AheadOfMergeRoot: 2`), target scope `"new"`, `AllowRename: "yes"` ⇒
      `Action == "stop"` and `SuspectedOwnerStem == "old"`.
  (b) same branch, `AheadOfMergeRoot: 0`, target scope `"new"`,
      `AllowRename: "yes"` ⇒ `Action == "rename"` (unchanged).
  (c) same branch, target scope `"old"` (matches current), any
      `AheadOfMergeRoot` value ⇒ `Action == "continue"` (equality check wins
      before the new branch is reached).
  (d) `CurrentBranch: "goal/<slug>"` (or any non-`impl/`/`implement/`
      prefixed branch) with `AheadOfMergeRoot` set on the observation ⇒
      `Action == "create"` (create path never calls
      `finishImplementBranchPlanTail`, so the field is inert there).
  (e) assert on the (a) case's `result.NextInstruction` /
      `result.Verdict.BranchPlan.Reason` that the message references
      resolving from session context or an explore, and the suspected stem —
      and does NOT claim to have parsed commit content.
- Optionally (not required to satisfy the ticket's stated verification, but
  cheap given the pattern already exists): a real-git-backed integration
  test in `session_state_test.go` following
  `TestEnterImplementCreatePathMergeRootRefConflictDetectedByRealGit`'s
  structure (create a merge-root branch, branch `impl/<root>/<stem>` off it,
  add a commit, switch scope, call `enter.implement`) to exercise
  `observeImplementBranch`'s real `MergeBase`/`rev-list` wiring end to end,
  not just the resolver's response to a fabricated observation.
- `go vet ./...` (or the repo's standard build/lint step, if any, in
  `agents-plugin-tool`) as a cheap compile-correctness check for the new
  `strconv` import and field additions.

## Escalations
- None.
