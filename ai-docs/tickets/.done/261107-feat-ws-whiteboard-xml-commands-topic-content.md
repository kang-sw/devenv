---
title: "Whiteboard: XML command string input, per-topic content, Now slot removed"
related:
  261107-epic-ws-dashboard-messenger-round2: the dashboard the board renders into; the user raised this while using it
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: a83eb620b21bc79b
sage-review-completeness-reviewed: a83eb620b21bc79b
completed: 2026-11-07
---

# Whiteboard: XML command string input, per-topic content, Now slot removed

## Background

`whiteboard.update(session_key, payload)` takes `payload` as a JSON object.
On 2026-11-07 the lead measured that it writes Korean text inside an
object-typed argument as `\uXXXX` escapes, even when it tries not to, while a
top-level string argument (Bash `command`, Write `content`) carries Korean
raw. The cost is in the model's output tokens, not in the MCP transport. In
this session the board payloads were about 86k characters as raw Korean but
231k as emitted. The same content as a flat XML command list measured 422
characters, against 587 for raw JSON and 1177 for escaped JSON. The whiteboard
is the only ws tool that carries large human-language text (user, (218)).

The user also found the Now slot awkward on the dashboard: the lead's chat
replies already render as markdown with mermaid diagrams there, so moving
between chat and the Now slot added nothing. Topics have no content of their
own today (`topic` is only a string on items), so per-topic context had no
home except the Now slot.

## Decisions

