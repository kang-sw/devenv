---
kind: print
---

# Whiteboard

The session whiteboard is a top-level lead's board for its user. The server
keeps the items you ask (IDs, statuses, verbatim answers) and topic bodies,
and renders them with the session's todos, agendas, ready and todo tickets,
and mailbox into a full page beside the terminal and an embed snapshot for a
host that shows HTML inline. Workers and subagents do not use it.

## Calls

- `{{.McpNamespace}}/whiteboard.update(session_key, commands)` applies one
  batch, rewrites the full page, bumps its change stamp, and writes a new
  embed snapshot. Make one call per turn with all of that turn's changes;
  empty `commands` only re-renders. The result gives `ids:` (the batch's
  `wb:add` IDs, in order), `full:` (the session's dashboard page URL while
  the dashboard daemon serves the session, else the full page path relative
  to the session root), and `embed:` (the snapshot's path, or its bare file
  name while an embed folder is set).
- `{{.McpNamespace}}/whiteboard.query(session_key, payload)` returns stored
  items, settled ones included, and changes nothing. Query before adding, so
  a question the board already holds is patched or reused, not added twice.
- After the first update, todo and agenda writes, ticket moves, closes, and
  creations, and mailbox sends and receives re-render the full page and bump
  its stamp themselves, with no embed.

## Commands

Write board text (titles, options, rationale, recommendations, answer readings, and topic bodies) in the conversation language, the `workflow.lang` value when set; IDs, class and status tags, `ok`/`no` answers, and option letters stay as written.

Every tag carries the prefix `wb:` and follows the command syntax rules.

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
<wb:title>Ship the <code>parser</code> before the renderer?</wb:title>
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
<wb:close topic="Old scope"/>
```

- `class`: `policy`, `technical`, or `announced`, the classes of the workflow
  manual's `### Asking the user`. `topic` groups items on the page;
  `wb:title` is one line.
- `id` on `wb:add`: omitted, the server mints one past the highest ID it has
  minted, adopted, or been floored to; a number adopts an ID from outside the
  board, such as a ticket queue's. An ID the store holds is refused.
- `wb:option`: one per option, lettered a, b, c by position; a `wb:patch`
  carrying any `wb:option` replaces and re-letters the whole list, so send
  `wb:recommend` with it. `wb:recommend`: an option letter, optionally
  followed by `: <why>`. On `wb:patch`, `clear="options"`, `clear="links"`,
  or `clear="options links"` empties those lists, and an empty
  `wb:recommend` or `wb:why` clears that field.
- Markdown: `wb:title` takes inline markdown (bold, italic, code, links);
  `wb:option`, `wb:why`, and `wb:recommend` take block markdown with line
  breaks kept: lists, code blocks, tables, images, and a `` ```mermaid ``
  fenced block drawn as a diagram. Item fields also render `<code>`, `<b>`,
  `<strong>`, `<i>`, `<em>`, `<kbd>`, and `<br>` written without attributes;
  any other tag shows as text. A link or image path is relative to the
  session root or absolute; images are local files only, links may also be
  http(s). An open technical or announced item shows its `wb:why` inline,
  one line per reason.
- `wb:why`: a technical or announced item's citation or reasoning, or a
  policy item's context. `wb:link`: one per link, such as a ticket stem, a
  commit, or `supersedes {{12}}`.
- Item references: write `{{12}}` in an item field, the markdown text of a
  topic body (never inside its HTML), or the reading part of an answer
  `value` to cite item `(12)`. The page shows `(12)`, linked when that item
  is on the page, and verbatim when the store holds no such item; a
  reference is never refused. A plain `(12)` is literal text, and a
  `wb:answer` body, the user's verbatim words, is never resolved.
- `wb:answer`: `value` is, for a policy item, its option letter, optionally
  followed by `, <text>`; for a technical or announced item, `ok`, `no`, or
  short text. A later answer replaces an earlier one. The body, required, is
  the user's verbatim words for this item. `ids="14 15"` records the same
  value and words on each listed item. `assumed="true"` records your reading
  of a vague reply; the item stays highlighted until you record the user's
  confirming or correcting words as another `wb:answer` on that ID without
  `assumed`.
- Status: an item starts `open`; `wb:answer` sets `answered`, or `assumed`
  with `assumed="true"`; `wb:patch status` sets `open`, `withdrawn`, or
  `superseded`. Answered, withdrawn, and superseded items are settled: the
  page keeps the 10 most recently settled expanded and the rest behind their
  topic's count chip. Options change only on an open item, since an answer
  is stored as an option letter.
- `wb:close` collapses a topic's settled items and its body; the topic needs
  an item or a body. Adding an item to a closed topic, or changing its body,
  reopens it.
- Topic body: one per topic, rendered above its items, for context that
  outlasts a chat reply: background, the shape of a decision, a diagram; a
  topic may hold a body and no items. `wb:topic` replaces the body whole,
  an empty one clears it, and the same body again is no change. `wb:edit`
  replaces the one exact match of `wb:old` with `wb:new` (an empty `wb:new`
  deletes it), or every match with `replace_all="true"`; no match, or
  several without `replace_all`, is refused. To insert, put the anchor in
  `wb:old` and the anchor plus the insertion in `wb:new`. Change a body with
  `wb:edit` rather than resending it; `whiteboard.query` with `topic` prints
  the stored body to quote from. A body shows open on the full page unless
  its topic is closed, and on the embed only when it changed since the
  previous embed.
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
- `wb:floor` raises the ID counter so the next minted ID is one past `id`; a
  floor at or below the counter is ignored. `wb:embed-dir`: see Host embed
  folder.

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
Record each item through `wb:answer`, its body that item's segment of the
reply: `(64) a, note: <text>` records the value `a, <text>`, and `(44), (45)
ok` records `ok` on both with one `ids="44 45"` command (an `assumed` policy
item records its option letter again), quoting the whole group. A reply that
names no ID gets your reading with `assumed="true"`.

Confirmed decisions reach a ticket only through `{{.SkillNamespace}}:lead-ticket`, once the user moves to capture; the board clears with the session key.
