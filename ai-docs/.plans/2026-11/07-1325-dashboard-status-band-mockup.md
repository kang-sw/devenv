# Plan: static mockup of a session status band for the dashboard session page

## Outcome

The user wants the live session page to show, per session, what their
terminal status band shows, so the page can stand in for the terminal. Build
a static HTML mockup only (no product code) for review. The user's terminal
band today shows (screenshot, 2026-11-07):

- model and effort: `claude-opus-5-5 [high]`
- mailbox id: `id:affluentxboxspongeprelaw`
- folder: `devenv`
- branch: `develop`, working-tree diff stat `+97 -13 ~2`
- context: `45% 137,609 / 300K` with a fill bar, plus `⚡ 97.0%` (cache hit rate)
- weekly usage: `17%/Sun (-72%)` (percent used, reset day, pace delta)
- cost: `$99.31`
- agenda keys: `dashboard-epic · dashboard-round2 · ...`

Requested content, in the user's priority order (2026-11-07): folder name,
branch state, branch sync (push/pull: ahead/behind upstream), usage or cost
when available, and above all the current context usage.

## Settled constraints

- Write the mockup under `ai-docs/ref/whiteboard-mockup/r5-status-band/`
  (one `index.html`, self-contained CSS, no external assets), next to the
  earlier `r4-dashboard` mockup. Reuse the real page's look: read
  `agents-plugin-tool/internal/wsdashboard/assets/` (dashboard.tmpl, CSS) and
  the r4 mockup, and match colors, fonts, spacing, both themes (a
  light/dark toggle on the mockup page is fine).
- Show the band in place on a trimmed copy of the session page (header,
  agents/conversation split, composer) so placement is judged in context.
- Give 2 or 3 clearly labeled placement/density variants (for example: a
  compact one-line strip under the page header; a two-row band like the
  terminal; context folded into the composer edge). Each variant is
  readable on a narrow phone width too (the user uses the page on mobile);
  include a narrow-width preview of each.
- Context usage is the most prominent element: a fill bar with percent and
  tokens/limit; show the states normal, warning (around 80%+), and critical
  (around 95%+) with the same visual language for each variant.
- Branch sync: show ahead/behind (`↑2 ↓1`), up to date, no upstream, and the
  dirty diff stat separately from sync.
- Usage/cost may be unavailable on some hosts (for example Pi without cost
  data): show how the band looks when those fields are absent.
- Static placeholder data only; no JS beyond theme/variant toggles.

## Boundary

Only the new mockup directory and this plan. No change to the product
template, CSS, or Go. Commit both on the worktree's branch; do not merge or
push.

## Verification

Open the file in a browser (or render it headless if available) and check
both themes and the narrow width; report anything not checked.
