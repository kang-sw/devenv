---
kind: print
---

# Whiteboard

The session whiteboard is a top-level lead's board for its user. The MCP
server keeps the items you ask the user (IDs, statuses, the user's verbatim
answers) in the session state and renders them, with the session's todos,
agendas, ready and todo tickets, mailbox, and a Now slot, into a full page the
user keeps open beside the terminal and an embed snapshot for a host that
shows HTML inline. Workers and subagents do not use it.

## Calls

- `{{.McpNamespace}}/whiteboard.update(session_key, payload)` applies one
  batch and renders: it rewrites the session's one full page, bumps its
  change stamp, and writes a new embed snapshot. Make one call per turn
  carrying all of that turn's changes; an empty payload only re-renders. The
  result gives `ids:` (the added IDs in `add` order, when any), `full:` (the
  session's dashboard page URL while the dashboard daemon serves the session,
  else the full page path relative to the session root), and
  `embed:` (the new snapshot's path, or its bare file name while `embed_dir`
  is set).
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

## Payload

Write board text (titles, options, rationale, recommendations, answer readings, and the Now text) in the conversation language, the `workflow.lang` value when set; item IDs, class and status tags, `ok`/`no` answers, and option letters stay as written.

```text
whiteboard.update payload: {
  add?:          [{id?, class, topic, title, options?, recommendation?, rationale?, links?}],
  answer?:       [{id, answer, answer_quote, assumed?}],
  patch?:        [{id, title?, options?, recommendation?, rationale?, links?, status?}],
  close_topics?: [topic],
  now?:          markdown,         "" clears the Now slot
  now_path?:     .md file path,    exclusive with now
  id_floor?:     number,
  embed_dir?:    absolute path,    "" clears
}
whiteboard.query payload: {topic?, status?, id?}
```

- `payload` is a JSON object and every array field an array; a JSON-encoded
  string in their place is refused.
- `class`: `policy`, `technical`, or `announced`, the classes of the workflow
  manual's `### Asking the user`. `topic` groups items on the page; `title`
  is one line.
- `id` on `add`: omit it and the server mints one past the highest ID it has
  minted, adopted, or been floored to. Pass a number to adopt an ID that
  exists outside the board, such as a ticket queue's; an ID the store already
  holds is refused.
- `options`: strings, lettered a, b, c by position; patching `options`
  re-letters them, so send `recommendation` with them. `recommendation`: an
  option letter, optionally followed by `: <why>`.
- Markdown: `title` takes inline markdown (bold, italic, code, links);
  `options`, `rationale`, and `recommendation` take block markdown with
  line breaks kept: lists, code blocks, tables, images, and a
  `` ```mermaid `` fenced block drawn as a diagram. Raw HTML shows as text in
  every field but `now`. A link or image path is relative to the session
  root or absolute; images are local files only, links may also be
  http(s). An open technical or announced item shows its `rationale` inline,
  one line per reason.
- `rationale`: a technical or announced item's citation or reasoning, or a
  policy item's context.
- `links`: strings such as a ticket stem, a commit, or `supersedes {{12}}`.
- Item references: write `{{12}}` in `title`, `options`, `recommendation`,
  `rationale`, `links`, the markdown text of `now` (never inside its HTML),
  or the reading part of an `answer` to cite item `(12)`. The page shows it
  as `(12)`, linked when that item is on the page, and leaves it verbatim
  when the store holds no such item; a reference is never refused. A plain
  `(12)` is literal text, and `answer_quote` is never resolved, since it is
  the user's verbatim words.
- `answer`: for a policy item, its option letter, optionally followed by
  `, <text>`; for a technical or announced item, `ok`, `no`, or short text. A
  later `answer` replaces an earlier one. `answer_quote` is required: the
  user's verbatim words for this item. `assumed: true` records your reading
  of a vague reply; the item stays highlighted until the user confirms or
  corrects it, which you record as another `answer` on the same ID without
  `assumed`, quoting the confirming or correcting words.
- Status: an item starts `open`; `answer` sets `answered`, or `assumed` with
  `assumed: true`; `patch.status` sets `open`, `withdrawn`, or `superseded`.
  Answered, withdrawn, and superseded items are settled: the page keeps the
  10 most recently settled expanded and the rest behind their topic's count
  chip. `options` change only on an open item, since an answer is stored as
  an option letter. `query.status` takes any of the five statuses.
- `close_topics`: collapses the topics' settled items at once; adding an item
  to a closed topic reopens it.
- `now`: block markdown as in `options`, with inline HTML written as is,
  never a full HTML document, for explanations and drawings: inline SVG, or
  canvas 2D drawn by an inline script, with no external `src`, `href`, or
  fetch. It stays until a later `now` replaces it or `""` clears it, always
  above the topics: open on the update that changed it, collapsed and greyed
  after. `now_path` names a markdown file holding it, relative to the session
  root or absolute, read once at that update. A large standalone visual goes
  to a `{{.McpNamespace}}/path.generate` file linked from it.
- HTML in `now`: a line that starts with a block tag such as `<div>` or
  `<details>`, or holds a lone tag such as `<svg ...>`, starts raw HTML that
  runs to the next blank line, so put a blank line between HTML and the
  markdown after it, and keep blank lines out of HTML, since an indented line
  after one becomes a code block. A line that starts with an inline tag such
  as `<b>` is markdown text, whose line breaks render as `<br>`.
- `id_floor`: raises the ID counter so the next minted ID is one past it; a
  floor at or below the counter is ignored.
- `embed_dir`: see Host embed folder.
- A batch is validated whole: any problem refuses all of it, and the refusal
  lists every problem and repeats this schema. An accepted batch applies in
  the order `embed_dir`, `id_floor`, `add`, `answer`, `patch`,
  `close_topics`, then `now` or `now_path`.

## Each turn

After the update, give one terminal line naming the `full:` URL or path the
update returned and the IDs you asked this turn:

```text
Whiteboard: <full> - (102), (103)
```

When the host's instructions give an embed syntax, emit the host's embed line
for the returned `embed:` in place of the `Whiteboard:` line.

**Never repeat in the terminal or chat what the update wrote on the board; the turn's terminal output is only that pointer (or embed) line plus any fixed-format lines a playbook mandates.**

## Host embed folder

When the host's instructions name a folder its embedded files must live in,
pass that folder as `embed_dir` on your first update after the session starts
or is revived, and embed the bare file name the update returns. The server
remembers the folder for the session and refuses an update once it has
disappeared; pass `embed_dir` again, or `""` to return embeds to
`ai-docs/whiteboards/`.

## Answers

The user answers by ID, typed or pasted from the page's copy-answers button,
for example `(62) b; (64) a, note: <text>; (19) no, note: <text>; (44), (45) ok`.
Record each item through `answer`, its `answer_quote` being that item's
segment of the reply: `(64) a, note: <text>` records the answer
`a, <text>`, and each ID of `(44), (45) ok` records `ok` (an `assumed`
policy item records its option letter again), quoting the whole group. A
reply that names no ID gets your reading with `assumed: true`.

Confirmed decisions reach a ticket only through `{{.SkillNamespace}}:lead-ticket`, once the user moves to capture; the board clears with the session key.
