---
kind: print
---

# Whiteboard

The session whiteboard is a top-level lead's board for its user. The MCP
server keeps the items you ask the user (IDs, statuses, the user's verbatim
answers) and each topic's body in the session state and renders them, with
the session's todos, agendas, ready and todo tickets, and mailbox, into a
full page the user keeps open beside the terminal and an embed snapshot for a
host that shows HTML inline. Workers and subagents do not use it.

## Calls

- `{{.McpNamespace}}/whiteboard.update(session_key, commands)` applies one
  batch and renders: it rewrites the session's one full page, bumps its
  change stamp, and writes a new embed snapshot. Make one call per turn
  carrying all of that turn's changes; empty `commands` only re-renders. The
  result gives `ids:` (the IDs of the batch's `wb:add` commands, in order,
  when any), `full:` (the session's dashboard page URL while the dashboard
  daemon serves the session, else the full page path relative to the session
  root), and `embed:` (the new snapshot's path, or its bare file name while
  an embed folder is set).
- `{{.McpNamespace}}/whiteboard.query(session_key, payload)` returns stored
  items, settled ones included, and changes nothing. Query before adding, so
  a question the board already holds is patched or reused rather than added
  twice.
- Once the session's first update has rendered, todo and agenda writes,
  ticket moves, closes, and creations
  (`{{.McpNamespace}}/tickets.move`, `{{.McpNamespace}}/tickets.close`,
  `{{.McpNamespace}}/tickets.create_empty`), and mailbox sends and receives
  (`{{.McpNamespace}}/mailbox.send`, `{{.McpNamespace}}/mailbox.recv`)
  re-render the full page and bump its stamp by themselves; they write no
  embed.

## Commands

Write board text (titles, options, rationale, recommendations, answer readings, and topic bodies) in the conversation language, the `workflow.lang` value when set; item IDs, class and status tags, `ok`/`no` answers, and option letters stay as written.

`commands` is one string of flat commands. Every tag carries the literal
prefix `wb:`, with no namespace declaration and no root element.

| command | attributes | content |
|---|---|---|
| `wb:add` | `id?`, `class`, `topic` | fields `wb:title`, `wb:option`..., `wb:recommend?`, `wb:why?`, `wb:link`... |
| `wb:answer` | `id` or `ids`, `value`, `assumed?` | the user's verbatim words |
| `wb:patch` | `id`, `status?`, `clear?` | any `wb:add` field, each replacing that field |
| `wb:topic` | `name` | the topic's body, replaced whole; empty clears it |
| `wb:edit` | `topic`, `replace_all?` | fields `wb:old`, `wb:new` |
| `wb:close` | `topic` | none |
| `wb:floor` | `id` | none |
| `wb:embed-dir` | `path` | none |

```xml
<wb:add class="policy" topic="Rollout">
<wb:title>Ship the parser before the renderer?</wb:title>
<wb:option>Parser first</wb:option>
<wb:option>Both at once</wb:option>
<wb:recommend>a: smaller review</wb:recommend>
<wb:why>A body may hold <details>, & and <option> as is.</wb:why>
<wb:link>supersedes {{12}}</wb:link>
</wb:add>
<wb:answer ids="14 15" value="ok">(14), (15) ok</wb:answer>
<wb:patch id="16" status="withdrawn"/>
<wb:topic name="Rollout">
## Context
Markdown up to the closing tag.
</wb:topic>
<wb:edit topic="Rollout">
<wb:old>## Context</wb:old>
<wb:new>## Context and motive</wb:new>
</wb:edit>
<wb:close topic="Old scope"/>
```

- Syntax: a body is raw text up to the first exact closing tag of its
  element, so `<`, `&`, inline HTML, and mermaid are written as is, never
  escaped. A body that must contain its own closing tag puts that text in
  `<![CDATA[...]]>`. A body loses one newline right after its opening tag
  and one right before its closing tag; nothing else is trimmed. An
  attribute value is quoted with `"` or `'` and is not decoded either, so a
  value holding `"` is quoted with `'`. A tag with no content may
  self-close. Text between commands or between a command's fields, an
  unknown tag, and an unknown attribute are refused.
- Commands apply in the order written. A batch is validated whole: any
  problem refuses all of it, and the refusal lists every problem with its
  command position, tag, and line, and repeats this section.
- `class`: `policy`, `technical`, or `announced`, the classes of the workflow
  manual's `### Asking the user`. `topic` groups items on the page;
  `wb:title` is one line.
- `id` on `wb:add`: omit it and the server mints one past the highest ID it
  has minted, adopted, or been floored to. Pass a number to adopt an ID that
  exists outside the board, such as a ticket queue's; an ID the store
  already holds is refused.
- `wb:option`: one per option, lettered a, b, c by position; a `wb:patch`
  carrying any `wb:option` replaces the whole list and re-letters it, so
  send `wb:recommend` with it. `wb:recommend`: an option letter, optionally
  followed by `: <why>`. On `wb:patch`, `clear="options"`, `clear="links"`,
  or `clear="options links"` empties those lists, and an empty
  `wb:recommend` or `wb:why` clears that field.
