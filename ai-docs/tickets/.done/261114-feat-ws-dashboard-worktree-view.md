---
title: "Worktree page reusing session cards and the file explorer without agent panels"
related:
  261114-feat-ws-dashboard-reusable-file-explorer-card: "completed prerequisite: reusable explorer delivered, all four phases have Results (.done); source capture 3d2e3463d"
  261114-refactor-ws-dashboard-whiteboard-asset-split: "completed landing-order prerequisite (.done): assets now assembled from dashboard-*.js/css; see pages.go#L34-L86 and the split ticket's exact-099 move map"
  261115-feat-ws-dashboard-local-browse-anchor: "completed prerequisite (.done): LocalBrowseAnchor host0 serves explicit-root files and path-only terminals (local_browse.go#L11-L18; terminal_routes.go#L368-L415)"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: e1669011bf135dea
sage-review-completeness-reviewed: e1669011bf135dea
completed: 2026-11-15
---

# Worktree page reusing session cards and the file explorer without agent panels

## Background

Clicking a develop-branch/worktree name in the sidebar should open a worktree-focused page with live file viewing and terminal creation. Owner (784)=b settles this as a **new page**, not a mode added to a session's conversation/agent panels.

The owner's clarification, translated from MAIN's verbatim relay: create a new page, reuse exactly the current session screen's mechanisms, including cards and pinning, and remove only the left conversation panel and agent panel, leaving the file explorer. File viewing and terminal spawning remain live.

The page's open decisions were settled by the owner through MAIN on 2026-11-15 (board IDs in brackets: local (972)-(982) = board (979)-(989), local (983) = board (990)).

## Decisions

- **New worktree page — (784)=b.** The sidebar's develop-branch/worktree name is the entry point. The page is rooted in the selected worktree rather than presented as a conversation with an agent.
- **Reuse the session mechanism.** Keep the existing card and pinning mechanisms; omit the conversation and agent panels. Do not implement a second card system or a separate simplified pinning model.
- **Reuse the explorer — (785)=ok.** Consume the reusable explorer component owned by `261114-feat-ws-dashboard-reusable-file-explorer-card` (the approved capture was committed as `3d2e3463d`). The worktree page is another consumer, not a competing explorer or workspace-picker UI.
- **Worktree-bound terminals — (785)=ok.** Terminals created from this page spawn with `cwd` equal to the selected worktree path. File viewing and terminal operations remain live rather than being a static directory snapshot.
- **Files and terminals through the host anchor — (894)=a.** With no agent
  session, the page browses its worktree and opens terminals through the
  daemon-owned host anchor delivered by
  `261115-feat-ws-dashboard-local-browse-anchor`: local viewer only, an
  explicit root (explorer) and folder (terminal) equal to the selected
  worktree path, and the page lists only that worktree's anchor terminals.
  Rejected: borrowing a live session of the same repository (unusable for a
  stored workspace with no live session; terminals tied to the borrowed
  session) and a separate session-less route family.
