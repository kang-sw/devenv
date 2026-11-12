# A local browser previews git-ignored HTML; LAN viewers keep the refusal

## Outcome

User decision (whiteboard item 471, 2026-11-12, option b with a scope note):
render a git-ignored HTML/SVG file in the dashboard file card, keeping the
dot-path refusal (`.env`, `.git/`, `.ssh/`), but only for a viewer the
daemon already treats as this machine: "a daemon reached through localhost is
the same level as local". A LAN viewer keeps today's behaviour (an ignored
framed file shows as its source).

Background: 5c9fc575b added `capDenied` (dot path OR git-ignored) as the gate
for minting a preview capability and for the capability route; the page
route (sub-resources, `view`) only applies `dotDenied`. Paged documents and
build outputs a user wants to look at usually live in ignored folders, so
the current rule shows them as source (user report: the
`ai-docs/paged/file-view-demo/index.html` demo, whose folder has a
`.gitignore` of `*`).

The daemon's local-viewer test is `localViewer(r)` in
`agents-plugin-tool/internal/wsdashboard/access.go` (loopback peer and
loopback Host), the same gate terminals and other trusted writes use.

Change:

- Preview route (`handleFSPreview`, `fs_routes.go` ~line 601): for a local
  viewer, a framed file gets a capability unless `dotDenied`; for any other
  viewer, unless `capDenied` (today's rule).
- Capability route (`fs_capability.go` ~line 371, the `!view` branch): the
  same split, so the frame and its sub-resources load for a local viewer.
  Requests from the sandboxed frame still reach the daemon from the same
  machine, so `localViewer` holds for them; confirm this (the frame's Host is
  the URL `capabilityURL` builds from `r.Host`).
- Outside-root route (`fs_outside.go` ~line 535, the `capDenied` check for
  files other than the linked one): apply the same split, keeping `g.Only`.
- Update the comments that state the rule (fs_capability.go header list
  around lines 38-55, the `capDenied` comment, the `fs_routes.go` source
  fallback comment, the fs_outside.go header) so they state the local/LAN
  split.

## Boundaries

- Go + comments in `internal/wsdashboard` only; no asset change needed unless
  a test shows otherwise. T0 (no route or store shape change).
- Keep the dot-path refusal and the symlink-resolution checks for everyone.
- Do not change `localViewer` itself.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing.

## Verification

- Tests: a git-ignored framed file in a test repository gets a frame
  capability and is served by the capability route for a loopback request,
  and is refused (source fallback / 404) for a non-loopback request; a dot
  path is refused for both. Extend the existing 5c9fc575b tests rather than
  duplicating their fixtures.
- `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test ./internal/wsdashboard/ -count=1`
  (never run heavy go build/test locally).

## Return

Commit on `delegate/ignored-html-local` in worktree
`/home/swkang/.ws-worktrees/devenv/gamma-washcloth-everyone` with this plan in
the same commit. Do not merge or push. Report commit, approach, how a
sandboxed frame request satisfies `localViewer`, and test results.
