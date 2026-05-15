# Brief: 260514-feat-ws-web-daemon-foundation Phase 3

## Intent

Implement bind-mode guards for the dashboard daemon foundation. The daemon
already has an auth-gated server shell and Phase 2 owner-auth hardening; this
phase adds explicit local, tunnel, and public bind-mode configuration so
accidental public exposure fails closed while intentional public mode is allowed
only under owner-auth guardrails.

## Approach

- Implement the Phase 3 skeleton contracts from commits `5ad6d25` and
  `ab86437`.
- Keep `ws-dashboard serve` as the public command and keep the binary thin.
- Preserve local loopback binding as the default.
- Keep tunnel mode loopback-oriented unless the caller supplies only loopback
  hosts.
- Require `--bind-mode public` before any non-loopback host such as `0.0.0.0`
  can be accepted.
- Require owner authentication to remain enabled before public mode can pass
  validation.
- Keep bind-mode validation pure/config-level where possible so tests do not
  require public network exposure.
- Activate and pass the ignored Phase 3 skeleton tests when the corresponding
  guard behavior is implemented.

## Constraints

- Scope is Phase 3 only. Do not implement Phase 4 end-to-end security smoke.
- Do not add PTY terminals, workspace/resource APIs, named-agent panels,
  dashboard feature panels, or WebSocket endpoint payload behavior.
- Do not weaken the Phase 2 owner-auth boundary; public serving does not bypass
  cookie/bearer auth, Host/Origin checks, or WebSocket pre-upgrade auth.
- Public mode must be explicit and actionable when rejected.
- Tests must not require binding a public network interface.
- Health output must remain exactly minimal and secret-free.

## Out of scope

Phase 4 security smoke, public-internet hardening beyond the configured
bind-mode fail-closed checks, durable auth storage, RBAC, PTY process lifecycle,
workspace discovery, wsstate view-model APIs, and real frontend assets.

## Details

- Existing Phase 3 skeleton final commit: `5ad6d25`.
- Ticket skeleton frontmatter update: `ab86437`.
- Implement mainly in `ws-dashboard/crates/daemon/src/cli.rs` and
  `ws-dashboard/crates/daemon/src/config.rs`.
- Integration targets live in `ws-dashboard/crates/daemon/tests/server.rs`.
- Key tests to activate and pass:
  - `accidental_public_bind_requires_explicit_public_mode`
  - `explicit_public_bind_mode_accepts_public_host_with_owner_auth`
  - `public_bind_mode_requires_owner_auth`
- Keep existing daemon route tests passing.
- Run at least `cargo test -p ws-dashboard-daemon` and `cargo test --workspace`.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` - daemon foundation
  bind-mode behavior and owner-auth guardrails.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - daemon entrypoints,
  owner-auth boundary, Host/Origin checks, and bind-mode change recipe.
- [Must] `ai-docs/tickets/ready/260514-feat-ws-web-daemon-foundation.md` -
  Phase 3 scope only; Phase 4 is excluded.
