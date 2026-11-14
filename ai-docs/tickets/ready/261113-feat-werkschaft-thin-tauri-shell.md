---
title: "Werkschaft thin Tauri shell around the local dashboard daemon"
related:
  261112-research-ws-ade-desktop-shell-and-rename: "Werkschaft concept; (493)=b builds this shell now"
  261110-epic-ws-dashboard-agent-host: "its shell-after-dashboard order (18)(19) is reversed by (493)=b"
  261109-research-windows-code-signing-signpath: "signing for the shipped exe/zip"
  261113-refactor-ws-dashboard-drop-lan-serving: "local-only daemon"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: e69b5db20e428804
sage-review-design-reviewed: e69b5db20e428804
---

# Werkschaft thin Tauri shell around the local dashboard daemon

## Background

Owner direction, 2026-11-13 ((493)=b, recorded in
`261112-research-ws-ade-desktop-shell-and-rename`, whose Proposals and Open
Questions hold the shell research): build a thin Tauri shell now, in
parallel with the browser dashboard. The owner's reasons: a thin shell
without self-update allows rendering verification in the native webviews
and is an easy way to hand someone one packaged exe (zip at first, an
installer as soon as possible); daemon updates far outnumber shell updates,
which suits dogfooding; the "updater" is just a daemon exe placed beside the
shell. Ticket creation approved by the owner as (523)=ok, with the EXEC
lead's daemon-ownership recommendation.

MAIN relays on 2026-11-14 at 15:11:55 and 15:14:52 reaffirm starting and
dogfooding the shell now, superseding the after-release scheduling assumption
(701). Owner (797)=a permits tonight's tooling install and implementation;
the first cargo build still requires MAIN's later heavy slot. The immediate
product increment is a runnable macOS `.app` or dev binary opening the live
dashboard, not completion of Windows, signing, or updater work.

## Decisions

1. **Thin shell scope** (MAIN lead's scope, owner-approved via (493)=b and
   (523)=ok): the shell attaches to a running daemon, or launches the daemon
   exe placed beside it in a later slice; Phase 1a only attaches. It loads
   exactly `http://127.0.0.1:<port>/` directly (a
   `tauri://` origin is refused by the same-origin write routes); it runs as
   a single instance; external links open in the system browser; closing
   the window leaves the daemon running; the shell does not update itself.
   Rejected: a bundled frontend or custom protocol (breaks same-origin
   writes); Electron (research Proposals: the sidecar dominates size, and
   Electron stays the fallback only if the webview matrix proves painful).
2. **Daemon ownership** ((523)=ok, EXEC recommendation): until self-update
   lands, the plugin launcher's SHA256-pinned daemon stays the authority.
   The shell attaches to whatever daemon is running, starts its beside-exe
   daemon only when none is, and never replaces or stops a running daemon.
   Revisit when the app self-updates, because an app-driven daemon update
   can raise the minor and lock out plugin sessions on the previous one.
   Rejected for now: the app owning the daemon and the launcher deferring
   to it (two install paths competing before a cross-version policy
   exists).
