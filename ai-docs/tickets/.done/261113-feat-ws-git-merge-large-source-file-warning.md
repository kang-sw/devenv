---
title: "git.merge: warn when a merge grows a source file past a size limit"
related:
  260911-feat-ws-git-merge-lead-owned-merge-authority: "predecessor (git.merge tool)"
  260912-feat-git-merge-generic-branch-promotion: "predecessor (generic source/target merges)"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 82eb8bb3d61b914f
sage-review-design-reviewed: 82eb8bb3d61b914f
completed: 2026-11-13
---

# git.merge: warn when a merge grows a source file past a size limit

## Background

Owner idea, 2026-11-13. Agents grow source files without bound (for example
`dashboard.js` at ~6.9k lines). Agents also restructure aggressively when they
assume they own the project, so the warning must route the refactor decision
to the user, not trigger it.

The `git.merge` handler (lead-owned) lives in
`agents-plugin-tool/internal/mcp/git_merge.go` (`mergeImplBranch`,
`git_merge.go#L76-L396`; MCP dispatch `agents-plugin-tool/internal/mcp/server.go#L1359-L1394`).

## Behavior

`git.merge` result carries warnings computed over merge-base..branch, for
files with known source extensions:

- **Soft threshold N**: a file that was <= N lines at the merge base and is
  > N after the merge ->
  "Warning: this change pushed <file> past N lines (now L); splitting it is
  recommended."
- **Hard threshold M**: any file >= M lines that the branch edited ->
  "Warning: edited <file> at L lines (>= M); splitting it is recommended."
- Every warning text tells the reader (the lead): do not split on your own;
  ask the user whether refactoring this project is permitted and whether to do
  it now.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- T0 additive: new advisory diagnostics in the `git.merge` result plus new
  built-in config keys (see Decisions); no manifest, launcher, or protocol
  change.
- The warning text ships to downstream agents: read
  `ai-docs/manuals/shipped-surface-boundary.md`.
- `ai-docs/manuals/release-compatibility.md` applies.

## Decisions

- (424)=a: warnings appear only in the `git.merge` result. Rejected:
  `git.commit` / edit hooks (deep workers receive them but cannot ask the user
  and may refactor unilaterally); ticket ready-promotion pre-check (owner:
  injects refactoring into the discussion context and lowers ticket quality);
  `review.stamp` / lead-review (owner: too many changed files per range to be
  useful).
- (425)=b: a file the user declines to split is recorded and not warned again
  (unless it grows past a recorded limit). Rejected: ask every time (warning
  fatigue on files like `dashboard.js`); a single project-wide
  refactor-permission setting.
- (426): N and M are config keys (defaults N=1000, M=2000; 0 disables each);
  the extension list is configurable; files marked generated or vendored in
  `.gitattributes` (`linguist-generated` / `linguist-vendored`) are excluded.
  Rejected: hard-coded values. Owner note 2026-11-13: N, M and the extension list
  are ordinary config keys that follow the existing config layers (manifest
  default, then project/user overrides) and are tunable through lead-tune like
  any other knob; no separate mechanism.
- (427): the warning text explicitly instructs the lead not to split on its
  own and to ask the user about refactor permission and timing. Rejected: a
  bare "splitting recommended".