- **Page layout - (972) [board (979)].** The page keeps the session template's files column (WsFiles) and card column (`#bcol`, WsCards with pinning) and omits the conversation tab, composer, agent tree and whiteboard.
- **Anchor wiring - (973) [board (980)].** Files and terminals go through the host0 anchor: explorer root = the worktree path; New terminal posts `folder:"path", path:<worktree>`; the card area lists `GET /s/host0/terminals?folder=<worktree>` (exact match, no nested or session terminals).
- **Entry point - (974) [board (981)].** A plain click on a rail worktree heading (`.wthd`) opens its page; the existing drag-to-reorder keeps its threshold, so a drag never navigates; the heading also opens on Enter.
- **Per-worktree browser state - (975) [board (982)].** Browser state is namespaced per worktree, never under host0: cards `ws.dashboard.cards.wt:<path>`, explorer cards `ws.dashboard.explorer.wt:<path>.<card-id>`, files column keys with the same `wt:<path>` identity. Source: explorer instance isolation (epic cross-child decision; 52673cf6 per-identity records).
- **Route - (976) [board (983)].** `/w/<absolute worktree path>` (forward slashes; Windows `/w/C:/...`), shell-framed like `/s/<reply-id>` (add it to shell.js `framed()`). The server answers only a path the rail's worktree source lists for a stored or live workspace, else 404. Rejected: a `?root=` query (the shell drops a page's query when it frames it, shShellInit) and an opaque ID (would need a new store); a path is stable across daemon restarts.
- **Remote viewers - (977) [board (984)].** A remote (LAN) viewer gets a 403 local-only text for `/w/...`, and the rail heading does not navigate for it. Rejected: a page shell with an unavailable state - the anchor is local-viewer only, so nothing on the page would work remotely.
- **No send-to-session - (978) [board (985)].** The explorer's "send to session"/attach action is hidden on the page. Rejected: shown disabled - there is no composer to receive it.
- **Toolbar - (979) [board (986)].** New terminal, New explorer (card), Search. The Web page card and the whiteboard are omitted; their routes are session-bound (`/s/<rid>/web`, board regions). The search palette runs as on the index page (`rid` empty: no session file hooks). Rejected: adapting the session-bound cards.
- **First open - (980) [board (987)].** The files column shows the worktree root and the card area is empty; nothing auto-spawns a terminal. Rejected: opening one terminal by default - starting a shell is a side effect the user triggers.
- **Header - (981) [board (988)].** Workspace, branch (or detached), path, a "worktree" chip and a "local only" chip; no session label, harness or state chips. Rejected: reusing the session header as is - those describe an agent session the page does not have.
- **Terminal lifetime - (982) [board (989)].** Anchor terminals created here keep running after the page closes and reappear on the next visit (folder filter). Rejected: closing them on page exit - this is the anchor's existing terminal lifetime; no new lifecycle.
- **Visual reference - (983) [board (990)]=a accepted.** `ai-docs/tickets/assets/261114-feat-ws-dashboard-worktree-view/worktree-page.html` (render `worktree-page.png`) is the approved reference for the sidebar entry, header, toolbar and files column. Owner clarification (via MAIN, 06:38Z/06:45Z): the card area uses exactly the session screen's existing card layout - no docking and no card spanning both columns; the mockup's horizontally spanning "Terminal 2" is not intended and is not a reference for card arrangement.
- **Compatibility tier: T0.** A new browser route, new browser-local storage keys and client changes only; no MCP tool, store shape, protocol, config manifest or tracker event changes, so an old binary is unaffected.

## Constraints