- **Input is one string of flat XML commands** (user, chat (195), "XML로 아예
  바꿔버리는 방향으로 가볼까요"): `whiteboard.update(session_key, commands:
  string)` replaces the `payload` object. `whiteboard.query` keeps its
  current short filter payload unchanged (user, chat (218) a: the whiteboard
  is the only tool with Korean bodies). Rejected: splitting into one tool per
  operation with flat string arguments, which loses the atomic multi-command
  batch every turn uses; per-topic content files with paths returned by the
  update (a 2-turn flow), which adds temp files and a turn.
- **Grammar** (chat (196)-(200), acknowledged "따로 이견 없습니다"):
  - Every command and field tag carries the literal prefix `wb:`, with no
    `xmlns` declaration (200). Rejected: unprefixed names, because `title`,
    `option`, and `link` collide with HTML that bodies may contain, and a
    body that quotes the format would end early.
  - A tag body is raw text up to the first exact closing tag of that element,
    such as `</wb:why>`. Entities are not decoded, so `<`, `&`, inline HTML,
    and mermaid are written as is; a CDATA section is accepted for a body
    that must contain its own closing tag literally (196). Rejected: strict
    XML, which would require escaping `<` and `&` and recreate the problem.
  - Short scalar values are attributes; human-readable text (titles,
    options, rationale, quotes, content) is a body (199). No root wrapper
    element. An unknown tag or attribute refuses the batch.
  - Commands apply in the order written, and the batch is atomic: any problem
    refuses the whole batch and every problem is listed (197). Rejected: the
    current fixed apply order (add, answer, patch, ...).
  - One `wb:answer` may record the same verbatim quote on several items with
    `ids="190 191 194"` (198).
- **Per-topic content** (board (190) ok, (191) ok, (194) ok): a topic becomes
  an entity that can carry one markdown body stored in the whiteboard state,
  not in files. `<wb:topic name="...">body</wb:topic>` replaces the body
  whole; `<wb:edit topic="...">` with `<wb:old>` and `<wb:new>` replaces one
  exact, unique match (with optional `replace_all="true"`), the find-replace
  contract of host file-edit tools. Insert-before/after is not a separate
  mode: the anchor goes inside `wb:old`. The body renders above that topic's
  items, collapses with `wb:close`, and a topic may hold a body with no
  items. Rejected: a separate `whiteboard.edit` tool (the first form of
  (194)), superseded by `wb:edit` inside the command string.
- **Now slot removed** (board (192) a): progress and explanations go to chat,
  which the dashboard renders as markdown and mermaid; lasting context goes to
  topic bodies. The `now_path` input, the `now_file:` result line, the Now
  slot rendering, and the `.now.md` orphan-scan pattern landed in f4b1d747c
  are removed with it (chat (216)). Rejected: hiding Now only on the
  dashboard, since the lead cannot tell which surface the user watches; a
  diagram-only Now, which overlaps topic bodies.
- **No transition period** (chat (217)): a call carrying the old `payload`
  argument is refused with a message pointing at the `commands` format.
  Rejected: accepting both forms, which doubles the parser and test surface
  for callers (leads) that reread the playbook every session.
- Topic bodies take the rendering the Now file had: block markdown with
  inline HTML written as is, `{{12}}` item references, local images, and
  mermaid (chat (196): bodies carry "inline HTML or mermaid").
- **Topic bodies render in the sandboxed frame the Now slot used** (lead,
  auto-confirmed from the fact populator's gap): each topic body renders
  through the `framedNow` mechanism (`wswhiteboard/render.go`,
  `assets/now-frame.js`), an opaque-origin frame that sends no cookie, one
  frame per body; the frame and the `publicAsset` mermaid route in
  `wsdashboard/access.go` stay, renamed as the implementer sees fit.
  Rejected: rendering bodies inline on the dashboard origin, where raw HTML
  and script would run beside the routes that accept messages and steer
  sessions; escaping HTML in bodies as option text does, which drops the
  inline-HTML capability the user acknowledged.

## Grammar sketch

Illustrative, agreed in chat; tag names are the contract, the content is not.

```xml
<wb:add id="195" class="policy" topic="화이트보드 재설계">
<wb:title>화이트보드 입력을 XML 커맨드 목록으로 바꾼다</wb:title>
<wb:option>문자열 인자 하나에 XML 커맨드를 나열한다</wb:option>
<wb:option>연산별 도구로 쪼갠다</wb:option>
<wb:recommend>a</wb:recommend>
<wb:why>본문에 <details>나 <option>이 있어도 안전하다.</wb:why>
<wb:link>261107-epic-ws-dashboard-messenger-round2</wb:link>
</wb:add>
<wb:answer ids="190 191 194" value="ok">(191), (194), (190) ok</wb:answer>
<wb:answer id="192" value="a">(192) a</wb:answer>
<wb:patch id="192" status="superseded"/>
<wb:topic name="화이트보드 재설계">
## 배경
본문은 닫는 태그까지 raw 텍스트다.
</wb:topic>
<wb:edit topic="화이트보드 재설계">
<wb:old>## 배경</wb:old>
<wb:new>## 배경과 동기</wb:new>
</wb:edit>
<wb:close topic="트래커 데이터 구조"/>
<wb:floor id="200"/>
```

- `wb:add` attributes `id?`, `class`, `topic`; children `wb:title`,
  `wb:option` (repeatable, lettered a, b, c by position as today),
  `wb:recommend`, `wb:why` (today's `rationale`), `wb:link` (repeatable).
- `wb:answer` attributes `id` or `ids`, `value` (today's `answer`),
  `assumed?`; the body is the required verbatim `answer_quote`.
- `wb:patch` attribute `id`, `status?`; the same children as `wb:add`, each
  replacing that field.
- `wb:close` and `wb:floor` replace `close_topics` and `id_floor`. The
  `embed_dir` setting keeps an equivalent empty-element command; naming it is
  the implementer's call.

## Constraints

- Every existing item rule carries over unchanged: classes, statuses,
  option re-lettering, answer and `assumed` semantics, item references, ID
  minting and adoption, the closed-topic reopen rule, the render and embed
  outputs, and query.
- The refusal message names each problem with its command position and tag
  and repeats the grammar, as the refusal repeats the schema today.
- Go's `encoding/xml` is strict and cannot read raw-`<` bodies; the parser is
  a small dedicated tokenizer.
- Stored session state that still holds Now text drops it; no migration of
  its content.
- Shipped text is downstream-first: the playbook and tool strings must not
  depend on this repository (`ai-docs/manuals/shipped-surface-boundary.md`).
- Manuals that apply: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md` (playbook edit),
  `ai-docs/manuals/wsflow-mirroring.md` (the playbook's wsflow and Pi
  copies), `ai-docs/manuals/ws-mcp.md` (tool schema change).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- e6b5ba64 (2026-11-07, commit): "User decision 2026-11-07: drop inline `now`, keep `now_path`. A payload carrying `now` is refused with its own message pointing at now_path and now_file" — bearing: contradiction-candidate
