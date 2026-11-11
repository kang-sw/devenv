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
  name while an embed folder is set). An `info:` line names the items a
  `wb:close` withdrew, items holding options under a class other than
  `policy` (only a policy item's options are offered as picks), and, at most
  once an hour, open topics unchanged for an hour with nothing waiting on the
  user; close the finished ones with `wb:close`.
- `{{.McpNamespace}}/whiteboard.query(session_key, payload)` prints the
  board as a topic, item, and detail tree and changes nothing; with no
  payload, every open topic and its open items. Query before adding, so a
  question the board already holds is patched or reused, not added twice;
  `text` also finds one already settled.
- After the first update, todo and agenda writes, ticket moves, closes, and
  creations, and mailbox sends and receives re-render the full page and bump
  its stamp themselves, with no embed.

## Commands

Write board text (titles, options, rationale, recommendations, answer readings, and topic bodies) in the conversation language, the `workflow.lang` value when set; IDs, class and status tags, `ok`/`no` answers, and option letters stay as written.

Every tag carries the prefix `wb:` and follows the command syntax rules.
`wb:topic` is the group: its `name` names the topic once for the
`wb:body`, `wb:edit`, `wb:add`, and `wb:close` inside it. A batch may hold
several `wb:topic` groups.

| command | attributes | content |
|---|---|---|
| `wb:topic` | `name`, `label?` | group of `wb:body`, `wb:edit`, `wb:add`, `wb:close`; none with `label` |
| `wb:body` | none | the topic's body, replaced whole; empty clears it |
| `wb:edit` | `replace_all?` | fields `wb:old`, `wb:new` |
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
<wb:add class="policy">
<wb:title>Ship the <code>parser</code> before the renderer?</wb:title>
<wb:option>Parser first</wb:option>
<wb:option>Both at once</wb:option>
<wb:recommend>a: smaller review</wb:recommend>
<wb:why>A body may hold <details>, & and <option> as is.</wb:why>
<wb:link>supersedes {{12}}</wb:link>
</wb:add>
</wb:topic>
<wb:answer ids="14 15" value="ok">(14), (15) ok</wb:answer>
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
- `class`: `policy`, `technical`, or `announced`, the classes of the workflow
  manual's `### Asking the user`. An item belongs to its group's topic,
  which groups items on the page; `wb:title` is one line.
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
  `wb:option`, `wb:why`, and `wb:recommend` take block markdown with line
  breaks kept: lists, code blocks, tables, images, and a `` ```mermaid ``
  fenced block drawn as a diagram, plus HTML such as `<img>`, `<table>`,
  `<details>`, and simple inline SVG, shown as static markup with script,
  style, frames, form controls, event attributes, and positioning removed.
  A line that starts with a block tag follows the body's HTML rule below. A
  link or image path, in markdown or in `href` and `src`, is relative to the
  session root or absolute; images are local files only, links may also be
  http(s). An open technical or announced item shows its `wb:why` inline,
  one line per reason, with only the title's tags rendered.
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
  of a vague reply; the page shows the item as a settled row marked assumed,
  with a note pen the user can veto or comment through. Record the user's
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
- Re-ask: a `wb:patch` that changes an item's `class` or options, or sets a
  settled or assumed item back to `open`, asks it again. The dashboard then
  drops the sent mark of an answer the user sent for it before the patch, so
  the item shows as waiting on the user. An answer sent before the patch
  and not yet delivered still reaches you, and it refers to the options it
  was sent against. A patch that only edits the title, `wb:why`,
  `wb:recommend`, or links, or resends the current class or options, is not
  a re-ask.
- `wb:close` collapses a topic's settled items and its body; the topic needs
  an item or a body. `result` is required: one sentence saying how the topic
  ended, shown dimmed beside the closed topic's name. Closing withdraws every
  item of the topic still `open`, with `result` as the reason, and an
  `info:` line names them; an `assumed` item stays assumed. Put a batch's
  answers to the topic's items before its `wb:close`, since an answer after
  the close finds the item withdrawn. Adding an item to a closed topic, or
  changing its body, reopens it and hides the result, but does not restore
  the withdrawn items; the next close replaces the result. Closed topics with
  nothing waiting on the user go last, most recently closed first; past 30,
  the dashboard page loads the rest on request and the file page and embed
  print how many are left out.
- Topic body: one per topic, rendered above its items, for context that
  outlasts a chat reply: background, the shape of a decision, a diagram; a
  topic may hold a body and no items. `wb:body` replaces the body whole,
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
whiteboard.query payload: {topic?, ids?, id?, status?, text?, depth?, fields?, body?, closed?}
```

- `payload` is a JSON object, every field optional. The filters `topic`,
  `id`, `ids`, `status`, and `text` AND together, except that `id` (a
  number) and `ids` (a list of numbers) union. `topic` and `status` take one
  string or a list; `topic` resolves each name as `wb:topic name` does, and
  `status` takes any of the five statuses. `text` matches a substring of an
  item's title, options, rationale, or answer, ignoring case, and each
  matching item line ends with `[match: <fields>]`.
- `depth`: `topics` prints one line per topic: open or closed, its item
  counts by status, its body length, its last change, and its close result
  when closed. `items` adds a line per item under its topic: ID, status,
  class, title. `summary` adds, under an open item, its full ask, and under
  an answered or assumed item one decoded answer line (`answer: b: <option
  text> - <note>`, ` (assumed)` appended); a withdrawn or superseded item
  keeps only its line. `full` adds every detail; `fields`, a list from
  `options`, `recommendation`, `rationale`, `answer`, `answered_at`,
  `answer_quote`, and `links`, picks which, all when omitted; `fields`
  without `depth` means `full`. Items answered together print their shared
  `answer_quote` once, then `same as (nn)`.
- `body: true` prints each listed topic's body after its topic line.
  `closed: true` lists closed topics, whose items follow the same depth and
  status as any topic's; otherwise one `omitted:` count stands for them.
- The defaults come from the most specific target present, and an explicit
  `depth`, `status`, `body`, or `closed` overrides them:

  | target | depth | status | body | closed topics |
  |---|---|---|---|---|
  | `ids` or `id` | `summary` | all | no | listed |
  | `topic` | `summary` | all | yes | listed |
  | `text` | `items` | all | no | listed |
  | none | `items` | `open` | no | folded |

  So `{"status": "answered"}` prints answered items at depth `items`; add
  `"depth": "full"` for their details, such as the `answer_quote` a ticket
  quotes. `ids` and `text` list only the topics holding a match.
- A topic line names a stemmed topic `topic <stem> "<label>"` and a
  stemless one `topic "<name>"`. Item fields, labels, and close results
  print `<code>` as backticks and `<br>` as a space, and drop the other
  inline tags; options, recommendations, and `wb:why` drop every tag and
  keep its text, an image as its alt text, script and style as nothing. The
  topic body and `answer_quote` print as stored, so a `wb:edit` quote
  copies the body exactly.
- Topics print in the order their first item was added, items in ID order.
  A last `omitted:` line counts what the defaults left out and names the
  filter that shows it.

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
names no ID gets your reading with `assumed="true"`. A `[<topic>] <text>`
line is the user's comment or question about that topic, from its note pen,
naming the topic by its stem, or its name when it has none, which a
`wb:topic name` takes as is; answer it in that topic, with no item ID
needed.

Confirmed decisions reach a ticket only through `{{.SkillNamespace}}:lead-ticket`, once the user moves to capture; the board clears with the session key.
