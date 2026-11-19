---
title: 대시보드 쉘 상단 핸들 바 및 전체 높이 세션 레일 재디자인
related:
  261113-feat-ws-dashboard-iframe-shell: overlay iframe shell and postMessage header mirroring this refines
  261113-feat-ws-dashboard-rail-usage-gutter: plan usage gutter and popover relocated from .rfoot to top center handle
  261115-feat-ws-dashboard-settings-in-usage-strip: .rfoot #gear and #gmenu outside data-region="rail" preserved
  261115-feat-werkschaft-electron-shell: 30px Windows Electron titlebar above dashboard; no dashboard window chrome
  261119-feat-ws-dashboard-paged-docking-gallery: shares 1600px / 2200px data-pincol rail and pinned-column breakpoints
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
   - **Wide view (`>=1600px`, docked rail):** Left `#rail` spans full viewport height from top to bottom (aligned with its existing `12px` outer margins). All top handles live strictly inside the right-hand session stage above the conversation/board area, so per-session chrome never mixes above the global session list.
   - **Narrow view (`<1600px`, and `data-pincol` overlay range `1600–2199.98px`):** `#rail` overlays from the left (`z-index: 45`) over the stage top handles (`z-index: 10`), toggled by a `34×34` `[≡]` hanging handle (`#railbtn`) at stage top-left.
   - **Uniform `34px` top-edge hanging handles (`border-radius: 0 0 8px 8px`, `background: var(--wb-card)`, `box-shadow: var(--wb-shadow-hi)`):** Match the existing top-right drawer/tracker handle height and visual grammar across left and center clusters:
     - **Left cluster (session-specific):**
       1. Narrow/overlay rail toggle handle (`#railbtn`, `34×34`, hidden while `#rail` is docked).
       2. Session identity handle (`height: 34px`, compact 2-row stack): row 1 = project/workspace name (and subtle timestamp/folder context when available), row 2 = bold session label / reply-id (click-to-copy preserved) + inline rename `✎` button (`.sname`/`.snm`/`.sren`/`.sinp` contracts preserved).
       3. Harness/model handle (`height: 34px`): harness badge + harness/model selector (`menu` postMessage popover preserved) + `hosted` badge when hosted, plus existing session-head status chips (failure/MCP/ended/nolive/counts) styled cleanly inside or alongside the left handle cluster without breaking the `34px` strip height.
     - **Center cluster (plan usage only):**
       1. Plan usage handle (`height: 34px`, `.usage-toggle`) centered in the stage topbar.
       2. **No center `live N · ? N` status handle** (owner explicitly rejected duplicating it in the topbar; live and open-question counts remain in `#rail .rhd`).
       3. `.usage-mark` and `.usage-clock` sizing fixed (`width: 19px; height: 16px; margin-right: 3px` with `.hb.sm`, or matched to badge width with clear right gap) so `Cl`+clock and `Co` never collide with `5h`/`주` columns.
       4. Clicking `.usage-toggle` opens `.usage-pop` anchored below the top center handle via `WsRail.popPlace`.
     - **Right cluster (strictly untouched):** Existing iframe-owned top-right handles (`[+]`, tracker pill, git branch `#ghandle`, search `#shandle`, drawer `#drhandle`) and their CSS/DOM contracts in `dashboard-drawer.css` and `tracker.css` must not be modified.
   - **Rail footer `.rfoot`:** Remove `.usage-toggle` from `.rfoot`; retain `[⚙ Settings][⌂ Home]` (`#gear` remains the first child of `.rfoot`; `#gmenu` stays outside `data-region="rail"`).
   - **Rejected:** Full-width bar spanning above `#rail` in wide view (mixed session chrome above the session list); multi-height variable top handles (owner required uniform `34px` matching top-right handles); keeping plan usage in `.rfoot` (too cramped on narrow rail width); duplicating `live N · ? N` in the top center bar.