- f4b1d747 (2026-11-07, commit): "User asked to drop inline `now` because it caused most whiteboard payload bloat (Korean \u escapes, resending the whole Now to fix one reference); `now_path` is the only Now input now." — bearing: supports
- e6b5ba64 (2026-11-07, commit): "Playbook edited once in agents-plugin/rsrc and propagated: manifest regen, wsflow rsrc regen, and a byte-identical hand copy for agents-plugin-pi (TestPiMirrorUpToDate pins it)." — bearing: constrains
- 261107-feat-ws-whiteboard-now-markdown (2026-11-07, ticket Result): "Now's {{nn}} decision: resolve in markdown text (Now goes through the board's own renderer), stay verbatim inside HTML; pinned by TestNowHTMLCaveats." — bearing: constrains
- 06fbf431 (2026-11-07, commit): "Parsing with goldmark (already the renderer here) rather than stripping code spans/fences with regexes" for the Now full-document check — bearing: supports
- c6339b07 (2026-11-06, commit): "User feedback from whiteboard dogfood (plan 06-1107-whiteboard-now-placement): the Now slot jumping between above and below the topics was poor UX" — bearing: supports
- f98d8e83 (2026-11-04, commit): "Validation choices: answer_quote is required for assumed answers too; a policy answer must lead with an option letter; ... close_topics on an unknown topic is refused" — bearing: constrains
- 261107-feat-ws-dashboard-link-actions (2026-11-07, commit 073fd387): "The link menu keys on a renderer-emitted marker, not every a[href], so raw Now-slot anchors and disk-board file: links stay untouched" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wswhiteboard (store.go, view.go, render.go, assets/page.tmpl), mcp/whiteboard_tools.go, mcp/server.go, mcp/dashboard_page.go, wsdashboard, three whiteboard.md copies and manifests |
| scope.surface | public-interface | whiteboard.update tool argument changes from payload to commands; playbook and manifests ship downstream |
| scope.new_public_symbol | yes | commands argument, wb: command grammar, topic body in State; whiteboard.update loses payload and now_path |
| scope.new_type_contract | yes | wswhiteboard.Apply takes a command string; State gains topic content; Env.ReadFile and Now fields removed |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/store_test.go, render_test.go, view_test.go, regions_test.go; internal/mcp/whiteboard_tools_test.go; new tokenizer tests would extend these |
| complexity.reuse_points | unconfirmed | renderNow pipeline and applyAdd/applyAnswer/applyPatch item rules are reused but this run did not read their bodies; the tokenizer is new |
| complexity.side_effect_risk | moderate | removing Now touches render, template, dashboard wiring, orphan scan, and stored-state compatibility |
| risk.correctness | high | new hand-written tokenizer with raw bodies, CDATA, and atomic in-order apply replaces the whole input path |
| risk.fit | moderate | repo-internal parts follow existing patterns, but three-way playbook mirroring plus manifest hash regeneration and a Pi contract that could not be located |
| risk.test | moderate | broad existing tests assert payload and Now behavior and must be rewritten; mcp package takes about 5 minutes |
| risk.security_or_contract | high | breaking tool contract for every lead; raw HTML in topic bodies, contained by keeping the Now sandbox frame for bodies |

