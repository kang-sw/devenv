# Plan: an open topic's body frame never blanks while it reloads

## Goal

Fix idea ticket `261112-bug-ws-dashboard-topic-frame-reload-blank` (user
approved the direction, board 362 ok, 2026-11-12). When a topic that stays
open is updated, `morphBoard` re-creates its body frame (`iframe.doc-frame`):
today it is blank for about two frames, jumps to an in-between height, then
blanks once more. Keep the old frame visible until the new one has loaded and
posted its height, then swap (double-buffer).

## Scope

- `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`:
  - the swap path (`swap` ~L450-480: `frameHeights`/`setFrameHeights`
    ~L2307-2321, `morphBoard`, `growTopic`/`topicMotion`, the `wbGrow` flag);
  - the frame message listener (~L2211-2300: the `wbDoc` size post that
    sets the frame height) and the late-listener re-ask (~L5631).
  - Approach: when a changed topic's section replaces an old one that had a
    body frame of the same `data-doc`, keep the old frame in place and
    visible, and mount the new frame hidden (no layout cost, e.g. absolutely
    positioned and invisible inside the same body box) until it posts its
    first height. Then size the new frame, reveal it, and remove the old one
    in the same frame. Keep theme posting, image relay, link menus and mermaid
    working for the new frame. If the new frame never posts (error), reveal it
    after a short timeout so the topic never stays stale.
- `assets/dashboard.css` only if the hidden-mount needs a class.
- Keep the existing behavior: no animation on first load, the editing
  deferral, `prefers-reduced-motion`, unchanged topics keep their nodes
  (`boardSeen`), and the fold-flicker fix from merge 8c441efda.
- On success, close the idea ticket in the same commit: move
  `ai-docs/tickets/idea/261112-bug-ws-dashboard-topic-frame-reload-blank.md`
  to `.done/` (`git mv` is fine in this worktree) and add a short
  `## Result` saying what changed.

## Verification

- `~/.local/bin/ws-rgo -C agents-plugin-tool -m 6 -- test -count=1 ./internal/wsdashboard/...`
  (never run Go build/test locally).
- Headless frame-by-frame capture against the real assets, before and after:
  an open topic whose body changes must show no blank frame and no
  in-between height jump. The earlier repro harness is in the lead's
  scratchpad: `/tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/pw/repro.js`
  (playwright-core under that `pw/node_modules`; chromium at
  `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`). Copy and
  adapt it rather than editing it. Write frame sheets to
  `/tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/frame-dbuf/`.
- Also check: a newly added topic still slides in, a closed topic still folds.

## Branch

Commit on `delegate/topic-frame-double-buffer` in this worktree. Do not merge
or push. Stop and report if the fix needs a change outside dashboard.js/css
(for example doc-frame.js or the Go renderer) beyond a small hook.
