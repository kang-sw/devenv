# Plan: 260525-feat-ws-dashboard-server-scoped-operation-forwarding — Phase 2: Backend local aliases and one-shot forwarding skeleton

## Relevant Ticket Contract

- Add Server Route-scoped daemon routes for **one-shot HTTP operations**:
  `server-local` dispatches **in-process** through existing local handlers;
  linked servers resolve through daemon-owned linked-server metadata +
  memory-only bearer tokens + endpoint hints (ticket Phase 2, lines 349-360).
- Introduce **one allowlisted forwarding helper** for JSON/ordinary HTTP routes.
  It preserves upstream status/error shape as much as practical; translates
  unknown / auth-required / tunnel-required / unreachable into **bounded gateway
  errors** without leaking endpoints/tokens/paths/daemon metadata (Decisions
  lines 79, 88-90; Constraints line 191-193); and rewrites returned
  `DashboardResourcesView` + nested `ResourcePath.serverId` to the selected
  Server Route (Decisions lines 91-94).
- Keep old bare local routes as **`server-local` compatibility aliases** —
  existing local tests and browser behavior must not change (Decisions line
  57-60; Phase 2 line 362-364).
- **Dot-free daemon-side validation is delivered in this phase.** Spec Phase 1
  entry states authoritative daemon-side rejection of dotted linked-server route
  requests "is delivered with server-scoped forwarding, not by this frontend
  contract" (`ai-docs/spec/ws-web-dashboard/index.md#L271-272`); Implementation
  Strategy requires rename + dot-free validation applied *within this phase's own
  cherry-pick* so the landed diff is never transiently non-compliant (ticket
  lines 244-251). The reused draft has neither.
- **Naming:** the phase's landed diff must read as `serverRoute`/`server_route`.
  The wire field name `ResourcePath.serverId` stays `serverId`
  (Decisions line 44-46; spec `#L233-238`).
- Verification boundary (Phase 2 lines 370-373): protected-route auth on new
  aliases; local-alias equivalence for representative routes; linked-server
  refusal states; bearer forwarding on ≥1 test remote route; upstream error
  preservation; resource-view rewriting.
- Completion also requires **extending the Phase 1 spec entry**
  (`#260703-ws-dashboard-server-route-scoped-operation-endpoints`) with the
  one-shot forwarding envelope, `server-local` alias behavior, and bounded
  gateway error shapes (Phase 2 lines 375-378; Spec Impact lines 253-262).

## Out of Scope

- SSE forwarding (document events, Activity events) and terminal WebSocket
  gatewaying — Phases 4-7 (Phase 2 line 366; Constraints lines 180-183).
- Full operation coverage (Git toolbar, files read/write, Activity, workspaces,
  terminals) — only the representative root-picker / open-workRoot / activation
  slice in `7462bad4` is in scope. Phases 3-6 forward the rest.
- Credential persistence, deployment automation, public endpoint hardening,
  multi-hop federation (Decisions lines 95-97; Phase 2 line 366).
- **terminal.rs is NOT touched by this phase.** Both Phase 2 source commits
  (`7462bad4`, `51a148b4`) touch only `router.rs`, `servers.rs`, and
  `tests/routes.rs`; the ticket's `terminal.rs` TERM-normalization conflict
  warning (line 224-227) applies to later terminal phases, not this one.

## Codebase Findings

- Phase 2 source commits (reference worktree
  `/home/swkang/devenv/.worktree/dashboard-server-scoped-forwarding-phase-7`,
  branch `implement/dashboard-server-scoped-forwarding-phase-7`):
  - `7462bad4` — feat skeleton: `router.rs` (+5 routes, import churn),
    `servers.rs` (+334 lines: handlers + forwarding helper), `tests/routes.rs`
    (+268). Uses `server_id`/`{server_id}`, no dot validation.
  - `51a148b4` — tests-only: `tests/routes.rs` (+179, `server-local` mutation
    alias coverage).
- `ws-dashboard/crates/daemon/src/servers.rs` — all helpers the `7462bad4` diff
  references **already exist** in current dev, so the servers.rs hunk is purely
  additive (no drift): `LOCAL_SERVER_ID` (`#L26`), `server_status` (`#L530`),
  `linked_server_refusal_message` (`#L561`), `server_error` (`#L593`),
  `rewrite_resources_for_linked_server` (`#L609`), `remote_url` (`#L757`),
  `dashboard_server_resources` (`#L427`, the existing forwarding precedent to
  mirror).
- `ws-dashboard/crates/daemon/src/root_picker.rs` — local dispatch targets exist
  with signatures matching the draft's call sites: `list_root_picker` (`#L112`,
  `State + Query`), `create_empty_directory` (`#L128`, `Json` only, no State),
  `pin_root_picker_directory` (`#L145`), `unpin_root_picker_directory` (`#L174`),
  `open_work_root` (`#L208`, `State + Json`), `set_work_root_activation`
  (`#L279`, `State + AxumPath + Json`); request structs at `#L83/#L90/#L96/#L102`.
- `ws-dashboard/crates/daemon/src/router.rs#L74-L217` — the protected route list;
  `7462bad4` inserts its 5 new routes right after the
  `servers/{server_id}/tunnel/reconnect` route (`#L91-94`). This is a clean
  insertion between two stable routes.
