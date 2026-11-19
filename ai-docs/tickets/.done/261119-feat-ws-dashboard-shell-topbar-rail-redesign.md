---
title: 대시보드 쉘 상단 핸들 바 및 전체 높이 세션 레일 재디자인
related:
  261113-feat-ws-dashboard-iframe-shell: overlay iframe shell and postMessage header mirroring this refines
  261113-feat-ws-dashboard-rail-usage-gutter: plan usage gutter and popover relocated from .rfoot to top center handle
  261115-feat-ws-dashboard-settings-in-usage-strip: .rfoot #gear and #gmenu outside data-region="rail" preserved
  261115-feat-werkschaft-electron-shell: 30px Windows Electron titlebar above dashboard; no dashboard window chrome
  261115-feat-ws-session-hire-approval: ready Phase 2 top-bar badge (.hirebadge) uses the transient center-cluster global-alert slot per Decision 1
  261119-feat-ws-dashboard-paged-docking-gallery: shares 1600px / 2200px data-pincol rail and pinned-column breakpoints
sage-review-completeness-reviewed: 3079dcf8e99e6e3e
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3079dcf8e99e6e3e
completed: 2026-11-19
---

# Dashboard Shell Topbar Handles and Full-Height Session Rail Redesign

## Background

Two related layout problems accumulated in the dashboard shell:

1. **Broken bottom-left plan usage strip:** After switching harness marks from `16×16` `.hi` icons to `24×15` `.hb` badges (`dddec48bf`), `usage.css` kept `.usage-mark { width: 16px; height: 16px; }`, causing the `Cl`/`Co` badges and `.usage-clock` stale icon to collide with the `5h`/`주` columns inside the cramped `.rfoot` footer. (`dashboard-rail.css` `.shp-sub` also retained a `23px` left indent sized for `16px + 7px`.)
2. **Header/rail visual mismatch and wasted header width:** The shell `.top` bar spans full width *above* `#rail`, placing session-specific metadata directly over the global session list in wide view (`>=1600px`), while existing top-right iframe controls (`[+]`, tracker, git branch, search, drawer) already use a clean `34px` top-edge hanging handle visual language (`dashboard-drawer.css`, `tracker.css`).

The owner approved an interactive mockup (`ai-docs/assets/mockups/261119/shell-redesign/shell-redesign-v4.html`, `v4-1` compact 2-row with the center live/ask handle removed) that spans `#rail` full height in wide view, scopes `34px` purpose-separated hanging handles to the right-hand session stage, relocates plan usage into the top center handle (leaving `.rfoot` clean with `[Settings][Home]`), and leaves existing top-right handles untouched.

## Decisions