- Markdown: `wb:title` takes inline markdown (bold, italic, code, links);
  `wb:option`, `wb:why`, and `wb:recommend` take block markdown with line
  breaks kept: lists, code blocks, tables, images, and a `` ```mermaid ``
  fenced block drawn as a diagram. Raw HTML shows as text in every item
  field; only a topic body renders it. A link or image path is relative to
  the session root or absolute; images are local files only, links may also
  be http(s). An open technical or announced item shows its `wb:why`
  inline, one line per reason.
- `wb:why`: a technical or announced item's citation or reasoning, or a
  policy item's context.
- `wb:link`: one per link, such as a ticket stem, a commit, or
  `supersedes {{12}}`.
- Item references: write `{{12}}` in an item field, the markdown text of a
  topic body (never inside its HTML), or the reading part of an answer
  `value` to cite item `(12)`. The page shows it as `(12)`, linked when that
  item is on the page, and leaves it verbatim when the store holds no such
  item; a reference is never refused. A plain `(12)` is literal text, and a
  `wb:answer` body is never resolved, since it is the user's verbatim words.
- `wb:answer`: `value` is, for a policy item, its option letter, optionally
  followed by `, <text>`; for a technical or announced item, `ok`, `no`, or
  short text. A later answer replaces an earlier one. The body is required:
  the user's verbatim words for this item. `ids="14 15"` records the same
  value and words on each listed item. `assumed="true"` records your reading
  of a vague reply; the item stays highlighted until the user confirms or
  corrects it, which you record as another `wb:answer` on the same ID
  without `assumed`, quoting the confirming or correcting words.
- Status: an item starts `open`; `wb:answer` sets `answered`, or `assumed`
  with `assumed="true"`; `wb:patch status` sets `open`, `withdrawn`, or
  `superseded`. Answered, withdrawn, and superseded items are settled: the
  page keeps the 10 most recently settled expanded and the rest behind their
  topic's count chip. Options change only on an open item, since an answer
  is stored as an option letter.
- `wb:close`: collapses the topic's settled items and its body at once; it
  needs a topic that has an item or a body. Adding an item to a closed
  topic, or changing its body, reopens it.
- Topic body: `wb:topic` gives a topic one body, rendered above its items,
  for context that outlasts a chat reply: background, the shape of a
  decision, a diagram. A topic may hold a body and no items. Progress and
  explanations go to chat, not to a body. `wb:topic` replaces the body
  whole, an empty one clears it, and the same body again is no change.
  `wb:edit` replaces the one exact match of `wb:old` with `wb:new` (an empty
  `wb:new` deletes it), and every match with `replace_all="true"`; no match,
  or several without `replace_all`, is refused. To insert, put the anchor
  text in `wb:old` and the anchor plus the insertion in `wb:new`. Change a
  body with `wb:edit` rather than resending it; `whiteboard.query` with
  `topic` prints the stored body to quote from. A body shows open on the
  full page unless its topic is closed, and on the embed only when it
  changed since the previous embed.
- A body is block markdown as in `wb:option`, with inline HTML written as
  is, never a full HTML document: inline SVG, or canvas 2D drawn by an
  inline script, with no external `src`, `href`, or fetch. A large
  standalone visual goes to a `{{.McpNamespace}}/path.generate` file linked
  from it.
- HTML in a body: a line that starts with a block tag such as `<div>` or
  `<details>`, or holds a lone tag such as `<svg ...>`, starts raw HTML that
  runs to the next blank line, so put a blank line between HTML and the
  markdown after it, and keep blank lines out of HTML, since an indented
  line after one becomes a code block. A line that starts with an inline tag
  such as `<b>` is markdown text, whose line breaks render as `<br>`.
- `wb:floor`: raises the ID counter so the next minted ID is one past `id`;
  a floor at or below the counter is ignored.
- `wb:embed-dir`: see Host embed folder.

## Query

```text
whiteboard.query payload: {topic?, status?, id?}
```

- `payload` is a JSON object of filters, all optional; `status` takes any of
  the five statuses. Items print in ID order; with `topic`, that topic's
  body prints first.

## Each turn

After the update, give one terminal line naming the `full:` URL or path the
update returned and the IDs you asked this turn:

```text
Whiteboard: <full> - (102), (103)
```

When the host's instructions give an embed syntax, emit the host's embed line
for the returned `embed:` in place of the `Whiteboard:` line.

**Never repeat in the terminal or chat what the update wrote on the board: items and topic bodies stay on the board, and the terminal or chat carries only progress, explanations, that pointer (or embed) line, and any fixed-format lines a playbook mandates.**

## Host embed folder

When the host's instructions name a folder its embedded files must live in,
send `<wb:embed-dir path="<folder>"/>` on your first update after the
session starts or is revived, and embed the bare file name the update
returns. The server remembers the folder for the session and refuses an
update once it has disappeared; send `wb:embed-dir` again, or with
`path=""` to return embeds to `ai-docs/whiteboards/`.

## Answers

The user answers by ID, typed or pasted from the page's copy-answers button,
for example `(62) b; (64) a, note: <text>; (19) no, note: <text>; (44), (45) ok`.
Record each item through `wb:answer`, its body being that item's segment of
the reply: `(64) a, note: <text>` records the value `a, <text>`, and
`(44), (45) ok` records `ok` on both with one `ids="44 45"` command (an
`assumed` policy item records its option letter again), quoting the whole
group. A reply that names no ID gets your reading with `assumed="true"`.

Confirmed decisions reach a ticket only through `{{.SkillNamespace}}:lead-ticket`, once the user moves to capture; the board clears with the session key.
