---
title: "whiteboard: a middle query verbosity level and stable topic keys"
related:
  261110-feat-ws-whiteboard-query-depth-and-fields: "the depth/fields/body/closed/text contract this refines"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3573a255edbfa06a
sage-review-completeness-reviewed: 3573a255edbfa06a
completed: 2026-11-10
---

# whiteboard: a middle query verbosity level and stable topic keys

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
   exactly, and so must every `wb:topic name` on update.

### Direction (user, 2026-11-10)

Today's `depth: full` output is the maximum verbosity, not noise to cut. The
gap is a missing middle level. Start from what a lead needs to know, find the
query it has to run for that today, and add the level that answers the need
without the rest. Trimming fields from `full` is not the goal on its own.

| need | query today | what the need actually reads |
|---|---|---|
| What did the user decide in topic X (after compaction, before writing a ticket) | `topic` at `full` | title, chosen option text, note, assumed flag |
| What is waiting on the user and what exactly was asked | default + `ids` at `full` | the full ask of open items only |
| Was this already asked (dedupe before `wb:add`) | `text` | matching items, which field matched |
| Quote the user's words into a ticket's Decisions | `ids` at `full` | `answer_quote` of the listed items |
| Quote a topic body for `wb:edit` | `topic` (body + every item at `full`) | the body |

## Decisions

Board items (655), (656), (658), and (666) are the user's; (657) and (659)-(665) are
lead technical calls recorded on the board as assumed (the user may veto
there).

- **D1. New depth `summary` between `items` and `full`** (657). An open item
  prints as at `full` (options, recommendation, rationale, links). A settled
  item prints its ID/status/class/title line and one decoded answer line.
  For a policy item whose value starts with an option letter the line is
  `answer: <letter>: <option text>`, followed by ` - <note>` when the value
  carries text after the letter (`b, note text` prints
  `answer: b: Parser first - note text`); a value with no matching option
  letter, and every technical/announced value, prints as stored
  (`answer: ok`). An assumed item appends ` (assumed)`. A withdrawn or
  superseded item prints only its item line. Option text and note are
  rendered per D7. `fields` still selects detail lines at `full` only. The
  topic body follows the existing `body` control at every depth (a `topic`
  target prints it by default).
  Rejected: trimming `full` itself (the user's direction keeps `full` as the
  maximum); a per-field opt-out list (callers would have to know the noise in
  advance).
- **D2. `summary` becomes the default depth for the `ids`/`id` and `topic`
  targets** (658, user; reverses (624) in
  261110-feat-ws-whiteboard-query-depth-and-fields, Decision 3). `text` and
  the unfiltered default stay at `items`; an explicit `depth: "full"` gives
  today's output. (624) protected two callers: Open Decision Queue
  reconciliation reads open items, which `summary` prints in full, and body
  quoting for `wb:edit` reads the topic body, which `topic` still prints.
  Rejected: keeping `full` as those defaults ((624); the measured noise came
  from exactly these defaults).
- **D3. A grouped answer's quote prints once per query output** (659): when
  several printed items carry the same `answer_quote` and answer time, the
  first prints it and the rest print `answer_quote: same as (NN)`.
  Rejected: dropping `answer_quote` from `full` (it is the only source of the
  verbatim words a ticket quotes).
- **D4. `answered_at` becomes its own field** (660) in the `fields` list;
  `answer` no longer carries it. With `fields` omitted, `full` still prints it.
- **D5. Multi-line field values indent every continuation line** to the
  field's own indentation (661). Bug fix; one answer.
- **D6. `text` stays a case-insensitive substring match, and every matching
  item line names the fields that matched** (662), e.g. `match: rationale`.
  Rejected: word-boundary/token matching (Korean particles attach to words,
  so `토픽` would miss `토픽을`; a marker fixes the misleading case without
  losing recall).
- **D7. Query output renders board inline markup as plain text** (663) in
  item fields (title, options, recommendation, rationale, links, the answer
  value and its decoded line) and in topic and close-result lines:
  `<code>x</code>` becomes `` `x` ``, `<br>` becomes a space, and the other
  allowed inline tags (`<b>`, `<strong>`, `<i>`, `<em>`, `<kbd>`) are dropped
  keeping their text. The topic body and `answer_quote` print verbatim: the
  body is the exact-match source for `wb:edit` quoting, and the quote is the
  user's own words. Text matching (D6) runs on the stored text, unchanged.
  Rejected: printing tags verbatim (today; costs tokens on every title line).
- **D8. Topic stem with label fallback** (656, user). A topic may carry a
  short stable key (stem) chosen by the lead, separate from its title; the
  title becomes an editable label. A topic with no stem, such as one on a
  board a downstream project already runs, keeps working with its label as
  the key. Rejected: a server-minted topic number (the lead must query to learn
  it every time); prefix or substring matching on the title (breaks when the
  title changes).
