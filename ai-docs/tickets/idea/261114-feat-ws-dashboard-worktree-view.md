---
title: "Worktree page reusing session cards and the file explorer without agent panels"
related:
  261114-feat-ws-dashboard-reusable-file-explorer-card: "completed prerequisite: reusable explorer delivered, all four phases have Results (.done); source capture 3d2e3463d"
  261114-refactor-ws-dashboard-whiteboard-asset-split: "completed landing-order prerequisite (.done): assets now assembled from dashboard-*.js/css; see pages.go#L34-L86 and the split ticket's exact-099 move map"
  261115-feat-ws-dashboard-local-browse-anchor: "completed prerequisite (.done): LocalBrowseAnchor host0 serves explicit-root files and path-only terminals (local_browse.go#L11-L18; terminal_routes.go#L368-L415)"
---

# Worktree page reusing session cards and the file explorer without agent panels

## Background

Clicking a develop-branch/worktree name in the sidebar should open a worktree-focused page with live file viewing and terminal creation. Owner (784)=b settles this as a **new page**, not a mode added to a session's conversation/agent panels.

The owner's clarification, translated from MAIN's verbatim relay: create a new page, reuse exactly the current session screen's mechanisms, including cards and pinning, and remove only the left conversation panel and agent panel, leaving the file explorer. File viewing and terminal spawning remain live.

This is an idea capture only. It does not authorize implementation, a new session, a route/API design or a separate explorer implementation.

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
- **Respect the split pause.** The historical new-UI start pause applied while `261114-refactor-ws-dashboard-whiteboard-asset-split` was active; that landing-order prerequisite is now complete (`ai-docs/tickets/.done/261114-refactor-ws-dashboard-whiteboard-asset-split.md`, Integration Result and Resolution; `agents-plugin-tool/internal/wsdashboard/pages.go#L34-L86` assembles the split assets). Capturing the idea still does not bypass MAIN's separate scheduling or start signal.

## Constraints

