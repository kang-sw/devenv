---
title: "Whiteboard: the Now slot takes markdown with inline HTML; lead replies render inline HTML"
related:
  261107-epic-ws-dashboard-messenger-round2: raised while dogfooding the round-2 session page; affects disk boards too, so not a child
  261107-feat-ws-dashboard-link-actions: same markdown renderer; served-page link targets apply to Now too
  261107-feat-ws-dashboard-conversation-history: done; its per-entry render cache renders a finished reply once
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: cebf58d92f987715
sage-review-completeness-reviewed: cebf58d92f987715
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
  validation messages (`store.go`), MCP-facing text (`internal/mcp/` holds no
  separate text that describes `now` as HTML: `whiteboard_panels.go` has only a
  `now_path` mention in a comment at line 144, and `whiteboard_tools.go#L271-L289`
  quotes the playbook's `## Payload` section in refusals, so the playbooks carry
  that text), and code comments that call Now the HTML channel
  (`wswhiteboard/markdown.go#L32`, `view.go#L27`, `render.go#L98-L99`,
  `store.go#L51-L52`).
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
- **Full script-vector strip list** (user, 2026-11-07, "(80) ok"): the reply
  sanitizer removes every script vector, not only `<script>` and `on*`:
  `javascript:`, `vbscript:`, and `data:` URLs in any URL attribute (entities
  decoded before the check, as 6881738e had to), `iframe`, `frame`,
  `object`, `embed`, `base`, `meta`, `form` and `formaction`, SVG `animate`
  and `set` that target `href`, and `<style>` elements; inline `style`
  attributes stay, which is how "CSS allowed" in (62) is read. Rationale: the
  pages send no CSP, so a missed vector runs in the daemon origin, which
  accepts message and steer posts; a `<style>` element restyles the whole page
  outside the bubble. Rejected: the literal "`<script>` + `on*`" list.
  Phase 1's adversarial tests cover each listed vector.
- **Allow-list sanitizer** (user, 2026-11-07, "(82) ok"): the reply sanitizer
  is an allow-list, not a deny list: only listed elements and attributes
  survive (formatting, lists, tables, `details`/`summary`, `img`, `a`, inline
  SVG shapes and text; `class`, filtered `style`, `title`, `alt`, sizing and
  table attributes, SVG presentation attributes - the implementer fixes the
  exact list within this class and tests it). Every URL-bearing attribute
  (`href`, `src`, `srcset`, `poster`, `xlink:href`, `background`, ...) goes
  through `resolveTarget` per the next decision; `<link>`, media elements
  (`video`, `audio`, `source`, `track`), and anything unlisted are removed;
  inside a kept inline `style`, `url(...)`, `@import`, and `expression(` are
  removed. The vectors in the previous decision are the minimum the
  allow-list must exclude, each with an adversarial test. Rejected:
  extending the deny list - each review round found vectors it missed
  (`<link>`, `style` `url()`, `srcset`).
- **Reply URLs follow the markdown contract** (user, "(81) a"): `href` and
  `src` in reply HTML go through `resolveTarget` like markdown links and
  images: network images refused, file paths mapped to the image route or the
  file view of 261107-feat-ws-dashboard-link-actions, http(s) links kept. The
  link-actions click menu therefore applies to these links. Rejected: leaving
  them verbatim - `<img src="https://...">` would fetch from every viewer,
  LAN phones included, and a relative `src` breaks against `/s/<rid>/`.
- **One ticket** (user, "(63) ok"): both changes switch the same renderer
  option, so they share this ticket.
- **Kept rules for Now**: a full document is still refused; inline SVG and canvas
  drawn by an inline script still work; no external `src`, `href`, or fetch.

## Constraints