## Phases

### Phase 1: XML command input, topic content, Now removal

- `agents-plugin-tool/internal/wswhiteboard/`: the command tokenizer and
  apply path replacing the JSON payload walk in `store.go`; topic entities
  with content in `State`; the Now fields and `now_path` removed; rendering
  of topic bodies in `view.go`, `render.go`, and `assets/page.tmpl` in place
  of the Now slot.
- `agents-plugin-tool/internal/mcp/whiteboard_tools.go`: the refusal for the
  old `payload` argument. The central precheck (`precheckToolArguments`,
  `internal/mcp/tool_arguments.go:65`) refuses undeclared arguments before any
  handler runs, so register `"whiteboard.update": {"payload"}` in
  `handlerOwnedArguments` (`tool_arguments.go:25`), the mechanism for retired
  names whose handler answers with a migration hint, and test the refusal
  through `callTool`, not only at the handler. `whiteboardRefusal`
  (`whiteboard_tools.go:274-304`) repeats one shared `## Payload` playbook
  section for both tools; split it so the update refusal repeats the
  `commands` grammar and the query refusal its filter payload. Also removal of the `now_file:` line and of `.now.md`
  from `whiteboardFilePattern`. The advertised tool schema (`commands` string
  for `whiteboard.update`; `whiteboard.query` keeps `payload`) lives in
  `agents-plugin-tool/internal/mcp/server.go#L4842-L4872`, not in
  `whiteboard_tools.go`.
- The dashboard's board regions render topic bodies and no Now slot. The
  board regions are built by `agents-plugin-tool/internal/mcp/dashboard_page.go`
  through `wswhiteboard.RenderRegions`; `agents-plugin-tool/internal/wsdashboard/`
  has no Now rendering. The `NowImageSrc` wiring
  (`internal/mcp/dashboard_page.go#L59-L61`) with its
  `wsdashboard.ImageDataURL` helper (`image_route.go#L83`) and the
  `publicAsset` LAN-public mermaid route (`access.go#L125`) exist for the Now
  frame and stay, renamed, for topic-body frames: the cookie-less frame
  cannot load the cookie-gated image route, so local images in bodies need
  the data-URL path. The image route itself serves board item images
  generally.
- Pi: `agents-plugin-pi/src/bridge.ts:1044` forwards each tool's MCP
  `inputSchema` unchanged as `parameters`, so Pi needs only the playbook
  mirror below; no bridge change.
- `agents-plugin/rsrc/whiteboard/whiteboard.md` rewritten for the command
  grammar (a short table of commands plus one example; the playbook is read
  before first use, so length is cost), mirrored to
  `agents-plugin-wsflow/rsrc/` and `agents-plugin-pi/rsrc/`, manifests
  regenerated.
- Implementer's call, each pinned by a test and recorded in the Result (and
  in the playbook where a caller writes it):
  - whether a body change bumps the topic's recency order, how a topic with
    neither items nor body is pruned, and the `embed_dir` command name;
  - tokenizer edges: attribute value quoting and escaping (a topic name
    containing `"` or `>`), and whether a body keeps or trims its leading and
    trailing newlines, which `wb:edit` exact matching and `wb:title` values
    depend on;
  - how `wb:patch` clears a list field (`options`, `links`), which today's
    `patch` does with an empty list, within the tag names of the grammar;
  - topic-body rules: `topicKnown` (`store.go`) counts content-only topics so
    `wb:close` on them is not refused under the unknown-topic rule;
    whether `wb:topic`/`wb:edit` on a closed topic reopens it as `add` does,
    and what `wb:edit` on a topic with no body returns; whether a body shows
    in a Quiet topic block and in the embed layout; whether the Now-only
    `NowFresh` open/stale behavior and `fullDocument` refusal carry over to
    bodies.

