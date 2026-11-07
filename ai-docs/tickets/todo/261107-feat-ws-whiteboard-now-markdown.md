---
title: "Whiteboard: the Now slot takes markdown with inline HTML; lead replies render inline HTML"
related:
  261107-epic-ws-dashboard-messenger-round2: raised while dogfooding the round-2 session page; affects disk boards too, so not a child
  261107-feat-ws-dashboard-link-actions: same markdown renderer; served-page link targets apply to Now too
  261107-feat-ws-dashboard-conversation-history: done; its per-entry render cache renders a finished reply once
---

# Whiteboard: the Now slot takes markdown with inline HTML; lead replies render inline HTML

## Background

The Now slot takes a raw HTML fragment and no markdown; it is "the only HTML
channel" (`agents-plugin-tool/internal/wswhiteboard/markdown.go`), written
unescaped (`view.go` `Now`, `render.go` `NowHTML`), and `store.go` refuses a
full document. Every other board field takes markdown with raw HTML shown as
text. The user wants Now to take markdown like the other elements, with
inline HTML still allowed (2026-11-07: "now 바 쪽이 지금 raw HTML 받게 되어
있죠? 이거 그냥 다른 화이트보드 엘리먼트들처럼 평범하게 markdown 받게 하고,
차라리 inline html도 쓸 수 있게 하는 방향으로 가면 어떨까 싶어요?").

The user then asked for the same HTML embedding in the session page
conversation (2026-11-07: "conversation 쪽 출력에도 혹시 now bar와 같은 html
렌더링을 적용하는 게 가능할까요? html embedding이 가능하게요."). Conversation
bubbles render through `wswhiteboard.RenderText` (`markdown_text.go`), the
same pipeline.

## Decisions

- **Inline HTML in Now only, on the board** (user, 2026-11-07, "(55) a"): `now`
  and `now_path` are rendered as markdown through the board's renderer with
  raw HTML passed through; every other board field keeps raw HTML shown as
  text. (The conversation is not a board field; see the lead-reply decisions.)
  Rejected: inline HTML in all board fields - those carry user quotes and
  code, where a stray tag would vanish. Now is the lead's free canvas and its
  HTML trust is unchanged from today.
- **`now_path` names a markdown file** (user, "(55) a, note: 이건 그러면
  now_path가 html이 아니라 md 받는다는 것도 명시해야 할 거고, 아무튼 스키마
  관련해서 이것저것 편집이 같이 들어가야 할 거 같네요"): every schema text
  that describes `now`/`now_path` as HTML is updated in the same change: the
  three whiteboard playbooks (`agents-plugin/rsrc/whiteboard/whiteboard.md`,
  `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`,
  `agents-plugin-pi/rsrc/whiteboard/whiteboard.md`), the store's refusal and
  validation messages (`store.go`), MCP-facing text
  (`internal/mcp/whiteboard_panels.go`), and code comments that call Now the
  HTML channel.
- **No migration flag** (user, "(56) ok"): an existing HTML fragment is read
  as markdown with inline HTML, since block HTML passes through. The playbook
  gains one caveat line: inside HTML, an indented line after a blank line
  becomes a code block.
- **Lead replies render inline HTML** (user, "(59) ok"): in the session page
  conversation, finished lead replies render markdown with inline HTML; user
  messages, mail bodies, and turn lines keep raw HTML shown as text. Rejected:
  HTML in every conversation entry - mail bodies come from peer sessions, a
  different trust boundary.
- **Finished replies only** (user, "(60) ok"): a streaming reply keeps raw HTML
  as text and switches to HTML once finished, rendered once through the entry
  cache, the same rule as mermaid in 261107-feat-ws-dashboard-link-actions.
  Rejected: HTML while streaming - half-closed tags shake the layout every
  tick.
- **Balanced on the server** (user, "(61) ok"): a reply's HTML is parsed and
  re-serialized on the server (e.g. `golang.org/x/net/html`, a new module
  dependency) so a stray closing tag stays inside the bubble and cannot break
  the page structure.
- **No scripts in replies** (user, "(62) a"): reply HTML allows HTML, CSS, and
  inline SVG; `<script>` elements and `on*` event attributes are removed in
  the same server pass. Rejected: scripts as in Now - up to 2000 entries
  reload on scroll-up so scripts would re-run, and a reply quoting a file's
  HTML without a fence would execute it; dynamic drawings belong in Now.
- **One ticket** (user, "(63) ok"): both changes switch the same renderer
  option, so they share this ticket.
- **Kept rules for Now**: a full document is still refused; inline SVG and canvas
  drawn by an inline script still work; no external `src`, `href`, or fetch.

## Constraints

- This changes the observable `now` semantics of a shipped protocol: follow
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`,
  and `ai-docs/manuals/shipped-surface-boundary.md`.
- Both the disk board and the daemon-served page render Now the same way.

## Phases

### Phase 1: Markdown Now slot, schema text, and lead-reply HTML

Render Now through the markdown pipeline with raw HTML allowed for that slot
(the only board field), update every schema text listed above, and render
finished lead replies with sanitized inline HTML.

Verification: conversation tests that a finished lead reply renders inline
HTML and inline SVG, a streaming reply shows HTML as text, user and mail
entries still escape HTML, a stray closing tag stays inside the bubble, and
`<script>` and `on*` attributes are removed. Render tests that Now markdown renders (headings, lists,
mermaid), inline HTML and inline SVG pass through, a `<script>` canvas
fragment still runs as before, other fields still escape HTML, a full
document is still refused, and `now_path` reads a `.md` file; the
playbook-drift tests across ws, wsflow, and Pi stay green.
