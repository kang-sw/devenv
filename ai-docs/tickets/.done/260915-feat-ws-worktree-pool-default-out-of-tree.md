---
title: "Default worktree pool out-of-tree: add $(GitRootDirName) primitive, relocate default to $(GitRoot)/../.ws-worktrees/$(GitRootDirName)"
related:
  260915-bug-ws-tickets-close-operates-on-server-cwd-not-worktree: same worktree-run dogfood session; both concern the worktree/parallel path
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 90cbd9e532779d9c
sage-review-completeness-reviewed: 90cbd9e532779d9c
completed: 2026-09-16
---

# Default worktree pool out-of-tree

## Background

The `worktree_pool` default is `$(GitRoot)/.ws-worktrees` — the pool sits
**inside** the repo working tree. During a parallel `ws:lead-run` (2026-09-15)
this produced real IDE clutter: editors auto-detect the nested worktrees and
index/scan them.

`worktree_pool` is already a project-scoped, user-overridable knob, and the
default is a use-time-resolved template (not persisted), so only the builtin
default and one new template token need to change; existing explicit overrides
are unaffected and unset projects pick up the new default automatically.

Note: git-status clutter is *already* handled — `registerPoolExclude`
(`worktree_tools.go`) appends the in-tree pool path to the repo's local
`.git/info/exclude` (never a committed `.gitignore`). The remaining, unhandled
problem is IDE indexing of the nested tree, which `.git/info/exclude` does not
suppress. This is why relocating the pool out of the working tree is the fix,
not a gitignore entry.

## Decisions (confirmed 2026-09-15)

- **New template primitive `$(GitRootDirName)` = `filepath.Base(GitRoot)`.**
  The substitution site currently supports only `$(GitRoot)`
  (`resolvePoolRoot`, `worktree_tools.go`).
- **New default: `$(GitRoot)/../.ws-worktrees/$(GitRootDirName)`**, then
  `filepath.Clean`'d so the resolved/reported path is not
  `.../repo/../.ws-worktrees/repo`.
- **Sibling (`../`), not `~/.cache`,** to keep worktrees on the **same
  filesystem** as the repo (efficient `.git` object sharing / hardlinks) while
  leaving the IDE workspace root.
- **Namespacing is collision-free by construction:** a basename is unique
  within its parent, so sibling repos sharing one `.ws-worktrees` parent map to
  distinct `<basename>/` subdirs.
- **Explicit `worktree_pool` overrides still win** — unchanged; only the
  builtin default template changes.
- **No automatic migration of legacy in-tree pools (settled: option a).**
  Existing `$(GitRoot)/.ws-worktrees` worktrees are not moved, drained, or
  reused after the default flips (reuse scan is scoped to `pathUnder(poolRoot,
  …)`, so a stale pool under a different root is simply not a reuse candidate).
  Rationale: legacy pools remain **visible inside the repo working tree**, so a
  user detects and prunes them trivially; the feature shipped days ago with no
  downstream adopters, so there is no accumulated stale-pool population to
  migrate. Documented as manual cleanup, not automated.
- **`registerPoolExclude` stays scoped to the in-tree fallback only.** It is
  already guarded by `pathUnder(mainRoot, poolRoot)`; an out-of-tree pool is
  outside the repo, needs no exclude entry, and must not get one.

## Constraints

- **MUST fall back** to in-tree `$(GitRoot)/.ws-worktrees` (current behavior)
  with a caller-visible advisory — via the existing per-call `res.Warnings`
  channel (`worktree_tools.go`), naming the reason and the in-tree fallback
  path — when the `$(GitRoot)/..` parent is not creatable/writable (mount root,
  read-only parent, container `/workspace`), so no repo becomes
  un-provisionable. A process-level dedup so the notice does not repeat on every
  call is acceptable but not required.
- Update **both** default-definition sites or dedupe them:
  `agents-plugin-tool/internal/mcp/server.go` (config builtin default) and
  `agents-plugin-tool/internal/mcp/worktree_tools.go` (`resolvePoolRoot`
  fallback). They must not diverge.
- The `worktree.acquire` MCP tool **description string**
  (`agents-plugin-tool/internal/mcp/server.go`, ~L3539) also states the old
  default (`$(GitRoot)/.ws-worktrees`) inline. It is documentation, not a
  default-definition site (no logic reads it), but it must be updated to the
  new default in the same change so the caller-facing doc does not go stale.
