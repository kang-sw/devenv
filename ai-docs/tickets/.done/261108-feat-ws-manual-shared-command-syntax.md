---
title: Shared XML command syntax - one parser, one manual section, a trimmed whiteboard section, inline tags in item fields
related:
  261108-feat-ws-tracker-caller: blocks its Phase 2 (tracker.update uses the shared parser and the shared syntax section)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 60afcdb2bbf206ea
sage-review-completeness-reviewed: 60afcdb2bbf206ea
completed: 2026-11-08
---

# Shared XML command syntax - one parser, one manual section, a trimmed whiteboard section, inline tags in item fields

## Background

The tracker's `tracker.update` takes its commands in the whiteboard's flat XML
syntax (`261108-feat-ws-tracker-caller` Decision 10, board (413) b). The
syntax rules today live only inside the whiteboard playbook
(`agents-plugin/rsrc/whiteboard/whiteboard.md`, 208 lines, mirrored in
`agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`), spliced into the
workflow manual when the whiteboard knob is on
(`applyWhiteboardVariants` in `internal/mcp/whiteboard_text.go#L73-L100`, called at `internal/mcp/workflow_manual.go#L352`; the section text comes from `whiteboardManualSection`, `whiteboard_text.go#L104-L127`), and
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
   itself. The whiteboard-on rendered manual must come out smaller than
   before the split, and every rule a test or refusal pins stays.
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
6. **Inline tag mechanism** ((419)): a dedicated goldmark inline parser
   turns the allowlisted tags into the equivalent markdown nodes (code span,
   strong, emphasis, line break; `kbd` as its own node rendered with no
   attribute); no raw-HTML parser is registered and `html.WithUnsafe` stays
   off, so no typed HTML reaches the page and the tags render identically to
   their markdown forms. This supersedes 261106-feat-whiteboard-rich-items'
   "typed tags are escaped text" for the allowlist only; every other tag
   stays escaped text. Rejected: enabling goldmark's raw-HTML parser plus a
   sanitizer pass (`sanitizeReply` keeps `class`, `title`, and `style`, and
   it would widen the escaping boundary to every tag).
7. **Mirror scope** ((420)): a playbook change propagates to the wsflow
   copy, the byte-identical Pi copy
   `agents-plugin-pi/rsrc/whiteboard/whiteboard.md` (pinned by
   `TestPiMirrorUpToDate`), and the `agents-plugin/rsrc/manifest.json` hash,
   as in e6b5ba64.
8. **Shared section source and refusals** (design review, auto-confirmed):
   the shared syntax text lives in its own rsrc file, not in
   `whiteboard.md`. The workflow manual renders it once (Decision 1); the
   `whiteboard.update` refusal (`whiteboardRefusal`,
   `internal/mcp/whiteboard_tools.go#L294-L310`) appends the shared section
   followed by the whiteboard's `## Commands` section, so one retry can still
   fix the call; `playbook.read` of the whiteboard playbook also appends the
   shared section. The worker names the file; it falls under Decision 7's
   mirror scope (`TestPiMirrorUpToDate` compares the whole rsrc tree, so the
   Pi copy, the wsflow regen, and the manifest hash all include it), and it
   must not surface as a standalone readable playbook. The refusal composition is one helper the tracker's
   refusal reuses with its own command section. Rejected: keeping the syntax
   under a heading in `whiteboard.md` (the tracker would quote the whiteboard
   playbook); letting the refusal quote only the command table (the retry
   loses the quoting, CDATA, and stray-text rules).
9. **Inline tag semantics** (design and completeness review,
   auto-confirmed): `<code>` content is literal, like a backtick span; `b`,
   `strong`, `i`, and `em` content is still parsed as inline markdown, so
   `<b>x *y*</b>` nests emphasis inside strong; tag names match
   case-insensitively; `<br>` and `<br/>` are both a line break; a tag
   carrying any attribute, an unclosed tag, or a mismatched closing tag stays
   escaped text.

## Constraints

- Manuals that bind edits here: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md` (the playbook and manual text),
  `ai-docs/manuals/wsflow-mirroring.md` (the wsflow copy),
  `ai-docs/manuals/ws-mcp.md` (`internal/mcp/workflow_manual.go`).
- The whiteboard's accepted command language does not change: every batch
  valid today stays valid with the same result, and every refusal keeps its
  position, tag, and line. Only Decision 5 changes rendering.
- The allowlisted tags must not carry any attribute: no `href`, `src`,
  `style`, or event handler reaches the page. (Today item fields render
  through the inline and block goldmark pipelines, which register no HTML
  parser, so a typed tag is escaped text - `internal/wswhiteboard/markdown.go#L28-L36`,
  `markdown.go#L185-L196`. `sanitize.go` is the finished-reply sanitizer
  (`sanitizeReply`) over the raw-HTML pipeline, not an item-field path, and
  its `replyGlobal` set keeps `class`, `title`, and `style`
  (`sanitize.go#L92`), so it cannot be reused unchanged for "no attribute
  survives".)