1. **Approved visual layout and handle grammar (`v4-1` compact 2-row, owner confirmed 2026-11-19):**
   - Canonical visual reference: `ai-docs/assets/mockups/261119/shell-redesign/shell-redesign-v4.html`.
   - **Wide view (`>=1600px`, docked rail):** Left `#rail` spans full viewport height from top to bottom (with `12px` top, bottom, and left outer margins, matching the overlay `#rail` insets in `dashboard-rail.css#L26` and mockup `v4`). All top handles live strictly inside the right-hand session stage above the conversation/board area, so per-session chrome never mixes above the global session list.
   - **Narrow view (`<1600px`, and `data-pincol` overlay range `1600–2199.98px`):** `#rail` overlays from the left (`z-index: 45`) over the stage top handles (`z-index: 10`), toggled by a `34×34` `[≡]` hanging handle (`#railbtn`) at stage top-left.
   - **Uniform `34px` top-edge hanging handles (`border-radius: 0 0 8px 8px`, `background: var(--wb-card)`, `box-shadow: var(--wb-shadow-hi)`):** Match the existing top-right drawer/tracker handle height and visual grammar across left and center clusters:
     - **Left cluster (session-specific):**
       1. Narrow/overlay rail toggle handle (`#railbtn`, `34×34`, hidden while `#rail` is docked).
       2. Session identity handle (`height: 34px`, compact 2-row stack): row 1 = project/workspace name (and subtle timestamp/folder context when available), row 2 = bold session label / reply-id (click-to-copy preserved) + inline rename `✎` button (`.sname`/`.snm`/`.sren`/`.sinp` contracts preserved).
       3. Harness/session handle (`height: 34px`): `.hb` harness badge + existing `.host.sse` chip (promoted to `.host.hbtn` with chevron by `leadChip()` in `dashboard-leads.js#L529-L544` when `leadCan()` is true, opening the existing `rpSessMake` session/model/effort popover via the `menu` postMessage — not a new harness-switching dropdown; mockup `v4`'s harness switcher is illustrative only) + `hosted` badge when hosted. Existing session-head status chips (failure/MCP/ended/nolive/counts) sit in a compact single-line group immediately after the harness/hosted handle inside the left cluster at the same `34px` strip height.
     - **Center cluster (plan usage + transient global-alert slot, owner confirmed `(16)=a`, 2026-11-19):**
       1. Plan usage handle (`height: 34px`, `.usage-toggle`) centered in the stage topbar.
       2. **Transient global-alert slot:** Reserve a sibling slot beside `.usage-toggle` in the center cluster for urgent cross-session alerts (specifically `261115-feat-ws-session-hire-approval` Phase 2 `.hirebadge` from `(950)=a`). The slot is omitted/hidden when empty so the steady-state topbar matches approved mockup `v4-1`.
       3. **No center `live N · ? N` status handle** (owner explicitly rejected duplicating it in the topbar; live and open-question counts remain in `#rail .rhd`).
       4. `.usage-mark` uses `.hb.sm` (`19×12px`, `dashboard.css#L96`) inside `.usage-mark { width: 19px; height: 16px; margin-right: 3px; }` with `.usage-clock` at `right: -4px; bottom: -3px` (matching mockup `v4` `#L77-L78,#L103`) so `Cl`+clock and `Co` never collide with `5h`/`주` columns; `.usage-pop` headings keep full-size `.hb`.
       5. Clicking `.usage-toggle` opens `.usage-pop` anchored below the top center handle via `WsRail.popPlace`.
     - **Right cluster (strictly untouched):** Existing iframe-owned top-right handles (`[+]`, tracker pill, git branch `#ghandle`, search `#shandle`, drawer `#drhandle`) and their CSS/DOM contracts in `dashboard-drawer.css` and `tracker.css` must not be modified.
   - **Narrow-width overflow precedence (`760px`–`~1100px`, owner confirmed `(17)=b` with priority refinement, 2026-11-19):** The single non-wrapping `34px` strip must stay usable down to `760px` (Electron minimum window width, `desktop/src/main.rs#L162`) without colliding with the untouched right-cluster `--drhw`/`--tkhw` clearance. The **center cluster has the lowest layout priority** when horizontal space narrows:
     1. First, flex/collapse the center `.usage-toggle` — stepping down to at most a single compact `34px` "view usage" button (compact icon/trigger that still opens `.usage-pop` on click) so the center cluster never squeezes or collides with the left or right clusters.
     2. Next, shrink/ellipsis the left cluster's session identity text (`min-width: 0; overflow: hidden; text-overflow: ellipsis`) and hide secondary `hosted` text / low-priority status chips.
     3. Never clip `#railbtn`, `.host.hbtn`, the single compact usage trigger, or the untouched right-cluster handles down to `760px`.
   - **Rail footer `.rfoot`:** Remove `.usage-toggle` from `.rfoot`; retain `[⚙ Settings][⌂ Home]` (`#gear` remains the first child of `.rfoot`; `#gmenu` stays outside `data-region="rail"`).
   - **Rejected:** Full-width bar spanning above `#rail` in wide view (mixed session chrome above the session list); multi-height variable top handles (owner required uniform `34px` matching top-right handles); keeping plan usage in `.rfoot` (too cramped on narrow rail width); duplicating `live N · ? N` in the top center bar; routing `.hirebadge` into `#rail .rhd` (hidden when narrow rail is closed and would amend `ready/` `(950)=a`) or into the untouched right cluster; dropping `.usage-toggle` entirely or moving it back into `.rfoot` at narrow widths; truncating session identity before collapsing the center usage cluster.

2. **Preserve overlay iframe shell architecture (technical call, 2026-11-19):**
   - Keep `#frames` full-viewport (`position: fixed; inset: 0; z-index: 0`) and `.dwrap` as the pointer-through overlay (`z-index: var(--z-cards)`). Keep `.top` as a direct child of `.dwrap` (using CSS grid placement on `.dwrap` / `display: contents` on `.dbody` in docked-rail mode) and keep `--dpad` (`16px`) untouched while adjusting only the docked left rail outer margin to `12px`, so `.dwrap > .top.dr-room` / `.dwrap > .top.tk-room` (`dashboard-drawer.css#L456`), `dashboard-shell.css#L37-L42`, and `testdata/session-switch-header.browser.cjs` keep matching with zero edits to `dashboard-drawer.css` and zero shift of the top-right handles.
   - Preserve `WsShell` `postMessage` contracts (`head`, `layout`, `scroll`, `title`, `nav`, `press`, `key`, `theme`, `menu`), `1600px` / `2200px` (`data-pincol`) breakpoints, and zero `.top`/main-top height drift across cached session switches.
   - On shell-owned non-session routes (`/` index `.top.sh-own`, `/w/<path>` worktree page), the shell topbar still keeps the narrow `[≡]` toggle and the shell-owned center plan usage handle visible while hiding session-only `r-head` handles when `head` is null.
   - **Rejected:** Rewriting `#frames` into a side-by-side layout container right of `#rail` (would break framed-page viewport breakpoints `1600px`/`2200px`, `data-pincol` syncing, and `261119-feat-ws-dashboard-paged-docking-gallery` assumptions); reparenting `.top` out of `.dwrap` (would break `.dwrap > .top` selectors in untouched `dashboard-drawer.css`).

3. **Hover-card subtitle alignment cleanup (technical call, 2026-11-19):**
   - Update `.shp-sub` `padding-left` in `dashboard-rail.css` from `23px` (old `16px` `.hi` icon + `7px` gap) to `31px` (`24px` `.hb` badge + `7px` gap) so the session hover card subtitle aligns cleanly under `.shp-nm`.

4. **Compatibility tier: T0 (transparent):**
   - Purely internal dashboard HTML/CSS/JS asset changes inside `agents-plugin-tool/internal/wsdashboard/assets/` (plus updated tests). Touches no MCP tool schema, `/mcp` hello, shared store shape, tracker event type, or config/rsrc manifest field.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/performance.md (declared for agents-plugin-tool/)
- **Top-right handle boundary:** Do not modify existing iframe-owned top-right handles (`[+]`, tracker, `#ghandle`, `#shandle`, `#drhandle`) or their `dashboard-drawer.css` / `tracker.css` appearance and placement.
- **Electron desktop shell compatibility:** Preserve `meta[name="theme-color"]`, do not draw custom window titlebar/drag chrome (Windows Electron renders a separate 30px titlebar above the dashboard), and preserve usability down to the 760px minimum window width.
- **Popover and region invariants:** `#gmenu` stays outside `data-region="rail"`; `#gear` stays the first child of `.rfoot`; `.usage-toggle` survives rail region SSE replacements without losing popover state or focus; session rename (`labelStart`), copy-to-clipboard (`snm`), and harness/model `menu` lifting (`data-fpopover`/`data-lifted`) keep working.
- **Pinned column / docking gallery coexistence:** Keep `1600px` and `2200px` (`data-pincol`) rail/pinned-column rules working cleanly; in `1600–2199.98px` with `data-pincol`, the rail overlays cleanly over `.top` and `#railbtn` appears in stage top-left.
- **File-touch / Windows test rule:** Keep live UI state in memory/DOM with no new file I/O. On Windows hosts, run Go tests on remote `w5090` only (never local `go test`).
- **Dark and light theme support:** Use existing `--wb-*` CSS tokens (`--wb-bg`, `--wb-card`, `--wb-muted`, `--wb-muted-fg`, `--wb-fg`, `--wb-line`, `--wb-primary`, `--wb-shadow`, `--wb-shadow-hi`, `--wb-ok`, `--wb-fail`, `--wb-warn-ink`) so both dark and light themes render cleanly.

## Prior Decisions

- dddec48bf (2026-11-19, commit): "feat(dashboard): two-letter harness badges with shared definition" — bearing: supports
- ae7c46b32 (2026-11-15, commit): "fix(wsdashboard): keep shell header and rail visible under a popover-lifted frame" — bearing: constrains
- 261115-feat-ws-dashboard-settings-in-usage-strip (2026-11-15, Result): "Moved Settings to the leftmost rail-footer gear; retained unrelated drawer cards, all preferences and the usage popover." — bearing: constrains
- 28bcbf4b4 (2026-11-16, commit): "fix(wsdashboard): restore stacked usage indicator columns" — bearing: constrains
- 261114-feat-ws-dashboard-usage-footer-meter (2026-11-15, commit 401d2fbf, as quoted in 261115-feat-ws-dashboard-settings-in-usage-strip): "Meter typography, spacing, signs and stale treatment match section A." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | assets/dashboard.tmpl, dashboard.css, dashboard-shell.css, dashboard-rail.css, dashboard-rail.js, dashboard-rail-popovers.js, usage.css, usage.js, dashboard-lifecycle.js under agents-plugin-tool/internal/wsdashboard/ |
| scope.surface | internal | dashboard HTML/CSS/JS assets only; window.WsUsage.mount (usage.js#L95) is page-internal, no MCP or shared-store surface |
| scope.new_public_symbol | no | none named; relocation reuses .usage-toggle, #railbtn, WsUsage.mount |
| scope.new_type_contract | no | none; WsUsage.mount (usage.js#L95) is page-internal JS and postMessage contracts are preserved |
| scope.test_surface | existing | i18n_test.go, rail_navigation_test.go (asserts Home sits between Settings and usage-toggle), usage.test.js, testdata/settings-root.browser.cjs, testdata/rail-navigation.browser.cjs, testdata/session-switch-header.browser.cjs, testdata/rail-worktree-admission.browser.cjs, testdata/usage-footer.browser.cjs |
| complexity.reuse_points | confirmed | WsRail.popPlace (rail.js#L663), .hb.sm (dashboard.css#L96), 34px hanging-handle grammar in dashboard-drawer.css#L145 and tracker.css#L219 were read |
| complexity.side_effect_risk | high | docked rail grid, overlay z-order, data-pincol 1600-2199.98px and 2200px rules, framed-page railph geometry and cached session-switch header height all change together |
| risk.correctness | high | zero .top height drift across session switches and the shared rail/pinned-column breakpoint rules in dashboard-rail.css#L16-L99 must stay correct at once |
| risk.fit | moderate | must coexist with the unlanded 261119-feat-ws-dashboard-paged-docking-gallery, which reworks the same 1600px/2200px rail and pinned-column rules |
| risk.test | high | geometry and z-order claims are verifiable only through Chromium browser tests and screenshots, and local go test is not allowed on this Windows host |
| risk.security_or_contract | low | T0 per Decisions; no MCP schema, store shape, or endpoint change; postMessage kinds are preserved and the gear and gmenu invariants are kept |

## Phases

### Phase 1: Implement stage-scoped 34px topbar handles, full-height docked rail, and top-center plan usage

Implement the approved `v4-1` shell layout and usage-strip relocation across `agents-plugin-tool/internal/wsdashboard/assets/` (and any template/view-model fields needed in `internal/wsdashboard/`), keeping the overlay iframe shell intact:

1. **Full-height docked rail & stage-scoped `.top` layout (`dashboard.tmpl`, `dashboard.css`, `dashboard-shell.css`, `dashboard-rail.css`, `dashboard-lifecycle.js`):**
   - Keep `.top` as a direct child of `.dwrap` and adjust `.dwrap` / `.dbody` grid rules so that when `#rail` is docked (`>=1600px`, except `1600–2199.98px` under `data-pincol`), `#rail` spans full viewport height on the left (`340px` column, `12px` top/bottom/left margins without modifying `--dpad` `16px` right padding) while `.top` occupies the top edge (`top: 0`) of the right-hand stage column only.
   - Mirror the exact placeholder geometry in framed pages (`data-frame` with `railph` + hidden `.top`) so framed page content sits cleanly below the `34px` top handles with zero vertical drift across cached session switches (`testdata/session-switch-header.browser.cjs`) and proper right-padding clearance (`--drhw`, `--tkhw` via untouched `dashboard-drawer.css#L456`) for untouched top-right iframe handles.
   - When `#rail` is in overlay mode (`<1600px`, or `1600–2199.98px` with `data-pincol`), show `#railbtn` as a `34×34` hanging handle at stage top-left and ensure the overlay `#rail` (`z-index: 45`) and `.rscrim` (`z-index: 44`) slide cleanly above the stage top handles.
   - Ensure shell-owned topbar items (narrow `#railbtn` and center `.usage-toggle`) stay functional on `/` (`.top.sh-own`, updating the `:not(.back)` rule in `dashboard-shell.css#L41` and `index` template `dashboard.tmpl#L384`) and `/w/<path>` as well as `/s/<rid>`.

2. **Purpose-separated `34px` hanging handles (`dashboard.tmpl`, `dashboard.css`, `dashboard-shell.css`):**
   - Style `.top` children as uniform `34px` top-edge hanging handles (`border-radius: 0 0 8px 8px`, `background: var(--wb-card)`, `box-shadow: var(--wb-shadow-hi)`, `gap: 6px`), matching the existing top-right handle aesthetic.
   - Left cluster:
     - Session identity handle in compact 2-row stack (`v4-1`: row 1 = project/workspace name + optional compact metadata, row 2 = `.sname` label/reply-id copy button + `.sren` inline rename pencil & `.sinp` input).
     - Harness/session handle (`.hb` + `.host.sse` promoted to `.host.hbtn` by `leadChip()` in `dashboard-leads.js#L529-L544` to open the existing `rpSessMake` session popover via `menu` postMessage + `hosted` pill when hosted; preserve `data-fpopover`/`data-lifted` alignment; add an internal view-model field only if needed to display active model text, T0).
     - Existing session-head status chips (failure/MCP/ended/nolive/counts) in a compact single-line group immediately after the harness/hosted handle at the same `34px` strip height.
   - Center cluster:
     - Shell-owned `.usage-toggle` hanging handle (no `live N · ? N` handle) plus an empty-hidden transient global-alert slot beside it for future urgent cross-session badges such as `261115-feat-ws-session-hire-approval` `.hirebadge` (absent in steady state so steady state matches mockup `v4-1`).
   - Right cluster:
     - Untouched iframe-owned handles (`[+]`, tracker, `#ghandle`, `#shandle`, `#drhandle`).
   - Narrow-width overflow precedence down to `760px` (center cluster has lowest layout priority):
     1. Flex and collapse center `.usage-toggle` first — down to at most a single compact `34px` "view usage" button (opening `.usage-pop` on click, preserving accessible label/tooltip) so it never squeezes or collides with left or right clusters.
     2. Next, shrink/ellipsis left identity text (`min-width: 0; overflow: hidden; text-overflow: ellipsis`) and hide secondary `hosted` text / low-priority status chips.
     3. Never clip `#railbtn`, `.host.hbtn`, the single compact usage trigger, or the untouched right-cluster handles down to `760px`.

3. **Relocate plan usage to top center handle, fix badge overlap, and clean `.rfoot` (`dashboard.tmpl`, `usage.css`, `usage.js`, `dashboard-rail.css`, `dashboard-rail.js`, `dashboard-rail-popovers.js`, `dashboard-lifecycle.js`):**
   - Remove `.usage-toggle` from `r-rail`'s `.rfoot`; style `.rfoot` as clean `[⚙ Settings][⌂ Home]` controls (`#gear` remains first child of `.rfoot`; `#gmenu` stays outside `data-region="rail"`).
   - Mount `.usage-toggle` inside the shell/standalone topbar center slot; rescope `.rail .rfoot` and `.rail .usage-toggle` in `usage.css#L2-L4`; update `WsUsage.mount` (`usage.js#L95`), `usageGutter` mount and `.usage-toggle` click wiring in `dashboard-rail.js#L42,#L521`, and `rp.kind === "usage"` re-anchor lookup in `dashboard-rail-popovers.js#L390` so `.usage-toggle` binds in the header (not restricted to `#rail`) and opens `.usage-pop` anchored beneath the top handle via `WsRail.popPlace`.
   - Update `usage.js` header gutter mark to emit `.hb.sm` (`19×12px`) inside `.usage-mark { width: 19px; height: 16px; margin-right: 3px; }` with `.usage-clock` at `right: -4px; bottom: -3px` (keeping full-size `.hb` in `.usage-pop` headings) so the `Cl`/`Co` badge and stale clock icon never overlap the `5h`/`주` columns.
   - Fix `.shp-sub` `padding-left` in `dashboard-rail.css` from `23px` to `31px`.

4. **Tests and visual verification:**
   - Update affected Go, JS unit, and Chromium browser tests (`i18n_test.go`, `rail_navigation_test.go` where `#L30` asserts Home sits between Settings and `.usage-toggle`, `usage.test.js`, `testdata/usage-footer.browser.cjs`, `testdata/settings-root.browser.cjs`, `testdata/rail-navigation.browser.cjs`, `testdata/session-switch-header.browser.cjs`, `testdata/rail-worktree-admission.browser.cjs`, and any shell header layout tests) to verify the new DOM layout, zero `.top` height drift across session switches, non-overlapping `.usage-mark` geometry, full-height wide rail, narrow overlay z-order over `.top`, narrow-width overflow behavior down to `760px`, and clean `.rfoot` at `360px`.
   - Run Go tests on remote `w5090` (`go test ./internal/wsdashboard/...`) and JS/browser tests where available.
   - Capture dark-theme screenshots (wide `>=1600px`, narrow `<1600px` closed and open, `760px` minimum width, usage popover open) and compare side by side against `ai-docs/assets/mockups/261119/shell-redesign/shell-redesign-v4.html` (LOOK verification), plus a light-theme sanity check.
   - **Done when:** all updated Go, Node, and Chromium browser tests pass, `dashboard-drawer.css` and `tracker.css` top-right handle appearance/placement remain unchanged, zero `.top` height drift, non-overlapping `.usage-mark`, center-first narrow-width degradation down to `760px` (compact single usage trigger still opening `.usage-pop`, no clipping of `#railbtn`/`.host.hbtn`/right handles), and the side-by-side LOOK comparison against `shell-redesign-v4.html` plus light-theme sanity check are clean.

### Result (5e5f37a) - 2026-11-19

Landed in b4170f9, d70dacc, 5e5f37a. The header is now one non-wrapping 34px strip of hanging handles (`.tph`: rail button `[≡]`, two-row identity, harness `.hb` + `.host.sse`/`.hbtn` + hosted, status chips). The center (`tcen` template, shell-owned in the session/shell/index headers) holds the `.usage-toggle` and an empty, hidden `#talert` slot. When docked, the rail runs full height 12px from the edges, done by lifting the `.dbody` grid item rather than porting it to `display: contents`. The strip keeps to the stage column, and `data-pincol` 1600–2199.98px resets both offsets. `.rfoot` is now Settings + Home only, drawn as labeled split buttons with the label taken from the localized `aria-label`. Usage marks use `.hb.sm` in a 19×16 mark. The popover centers under the handle. `.shp-sub` is 31px. `WsRail.topFit`/`topWatch` handle the center-first step-down (compact usage, then chips hidden plus identity ellipsis). `dashboard-drawer.css` and `tracker.css` are untouched.

Verification (w5090, Chromium 1248, Playwright 1.64.1): the new `TestShellTopbarBrowser` checks:
- wide: rail at (12,12)→vh−12 and the strip at x=372, h=34
- narrow: the `[≡]` handle at (16,0) is 34×34, and the open overlay covers the strip
- pincol at 1800px overlays the rail
- 760px: precedence invariants, no collision with the right handles, essentials unclipped
- usage marks clear of their columns; the popover sits below and centered under its handle
- the footer at 360px fits

Also: `TestSessionSwitchHeaderBrowser` shows zero drift (height 34, railTop 12, mainTop 46). The static, pages, i18n, rail-navigation and pincol tests pass. On the full package, the 17 failing browser tests are identical to HEAD's baseline failures, with none new. LOOK: compared shots of dark wide, wide+usage, narrow closed and open, 760px, and light wide against the `shell-redesign-v4.html` captures: match. Known deviations:
- The identity row 1 carries no timestamp (the view model has none).
- Hosted shows as an icon pill.
- The existing detailed usage popover is kept.
- The tracker `#tkqs` palette button shows as an extra handle.

Decisions: the docked rail is 344px wide (margin-left −4px on the stretched grid item keeps the mockup's 16px stage gap). `shScroll` no longer lifts the rail. `usage-footer.browser.cjs` is now a source-only LOOK harness for the header handle.