- Explicit `worktree_pool` overrides still win.
- `filepath.Clean` the resolved pool root before use and before reporting.
- `ai-docs/manuals/ws-mcp.md` applies (paths under
  `agents-plugin-tool/internal/mcp/`): read before editing.
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Open questions (deferred)

- Should `worktree.release`/prune diagnostics surface the (now less visible)
  out-of-tree pool location so orphans stay discoverable? **Deferred** to a
  follow-up (2026-09-15); not in scope for this ticket.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/worktree_tools.go, agents-plugin-tool/internal/mcp/server.go |
| scope.surface | internal | resolvePoolRoot, registerPoolExclude, builtinConfigDefaults are all unexported (worktree_tools.go#L90, worktree_tools.go#L118, server.go#L481-L488) |
| scope.new_public_symbol | no | none — new template token is a string substitution inside the existing unexported resolvePoolRoot, not a new symbol |
| scope.new_type_contract | no | none — resolvePoolRoot's signature (worktree_tools.go#L90) is unchanged |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/worktree_tools_test.go: TestResolvePoolRoot#L57, TestPathUnder#L86, TestProvisionWorktreeAbsolutePoolOverride#L305 |
| complexity.reuse_points | confirmed | reuses resolvePoolRoot's existing $(GitRoot) substitution + trailing filepath.Clean (worktree_tools.go#L90-L99) and registerPoolExclude's existing pathUnder(mainRoot, poolRoot) guard (worktree_tools.go#L187) |
| complexity.side_effect_risk | moderate | changes the on-disk location where worktree.acquire's os.MkdirAll creates directories for every unset-config project (worktree_tools.go#L184), moving default creation outside the repo tree |
| risk.correctness | moderate | Phase 2's unwritable-parent fallback must not hard-fail provisioning, and the reuse-scan pathUnder(poolRoot, e.Path) guard (worktree_tools.go#L198) and registerPoolExclude's pathUnder(mainRoot, poolRoot) guard (worktree_tools.go#L187) must keep behaving correctly once poolRoot moves outside mainRoot |
| risk.fit | low | follows the template-substitution pattern resolvePoolRoot already implements for $(GitRoot) (worktree_tools.go#L90-L99); no architectural change |
| risk.test | low | resolvePoolRoot and pathUnder are pure functions with existing table-driven tests to extend (worktree_tools_test.go#L57-L105) |
| risk.security_or_contract | moderate | changes the builtin default of a project-scoped config knob (server.go#L486); the worktree.acquire tool description still states the old default inline (server.go#L3539) and is not named among the ticket's default-definition sites to update, so it would drift stale if left unedited |

## Phases

### Phase 1: `$(GitRootDirName)` primitive and out-of-tree default

Add `$(GitRootDirName)` = `filepath.Base(gitRoot)` to the template substitution
in `resolvePoolRoot`. Change the builtin default template (both the
`server.go` config default and the `worktree_tools.go` fallback, kept in sync
or deduped to one source) to `$(GitRoot)/../.ws-worktrees/$(GitRootDirName)`,
`filepath.Clean`'d. Explicit `worktree_pool` overrides must continue to resolve
unchanged (only the builtin default string changes). Confirm the reuse scan and
`registerPoolExclude`'s `pathUnder(mainRoot, poolRoot)` guard behave correctly
when the resolved pool is now out-of-tree (exclude must not fire).

### Result (100e0be) - 2026-09-16

Landed together with Phases 2-3 in one worker session (tightly coupled Go
package change). `defaultWorktreePoolTemplate = "$(GitRoot)/../.ws-worktrees/$(GitRootDirName)"`
is now the single source `server.go`'s `builtinConfigDefaults` and the
`worktree.acquire` tool description both read (via `fmt.Sprintf`), so the two
default-definition sites plus the doc string cannot diverge.
`resolvePoolRoot` substitutes `$(GitRootDirName)` (`filepath.Base(gitRoot)`)
alongside the existing `$(GitRoot)` token, still `filepath.Clean`'d.
`registerPoolExclude`'s `pathUnder(mainRoot, poolRoot)` guard needed no code
change: it already scopes exclude registration to in-tree pools, so the
now-out-of-tree default correctly gets no exclude entry (test:
`TestProvisionWorktreeCreateNew`).

### Phase 2: In-tree fallback and advisory for unwritable parents

When `$(GitRoot)/..` is not creatable/writable, resolve to the legacy in-tree
`$(GitRoot)/.ws-worktrees` and emit the fallback advisory (see Constraints).
The writability probe belongs at the provisioning site (`provisionWorktree`'s
`os.MkdirAll(poolRoot)`, `worktree_tools.go`), which keeps `resolvePoolRoot` a
pure string function; the implementer may instead give `resolvePoolRoot` an
injectable probe, but must then update `scope.new_type_contract` in Route Facts
(it currently reads `no`). Provisioning must never hard-fail solely because the
sibling parent is not writable.

### Result (e02d2eb) - 2026-09-16

`provisionWorktree`'s `os.MkdirAll(poolRoot)` failure path falls back to
`legacyInTreePoolTemplate` and appends a `res.Warnings` advisory naming the
reason and the in-tree path, only when the pool is the builtin default and
resolves outside `mainRoot`. `resolvePoolRoot` stayed a pure string function
(no injectable probe); `scope.new_type_contract` remains `no` as recorded.

Round-1 review (single-allocation, `reviewer`) caught a Critical: the initial
gate compared `poolConfigValue == ""`, but `worktree.acquire`'s real dispatch
path (`wsconfig.Resolver.Get`) never passes an empty value — it substitutes
the literal builtin default template before calling `provisionWorktree` — so
the fallback was unreachable in production on exactly the hosts (mount root,
read-only parent, container `/workspace`) the Constraints named. Fixed in
commit `e02d2eb` with `isDefaultPoolConfig` (empty OR the literal default
template). The same review found `releaseWorktree` was not fallback-aware
(a fallback-provisioned worktree would be unreleasable) and a portability gap
in the new test (Windows `os.Getuid()`/chmod semantics; raw vs.
git-canonicalized root path). Both fixed in the same commit; round 2 verified
all three findings fixed, verdict clean (see Verification below).

### Phase 3: Tests

Unit coverage for: `$(GitRootDirName)` resolution; the new default resolved +
`filepath.Clean`'d path; unwritable-parent fallback + advisory; explicit
`worktree_pool` override precedence (override wins over the new default);
and the in-tree exclude guard firing only for the in-tree fallback, not the
out-of-tree default. Existing `worktree.acquire` / `worktree.release` suite
stays green.

### Result (a6940d3) - 2026-09-16

Added/updated in `agents-plugin-tool/internal/mcp/worktree_tools_test.go`:
`TestResolvePoolRoot` (extended for `$(GitRootDirName)` + new default),
`TestResolvePoolRootDefaultTemplate` (pins the literal template),
`TestProvisionWorktreeCreateNew` (default is out-of-tree, no exclude entry),
`TestProvisionWorktreeInTreeOverrideRegistersExclude` (exclude registration
still covered via an explicit legacy override),
`TestProvisionWorktreePoolResolvesFromLinkedWorktree` (updated expected pool),
`TestProvisionWorktreeAbsolutePoolOverride` (override precedence, unchanged),
`TestProvisionWorktreeDefaultFallsBackWhenParentUnwritable` (table over
`poolConfigValue` in `{"", defaultWorktreePoolTemplate}` — the second case is
the exact shape that caught the round-1 Critical — covering fallback
resolution, the advisory warning, in-tree exclude registration, and
`releaseWorktree` accepting the fallback-provisioned worktree; skips on
Windows and under uid 0). Full suite: `go test ./...` from
`agents-plugin-tool/` green (all packages `ok`), `go vet ./...` clean,
`gofmt -l` clean on every file this ticket touched.

## Verification (acceptance)

- New default resolves to the `filepath.Clean`'d sibling path on a normal repo;
  in-tree fallback + advisory on an unwritable parent.
- Explicit `worktree_pool` override is unaffected.
- Both default sites agree (or are deduped).
- The `worktree.acquire` tool description string no longer states the old
  in-tree default.
- `registerPoolExclude` does not touch `.git/info/exclude` when the pool is
  out-of-tree.
- Existing worktree suites green.

## Notes

Captured under the "Dogfood surprises get captured" discipline. Sibling
same-session dogfood tickets:
`260915-bug-ws-ticket-facts-new-public-symbol-likely-enum`,
`260915-bug-ws-tickets-close-operates-on-server-cwd-not-worktree` (investigation),
`260915-bug-ws-route-resolve-implement-branch-handling-random-codename`.