2. **Preserve overlay iframe shell architecture (technical call, 2026-11-19):**
   - Keep `#frames` full-viewport (`position: fixed; inset: 0; z-index: 0`) and `.dwrap` as the pointer-through overlay (`z-index: var(--z-cards)`), updating the grid layout of `.dwrap` / `.dbody` in both the shell template and framed pages (`data-frame` with `railph` and hidden `.top` placeholder) so the docked rail spans full height and `.top` sits in the stage column.
   - Preserve `WsShell` `postMessage` contracts (`head`, `layout`, `scroll`, `title`, `nav`, `press`, `key`, `theme`, `menu`), `1600px` / `2200px` (`data-pincol`) breakpoints, and zero `.top`/main-top height drift across cached session switches.
   - On shell-owned non-session routes (`/` index `.top.sh-own`, `/w/<path>` worktree page), the shell topbar still keeps the narrow `[≡]` toggle and the shell-owned center plan usage handle visible while hiding session-only `r-head` handles when `head` is null.
   - **Rejected:** Rewriting `#frames` into a side-by-side layout container right of `#rail` (would break framed-page viewport breakpoints `1600px`/`2200px`, `data-pincol` syncing, and `261119-feat-ws-dashboard-paged-docking-gallery` assumptions).

3. **Hover-card subtitle alignment cleanup (technical call, 2026-11-19):**
   - Update `.shp-sub` `padding-left` in `dashboard-rail.css` from `23px` (old `16px` `.hi` icon + `7px` gap) to `31px` (`24px` `.hb` badge + `7px` gap) so the session hover card subtitle aligns cleanly under `.shp-nm`.

4. **Compatibility tier: T0 (transparent):**
   - Purely internal dashboard HTML/CSS/JS asset changes inside `agents-plugin-tool/internal/wsdashboard/assets/` (plus updated tests). Touches no MCP tool schema, `/mcp` hello, shared store shape, tracker event type, or config/rsrc manifest field.

## Constraints

- **Implementation convention manuals to read before editing:**
  - `ai-docs/manuals/shipped-surface-boundary.md` (`agents-plugin-tool/`)
  - `ai-docs/manuals/release-compatibility.md` (`agents-plugin-tool/internal/`)
  - `ai-docs/manuals/performance.md` (`agents-plugin-tool/`)
- **Top-right handle boundary:** Do not modify existing iframe-owned top-right handles (`[+]`, tracker, `#ghandle`, `#shandle`, `#drhandle`) or their `dashboard-drawer.css` / `tracker.css` appearance and placement.
- **Electron desktop shell compatibility:** Preserve `meta[name="theme-color"]`, do not draw custom window titlebar/drag chrome (Windows Electron renders a separate 30px titlebar above the dashboard), and preserve usability down to the 760px minimum window width.
- **Popover and region invariants:** `#gmenu` stays outside `data-region="rail"`; `#gear` stays the first child of `.rfoot`; `.usage-toggle` survives rail region SSE replacements without losing popover state or focus; session rename (`labelStart`), copy-to-clipboard (`snm`), and harness/model `menu` lifting (`data-fpopover`/`data-lifted`) keep working.
- **Pinned column / docking gallery coexistence:** Keep `1600px` and `2200px` (`data-pincol`) rail/pinned-column rules working cleanly; in `1600–2199.98px` with `data-pincol`, the rail overlays cleanly over `.top` and `#railbtn` appears in stage top-left.
- **File-touch / Windows test rule:** Keep live UI state in memory/DOM with no new file I/O. On Windows hosts, run Go tests on remote `w5090` only (never local `go test`).
- **Dark and light theme support:** Use existing `--wb-*` CSS tokens (`--wb-bg`, `--wb-card`, `--wb-muted`, `--wb-muted-fg`, `--wb-fg`, `--wb-line`, `--wb-primary`, `--wb-shadow`, `--wb-shadow-hi`, `--wb-ok`, `--wb-fail`, `--wb-warn-ink`) so both dark and light themes render cleanly.

## Phases

### Phase 1: Implement stage-scoped 34px topbar handles, full-height docked rail, and top-center plan usage

Implement the approved `v4-1` shell layout and usage-strip relocation across `agents-plugin-tool/internal/wsdashboard/assets/` (and any template/view-model fields needed in `internal/wsdashboard/`), keeping the overlay iframe shell intact:

1. **Full-height docked rail & stage-scoped `.top` layout (`dashboard.tmpl`, `dashboard.css`, `dashboard-shell.css`, `dashboard-rail.css`, `dashboard-lifecycle.js`):**
   - Adjust `.dwrap` / `.dbody` grid rules so that when `#rail` is docked (`>=1600px`, except `1600–2199.98px` under `data-pincol`), `#rail` spans full viewport height on the left (`340px` column, `12px` top/bottom/left margins) while `.top` occupies the top edge (`top: 0`) of the right-hand stage column only.
   - Mirror the exact placeholder geometry in framed pages (`data-frame` with `railph` + hidden `.top`) so framed page content sits cleanly below the `34px` top handles with zero vertical drift across cached session switches (`session-switch-header.browser.cjs`) and proper right-padding clearance (`--drhw`, `--tkhw`) for untouched top-right iframe handles.
   - When `#rail` is in overlay mode (`<1600px`, or `1600–2199.98px` with `data-pincol`), show `#railbtn` as a `34×34` hanging handle at stage top-left and ensure the overlay `#rail` (`z-index: 45`) and `.rscrim` (`z-index: 44`) slide cleanly above the stage top handles.
   - Ensure shell-owned topbar items (narrow `#railbtn` and center `.usage-toggle`) stay functional on `/` (`.top.sh-own`) and `/w/<path>` as well as `/s/<rid>`.

2. **Purpose-separated `34px` hanging handles (`dashboard.tmpl`, `dashboard.css`, `dashboard-shell.css`):**
   - Style `.top` children as uniform `34px` top-edge hanging handles (`border-radius: 0 0 8px 8px`, `background: var(--wb-card)`, `box-shadow: var(--wb-shadow-hi)`, `gap: 6px`), matching the existing top-right handle aesthetic.
   - Left cluster:
     - Session identity handle in compact 2-row stack (`v4-1`: row 1 = project/workspace name + optional compact metadata, row 2 = `.sname` label/reply-id copy button + `.sren` inline rename pencil & `.sinp` input).
     - Harness/model handle (`.hb` + harness/model button + `hosted` pill when hosted; preserve `r-head` click-to-open session menu and `data-fpopover`/`data-lifted` alignment, plus session-head status chips like failure/MCP/ended/nolive).
   - Center cluster:
     - Shell-owned `.usage-toggle` hanging handle alone (no `live N · ? N` handle).
   - Right cluster:
     - Untouched iframe-owned handles (`[+]`, tracker, `#ghandle`, `#shandle`, `#drhandle`).

3. **Relocate plan usage to top center handle, fix badge overlap, and clean `.rfoot` (`dashboard.tmpl`, `usage.css`, `usage.js`, `dashboard-rail.css`, `dashboard-lifecycle.js`):**
   - Remove `.usage-toggle` from `r-rail`'s `.rfoot`; style `.rfoot` as clean `[⚙ Settings][⌂ Home]` controls (`#gear` remains first child of `.rfoot`; `#gmenu` stays outside `data-region="rail"`).
   - Mount `.usage-toggle` inside the shell/standalone topbar center slot and update `WsUsage.mount` so it binds to the header `.usage-toggle` (not restricted to `#rail`) and opens `.usage-pop` anchored beneath the top handle via `WsRail.popPlace`.
   - Fix `.usage-mark` and `.usage-clock` sizing in `usage.css` (using `.hb.sm` or exact badge width + right gap) so the `Cl`/`Co` badge and stale clock icon never overlap the `5h`/`주` columns.
   - Fix `.shp-sub` `padding-left` in `dashboard-rail.css` from `23px` to `31px`.

4. **Tests and visual verification:**
   - Update affected Go, JS unit, and Chromium browser tests (`i18n_test.go`, `usage.test.js`, `settings-root.browser.cjs`, `rail-navigation.browser.cjs`, `session-switch-header.browser.cjs`, `rail-worktree-admission.browser.cjs`, and any shell header layout tests) to verify the new DOM layout, zero `.top` height drift across session switches, non-overlapping `.usage-mark` geometry, full-height wide rail, narrow overlay z-order over `.top`, and clean `.rfoot` at 360px.
   - Run Go tests on remote `w5090` (`go test ./internal/wsdashboard/...`) and JS/browser tests where available.
   - Capture dark-theme screenshots (wide `>=1600px`, narrow `<1600px` closed and open, usage popover open) and compare side by side against `ai-docs/assets/mockups/261119/shell-redesign/shell-redesign-v4.html` (LOOK verification), plus a light-theme sanity check.
