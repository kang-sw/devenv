# Brief: 260523-feat-ws-dashboard-workroot-registry-activation

## Intent

Implement Phase 1 of the dashboard workRoot registry activation ticket: replace
opened-workRoot-only authority with a daemon-owned durable registry that keeps
known workRoots visible, separates current availability from user-controlled
activation, and gates file, Activity, and terminal APIs through that activation
state.

## Scope Boundary

Selected scope is Phase 1: Durable registry and activation spine.

Implement durable membership, v1 opened-root migration, public
availability/activation fields, online/offline activation transitions, and API
gating/error distinctions. Leave Phase 2 bounded polling and richer live-status
refresh cadence out of scope except for any existing explicit resource refresh
paths needed to recompute availability.

## Caller-Visible Contract

Dashboard resource loads expose known workRoots from a daemon-local registry,
not only the currently opened workRoot map. Known workRoots remain visible when
their activation is offline or when their live availability has degraded.

WorkRoot view models expose availability separately from activation:

- `activation`: `online | offline`
- `availability`: initial public states for available, missing, moved,
  inaccessible, and unknown

A reachable workRoot with `activation: offline` remains visible but cannot be
used for file, Activity, or terminal APIs until brought online. This state is
not equivalent to missing or inaccessible availability.

Existing persisted opened-workRoots state migrates to registry membership with
`activation: online`, preserving current restart behavior.

Route behavior distinguishes:

- unknown workRoot id: not found
- known workRoot with offline activation: bounded offline response
- online workRoot with unavailable/degraded availability: bounded unavailable
  response without host paths

Online/offline changes are exposed through dashboard controls that use logical
command ids and targets so later keybindings can dispatch the same actions.

## Contract Instructions

Update the Rust core resource model and serialized view model rather than
bolting activation into the existing `WorkRootStatus` meaning. Existing public
`WorkRootStatus` online/offline vocabulary must not become activation.

Expected Rust surfaces include:

- `ws-dashboard/crates/core/src/resources.rs`
- `ws-dashboard/crates/core/src/view_model.rs`
- `ws-dashboard/crates/core/src/mock.rs` or equivalent fixture source
- `ws-dashboard/crates/daemon/src/persistent_state.rs`
- `ws-dashboard/crates/daemon/src/work_root_files.rs`
- `ws-dashboard/crates/daemon/src/resources.rs`
- `ws-dashboard/crates/daemon/src/root_picker.rs`
- `ws-dashboard/crates/daemon/src/discovery.rs`
- `ws-dashboard/crates/daemon/src/terminal.rs`
- `ws-dashboard/crates/daemon/src/work_root_activity.rs`

Expected frontend surfaces include:

- `ws-dashboard/frontend/src/resourceModel.ts`
- resource navigation/detail UI that renders workRoot availability and
  activation distinctly
- dashboard command dispatch entries for online/offline activation controls
- frontend tests or fixtures that compile against the new model

The daemon-local registry is the source of membership and activation. Browser
state may select or render a workRoot, but it must not become resource
authority. Persist registry state through the existing dashboard state store or
a versioned successor; migrate existing opened-workRoots v1 entries as online
registry entries.

Reuse existing discovery to recompute availability and kind. Preserve stable
opaque ids where possible so restored selections and panes do not churn.

Forbidden temporary wiring:

- no mock-only resource path for production registry rows
- no host paths in browser-visible error bodies
- no treating browser localStorage as registry authority
- no overloading `status: online | offline` to mean both availability and
  activation
- no Phase 2 bounded polling implementation in this slice

## Integration Test Instructions

Add or extend integration tests at the daemon/resource boundary and the
frontend model boundary.

Required assertions:

- existing opened-workRoots persistence migrates/restores as known registry
  membership with `activation: online`
- resource serialization includes distinct availability and activation fields
  and fixtures compile/render against them
- all-workRoots-offline state still returns visible workspace/workRoot rows
- file listing/read, terminal create, and Activity routes reject known offline
  workRoots with the bounded offline error
- unknown workRoot ids still return not found
- unavailable/missing online workRoots return bounded unavailable responses
  without host paths
