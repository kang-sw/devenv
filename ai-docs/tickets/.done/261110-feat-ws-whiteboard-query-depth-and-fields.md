---
title: "whiteboard.query returns a topic -> item -> detail tree with controllable depth and content"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 07719ec8ac5a6ec0
sage-review-completeness-reviewed: 07719ec8ac5a6ec0
completed: 2026-11-10
---

# whiteboard.query: depth and content control

## Background

`whiteboard.query` (`handleWhiteboardQuery`,
`agents-plugin-tool/internal/mcp/whiteboard_tools.go`; `Query` and
`FormatItems`, `agents-plugin-tool/internal/wswhiteboard/store.go`) filters
by exact `topic`, `status`, and `id`, with no limit. An unfiltered call
returns every item, settled and archived ones included, with every detail:
options, recommendation, rationale, answer, the verbatim answer quote, the
answer time, and links. The lead session that raised this has a board of 222
items, about 66k characters per unfiltered query, and the whiteboard
playbook tells a lead to query before adding. The user asked for a more
deliberate API whose depth and content the caller controls, organised by
topic and open question (whiteboard, topic "whiteboard.query 반환 크기",
(618) note, then (622)-(627)).

## Decisions

1. **The result is a three-level tree: topic -> item -> item details, with
   `depth`, `fields`, and `body` controls (user, (622) a).** The payload
   becomes `{topic?, ids?, id?, status?, text?, depth?, fields?, body?,
   closed?}`.
   - `depth`: `topics` prints one line per topic: name, open or closed,
     item counts by status, body length in characters, last change time
     (the latest `ChangedAt` of its items or body, RFC 3339 UTC), and the close
     result when closed. `items` adds one line per item under its topic:
     ID, status, class, title. `full` adds the item's detail lines.
   - `fields`: at depth `full`, the detail fields to print, from `options`,
     `recommendation`, `rationale`, `answer`, `answer_quote`, `links`
     (`answered_at` travels with `answer`); omitted means all.
   - `body: true` prints topic bodies.
   - `closed: true` lists closed topics as topic lines in place of the one
     fold line; their items follow the same depth and status rules as any
     topic's. `topic`, `ids`, and `text` include closed topics without it,
     since they name or search items explicitly.
   - Combination: filters AND together; `id` and `ids` union. The per-filter
     defaults of Decisions 2-4 come from the most specific target present
     (`ids` or `id`, then `topic`, then `text`, then none), and an explicit
     `depth`, `status`, `body`, or `closed` always overrides the default. So
     a call carrying only `status`, `depth`, `fields`, or `closed` takes the
     unfiltered defaults of Decision 2 with its explicit values applied
     (`{status: "answered"}` prints answered items at depth `items`; add
     `depth: "full"` for their details), and `topic` with `text` takes the
     `topic` defaults with the text filter ANDed in. The playbook `## Query`
     section states this rule and the store tests cover it.
   - The payload stays one JSON object of filters, as
     `261107-feat-ws-whiteboard-xml-commands-topic-content` settled; this
     extends that object and does not move query to a command string. The
     advertised MCP schema (`payload` as an opaque object defined by the
     playbook) does not change.
   Rejected: `depth` and `body` without `fields` (b); only changing the
   per-filter defaults with no depth control (c).
2. **An unfiltered call defaults to depth `items`, items with status
   `open` only (answered, assumed, withdrawn, and superseded are settled), and
   closed topics folded into one count line (user ok, (623)).** The
   playbook's "query before adding" needs the open questions and a map of
   topics; settled details are fetched by `topic` or `ids`. `assumed` is
   excluded with the settled statuses even though the store does not count
   it as settled (store.go `IsSettled`), because the page already shows it
   among settled items and the user asked for open items only; the omission
   line counts it by name, so `status: "assumed"` finds it.
3. **`topic` and `ids` queries keep today's output by default (user ok,
   (624)).** Depth `full`, every status, and with `topic` the topic body
   first, so existing callers (the Open Decision Queue reconciliation, body
   quoting for `wb:edit`) lose nothing. "Today's output" means the same
   content, not the same line format: item lines move under their topic
   line in the Decision 1 tree and drop the per-line topic, so the
   `whiteboard_tools_test.go` assertion that pins the flat
   `(01) [answered] [technical] t: settle me` line for an `id` query changes
   with the unfiltered-query assertion.