- Until the tracker exists, "the tracker is enabled" is simply false; leave
  the condition as one place the caller ticket extends with its enable key.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/rsrc/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- b3873db4 (2026-11-08, commit): "(413) b chose whiteboard-style XML for tracker.write; the user's note on manual cost led to (414) one shared syntax section rendered once and (415) a separate ticket that blocks the core's Phase 2." — bearing: supports
- 261106-feat-whiteboard-rich-items (2026-11-06, commit): "Escaping boundary: neither goldmark pipeline registers the HTML block or raw HTML parser, so typed tags are escaped text ... html.WithUnsafe stays off as a backstop. Rejected: custom escaping renderers for RawHTML/HTMLBlock nodes" — bearing: contradiction-candidate
- 261107-feat-ws-whiteboard-now-markdown (2026-11-07, commit): "Markdown-resolved links and images in a reply are recorded in a per-render table and marked with data-wbref=<index>; the sanitizer takes the URL from the table, never the markup." — bearing: constrains
- 261107-feat-ws-whiteboard-xml-commands-topic-content (2026-11-07, commit): "A body loses exactly one newline after its opening tag and one before its closing tag, uniformly for every body ...; nothing else is trimmed, so wb:edit matches text exactly as written on its own lines." — bearing: constrains
- f98d8e83 (2026-11-04, commit): "The inner payload schema has one source: refusals quote the rendered whiteboard playbook's ## Payload section instead of a Go copy, so schema text cannot drift from the playbook." — bearing: constrains (the anchor is now `## Commands` / `## Query`; Decision 8 extends it to the shared section)
- 261105-feat-whiteboard-knob-harness-and-session-scope (2026-11-05, commit): "applyWhiteboardVariants moved out of printPlaybook into its two callers (the playbook.read handler and handleWorkflowManual)." — bearing: constrains
- 261105-feat-whiteboard-explicit-item-ref-syntax (2026-11-05, ticket Result): "The rsrc renderer replaces only {{.Name}} placeholders (wsrsrc/loader.go substituteVars), so the playbook example needs no escaping" and MCP whiteboard tests pin the refusal schema and the knob-on manual carrying the example intact — bearing: constrains
- e6b5ba64 (2026-11-07, commit): "Playbook edited once in agents-plugin/rsrc and propagated: manifest regen, wsflow rsrc regen, and a byte-identical hand copy for agents-plugin-pi (TestPiMirrorUpToDate pins it)." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/whiteboard/whiteboard.md, wsflow mirror, internal/wswhiteboard/commands.go, new parser package, internal/mcp/workflow_manual.go, internal/mcp/whiteboard_text.go, internal/wswhiteboard/ item-field rendering |
| scope.surface | cross-module | wswhiteboard, a new shared parser package, internal/mcp, rsrc playbooks across ws and wsflow |
| scope.new_public_symbol | yes | the new shared parser package's exported entry points (tokenizer, batch validation, caller-supplied tag prefix and command table) |
| scope.new_type_contract | yes | caller-supplied tag prefix and command table in the shared parser (commands.go tagSpec and parseCommands today are unexported in wswhiteboard) |
| scope.test_surface | new-files | existing wswhiteboard/commands_test.go, markdown_test.go, internal/mcp manual tests, wsflow tests; the new parser package needs its own test file |
| complexity.reuse_points | confirmed | tokenizer in internal/wswhiteboard/commands.go, whiteboardManualSection in internal/mcp/whiteboard_text.go, inline/block pipelines in internal/wswhiteboard/markdown.go read |
| complexity.side_effect_risk | moderate | the parser gates every whiteboard.update batch and the manual render reaches every whiteboard-on lead; the Pi copy and manifest hash also move with the playbook (not named in the ticket) |
| risk.correctness | moderate | every valid batch must keep its result and every refusal its position, tag, and line; pinned only by the existing commands_test.go cases |
| risk.fit | moderate | the ticket names the wsflow mirror but not the byte-identical agents-plugin-pi copy, the rsrc manifest regen, or the refusal path that quotes the playbook's Payload section |
| risk.test | moderate | the render test for allowlisted tags is new and the shared-section-appears-once check depends on a playbook split not yet designed |
| risk.security_or_contract | high | Decision 5 opens item fields to raw tags where today the escaping boundary is the whole defense, and sanitize.go is a reply sanitizer that keeps class, title, and style |

## Phases

### Phase 1: Shared parser, shared section, trimmed whiteboard section, inline tags