3. **Attach contract** (MAIN's 2026-11-14 direction): native code reads
   `<daemon-cache-root>/dashboard/endpoint.json`, respecting the daemon's
   cache-root override, takes its port, and checks existing `GET /health`
   before opening the exact loopback URL. Phase 1a never invokes
   `ws-mcp daemon-start`: that command runs `Ensure` and can replace a daemon,
   so the earlier unconditional attach/start proposal is superseded. A
   missing or unhealthy daemon shows a retry/error view asking the user to
   start it normally. Never hand the endpoint token to page JavaScript.
4. **Packaging**: local macOS `.app` or dev binary first; zip with the daemon
   exe follows in a later delivery slice. Signing remains outside this
   ticket, tracked by `261109-research-windows-code-signing-signpath`;
   Windows SmartScreen and macOS Gatekeeper/notarization boundaries remain.
   An unsigned local dogfood artifact is not signed distribution acceptance.
5. **Compatibility tier: T0**: an additive artifact; no daemon API, tool,
   protocol, or store change.
6. **Placement and tooling**: use `agents-plugin-tool/desktop/` with a nested
   Cargo module, not a root module. Prefer a pinned prebuilt local Tauri CLI
   under `desktop/` (an npm devDependency is acceptable) over global
   `cargo install`. The sizing report records Rust 1.95 under `~/.cargo/bin`;
   it is not on the harness PATH. The implementation worker may fetch normal
   dependencies when authorized; coordinate the first cargo build with MAIN's
   heavy slot rather than treating tooling availability as build evidence.
7. **Existing browser authority only in Phase 1a**: no new UI credential or
   daemon API change. The later grant in Decision 8 is outside Phase 1a;
   no new credential is needed merely to display today's dashboard. Neither
   the later grant nor the former Rust-held model authorizes new
   shell-only authority in Phase 1a. The unchanged browser dashboard surface
   is sufficient for the independent first window.
8. **Later shell-only HTTP authority** (owner (798)=a, relayed by MAIN at
   15:17:49 on 2026-11-14): when an actual new authority need exists, use a
   per-launch, short-lived JS-memory scoped grant, superseding (702)=a's
   Rust-held privileged-command model for that need. Revoke on shell exit,
   with crash lease/expiry fallback. This is a confirmed later gated slice,
   not a Phase 1a requirement or permission to change daemon routes now.

## Constraints

- Phase 1a uses existing endpoint discovery and `/health` only for attach;
  it adds no daemon route or auth/bootstrap model and never starts, stops,
  swaps, or takes lifetime ownership of the daemon. Closing leaves it running.
- Preserve hostGuard, sameOrigin, Sec-Fetch checks, CSP, sandboxed frames,
  capability/file-root checks, the 0600 endpoint file, and the existing
  lifecycle/MCP bearer boundaries ((491), (492)=a). Loading the existing page
  does not grant it native filesystem/process or general IPC authority.
- For the later grant only: exclude `/mcp`, shutdown/lifecycle/master
  credentials, and generic native command/filesystem authority. Release only
  to the exact discovered `http://127.0.0.1:<port>/` trusted window through
  authenticated bootstrap and trusted rediscovery; never to preview/opaque
  frames. Use one narrowly scoped Tauri bootstrap command with an explicit
  remote-origin/window ACL, not a port-wildcard or general native bridge.
  Keep the grant out of URLs, persistent browser storage, logs, and general
  postMessage traffic. Retain Host/Origin/Fetch Metadata guards, no general
  CORS, sanitizer, and capability roots. Current framing CSP is not evidence
  of strong anti-XSS CSP; any stronger policy needs a content-isolation plan.
- Native notifications, tray, login items and deep links are out of scope
  for the thin shell (research Topic 1 lists them for later).
- The settled shell location is `agents-plugin-tool/desktop/`; no root Cargo
  files/module directories or reintroduced `claude-plugin/` tree.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 261113-refactor-ws-dashboard-drop-lan-serving (2026-11-13, commit 14aabe4b): "(523)=ok: created 261113-refactor-ws-dashboard-drop-lan-serving (T1 per (490)-(492)) and 261113-feat-werkschaft-thin-tauri-shell (scope from MAIN, daemon ownership per EXEC rec)." — bearing: supports
- 261112-research-ws-ade-desktop-shell-and-rename (2026-11-13, commit 827e763a): "MAIN relayed the owner's product-concept answers (490)-(496) and left extend-vs-new to EXEC" — bearing: supports
- 261110-research-ws-dashboard-agent-host-foundations (2026-11-10, Confirmed Decisions): "Superseded by (493)=b and MAIN's 2026-11-14 start-now direction" — bearing: supports
- 261110-epic-ws-dashboard-agent-host (2026-11-10, Cross-Child Decisions): "pursue the full agent-host scope above as one epic, terminal and desktop shell included." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phase 1 creates a nested Cargo/Tauri package and pinned local CLI setup under agents-plugin-tool/desktop/. VERIFIED: git ls-tree -r --name-only HEAD agents-plugin-tool/desktop returns no files; creating the native package is implementation work, not a prerequisite. |
| scope.surface | public-interface | INFERRED from Phase 1: a new user-facing macOS window over the existing HTTP dashboard; no daemon API change. agents-plugin-tool/internal/wsdashboard/daemon.go#L670-L712 serves the existing page and /health. |
| scope.new_public_symbol | no | Phase 1 specifies no exported daemon/tool symbol or new IPC command; the new executable artifact is the public surface. Later grant/bootstrap remains gated by Decision 8. |
| scope.new_type_contract | no | Phase 1 consumes the existing endpoint and health shapes without changing them: agents-plugin-tool/internal/wsdashboard/endpoint.go#L48-L82, agents-plugin-tool/internal/wsdashboard/daemon.go#L748-L757. No new cross-module signature is specified. |
| scope.test_surface | unknown | Phase 1 specifies observed native first-window acceptance but no automated test files. Existing daemon tests include agents-plugin-tool/internal/wsdashboard/access_test.go and agents-plugin-tool/internal/wsdashboard/daemon_test.go; no desktop package/test exists at HEAD. |
| complexity.reuse_points | confirmed | VERIFIED via read-only git show HEAD: existing endpoint discovery/0600 write (agents-plugin-tool/internal/wsdashboard/endpoint.go#L48-L121), WS_CACHE_HOME override and default ~/.cache/ws@kang-sw-devenv (agents-plugin-tool/internal/wsstate/paths.go#L20-L23 and #L90-L101), token-free local browser access (agents-plugin-tool/internal/wsdashboard/access.go#L18-L49 and #L163-L183), and health validation of service plus endpoint PID (agents-plugin-tool/internal/wsdashboard/client.go#L21-L49). |
| complexity.side_effect_risk | moderate | INFERRED: navigation/open-browser handling adds native behavior, but Phase 1 forbids all daemon lifecycle actions. daemon-start really can replace a daemon: agents-plugin-tool/cmd/ws-mcp/proxy.go#L347-L405, agents-plugin-tool/internal/wsdashboard/ensure.go#L95-L160. |
| risk.correctness | moderate | INFERRED: discovery, unhealthy-endpoint retry and trusted navigation require new native code. Existing sameOrigin rejects custom schemes (agents-plugin-tool/internal/wsdashboard/inbox_routes.go#L149-L161); no native window has been observed in this documentation assignment. |
| risk.fit | moderate | INFERRED: first native package must stay nested and reuse the remote page, not become a second frontend. Placement/CLI are settled in Decision 6; single-instance, Windows, signing, updater and grant remain later gates, not Phase 1 prerequisites. |
| risk.test | high | INFERRED: no native implementation or WKWebView acceptance evidence exists here; Phase 1 requires a real runnable artifact and live first window. Sizing report /Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/shared/docs/tauri-thin-shell-sizing.md records toolchain observations, not build success; first heavy build requires MAIN's slot. |
| risk.security_or_contract | high | INFERRED: remote content in a native window must gain no new credential/IPC authority. Preserve Host/Origin/Fetch guards (agents-plugin-tool/internal/wsdashboard/daemon.go#L715-L725, agents-plugin-tool/internal/wsdashboard/terminal_routes.go#L217-L225), lifecycle/MCP bearer (daemon.go#L760-L775, agents-plugin-tool/internal/wsdashboard/proxy.go#L145-L168), framing-only CSP (agents-plugin-tool/internal/wsdashboard/pages.go#L1407-L1419), opaque frames and sanitizer (agents-plugin-tool/internal/wswhiteboard/render.go#L630-L653, agents-plugin-tool/internal/wswhiteboard/sanitize.go#L149-L152), capability roots (agents-plugin-tool/internal/wsdashboard/fs_capability.go#L37-L49). Owner (798)=a supersedes (702)=a only for the later gated grant, not Phase 1. |

## Phases

### Phase 1: Attach-only macOS first window (Phase 1a cut)

Create the nested Rust/Tauri shell and local pinned CLI setup per Decision 6.
Discover the existing daemon's port with cache override support, check
`/health`, and open exactly `http://127.0.0.1:<port>/` in WKWebView. Route
external links to the system browser while retaining intended dashboard
navigation. Show retry/error UI when no healthy daemon is available. Close
without touching daemon lifetime. Deliver the runnable `.app` or dev binary
path so owner dogfood can begin immediately.

Verification for the implementation worker: record the runnable artifact path
and an observed live dashboard first window; exercise default and overridden
cache discovery, missing/unhealthy endpoint and successful retry, external
browser navigation, and close leaving the same daemon running. Confirm no
endpoint token reaches page JS and no new credential, daemon API, or daemon
start/stop/swap is introduced. Record the first-window evidence without
claiming the broader webview matrix passed. No build, install, fetch, or
implementation verification is performed by this documentation assignment.

#### Source preparation checkpoint - 2026-11-14

The attach-only package is prepared under `agents-plugin-tool/desktop/`, with
pinned npm-local CLI and Cargo dependencies, a native Connection → Retry menu,
credential-free endpoint/health discovery, exact-origin navigation policy,
script-free local error view, zero native command permissions, and unit-test
contracts. The temporary geometric W icon is a dogfood placeholder.

Dependency preparation succeeded: npm installed the pinned local CLI; the
single authorized repair fetch locked 429 packages and exited 0 in 12.1 seconds
(`/tmp/werkschaft-cargo-fetch-repair-z_9t7i73.log`). The earlier fetch failed
before downloading because the manifest preceded its Rust target; root
explicitly authorized completing source before the one repair fetch. No build,
check, tests, app launch, live-daemon GET, heavy child, or daemon lifecycle
operation has run. This is not a Phase 1 Result or runnable-artifact evidence.

`agents-plugin-tool/desktop/README.md` and `scripts/build-slot.py` give the exact
one-attempt test/debug-app recipe: 1,500 cumulative seconds including at most
15 seconds of owned-process-group termination, two compile jobs, frozen locks,
local output paths and fresh logs. Expected artifact after successful build:
`desktop/target/debug/bundle/macos/Werkschaft.app`; direct dogfood binary:
`desktop/target/debug/werkschaft-desktop`. The separate bounded native capture
recipe rediscovers the endpoint at execution time and exercises missing,
unhealthy, retry, external-browser navigation and daemon survival. Independent
lite review and Phase 1 acceptance remain pending MAIN's actual slot. Phases
2–4 remain outside this invocation.

#### Executed evidence checkpoint - 2026-11-14

Source under observation: `0f16157b2adaf79475ebc665cdeec141b0ee3611`.
MAIN's actual 16:11:06 START allocated the existing 1,500-second build runner
and bounded eight-minute native observation together. The literal command was
run once from `agents-plugin-tool/desktop/`:
`python3 scripts/build-slot.py --allocated`. No fix or rerun followed.

Build evidence directory:
`/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/werkschaft-build-slot-1tu9dugz/`.
The complete `unit-tests.log` and `debug-app.log` were read. Nine Rust unit tests
passed; the frozen, two-job debug build bundled `Werkschaft.app` (24.49 MiB).
The runner exited 0 after 74.96 seconds. Both owned groups (5931 and 14566) were
confirmed absent and retired, with no lingering groups. Signing-skip warnings
were expected under the explicit `--no-sign` scope. The newly authored mocked
Python runner regression was **NOT RUN**: MAIN's literal command took precedence
and did not include it. No separate check or additional test command was run.

The built local artifact and direct binary are retained, not installed into any
owner plugin cache:

- `/Users/kang-sw/.ws-worktrees/devenv/spruce-december-suave/agents-plugin-tool/desktop/target/debug/bundle/macos/Werkschaft.app`
- `/Users/kang-sw/.ws-worktrees/devenv/spruce-december-suave/agents-plugin-tool/desktop/target/debug/werkschaft-desktop`

Native observation evidence directory:
`/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/werkschaft-native-observation-doyy5y7h/`.
The retained `observe.py`, `summary.json`, baseline/final health records, all four
case logs, and `post-slot-process-scan.json` describe the actual procedure.
It ran for 20.18 seconds. Default-cache shell PID 39213 (started 16:15:46.307868Z)
and overridden-cache shell PID 41145 both remained alive during their bounded
observations and logged `Werkschaft attached to http://127.0.0.1:29417`.
The isolated missing/unhealthy endpoint shells (39683 and 40405) remained alive
without an attach log; their native logs were empty. This is process/attach-log
evidence, not an observed error view or rendered first window. Isolated cache
files contained only port/PID; the shared endpoint was not modified.

Fresh default-cache endpoint and direct, proxy-free `/health` reads before and
after the native cases matched service `ws-dashboard`, port 29417, PID 98532,
and content hash
`893e6dc87abdabf3d3f9359db9c5af315f17df50da19bba6f801045d7292ad23`.
Those are observed identities, not source pins. Owned shells were terminated
through their retained subprocess handles; no daemon lifecycle command,
profiling command, or explicit daemon write request was issued. All owned shells/supervisor exited and the
isolated caches were removed. At 16:16:42.418958Z the final read-only process
scan found no owned build/shell/supervisor groups or transitive descendants;
`lsof` found no owned listeners (exit 1, no matches). Root subsequently released
the heavy slot; no further execution is authorized by this checkpoint.

**UNVERIFIED acceptance:** rendered first-window pixels and captures, ordinary
native UI navigation, native Connection → Retry interaction, external-browser
handoff, and normal window-close behavior. Screen Recording/Accessibility
permissions were not known already granted, so no screenshot, native UI
automation, permission probe/prompt/approval workaround, or browser substitution
was attempted. The unhealthy fixture was changed to a valid port/PID only in
its owned cache, but no native menu interaction was attempted; the separate
override launch is not a successful native retry. Native-process termination
with unchanged daemon health is not evidence of normal UI-close behavior.
No endpoint token appeared in the retained native logs, but live page-JavaScript
inspection was not performed; the no-token/native-authority claims additionally
rest on source and the executed unit-test contracts, not a visual inspection.

This checkpoint deliberately adds **no Phase 1 Result** and does not declare
Phase 1 accepted. The outstanding native acceptance boundary remains explicit;
the ticket stays in `ready/`, with Phases 2–4 untouched. Independent static lite
review is being recorded separately, with no further build, test, app, browser,
health request, permission probe, or child execution beyond that static reviewer.

#### Independent static lite review checkpoint - 2026-11-14

One fresh reviewer inspected the fixed source range
`2e1d9ea88730105cdb80a182bbd9a004d25fe064..0f16157b2adaf79475ebc665cdeec141b0ee3611`
and retained execution evidence, using read-only source/doc/git/file access.
No additional test, check, build, app, browser, daemon GET, permission probe,
native automation, or nested reviewer was run. Detailed artifact:
`/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@e9caa154/review-paths/bc9f649f-01-261113-feat-werkschaft-thin-tauri-shell-lite-static.md`.

Verdict: **non-clean, one Important finding; no Critical or Minor findings**.
The runner still carries a group that is present after a foreground step into
the next long build. If the helper subsequently exits and its numeric PGID is
reused, final presence probing cannot establish continued ownership. The
reviewer recommends resolving/cleaning such a group before advancing, or
stopping that attempt, with a present-after-step/disappear/reuse regression.
The actual successful attempt observed both groups absent immediately, so
this unexercised path does not invalidate its recorded cleanup evidence.
The finding remains **unfixed** because root authorized only evidence
recording/static review after slot release; no source remediation or rerun
was performed in that scope.

The reviewer separately assessed exact-origin/new-window/subframe navigation,
zero native command permissions, cache overrides, endpoint-token exclusion,
bounded health identity checks and the unchanged daemon boundary. No actionable
finding was raised in those areas. The public WK callback lacks frame identity
and is not a user-gesture-only detector; retained native behavior remains
unverified. The review explicitly distinguishes the pre-navigation attach log
from asynchronously completed navigation and rendered pixels.

The mocked Python regression remains **NOT RUN**. All previously listed native
acceptance gaps remain **UNVERIFIED**. Static review is not Phase 1 acceptance;
no Result, later phase, closure or merge is authorized by this checkpoint.

### Phase 2: Daily-use reliability (later Phase 1b cut)

Add single-instance launch/focus next; it is not a Phase 1a blocker. Verify
rediscovery/reconnect after daemon restart and broader WKWebView acceptance:
dashboard writes, terminals/live transports, clipboard, isolated content
frames, internal/external links, and notification fallback. Compare LOOK
captures with Chromium and list differences. Add safe missing-daemon start
only after establishing a race-safe non-replacement path under Decision 2;
verify it cannot replace or stop an existing daemon and does not bind daemon
lifetime to shell exit. The later grant decision (798)=a is independent of
this reliability slice.

### Phase 3: Windows and packaged delivery (later Phase 1c cut)

After the first-window slice, add Windows/WebView2 acceptance and zip delivery
with the daemon exe. Verify attach, safe missing-daemon start, second-launch
focus, external links, writes, close survival, and WebView2 LOOK comparison
with Chromium; report platform differences explicitly. Signing/notarization,
installer, updater, rename, and built-in agent remain separate later work,
not prerequisites for Phase 1a. Preserve Decision 4's signing boundary.

### Phase 4: Scoped grant only when new shell-only HTTP authority is needed

Independent of later platform delivery; gated on an actual new authority
need after the first window. Do not add a ceremonial
bootstrap key for today's browser surface. Apply Decision 8 and the grant
constraints before introducing any later daemon registration/bootstrap
contract. Keep the endpoint credential native and separate from the grant.

Verification when this phase is activated: demonstrate daemon recognition of
only the scoped authority, authenticated bootstrap and rediscovery after
restart, bounded expiry and exit/crash revocation, exact trusted-window ACL,
rejection of external and preview/opaque-frame access, and absence from URLs,
persistent storage, logs, and general postMessage. Verify `/mcp`, lifecycle,
master credentials, and generic native commands/filesystem remain excluded;
existing browser guards, sanitizer and capability roots still hold. Document
reload/re-bootstrap and any fetch/stream transport implications explicitly;
no assumption that browser WebSocket or EventSource accepts bearer headers.
These are future acceptance requirements, not implementation evidence.
