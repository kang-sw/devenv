---
title: Shared XML command syntax - one parser, one manual section, a trimmed whiteboard section, inline tags in item fields
related:
  261108-feat-ws-tracker-caller: blocks its Phase 2 (tracker.write uses the shared parser and the shared syntax section)
---

# Shared XML command syntax - one parser, one manual section, a trimmed whiteboard section, inline tags in item fields

## Background

The tracker's `tracker.write` takes its commands in the whiteboard's flat XML
syntax (`261108-feat-ws-tracker-caller` Decision 10, board (413) b). The
syntax rules today live only inside the whiteboard playbook
(`agents-plugin/rsrc/whiteboard/whiteboard.md`, 208 lines, mirrored in
`agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`), spliced into the
workflow manual when the whiteboard knob is on
(`internal/mcp/workflow_manual.go:342-352`, `applyWhiteboardVariants`), and
the tokenizer lives inside `internal/wswhiteboard/commands.go`. Copying the
rules into a tracker section would load them twice for a lead using both.
User on (413): "지금 화이트보드 xml쪽 매뉴얼도 상당히 길긴 해서요, 이것까지
중복해서 들어가면 꽤 비싸질 거라 화이트보드만 쓸 때 / 트래커만 쓸 때 / 둘 다
쓸 때 등 (아마도 한번만 활성화하는 공통화 같은 방식으로) 매뉴얼 컨텐츠의
최적화가 필요할 걸로 봅니다. 겸사겸사 문구 자체도 필요한 내용만 담도록
하고요."

Dogfood 2026-11-08 (user screenshot of board items (403) and (406)): item
titles the lead wrote with `<code>...</code>` render the tags literally, while
the same items' rationale lines written with backticks render as code; agents
mix XML commands, markdown, and inline HTML and keep making this slip
(playbook line 105: "Raw HTML shows as text in every item field"). User:
"raw html도 해석할 수 있게 해야 할까요 .. ? 에이전트가 자주 실수하는 부분인
거 같아요. xml이랑 마크다운이 섞여버려서."

## Decisions

All confirmed on the session board, 2026-11-08.

1. **One shared syntax section** ((414)): the command syntax (body rules,
   attribute quoting, CDATA, unknown-tag and stray-text refusals, the refusal
   format) moves into one workflow-manual section, rendered once when the
   whiteboard or the tracker is enabled. Each feature section keeps only its
   command table and its own rules. Rejected: a copy per feature section -
   loaded twice when both are on.
2. **Shared parser** ((414)): the tokenizer and the batch-validation core
   move out of `wswhiteboard` into a shared package the whiteboard and the
   tracker both use, so the document and the code share one boundary. Each
   caller supplies its tag prefix (`wb:` for the whiteboard) and its command
   table.
3. **Wording trim** ((413) note, (415)): the whiteboard section is rewritten
   to carry only what a lead needs to act; no behavior change from the trim
   itself.
4. **Separate ticket** ((415)): this work ships on its own, ahead of the
   tracker; it benefits whiteboard-only users. Rejected: a phase inside the
   caller ticket.
5. **Inline tags in item fields** ((416) a): item fields (`wb:title`,
   `wb:option`, `wb:why`, `wb:recommend`) render a small safe allowlist of
   inline tags - `code`, `b` / `strong`, `i` / `em`, `kbd`, `br` - with no
   attributes; any other tag keeps showing as text. Topic bodies are
   unchanged. The playbook's "Raw HTML shows as text in every item field"
   line goes. Rejected: refusing such tags at update time with a fix hint
   (fixes the habit but costs a retry round trip each time); leaving it with
   a stronger manual line.

## Constraints

- Manuals that bind edits here: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md` (the playbook and manual text),
  `ai-docs/manuals/wsflow-mirroring.md` (the wsflow copy),
  `ai-docs/manuals/ws-mcp.md` (`internal/mcp/workflow_manual.go`).
- The whiteboard's accepted command language does not change: every batch
  valid today stays valid with the same result, and every refusal keeps its
  position, tag, and line. Only Decision 5 changes rendering.
- The allowlisted tags pass through the existing sanitizer path
  (`internal/wswhiteboard/sanitize.go`); no attribute survives, so no
  `href`, `src`, `style`, or event handler reaches the page.
- Until the tracker exists, "the tracker is enabled" is simply false; leave
  the condition as one place the caller ticket extends with its enable key.

## Phases

### Phase 1: Shared parser, shared section, trimmed whiteboard section, inline tags

- Extract the parser (Decision 2) with no change to whiteboard behavior; the
  existing `commands_test.go` cases pass unchanged against it.
- Split the playbook into the shared syntax section and the whiteboard
  section (Decision 1), and trim the whiteboard section's wording (Decision
  3); render the shared section once through `workflow_manual.go` when either
  feature is on. Mirror into wsflow.
- Item-field inline allowlist (Decision 5) and the matching playbook line.
- Verification: `go test` for `wswhiteboard`, the new parser package,
  `internal/mcp` (manual render with the whiteboard on and off; the shared
  section appears exactly once), and the wsflow mirror tests; a whiteboard
  render test showing `<code>x</code>` in a title as code and
  `<a href=...>` as text; a before/after line and byte count of the rendered
  manual with the whiteboard on, reported in the Result.
