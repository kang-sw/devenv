---
title: "Dashboard daemon drops LAN serving (T1: accept-and-warn for one minor, then delete)"
related:
  261112-research-ws-ade-desktop-shell-and-rename: "Werkschaft concept: local-only product, (490)-(492)"
  261113-feat-ws-dashboard-browser-pane: "its LAN-token cookie strip (decision 17) ends with the cookie"
  261115-feat-ws-dashboard-settings-in-usage-strip: "interim LAN-box relocation only while LAN exists; removal remains authoritative in either landing order"
---

# Dashboard daemon drops LAN serving (T1: accept-and-warn for one minor, then delete)

## Background

Owner direction, 2026-11-13 (Werkschaft product-concept round, recorded in
`261112-research-ws-ade-desktop-shell-and-rename`): the product is
local-only. Remote access comes later through a separate remote daemon or a
tunnel. Ticket creation approved by the owner as (523)=ok.

LAN serving reaches these surfaces today (inventory 2026-11-13, not
exhaustive): the `dashboard.lan_enabled` key in the three
`dashboard-config-manifest.json` copies (ws, wsflow, pi), the `--lan` flag,
`cmd/ws-mcp/dashboard.go`, `cmd/ws-mcp/proxy.go`,
`internal/mcp/dashboard_config.go`, `internal/mcp/process_env.go`,
`internal/wsdashboard/access.go` (LAN token, cookie at `:109`),
`internal/wsdashboard/daemon.go`, and `ai-docs/manuals/ws-mcp.md`.

## Decisions

1. **T1 removal** ((490), technical, owner ok 2026-11-13): for one minor,
   `dashboard.lan_enabled` and `--lan` are accepted but ignored, and each
   use logs a warning that LAN serving was removed; the next minor deletes
   them. The daemon binds loopback only from this ticket on. Rejected:
   deleting the key and flag at once (T2; a config or launcher still passing
   them would fail).
2. **Browser-threat boundaries stay** ((491), announced, owner ok
   2026-11-13): hostGuard, sameOrigin, Sec-Fetch checks, CSP, sandboxed
   iframes, and capability checks are unchanged. Only boundaries that exist
   solely for LAN viewers are removed (the LAN token, its cookie, and the
   LAN branch of the access checks).
3. **Other-local-OS-user protections stay** ((492)=a, owner 2026-11-13): the
   endpoint token file at 0600, and the bearer on `/shutdown` and the proxy.
4. **Compatibility tier: T1** per Decision 1.
5. **Settings relocation is interim** (MAIN technical call, 2026-11-15 at 02:34:12 UTC): owner decision (879) in `261115-feat-ws-dashboard-settings-in-usage-strip` does not supersede this removal. If this ticket lands first, Settings shows the version only and must never revive LAN. If Settings lands first, it moves the existing LAN box as-is with authentication/URL semantics unchanged, and this ticket removes that relocated box alongside the LAN token/cookie/access branch later. Neither ticket is a hard prerequisite of the other; the version and unrelated settings remain.

## Constraints

- `localViewer` remains the gate for routes that require a same-machine
  viewer; with LAN gone it always holds for a loopback request, but the
  checks stay so a later remote daemon starts from the same guards.
- Daemon code under `agents-plugin-tool/internal/` follows
  `ai-docs/manuals/release-compatibility.md`; the manifest key change follows
  its config-manifest rules in all three package copies.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Phases

### Phase 1: Loopback only, with LAN settings accepted and ignored

- Bind loopback only; remove the LAN token, its cookie, and the LAN access
  branch (Decision 2).
- `dashboard.lan_enabled` and `--lan` are accepted, ignored, and warned
  about (Decision 1); the manifest entries say so.
- Update `ai-docs/manuals/ws-mcp.md`.

Verification: cover removal both before and after the interim Settings
relocation: no LAN block remains or is revived, while Settings and its version
remain available. Go tests that the daemon binds loopback only with
`lan_enabled` true and with `--lan`, and that each logs the removal warning;
the browser-threat guard tests (hostGuard, sameOrigin, Sec-Fetch, CSP) and
the endpoint-token / bearer tests still pass unchanged; manifest drift tests
pass.

### Phase 2: Delete the key and flag (next minor)

Delete `dashboard.lan_enabled` and `--lan` one minor after Phase 1 shipped.

Verification: the key and flag are gone from code, manifests, and docs; the
release-compatibility scenario sheet records the removal.