- Compatibility tier: T0 (additive).
- (428): the suppression list lives in .gitattributes as a `ws-size-guard` attribute (`-ws-size-guard` disables the warning for matching paths; `ws-size-guard=<lines>` overrides the limit for them). Rejected: a new `.wsignored` file (owner's initial suggestion) - .gitattributes already gives gitignore-style per-directory patterns evaluated by `git check-attr`, and holds the generated/vendored exclusion. A per-path limit value gives (425) b's "warn again only after notable growth".
- Config declaration (technical, EXEC 2026-11-13, from fact population): N, M,
  and the extension list are built-in Go config keys declared like the
  worktree-nudge knobs (`declaredConfigKey` in
  `agents-plugin-tool/internal/mcp/worktree_nudge.go`, registered in
  `builtinConfigDefaults`), named `git.merge_size_soft_lines` (integer,
  minimum 0, default 1000), `git.merge_size_hard_lines` (integer, minimum 0,
  default 2000), and `git.merge_size_extensions` (string, space-separated
  extensions without dots), default scope project. Add `git.` to
  `reservedConfigPrefixes` (`config_manifest.go`) so no manifest claims it.
  They follow the config layers and `config.tune` like every built-in knob,
  which satisfies the owner note on (426). Rejected: keys in the shipped
  config manifests - each manifest is locked to one namespace (`claude.` /
  `dashboard.`, `validateManifestKey` in `config_manifest.go`), so manifest
  keys would need a new namespace manifest in ws, wsflow, and Pi plus
  launcher env changes, with no per-package default to justify it.
- Default extension list (technical): `go js mjs cjs ts tsx jsx py rs java kt
  c cc cpp h hpp cs swift rb php css scss html vue svelte sh`. Rejected: no
  default (the feature would be off until configured).
- `ws-size-guard` semantics (technical): attributes are read with
  `git check-attr --source <merge commit>` so they reflect the merged tree;
  when the installed git refuses `--source` (older than 2.40), read them
  from the working tree instead, since the warning is advisory. Unspecified,
  or set without a value, applies the configured N and M. `-ws-size-guard`
  (unset) disables both warnings for the path. `ws-size-guard=<L>` replaces
  both N and M for the path with one limit: warn only when the file was
  <= L at the merge base and is > L after the merge. A path whose
  `linguist-generated` or `linguist-vendored` attribute is set is skipped.
  Rejected: a per-path value that overrides only N (the hard rule would keep
  warning on every edit of a file the user already declined to split).
- Rule details (technical, review-raised): line counts L use the merged
  tree; a file added on the branch counts as 0 lines at the merge base. A
  file gets at most one warning: the hard-threshold text when L >= M,
  otherwise the soft text. Warnings run only for `impl/*` and `goal/*`
  sources (the units a lead integrates); generic branch promotion such as
  `develop` -> `main` emits none, since it would repeat warnings already
  given at each unit merge. The declared keys are registered wherever the
  existing declared integer keys are (`builtinConfigDefaults`, the
  declared-key lookup in `config_registry.go`, the tuning-knob listing in
  `server.go`). The suggested `.gitattributes` line quotes a path that needs
  it per gitattributes rules. Rejected: warning on every merge (release
  merges repeat unit warnings); two warnings for one file.
- Recording a declined split (technical): the lead records it by editing
  `.gitattributes` in a normal commit; the warning text names the exact line
  to add (`<path> ws-size-guard=<current lines + margin>` or
  `<path> -ws-size-guard`). No tool writes `.gitattributes`. Rejected: a new
  tool (more surface for one line of text the lead can write).

## Prior Decisions

- 260911-feat-ws-git-merge-lead-owned-merge-authority (2026-09-11, commit f6255a1f): "Phase 1 adds a lead-only git.merge tool that derives the target from the local impl ref, validates optional assertions, and preserves merge records through the shared commit-message builder." — bearing: supports
- 260912-feat-git-merge-generic-branch-promotion (2026-09-12, Decisions): "Accept any exact local source branch and exact local target branch through `ws/git.merge`. Preserve the existing optional current-branch source; require an explicit target when it cannot be derived from an `impl/*` source." — bearing: constrains
- c7c1de4d (2026-09-12, commit): "Collect all checkable safety findings, retain them as non-waivable diagnostics, and recheck candidate tips before checkout and merge to detect intervening changes." — bearing: constrains
- c7c1de4d (2026-09-12, commit): "Expose frontier reachability and uncovered candidate commits as evidence without inferring review quality or unresolved findings; preserve existing non-release success and conflict behavior." — bearing: constrains
- 261112-feat-ws-mailbox-undelivered-notice (2026-11-13, Result 95f335508): "Added the global-only integer key (minimum 1, default 3) to ws and wsflow, and to Pi because the existing three-package parity contract requires it." — bearing: constrains
- e628e96c (2026-11-13, commit): "Owner note on board topic file-size-guard: thresholds follow existing config layers and lead-tune." — bearing: supports
- c052d920 (2026-11-13, commit): "Warnings only in git.merge result (lead-owned); commit/edit hooks, ticket promotion and review.stamp rejected per owner." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/git_merge.go, agents-plugin-tool/internal/mcp/server.go (dispatch reads the keys), worktree_nudge.go-style declaredConfigKey declarations and builtinConfigDefaults, config_manifest.go (reserved prefix), ai-docs/manuals/ws-mcp.md; no manifest or launcher change |
| scope.surface | public-interface | git.merge MCP result gains warning text/diagnostics; new config keys visible in config.list/config.tune |
| scope.new_public_symbol | yes | built-in config keys git.merge_size_soft_lines / git.merge_size_hard_lines / git.merge_size_extensions and the ws-size-guard gitattribute |
| scope.new_type_contract | no | prefer reusing implMergeDiagnostic (git_merge.go#L23-L30) with classification advisory; no new type required |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/git_merge_test.go (mergeFixture, mergeMessage helpers), config_manifest_test.go, config_manifest_parity_test.go |
| complexity.reuse_points | confirmed | implMergeDiagnostic advisory entries (dirty_after_merge, cleanup_failed in git_merge.go), wsgit.Runner.RunGit, declaredConfigKey (worktree_nudge.go#L20-L40) or manifest loader (config_manifest.go) |
| complexity.side_effect_risk | moderate | runs after the merge commit and must never change status or fail the merge; extra git calls (diff --numstat, check-attr) on every merge |
| risk.correctness | moderate | line counting at merge base vs merged tree, renames, binary files, and merge-conflict early return path need handling |
| risk.fit | moderate | built-in Go declaration settled (Decisions); manifests untouched |
| risk.test | moderate | git_merge_test.go has fixtures for real repos; check-attr and large-file fixtures are new |
| risk.security_or_contract | moderate | cross-version config surface (T0 claimed) and warning text ships downstream |

## Phases

### Phase 1: Size warnings in the git.merge result

After a successful merge in `mergeImplBranch` (after `result.Status =
"merged"` and the merge commit are set; the conflict and policy-blocked paths
return earlier and get no warning), compute the warnings over
merge-base..branch for files whose extension is in the configured list:
line counts at the merge base and in the merged tree, the soft rule (crossed
N), the hard rule (edited while >= M), and the `ws-size-guard` /
linguist exclusions from Decisions. Emit each warning as an
`implMergeDiagnostic` with `Classification: "advisory"`, like
`dirty_after_merge`; every text names the file, the line count, the
threshold, the instruction from (427), and the `.gitattributes` line that
records a declined split. A failure while computing warnings (git error,
binary file, unreadable blob) never changes the merge status: skip that file,
or append one advisory diagnostic saying the check could not run. Read the
three keys in the `git.merge` dispatch through the session resolver and pass
them into `mergeImplBranch`; without a session key, use the defaults. Declare
the keys and the reserved prefix per Decisions. Update the `git.merge`
section of `ai-docs/manuals/ws-mcp.md` if it lists result diagnostics.

Verification: `go test ./internal/mcp/...` passes. New `git_merge_test.go`
cases on real fixture repos cover: crossing N (warns), already above N but
below M (no warning), editing a file >= M (warns), an unlisted extension (no
warning), `-ws-size-guard` (no warning), `ws-size-guard=<L>` (warns only when
crossing L), a `linguist-generated` file (no warning), a key set to 0
(disables that rule), and a conflicting merge (no warning, status
unchanged). Config tests show the three keys in `config.list` with their
defaults and accept a `config.tune` override.

### Result (c55b2e282) - 2026-11-13

- Landed: `agents-plugin-tool/internal/mcp/git_merge_size.go` computes
  advisory `size_soft_limit` / `size_hard_limit` diagnostics (and one
  `size_check_failed` when Git fails) after a landed `impl/*` or `goal/*`
  merge, over merge-base..source for files with a configured extension.
  Line counts use `git grep -c -I -z -e ''` (binary/empty files drop out);
  renames take the old path's merge-base count, and a pure rename (R100) is
  not an edit (ce4cbd83d). Attributes come from `git check-attr --source
  <merge commit>`, falling back to the working tree. Each Resolution tells
  the lead not to split on its own, to ask the user about refactor permission
  and timing, and names the `.gitattributes` lines
  (`/<path> ws-size-guard=<lines + 20%, rounded up to 100>` or
  `/<path> -ws-size-guard`; anchored, glob-escaped, C-quoted when needed).
- Config: `git.merge_size_soft_lines` (1000), `git.merge_size_hard_lines`
  (2000), `git.merge_size_extensions` declared as built-in keys
  (wsconfig items with project default scope, `builtinConfigDefaults`,
  `resolveConfigEntryForKey`, tuning catalog kind `git_merge_size`); `git.`
  added to `reservedConfigPrefixes`. The dispatch resolves them through the
  session resolver; keyless calls use the defaults. The `git.merge` tool
  description mentions the advisory diagnostics.
- Verification: `go test -run 'MergeSize|Gitattributes|GitMergeSize|ImplMerge|GenericMerge' ./internal/mcp/`
  passes; `go test ./internal/wsconfig/` passes. Full `go test ./internal/mcp/`
  fails only TestRenderDispatchSeparatesResourceAndWorktreeRoots,
  TestRenderFromWorktreeSessionCommitsInThatWorktree, and
  TestRenderPathMintedChildRecordsLeadParent, which fail identically on the
  base commit (macOS /private/var canonicalization).
- Decisions: tests live in a new `git_merge_size_test.go` rather than
  `git_merge_test.go` (cosmetic); a malformed `ws-size-guard=<value>` skips
  the path; `ai-docs/manuals/ws-mcp.md` unchanged because it has no
  `git.merge` section listing result diagnostics; a target-side rename of a
  file the source edited is skipped (advisory false negative, accepted).