- explicit online/offline activation controls use dashboard command dispatch
  rather than direct click-only side effects

Run at minimum:

- `cargo fmt --all --check`
- `cargo test -p ws-dashboard-core`
- `cargo test -p ws-dashboard-daemon`
- `npm run build`
- relevant frontend/browser tests if changed by the implementation

## Implementation Strategy Decisions

- Durable registry state replaces opened-workRoot-only persistence as the
  membership/activation spine.
- The existing opened-root behavior is migration input, not a parallel
  long-term authority.
- Availability is derived from filesystem/Git discovery; activation is
  user-controlled dashboard state.
- Explicit activation changes must be command-routable for future Tmux-like
  keybindings.
- Phase 2 polling is deferred.

## Rejected Alternatives

- Reusing `WorkRootStatus::Online/Offline` as activation was rejected because
  the current status vocabulary already describes reachability/availability.
- Silently hiding missing, inaccessible, or offline known workRoots was
  rejected; known workRoots stay visible until a future explicit forget/remove
  policy exists.
- Browser-local persistence as registry authority was rejected; daemon/resource
  APIs remain authoritative.

## Approach

- Introduce a public availability/activation split in core types and frontend
  types first.
- Introduce a daemon-local registry abstraction that records known membership,
  provenance, and activation state.
- Migrate current persistent opened-workRoot entries into the registry as
  online entries.
- Rewire resource loading to derive rows from the registry plus discovery
  availability.
- Gate file, Activity, and terminal route resolution through registry
  membership, activation, and availability checks.
- Add command-routed online/offline controls in the browser.
- Update fixtures and tests together with the public model.

## Constraints

- Preserve owner-auth protections and opaque ids.
- Do not expose host paths, Git internals, cache paths, or daemon-private
  session ids to the browser.
- Keep linked worktree discovery expansion out of this phase except where
  existing discovery already classifies a selected path.
- Keep polling/watchers out of this phase.
- Keep changes scoped to dashboard registry, resource model, activation
  controls, route gating, and required tests.

## Out of scope

- Linked Git worktree expansion from `git worktree list`.
- Bounded polling cadence or filesystem watchers.
- Future forget/remove/delete UX.
- Read-only file pane restore.
- Broader workbench layout or Activity Console local-state persistence.
- Terminal PTY survival across daemon restart.

## Details

The implementation may rename, replace, or deprecate current public `status`
fields, but callers must be able to distinguish availability from activation in
the serialized resource model. If compatibility fields remain temporarily, tests
must prove the new fields are authoritative and unambiguous.

Activation transitions should be logical dashboard actions, for example
workRoot online/offline command ids with opaque workRoot targets. The exact HTTP
route shape is implementation-owned, but UI controls must dispatch through the
same command path future keybindings will use.

Unavailable/offline error payloads should be stable enough for frontend handling
and tests, but they should stay bounded and avoid host paths.

## Verification Contract

Implementation is not complete until Rust and TypeScript builds/tests pass for
the changed contract and route gating behavior. If browser-visible resource UI
changes are material, run the existing browser/dashboard test gate or document
why only compile/model tests were sufficient for this slice.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` — specs
  `260523-dashboard-workroot-registry-activation`,
  `260516-ws-web-dashboard-resource-view-model-contract`,
  `260516-ws-web-dashboard-workroot-io-restore-model`,
  `260516-ws-web-dashboard-mock-view-model-fixtures`,
  `260516-ws-web-dashboard-local-workroot-discovery-provider`, and
  `260516-ws-web-dashboard-root-picker-empty-directory-creation`.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` — dashboard resource,
  persistence, frontend model, protected route, and browser verification
  invariants.
- [Maybe] `ai-docs/tickets/idea/260523-feat-ws-dashboard-linked-worktree-discovery.md`
  — downstream linked worktree discovery consumer of this spine.
- [Maybe] `ai-docs/tickets/todo/260523-feat-ws-dashboard-readonly-file-pane-restore.md`
  — adjacent restore work depending on remembered workRoots.
