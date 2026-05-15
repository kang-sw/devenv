---
domain: ws-web-dashboard
description: "Personal ws dashboard daemon, owner-auth boundary, UI serving, core resource vocabulary, and host-control separation."
sources:
  - ws-dashboard/
related:
  mcp-runtime: "Dashboard code may consume ws runtime state through daemon-owned view models, but it must not become ws MCP session authority."
---

# ws Web Dashboard

## Entry Points

- `ws-dashboard/crates/daemon/src/main.rs`, `cli.rs`, and `config.rs` keep `ws-dashboard serve` as a thin command adapter over normalized serving config.
- `ws-dashboard/crates/daemon/src/server.rs` owns listener binding, startup auth state creation, pairing URL emission, and graceful shutdown.
- `ws-dashboard/crates/daemon/src/router.rs` and `auth.rs` own the browser route boundary, one-time pairing expiry, bearer-auth exception, Host/Origin entrypoint checks, and owner session cookie checks. {#260515-ws-web-daemon-foundation}
- `ws-dashboard/crates/core/src/ids.rs` and `resources.rs` own the public dashboard resource vocabulary shared by later API, frontend, discovery, and event-stream work. {#260516-ws-web-dashboard-core-resource-vocabulary}

## Module Contracts

- Loopback is only a reachability default, not authorization: every browser route except `/pair` must pass owner-session auth before handler execution, including fallback paths. {#260515-ws-web-daemon-foundation}
- The pairing URL is constructed after the listener is bound so port `0` resolves to the actual socket; changing startup reporting must preserve a single owner-visible pairing URL without leaking it through request diagnostics.
- `OwnerAuthState` is cloned into Axum state, so auth storage changes must preserve shared one-time pairing consumption, pairing TTL enforcement before consumption, and rejection of session cookies before pairing succeeds.
- Bearer auth is a daemon-local protected-route exception for CLI/smoke callers; it authenticates before cookie pairing state is considered, so browser cookie flow changes must not accidentally remove or broaden that non-browser path.
- Authenticated browser requests and WebSocket upgrade requests pass conservative Host/Origin checks; missing headers are tolerated for ordinary clients, but clearly non-loopback hosts/origins fail before handler or upgrade behavior.
- Health output is deliberately exact and minimal (`ok\n`); host paths, cache paths, Git roots, wsstate data, diagnostics, pairing tokens, and session values belong only in authenticated diagnostic surfaces.
- Bind reachability and browser authorization stay separate: local/tunnel modes keep non-loopback hosts rejected, while public mode may accept non-loopback hosts only when owner auth remains enabled. Relaxing Host/Origin or route auth is not part of public bind enablement.
- The daemon is not ws MCP authority. Future wsstate or named-agent views should be daemon-owned projections rather than adopting the caller's MCP root, harness, model backend, or agent session ownership.
- Dashboard core exposes the physical open/spawn/run target as `workRoot`, not `worktree`; serialized resource paths must keep `workRootId` camelCase vocabulary so later HTTP routes and fixtures do not leak Git-worktree-specific names into the generic directory hierarchy. {#260516-ws-web-dashboard-core-resource-vocabulary}

## Coupling

- `server.rs` creates the auth state used by the router; route tests that build `AppState` directly must stay aligned with the server startup path.
- `static_dir` is accepted in config before static serving exists. When assets are wired, they must be added under the protected router/layer, not as another top-level unauthenticated route.
- Startup output intentionally prints the pairing URL for the local owner, while structured logs should avoid query-string/token material; request logging changes must preserve that split.

## Extension Points & Change Recipes

- **Add an authenticated HTTP route or static asset serving**: add it inside the protected router, then add both unauthenticated rejection and paired-cookie success tests.
- **Change pairing, session, or bearer behavior**: update `auth.rs`, `/pair` status/cookie handling, and route tests together; preserve one-time consumption, expiry failure without cookie installation, no pre-pair session-cookie authentication, and the narrow bearer path for protected HTTP smoke callers.
- **Change bind-mode guardrails**: update CLI vocabulary, config validation, and server tests together; preserve local/tunnel rejection for non-loopback hosts, public-mode owner-auth requirement, and the separation from browser Host/Origin authorization.
- **Expose diagnostics**: add an authenticated route; do not expand `/healthz` beyond the minimal body contract.
- **Change core resource vocabulary**: update `ids.rs`, `resources.rs`, public re-exports, and serde contract tests together; keep `workRoot` naming stable unless the dashboard spec and dependent API/fixture tickets are intentionally revised.

## Common Mistakes

- Adding a new route to the top-level router beside `/pair`, which bypasses owner auth.
- Reintroducing `WorktreeId`, `worktreeId`, or `worktree_id` at the public dashboard core/API boundary; linked Git worktrees are a `WorkRootKind`, not the identity vocabulary.
- Treating `localhost`, `127.0.0.1`, explicit public bind mode, or passing Host/Origin checks as sufficient authorization for host-control features.
- Logging full request URIs or health payloads that include pairing tokens, session cookies, wsstate internals, paths, or Git roots.
- Building startup URLs before binding the listener and accidentally exposing `:0` to the owner.

## Technical Debt

- Pairing, bearer, and session secrets are process-memory only and have no persistence yet.
- `static_dir` is parsed but not served in the foundation shell.
