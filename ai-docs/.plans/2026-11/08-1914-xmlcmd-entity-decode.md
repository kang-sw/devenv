# Plan: decode XML entities in command item fields and attributes

Branch `delegate/xmlcmd-entity-decode` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`), based on develop.
Commit here; do not merge or push. Do not run git stash.

## Why

The command syntax (`whiteboard.update`, `tracker.update`, shared parser
`agents-plugin-tool/internal/wsxmlcmd/`) tells agents to write `<`, `&` as is
and never escape. Agents escape anyway: the lead wrote
`<code>&lt;key&gt;</code>` in a `wb:option`, and the page showed the literal
`&lt;key&gt;`. The user chose (board (486) a) to make the parser tolerant
rather than add a "do not escape" warning to the playbook, which tends to
backfire.

## Decision (settled, (486) a)

- Decode the five predefined XML entities (`&lt; &gt; &amp; &quot; &apos;`)
  and numeric character references (`&#60;`, `&#x3c;`, case-insensitive x)
  in **item fields and attribute values**. Any other `&` sequence stays as
  written (`a && b`, `&nbsp;`, `&foo;` unchanged). Invalid numeric refs
  (out of range, surrogate, zero) stay as written.
- CDATA content is never decoded (it is the escape hatch for literal text).
- **Not decoded** (kept verbatim):
  - topic bodies (`wb:topic` body): they carry real HTML/SVG, where an
    author's `&lt;` inside raw HTML is intentional;
  - `wb:edit` fields `wb:old` / `wb:new`: they are exact-match operands
    against a stored topic body and must stay verbatim;
  - `wb:answer` body: the user's verbatim words.
- Decoded for the whiteboard: `wb:add` and `wb:patch` fields (title,
  option, recommend, why, link), and every attribute value.
- `tracker.update` uses the same parser: apply the same rule. Decode its
  plain-text fields and attributes; keep verbatim any tracker field that is
  rendered as raw-HTML-capable markdown (like a topic body) or used as an
  exact-match operand. Read the tracker grammar
  (`internal/wstracker` / `internal/mcp` tracker tools) and the renderer to
  classify each field; state the classification in the commit's AI Context.

Mechanism (your call, keep it simple): an opt-in per grammar, e.g. a
per-Spec or per-field flag in `wsxmlcmd.Grammar` that marks which fields
decode, with attributes always decoded; or decoding at the caller. Prefer
the parser owning the decode function and the grammar declaring where it
applies, so both tools share one implementation. Decoding happens after
the body is read (so `</wb:title>` detection is unaffected) and after CDATA
is unwrapped only for non-CDATA segments: a field mixing text and CDATA
decodes only the text segments.

## Docs

`agents-plugin/rsrc/lead-workflow-manual/command-syntax.md` (and its
mirrors in `agents-plugin-wsflow/rsrc/` and `agents-plugin-pi/rsrc/`;
regenerate rsrc manifests per `ai-docs/manuals/wsflow-mirroring.md` and
`skill-authoring.md`): keep "write as is, never escaped" as the rule; add
one short sentence that an item field or attribute value that does escape
(`&lt;`, `&amp;`, numeric refs) is decoded, while a topic body, `wb:edit`
text, and an answer are kept exactly as written. The manual is shared by
both tools, so word it tool-neutrally (e.g. "fields a tool marks as plain
text") or per tool if the section already names tools. Read
`ai-docs/manuals/shipped-surface-boundary.md` and `skill-authoring.md`
first. If the whiteboard playbook (`agents-plugin/rsrc/whiteboard/`) or
tracker manual repeats the "never escaped" rule, keep them consistent.

Note: a parallel delegate is editing `agents-plugin/rsrc/whiteboard/
whiteboard.md` and `internal/wswhiteboard/` (topic header work). Avoid
touching those files unless needed; if needed, keep the change minimal.

## Tests

- `wsxmlcmd`: entity decode table (each entity, numeric decimal/hex,
  unknown entity kept, bare `&` kept, invalid numeric kept, CDATA not
  decoded, mixed text+CDATA field), attribute decode, and a non-decoding
  field kept verbatim.
- Whiteboard command parser: `wb:option` with `&lt;key&gt;` stores
  `<key>`; `wb:topic` body and `wb:edit` old/new keep `&lt;`; `wb:answer`
  body keeps it.
- Tracker: one test per class of field you decided.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./internal/wsxmlcmd/ ./internal/wswhiteboard/ ./internal/wstracker/ ./internal/mcp/ && go test -count=1 ./internal/wsxmlcmd/ ./internal/wswhiteboard/ ./internal/wstracker/ ./internal/wsrsrc/ && go test -count=1 -run 'Whiteboard|Tracker|Syntax|Manual' ./internal/mcp/`
plus any rsrc/manifest drift tests the mirroring manual names.

## Return

Commit hashes, changed files, the tracker field classification, test
output, anything left open.
