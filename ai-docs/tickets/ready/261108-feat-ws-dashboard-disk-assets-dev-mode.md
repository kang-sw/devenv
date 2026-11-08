---
title: Dashboard disk-assets dev mode - serve dashboard and whiteboard assets from a working tree
related:
  261108-feat-ws-tracker-drawer-ui: unblocks its UI tuning loop; its Decision 4 now points here
  261106-epic-ws-dashboard-serve-daemon: the daemon this extends
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3dc739715b20b2bd
sage-review-completeness-reviewed: 3dc739715b20b2bd
---

# Dashboard disk-assets dev mode - serve dashboard and whiteboard assets from a working tree

## Background

Every dashboard or whiteboard UI tweak today needs `./install.sh update` and a
daemon restart, because both asset sets are compiled in through `embed.FS`:
`wsdashboard/pages.go` (`dashboardFS`: dashboard.tmpl, dashboard.css,
dashboard.js, sprite.svg) and `wswhiteboard/render.go` (`assetFS`: page.tmpl,
page.css, the page scripts, sprite.svg; mermaid.min.js in `markdown.go`). The
whiteboard assets are also loaded once per process (`loadAssets` behind a
`sync.Once`), and the daemon reaches them through `wswhiteboard.PageAssets()`.

The user is in a stretch of small visual fixes (option chip wrap, settled-chip
tone, the tracker mockup item window) and asked whether they could land
without a plugin restart. A disk-assets dev mode was already decided as
Decision 4 of `261108-feat-ws-tracker-drawer-ui` ((412) c part b: "재설치 없이
계속 목업 보고 갱신할 수 있게 값싸게 ... 미세조정이 많을 거 같아서요"), but
that ticket waits on the tracker caller core and covered dashboard assets only.
Board (429) a (2026-11-08) split it out as this standalone ticket, to run now,
covering the whiteboard assets as well.

## Decisions

1. **Standalone, now, both asset sets** ((429) a): the daemon can serve the
   dashboard assets and the whiteboard page assets from a working-tree
   directory instead of the embedded copies, so template, CSS, and JS changes
   show on a browser reload and only Go changes need a rebuild. Rejected:
   keeping it in drawer-ui Phase 1 with whiteboard assets added (waits on the
   caller core); keeping the original dashboard-only scope (does not cover the
   whiteboard fixes the user is making now).
2. **Dev mode over a separate dev server** ((412) c part b): rejected a
   separate `go run` dev server with a fake store - mostly redundant with this.

3. **Setting** ((432)): the worker picks a config key or an environment
   variable and records why. A config key lives in the `dashboard.` namespace
   of both dashboard manifests (ws and wsflow) with the same default (off) in
   both, so `TestShippedDashboardManifestsDifferOnlyInTheEnabledDefault` stays
   unchanged. Off by default; it never serves a file outside the given
   directory.
4. **One source-root value** ((433)): the setting names the
   `agents-plugin-tool` source root, and the daemon resolves
   `internal/wsdashboard/assets` and `internal/wswhiteboard/assets` under it.
   Rejected: two separate directory settings - two values to keep in step for
   a layout that is the tool's own source tree.
5. **Read per request, mermaid embedded** ((434)): in dev mode the dashboard
   and whiteboard templates, CSS, JS, and sprites are read from disk and the
   templates re-parsed on each request, bypassing the `sync.Once` caches; the
   embedded path keeps its caches unchanged. The vendored mermaid bundle stays
   embedded even in dev mode: `MermaidAsset()` names the served file by
   hashing the embedded bytes, the MCP server and `access.go` share that name,
   and the vendored file rarely changes. Rejected: a file-watcher cache - a
   dev-only path needs no invalidation logic.
6. **Daemon-served pages only** ((435)): the MCP server's whiteboard page file
   and embed snapshot keep the embedded assets. Rejected: plumbing the setting
   into the MCP server process too - the user views through the dashboard URL,
   and one process to configure keeps the change small.
7. **Manual reload** ((436)): a browser reload picks up a change. Rejected:
   auto-reload on file change - more moving parts; it can follow as its own
   ticket.
8. **Process-level switch** ((437), auto-confirmed from fact population): the
   daemon turns dev mode on at startup through a process-level setter in
   `wswhiteboard` and the matching switch in `wsdashboard`. Rejected:
   threading a parameter through every render call - the daemon and the MCP
   server are separate processes, so a process-level switch cannot reach the
   MCP server's renders (Decision 6), and render call sites stay untouched.
9. **drawer-ui follows** ((431)): `261108-feat-ws-tracker-drawer-ui`
   Decision 4 now points here, its Phase 1 no longer carries the dev mode, and
   its dev-mode constraint lines moved here.

## Constraints

- The embedded path (dev mode off) must behave byte-for-byte as today.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)

## Prior Decisions

