# Latest topic log highlight (owner 773)

## Goal
On the whiteboard page (agents-plugin-tool/internal/wswhiteboard, page assets), the most
recently added agent log line in each topic card gets a light-green highlight:

1. Persistent: the latest log line of each topic has a light-green background. When a newer
   log line arrives in that topic, the highlight moves to the new line (the old one returns
   to the normal background).
2. Blink: a newly arrived latest log line blinks light green 2-3 times (about 3 s total),
   then keeps the persistent light-green background.
3. Blink start timing (owner, verbatim): "이 타이머는 유저가 브라우저에서 "처음 확인한 시점"(시야에 들어온 시점)에 시작하기로 하죠."
   The blink starts when the line first enters the viewport (IntersectionObserver or
   equivalent), not when it is inserted. A line inserted off-screen blinks when the user
   first scrolls it into view; once blinked it never blinks again for that line.
4. Page load: lines already present at initial load show only the persistent background,
   no blink.
5. prefers-reduced-motion: reduce -> no blink, persistent background only.
6. Works in both light and dark themes (use existing CSS variables/tokens where possible).
7. Folded/collapsed history, closed topics: follow the same rule for whatever log line is the
   topic's latest visible agent log; do not change fold behavior.

## Scope
- Branch impl/develop/latest-log-blink-773, based on impl/develop/recap-clamp-758 (758 recap
  clamp, not yet merged; keep its changes intact).
- Files: wswhiteboard page assets (page.css, page.js or equivalent, page.tmpl only if a
  marker class/attribute is needed) plus focused tests in the existing wswhiteboard test
  style (Go template/render tests and/or existing headless test fixtures).
- No store, protocol, MCP, or server API change. T0 (dashboard asset content).
- Do not touch dashboard (wsdashboard) assets.

## Verification constraint
Static only. Do NOT run builds, go test, Node tests, browsers, probes, or daemons: the
machine's single heavy slot is held by another session. Run `git diff --check` only.
In the final message, list the exact deferred commands (focused go test / node test /
headless browser check) that the lead will run later, including a browser check that:
a new log appended off-screen does not blink until scrolled into view; on-screen append
blinks ~3 s then stays light green; previous latest loses the highlight; reduced-motion
shows no blink; page reload shows no blink.

## Commit
One commit with ## AI Context per AGENTS.md commit rules; include this plan file.