- Extract the parser (Decision 2) with no change to whiteboard behavior; the
  existing `commands_test.go` cases pass unchanged against it.
- Split the playbook into the shared syntax section and the whiteboard
  section (Decision 1), and trim the whiteboard section's wording (Decision
  3); render the shared section once through `workflow_manual.go` when either
  feature is on. Mirror per Decision 7 (wsflow, Pi copy, manifest hash).
- Item-field inline allowlist (Decisions 5 and 6) and the matching playbook line.
- Verification: `go test` for `wswhiteboard`, the new parser package,
  `internal/mcp` (manual render with the whiteboard on and off; the shared
  section appears exactly once), and the wsflow mirror tests; a whiteboard
  render test showing `<code>x</code>` in a title as code and
  `<a href=...>` as text, plus the Decision 9 cases (nested `<b>x *y*</b>`,
  literal markdown inside `<code>`, `<BR/>`, `<code class=x>` and an
  unclosed `<b>` as text); a refusal test that the `whiteboard.update`
  refusal carries both the shared syntax rules and `## Commands`; a
  before/after line and byte count of the rendered manual with the
  whiteboard on (smaller after), reported in the Result.

### Result (03b13155e) - 2026-11-08

Landed in c613625eb, c0f2e43be, 89610851b, 03b13155e.

- **Shared parser (Decision 2).** The new package `internal/wsxmlcmd` holds the
  tokenizer, `Grammar{Prefix, Commands}`, `Command`/`Field` accessors, and the
  shared value checks (`NonEmpty`, `RequiredAttr`, `EnumAttr`, `IntAttr`,
  `BoolAttr`). `wswhiteboard` now declares only its `wb:` grammar table. Logic
  and messages are unchanged; `commands_test.go` changed accessor names only.
- **Shared section (Decisions 1, 8).** The section lives in
  `rsrc/lead-workflow-manual/command-syntax.md`, a part file read through the
  new `wsrsrc.LoadPart` (hash-verified, bare stem only). It does not load as a
  playbook. `mcp/command_syntax.go` provides:
  - `commandSyntaxEnabled`, the single gate where the tracker will add its
    knob;
  - the manual section, rendered once as `### Command syntax` ahead of
    `### Whiteboard`;
  - `commandRefusal`, which writes problems, then the shared section, then
    `## Commands`, and which the next command tool can reuse.

  `playbook.read("whiteboard")` appends the section.
- **Trim (Decision 3).** No pinned rule was dropped. Two mandate bullets were
  removed because the playbook already states them ("each turn", "answers by
  ID"), along with a duplicated "progress goes to chat" clause, the `wb:edit`
  example, and the tool list on the re-render bullet. Rendered manual with the
  whiteboard on:
  - `playbook.read`: 427 lines / 21790 bytes before, 417 / 21132 after.
  - `workflow_manual`: 455 / 23192 before, 445 / 22528 after.
- **Inline tags (Decisions 5, 6, 9).** `wswhiteboard/inline_tags.go` adds a
  goldmark inline parser plus an AST transformer. No raw HTML parser was added
  and `WithUnsafe` stays off. The item-field markdown bullet in the playbook
  names the allowlist.
- **Mirrors (Decision 7).** The wsflow mirror, the Pi copy, and the manifests
  are regenerated.
- **Verification.** These pass: `go test ./internal/mcp ./internal/wsrsrc/...
  ./internal/wswhiteboard ./internal/wsxmlcmd ./internal/wsdashboard
  -count=1`, the wsflow suite (`uvx pytest tests`, 20 passed), gofmt, and
  `go vet`. New tests cover:
  - the shared section appearing exactly once and in order with the knob on,
    and absent with it off;
  - the refusal's composition (shared section before `## Commands`; none on
    `whiteboard.query`);
  - the `playbook.read` append, and the part not loading as a playbook;
  - `LoadPart`;
  - the allowlist in every layout, including the Decision 9 cases.
- **Decisions taken in implementation:**
  - `<kbd>` content is parsed as inline markdown, the same as `<b>`/`<i>`.
  - `RenderText` (dashboard streaming reply, `.md` file view) shares the block
    pipeline but keeps escaping every tag, through `pageData.noInlineTags`,
    because the allowlist is an item-field rule.
  - The part file sits under `lead-workflow-manual/`. A root flat file would
    load through the flat fallback, and a heading inside `whiteboard.md` would
    make the tracker quote the whiteboard.
- **Review.** The lite review came back clean with one minor: the trim had
  dropped "Workers and subagents do not use it." It is restored in 03b13155e.
- **Note.** Commit c613625eb lacks the session attribution trailers. It was
  left unamended because history is never rewritten on this branch.