4. **`text` is a text search (user ok, (625); field named `text`, not
   `q`).** Case-insensitive substring
   over item title, options, rationale, and answer; matches print at depth
   `items` across every status. It lets a lead check whether a question was
   already answered while the unfiltered default hides settled items, with
   the same meaning as `tracker.query`'s `text`. The board item proposed the
   name `q`; it is `text` because `tracker.query`'s `q` is a structured
   search grammar, and one field name meaning two things across sibling tools
   misleads a caller (auto-confirmed from the fact populator's gap).
5. **The result's last line names what was left out and the filter that
   shows it (user ok, (626)).** For example the number of settled items and
   closed topics omitted, pointing at `topic`, `ids`, `status`, or
   `closed: true`, so an omission is never read as absence.
6. **`topic` and `status` take one string or a list; `id` stays and `ids`
   takes a list (user ok, (627)).** Existing single-value calls keep
   working.
7. **The lead-ticket queue reconciliation queries the queue's IDs.** The
   shipped line telling a lead to query the board and add every open queue
   item the query does not return (`lineTicketQueueBoard`,
   `agents-plugin-tool/internal/mcp/whiteboard_text.go`) changes to query
   with `ids` set to the queue's item IDs, because the new unfiltered
   default omits settled items and an item settled on the board but still
   open in the ticket would otherwise be re-added and refused as an ID the
   store holds. (Direct consequence of Decision 2.) The rendered-text test
   that pins the line's phrase "the query does not return" changes with
   the new wording if the phrase does not survive.

## Constraints

- The playbook `## Query` section (`agents-plugin/rsrc/whiteboard/whiteboard.md`)
  is the grammar source: the query refusal prints it
  (`whiteboardRefusal`), so it states the new payload, the depth levels,
  the fields, and the per-filter defaults. Its wsflow mirror
  (`agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`) changes with it.
- Everything else that states or pins the query behavior changes with it
  (auto-confirmed from the fact populator's gap):
  - the playbook's call description (`whiteboard.md` "returns stored items,
    settled ones included"), which stops being true for an unfiltered call;
  - the Pi byte mirror `agents-plugin-pi/rsrc/whiteboard/whiteboard.md`,
    which `agents-plugin-pi/test/version-check.test.ts` holds byte-identical
    to the source;
  - the `whiteboard/whiteboard.md` hash in the ws, wsflow, and Pi rsrc
    `manifest.json` files, regenerated per
    `ai-docs/manuals/wsflow-mirroring.md`;
  - the mcp test asserting an unfiltered query lists an answered item
    (`whiteboard_tools_test.go`), rewritten for Decision 2, and the refusal
    test that expects the literal `whiteboard.query payload:` grammar line,
    which the new grammar line keeps.
- The `runtime.json` per-tool ranges do not move: the launcher checks tools
  by name only.
- Unknown payload fields, an unknown `depth` or `fields` value, and a
  non-string non-list `topic` or `status` are refused with the problems
  listed, as today's `checkKnownFields` and enum checks do.
- Output stays compact text (the ws-mcp tool output convention); no JSON
  format is added.
- Never run go build/test/vet on the lead machine; run them remotely:
  `~/.local/bin/ws-rgo -C <checkout>/agents-plugin-tool -m 30 -- test
  <packages> -count=1 -timeout 30m`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for
  agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for
  agents-plugin/rsrc/, agents-plugin-wsflow/rsrc/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for
  agents-plugin/rsrc/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for
  agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 261107-feat-ws-whiteboard-xml-commands-topic-content (2026-11-07, Decisions): "`whiteboard.query` keeps its current short filter payload unchanged (user, chat (218) a: the whiteboard is the only tool with ..." — bearing: constrains
- c87afb80a (2026-11-07, 261107-feat-ws-whiteboard-xml-commands-topic-content Result): "`whiteboard.query` keeps its payload; with `topic` it also prints that topic's stored body, so a lead can quote it in `wb:edit`." — bearing: supports
- ab9ccd98 (2026-11-07, commit): "runtime.json per-tool ranges were not bumped: they are checked by name only, and the ticket did not ask for it." — bearing: supports
- 420c93d5 (2026-11-07, commit): "the variant lines in whiteboard_text.go are a second shipped surface carrying the old grammar's vocabulary, outside the playbook. Tests that pin the variant text move with it." — bearing: constrains
- f98d8e83 (2026-11-04, commit): "The inner payload schema has one source: refusals quote the rendered whiteboard playbook's ## Payload section instead of a Go copy, so schema text cannot drift from the playbook." — bearing: constrains
- f98d8e83 (2026-11-04, commit): "Store lives in its own pure package internal/wswhiteboard (Apply/Query/BuildView/RenderPage with an injected Env) so batch semantics are unit-tested without the MCP server" — bearing: constrains
- d17abbea (2026-11-04, commit): "The rule ... is removed from the canonical playbook, its wsflow mirror, and the Pi rsrc copy. The manifest hash for lead-discuss.md is regenerated in all three rsrc trees." — bearing: constrains
- 260910-feat-ws-agent-list-bounded-output (2026-09-10, Decisions): "The response should state how many older dormant records were hidden and direct callers to an explicit broader query." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wswhiteboard/store.go, agents-plugin-tool/internal/mcp/whiteboard_tools.go, agents-plugin-tool/internal/mcp/whiteboard_text.go, agents-plugin/rsrc/whiteboard/whiteboard.md and its wsflow and Pi copies, the three rsrc manifest.json hashes |
| scope.surface | public-interface | whiteboard.query payload grammar and default output, the shipped lineTicketQueueBoard text |
| scope.new_public_symbol | no | no new tool; Query and FormatItems in wswhiteboard change in place |
| scope.new_type_contract | yes | whiteboard.query payload {topic?, ids?, id?, status?, text?, depth?, fields?, body?, closed?} and the topic-item-detail output tree with an omission line |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/store_test.go#L643-L712, agents-plugin-tool/internal/mcp/whiteboard_tools_test.go#L578-L616 and L969, L1199 |
| complexity.reuse_points | confirmed | checkKnownFields and requiredEnum store.go#L1148, whiteboardRefusal whiteboard_tools.go#L307-L323, State.TopicBody, State.ClosedTopics and CloseResults store.go#L231-L238 |
| complexity.side_effect_risk | moderate | read-only tool, but the unfiltered default changes what shipped playbook lines and queue reconciliation see |
| risk.correctness | moderate | per-filter default matrix, closed-topic folding, and omission-line counts must agree across depth, status, text, ids, and topic combinations |
| risk.fit | moderate | whiteboard.md#L27-L29 states settled items are returned and changes with Decision 2; the Pi byte mirror and three rsrc manifests change with the playbook |
| risk.test | moderate | whiteboard_tools_test.go#L608-L610 asserts an unfiltered query lists an answered item, which Decision 2 reverses; Pi byte-mirror and manifest-hash tests must be regenerated |
| risk.security_or_contract | moderate | shipped tool output contract changes for existing unfiltered callers; runtime.json tool pins are checked by name only, agents-plugin/bin/ws-mcp-launcher.py#L200-L201 |

## Phases

### Phase 1: tree query

Implement Decisions 1-7: `Query` and its formatter in `wswhiteboard`, the
handler, the playbook `## Query` section and call description with their
wsflow and Pi mirrors and the rsrc manifest hashes, and the lead-ticket
queue line.

Verification:

- Store tests for each depth, `fields` selection, `body`, `closed`, `text`,
  list-valued `topic` and `status`, `ids`, and the omission line.
- The unfiltered default prints only open items under their topics and
  folds closed topics into one line; `topic` and `ids` queries print what
  they print today; `closed: true` and the combination rule (AND, explicit
  overrides) behave as Decision 1 states.
- A single-string `topic` or `status` is still accepted (Decision 6), and
  a non-string non-list one is refused.
- `lineTicketQueueBoard` names the `ids` query (Decision 7), and the Pi
  byte-mirror and rsrc manifest-hash tests pass.
- Refusals for an unknown field, depth, or field name list the problems and
  the playbook grammar.
- The mcp whiteboard tool tests and the rendered-text tests that pin
  `lineTicketQueueBoard` pass remotely through `ws-rgo`.

### Result (cca49a60a) - 2026-11-10

Landed Decisions 1-7. `Query` and its formatter (now `FormatQuery`) moved
to `agents-plugin-tool/internal/wswhiteboard/query.go` with the payload
value checks, building a `QueryResult` topic -> item -> detail tree; the
handler prints it; the playbook `## Query` section (with a per-target
defaults table) and call description changed, with byte-identical wsflow
and Pi mirrors and the three rsrc manifest hashes; `lineTicketQueueBoard`
now queries with `ids` set to the queue's item IDs.

Verification (remote, `ws-rgo`): `go test ./internal/wswhiteboard/...
-count=1` ok; `go test ./internal/mcp/... ./internal/wsrsrc/... -count=1`
ok; local `node --test test/version-check.test.ts` in `agents-plugin-pi`
8/8 pass. New `query_test.go` covers each depth, `fields`, `body`,
`closed`, `text`, list-valued `topic`/`status`, `id`/`ids` union, the
omission line, explicit overrides, and refusals.

Decisions taken:
- The closed-topic fold count and the omission line are one last
  `omitted:` line, so the fold count is not printed twice.
- Omission counts cover only default-driven omissions (the unfiltered
  open-only status at depth `items`/`full`, and folded closed topics).
- `ids` and `text` list only topics holding a match; `topic` and
  unfiltered calls list every topic they reach (the unfiltered call is a
  topic map). Topics print in first-item-added order, body-only topics
  after.
- Empty strings and empty lists are refused for `topic`, `status`,
  `fields`, `ids`, and `text`; `fields` also takes a single string.
- `id: 0` now filters for item 0 (no match) instead of meaning no filter,
  as the old `id != 0` check did (lite review, Minor; IDs start at 1).