- Preserve the explorer's existing read-only file-viewing scope and guarded-root/host-action boundaries. A selected worktree and a new page do not authorize broader filesystem access, filesystem mutation, new symlink permissions or a new terminal authority model.
- Preserve the reusable explorer component's state and attachment boundaries rather than copying its internals. This idea does not override its approved feature scope or select new storage fields, libraries or component event names.
- A worktree page is not authority to create an agent session, enable a root flag, control another session or add a new conversation/agent panel under a different name.
- Apart from the host anchor above, no URL shape, endpoint, wire schema, persistence model, additional view policy or new public component API is selected by this capture. Remote (LAN) viewers get no worktree-page files or terminals, since the anchor is local-viewer only. Those remain outside this settled page concept until separately designed.
- Before future tooling implementation, read the matching `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md` rules. This documentation-only idea changes no cross-version surface.
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
| scope.new_public_symbol | yes | A new browser-visible page route is required by Decisions; its URL and any exported Go symbol remain unselected. |
| scope.new_type_contract | unknown | The ticket expressly leaves URL/wire/persistence contracts unselected. Current card storage uses a session identity and the terminal client captures data-rid; no worktree-page identity contract has been chosen. |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/shell_test.go, cards_test.go, explorer_cards_test.go, files_test.go, local_browse_test.go and terminal_routes_test.go cover reuse and guards; worktree-page-specific assertions remain to be designed. |
| complexity.reuse_points | confirmed | Read WsCards.mount/persist (assets/cards.js#L659-L677, #L1356-L1396), WsFiles.create and per-card adapters (assets/files.js#L94-L127, #L1631-L1662), session integration (assets/dashboard-board.js#L166-L282), browseRoot (fs_routes.go#L205-L239) and terminalFolder/listTerminals (terminal_routes.go#L321-L400). |
| complexity.side_effect_risk | high | INFERRED: explorer setup is currently gated by live/tabbar and fpane, card persistence by rid, and the terminal client derives unfiltered list/create routes from data-rid; adapting a session-less consumer can mix worktree state or terminal lists (assets/dashboard-board.js#L166-L226; assets/terminal.js#L24-L56, #L439-L482). |
| risk.correctness | high | INFERRED: files, restored cards/pins and exact terminal cwd must remain distinct for two worktrees sharing host0; passing the transport anchor as their only storage identity would alias existing keys (assets/files.js#L1634-L1644; terminal_routes.go#L321-L359). |
| risk.fit | moderate | INFERRED: existing core and anchor fit the approved reuse, but shell framing currently accepts only / and /s/<reply-id>, and the no-conversation consumer has no attachment/send policy (assets/shell.js#L86-L104; assets/files.js#L279-L288). |
| risk.test | high | INFERRED: current shell and explorer-instance fixtures cover sessions/picker, not a worktree page; Phase 1 still defers runtime acceptance and page-specific tests, including two-root reload/pinning and anchor-filtered terminal reconciliation. |
| risk.security_or_contract | high | INFERRED: session-less file content and shell creation require explicit validated roots/folders and local-viewer trust; host0 has no session page and must not be used as one or borrow a session (pages.go#L1166-L1172; fs_routes.go#L205-L212; terminal_routes.go#L220-L257, #L368-L415). |

## Open Decision Queue

Last ID: (983)

Mockup: `ai-docs/tickets/assets/261114-feat-ws-dashboard-worktree-view/worktree-page.html` (render: `worktree-page.png`).
Every surface below is T0: a new browser route, new browser-local storage keys, client changes; no MCP tool, store shape, protocol or tracker event changes.

(972) [open] [announced] The page keeps the session template's files column (WsFiles) and card column (#bcol, WsCards with pinning) and omits the conversation tab, composer, agent tree and whiteboard. - (784)=b, (785)=ok.
(973) [open] [announced] Files and terminals go through the host0 anchor: explorer root = the worktree path; New terminal posts `folder:"path", path:<worktree>`; the card area lists `GET /s/host0/terminals?folder=<worktree>` (exact match, no nested or session terminals). - (894)=a; anchor Decisions (02b75478c).
(974) [open] [announced] A plain click on a rail worktree heading (`.wthd`) opens its page; the existing drag-to-reorder keeps its threshold, so a drag never navigates; the heading also opens on Enter. - (784)=b names the sidebar worktree name as the entry point.
(975) [open] [announced] Browser state is namespaced per worktree, never under host0: cards `ws.dashboard.cards.wt:<path>`, explorer cards `ws.dashboard.explorer.wt:<path>.<card-id>`, files column keys with the same `wt:<path>` identity. - explorer instance isolation (epic cross-child decision; 52673cf6 per-identity records).
(976) [open] [technical] Route `/w/<absolute worktree path>` (forward slashes; Windows `/w/C:/...`), shell-framed like `/s/<reply-id>` (add it to shell.js framed()); the server answers only a path the rail's worktree source lists for a stored or live workspace, else 404 over a `?root=` query - the shell drops a page's query when it frames it (shShellInit) - and over an opaque ID, which would need a new store; a path is stable across daemon restarts.
(977) [open] [technical] A remote (LAN) viewer gets a 403 local-only text for `/w/...` and a non-navigating heading over a page shell with an unavailable state - the anchor is local-viewer only, so nothing on the page would work remotely; fewer moving parts.
(978) [open] [technical] The explorer's "send to session"/attach action is hidden on the page over shown-disabled - there is no composer to receive it.
(979) [open] [technical] Toolbar: New terminal, New explorer (card), Search; Web page card and whiteboard are omitted over adapting them - their routes are session-bound (`/s/<rid>/web`, board regions). The search palette runs as on the index page (`rid` empty: no session file hooks).
(980) [open] [technical] First open shows the files column at the worktree root and an empty card area; nothing auto-spawns a terminal over opening one terminal by default - starting a shell is a side effect the user triggers.
(981) [open] [technical] Header: workspace, branch (or detached), path, a "worktree" chip and a "local only" chip; no session label, harness or state chips over reusing the session header as is - those describe an agent session the page does not have.
(982) [open] [technical] Anchor terminals created here keep running after the page closes and reappear on the next visit (folder filter) over closing them on page exit - this is the anchor's existing terminal lifetime; no new lifecycle.
(983) [open] [policy] Owner check: does the mockup (`worktree-page.png`) show the intended page - sidebar entry, header, files column without conversation/agent panels, card area with worktree-bound terminals?
- context: owner policy is mockup first for new UI; the mockup becomes the ticket's visual reference for side-by-side verification.
- alternatives: accept as is; accept with changes (name them); reject the layout.
> Recommend accept: it is the session screen minus the two panels the owner named, with no new UI concepts.

## Phases

### Phase 1: Deliver the approved worktree-focused page after its prerequisites

When the idea is separately prepared for implementation and MAIN schedules its start, reuse the current session page's card/pinning mechanisms and the approved reusable explorer component. Open the page from a sidebar develop-branch/worktree name, omit conversation and agent panels, keep live file viewing and terminal creation, and bind terminal `cwd` to that selected worktree.

Future verification must distinguish two worktrees: selecting one opens its own files and starts terminals in its exact path, without accidentally using the source session's worktree. Check reused card/pinning behavior, absence of conversation/agent panels, live file viewing, and unchanged explorer/file/terminal authority boundaries. Implementation design, tests, screenshots, runtime acceptance and any cross-version surface choices belong to later work, not this idea capture.
