---
kind: print
---

# Whiteboard

The session whiteboard is a top-level lead's board for its user. The MCP
server keeps the items you ask the user (IDs, statuses, the user's verbatim
answers) in the session state and renders them, with the session's todos,
agendas, and a Now slot, into a full page the user keeps open beside the
terminal and an embed snapshot for a host that shows HTML inline. Workers and
subagents do not use it.

## Calls

- `{{.McpNamespace}}/whiteboard.update(session_key, payload)` applies one
  batch and renders: it overwrites the full page, bumps its change stamp, and
  writes a new embed snapshot. Make one call per turn carrying all of that
  turn's changes; an empty payload only re-renders.
- `{{.McpNamespace}}/whiteboard.query(session_key, payload)` returns items,
  archived ones included, and changes nothing. Query before adding items the
  board may already hold.
- Todo and agenda writes re-render the full page by themselves once the
  board exists.

## Payload

```text
whiteboard.update payload: {
  add?:          [{id?, class, topic, title, options?, recommendation?, rationale?, links?}],
  answer?:       [{id, answer, answer_quote, assumed?}],
  patch?:        [{id, title?, options?, recommendation?, rationale?, links?, status?}],
  close_topics?: [topic],
  now?:          html fragment,    "" clears the Now slot
  now_path?:     path,             exclusive with now
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
- `options`: plain strings, lettered a, b, c by position. `recommendation`:
  an option letter, optionally followed by `: <why>`.
- `rationale`: a technical or announced item's citation or reasoning, or a
  policy item's context.
- `links`: strings such as a ticket stem, a commit, or `supersedes (12)`;
  every `(nn)` in a link names an item in the store.
- `answer`: a policy item's option letter, optionally followed by
  `, <text>`; otherwise `ok`, `no`, or short text. `answer_quote` is
  required: the user's verbatim words for this item. `assumed: true` records
  your reading of a vague reply; the item stays highlighted until the user
  confirms or corrects it, which you record as another `answer` on the same
  ID without `assumed`, quoting the confirming or correcting words.
- `patch.status`: `open`, `withdrawn`, or `superseded`. `options` change only
  on an open item, since an answer is stored as an option letter.
- `close_topics`: collapses the topics' settled items at once; adding an item
  to a closed topic reopens it.
- `now`: an HTML fragment, never a full document, for explanations and
  drawings in inline SVG or canvas 2D without external resources; it shows
  above the topics on the update that changed it. `now_path` names a file
  holding the fragment, read once at that update. A large standalone visual
  goes to a `{{.McpNamespace}}/path.generate` file linked from the fragment.
- `id_floor`: raises the ID counter so the next minted ID is one past it; a
  floor at or below the counter is ignored.
- `embed_dir`: see Host embed folder.
- A batch is validated whole: any problem refuses all of it, listing every
  problem with this schema. An accepted batch applies in the order
  `embed_dir`, `id_floor`, `add`, `answer`, `patch`, `close_topics`, `now`;
  the result lists the added IDs in `add` order, the full page, and the
  embed.

## Each turn

After the update, give one terminal line naming the full page the update
returned and the IDs you asked this turn:

```text
Whiteboard: ai-docs/whiteboards/<reply-id>.html - (102), (103)
```

When the host's instructions give an embed syntax, emit the embed line for
the returned embed instead.

## Host embed folder

When the host's instructions name a folder its embedded files must live in,
pass that folder as `embed_dir` on your first update after the session starts
or is revived, and embed the bare file name the update returns. The server
remembers the folder for the session and refuses an update once it has
disappeared; pass `embed_dir` again, or `""` to return embeds to the
repository.

## Answers

The user answers by ID, typed or pasted from the page's copy-answers button,
for example `(62) b; (64) a, note: <text>; (19) no, note: <text>; (44), (45) ok`.
Record each item through `answer`, its `answer_quote` being that item's
segment of the reply; every ID in an `ok` group is confirmed.

Confirmed decisions reach a ticket only through `{{.SkillNamespace}}:lead-ticket`, once the user moves to capture; the board clears with the session key.
