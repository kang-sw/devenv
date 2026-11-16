# Dashboard memo pad polish: C2 masonry (Google Keep style), 480px

## Context

Owner 2026-11-16: memo (post-it) popover polish mockup, wider. Variant pick
(1264)=d (C2 masonry): "Google Keep style; good enough, take this as the
first draft". Approved mockup:
`ai-docs/assets/mockups/261116/memo-polish/memo-polish.html` (`#v=c2`, all
states incl. `s=mixed` and `s=live`), committed 9bfa65f72.

## Outcome

1. The memo pad matches mockup variant C2: 480px wide (in a `--memo-w`
   variable), neutral pad, B's paper notes (muted amber, glue strip, folded
   corner), two columns placed shortest-column-first in list order, no
   minimum height, no length rule.
2. Placement is kept when an edit is saved; re-placement happens on add,
   delete, undo, workspace switch, load, and pad width change. The memo being
   edited grows in place in its column.
3. One column under the existing narrow/mobile rule (< 700px) and when the pad
   is too narrow for two columns.
4. Shared changes from the mockup: no resize grips (editor grows with
   content), header with icon / "Memos" / workspace chip (ellipsized, full
   path on hover) / count / 28px close, key-hint row inside the add box with
   reserved space (shortened in the narrow column), delete fades in on
   hover/focus and turns red on its own hover, undo as a bottom snackbar (same
   5 s), empty-state line, `word-break: keep-all` + `overflow-wrap: anywhere`.
5. All existing behaviour kept: click/Enter to edit, Enter saves, Shift+Enter
   newline, Esc cancels, links open in a new tab, 8000-byte limit message,
   read-only on LAN, topic picker, undo.
6. A light-theme value for the paper colour is provided.

## Constraints

- View-only (T0): no store, MCP or API change; memos stay `{id, text}`.
- Files expected: `internal/wsdashboard/assets/memo.css`, `memo.js`,
  `dashboard.tmpl` (`memopad`), possibly `.wsc-editor` rules in
  `dashboard.css`. No new CSS class names that collide across modules
  (TestCSSClassesDoNotCollideAcrossModules).
- Memo strings stay as today (hard-coded English); i18n is out of scope.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing
  `agents-plugin-tool/`.

## Verification

- Go tests ONLY on remote w5090 (`D:/sys_lock.sh remote w5090 run ...`); NO
  local `go test`, test exes, ws/ws-mcp exes or launcher. Node tests run
  locally. Acceptable reds: the two Node-only TestTopicStateBadge* on w5090.
- Node test for the placement function: shortest-column order, ties left,
  placement kept on save, re-placed on add/delete.
- LOOK: side-by-side against the mockup `#v=c2` at the same states (list,
  mixed 8, edit, hover delete, undo, empty), dark and light, via a static
  harness page with the real CSS/JS. Temp under D:/Temp/memo-polish-impl/,
  deleted after (keep the comparison PNGs until the lead confirms).
- R1 review; fixes-only R2 for substantive findings.