- **Router conflict is mechanical, not semantic.** The loopback no-auth debug
  logic (`bc799496`) lives at `router.rs#L219-220`
  (`if state.config.owner_auth_enabled { protected.layer(require_owner_auth) }`),
  **outside** the cherry-pick's hunk range (top imports + route list `#L74-217`).
  It survives untouched. The only real conflict is rustfmt import-ordering churn:
  the draft reorders `use crate::servers::{…}` / `use crate::terminal::{…}` etc.,
  and current dev (`#L33-37`) already sits in the newer grouping the draft moves
  toward — resolve by keeping current ordering and merging in the 5 new
  `server_scoped_*` names from `crate::servers`.
- `ws-dashboard/crates/daemon/tests/routes.rs` — existing integration-test
  harness the new tests extend (new fns:
  `server_scoped_one_shot_routes_are_protected_and_dispatch_local_aliases`,
  `server_scoped_one_shot_routes_return_bounded_refusals`,
  `linked_server_one_shot_forwarding_preserves_bearer_errors_and_rewrites_resources`,
  plus `51a148b4`'s `server-local` mutation-alias tests).
- **Risk signal — net-new work the reused commits do NOT contain:**
  (1) `server_id`→`server_route` rename in the phase's own new code;
  (2) daemon-side dot-free rejection (bounded invalid-route refusal). Both are
  required by the Decisions/spec for the landed diff to be compliant. Do not
  land the raw cherry-pick and defer these.

## Implementation Plan

1. Fetch/cherry-pick `7462bad4` then `51a148b4` (oldest first) onto
   `implement/phase-2-backend-local-aliases-forwarding`. Reuse the source
   commits via cherry-pick rather than reimplementing (ticket line 232-235).
   Do NOT replay the draft's docs/plan/closeout commits.
2. Resolve the `router.rs` cherry-pick conflict mechanically: keep current
   dev's import grouping and no-auth debug layer (`#L219-220`) intact; merge in
   the 5 new `server_scoped_*` imports from `crate::servers` and the 5 route
   registrations after `servers/{server_id}/tunnel/reconnect` (`#L94`). Confirm
   the no-auth path and route list both survive.
3. Within the same cherry-pick, apply the `serverId`→`serverRoute` /
   `server_id`→`server_route` rename to this phase's **own new code** in
   `servers.rs` and `router.rs`: the 5 new handler fn param bindings, the
   `{server_id}` path segments on the 5 new routes, `ServerScopedResolution` /
   `ServerScopedForwardOperation` internals, and helper params. Leave the wire
   field `ResourcePath.serverId` and `PersistedLinkedServer.id`/`server.id`
   usage as-is (wire/storage identity, per Decisions line 44-46).
   - Judgment note: the 3 pre-existing `servers/{server_id}/...` routes
     (`router.rs#L84/#L88/#L92`) are not in this phase's cherry-pick. The path
     param binding name is not browser-visible (URL structure is unchanged), so
     renaming them is optional consistency cleanup, not a contract requirement.
     Recommend renaming the sibling 3 to `{server_route}` in the same commit for
     a uniform diff; if the executor prefers minimal scope, leaving them is
     defensible — but do not leave the phase's *new* routes as `{server_id}`.
4. Add daemon-side dot-free validation (net-new; absent from `7462bad4`):
   in `resolve_server_scoped_forwarding` (servers.rs), before the linked-server
   lookup, reject a `server_route` containing `.` with a bounded
   `ServerScopedResolution::Refusal` (invalid-route message telling the owner to
   re-add the linked server under a dot-free Server Route; do not echo the
   value). This also covers existing persisted dotted linked-server ids without
   silently rewriting them (Decisions lines 74-77; Constraints line 167-169).
   Keep it leak-free via the existing `server_error` bounded-message style.
5. Extend the Phase 1 spec entry
   `ai-docs/spec/ws-web-dashboard/index.md#L229-279`
   (`#260703-ws-dashboard-server-route-scoped-operation-endpoints`) with: the
   one-shot forwarding envelope (local in-process dispatch vs. bearer-authed
   linked-server forward, `DashboardResourcesView`/`ResourcePath.serverId`
   rewrite to the selected route), `server-local` alias equivalence, the bounded
   gateway error set (unknown / invalid-route / auth-required / tunnel-required /
   unreachable / upstream-rejection), and the daemon-side dot-rejection now being
   authoritative. Reconcile against the existing prose; do not restate Phase 1.
6. Re-author the test coverage against current naming so the tests assert
   `server_route`/`serverRoute` route shapes and add a case for the new
   dot-rejection refusal.

## Verification Plan

- `cargo test -p ws-dashboard-daemon --test routes` (from
  `ws-dashboard/crates/daemon`) — new server-scoped tests + existing local
  route/alias tests must pass; confirms protected-route auth, local-alias
  equivalence, refusal states, bearer forwarding, upstream error preservation,
  and resource-view rewriting (Phase 2 lines 370-373).
- Add/confirm a test asserting a dotted `server_route` yields the bounded
  invalid-route refusal (net-new logic in step 4).
- `cargo build -p ws-dashboard-daemon` (or workspace build) to confirm the
  rename compiles cleanly across router/servers.
- Confirm the extended spec entry stays anchor-valid after the edit
  (`ws/spec_index_verify` or equivalent).

## Escalations

- None. Confidence: high. The forwarding helper, error enum, and resolution
  types come ready-made from `7462bad4` with all referenced helpers pre-existing;
  the only judgment calls (router import-churn resolution, param-rename scope,
  where to insert dot validation) are bounded and specified above. Escalate to
  research only if step-2 conflict resolution reveals unexpected semantic drift
  in the no-auth debug layer, or if `cargo test` surfaces a signature mismatch
  in a local dispatch target not caught by the survey.
