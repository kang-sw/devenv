# Plan: dashboard Agents panel - full live state only for same-worktree peers

## Outcome

The user (2026-11-07): "현재 작업 상태를 표시하는 메일박스는 워크트리가 같은
세션으로 한정하는 게 좋겠어요. 다른 메일박스 세션은 Agents 패널에서 더 작게
표현하거나". Confirmed on the whiteboard as items (83)-(87):

- (83) In the session page's Agents panel "peer leads" group, only peers in
  the **same worktree** keep today's full row: an expandable tree with the
  live phase, the running tool, finished batches, and the subagent tree.
  "Same worktree" means the peer's registration `Project` (session root path,
  `internal/wsdashboard/registry.go` `Registration.Project`) equals the
  viewed session's `Project`. Unserved peers are already one-line rows and
  stay unchanged.
- (84 b) A served live peer in a **different worktree** renders as one
  compact line with no expansion: the name linked to its session page, the
  harness, the current phase mark (the spinner, ✓, and so on, as
  `template "mark" .Lead.Phase` gives today), and the last-mail time. It
  shows no running tool, no batches, no subagent tree, and no subagent count.
  Ended peers keep their current row.
- (85) Ordering: same-worktree peers first, then other-worktree peers, each
  group still by most recent mail exchange. The live/ended split and the
  ended-peer cap (`endedPeerCap`) keep their current semantics.
- (86) The panel header's mail chip still counts every live peer, whatever
  its worktree.

## Settled constraints

- Expected files: `agents-plugin-tool/internal/wsdashboard/pageview.go`
  (`peerGroup` needs the viewed session's Project, and `PeerRow` needs a
  same-worktree flag or equivalent), `assets/dashboard.tmpl` (the
  `peer`/`peerrow` templates), `assets/dashboard.css` if the compact row
  needs a style, and the tests in `pageview_test.go` and `pages_test.go`.
- Compare paths after `filepath.Clean`. Do not resolve symlinks, and do not
  query git.
- The other-worktree compact row must not run the peer's subagent tree
  lookup (`agentTree`), so the page does no work it does not show.
- A parallel ticket worker is editing `dashboard.js` and the reply render.
  Do not touch `dashboard.js`, and keep the template change local to the peer
  templates.
- No protocol, route, or registry schema change.
- Shipped-surface boundary applies
  (`ai-docs/manuals/shipped-surface-boundary.md`).

## Boundary

Commit on branch `delegate/peer-worktree-compact` only; do not merge or push.
If the viewed session's Project is not reachable where `peerGroup` is called
without a wider refactor, stop and report.

## Verification

In agents-plugin-tool:
- `go build ./... && go vet ./internal/... ./cmd/...`
- `go test -count=1 ./internal/wsdashboard/`
- Unit tests for:
  - a same-worktree served live peer with an adapter keeping the full tree;
  - an other-worktree one rendering compact, with the phase mark and no
    tree, tool, or count;
  - the ordering;
  - the mail chip counting both.
- A headless Chromium render of a session page with one peer of each kind,
  light and dark, using the cached Playwright under
  ~/.npm/_npx/*/node_modules/playwright.

Report what was not checked.
