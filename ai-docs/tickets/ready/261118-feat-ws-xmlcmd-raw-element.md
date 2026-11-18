---
title: "명령 문자열을 표준 XML로: raw 요소와 본문 엔티티 디코딩 통일"
related:
  261117-refactor-playbook-compression: landing-order - its whiteboard diet documents standard XML and `wb:raw`; this parser change must land on track/playbook before that branch merges to develop
  261118-feat-ws-whiteboard-zip-handoff: sibling - same landing on track/playbook
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: f721956f2aa46e67
sage-review-completeness-reviewed: f721956f2aa46e67
---

# 명령 문자열을 표준 XML로: raw 요소와 본문 엔티티 디코딩 통일

## Background

The `commands` string of `whiteboard.update` (`wb:` prefix) and of the tracker
tool (`tk:` prefix) is parsed by the shared package
`agents-plugin-tool/internal/wsxmlcmd`. Today a body is raw text up to the
first exact closing tag of its element; `<![CDATA[...]]>` protects a body that
must contain its own closing tag; entity escapes are decoded only in attribute
values and in some fields (`DecodeBody` for `tk:`, `DecodeFields` for the
`wb:add`/`wb:patch` item fields), while `wb:panel`, `wb:log`, `wb:answer`,
`wb:old`/`wb:new`, and `wb:body` keep escapes literally.

Two problems, raised by the owner on 2026-11-18 while cutting the whiteboard
playbook (261117-refactor-playbook-compression):

- The per-field escape rules are a large part of the playbook text and a
  frequent source of agent mistakes. The owner wants one canonical grammar.
- CDATA cannot nest: a body that describes XML or CDATA (a mockup, a code
  sample, an example of these very commands) exhausts it at the first `]]>`.

Owner (96)=a (2026-11-18 14:12): command strings follow standard XML, with a
dedicated raw element for text that must go in unchanged. The owner-approved
prose is `agents-plugin/rsrc/lead-workflow-manual/command-syntax.md` as landed
by the whiteboard diet commit (owner check (110)=ok); this ticket makes the
parser match it.

Code anchors (verify; line numbers drift):

- `agents-plugin-tool/internal/wsxmlcmd/wsxmlcmd.go`: package doc comment
  :8-44; `readContent` :762-946 with the CDATA case near :825, the
  `wb:change`-style wrapper branch near :841, and the rejected-group branch
  :861-933; `joinBody` :955-977 (decodes only when `decode && !seg.cdata`);
  attribute decoding near :720; `group()` pre-command text check near :515;
  `skipUnknown` near :1069.
