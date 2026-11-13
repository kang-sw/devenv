---
kind: print
---

# Whiteboard

The session whiteboard is a top-level lead's board for its user. Each topic
holds current content, append-only panels, and a compact discussion log,
beside the items you ask (IDs, statuses, verbatim answers). Use current
content for the durable state of the work, panels for substantial discussion
or visuals, and an agent log line for the current-state summary. The latest
agent line supplies that summary; later server events do not replace it.
Embeds keep the items but show panel recaps and the latest agent line instead
of current content or accumulated payloads, with a link to the full board.
Workers and subagents do not use the board.

## Calls

- Collect a turn's board changes into one
  `{{.McpNamespace}}/whiteboard.update` call.
- Before adding a question, use `{{.McpNamespace}}/whiteboard.query` with
  `depth: "items"` and a `text` or `topic` filter, so existing ask IDs and
  statuses are available for reuse or patching. Default discovery is compact
  topic metadata; neither form implicitly retrieves bodies.

## Commands

Write authored board text in the conversation language, the `workflow.lang` value when set; IDs, class and status tags, `ok`/`no` answers, and option letters stay as written.

Every tag carries the prefix `wb:` and follows the command syntax rules.
`wb:topic` is the group: its `name` names the topic once for the
`wb:body`, `wb:edit`, `wb:panel`, `wb:log`, `wb:add`, and `wb:close` inside it. A
batch may hold several `wb:topic` groups.

| command | attributes | content |
|---|---|---|
| `wb:topic` | `name`, `label?` | group of `wb:body`, `wb:edit`, `wb:panel`, `wb:log`, `wb:add`, `wb:close`; none with `label` |
| `wb:body` | none | current content, replaced whole; empty clears it |
| `wb:edit` | `replace_all?` | fields `wb:old`, `wb:new`, editing current content only |
| `wb:panel` | `recap`, `id?` | optional payload; `recap` must be nonempty |
| `wb:log` | none | one nonempty agent-authored line of inline Markdown |
| `wb:add` | `id?`, `class` | fields `wb:title`, `wb:option`..., `wb:recommend?`, `wb:why?`, `wb:link`... |
| `wb:close` | `result` | none |
| `wb:answer` | `id` or `ids`, `value`, `assumed?` | the user's verbatim words |
| `wb:patch` | `id`, `status?`, `class?`, `clear?` | any `wb:add` field, each replacing that field |
| `wb:floor` | `id` | none |
| `wb:embed-dir` | `path` | none |

```xml
<wb:topic name="rollout" label="Rollout">
<wb:body>
## Context
Markdown up to the closing tag.
</wb:body>
<wb:panel recap="Parser-first tradeoff">Keep the review slice small; see {{12}}.</wb:panel>
<wb:log>In review: parser branch, waiting on CI</wb:log>
<wb:add class="policy">
<wb:title>Ship the <code>parser</code> before the renderer?</wb:title>
<wb:option>Parser first</wb:option>
<wb:option>Both at once</wb:option>
<wb:recommend>a: smaller review</wb:recommend>
<wb:why>Write `<details>` in backticks to show the tag; & and a < b need no escaping.</wb:why>
<wb:link>supersedes {{12}}</wb:link>
</wb:add>
</wb:topic>
<wb:answer ids="14 15" value="ok">14,15=ok</wb:answer>
<wb:patch id="16" status="withdrawn"/>
<wb:topic name="Old scope">
<wb:close result="Folded into Rollout."/>
</wb:topic>
```

- Topic stem: `<wb:topic name="ship-gate" label="Release gate">` keys the
  topic by the stem `ship-gate` (lowercase letters, digits, and hyphens, at
  most 40) and shows `label` as its name; later groups write
  `<wb:topic name="ship-gate">` alone. Give a topic a stem when you create
  it, so a long or changing title is never retyped. A group with `label`
  on a stemmed topic renames the label, and one whose stem is new but whose
  `label` is an existing stemless topic's name adopts that topic under the
  stem; either may be sent alone as `<wb:topic name="..." label="..."/>`.
  A topic without a stem is keyed by its name. A `name` resolves to a stem
  first, then a stemless topic's name, then a stemmed topic's label, so an
  old title still reaches an adopted topic. Stems and labels are one
  namespace: a stem or label another topic already uses is refused.