- **D9. Stem syntax: `wb:topic` gains an optional `label` attribute** (664).
  `<wb:topic name="ship-gate" label="0.48.16 출시 게이트">` creates or
  addresses the topic keyed `ship-gate` and sets its label; later groups use
  `<wb:topic name="ship-gate">` alone, and a group with `label` on an existing
  stemmed topic renames the label. With `label` present, `name` must be a stem
  (`^[a-z0-9][a-z0-9-]{0,39}$`). A labelled group may hold no commands
  (written self-closing, `<wb:topic name="s" label="..."/>`) so a rename or
  adoption can be sent on its own; an unlabelled group still needs at least
  one command.
  - Adoption: a `label` group whose stem is unknown but whose `label` exactly
    equals a stemless topic's key gives that topic the stem (its items, body,
    close state, and close result move to the new key).
  - Uniqueness: stems and labels share one namespace of topic names. A stem
    equal to another topic's stem, label, or stemless key is refused; a
    label equal to another topic's label, stem, or stemless key is refused
    (adoption, where the label equals the adopted topic's own key, is the one
    exception). A stemless topic whose own key already fits the stem pattern,
    addressed with that key as `name` plus a `label`, takes that key as its
    stem and the label as its label (adoption in place; no key change).
  - An empty labelled group must rename an existing stemmed topic or adopt a
    stemless one; one that would create a new topic with nothing in it is
    refused, since a topic with no items and no body does not persist.
  - Resolution: one resolver serves every topic-addressing site (the
    `wb:topic` group for all commands inside it, and the query `topic`
    filter): a name matches a stem first, then a stemless topic's key, then
    a stemmed topic's label (666, user), so references that still use an
    adopted topic's old title keep working. Uniqueness keeps every match
    single. Rejected: refusing a stemmed topic's label with a hint naming the
    stem (one more round trip, and uniqueness already rules out the silent
    duplicate).
  - Grammar: the shared command parser (`internal/wsxmlcmd`) today allows one
    group attribute and refuses an empty group. Extend it additively, as an
    opt-in per group declaration (an optional extra attribute, and empty
    groups allowed when that attribute is present), so the tracker's `tr:in`
    grammar is unchanged.
  - Storage: labels live in a new `State` field mapping stem to label, not on
    `TopicDoc` (which means "has a body"). The entry is dropped when its
    topic disappears (no items and no body left, including after archive
    eviction of its last items), so a stale stem never blocks a new topic.
  - Output: the page shows the label; `whiteboard.query` prints
    `topic ship-gate "<label>" [...]` for a stemmed topic and
    `topic "<label>" [...]` for a stemless one.
  Rejected: a separate `wb:rename`/`stem` command (more surface for the same
  effect); stems on every topic by migration (breaks the user's fallback
  requirement for existing boards); duplicate labels (a label lookup would be
  ambiguous).
- **D10. The topic note pen and other page-to-lead lines name a topic by its
  key** (665): the stem when present, else the label, so the lead can paste
  it into `wb:topic name` or `topic` directly. The page template puts the key
  in the topic's `data-topic` attribute and the label in its visible name
  (`.tname`); both senders that build `[<topic>] <text>` lines and draft keys
  from `data-topic` (`internal/wswhiteboard/assets/form.js` and
  `internal/wsdashboard/assets/dashboard.js` topicNote/topicNotes, saved
  drafts, board-swap matching) then emit the key with no logic change.

## Constraints

- Ships after 0.48.16: implement now on a worktree branch; the lead merges it
  into `develop` only after the 0.48.16 release is published.
- Store compatibility: a store written before this change loads unchanged and
  every existing topic is stemless; no on-disk migration step.
- The whiteboard playbook ships in three byte-identical copies:
  `agents-plugin/rsrc/whiteboard/whiteboard.md`,
  `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`,
  `agents-plugin-pi/rsrc/whiteboard/whiteboard.md`. Update its Commands and
  Query sections for D1-D10 and keep the copies identical.