- Preserve the explorer's existing read-only file-viewing scope and guarded-root/host-action boundaries. A selected worktree and a new page do not authorize broader filesystem access, filesystem mutation, new symlink permissions or a new terminal authority model.
- Preserve the reusable explorer component's state and attachment boundaries rather than copying its internals; do not add libraries or component event names.
- A worktree page is not authority to create an agent session, enable a root flag, control another session or add a new conversation/agent panel under a different name.
- Beyond the route and storage keys settled above, no new endpoint, wire schema, server-side persistence or public component API is added. The page reuses the existing `/s/host0/fs/*` and `/s/host0/terminals` anchor APIs unchanged.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md` before editing `agents-plugin-tool/internal/`.
- Split assets: edit the `dashboard-*.js/css` fragments and `dashboard.tmpl` that own each concern (pages.go dashboardScriptFiles); all fragments share one IIFE scope, and a helper used in a fragment must exist in that bundle (tracker.js internals such as `esc` are not in scope - see f02e3b1b8).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 261114-feat-ws-dashboard-reusable-file-explorer-card (2026-11-15, commit 52673cf6): "Each explorer card uses one session/card-ID-keyed browser record; close destroys requests/listeners before deleting that captured adapter's record." — bearing: constrains
- 261115-feat-ws-dashboard-local-browse-anchor (2026-11-15, commit 02b75478): "Anchor GET terminals accepts an optional exact folder filter and never returns session orphans." — bearing: supports
- 261114-refactor-ws-dashboard-whiteboard-asset-split (2026-11-15, commit 0c46ba10): "Separator-free fragment joins preserve every upstream asset byte, one dashboard IIFE, evaluation order, CSS cascade" — bearing: constrains
- 261115-feat-ws-dashboard-settings-in-usage-strip (2026-11-15, Decisions): "This ticket implements only the interim empty-selection session screen; it neither delivers nor blocks on that future view." — bearing: supports
- 181c2abb (2026-11-15, commit): "host0 owns /fs/roots with a different answer shape (host home + volumes). Two shapes on one path name would be ambiguous, so the session route takes its own name." — bearing: constrains
- 261114-epic-ws-dashboard-file-explorer (2026-11-14, Cross-Child Decisions): "Explorer instances are isolated. No child may restore the current singleton as the state owner for all explorer cards." — bearing: constrains
- df68e311 (2026-11-14, commit): "Owner784=b explicitly chooses a new page reusing session cards and pinning while removing conversation and agent panels" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Expected integration sites: agents-plugin-tool/internal/wsdashboard/pages.go, assets/dashboard.tmpl, assets/dashboard-board.js, assets/dashboard-rail.js, assets/shell.js, assets/files.js and assets/terminal.js; exact edit set is not selected by this idea. |
| scope.surface | public-interface | New browser page and rail entry point; existing /s/host0/fs/* and /s/host0/terminals HTTP APIs reused (pages.go#L1164-L1259; local_browse.go#L11-L18). No MCP change is requested. |
| scope.new_public_symbol | yes | New browser route `/w/<abs path>` ((976)); exported Go symbols are the implementer's choice. |
| scope.new_type_contract | yes | Browser-local keys `ws.dashboard.cards.wt:<path>` / `ws.dashboard.explorer.wt:<path>.<card-id>` ((975)); current card storage uses a session identity and the terminal client captures data-rid, so a `wt:<path>` identity is threaded through. No wire schema change. |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/shell_test.go, cards_test.go, explorer_cards_test.go, files_test.go, local_browse_test.go and terminal_routes_test.go cover reuse and guards; worktree-page-specific assertions remain to be designed. |
| complexity.reuse_points | confirmed | Read WsCards.mount/persist (assets/cards.js#L659-L677, #L1356-L1396), WsFiles.create and per-card adapters (assets/files.js#L94-L127, #L1631-L1662), session integration (assets/dashboard-board.js#L166-L282), browseRoot (fs_routes.go#L205-L239) and terminalFolder/listTerminals (terminal_routes.go#L321-L400). |
| complexity.side_effect_risk | high | INFERRED: explorer setup is currently gated by live/tabbar and fpane, card persistence by rid, and the terminal client derives unfiltered list/create routes from data-rid; adapting a session-less consumer can mix worktree state or terminal lists (assets/dashboard-board.js#L166-L226; assets/terminal.js#L24-L56, #L439-L482). |
| risk.correctness | high | INFERRED: files, restored cards/pins and exact terminal cwd must remain distinct for two worktrees sharing host0; passing the transport anchor as their only storage identity would alias existing keys (assets/files.js#L1634-L1644; terminal_routes.go#L321-L359). |
| risk.fit | moderate | INFERRED: existing core and anchor fit the approved reuse; shell framing currently accepts only / and /s/<reply-id> ((976) adds /w/), and the send action is hidden on the page ((978)) (assets/shell.js#L86-L104; assets/files.js#L279-L288). |
| risk.test | high | INFERRED: current shell and explorer-instance fixtures cover sessions/picker, not a worktree page; Phase 1 must add page-specific tests, including two-root reload/pinning and anchor-filtered terminal reconciliation. |
| risk.security_or_contract | high | INFERRED: session-less file content and shell creation require explicit validated roots/folders and local-viewer trust; host0 has no session page and must not be used as one or borrow a session (pages.go#L1166-L1172; fs_routes.go#L205-L212; terminal_routes.go#L220-L257, #L368-L415). |

## Phases

### Phase 1: Worktree page

Implement the page per `## Decisions`.

- **Server.** Serve `/w/<abs path>` from the dashboard daemon: validate the path against the rail's worktree source (stored or live workspace worktrees), 404 otherwise, 403 local-only text for a remote viewer. Render the session template without the conversation tab, composer, agent tree and whiteboard, with the (981) header and the (979) toolbar. Mark the page so the client knows its worktree path and that it has no session `rid` (the index page's empty-`rid` behaviour for search).
- **Shell.** Add `/w/` to shell.js `framed()` so the page is framed like `/s/<reply-id>`.
- **Rail.** A plain click or Enter on `.wthd` opens `/w/<path>` (local viewers only); the drag-to-reorder threshold is unchanged, so a drag never navigates.
- **Client.** Files column rooted at the worktree through the host0 anchor; New explorer cards and terminals through the anchor (`folder:"path", path:<worktree>`); the card area lists `GET /s/host0/terminals?folder=<worktree>`. Thread a `wt:<path>` identity through card persistence, explorer card records and files-column keys instead of `rid` ((975)); hide the send-to-session action ((978)); no auto terminal ((980)).

