# Panel "view large" button (ticketless UX fix, (527)=a)

Source: owner request relayed by MAIN, 2026-11-13 (board topic
`panel-float`). Owner's words: "메시지 보드 우상단에 floating으로 보기(그 지금
파일이나 그런 카드에도 있는 버튼이요) 따로 있어야 할 거 같아요. 불편할 때가
종종 있네요 .." Screenshot:
`/Users/kang-sw/.cache/ws@kang-sw-devenv/dashboard/sessions/sweatradartakingoverwrite/inbox/attachments/d08b2aadfeab9fef-1.png`.
It shows a whiteboard panel (`details.entry-panel` with its sandboxed
`iframe.doc-frame`) inside a topic, clipped on the left because the panel's
content (a 1240px mockup) is wider than the topic.

## Outcome

- A panel with a body gets a "크게 보기" button at the right end of its head
  row (`summary`, after `.pmeta`). It is the same button board cards have
  (`cards.js` `bc-max`, icon `maximize-2`, tooltip `js.card.max`).
- Pressing it shows that panel large over the board, as a maximized card is
  shown (`cards.js` "the maximize sheet", `dashboard.css` `[data-max]` and
  `.bc-scrim` ~648-654): fixed over both columns with a scrim behind it.
- A "back" control in the same place (icon `minimize-2`), Escape, or a
  scrim click restores it. Focus returns to the button.
- While large, the panel's content gets the large width. A content taller
  than the box scrolls inside the box.

## Technical decisions (settled by the lead)

- **In place, never moved.** Like the card maximize sheet, the panel node
  stays where it is and is drawn `position: fixed` by an attribute and CSS.
  - Never move, clone or re-create the iframe, and never change its
    `srcdoc`, `data-doc`/`data-doc-key` or the docKey/postMessage bridge.
  - The frame must not reload: a script's state inside it survives a
    maximize and restore.
  - The frame's existing height reports keep working. The box scrolls if
    the frame is taller than the box.
- **Clicks.** A button click must not toggle the `details`. A closed
  panel opens as it is maximized.
- **Board swaps.** A board swap, a panel re-render or a new panel version
  may arrive while the panel is large (stream updates, `frameGraft` /
  `slideBoard` in `dashboard.js`). Leave no orphaned scrim and no stuck
  state.
  - Preferred: re-apply the state to the node with the same `data-key`.
  - Acceptable: restore to normal. Report which one you chose.
- **One at a time.** One thing is large at a time, a card or a panel.
  - Share the scrim and Escape with the card maximize sheet if that is a
    small change, e.g. a small hook exposed from `cards.js`.
  - Otherwise keep a separate scrim with the same look, and maximizing one
    restores the other.
- **Strings.** Reuse `js.card.max` for the button.
  - For the restore tooltip, reuse `js.card.min` only if its wording fits
    a panel. Otherwise add one new key to every locale table under
    `wsi18n/tables/`, keeping the i18n tests green.
- **Scope.** Panels on the dashboard's session board are the target.
  - Panel versions under `.panel-versions` may get the button too if that
    falls out naturally.
  - The plain whiteboard page (`wswhiteboard` page assets) must not break.
    A button there that does nothing is not acceptable: show it only where
    it works.

## Boundary (return to the lead before crossing)

- Return before acting if any of these is needed: moving or re-creating the
  iframe, a reload, a change to the doc-frame bridge or the frame's sandbox,
  or a Go renderer change beyond adding the button's markup.
- Also return if a containing block traps `position: fixed`, i.e. an
  ancestor of the panel has `transform`, `filter`, `contain` or
  `content-visibility`. Today none is set on the topic path; verify this
  yourself.
- Keep the change surgical. A perf worker is editing the board assets in
  parallel, and other UX fix branches touch `dashboard.css`. Do not reformat
  or move unrelated rules.
- Do not touch the shared daemon:
  - No `scripts/dev.py`.
  - Kill nothing you did not start.
  - Never contact 127.0.0.1:29417.
  - For headless checks, build `ws-mcp` to `/tmp` and run your own daemon:
    `WS_CACHE_HOME=<tmp> WS_CONFIG_HOME=<tmp> HOME=<tmp> <bin> dashboard --port 0`.
    Seed a session with a topic and a panel through `ws-mcp serve --stdio`
    against the same temp cache; `/tmp/uxqa/qa*.js` shows a working setup.
    Stop the daemon by PID when done.

## Verify

- Headless Chromium at 2560x1400, 1400x900 and 420x800, light and dark. Use a
  panel holding a 1240px-wide element and a `<script>` that counts up into
  the DOM.
  - After maximize:
    - the box is fixed over the board;
    - the iframe element is the same node, with no new load event and a
      counter that kept counting;
    - the 1240px content is not clipped at 2560;
    - inner scroll works.
  - After restore by the button, by Escape and by a scrim click:
    - layout metrics match those before;
    - focus is on the button;
    - no scrim is left.
  - A card maximize while a panel is large, and the reverse: only one is
    large.
  - A board swap or a panel update while large: no orphaned scrim. Report
    the behavior.
  - Clicking the button on a closed panel opens and maximizes it, and the
    details toggle does not flip back.
- `go test ./internal/wsdashboard/... ./internal/wswhiteboard/... ./internal/wsi18n/...`
  from `agents-plugin-tool`. The CSS collision guard must stay green.
