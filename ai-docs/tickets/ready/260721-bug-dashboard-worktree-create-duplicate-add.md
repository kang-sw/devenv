---
title: "Dashboard 'create worktree' action adds the resulting work root twice"
---

# Dashboard 'create worktree' action adds the resulting work root twice

## Symptom

Reported via live dogfooding of the ws-dashboard on 2026-07-21. Root cause is
**unconfirmed** — this ticket captures the observation and a hypothesis, not a
diagnosis.

Creating a git worktree through the dashboard's "create worktree" action (the
`GitWorktreeAddModal` submit flow) results in the newly created work root
appearing twice (duplicate entry) instead of once.

## Hypothesis

The manual-add event (the direct response from the create-worktree submit
call) and a server-side auto-detection/rediscovery event overlap, so the same
worktree path gets inserted twice. Suggested fix direction: add
path-based (worktree filesystem path) uniqueness/dedup filtering so a work
root that is already present is not re-added — `serverScopedIdentity` (see
below) is a likely candidate identity key if the dedup needs to happen at the
frontend resource-cache layer, though the daemon side has its own path-derived
id (`local_work_root_id_for_path`) that may be the more correct layer to
enforce it at.

## Investigation Starting Points

Located during a light locate pass (not a root-cause investigation — these are
starting points only):

### Frontend (`ws-dashboard/frontend/src`)

- `App.tsx:1417-1430` — `GitWorktreeAddModal` `onCreated` handler: the manual
  "create worktree" submit-result path. Applies `response.resources` via
  `resourceRefreshCoordinatorRef.current?.applyExternalResources(...)` and
  sets `selectedId` from `response.createdWorkRootId`. This is a single,
  direct apply with no additional refetch in this exact branch.
- `App.tsx:689-717` — `handleWorkRootOpened`: applies an opened-view
  resources snapshot immediately via `applyExternalResources`, then
  *additionally* fires `void loadResources("open")` right after (an extra
  canonical refetch layered on top of the direct-response apply). This is the
  same "two overlapping updates for one user action" shape the user's
  hypothesis describes, even though it fires on work-root *open* rather than
  *create*; worth checking whether an analogous double-apply exists on the
  create path once the daemon side is understood (see below).
- `resourceRefresh.ts:5` — `resourceAvailabilityPollIntervalMs = 5_000`: an
  independent background poll (`reason: "poll"`) that re-fetches and
  re-applies resources every 5s regardless of user action, and could race
  with a just-completed manual create.
- `resourceRefresh.ts:150-160` — `applyExternalResources`: forcibly applies
  external resources by bumping `issuedSequence`/`appliedSequence` without
  checking `isInFlight()`. A concurrently in-flight poll fetch issued at a
  sequence *not* less than the bumped `appliedSequence` is not treated as
  stale and would still apply afterward, re-running `applyResources` a second
  time for the same event window.
- `resourceModel.ts:204-209` — `mergeResourcesByServer`: replaces a given
  server's entire resources entry wholesale (accumulates only *across*
  servers, per its own contract comment). There is **no path-based dedup at
  this layer** — it trusts whatever `DashboardResourcesView` payload it is
  given, for either the direct-response apply or the poll/open re-fetch apply.
- `resourceModel.ts:81-86` — `serverScopedIdentity(serverRoute, ...parts)`:
  the existing identity-key helper (`[serverRoute, ...parts].join("/")`)
  already used elsewhere for work-root identity; likely the correct key
  shape to dedup on if a fix lands at the frontend cache layer.

### Daemon (`ws-dashboard/crates/daemon/src`)

- `git_worktree.rs:244-246` (rollback at `:265`, `:268`) — the create-worktree
  submit handler explicitly calls
  `state.opened_work_roots.register_registry_entry(created_id.clone(), RegisteredWorkRoot { .. })`
  to register the newly created worktree as its own registry entry.
- `discovery.rs:69-104` —
  `LocalDashboardResourcesProvider::dashboard_resources_with_registry_sync`:
  iterates `self.candidates` (registered roots) and, per candidate, *also*
  auto-discovers linked worktree paths via `git_worktree_paths(&candidate.path)`
  off the primary root. A manually-registered new worktree can plausibly
  surface both as its own top-level candidate (from the registry write above)
  and as an auto-discovered linked path off the original primary root's git
  worktree list.
- `discovery.rs:185-194` — `WorkspaceBuilder::push`: does dedup by
  `local_work_root_id_for_path`-derived `work_root_id`
  (`if self.work_roots.iter().any(|root| root.id == work_root_id) { return; }`),
  but only *within a single `WorkspaceBuilder`'s* `work_roots` Vec (i.e. within
  one workspace). There is no cross-workspace dedup if the two occurrences
  (registered candidate vs. auto-discovered linked path) resolve into
  *different* `workspace_key` BTreeMap entries — unconfirmed whether that can
  actually happen for a freshly created worktree of an existing primary root.

## Related

- `260710-idea-dashboard-open-work-root-full-registry-redundant-rediscovery`:
  same call cluster (`open`/create paths triggering full-registry
  rediscovery); adjacent but not confirmed to be the same underlying cause.

## Phases

### Phase 1: Reproduce, confirm mechanism, and fix

Confirm the duplicate-add is caused by the manual create-worktree add path
(`GitWorktreeAddModal`'s `onCreated` handler applying `response.resources`
directly) and the server-side auto-discovery/normalize path both inserting
the same work root, then add path-based (worktree filesystem path /
`serverScopedIdentity`) deduplication so a work root that is already present
is not re-added. Candidate seam: `mergeResourcesByServer`
(`resourceModel.ts:204-209`, currently no path dedup at that layer) and/or
the `onCreated` add path (`App.tsx:1417-1430`) - the investigation notes
above are starting points, not a prescribed fix location; the daemon-side
`local_work_root_id_for_path` / `WorkspaceBuilder::push` dedup
(`discovery.rs:185-194`) is a plausible alternative or complementary layer if
investigation shows the two occurrences land in different `workspace_key`
entries.

**Verification**: creating a worktree through the dashboard's "create
worktree" action yields exactly one entry for the new work root; legitimately
distinct work roots (pre-existing worktrees, other primary roots) still
appear as separate entries; existing frontend tests (`test:resource-model`,
`test:workbench`) stay green; manual dogfood repeat of the original
create-worktree flow confirms no duplicate.

## Spec Impact

No spec text change expected. `ai-docs/spec/ws-web-dashboard/index.md`
`## Git Worktree Creation {#260524-ws-dashboard-git-worktree-creation}`
already describes submit as refreshing canonical dashboard resources and
selecting/focusing *the* created linked workRoot (singular) - duplicate
entries are an implementation defect against that existing contract, not a
new or varying specified behavior, so this fix restores conformance rather
than changing the spec.
