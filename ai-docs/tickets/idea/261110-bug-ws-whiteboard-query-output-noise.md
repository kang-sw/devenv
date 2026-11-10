---
title: "whiteboard.query: cut context noise in detailed output"
related:
  261110-feat-ws-whiteboard-query-depth-and-fields: "the depth/fields/body/closed/text contract this refines"
---

# whiteboard.query output noise

## Background

Dogfooding `ws/whiteboard.query` on a real session board (2026-11-10, 61
topics, ~260 items) for context cost. The summary shapes are lean: the
default query and `depth: topics` print one line per open topic plus one
`omitted:` line (~600 chars), and `ids` targeting is precise. The detailed
shapes carry most of the noise. A `topic` query on one settled topic of 11
items printed ~10k chars; roughly half of it was detail that settled items no
longer need.

Observed, with the query that showed it:

1. **Settled items print their full ask.** A settled policy item prints every
   option, the recommendation, and the rationale, though only the chosen
   option matters once it is answered. The answer value alone (`a`) cannot be
   read without the options, so the full list is kept today for decoding.
2. **`answer_quote` repeats for grouped answers.** An answer recorded with
   `ids="640 642 643 644 645"` prints the same quote under each of the five
   items.
3. **`answered_at` rides along with `answer`.** `fields: ["answer"]` still
   prints `answered_at` on every item.
4. **Multi-line rationale breaks the tree.** Continuation lines of a
   multi-line `rationale` (and `why`) print at column 0, not indented under
   the item, so they read like new top-level lines.
5. **`text` is a raw substring match.** `text: "LAN"` matched "lane" (a commit
   graph item) and items whose match is only in a rationale; the output does
   not say which field matched.
6. **Inline HTML tags in titles.** `<code>`, `<b>` from board markup print
   verbatim in every title line.
7. **`topic` needs the exact full title.** Long titles must be retyped
   exactly; there is no unique-prefix or substring form.

## Direction (user, 2026-11-10)

Today's `depth: full` output is the maximum verbosity, not noise to cut. The
gap is a missing middle level. Start from what a lead needs to know, find the
query it has to run for that today, and add the level that answers the need
without the rest. Trimming fields from `full` is not the goal on its own.

Needs seen while dogfooding, with the query each takes today:

| need | query today | what the need actually reads |
|---|---|---|
| What did the user decide in topic X (after compaction, before writing a ticket) | `topic` at `full` | title, chosen option text, note, assumed flag |
| What is waiting on the user and what exactly was asked | default + `ids` at `full` | the full ask of open items only |
| Was this already asked (dedupe before `wb:add`) | `text` | matching items, which field matched, a short snippet |
| Quote the user's words into a ticket's Decisions | `ids` at `full` | `answer_quote` of the listed items |
| Quote a topic body for `wb:edit` | `topic` (body + every item at `full`) | the body |

A middle level along these lines would print an open item's full ask and a
settled item as title + decoded answer (`a: <option text>`, note, assumed),
leaving options, recommendation, rationale, quote, and time to `full` or an
explicit `fields` list. Whether it becomes the default for `topic` and `ids`
targets is open.

## Topic key (user, 2026-11-10, board item 656)

Each topic gets a short stable key (stem) chosen by the lead, separate from its
title; the title becomes an editable label. `wb:topic` and `topic` queries
take the stem. A topic with no stem, such as one on a board a downstream
project already runs, keeps working with its label as the key. This settles
observation 7.

Rejected: a server-minted topic number (shortest, but the lead must query to
learn it every time); prefix or substring matching on the title (breaks when
the title changes).

## Open Questions

- Make the middle level the default for `topic` and `ids` targets, keeping
  `full` as the explicit maximum?
- For a settled policy item, print the chosen option text in place of the
  option list and recommendation at the default detail, keeping the full ask
  behind an explicit `fields` request? (Decides 1.)
- Print a grouped answer's quote once (on the first item, others referencing
  it), or drop `answer_quote` from defaults when it only restates the value?
  (Decides 2.)
- Make `answered_at` its own field? (Decides 3.)
- Word-boundary or token matching for `text`, and a matched-field marker?
  (Decides 5.)
- Strip board inline markup to plain text (or backticks) in query output?
  (Decides 6.)
- Accept a unique prefix or substring for `topic`? (Decides 7.)

Item 4 is a formatting bug with one answer: indent continuation lines under
their field.
