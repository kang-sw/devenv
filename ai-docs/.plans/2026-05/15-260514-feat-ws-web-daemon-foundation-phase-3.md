# Survey: 15-260514-feat-ws-web-daemon-foundation-phase-3

## Reusable Components
- `ws-dashboard/crates/daemon/src/cli.rs#L20-L39` — `ServeArgs`: existing Clap `serve` argument surface already carries `--host`, `--bind-mode`, `--port`, and `--static-dir`; Phase 3 can keep the public command thin.
- `ws-dashboard/crates/daemon/src/cli.rs#L42-L51` — `BindMode`: local/tunnel/public enum is already a Clap `ValueEnum` with local default and public intent vocabulary.
- `ws-dashboard/crates/daemon/src/config.rs#L8-L20` — `ServeConfig`: central normalized serving config already records bind address, static dir, bind mode, and owner-auth enabled state.
- `ws-dashboard/crates/daemon/src/config.rs#L23-L44` — `ServeConfig::default_loopback` and `ServeConfig::from_args`: default/test/CLI normalization seam; all current bind guard decisions flow through `from_args`.
- `ws-dashboard/crates/daemon/src/config.rs#L47-L69` — `validate_bind_guard`: pure guard function for host/mode/auth checks; skeleton already rejects local-mode public hosts and public mode without auth.
- `ws-dashboard/crates/daemon/src/auth.rs#L110-L169` — `OwnerAuthState` auth entrypoints: cookie/bearer auth plus Host/Origin/WebSocket pre-upgrade gates that public bind must not bypass.
- `ws-dashboard/crates/daemon/src/router.rs#L19-L33` — `build_router`: `/pair` stays top-level while `/healthz`, `/`, and fallback are protected through `require_owner_auth`.
- `ws-dashboard/crates/daemon/src/server.rs#L23-L45` — `run_with_shutdown`: startup binds `ServeConfig::bind_addr`, creates auth state, emits one pairing URL, and injects config/auth into router state.

## Existing Patterns
- Thin binary pattern: see `ws-dashboard/crates/daemon/src/main.rs#L1-L10` — CLI parsing, logging init, config normalization, and server execution stay outside `main` logic.
- Config-level tests avoid public sockets: see `ws-dashboard/crates/daemon/tests/server.rs#L23-L103` — bind-mode contracts are asserted through `ServeConfig::from_args` and `validate_bind_guard`.
- Startup smoke stays loopback-only: see `ws-dashboard/crates/daemon/tests/server.rs#L105-L126` — server execution tests bind default loopback with shutdown rather than public interfaces.
- Route auth regression tests use in-memory Axum services: see `ws-dashboard/crates/daemon/tests/routes.rs#L29-L58` and `ws-dashboard/crates/daemon/tests/routes.rs#L75-L110` — existing owner-auth coverage should remain socket-free.
- Phase 2 auth coverage protects local Host/Origin and bearer behavior: see `ws-dashboard/crates/daemon/tests/routes.rs#L216-L331` — useful regression set when public bind config changes.

## Relevant Interfaces
- `ws-dashboard/crates/daemon/tests/server.rs#L53-L103` — Phase 3 skeleton tests: remove/replace the fail-closed skeleton test at lines 53-64 and activate `accidental_public_bind_requires_explicit_public_mode`, `explicit_public_bind_mode_accepts_public_host_with_owner_auth`, and `public_bind_mode_requires_owner_auth`.
- `ws-dashboard/crates/daemon/src/config.rs#L71-L78` — `parse_bind_host`: accepts IP literals and `localhost` only; tunnel/public host semantics depend on this parser.
- `ws-dashboard/crates/daemon/src/auth.rs#L247-L337` — Host/Origin allowlist helpers: currently loopback-only and independent from `ServeConfig::bind_mode`; changing public bind must not silently broaden browser entrypoints unless explicitly intended.
- `ws-dashboard/crates/daemon/src/router.rs#L60-L81` — `require_owner_auth`: all non-pair routes, including WebSocket upgrade-shaped requests, use the same owner-auth gate before handlers.
- `ws-dashboard/crates/daemon/Cargo.toml#L11-L25` — daemon dependencies already include Clap, Axum, Tokio, Tower, tracing, and rand; no new dependency appears necessary for pure bind-mode validation.

## Constraints
- `ai-docs/.plans/2026-05/15-260514-feat-ws-web-daemon-foundation-phase-3.brief.md#L11-L26` — Phase 3 requires explicit public mode for non-loopback hosts, owner auth for public mode, loopback defaults, tunnel loopback orientation, and pure/config-level validation where possible.
- `ai-docs/.plans/2026-05/15-260514-feat-ws-web-daemon-foundation-phase-3.brief.md#L28-L43` — no Phase 4 smoke, PTY, workspace/API, named-agent, dashboard panel, WebSocket payload, durable auth, RBAC, or frontend-asset work.
- `ai-docs/spec/ws-web-dashboard/index.md#L33-L52` — `/pair` remains the only unauthenticated browser entrypoint, Host/Origin checks remain part of auth, WebSocket upgrades stay pre-gated, and health remains exactly `ok\n` without secrets.
- `ai-docs/tickets/ready/260514-feat-ws-web-daemon-foundation.md#L148-L162` — public bind requires explicit opt-in and owner auth; bind decisions must be testable without public network exposure.
- `ai-docs/mental-model/ws-web-dashboard.md#L20-L27` — loopback is not authorization, cloned auth state must remain shared, and bind-mode work must not be implemented by simply relaxing Host/Origin parsing.
- `ai-docs/mental-model/ws-web-dashboard.md#L31-L33` — `server.rs` and direct `AppState` route tests are coupled; startup pairing URL output and structured logs intentionally split token-bearing output from logs.

## Opinion
- The implementation surface is compact and mostly skeleton-ready: the main code gap is the final `bail!("public bind mode is not implemented yet...")` in `validate_bind_guard`, plus activating the ignored Phase 3 tests.
- The highest regression risk is conflating serving reachability with browser entrypoint validation: accepting `0.0.0.0` in `ServeConfig` should not automatically make arbitrary Host/Origin values pass the owner-auth middleware.
- `owner_auth_enabled` has no CLI disable knob and is always true in `from_args`; the negative public-auth test already reaches the pure guard directly, so no public auth-disable surface is needed for this phase.