Verification, from `agents-plugin-tool/`: `go build ./...`,
`go vet ./internal/... ./cmd/...`, `go test -count=1 ./internal/wswhiteboard/
./internal/wsrsrc/ ./internal/wsdashboard/ ./internal/mcp/` (the mcp package
takes about 5 minutes); `python3 -m unittest discover agents-plugin-wsflow/tests`
from the repo root; the Pi test suite if the bridge contract changed. Tests
cover at least: Korean bodies round-trip raw; a body containing `<details>`,
`<option>`, `&`, and a mermaid fence; CDATA holding a literal closing tag;
in-order application and whole-batch refusal with positioned problems;
`ids=` grouped answers; `wb:topic` replace and clear; `wb:edit` unique,
missing, ambiguous, and `replace_all` matches; a content-only topic; the old
`payload` argument refused. A headless Chromium check of a page with a topic
body (light and dark) if practical.

### Result (c87afb80a) - 2026-11-07

Landed in ab9ccd98c (spine), d192e81ff (wswhiteboard tests), d6526beea (mcp
and wsdashboard tests), 420c93d58 (knob-on text names the wb: commands), and
c87afb80a (review fix: explanations go to chat).

- `whiteboard.update(session_key, commands)`: one string of flat `wb:`
  commands read by a dedicated tokenizer (`wswhiteboard/commands.go`), applied
  in order on a working copy, atomic, every problem listed with command
  position, tag, and line. The old `payload` argument is refused through
  `handlerOwnedArguments` with a pointer at `commands`; each tool's refusal
  repeats its own playbook section (`## Commands`, `## Query`).
- Topic bodies live in `State.Topics` (`wb:topic` replaces, `wb:edit`
  find-replaces), render above the topic's items: inline on the static page,
  one sandboxed frame per body on the dashboard (the former Now frame,
  renamed `doc-frame`, with per-topic heights kept across swaps by
  `data-doc`). Now slot, `now_path`, `now_file:`, and the `.now.md` scan
  pattern removed; stored `now` fields drop on the next write.
- Playbook rewritten (command table plus one example) and mirrored to wsflow
  (regen) and Pi (hand copy); manifests regenerated. The knob-on workflow
  mandate now sends progress and explanations to chat.

Implementer's calls (each pinned by a test):
- A body loses one newline after its opening tag and one before its closing
  tag, uniformly; nothing else is trimmed. Attribute values are quoted with
  `"` or `'` and never decoded.
- `wb:patch clear="options|links"` empties list fields; an empty
  `wb:recommend`/`wb:why` clears those. The embed-folder command is
  `<wb:embed-dir path="..."/>`.
- A body change bumps topic recency and reopens a closed topic; the same
  body is no change; `wb:edit` on a topic without a body is refused;
  clearing the body of an item-less topic drops the topic and its closed
  mark. `wb:close` accepts body-only topics.
- `NowFresh` is not carried over: a body is open on the full page unless its
  topic is closed, and open on the embed only when changed since the
  previous embed. The full-document refusal carries over to bodies. A closed
  topic with a body is Quiet with the body as a closed fold.
- `whiteboard.query` keeps its payload; with `topic` it also prints that
  topic's stored body, so a lead can quote it in `wb:edit`.

Verification: `go build ./...`; `go vet ./internal/... ./cmd/...`;
`go test -count=1 ./internal/wswhiteboard/ ./internal/wsrsrc/
./internal/wsdashboard/ ./internal/mcp/` all ok (mcp 311s);
`python3 -m unittest discover agents-plugin-wsflow/tests` OK (20 tests); the
three `whiteboard.md` copies are byte-identical. A headless Chromium render
of a static page with two topic bodies (one body-only) looked right in light
and dark. The Pi bridge was not changed, so the Pi suite was not run.
Lite review: one Important (mandate contradiction) fixed in c87afb80a.