Verification:

- Two worktrees: opening each shows its own files; a terminal started from each runs in its exact path (`pwd`), and each page lists only its own terminals; cards, pins and explorer cards restored after reload stay per worktree with no aliasing to host0 or to a session's keys.
- A session page is unchanged: its cards, pins, explorer and terminals still use its `rid` identity (existing session/explorer/terminal tests stay green).
- Route guards: an unlisted path is 404, a remote viewer gets 403, a drag on `.wthd` does not navigate, click and Enter do.
- No conversation tab, composer, agent tree, whiteboard, Web page card or send-to-session action on the page; terminals survive a page close and reappear on revisit.
- Side-by-side: capture the live page headless (disposable fixture daemon, never the shared daemon) and compare it against `ai-docs/tickets/assets/261114-feat-ws-dashboard-worktree-view/worktree-page.png` for the sidebar entry, header, toolbar and files column. The card area is compared against the session screen's own card layout, not the mockup (owner clarification in (983)).
- Scoped `go test -run` for the touched wsdashboard areas plus `./internal/wsi18n` (`GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`); never the full `./internal/wsdashboard` package.

### Result (60cb167ee) - 2026-11-15

- Added the local-only `/w/<path>` page, framed through the existing shell and selected by rail click or Enter without changing the reorder threshold. Cold bookmarks refresh the same rail worktree source/projection, including pooled-worktree exclusions and branch/detached metadata. Unix URLs supply the leading slash through `/w/`; Windows drive paths remain `/w/C:/...`.
- Reused the session template's viewport layout, WsFiles, WsPreview and WsCards. The page has only the files and card columns, the worktree header and New terminal/New explorer/Search toolbar; no session, conversation, composer, agent tree, whiteboard, Web action or send-to-session control is introduced.
- Kept the session/search `rid` empty, routed files/previews/terminal actions through host0, and namespaced browser records under `wt:<path>`. Terminal lists use the exact folder filter; creating a terminal uses the selected path and never happens automatically.
- Compatibility remains **T0**: `assets/dashboard.js:16-19` selects the new identity only for a worktree page; `assets/dashboard-board.js:185-189,245` selects the new keys/anchor while ordinary sessions retain their original rid and keys; `assets/files.js:1635` retains the existing per-card record format; `assets/terminal.js:24-25,441,477` and `assets/preview.js:28` preserve the session transport fallback. No shared store, protocol, MCP API or manifest changed; an old binary never reads or rewrites these new browser keys.
- Verification used only disposable fixture daemons and holders with offline Go settings. Focused dashboard regression batch covering worktree routes, shell, rail, files/explorer/cards, previews, terminals and anchor guards passed (38.075 s); `./internal/wsi18n` passed (1.700 s); race-enabled worktree/anchor trust checks passed (3.570 s). The new route test uses a bounded 10-second Git/render budget because the shared 500 ms health client timed out with race instrumentation.
- `TestWorktreePageBrowser` passed (16.56 s): two roots' live previews and terminal `pwd`, exact terminal lists, per-worktree cards/explorers/pins after reload, preview restoration, click/Enter/drag and non-local gesture suppression, index-scope search, terminal survival across page closure and unchanged session isolation. Windows route framing is covered by the Node shell contract; native Windows execution was not run.
- Compared `/tmp/exec5/wv-impl/worktree-page.png` vs `ai-docs/tickets/assets/261114-feat-ws-dashboard-worktree-view/worktree-page.png`: **match** for the selected sidebar entry, workspace/branch/path and chips, toolbar actions and explorer-only files column. The implementation uses the current reusable explorer and responsive session rail rather than copying illustrative mockup styling. Card placement follows the session's normal/pinned layout, with no docking or spanning; pinned capture: `/tmp/exec5/wv-impl/pinned-page.png`.
- Per the caller's no-subagent restriction, the worker performed the Lite correctness/test-integrity read itself; this was not independent review. No outstanding functional findings. Full dashboard runs and shared daemon rebuild/install/start/stop/swap were intentionally omitted. Self-review report: `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@97ab43fd/review-paths/bef381a0-01-261114-worktree-view-self-review.md` (ephemeral).
