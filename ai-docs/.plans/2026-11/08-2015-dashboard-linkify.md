# Plan: dashboard auto-link of bare URLs and file paths

Branch `delegate/dashboard-linkify` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`), based on develop.
Commit here; do not merge or push. Do not run git stash. Read
`ai-docs/manuals/shipped-surface-boundary.md` before editing under
`agents-plugin-tool/`.

## Why

User (board (493) ok, (494) a): text that should be clickable - a bare
`http://127.0.0.1:29417/s/...` URL in a lead reply, a file path - renders as
plain text on the dashboard. Today goldmark (`internal/wswhiteboard/markdown.go`)
links only `[text](dest)` and `<https://...>`.

## Settled

- (493) Bare http(s) URLs become links on every markdown pipeline: item
  fields (inline and block), topic bodies (raw pipeline, markdown text only,
  never inside raw HTML), and lead replies on the dashboard (find where
  finished and in-progress replies are rendered; `sanitize.go` filters the
  finished-reply output). Use goldmark's GFM `extension.Linkify` (or its
  parser) rather than a hand-written URL regex, restricted to http/https
  (no `www.`-only or email autolinks unless Linkify forces them; if it does,
  make sure they still pass `resolveTarget`).
- (494) a: also file paths, by pattern, outside code spans and code blocks:
  - absolute paths starting with `/` (and Windows drive paths `C:\` / `C:/`),
    with at least two segments;
  - relative paths with at least one `/` and a final segment carrying a file
    extension (`ai-docs/x.md`, `internal/a/b.go`);
  - either form with a `:line` or `:line:col` suffix (`path.go:12`); the
    link target drops or carries the line the way the existing link-action
    menu and `fileTarget` already handle a suffix (check how `FileLink` /
    sysopen treat it; do not invent a new URL scheme).
  - Trailing sentence punctuation (`.`, `,`, `)`, `:` not followed by a
    digit) is not part of the path. A path inside prose words like `a/b`
    without an extension stays text.
  - Whether the path exists is not checked at render time; the click path
    (link-action menu: open in browser, open in system, copy) handles it.
  - Not in scope: ticket stems, commit hashes.
- Every generated link goes through the existing `resolveTarget` allow-list
  and renders exactly like an author-written link to the same destination
  (same classes, same data attributes, same link-action menu). No new
  security surface: a generated link must never produce an href the
  allow-list would refuse for a written link.
- The plain-text file render (`TestFileTextRendersFileTargetsAsPathText`
  family) and any embed or file page keep their current link behavior for
  their medium; follow what those pages already do for written links.

## Tests

Go tests in `internal/wswhiteboard` (and wherever lead replies render): bare
URL in a title, option, topic body markdown, and a reply becomes a link;
URL inside inline code and fenced code stays text; URL inside raw HTML in a
topic body is untouched; trailing `.`/`)` excluded; absolute path, relative
path with extension, `path:line` become links; `a/b` prose and a path in
code stay text; a generated link's href equals the written-link href for the
same destination.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./internal/wswhiteboard/ ./internal/wsdashboard/ ./internal/mcp/ && go test -count=1 ./internal/wswhiteboard/ ./internal/wsdashboard/ ./internal/mcp/`
plus `node --check` on changed JS. If any client-side renderer is touched,
a short headless smoke (Playwright under ~/.npm/_npx) is welcome but not
required.

## Return

Commit hashes, changed files, test output, the exact path patterns you
implemented, and anything left open.
