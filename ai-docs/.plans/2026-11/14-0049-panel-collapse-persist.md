# Keep a hand-collapsed board panel collapsed (ticketless UX fix)

Source: owner request relayed by MAIN, 2026-11-13 (board topic
`panel-collapse-persist`). Owner's words: "제가 지금 계속 이 메시지 판넬 손으로
접고있는데 다시 펼쳐지네요 ... 접힌 건 유지되고(로컬 스토리지? 필요할지도 ..
세션 왔다갔다 할 때 감안하면요), 다시 같은 id로 overwrite할 때에만 다시
펼쳐지게 하면 좋겠는데." Screenshot:
`/Users/kang-sw/.cache/ws@kang-sw-devenv/dashboard/sessions/sweatradartakingoverwrite/inbox/attachments/6f685450ea0084f2-1.png`.
It shows a topic stream where whiteboard panels (`details.entry-panel`), folded
by hand, sit between log folds.

## Outcome

- A panel the user collapsed by hand stays collapsed:
  - across board updates and swaps;
  - across switching sessions and coming back;
  - across a page reload.
- It opens again only when that panel is overwritten: a new version under
  the same panel id, which is a new entry.
- The new version follows the server's default `open`. The collapsed choice
  for the old version does not carry to the new one.
- A panel the user opened by hand keeps today's behavior.

## What exists (read and verify; do not trust)

- `dashboard.js` ~123-157 keeps details open state in memory (`openState`,
  `openKey`, `restoreOpen`, `forgetOpen`). A panel entry's key is
  `k:entry:<entry id>`, with `:history` for historical versions, via its
  `data-key`.
- `dashboard.js` ~7190-7250 (the draft) saves only open=true keys.
  A collapse is therefore lost when the page reloads, and the server's
  `open` on the latest panel (`page.tmpl` "entry", `{{if .Open}} open`) wins.
- Find the real cause of the owner's reopen before fixing. Possible causes:
  - a session switch or reload loses the memory state;
  - a board swap replaces the node and the restore misses it;
  - the key changes between renders.
- Report the cause you found, with evidence.
- The standalone whiteboard page's `live.js` keys open state on the panel id
  (`wb-open-<pN>`). That key would survive an overwrite, so do not copy that
  keying for this rule.

## Requirements

- Persist hand collapses of panel entries in `localStorage`.
  - Scope the key by session and by the entry's key, e.g. its `data-key`, so a
    new version, which is a new entry, is not matched.
  - Use the page's existing storage helpers (`save`/`load` or the like) and
    key prefix style.
- Bound the storage: prune entries for panels no longer on that session's
  board, or keep a small capped LRU. Choose one and state which.
- Restore it on the same path as today's `restoreOpen`, so a swap, a session
  switch and a reload all respect it.
- When the user opens the panel again, remove its stored collapse.
- Apply this to board panels only (`details.entry-panel`). Other folds keep
  today's memory-only behavior.
- Compatibility: browser storage only, so T0. If the fix seems to need a
  server or store change, stop and return to the lead first.

## Boundary

- Touch `wsdashboard/assets/dashboard.js` only. Add a CSS or template
  change only if the cause demands it, and say why. Keep it surgical,
  because a perf worker and other UX branches are editing the board assets.
- No i18n changes.
- Do not touch the shared daemon:
  - No `scripts/dev.py`.
  - Kill nothing you did not start.
  - Never contact 127.0.0.1:29417.
  - For headless checks, build `ws-mcp` to `/tmp` and run your own daemon:
    `WS_CACHE_HOME=<tmp> WS_CONFIG_HOME=<tmp> HOME=<tmp> <bin> dashboard --port 0`.
    Seed sessions and panels through `ws-mcp serve --stdio` against the same
    temp cache; `/tmp/uxqa/qa*.js` shows a working setup.
    Stop the daemon by PID when done.

## Verify (headless Chromium, your own daemon)

- Collapse the latest panel. It stays collapsed after:
  - (1) another topic updates (a board swap);
  - (2) the same topic gets a non-panel entry;
  - (3) switching to another session and back;
  - (4) a page reload.
- Overwrite the same panel id with a new version: the new version is open.
  Its older version, now under `.panel-versions`, keeps its own state.
- Open it again by hand, then reload: it stays open, and its storage key is
  gone.
- A panel never touched behaves as before.
- `go test ./internal/wsdashboard/...` from `agents-plugin-tool`.
