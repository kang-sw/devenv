# Static mockup: dashboard git drawer commit graph

User request (board, 2026-11-09, verbatim): "흠, 근데 검토에서 일단 그래프 뷰 목업만 좀 먼저 만들수 있을까요?" — a static mockup of the graph view for review before the browser side of `261109-feat-ws-dashboard-git-commit-graph` is built. A worker is building that ticket's server side meanwhile; this mockup decides the look.

## Inputs

- Ticket: `ai-docs/tickets/ready/261109-feat-ws-dashboard-git-commit-graph.md` (Decisions: lanes for local branches + upstreams with impl/goal icons and worktree markers; hover tooltip; click opens a centered floating panel, item-window style; right = file list, left = multi-file unified diff viewer; copy-hash button; first-parent for merges).
- Look and tokens: `agents-plugin-tool/internal/wsdashboard/assets/dashboard.css`, `dashboard.tmpl` (the git drawer `.gleft` placeholder and `gitbody`), the git drawer's right panel and worktree card; the tracker item window (`assets/tracker.js` / its CSS) for the floating-panel frame and the `idcopy` copy button.
- Existing mockups as format precedent: `ai-docs/ref/whiteboard-mockup/r5-status-band/index.html` (single self-contained static page), `ai-docs/ref/tracker-mockup/`.

## Outcome

`ai-docs/ref/git-graph-mockup/index.html`: one self-contained static page (inline CSS/JS/SVG, no network), opened from the filesystem, showing:

1. The git drawer as it looks in the dashboard (drawer frame, right-side status panel roughly as today) with the left side filled by the commit graph: inline SVG lanes and dots, merge edges, branch/upstream labels with impl/goal icons and abbreviations like the handle, a worktree checkout marker, HEAD marker, ahead/behind of an upstream, subject / author / relative time columns, and a "more" row at the bottom. Fixture data: realistic, ~40 commits modelled on this repo's shape (develop, main, two or three `impl/develop/<slug>` branches, one `goal/...`, merges into develop, `origin/develop` behind by a few commits, a detached worktree).
2. Hover tooltip on a commit (subject, author, relative time, short hash, refs).
3. Click opens the floating commit panel: header (full message, author/committer, times, copy-hash button, parents), right file list (status letter, rename source, +/- numstat), left multi-file unified diff viewer with a few file sections: one with highlighted code (fake class-based token colours), one large file behind a "load diff" button, one binary, one deleted, and intra-line change marks on a paired line. Clicking a file in the list scrolls to its section.
4. Light and dark themes via the dashboard's own tokens (a theme toggle on the page), and a narrow-width view note (under 760px the graph is hidden, per the ticket).
5. A short on-page legend listing which parts are Phase 1 vs Phase 2.

Mockup JS may be plain inline script with hard-coded fixtures; no build, no tests. Reuse the dashboard's CSS tokens and class conventions so the worker can lift styles. No change to shipped code.

## Verification

Open the page in headless Chromium (as earlier dashboard workers did) and take screenshots: drawer with graph (light + dark), tooltip, open panel with diff. Save screenshots to the scratchpad, not the repo. Commit only the mockup page (and this plan) on branch `git-graph-mockup`. Return the screenshot paths and the page path.