- This changes the observable `now` semantics of a shipped protocol: follow
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`,
  and `ai-docs/manuals/shipped-surface-boundary.md`.
- Both the disk board and the daemon-served page render Now the same way.
- This ticket supersedes the earlier "Now is the only HTML channel / raw HTML
  stays off" decisions (261106-feat-whiteboard-rich-items, 6881738e) for the
  Now slot and finished lead replies only; every other board field and
  conversation entry kind keeps raw HTML off.
- Now keeps today's `NowHTML` path apart from the markdown step: rendered on
  each board render, not balanced, scripts kept. Test the board's hard-wrap
  setting on an existing multi-line fragment that starts with an inline tag
  (a `<br>` may be inserted), and decide and test whether `{{n}}` item
  references resolve inside Now; add a playbook caveat line for whichever
  holds.
- With the HTML block parser on, a line starting with a block tag (such as
  `<details>`) turns the following lines into raw HTML up to the next blank
  line (6881738e records this); expected-output tests cover it for replies
  and Now.
- Landing order with 261107-feat-ws-dashboard-link-actions is free, but that
  ticket adds a system-open route trusting `sameOrigin` plus a loopback
  viewer in the same page origin: whichever lands second keeps this
  sanitizer and those checks together, and the sanitizer tests include a
  vector aimed at system open. Both tickets add `RenderText`/`TextTargets`
  options and split the reply render into streaming and finished variants;
  the link-actions `.md` file view stays in escaped mode. Reply links the
  sanitizer resolves carry the same renderer marker link-actions' click
  handler keys on.
- A reply `href` or `src` that `resolveTarget` refuses: an anchor loses the
  attribute and keeps its text; an image is replaced by its path as text, as
  markdown does today. Tested both before and after link-actions lands.
  Where `resolveTarget` returns its third outcome (accepted, empty href,
  path text: a file target on a served page before link-actions), an anchor
  drops the `href` and keeps its text. `mailto:` links go through
  `resolveTarget` like any other href.
- `srcset` is resolved per candidate through `resolveTarget` (image), the
  refused candidates dropped and the attribute removed when none remains;
  tested.
- A kept inline `style` loses `position` (and with it fixed/sticky overlays)
  besides `url(`, `@import` and `expression(`, and the finished-reply bubble
  sets `contain: paint`, so a reply cannot lay a layer over the composer or
  the steer controls. One test covers a `position:fixed` reply.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/rsrc/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 261106-feat-whiteboard-rich-items (2026-11-06, Decisions): "No raw HTML in item markdown. goldmark's raw HTML stays off; the Now slot remains the only HTML channel. Rejected: allowing HTML in item fields" — bearing: constrains
- 6881738e (2026-11-06, commit): "Escaping boundary: neither goldmark pipeline registers the HTML block or raw HTML parser, so typed tags are escaped text; html.WithUnsafe stays off as a backstop." — bearing: constrains
- 261030-feat-session-whiteboard-mode (2026-10-30, Decisions): "Explanations and drawings go in the Now slot: the lead passes an HTML fragment through `now`, and the server wraps it." — bearing: constrains
- ec9130b0 (2026-11-04, commit): "now_path added because verbatim HTML inside MCP tool arguments is error-prone (user concern); read once and stored so the board never changes silently." — bearing: constrains
- 3d92b646 (2026-11-07, commit): "User answered board items (48)-(58) ... Now slot markdown with inline HTML for Now only (55a), now_path takes markdown and schema texts change together, no migration flag" — bearing: supports
- 8140d4cc (2026-11-07, commit): "User asked for Now-like HTML embedding in the conversation; board answers (59)-(63): lead replies only, finished replies only, server-side balancing, no scripts or on* attributes (62a)" — bearing: supports
- 261106-feat-ws-dashboard-browser-interaction (2026-11-07, Result): "The served-page image resolver (090f4d005): `View.ImageSrc` / `ViewInput.ImageSrc` in `internal/wswhiteboard`. `fileTarget` consults it for an image's file target" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wswhiteboard (markdown.go, markdown_text.go, view.go, render.go, store.go), wsdashboard conversation render, three whiteboard playbooks |
| scope.surface | cross-module | observable now/now_path semantics of a shipped protocol, wswhiteboard plus wsdashboard plus ws/wsflow/Pi playbooks |
| scope.new_public_symbol | unknown | ticket names no symbol; a renderer option for raw HTML and a reply-sanitizing function are implied but unnamed |
| scope.new_type_contract | unknown | ticket fixes no signature; RenderText (markdown_text.go#L22) may gain an option, undecided |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/render_test.go, markdown_test.go, internal/wsdashboard/conversation_markdown_test.go, internal/wsrsrc/pi_mirror_test.go, wsflow_mirror_test.go |
| complexity.reuse_points | confirmed | RenderText and newMarkdown pipeline (markdown.go, markdown_text.go), per-entry cache (wsdashboard/entry_cache.go), existing mermaid handling |
| complexity.side_effect_risk | high | enabling raw HTML changes the escaping boundary that the whole board and conversation rely on, and adds golang.org/x/net (absent from go.mod and go.sum) |
| risk.correctness | high | server-side HTML parse, balance, and script/on* stripping must hold for finished replies only, with streaming and cache keys staying consistent |
| risk.fit | moderate | follows the shared renderer, but options in one pipeline must not leak raw HTML to other fields or entry kinds |
| risk.test | moderate | existing render and conversation tests extend, but sanitization needs adversarial cases and Now script behavior needs a browser-free proxy |
| risk.security_or_contract | high | new HTML trust path from lead replies, shipped now/now_path semantics change mirrored in three playbooks |

## Phases

### Phase 1: Markdown Now slot, schema text, and lead-reply HTML

Render Now through the markdown pipeline with raw HTML allowed for that slot
(the only board field), update every schema text listed above, and render
finished lead replies with sanitized inline HTML.

Verification: conversation tests that a finished lead reply renders inline
HTML and inline SVG, a streaming reply shows HTML as text, user and mail
entries still escape HTML, a stray closing tag stays inside the bubble,
every script and fetch vector the sanitizer decisions name is removed (one
adversarial case each, including one aimed at the system-open route), and
reply `href`/`src` follow the markdown contract (network image refused, file
path mapped, http(s) link kept, refused target handled as above). Render tests that Now markdown renders (headings, lists,
mermaid), inline HTML and inline SVG pass through, a `<script>` canvas
fragment still runs as before, other fields still escape HTML, a full
document is still refused, and `now_path` reads a `.md` file; the
playbook-drift tests across ws, wsflow, and Pi stay green.
