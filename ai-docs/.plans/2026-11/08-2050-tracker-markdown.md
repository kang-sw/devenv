# Plan: tracker bodies and comments render as markdown

Branch `delegate/tracker-markdown` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`), based on
develop. Commit here; do not merge or push. Do not run git stash.

## Why

The user (board, 2026-11-08): tracker item bodies show raw markdown
(`# 이거는 마크다운 테스트` prints literally). Reuse the markdown renderer the
whiteboard topic bodies and the dashboard conversation already use
(`agents-plugin-tool/internal/wswhiteboard/markdown.go`, exposed to the
dashboard as `wswhiteboard.RenderText`, see `wsdashboard/pageview.go` and
`file_route.go`), instead of adding a second, JS-side markdown renderer.

## Decisions

1. **Rendered on the daemon.** The tracker view the daemon serves carries,
   next to each item body and each comment text (and the notes on status /
   close / reject / return / handoff events if the renderer shows them as
   prose), its rendered HTML. `tracker.js` uses the HTML when present and
   falls back to today's escaped plain text when absent, which keeps the
   mockup (file://, no daemon) working unchanged. Read
   `wsdashboard/tracker_routes.go` first: the dashboard knows the tracker only
   through JSON-level types, so add the rendering where that boundary allows
   (in the daemon's TrackerHost implementation in `cmd/ws-mcp/tracker_host.go`
   or in the route over the JSON), keeping `wsdashboard` free of tracker
   packages and `wstracker`'s view-state shape unchanged for its Go golden
   tests. Pick the placement with the smallest contract change and say why.
2. **Safe mode, because the content is other people's.** Tracker text comes
   from every member of a team store, so it is untrusted input to the user's
   page: render with raw HTML off (typed HTML shows as text; the renderer
   already has a rawHTML switch, `blockMarkdown(..., rawHTML bool)`), no
   whiteboard item references (`{{12}}`), no local file-path links or file
   actions, no local images; http(s) links only, opened with
   `rel="noopener noreferrer"` and `target="_blank"` as the dashboard does
   elsewhere. Mermaid fences may render if the existing path makes that free;
   otherwise they stay code blocks. Add tests that `<script>`, `<img onerror>`,
   `javascript:` links, and raw `<div>` arrive as inert text.
3. **Look**: the rendered body uses the same typographic styles as whiteboard
   topic bodies / conversation messages (headings, lists, code, tables, quotes)
   scoped under the tracker's root so nothing leaks; headings inside an item
   body are scaled down so an `# h1` in a body does not outshout the item
   title. Comments render the same way, compact.

## Constraints

- Another delegate (branch `delegate/tracker-interaction`) is reworking
  `tracker.js` / `tracker.css` heavily right now, including a wrapper around
  the item-window body for margins. Keep your `tracker.js` change to the
  functions that turn a body/comment string into HTML plus the minimal call
  sites, and your CSS to a new scoped block, so the merge stays small.
- `TestRenderPageEmbedSizeBudget` (wswhiteboard) has ~1 KB headroom; do not
  grow the whiteboard embed.
- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing.
- Update the mockup README only if it states bodies are plain text.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./internal/wsdashboard/ ./internal/wswhiteboard/ ./cmd/ws-mcp/ && go test -count=1 ./internal/wsdashboard/ ./internal/wswhiteboard/ ./internal/wstracker/ ./internal/wstrackerhost/ ./cmd/ws-mcp/`, `node --check` on tracker.js. Live check: the sim daemon `http://127.0.0.1:29503/` (member bob, store team-sim; see `~/.ws-sim/tracker/README.txt`) after you run your built binary there is NOT required; instead unit-test the view JSON, and screenshot the mockup to confirm the fallback still renders. Do not open the drawer on the real daemon 29417 (it marks the user's badges seen) and do not write to any sim store.

## Return

Commit hashes, changed files, where the rendering lives and why, the safe-mode
tests, test output, open items.