- `agents-plugin-tool/internal/wsxmlcmd/wsxmlcmd.go` near :796 and :807 (the
  literal-code lookahead and its CDATA check inside `readContent`; not
  `literal_code.go`, which has 460 lines), and
  `agents-plugin-tool/internal/wsxmlcmd/literal_code.go` :138, :197, :305,
  :327 (each scanner hides a body close behind CDATA, so raw needs the same
  treatment there) (wsxmlcmd.go#L793-L812, literal_code.go#L126-L148).
- `agents-plugin-tool/internal/wswhiteboard/commands.go` :12-43 (doc comment
  and the `DecodeFields` wiring); `agents-plugin-tool/internal/mcp/tracker_tools.go`
  :132-134 (`DecodeBody: true`).
- `agents-plugin-tool/internal/mcp/command_syntax.go:67` (`commandRefusal`
  points at the playbook and does not restate the rules).
- `command-syntax.md` has three byte-identical copies (`agents-plugin`,
  `agents-plugin-wsflow`, `agents-plugin-pi`), hash-pinned in their manifests,
  the workflow bundles, and the Pi `dist/index.js`.

## Decisions

- **Standard XML text.** In every text body and attribute value, `&lt;`,
  `&gt;`, `&amp;`, `&quot;`, `&apos;`, and numeric character references are
  decoded. (96)
- **One decoding rule for every body.** The decoded text is what is stored;
  for a Markdown field it is the Markdown source the page renders, so
  `&lt;b&gt;` stores `<b>` and renders bold. No field keeps escapes
  literally: the `DecodeBody` / `DecodeFields` distinction is removed and
  `wb:panel`, `wb:log`, `wb:answer`, `wb:old`, `wb:new`, and every other body
  decode the same way. `wb:old` therefore matches stored text after decoding.
  (96) This supersedes board (486) (d39b84d6, 2026-11-08: parser tolerance,
  with "never escaped" as the manual's rule and verbatim answer and operand
  bodies); the owner reversed it under (96)=a.
  Rejected: per-field escape rules (the current state); escaping only in
  some fields.
- **Raw element.** `<prefix:raw>...</prefix:raw>` (`wb:raw`, `tk:raw`) takes
  everything up to the first closing `</prefix:raw>` exactly as written:
  no entity decoding, CDATA markers kept as literal text, tags of any kind
  kept. (96)
  Rejected: CDATA as the only escape hatch (cannot nest); a long sentinel
  name such as `NOESCAPEXML` (the owner chose a short `raw` name under
  (96)=a); escaping everything with no raw form (heavy for HTML/SVG
  mockups).
- **Old-style input stays accepted.** A bare `<` or `&` that does not start
  a known tag, CDATA section, or entity is taken literally, and CDATA keeps
  unwrapping as today. Compatibility Tiers prefer T0-T2; refusing it would
  break every session still holding the previous playbook text. (111)
- **No closing raw tag inside raw.** A literal `</prefix:raw>` is written
  outside a raw element, escaped (`&lt;/wb:raw>`); a consequence of "up to
  the first closing tag". (112)
- **Raw is a text segment.** It may appear anywhere text content is allowed,
  any number of times, mixed with escaped text in one body. (113)
  Rejected: raw only as a whole body (the existing segment model already
  joins text and CDATA segments, so raw is one more segment kind).
- **Raw where text is refused.** Between commands, between a command's
  fields, or in a group outside a command, a raw element is refused. Leaving
  `raw` out of the command and field tables gives this for free: it is
  refused as an unknown command (wsxmlcmd.go#L398-L403) or unknown field
  (wsxmlcmd.go#L1189-L1195). (114) Rejected: accepting it there as
  whitespace.
- **Edge trimming.** The one-newline trim after the opening tag and before
  the closing tag applies to the element's whole content and never inside a
  raw segment; a raw segment at a body edge keeps its own newlines. (115)
  Rejected: trimming raw content too.
- **Inter-element whitespace.** Whitespace-only text between commands and
  between fields (indentation, newlines) is ignored, so the indented example
  in the whiteboard playbook's `## Commands` parses. (116) The current
  parser already does this (`skipSpace` at wsxmlcmd.go#L335, #L426, #L505,
  #L1158); Phase 1 adds a regression test only. Rejected: refusing it.
- **Compatibility, new-playbook caller on an old daemon: T1.** Stored data
  keeps its shape and old binaries read it unchanged (T0 for the store). An
  old daemon given `prefix:raw` inside a body usually reads it as literal
  text and stores the tags and their content as written (the `readContent`
  default branch writes `<` literally, wsxmlcmd.go#L860-L944); it refuses
  the batch instead when the raw content holds the enclosing body's closing
  tag or a `wb:change` opening, and it refuses `prefix:raw` as an unknown
  command or field where a command or field is expected
  (wsxmlcmd.go#L398-L403, wsxmlcmd.go#L1189-L1195). (117) T1 holds (review,
  auto-confirmed): the old daemon either refuses, changing nothing, or
  stores only what that caller sent, so the extra markers degrade that
  entry's display only; the window is a new-plugin session talking to an
  older daemon, which the new plugin's start normally replaces. Rejected:
  claiming T0; a version gate or minor bump (a T3 boundary for a
  display-only degradation). The opposite direction (an old-playbook caller
  on the new daemon) is (119).
- **Raw inside code spans and fences.** `<prefix:raw>` opens a raw segment
  wherever CDATA would open, including inside a Markdown code span or fence
  (as CDATA does today, wsxmlcmd.go:807, 0d43b330); text inside a CDATA
  section is never a raw tag. (review, auto-confirmed: one delimiter rule
  for the scanners and the differential/fuzz oracles.)
- **Raw tag forms.** The opening tag is exactly `<prefix:raw>` and the
  closing tag exactly `</prefix:raw>`; any other form (attributes,
  self-closing, inner whitespace) is not a raw tag and follows old-style
  handling. An unclosed raw element is refused with the same problem class
  and position reporting as an unclosed CDATA section. (review,
  auto-confirmed: mirrors the existing CDATA handling.)
- This ticket also changes the body entity semantics of 261107 /
  ab9ccd98 ("raw bodies with CDATA escape hatch"), under the same owner
  reversal (96)=a.
- **Compatibility, old-manual caller on the new daemon: accepted as T1.**
  The old manual said raw-HTML bodies, exact-match operands (`wb:old`), and
  verbatim answers are "kept exactly as written", so such a caller may send
  `&lt;` meaning the literal entity. The new daemon decodes it: that entry
  may render a tag where the caller meant text, and a `wb:old` copied from
  stored text holding `&lt;b&gt;` decodes to `<b>`, which usually fails to
  match (a refusal, no change) and, only if the panel holds exactly one real
  `<b>`, patches that spot instead. The window lasts until the session
  rereads the playbook (restart, compaction revival); an entry already
  stored that way stays as stored and is fixed by reposting it. (119)=a
  Rejected: retrying an unmatched `wb:old` undecoded (keeps the wrong-spot
  case, adds window-only code); keeping the per-field exemptions for one
  minor release (two rule sets in code and playbook for that period).
- **One ticket for `wb:` and `tk:`.** Both tools share the parser; the change
  is one parser segment kind plus the removal of two option flags. (118)
  Rejected: separate tickets per tool (the two tools would run different
  grammars between landings).

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Shipped surfaces: read `ai-docs/manuals/shipped-surface-boundary.md`
  (refusal strings and doc comments are shipped text).
- MCP tooling: read `ai-docs/manuals/ws-mcp.md`.
- Cross-version surface (MCP argument semantics): read
  `ai-docs/manuals/release-compatibility.md`; the tier is recorded above.
- rsrc and mirrors: read `ai-docs/manuals/skill-authoring.md` and
  `ai-docs/manuals/wsflow-mirroring.md` if any rsrc text changes. The
  command-syntax.md prose is already owner-approved; change it only where the
  implementation forces a correction, and report it.
- Go tests run only on the remote w5090 (`bash D:/sys_lock.sh remote w5090
  run ...`); never run `go test` or ws executables on the owner's Windows PC.
- No prose-verbatim tests: tests assert parser behavior, not playbook
  sentences.
- Regenerating hash-pinned rsrc follows the Session Procedure of
  261117-refactor-playbook-compression (regen chain, Pi rsrc hand-copy,
  bundle scripts).

## Prior Decisions

- d39b84d6 (2026-11-08, commit): "Board (486) a: parser tolerance chosen over a "do not escape" playbook warning, which tends to backfire. The manual keeps "never escaped" as the rule and adds one tool-neutral bullet on the decode." — bearing: contradiction-candidate
- 261115-feat-ws-whiteboard-panel-first-stream (2026-11-16, commit 0d43b330): "Treat literal code as delimiter-inert, not as a bypass of the existing body codec. CDATA still unwraps inside both spans and fences, with raw segment/entity behavior and source offsets preserved" — bearing: constrains
- 261115-feat-ws-whiteboard-panel-first-stream (2026-11-15, Result): "a matcher candidate containing its own body's exact raw closing tag outside CDATA is not a literal. Structural reading stands; a candidate does not itself add a problem or change fatal/command-local acceptance." — bearing: constrains
- 6a364ea1 (2026-11-16, commit): "The rejected-group fallback formerly used a raw first-close reader and bypassed nested-group detection. Its ordered-only structural scan now respects quotes/comments/CDATA and command bodies" — bearing: constrains
- cf13f132 (2026-11-17, commit): "Wrapper-tail walk now skips raw field bodies by their own close, refuses an orphan quoted-opener-then-fields pair command-locally instead of silently dropping it" — bearing: constrains
- d39b84d6 (2026-11-08, commit, second record): "readBody now keeps text and CDATA as segments. It strips the body's one newline at each end before decoding, so a decoded `&#10;` at either end is not taken for the newline after the opening tag." — bearing: supports
- 261108-feat-ws-manual-shared-command-syntax (2026-11-08, Decisions): "One shared syntax section ((414)): the command syntax (body rules, attribute quoting, CDATA, unknown-tag and stray-text refusals, the refusal format) moves into one workflow-manual section" — bearing: supports
- 261107-feat-ws-whiteboard-xml-commands-topic-content (2026-11-07, commit ab9ccd98): "Ticket decisions followed as written: wb: prefix, raw bodies with CDATA escape hatch, attributes for scalars, in-order atomic batch, ids= grouped answers, per-topic bodies in state, Now removed, no transition period." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wsxmlcmd/wsxmlcmd.go, wsxmlcmd/literal_code.go, wswhiteboard/commands.go, mcp/tracker_tools.go, mcp/command_syntax.go, command-syntax.md copies, about a dozen test files |
| scope.surface | cross-module | exported wsxmlcmd.Spec fields DecodeBody and DecodeFields are removed and used by wswhiteboard and mcp; MCP commands argument semantics change |
| scope.new_public_symbol | no | none required; raw is a segment kind keyed off Grammar.Prefix plus raw |
| scope.new_type_contract | yes | wsxmlcmd.Spec loses DecodeBody and DecodeFields |
| scope.test_surface | existing | wsxmlcmd/wsxmlcmd_test.go, wswhiteboard/commands_test.go, wswhiteboard/acceptance_test.go, mcp/tracker_tools_test.go and the literal tests assert the old exemptions |
| complexity.reuse_points | confirmed | bodySegment and joinBody segment model with the CDATA case in readContent, wsxmlcmd.go#L725-L977 |
| complexity.side_effect_risk | high | one parser serves wb and tk; four literal_code.go scanners and the rejected-group, wrapper and skipUnknown paths all treat CDATA as hiding a close, and stored text decoding changes |
| risk.correctness | high | raw must be threaded through every content scanner without shifting Source offsets, refusal positions or fatal and command-local behavior |
| risk.fit | moderate | follows the existing segment model, but the tokenizer has no grammar prefix today and the scanners take only a closing string |
| risk.test | high | many tests pin the old exemptions and the literal code differential and fuzz oracles must stay aligned |
| risk.security_or_contract | moderate | T1 contract change; an old daemon stores raw tags as literal text; stored Markdown now decodes entities, though render.go escapes raw HTML |

## Phases

### Phase 1: Raw element and uniform decoding

Scope:

- `wsxmlcmd`: add the raw segment beside CDATA in `readContent`, and handle
  it in every path that scans content: the rejected-group branch,
  `skipUnknown`, the wrapper branch, `literal_code.go`, and the `group()`
  pre-command text check. Keep `Source` offsets and refusal positions
  correct across raw segments.
- Remove the `DecodeBody` / `DecodeFields` options and their call sites;
  decode every non-raw, non-CDATA text segment.
- Refusals that tell the caller to use CDATA for a body close inside code
  (`crossingGuidance` near wsxmlcmd.go:616, and near :801) name
  `<prefix:raw>` instead, with the grammar's prefix plumbed to them; update
  any test that pins those strings. (review, auto-confirmed: raw is the
  documented answer now; CDATA guidance would contradict command-syntax.md.
  Rejected: keeping CDATA guidance.)
- Update every comment that states the old body rules: `wsxmlcmd.go`
  :22-38, :59, :78-83, :732-734, :823-824, :1074; `wswhiteboard/commands.go`
  :18-21; `mcp/tracker_tools.go` :128-131. The tracker playbook carries no
  body or escape rule text.
- Update the package doc comment (`wsxmlcmd.go`) and the `commands.go` doc
  comment to the new grammar.
- Update tests that assert the old exemptions (verify each):
  `wswhiteboard/commands_test.go` (:28, :297-299, :362-390, :396-434),
  `wswhiteboard/acceptance_test.go` (:148, :210-235, :455-458),
  `acceptance_folders_test.go:135`, `entries_test.go` (:38-40, :154),
  `history_capture_test.go` (:57-73), `mcp/tracker_tools_test.go`
  (:762-780), `mcp/whiteboard_tools_test.go:1271`, the `literal_*_test.go`
  files, `wsxmlcmd/wsxmlcmd_test.go` (:535 and :537 construct the removed
  `DecodeBody` / `DecodeFields` options), and
  `wswhiteboard/literal_differential_test.go:446` (reads
  `spec.DecodeFields`).

Acceptance (behavior tests in `wsxmlcmd` plus one end-to-end case each for
`wb:` and `tk:`):

- `<wb:panel name="m" recap="r"><wb:raw><div><b>a < b</b></div></wb:raw></wb:panel>`
  stores `<div><b>a < b</b></div>` exactly.
- A raw segment containing `<![CDATA[x]]>`, `]]>`, `</wb:panel>`, and
  `&lt;` stores all of them literally.
- `&lt;b&gt;x&lt;/b&gt;` in `wb:panel`, `wb:log`, `wb:answer`, `wb:old`,
  `wb:new`, item fields, and a `tk:` body stores `<b>x</b>`.
- `wb:old` written escaped matches stored text containing `<`.
- Mixed body: `a &lt; b <wb:raw><i>c</i></wb:raw> d` stores
  `a < b <i>c</i> d`.
- An unclosed `<wb:raw>` refuses the batch with its position and line.
- Old-style input still works unchanged: a body with a bare `<b>`, a bare
  `&`, and a CDATA section stores what it stores today.
- The indented example from the whiteboard playbook's `## Commands` parses
  and applies.

Verification: `go test ./...` in `agents-plugin-tool` on w5090; regenerate
rsrc only if a rsrc file changed.
