# Survey: 260514-feat-ws-web-daemon-foundation

## Reusable Components
- `ws-dashboard/crates/daemon/src/main.rs#L4-L10` — thin `ws-dashboard` binary: parses `Cli`, initializes logging, and delegates all serving behavior to the daemon library.
- `ws-dashboard/crates/daemon/src/cli.rs#L5-L47` — `Cli`, `Command::Serve`, and `ServeArgs`: existing Clap public shape for `ws-dashboard serve --host --port --static-dir` plus `--log-filter`.
- `ws-dashboard/crates/daemon/src/config.rs#L6-L26` — `ServeConfig`: central normalization point for bind address and optional static directory.
- `ws-dashboard/crates/daemon/src/auth.rs#L3-L60` — `OwnerAuthState`, `PairingToken`, `OwnerSessionCookie`, `PairingOutcome`: skeleton auth surface already names one-time pairing, session issue, and header auth checks.
- `ws-dashboard/crates/daemon/src/router.rs#L6-L20` — `AppState` and `build_router`: existing Axum router seam for tests and server startup.
- `ws-dashboard/crates/daemon/src/server.rs#L6-L24` — `StartupInfo`, `run`, and `startup_info`: server startup seam with testable pairing URL construction.
- `ws-dashboard/crates/daemon/src/logging.rs#L1-L6` — `logging::init`: structured logging hook already isolated from request/server logic.
- `ws-dashboard/crates/harness-core/src/redaction.rs#L1-L12` — `SecretFilter`/`NoopSecretFilter`: existing secret-filtering vocabulary, though the current implementation does not redact query tokens.

## Existing Patterns
- Nested dashboard workspace: see `ws-dashboard/Cargo.toml#L1-L27` and `ws-dashboard/crates/daemon/Cargo.toml#L7-L25` — dependencies for Axum, Tokio, rand, tower-http, tracing, and tower tests are already declared in the dashboard workspace, not the repo root.
- Router tests use in-process services: see `ws-dashboard/crates/daemon/tests/routes.rs#L13-L24` and `ws-dashboard/crates/daemon/tests/routes.rs#L31-L65` — tests are designed around `tower::ServiceExt::oneshot` against `build_router`, avoiding socket binding for route behavior.
- Server tests keep startup pure where possible: see `ws-dashboard/crates/daemon/tests/server.rs#L15-L36` — default bind and pairing URL construction are asserted without starting a listener.
- Future dashboard resource IDs are opaque: see `ws-dashboard/crates/core/src/ids.rs#L1-L29` and `ws-dashboard/crates/core/src/resources.rs#L36-L42` — resource naming exists but should not leak into Phase 1 health output or URL identity.
- Harness split is separate from daemon auth: see `ws-dashboard/crates/harness-core/src/capabilities.rs#L1-L11` and `ws-dashboard/crates/harness-cli/src/main.rs#L3-L13` — harness capabilities are their own crate/binary surface.

## Relevant Interfaces
- `ws-dashboard/crates/daemon/src/auth.rs#L16-L21` — `PairingOutcome`: callers can distinguish valid, invalid, and reused pairing attempts.
- `ws-dashboard/crates/daemon/src/auth.rs#L31-L53` — `OwnerAuthState` methods: pairing token exposure, token consumption, cookie issuance, and request-header auth are the auth integration points.
- `ws-dashboard/crates/daemon/src/cli.rs#L20-L34` — `ServeArgs`: host/port/static-dir fields are the only current CLI inputs for serving configuration.
- `ws-dashboard/crates/daemon/src/config.rs#L14-L26` — `ServeConfig::default_loopback` and `ServeConfig::from_args`: tests and CLI both expect config normalization here.
- `ws-dashboard/crates/daemon/src/router.rs#L6-L12` — `AppState`: router state clones config and auth together, so shared auth state must survive Axum cloning.
- `ws-dashboard/crates/daemon/src/server.rs#L6-L10` — `StartupInfo`: bound address and owner pairing URL are the startup observable data.
- `ai-docs/spec/ws-web-dashboard/index.md#L10-L39` — daemon foundation behavior: loopback is default reachability only, `/pair` bootstraps owner cookie auth, and other routes remain auth-gated.
- `ai-docs/mental-model/mcp-runtime.md#L20-L38` and `ai-docs/mental-model/plugin-runtime.md#L20-L45` — ws MCP/plugin authority boundaries the daemon must not absorb.

## Constraints
- Phase 1 scope is narrower than the full spec/ticket: `ai-docs/.plans/2026-05/15-260514-feat-ws-web-daemon-foundation.brief.md#L31-L45` excludes token TTL/persistence, complete Host/Origin policy, public bind modes, WebSockets, PTYs, workspace discovery, wsstate view models, and frontend package setup.
- `/pair` is the only unauthenticated browser route; health and placeholder UI are authenticated: `ai-docs/tickets/ready/260514-feat-ws-web-daemon-foundation.md#L82-L96`.
- Health output must stay minimal and omit host paths, cache paths, Git roots, wsstate internals, tokens, session IDs, and diagnostics: `ai-docs/tickets/ready/260514-feat-ws-web-daemon-foundation.md#L57-L60`.
- Public bind behavior is not Phase 1; unsupported non-loopback binds should fail closed rather than partially enabling Phase 3: `ws-dashboard/crates/daemon/src/cli.rs#L20-L34` and `ai-docs/tickets/ready/260514-feat-ws-web-daemon-foundation.md#L120-L132`.
- Request logging must not leak pairing query strings; `tower-http` is available at `ws-dashboard/Cargo.toml#L24-L27`, but default URI tracing would need scrutiny because `/pair?token=...` is sensitive.
- The daemon remains separate from ws MCP stdio session authority and named-agent ownership: `ai-docs/spec/ws-web-dashboard/index.md#L25-L28`, `ai-docs/mental-model/mcp-runtime.md#L35-L38`, and `ai-docs/mental-model/named-agent-runtime.md#L20-L28`.

## Opinion
- Main implementation risk is shared auth state: `OwnerAuthState` currently derives `Clone` over plain token data, but one-time token consumption and session reuse need clone-safe interior state for router clones.
- Test scaffolds use a placeholder cookie header in `ws-dashboard/crates/daemon/tests/routes.rs#L27-L29` while `issue_session_cookie` returns an opaque wrapper at `ws-dashboard/crates/daemon/src/auth.rs#L13-L14`; tests will likely need to consume the real `Set-Cookie` value instead of preserving the placeholder.
- There is no existing in-repo Axum server implementation beyond this skeleton, so the skeleton and declared crate dependencies are the authoritative local pattern.
- Spec wording around Host/Origin validation could over-expand Phase 1; the brief explicitly defers complete Host/Origin policy, so only avoid weakening the future auth boundary while implementing the shell.
