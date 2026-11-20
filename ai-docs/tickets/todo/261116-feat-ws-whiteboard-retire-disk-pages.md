---
title: "Retire the whiteboard's disk-rendered pages (file-only mode) in favor of daemon-served boards"
related:
  261116-feat-ws-dashboard-session-page-payload-and-render-cost: "sibling; the payload ticket explicitly excludes this retirement, and this one would unblock the client-side rendering it rejected for now"
  261116-feat-ws-dashboard-generalized-mentions: "sibling undesigned todo from the same day; same decision-queue handling"
---

# Retire the whiteboard's disk-rendered pages (file-only mode) in favor of daemon-served boards

## Background

Owner statements, 2026-11-16 (verbatim Korean, then gist):

> "어차피 이제 데몬을 무조건 켜게 되어있으니까 file only whiteboard mod도 많이 의미가 식은 거 같고, 그래서 아예 파일만 읽는 화이트보드 모드를 버리고 그냥 데몬의 실시간 전용으로 쓴다 그러면 예를 들어 클라이언트 사이드 렌더링을 하고 데이터만 fetch해오는 식으로 상당히 최적화가 될 수 있을 거 같아서요." (22:41)

Gist: the daemon is always running now, so a whiteboard mode that only reads files has lost its point; drop it and make the board daemon-live only, which would allow large optimization such as client-side rendering that fetches only data.

> "embed도 사실 버려도 되긴 합니다. 이제 안 쓰는 경로거든요." (23:02, about the `embed:` disk file)

Gist: the `embed:` file can be dropped too; it is a path nobody uses anymore.

Decision origin: whiteboard item (1205)=a split the work. The payload and render-cost reduction goes first (`261116-feat-ws-dashboard-session-page-payload-and-render-cost`); this ticket captures the retirement as a separate piece. It is functionality removal, so AGENTS.md `### Approval Protocol` "Always ask" applies and the owner must approve the final design.

### Code facts (re-verified at drafting; line numbers drift, re-check)

Paths are relative to `agents-plugin-tool/`.

The disk writer is `renderWhiteboard` (`internal/mcp/whiteboard_tools.go:499`, running in the MCP server process). It writes `ai-docs/whiteboards/<rid>/index.html` (`RenderPage`, `LayoutFull`), `index.stamp.js` (`:556,563`), and an embed page `<rid>-<hash>.html` (`LayoutEmbed`) or into the host's embed dir (`EmbedDir`; the host-owned-folder branch is around `:543-549`). Outputs publish after the board commit (`record.boardOutput.publish()`, `internal/mcp/session_state.go:666`). Mermaid is copied beside the page when needed (`ensureWhiteboardMermaid`, `:569`). It runs on every `whiteboard.update` (`:277`) and, through `refreshWhiteboardAfter` (`:462`, called from `internal/mcp/server.go:826`), after todo and agenda tools, as a full-history render with no memo.

The update result reports `full:` as the daemon URL when a daemon serves the session (`dashboardPageURL`, `internal/mcp/dashboard_sessions.go`, used at `whiteboard_tools.go:327-331`), else the disk path; `embed:` is always a disk file (`:332`).

Code that exists only for the disk pages:

- `RenderPage` (`internal/wswhiteboard/render.go:308`) and the `.Full` branches of `internal/wswhiteboard/assets/page.tmpl` (lines 1-12 onward). The `.Embed` branches of `page.tmpl` moved to `261119-feat-ws-whiteboard-retire-embed-snapshot` (`(1818)`, `(1821)`); only that ticket claims their deletion.
- `live.js` stamp polling and the standalone viewer/change-mark script bundle (`render.go:550-554`, `chgWire` at `:524`).
- `StampFile`, `FileLink`, `FullBoardLink` (`internal/wswhiteboard/view.go:33-60`; `fileHref`, `markdown.go:577-585`).
- The unframed branches under `DocFramed=false` (`render.go:607,617-626`, `:825`).
- The `served:false` panel variants (`whiteboard_tools.go:105-114`).
- Orphan GC (`whiteboardGCCadence` and the once-a-day marker, `:39-43,741`). The embed decision `(1820)=a` in `261119-feat-ws-whiteboard-retire-embed-snapshot` relies on this GC and its `<reply-id>-<hash>.html` pattern to remove leftover embed files, so retiring the GC must keep or replace that cleanup. Embed eviction (`evictWhiteboardEmbeds`, `:642`) moved to `261119-feat-ws-whiteboard-retire-embed-snapshot` (`(1818)`, `(1821)`); only that ticket claims its deletion.
- i18n subsetting for static pages (`render.go:154-176`).
- About 19 `wswhiteboard` test files call `RenderPage`.

The authoritative board store (the per-reply-id board folder metadata, committed state and similar) is not a rendered page; the leading expectation is that it stays.

Shipped docs describe `full:`, `embed:`, the host embed folder and the `wb:embed-dir` command (`agents-plugin/rsrc/whiteboard/whiteboard.md`, around lines 40, 101, 330, 455-475, and the mirrored `agents-plugin-wsflow` copy). The lead must not edit that doc as part of drafting; any doc change is either owner-edited or a separate owner-approved step recorded in this ticket.

## Constraints

- Read before editing, per AGENTS.md `### Implementation Conventions`: `ai-docs/manuals/shipped-surface-boundary.md`, `ai-docs/manuals/release-compatibility.md`, `ai-docs/manuals/ws-mcp.md` (for `internal/mcp/`); for the shipped doc and rsrc: `skill-authoring.md` and `wsflow-mirroring.md`.
- Compatibility tier must be settled in `## Decisions` before `ready/`, with cited read/write sites per AGENTS.md `### Compatibility Tiers`. Non-authoritative: keeping `wb:embed-dir` and the `embed:` result line as accepted no-ops is T0/T1 (old plugin text keeps working, nothing breaks, the line just carries no file); refusing them is a T3-like boundary for installed plugin text and needs the owner's explicit approval.
- Shipped text must not name this repository's own posture (Architecture Rules 4 and 5); the downstream user is the audience.
- Out of scope here: the payload and render-cost work in `261116-feat-ws-dashboard-session-page-payload-and-render-cost`, and any client-side rendering redesign (this ticket only removes the disk pages; it may enable that later).

## Open Questions

None is decided; each settles through the Open Decision Queue before `ready/`.

- What `full:` returns when no daemon serves the session: an error, or an auto-start of the daemon.
- The embed slice (`embed:` line, `wb:embed-dir`, host embed users, old embed text, takeover-recipe dependence on `wb:embed-dir`) moved to `261119-feat-ws-whiteboard-retire-embed-snapshot` (`(1818)`).
- Whether the authoritative board folder store stays (expected yes; only rendered pages retire).
- Whether existing `ai-docs/whiteboards/**/*.html` files are deleted by a migration, left in place, or ignored by git.
- Whether the shared template and renderer split so the daemon page no longer carries the full-page disk branches, and what the ~19 `RenderPage` tests become (the embed layout branches go with `261119-feat-ws-whiteboard-retire-embed-snapshot`).

## Phases

### Phase 1: Retire disk pages

Not yet designed. Design settles through this ticket's Open Decision Queue before it moves to `ready/`. Owner approval is required (functionality removal).