- `class`: `policy`, `technical`, `announced`, or `check`, the kinds of ask
  in the workflow manual's `### Asking the user`. A `check` is a gate, not a
  decision class: a request that the user look at, verify, or confirm a
  finished artifact, or an action whose choice is already settled ("OK this
  draft and I'll proceed", "verify on your machine"). A "should we do X?" or
  "which one?" ask, a yes/no one included, is a `policy` item with options,
  never a check. Write one verifiable thing per check, never several bundled.
  It renders as a checklist in the topic, and the user answers each
  one individually: `ok`, `no`, or a note. An item belongs to its group's
  topic, which groups items on the page; `wb:title` is one line.
- `id` on `wb:add`: omitted, the server mints one past the highest ID it has
  minted, adopted, or been floored to; a number adopts an ID from outside the
  board, such as a ticket queue's. An ID the store holds is refused.
- `wb:option`: one per option, lettered a, b, c by position; a `wb:patch`
  carrying any `wb:option` replaces and re-letters the whole list, so send
  `wb:recommend` with it. `wb:recommend`: an option letter, optionally
  followed by `: <why>`. On `wb:patch`, `clear="options"`, `clear="links"`,
  or `clear="options links"` empties those lists, and an empty
  `wb:recommend` or `wb:why` clears that field.
- Markdown: `wb:title` takes inline markdown (bold, italic, code, links)
  and the tags `<code>`, `<b>`, `<strong>`, `<i>`, `<em>`, `<kbd>`, and
  `<br>` written without attributes; any other tag shows as text.
  `wb:option`, `wb:why`, and `wb:recommend` take block markdown
  with line breaks kept: lists, code blocks, tables, images, and a
  `` ```mermaid `` fenced block drawn as a diagram, plus HTML such as `<img>`, `<table>`,
  `<details>`, and simple inline SVG, shown as static markup with script,
  style, frames, form controls, event attributes, and positioning removed;
  text such as `Vec<T>` reads as an unknown tag and disappears unless it is
  in backticks. A line that starts with a block tag follows the body's HTML
  rule below. A link or image path, in markdown or in `href` and `src`, is
  relative to the session root or absolute; images are local files only,
  links may also be http(s). An open technical or announced item shows its `wb:why` inline,
  one line per reason, with only the title's tags rendered; a check's
  `wb:why` shows the same way under its row.
- `wb:why`: a technical or announced item's citation or reasoning, a
  policy item's context, or a check's note on what to look at. `wb:link`: one per link, such as a ticket stem, a
  commit, or `supersedes {{12}}`.
- Authored references: `{{12}}` cites item `(12)` in item fields, current
  content, panels, log text, and the reading part of an answer `value`.
  `{{p3}}` cites the latest version of panel `p3`; `#p3` is its stable link
  target across re-posts. Known item references show `(12)` and link when
  their item is on the page; known panel references reveal their latest
  version, loading older history if needed. Unknown references stay
  literal, as do references inside code or raw HTML. A plain `(12)` is
  literal text. References are citations, not user-selected mentions or
  answers; a `wb:answer` body, the user's verbatim words, is never resolved.
- `wb:answer`: `value` is, for a policy item, its option letter, optionally
  followed by `, <text>`; for a technical, announced, or check item, `ok`,
  `no`, or short text. A later answer replaces an earlier one. The body, required, is
  the user's verbatim words for this item. `ids="14 15"` records the same
  value and words on each listed item. `assumed="true"` records your reading
  of a vague reply; the page shows the item as a settled row marked assumed,
  with a note pen the user can veto or comment through (an assumed check
  stays in its topic's check list). Record the user's
  later confirming or correcting words as another `wb:answer` on that ID
  without `assumed`.
- Status: an item starts `open`; `wb:answer` sets `answered`, or `assumed`
  with `assumed="true"`; `wb:patch status` sets `open`, `withdrawn`, or
  `superseded`. Answered, withdrawn, and superseded items are settled, and an
  assumed item shows among them: the page keeps the 10 most recently settled
  expanded and the rest behind their topic's count chip. Options and `class`
  change only on an item that is open once the patch's own `status` applies,
  since an answer is stored as an option letter under its class: to turn a
  settled item into a policy question under its existing ID, send
  `status="open"`, `class="policy"`, and the options in one `wb:patch`.
- Re-ask: a `wb:patch` that changes the title, `class`, or options of an
  item open after the patch, or sets a settled or assumed item back to
  `open`, asks it again. The dashboard then drops the sent mark of an answer
  the user sent for it before the patch, so the item shows as waiting on the
  user. An answer sent before the patch and not yet delivered still reaches
  you, and it refers to the title and options it was sent against. A patch
  that only edits `wb:why`, `wb:recommend`, or links, or resends the current
  title, class, or options, is not a re-ask; a title fix on an item the
  patch leaves settled stamps nothing.
- `wb:close` requires an existing topic and a `result`: one sentence saying
  how it ended. Closing withdraws every item still `open`; an `assumed`
  item stays assumed. Put a batch's answers before its `wb:close`, since
  an answer after the close finds the item withdrawn. Adding an item,
  changing nonempty current content, or appending a panel or agent line reopens a
  closed topic without restoring withdrawn items. Server events alone do
  not reopen it. The next close replaces the result, not the history.
- Current content: one body per topic, holding the durable current state,
  not the accumulating conversation. A topic can have content or entries
  without items. `wb:body` replaces content whole; empty clears it and the
  same content again is no change. `wb:edit` replaces one exact `wb:old`
  match with `wb:new` (empty deletes it), or every match with
  `replace_all="true"`; no match, or several without `replace_all`, is
  refused. To insert, quote the anchor in `wb:old` and the anchor plus the
  insertion in `wb:new`. Retrieve content with `body: true` to obtain an
  exact edit anchor; a topic-only discovery query does not print it.
- Current content and panel payloads take block Markdown with inline HTML,
  never a full HTML document: inline SVG, or canvas 2D drawn by an inline
  script, with no external `src`, `href`, or fetch. Served payloads run in
  isolated sandboxes. Put a standalone document in a
  `{{.McpNamespace}}/path.generate` file and link it from the board.
- HTML in a body: a line that starts with a block tag such as `<div>` or
  `<details>`, or holds a lone tag such as `<svg ...>`, starts raw HTML that
  runs to the next blank line, so put a blank line between HTML and the
  markdown after it, and keep blank lines out of HTML, since an indented
  line after one becomes a code block. A line that starts with an inline tag
  such as `<b>` is markdown text, whose line breaks render as `<br>`.
- Panels: `recap` is required and nonempty; a recap-only panel is a message.
  An optional payload carries a longer opinion, mockup, diagram, or image.
  Omit `id` to mint a board-wide `pN`; use an existing `id="pN"` in the
  same topic to append a complete new version, even when the text is the
  same. Each immutable version has its own durable `eN` entry locator;
  older versions remain in history while `pN` always targets the latest.
  There is no `panel` attribute on `wb:edit`: re-post the whole panel,
  including its recap and any payload, instead of editing it in place.
  Split content-heavy material into several (sub)panels rather than one
  huge panel; payloads are capped at 2 MiB of UTF-8 bytes, with an external
  file link as the alternative.
- Agent log: `wb:log` appends one line of inline Markdown, without images,
  HTML, or block structures. Append a current-state line when the work's
  state changes, so the latest agent line remains a useful summary; put
  substantial discussion in a panel instead of rewriting current content.
  The server records successful item/content/panel/close/reopen transitions
  as system events. Agent lines give a concise interpretation of current
  state; verbatim duplication of the server's event records is not required.
- Keep asks in items: "OK this draft and I'll proceed" is `wb:add` with
  `class="check"` (or `policy` for a choice), not a panel or log line.
  Options, recommendations, `wb:why`, links, and check answers remain item
  data, not automatically created panels. `wb:recap` is removed without an
  alias, and `wb:append` is refused; the panel's `recap` attribute is not
  the retired command.
- `wb:floor` raises the ID counter so the next minted ID is one past `id`; a
  floor at or below the counter is ignored. `wb:embed-dir`: see Host embed
  folder.

## Query

```text
whiteboard.query payload: {topic?, text?, closed?, entries?, since?, last?, kind?, panel?, entry?, id?, ids?, status?, depth?, fields?, body?}
```

`payload` is a JSON object. Choose discovery, explicit item/content access,
or direct retrieval; their selectors are not silently mixed.

### Discovery and entry filters

- Without explicit item/content selectors, default discovery is metadata:
  each topic's latest agent line, its last ten log entries, and panel recaps.
  The recap list uses the newest matching version of each `pN`; a text
  match may therefore name a historical `eN`, not the current version.
  A `topic` or `text` filter still does not return current content or panel
  payloads. System events cannot replace the latest-agent summary.
- `topic` takes a name or list, resolving stems and labels as `wb:topic`
  does. `closed: true` includes closed topics; topic/text-targeted discovery
  includes matching closed topics by default, while untargeted discovery
  leaves them out. An explicit `closed` overrides that default.
- `entries` selects `"log"`, `"panel"`, or `"all"`. `kind` selects
  `"agent"` or `"system"` log entries. `since` is an exclusive RFC 3339
  timestamp or durable `eN` boundary; equal timestamps remain distinguishable
  by entry ID. `last` is a positive integer selecting the newest matching
  tail. Filters apply before the limit; the selected window prints oldest
  to newest. With explicit `entries`, historical panel-version metadata is
  eligible for the same filtered tail; discovery never returns payloads.
- `text` is case-insensitive substring search across current content,
  panel recaps/payloads, log text, and item fields. Matching a payload does
  not print it: use the direct retrieval form to inspect exact bytes.

### Explicit items and current content

- Any explicit `id`, `ids`, `status`, `depth`, `fields`, or `body` selects
  the item/content interface. Do not combine these with `entries`,
  `since`, `last`, or `kind`; incompatible selectors are refused.
  `topic`, `text`, and `closed` can still narrow an item query.
- `id` is a positive numeric item ID; `ids` is a list. When both are
  present they union; other item filters AND together. `status` takes one
  or a list of `open`, `answered`, `assumed`, `withdrawn`, `superseded`.
- `depth`: `topics` lists topic metadata; `items` adds item lines;
  `summary` adds full open asks or decoded answered/assumed readings;
  `full` adds details. `fields` selects from `options`, `recommendation`,
  `rationale`, `answer`, `answered_at`, `answer_quote`, and `links`; it
  implies `full` when depth is omitted. Use `answer_quote` for the user's
  exact words, rather than reconstructing them from an interpreted value.
- `body: true` explicitly retrieves current content, verbatim for exact
  `wb:edit` anchors. `body: false` does not. Neither form returns a panel
  payload, and `topic` alone never implies body retrieval.

### Direct immutable-entry retrieval

- `panel: "p7"` retrieves that panel's latest full payload verbatim;
  `panel: "e47"` retrieves one exact immutable panel version's payload.
  A recap-only version has an empty payload, not a fallback to an older
  version that carried one.
- `entry: "e47"` retrieves one agent/system log entry, with topic, kind,
  timestamp, and verbatim text. A panel version is retrieved through
  `panel`, not `entry`.
- A direct result is one metadata header, a newline, then untouched stored
  text; no trailing newline is added.
- Either direct selector may carry an optional `topic` assertion naming
  its owner, but no other selectors. `panel` and `entry` cannot be combined;
  missing locators, a wrong topic/kind, or incompatible filters are refused
  rather than redirected to a different entry.

## Each turn

After an update, give one terminal line naming its `full:` URL or path and
any new ask IDs from that turn; omit the ` - ...` suffix when there were no
new asks. Do not make an update or emit a status line just to fill this
template on an unchanged turn:

```text
Whiteboard: <full> - (102), (103)
```

When the host's instructions give an embed syntax, emit the host's embed line
for the returned `embed:` in place of the `Whiteboard:` line.

**While the user is watching the board, put as much of the conversation as practical on it, and do not repeat what the update wrote there in the terminal or chat. The user is watching when their current message starts with `[user message from the {{.McpNamespace}} whiteboard]` or carries a `<wb:answers>` block or `<wb:mention>` elements, when the host embeds the board inline in the conversation, or when they say they are watching. Otherwise the chat carries the conversation as usual.**

## Host embed folder

When the host's instructions name a folder its embedded files must live in,
send `<wb:embed-dir path="<folder>"/>` on your first update after the
session starts or is revived, and embed the bare file name the update
returns. The server remembers the folder for the session and refuses an
update once it has disappeared; send `wb:embed-dir` again, or with
`path=""` to return embeds to `ai-docs/whiteboards/`.

## Answers

A message sent from the dashboard opens with the user's board answers as a
`<wb:answers>` block, the user's typed text after it; the page's
copy-answers button builds the same block:

```text
<wb:answers>
62=b
44..46,51=ok
64=a^<note>
19=^<note>
[<topic>]^<note>
</wb:answers>
```

An item line is `<ids>=<value>[^<note>]`: comma-separated IDs, `44..46`
meaning 44 through 46, all taking the same value; the value is the option
letter, `ok`, `no`, or short text; the note follows the first `^`, with
`\n` a line break and `\\` a backslash, and `19=^<note>` is a note alone.
Record each item through `wb:answer`, its body that item's line:
`64=a^<note>` records the value `a, <note>`, and `44..46,51=ok` records `ok`
on all four with one `ids="44 45 46 51"` command quoting the whole line (an
`assumed` policy item records its option letter again). A `[<topic>]^<note>`
line is the user's comment or question about that topic, from its note pen,
naming the topic by its stem, or its name when it has none, which a
`wb:topic name` takes as is; answer it in that topic, with no item ID
needed. A typed reply in the older form, such as
`(62) b; (64) a, note: <text>; (44), (45) ok` with `[<topic>] <text>`
lines, reads the same way, each segment that item's body. A reply that
names no ID gets your reading with `assumed="true"`.

Board text the user selected and attached follows the answer block (or
opens the message without one), before the typed text, one element per
line in the order added:

```text
<wb:mention topic="<topic>" id="62" entry="e47">selected text</wb:mention>
```

`topic` names the topic as a topic note line does. Optional numeric `id`
attributes the selection to an item, including a system event about that
item. Optional `entry="eN"` identifies the selected immutable panel version
or agent/system log entry; it is not the stable latest-panel `pN` target.
Selections crossing entries arrive as ordered entry-specific mentions.
Existing topic-only and topic/item mentions remain valid. The content is
the selected text with `&`, `<`, and `>` written `&amp;`, `&lt;`, and `&gt;`.
Resolve a panel locator with `panel: "eN"` and a log locator with
`entry: "eN"` when its exact context is needed; do not silently substitute
the latest panel version. Mentions supply context for the user's typed
message, not answers or authored `{{12}}`/`{{p3}}` references.

Confirmed decisions reach a ticket only through `{{.SkillNamespace}}:lead-ticket`, once the user moves to capture. Authoritative topic folders remain after the session key expires; do not treat key expiry as board deletion.
