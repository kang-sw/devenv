---
domain: ws-web-dashboard
description: "Personal ws dashboard daemon, owner-auth boundary, UI serving/route basis, frontend visual system, resource view-model API/fixtures, and host-control separation."
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
- `ws-dashboard/crates/core/src/events.rs` owns the public instance event envelope shared by later PTY, named-agent, exec, diagnostic, viewer, and translation streams. {#260516-ws-web-dashboard-instance-event-envelope-fixtures}
- `ws-dashboard/crates/core/src/view_model.rs`, `daemon/src/resources.rs`, and `daemon/src/mock.rs` own the first authenticated resource hierarchy API and fixture-backed provider seam. {#260516-ws-web-dashboard-resource-view-model-contract} {#260516-ws-web-dashboard-mock-view-model-fixtures}
- `ws-dashboard/crates/daemon/src/events.rs` owns deterministic fixture-backed instance transcripts, cursor backfill behavior, and the authenticated instance event route scaffold until live stream sources exist. {#260516-ws-web-dashboard-instance-event-envelope-fixtures} {#260516-ws-web-dashboard-authenticated-instance-event-stream-scaffold}
- `ws-dashboard/crates/daemon/src/discovery.rs` owns live local workRoot discovery and maps remembered/opened paths into the same resource view-model provider contract. {#260516-ws-web-dashboard-local-workroot-discovery-provider}
- `ws-dashboard/crates/daemon/src/root_picker.rs` owns the authenticated backend root picker, empty-directory creation, and open-workRoot route handlers. {#260516-ws-web-dashboard-root-picker-empty-directory-creation}
- `ws-dashboard/frontend/` owns the React/Vite browser shell that is served by the daemon when `--static-dir` points at its production build output and renders the first inspectable resource hierarchy. {#260516-ws-web-dashboard-protected-frontend-shell} {#260516-ws-web-dashboard-inspectable-navigation-shell}
- `ws-dashboard/frontend/src/routeBasis.ts` owns browser-path normalization to the daemon-reported server id; it is route chrome, not resource authority. {#260516-ws-web-dashboard-server-scoped-browser-routes}
- `ws-dashboard/frontend/src/workbench/` owns the dashboard workbench registry, layout attachment identity, sanitized layout serialization, and Dockview bridge boundary before the visible split-group shell is wired in. {#260516-ws-web-dashboard-workroot-workbench-substrate}
- `ws-dashboard/frontend/DESIGN.md` and `frontend/src/styles.css` own the dashboard-local dark visual vocabulary for browser UI work. {#260516-ws-web-dashboard-dark-visual-system}

## Module Contracts

- Loopback is only a reachability default, not authorization: every browser route except `/pair` must pass owner-session auth before handler execution, including fallback paths. {#260515-ws-web-daemon-foundation}
- The pairing URL is constructed after the listener is bound so port `0` resolves to the actual socket; changing startup reporting must preserve a single owner-visible pairing URL without leaking it through request diagnostics.
- Valid `/pair?token=...` exchanges consume the token, install the owner cookie, and redirect to token-free `/`; missing, invalid, reused, or expired tokens must stay non-redirecting and cookie-free so browser history and retry paths do not retain usable pairing URLs. {#260516-ws-web-dashboard-token-free-pairing-landing}
- `OwnerAuthState` is cloned into Axum state, so auth storage changes must preserve shared one-time pairing consumption, pairing TTL enforcement before consumption, and rejection of session cookies before pairing succeeds.
- Bearer auth is a daemon-local protected-route exception for CLI/smoke callers; it authenticates before cookie pairing state is considered, so browser cookie flow changes must not accidentally remove or broaden that non-browser path.
- Authenticated browser requests and WebSocket upgrade requests pass conservative Host/Origin checks; missing headers are tolerated for ordinary clients, but clearly non-loopback hosts/origins fail before handler or upgrade behavior.
- Health output is deliberately exact and minimal (`ok\n`); host paths, cache paths, Git roots, wsstate data, diagnostics, pairing tokens, and session values belong only in authenticated diagnostic surfaces.
- Bind reachability and browser authorization stay separate: local/tunnel modes keep non-loopback hosts rejected, while public mode may accept non-loopback hosts only when owner auth remains enabled. Relaxing Host/Origin or route auth is not part of public bind enablement.
- The daemon is not ws MCP authority. Future wsstate or named-agent views should be daemon-owned projections rather than adopting the caller's MCP root, harness, model backend, or agent session ownership.
- Dashboard core exposes the physical open/spawn/run target as `workRoot`, not `worktree`; serialized resource paths must keep `workRootId` camelCase vocabulary so later HTTP routes and fixtures do not leak Git-worktree-specific names into the generic directory hierarchy. {#260516-ws-web-dashboard-core-resource-vocabulary}
- The resource view-model API preserves `server -> workspace -> workRoot -> mainInstance -> subInstance` as data even when rows are compactable; compaction is a browser presentation hint, not a route identity or daemon-side pre-collapse. {#260516-ws-web-dashboard-resource-view-model-contract}
- The daemon resource route belongs inside the protected router and returns the public core view-model shape through a provider seam; adding live discovery later must swap/extend the provider without changing fixture/frontend-facing JSON vocabulary. {#260516-ws-web-dashboard-resource-view-model-contract}
- `daemon/tests/fixtures/dashboard_resources.json` is the shared golden mock artifact; Rust mock data and route tests must consume that fixture instead of maintaining a second in-code sample. {#260516-ws-web-dashboard-mock-view-model-fixtures}
- Local discovery preserves workRoot identity from the remembered candidate path, not the canonical symlink target, so status or kind changes do not churn `resourcePath.workRootId`. {#260516-ws-web-dashboard-local-workroot-discovery-provider}
- Existing directories must be readable before they are classified as online Git/plain roots; metadata success alone is not enough because unreadable directories must surface as inaccessible. {#260516-ws-web-dashboard-local-workroot-discovery-provider}
- Root picker routes belong inside the same owner-auth protected router as other dashboard APIs; they may expose host paths only as authenticated picker/open request data, never as public resource ids. {#260516-ws-web-dashboard-root-picker-empty-directory-creation}
- The only filesystem mutation in the first root picker backend is single-segment empty-directory creation; generic delete, rename, move, copy, and recursive folder operations stay absent. {#260516-ws-web-dashboard-root-picker-empty-directory-creation}
- Static dashboard serving is configuration-gated: `/`, `/servers`, and `/servers/{*app_path}` serve `static_dir/index.html`, `/assets/{*asset_path}` serves only safe relative paths below `static_dir/assets`, and all stay inside the owner-auth protected router. Without `static_dir`, the daemon keeps the minimal fallback HTML and asset requests 404. {#260516-ws-web-dashboard-protected-frontend-shell} {#260516-ws-web-dashboard-server-scoped-browser-routes}
- The browser shell consumes `/api/dashboard/resources` as its source of truth, not a copied frontend fixture or browser route params; refresh failures after a successful load must stay visible while stale rows remain inspectable. After resources load, the frontend may replace `/`, `/servers`, or mismatched `/servers/:serverId...` with the daemon-reported server route while preserving query/hash, but this normalization must not make the browser path authoritative over resource ids. {#260516-ws-web-dashboard-inspectable-navigation-shell} {#260516-ws-web-dashboard-server-scoped-browser-routes}
- Frontend mouse actions carry `data-command-id` command identities so later keyboard bindings can dispatch the same command layer instead of forking interaction behavior. {#260516-ws-web-dashboard-inspectable-navigation-shell}
- Workbench layout attachments are browser-side view attachments, not daemon resources. Serialized workbench layout may keep attachment ids, arrangement, and active attachment identity, but daemon resource ids, surface kind metadata, and registry-derived row policy stay in the dashboard model/registry rather than persisted layout JSON. {#260516-ws-web-dashboard-workroot-workbench-substrate}
- Dockview is a mechanical layout substrate behind the dashboard workbench bridge. New workbench behavior should add dashboard-owned policy around creation, move/drop validation, focus, close/detach, and restore sanitization instead of returning raw Dockview panel/group handles or making Dockview lifecycle APIs product-level capabilities. {#260516-ws-web-dashboard-workroot-workbench-substrate}
- Dashboard frontend components use the local semantic token layer in `styles.css`; visual work should remap or extend tokens instead of hardcoding raw light-theme colors, rounded cards, shadows, or gradients into feature components. {#260516-ws-web-dashboard-dark-visual-system}
- Instance events carry stream and resource identity on each event, not only on an outer transcript, so future streaming routes can emit individual events without an out-of-band identity envelope. {#260516-ws-web-dashboard-instance-event-envelope-fixtures}
- Unknown cursors do not replay a full transcript; fixture backfill returns an empty event set for unrecognized cursors and reserves missing-stream behavior for unknown stream ids. {#260516-ws-web-dashboard-instance-event-envelope-fixtures}
- The instance event route is authenticated and fixture-backed; it must not bind the daemon to live PTY, named-agent, exec, diagnostic, viewer, translation, ws MCP, or named-agent session authority until later source-specific tickets add those producers. {#260516-ws-web-dashboard-authenticated-instance-event-stream-scaffold}

## Coupling

- `server.rs` creates the auth state used by the router; route tests that build `AppState` directly must stay aligned with the server startup path.
- `static_dir` couples CLI config, `router.rs`, route tests, and frontend build output: production UI availability depends on passing the built `frontend/dist` directory, while auth behavior for `/`, `/servers...`, and assets is enforced by router placement rather than frontend code.
- Startup output intentionally prints the pairing URL for the local owner, while structured logs should avoid query-string/token material; request logging changes must preserve that split.
- Dashboard resource JSON couples `core/src/view_model.rs`, the daemon provider trait, the protected route tests, and the golden fixture; field or hierarchy changes must update all four in one logical change.
- Live discovery is intentionally behind the provider seam while `/api/dashboard/resources` remains mock-backed; switching routes to live data must preserve the mock fixture path for deterministic frontend and contract tests.
- Root picker open routes couple `root_picker.rs` to `discovery.rs`: opening an existing directory returns the same dashboard resource view-model shape as the provider, so provider field changes ripple into route tests.
- The frontend resource shell couples `frontend/src/App.tsx` to the core JSON contract by TypeScript shape only; changing API field names requires updating Rust serde tests, golden fixture, route tests, and the React view together.
- `DESIGN.md` and the semantic variables in `styles.css` are a dashboard-local visual contract; component styling changes should keep that guide and token vocabulary synchronized without using daemon API or command-id changes as visual-system workarounds.
- Instance event JSON couples `core/src/events.rs`, daemon fixture transcripts, and future stream routes; changing cursor, category, payload, or resource identity fields must keep fixture and serialization tests aligned.
- Instance route tests are the auth boundary for the stream scaffold; adding SSE/WebSocket transport later must preserve rejection before stream acceptance or upgrade behavior.

## Extension Points & Change Recipes

- **Add an authenticated HTTP route or static asset family**: add it inside the protected router, then add both unauthenticated rejection and paired-cookie success tests; static file serving must keep traversal rejection before filesystem reads.
- **Change pairing, session, or bearer behavior**: update `auth.rs`, `/pair` redirect/cookie handling, and route tests together; preserve token-free success redirect, one-time consumption, failure paths without redirects or cookie installation, no pre-pair session-cookie authentication, and the narrow bearer path for protected HTTP smoke callers.
- **Change bind-mode guardrails**: update CLI vocabulary, config validation, and server tests together; preserve local/tunnel rejection for non-loopback hosts, public-mode owner-auth requirement, and the separation from browser Host/Origin authorization.
- **Expose diagnostics**: add an authenticated route; do not expand `/healthz` beyond the minimal body contract.
- **Change core resource vocabulary**: update `ids.rs`, `resources.rs`, public re-exports, and serde contract tests together; keep `workRoot` naming stable unless the dashboard spec and dependent API/fixture tickets are intentionally revised.
- **Change resource view-model shape**: update `core/src/view_model.rs`, the golden JSON fixture, mock provider assumptions, and authenticated route tests together; preserve full hierarchy unless the spec intentionally changes compaction semantics.
- **Replace mock resources with live discovery**: switch routing/config to select the live provider, keep the mock fixture for deterministic frontend/contract tests, and do not pull ws MCP session authority, PTY streams, named-agent ownership, or filesystem-discovery policy into the core structs.
- **Extend root picker operations**: add only explicit owner-authenticated operations with route tests for unauthenticated rejection and success; destructive or broad file-manager verbs require a new ticket and should not be piggybacked onto picker listing/open behavior.
- **Add live instance stream sources**: produce the shared event envelope from PTY, named-agent, exec, diagnostic, viewer, or translation sources; do not define feature-specific stream identity or cursor formats outside `core/src/events.rs`.
- **Change instance stream transport**: keep fixture-backed finite JSON as a deterministic contract test path while adding SSE/WebSocket or live producers behind the same auth gate and event envelope.
- **Extend the browser shell beyond inspectable navigation**: keep feature depth layered on top of the existing resource shell, route visible actions through command ids, use the dark semantic token system for new surfaces, and preserve the reserved viewer region until a later viewer ticket implements real document/editor/terminal behavior. {#260516-ws-web-dashboard-inspectable-navigation-shell} {#260516-ws-web-dashboard-dark-visual-system}
- **Add a workbench surface or lifecycle behavior**: register surface kind and row/lifecycle policy in the workbench registry, keep daemon identity as model metadata, serialize only attachment arrangement, and expose any Dockview operation through the workbench bridge rather than raw Dockview handles. {#260516-ws-web-dashboard-workroot-workbench-substrate}

## Common Mistakes

- Adding a new route to the top-level router beside `/pair`, which bypasses owner auth.
- Serving frontend assets through a separate unauthenticated static-file service or fallback route; the frontend is not an auth boundary and must rely on daemon owner auth.
- Reintroducing `WorktreeId`, `worktreeId`, or `worktree_id` at the public dashboard core/API boundary; linked Git worktrees are a `WorkRootKind`, not the identity vocabulary.
- Pre-collapsing singleton workspace/workRoot/mainInstance chains in the daemon and breaking stable row identity for later URLs and refresh updates.
- Editing Rust mock constructors without updating `dashboard_resources.json`; the fixture is the source of truth for deterministic mock responses.
- Hashing canonical paths for remembered workRoot ids; symlink targets and missing paths can change while the dashboard must keep the same remembered workRoot identity.
- Duplicating dashboard resource fixtures in React state, hiding post-load refresh errors, or trusting `/servers/:serverId` over `/api/dashboard/resources`; all make the browser disagree with the daemon-owned resource view.
- Persisting workbench surface kind, row policy, or daemon ids in layout JSON; those belong to the dashboard registry/resource model and make restored layouts authoritative over daemon state.
- Returning raw Dockview panel or group handles from dashboard workbench APIs; that bypasses the adapter policy for detach, placement, floating/popout, and later PTY/TUI resize constraints.
- Adding new dashboard UI with raw light palette values, rounded cards, decorative shadows, or gradients instead of the semantic dark tokens and square operational style.
- Treating `localhost`, `127.0.0.1`, explicit public bind mode, or passing Host/Origin checks as sufficient authorization for host-control features.
- Logging full request URIs or health payloads that include pairing tokens, session cookies, wsstate internals, paths, or Git roots.
- Building startup URLs before binding the listener and accidentally exposing `:0` to the owner.

## Technical Debt

- Pairing, bearer, and session secrets are process-memory only and have no persistence yet.