- 261108-feat-ws-tracker-drawer-ui (2026-11-08, Decisions): "Disk-assets dev mode ((412) c, part b): the daemon can serve dashboard assets from a working-tree directory instead of the embedded copy, so JS/CSS changes show on reload and only Go changes need a rebuild." — bearing: supports
- 261106-feat-ws-dashboard-daemon-core (2026-11-06, Decisions): "Shared page assets. The daemon serves the shared static assets the whiteboard page loads by relative path, including the mermaid asset, so a served page renders the same as the static one." — bearing: constrains
- 261106-feat-whiteboard-rich-items (2026-11-06, Result): "Mermaid: the 11.17.2 dist/mermaid.min.js IIFE is vendored in assets/mermaid/ ... MermaidAsset() names it wb-mermaid-<sha12>.js" (name hashes the embedded bytes, so a disk-read mermaid changes the served name) — bearing: constrains
- 261107-feat-ws-dashboard-i18n (2026-11-07, Result): "Standalone whiteboard outputs: the page written to disk, the embed, and the fingerprint stay English (View.Lang empty); only the served page follows its viewer." — bearing: constrains
- 261107-feat-ws-dashboard-pull-down-drawer (2026-11-07, Result): "No Go, SSE, or wswhiteboard change: the served board page is the session template in board mode, so both pages share the drawer." — bearing: supports
- 2bae93f7 (2026-11-06, commit): "No wsflow mirror: these are Go-embedded assets under agents-plugin-tool, not covered by wsflow-mirroring." — bearing: constrains
- 261107-bug-dashboard-dev-daemon-shadowed-by-release (2026-11-07, Observation): "the running dashboard daemon was still the previous day's binary ... Getting the new code required POST /shutdown plus starting ws-mcp dashboard by hand." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/pages.go, agents-plugin-tool/internal/wswhiteboard/render.go and markdown.go, plus the dev-mode setting site and possibly both dashboard-config-manifest.json files |
| scope.surface | cross-module | wsdashboard consumes wswhiteboard.PageAssets and MermaidAsset (pages.go#L113-L118); the seam spans both packages |
| scope.new_public_symbol | unknown | a setting or exported seam is needed (wswhiteboard must receive it from wsdashboard) but the config key versus environment variable is open (decision 432) |
| scope.new_type_contract | unknown | seam signature not designed; ticket leaves it to the worker |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/pages_test.go, daemon_test.go, agents-plugin-tool/internal/wswhiteboard/render_test.go, agents-plugin-tool/internal/mcp/dashboard_config_test.go; new dev-mode tests extend them |
| complexity.reuse_points | confirmed | loadDashboardAssets (pages.go#L103) and loadAssets (render.go#L67) are the existing read seams; both read via embed.FS ReadFile and ParseFS |
| complexity.side_effect_risk | moderate | MermaidAsset is also called by the MCP server and access.go, and its name hashes the bytes; sync.Once caches must be bypassed only in dev mode |
| risk.correctness | moderate | per-language template re-parse (wsi18n.Languages loop) and the shared mermaid name and bytes must stay consistent between dev and embedded paths |
| risk.fit | moderate | wswhiteboard has no daemon-supplied config today; the setting must cross the package boundary without changing the MCP server's embedded behavior |
| risk.test | moderate | live browser reload check is manual; escape-refusal and no-restart tests need a temp working-tree copy |
| risk.security_or_contract | moderate | serving files from a configured directory must refuse path escape; daemon routes sit behind the existing access control |

## Phases

### Phase 1: Disk-assets dev mode for dashboard and whiteboard assets

- Add the dev-mode setting and route every dashboard and whiteboard asset
  read (templates, CSS, JS, sprites; mermaid stays embedded) through one seam that reads the
  embedded copy by default and the working-tree file when dev mode is on.
- Implementation notes from design review (all autonomous):
  - Both loaders read a fixed list of file names, so there is no
    request-derived path; the real escape vector is a symlink inside an assets
    directory pointing outside the source root. Resolve with
    `filepath.EvalSymlinks` and check containment (`go.mod` is go 1.23, so no
    `os.Root`; `os.DirFS` follows symlinks).
  - The daemon reads no config itself: `ws-mcp serve` resolves dashboard
    settings and passes flags to the detached daemon (as `--lan`/`--port` do
    through `Ensure`), and a running same-version daemon is replaced only on a
    mode mismatch. A config key must either join that mode match (as LAN does)
    or the "how to turn it on" report must name the `POST /shutdown` step.
    A machine-local source-root path fits the global config layer, like
    `dashboard.lan_enabled`.
  - A dev-path read or parse failure (a half-edited template) is surfaced in
    the response or the log and never cached; the next request retries.
    Decide and record whether a missing source root at startup fails the
    daemon or only logs.
  - `wsdashboard` daemon tests use a fake page source that does not render
    through `wswhiteboard`, so test a `page.tmpl` change at the
    `wswhiteboard.RenderRegions` level with the switch on, and `page.css`
    through a daemon test (it is inlined via `PageAssets`). Process-level
    switches are reset between tests; no `t.Parallel` on them.
  - Record the setting choice (config key or environment variable) and why in
    the phase `### Result`.
- Verification:
  - tests: off by default (embedded bytes served); on, a changed CSS file and
    a changed template in a temp working-tree copy show on the next request
    without restart, for both the dashboard and the whiteboard page; a
    symlinked asset that resolves outside the source root is refused; a broken
    template on the dev path reports its error and recovers once fixed;
  - `cd agents-plugin-tool && go test -count=1 ./internal/wsdashboard/ ./internal/wswhiteboard/`
    and the manifest parity test if a config key was added;
  - a live check: daemon from the worktree build with dev mode on, edit
    `page.css` in the worktree, reload the session page, and see the change
    (screenshots before/after to `/tmp/disk-assets-dev-mode/`).
- Report how the user turns it on (exact command or config line).
