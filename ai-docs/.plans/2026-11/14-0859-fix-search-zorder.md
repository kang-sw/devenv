# Fix: search button click + z-order layering

## Bug 1: Shell header search button click does nothing
The shell header's search button (🔍 검색 Ctrl K) does not open the search
palette when clicked. Keyboard Ctrl+K works. The click handler is `shSearch`
(dashboard.js ~line 7518) which clicks `#tkqs`. The click path broke on
develop@f70d75040.

## Bug 2: Session cards render above popups and fullscreen
The "claude" harness chip dropdown and the fullscreen/maximize view render
behind session cards. The session card iframe shell likely creates too high
a stacking context. The maximize iframe also renders behind cards.

## Scope
1. Diagnose and fix the search button click path.
2. Fix z-order: define a clean z-index scale in dashboard.css so that
   cards < floating panels < popovers < fullscreen/modal. The session
   iframe shell's stacking context must not override page-level overlays.
3. Add a regression test for the search button click (Go asset test).
4. Run `go test ./internal/wsdashboard/...` and confirm pass.
5. Commit on branch fix/ux-search-zorder. Do NOT push.

## Prior work
A stopped Pi worker produced a partial patch (129 lines) saved at
/tmp/pi-search-zorder-wip.patch. Read it for any useful diagnosis but
verify independently — it was written on a wrong branch and may be stale.

## Constraints
- Do NOT touch the shared daemon (port 29417) or run scripts/dev.py
- Work ONLY in /Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying
- Minimal change only