- Manuals to read before editing: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md` (playbook text),
  `ai-docs/manuals/wsflow-mirroring.md`, `ai-docs/manuals/ws-mcp.md`.
- Never run go build/test/vet locally (it freezes the user's machine): use
  `~/.local/bin/ws-rgo -C <checkout>/agents-plugin-tool -m 30 -- <go args>`.
  Never touch the user's dashboard daemons (ports 29417 / 45743).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 261110-feat-ws-whiteboard-query-depth-and-fields (2026-11-10, Decisions): "`topic` and `ids` queries keep today's output by default (user ok, (624)). Depth `full`, every status, and with `topic` the topic body first, so existing callers ... lose nothing." — bearing: reversed by D2 (user, (658))
- 261110-feat-ws-whiteboard-query-depth-and-fields (2026-11-10, Decisions): "The result is a three-level tree: topic -> item -> item details, with `depth`, `fields`, and `body` controls (user, (622) a)." — bearing: supports
- cca49a60 (2026-11-10, commit): "the topic body keeps its "topic body:" + <<< >>> markers after the topic line so wb:edit quoting is unchanged." — bearing: constrains
- cca49a60 (2026-11-10, commit): "Manifest hashes were set to sha256sum of the new playbook (the manifest format) since remote regen does not return files; the wsrsrc drift tests and the Pi version-check test pass." — bearing: constrains
- 668a0622 (2026-11-10, commit): "User (board 656): option a, lead-chosen topic stem with the title as an editable label; topics without a stem (existing downstream boards) keep using the label as the key." — bearing: supports
- b60b9720 (2026-11-10, commit): "User (board 655): treat today's full output as max verbosity and fill the missing middle level driven by what a lead needs to read, rather than trimming noise wholesale; work scheduled after the 0.48.16 ship." — bearing: supports
- 6fa5e857 (2026-11-09, commit): "User decision (whiteboard (35) a): wb:close withdraws the topic's open and assumed items; a reopen does not restore them." — bearing: constrains
- d39b84d6 (2026-11-08, commit): "Whiteboard: wb:add and wb:patch fields (title, option, recommend, why, link) decode. The topic body (raw HTML), wb:edit old/new (exact-match operands), and the wb:answer body ... stay verbatim." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wswhiteboard/query.go, store.go, commands.go, assets/form.js, page template, agents-plugin-tool/internal/wsxmlcmd, agents-plugin-tool/internal/wsdashboard/assets/dashboard.js (data-topic consumers, no logic change expected), agents-plugin-tool/internal/mcp/whiteboard_tools.go, three rsrc/whiteboard/whiteboard.md copies plus their rsrc/manifest.json hashes |
| scope.surface | public-interface | whiteboard.query payload and output format, wb:topic command syntax, shipped playbook Commands and Query sections |
| scope.new_public_symbol | yes | depth value summary, wb:topic label attribute, fields value answered_at, match: and answer_quote: same as output lines |
| scope.new_type_contract | yes | topic stem and label in the stored State; today only body topics have a TopicDoc and items key by topic name string, store.go#L198-L242 |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/query_test.go, store_test.go, commands_test.go, agents-plugin-tool/internal/mcp/whiteboard_tools_test.go |
| complexity.reuse_points | confirmed | writeDetails query.go#L330-L357, text matcher query.go#L254-L268, inline tag allowlist inline_tags.go#L13-L25, topic note line form.js#L120-L126 |
| complexity.side_effect_risk | moderate | stem adoption moves items, body, close state, and close result to a new key in the persisted store, and the page-to-lead note lines change |
| risk.correctness | moderate | stem/label resolution order, adoption, collision refusal, and legacy stemless fallback all touch topic addressing used by every command |
| risk.fit | moderate | new stored topic contract (stems) and changed query output on a shipped tool; ids/topic defaults change to summary (D2, reversing (624)) |
| risk.test | moderate | many independent decisions each need a dedicated query or store test, plus pre-change store loading and three-copy playbook hash parity |
| risk.security_or_contract | moderate | default output of an existing MCP tool changes for current callers and the on-disk store gains fields that must stay backward compatible |

## Phases

### Phase 1: summary depth, query output fixes, and topic stems

Implement D1-D10 in `internal/wswhiteboard` (query, store, commands, render)
and the MCP layer (`internal/mcp/whiteboard_tools.go`), the shared command
parser `internal/wsxmlcmd` (additive group grammar, D9), the page template and
senders (D10), and the playbook copies with their hashes in the three
`rsrc/manifest.json` files.

Verification:
- Query tests per decision: a settled policy item at `summary` prints the
  decoded option text and note but no option list; an open item at `summary`
  matches `full`; a free-text answer prints as stored; an assumed item shows
  `(assumed)`; a withdrawn item prints only its line; `ids`/`topic` default
  to `summary` and explicit `full` matches today's output; a grouped quote
  prints once; `fields: ["answer"]` omits `answered_at` while `full` with
  `fields` omitted prints it; a multi-line rationale indents every line; a
  `text` match names its fields; `<code>`/`<b>` in titles render as plain
  text while the topic body and `answer_quote` stay verbatim.
- Store/command tests: create a stemmed topic, address it by stem, rename its
  label with a self-closing labelled group, adopt a stemless topic by label,
  adopt a stemless topic in place by its own stem-shaped key, refuse an
  empty labelled group that would create a topic, refuse a stem or label
  collision and a malformed stem with `label`, drop a
  label entry when its topic disappears, address a stemmed topic by its
  label (666), print the
  stemmed and stemless query header forms, and load a pre-change store with
  every topic working by label. The tracker `tr:in` grammar tests pass
  unchanged.
- Page tests: `data-topic` carries the key and the visible name the label;
  a topic-note line from the page names the key.
- Remote `go test ./... -count=1` and `go vet ./...` pass; the three playbook
  copies are sha256-identical and each `rsrc/manifest.json` pins the new hash.

### Result (8b60d2113) - 2026-11-10

Landed in 39fdc7ef6 (code), 82b0ca9a4 (playbook), 8b60d2113 (review fix).

- D1-D7: `internal/wswhiteboard/query.go` adds `DepthSummary`
  (`writeSummary`/`decodedAnswer`), the `answered_at` field, grouped-quote
  folding (`same as (nn)`), `writeIndented` for continuation lines,
  `[match: <fields>]` on text matches, and `Plain` (in `inline_tags.go`) for
  item fields, labels, and close results; body and `answer_quote` print as
  stored. `ids`/`id` and `topic` default to `summary`.
- D8/D9: `wsxmlcmd.Group.Extra` is an opt-in optional group attribute; a
  group carrying it comes out of `Parse` as a Pos-0 command
  (`Command.IsGroup`) ahead of its commands and may be empty. The whiteboard
  grammar sets `Extra: "label"`. New `internal/wswhiteboard/topics.go`
  holds `State.Labels` (stem -> label), `resolveTopic` (stem, then stemless
  key, then label; shared by every group and the query `topic` filter),
  `applyLabel` (rename, in-place adoption, adoption by label, new topic;
  collision and empty-create refusals), `renameTopic`, and `pruneLabels`
  (end of every accepted batch, after archive eviction). Query headers print
  `topic <stem> "<label>"` / `topic "<label>"`.
- D10: `TopicView.Label`; the template keeps the key in `data-topic` (section
  and topic note) and shows the label in `.tname`. Senders are unchanged.
- Playbook Commands/Query/Answers sections updated in all three
  byte-identical copies; manifests pin sha256 257ddf0a....

Verification: remote `go test ./... -count=1` passes; `go vet ./...` clean;
`python3 -m unittest discover agents-plugin-wsflow/tests` OK (20 tests);
`node --test test/version-check.test.ts` in agents-plugin-pi passes. New
tests: `TestQuerySummary`, `TestQueryFullOutputShape`,
`TestQueryTextMatchFields`, `TestPlain`, `topics_test.go` (create/address/
rename, adoption by label and in place, stale close state, refusals,
label pruning incl. archive eviction, query headers and label addressing,
pre-stem store load), `TestStemmedTopicPageNamesItsKey`,
`TestGroupExtraAttribute`; existing query/MCP tests updated for the summary
default.

Decisions:
- D5 continuation lines indent to the line's own indentation (4 spaces for
  detail lines, 2 for the item line), per the ticket wording.
- `rail.js` now shows the `.tname` label rather than `data-topic`, since
  `data-topic` carries a stemmed topic's key; a display fix, not a sender
  change.
- `Plain` replaces tags one by one without pairing them; a stray `</code>`
  becomes a backtick, which is harmless in text output.
- Lite review minors left as is: a group that adopts a topic and edits its
  body in the same batch records no body change marks (base keys the body
  under the old title; page highlight only), and dashboard topic-note chips
  show the stem for a stemmed topic (D10 as specified).

## Sage Review Round 1 (2026-11-10)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Topic addressing breaks when a name or label equals a stemmed topic's label | important | missing |
| 2 | A label rename or adoption cannot be written by itself | important | autonomous |
| 3 | D7 needs to say that the topic body and answer_quote stay verbatim | minor | autonomous |
| 4 | The dashboard sends topic-note lines too, so changing form.js alone misses D10 | minor | autonomous |
| 5 | Where the label is stored and when it is removed is not stated | minor | autonomous |

### Completeness Reviewer — concern

| # | Title | Severity |
|---|-------|----------|
| 1 | D7 does not say whether the topic body is rendered as plain text | important |
| 2 | Label uniqueness and label-collision behavior unspecified | important |
| 3 | Label resolution differs between query and commands | minor |
| 4 | Other topic-addressing commands not covered | minor |
| 5 | summary depth with the topic body is unspecified | minor |
| 6 | D10 is open-ended and has no verification | minor |
| 7 | D1 decoded-answer wording is ambiguous | minor |
