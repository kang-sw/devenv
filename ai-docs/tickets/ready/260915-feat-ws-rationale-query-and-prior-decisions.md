---
title: rationale.query tool and the ticket `## Prior Decisions` section
related:
  260909-epic-ws-worker-interpreter-refoundation: source of the numbered epic Decisions cited below (7, 14, 16)
  260909-research-ws-refoundation-evidence-audit: binding anchor; verdict A4 named the design-review contract check as the one spec consumption point that changed an agent's action
  260909-refactor-retire-spec-mental-model-layers: retired the write-side layer this ticket replaces on the read side
  260728-feat-lead-backfill-docs-entry-skill: prior art for post-landing documentation, retired; its "candidates, not verdicts" rule is reused here
  260915-refactor-lead-run-playbook-diet-drop-assignment-note: validation case; it reversed epic Decision 16 (`session.note`) and declared the reversal
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 87fe282dd016a6d2
sage-review-completeness-reviewed: 87fe282dd016a6d2
---

# rationale.query tool and the ticket `## Prior Decisions` section

## Background

The refoundation retired the spec and mental-model layers and declared tests
the behavioral contract, with rationale living in commit `## AI Context` bodies
and ticket decision sections. Bootstrapping downstream projects showed the gap
that left: there is no place that answers "what did this project already decide
about this subsystem, and why" without a hand-run history dig. A fresh agent
cannot query for a decision it does not know exists, and a reviewer has no
anchor for "does this ticket quietly reverse a recorded decision".

The evidence says the corpus is there and the missing piece is retrieval, not
authorship. Measured on this repository on 2026-09-15:

| corpus | count |
|---|---|
| commits reachable from HEAD | 6,559 |
| commits carrying `## AI Context` | 6,145 (94%) |
| commits whose body names a ticket stem (regex below) | 3,405 (52%) |
| `.done/` tickets with `## Decisions` / `### Result` | 347 / 430 |
| `git log` of the whole history with bodies and touched paths | 0.4 s |
| `git log -L` on one function's line range | 0.05 s |
| `git log -S` over one subtree | 0.2 s |

A maintained document was rejected (see Decisions): the retirement ticket's
triage found 175 of 228 spec anchors derivable from code and 53 stale, and the
truth criterion says a layer survives only when reconstructing its value on
demand costs more than maintaining it. This ticket lowers the reconstruction
cost instead: one read-only MCP tool over the sources that never rot, consumed
at the two points where the answer changes an action.

## Decisions

1. **Read-side, not write-side.** Ship a search tool over commit rationale and
   ticket decision sections. No new tracked document, no directory, no anchor
   syntax, no frontmatter key, no promotion gate. Rejected: a per-subsystem
   design document under `ai-docs/manuals/` (every manual's summary is injected
   into every lead `workflow_manual`, so per-subsystem files spam the ambient
   block) or `ai-docs/design/` (spec under a new name; the drift base rate above
   applies). Rejected: a hash-stamped untracked digest generator; it derives
   from code and cannot recover intent.
2. **Lexical ranking, no embeddings.** BM25 over short records, standard
   library only. Path and site addressing are orthogonal to vocabulary and
   carry the cold-start case. Embeddings are reconsidered only if Phase 3
   measures a miss rate that path and site addressing do not cover.
3. **Three addressing modes in one tool.** `paths` (file globs), `site` (a line
   range located by regex or `#L` reference, resolved through `git log -L`),
   and `pickaxe` (`git log -S`). All three produce the same record model and
   output shape.
4. **Records are pointers plus quoted text, never summaries.** Epic Decision 7.
   The reader follows the hash or stem to verify.
5. **The fact populator is the first consumer.** It already verifies the ticket
   against the tree and other tickets and edits exactly one file; recorded
   decisions are one more thing to verify against. It writes a bounded
   `## Prior Decisions` section into the ticket body, so the sage stamp digests
   it, the design reviewer reads it without spawning anything, and the worker
   receives it as part of the ticket. Rejected: the design reviewer calling the
   tool itself (its context must stay fresh and dense). Rejected: `lead-ticket`
   spawning a discovery delegate at authoring time (adds a spawn and lead
   prose; the populator's report already reaches the lead before the gate).
   Deferred: a standalone `rationale-discovery` delegate for on-demand use.
6. **The design reviewer gets one new checklist item, with one shape of
   finding.** "Unacknowledged reversal": the ticket reverses a verified,
   still-current recorded decision without naming it. A named reversal
   (`supersedes <hash or stem>: <reason>`) is never a finding; that line is
   how supersession gets recorded and becomes searchable. Rejected: a general
   "alignment with project rationale" judgment, which would let stale decisions
   block legitimate changes of direction.
7. **A contradiction is a design choice, not a factual claim.** The populator
   lists it and counts it; it never rewrites the ticket's plan over it. This
   keeps the populator on facts and pointers and the reviewer on judgment.
8. **No cache in v1.** A full scan costs 0.4 s here. A newest-20,000-commit
   scan cap bounds large repositories and reports truncation. A cache is a
   follow-up if a downstream repository measures above 2 s.
9. **The worker playbook is untouched.** It reads the ticket, so it receives
   `## Prior Decisions` without a rule change. A worker-side `site:` habit is a
   follow-up candidate, not this ticket.

## Constraints

- Convention: `ai-docs/manuals/ws-mcp.md` (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: `ai-docs/manuals/skill-authoring.md` (declared for `agents-plugin/rsrc/`, `agents-plugin-wsflow/rsrc/`)
- Convention: `ai-docs/manuals/wsflow-mirroring.md` (declared for `agents-plugin/rsrc/`, `agents-plugin-wsflow/`)
- Convention: `ai-docs/manuals/shipped-surface-boundary.md` (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Shipped text (tool description, playbook prose) names no ticket, hash, or
  layout of this repository. The validation cases in Phase 3 stay in this
  ticket and in test fixtures only.
- The tool is read-only: it never writes to the tree, never creates a cache
  directory in v1, and requires `session_key` like `git.log`.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go`
  `TestPlaybookPrintRetiredSpecStemsGone` pins seven retired stems; do not
  reuse `lead-backfill-docs` or `doc-gap-discovery` for anything.
- The `## Prior Decisions` section is bounded: at most 8 threads, at most 200
  characters per quote. This bound is the worker's added mandated read.
- Closed inventory is immutable (epic Decision 14): Phase 3 runs the tool
  against `.done/` tickets; it never re-populates or edits them.
- The `## Route Facts` reader parses that section only; an extra section before
  it is tolerated by construction. Phase 2 verification confirms this with the
  existing route tests.
- Phase 1's `agents-plugin-pi/runtime.json` edit and Phase 2's hand copy into
  `agents-plugin-pi/rsrc/` each land either before a pi tag is cut or after
  its acceptance run
  (260914-chore-ws-pi-release-path-acceptance-and-docs), never between them.
- All ticket content in English.

## Prior Art

- `git.log` in `agents-plugin-tool/internal/wsgit/git.go` (`LogArgs`, `ParseLog`)
  and its dispatch, schema, formatter, and `toolSchemaRequiresSessionKey`
  entry in `agents-plugin-tool/internal/mcp/server.go`: copy this shape for
  dispatch, root resolution, and compact-text-by-default output.
- `tickets.query` in `agents-plugin-tool/internal/wsdoc/`: reuse its ticket
  directory walk and frontmatter parsing.
- `ticket-fact-populator.md` step 3 Constraints bullet: the existing pattern
  of "write one derived section into the ticket, replacing it whole".
- `tickets.query(mentions_ticket_stem)` itself carries no stem-extraction
  regex to reuse: it validates a caller-given stem against
  `^\d{6}-[\w-]+$` and then matches a ticket body by plain substring
  `strings.Contains`, not by scanning free text for stems
  (agents-plugin-tool/internal/wsdoc/tickets.go#L12,109-114,141-144). The stem
  regex below is new to this tool.

## Tool Specification

Everything in this section is the contract. Implement it as written; where the
implementation must deviate, record the deviation in the Phase 1 Result.

### Name, description, schema

Name: `rationale.query`. Read-only. Requires `session_key` (add it to the
`toolSchemaRequiresSessionKey` list next to `git.log`).

Description string, verbatim:

```
Search recorded rationale: commit ## AI Context and ## Ticket Updates bullets and ticket decision sections. Address by path globs, a code site (git log -L), a pickaxe string (git log -S), or a text query. Returns pointers with quoted records grouped by ticket thread, newest first. Defaults to compact text; use format=json for structured output.
```

Input schema (all optional unless stated):

| property | type | meaning |
|---|---|---|
| `query` | string | BM25 text query over record text. Combines with any addressing mode. |
| `paths` | string[] | File globs. A commit record matches when any touched path matches; a ticket record matches when the ticket names a matching path or a commit in its thread touched one. |
| `site` | string | `<path>:/<regex>/,+N` or `<path>#L<start>-L<end>`. Exclusive with `paths` and `pickaxe`. `+N` defaults to `+10`. |
| `occurrence` | integer | Which regex match in the file `site` uses when it matches more than once. Default 1. |
| `pickaxe` | string | `git log -S<string>`. Exclusive with `paths` and `site`. |
| `since`, `until` | string | `YYYY-MM-DD` inclusive bounds on record date. |
| `stems` | string[] | Keep only records in these ticket threads. |
| `exclude_stem` | string | Drop the ticket's own records and commits that name only this stem. |
| `kinds` | string[] | Subset of `commit`, `ticket`. Default both. |
| `order` | string | `relevance` or `time`. Default `relevance` when `query` is given, else `time`. |
| `limit` | integer | Maximum records returned. Default 30, cap 100. |
| `format` | string | `json` for structured output. |

Glob semantics: `*` matches within one path segment, `**` matches across
segments; a pattern with no glob metacharacter is a prefix match on path
segments (`internal/mcp` matches `internal/mcp/server.go`). Invalid glob,
malformed `site`, a `site` path absent at HEAD, an empty `pickaxe`, a bad date,
or `site`/`pickaxe`/`paths` given together is an error, not an empty result.

### Record model

A record is one bullet or one paragraph of rationale with a pointer.

Commit records come from every commit reachable from HEAD, newest first,
scanned with `git log --date=short --diff-merges=first-parent --name-only` and
a sentinel format carrying hash, date, subject, body. Scan at most 20,000
commits; when history is longer, set `truncated: true` and say so in
`omitted:`. Within a body, the blocks under `## AI Context` and
`## Ticket Updates` (from the heading to the next `## ` heading or end of body)
are split into records: a record starts at a line beginning `- ` and continues
through following lines that start with whitespace or `>`; a blank line or the
next `- ` ends it. A block with no bullets yields one record per
blank-line-separated paragraph. Each commit record carries: `hash`, `date`,
`subject`, `paths` (the commit's touched paths), `stems` (every ticket stem the
body mentions).

Ticket records come from every ticket file under all five status directories,
including `.done/` and `.dropped/`. The sections `## Decisions`,
`## Cross-Child Decisions`, `## Rejected Alternatives`, `## Open Questions`,
`## Resolution`, `### Confirmed Decisions`, and every
`### Result (<hash>) - <date>` are split
into records by the same bullet-or-paragraph rule. Each ticket record carries:
`stem`, `status` (directory name without the dot), `section` (the heading
text), `date` (the Result or Resolution heading's date when present, else the
stem's `YYMMDD` as `20YY-MM-DD`), `paths` (backticked tokens in the ticket body
containing `/` and ending in a file extension, plus `path#L` references, plus
the union of paths touched by commits in the ticket's thread).

Ticket stem regex, new to this tool (not shared with `tickets.query`, which
matches by plain substring — see Prior Art):
`\b\d{6}-[a-z]+-[a-z0-9]+(?:-[a-z0-9]+)*\b`. The category segment is any
lowercase word, not the six authoring categories: closed inventory is immutable
and history names stems in retired categories (`workset`, `todo`, `design`,
`perf`, `test`, `idea`) that commits still mention. Measured on this
repository, the six-category form recognizes 2,920 stem-naming commits (45%)
against 3,405 (52%) for this form; the extra matches are retired-category and
category-less legacy stems.

### Threads

A thread is one ticket stem plus every commit record whose body mentions that
stem. A commit mentioning several stems appears under each. Commit records
mentioning no stem form the `(no ticket)` thread. `limit` counts each record
once.

### Site mode

Parse `site`. For the regex form, read the file at HEAD, count regex matches,
and select the `occurrence`-th; zero matches is an error, more than one with
`occurrence` unset is allowed but reported in `omitted:` as
`site matched K locations; used occurrence 1`. Run
`git log -L <start>,<end>:<path> -w --no-patch` with the sentinel format and
`-n 500`; the returned commits are the whole chain, newest first, and `limit`
applies only to the records built from it. When the chain hits 500, set
`site.truncated: true` (the top-level `truncated` is the scan cap only) and
append `site chain truncated at 500` to `omitted:`; `introduced` is then
unknown and the JSON field is null. Build commit records from the chain commits exactly
as above (their AI Context bullets and stems). Additionally return the chain
summary: `introduced` (oldest chain commit hash and date), `last_changed`
(newest), `changes` (chain length). Renames across files are not followed;
site mode always appends `renames not followed` to `omitted:`.

### Pickaxe mode

Run `git log -S<pickaxe> --diff-merges=first-parent --name-only` with the
sentinel format, bounded by the scan cap, and build commit records as above.

### Ranking

Tokenize lowercase on any non-alphanumeric character, dropping tokens shorter
than 2 characters. A record's document is its text plus the commit subject or
the ticket title. BM25 with `k1 = 1.2`, `b = 0.75`, IDF computed over the whole
scanned corpus (not the filtered subset). Ties, and `order: time`, sort by date
descending, then by hash for stability. Threads are ordered by their best
record; records within a thread are always date descending.

### Output, compact text (default)

```
thread: <stem> (<status>) — <N> records
  <date> ticket <section> — <quoted record, collapsed to one line, cut at 300 characters>
  <date> <short-hash> — <quoted record, collapsed to one line, cut at 300 characters>
  paths: <up to 5 distinct touched paths, then "+K more">
thread: (no ticket) — <N> records
  <date> <short-hash> — <quoted record>
site: introduced <short-hash> <date>; last changed <short-hash> <date>; <N> changes
omitted: scanned <N> commits[; <K> records past limit][; truncated at 20000][; site matched K locations; used occurrence 1][; site chain truncated at 500][; renames not followed]
```

The `site:` line appears only in site mode. The `omitted:` line is always the
last line and always carries the scanned count, so it is never empty.

### Output, `format: json`

```
{
  "threads": [
    { "stem": "<stem>" | null, "status": "<status>" | null, "title": "<ticket title>" | null,
      "records": [
        { "kind": "commit" | "ticket", "pointer": "<full hash>" | "<stem>#<section>",
          "date": "YYYY-MM-DD", "text": "<full record text>", "subject": "<commit subject>" | null,
          "paths": ["..."], "stems": ["..."], "score": <number> }
      ] }
  ],
  "site": { "introduced": {"hash": "...", "date": "..."} | null, "last_changed": {...}, "changes": <n>, "truncated": <bool> } | null,
  "scanned_commits": <n>, "truncated": <bool>, "omitted": ["..."]
}
```

### Tests (Phase 1 acceptance)

Fixture repository built in-test with `git init` and scripted commits, plus a
fixture `ai-docs/tickets/` tree. Pin:

1. Bullet extraction: a body with two AI Context bullets, one with an indented
   continuation line, one `## Ticket Updates` bullet with a `> Forward:` line,
   yields three records with exact text; prose outside those blocks yields none.
2. Paragraph fallback: a `### Result` section with two paragraphs and no
   bullets yields two records.
3. Stem linkage: a commit naming two stems appears under both threads; a commit
   naming none lands in `(no ticket)`.
4. Path filter: `paths: ["internal/mcp"]` matches a commit touching
   `internal/mcp/server.go`, and matches a ticket record whose only path link is
   through such a commit in its thread.
5. `exclude_stem` drops the ticket's own records and commits naming only it.
6. Ranking: with `query`, a record containing the query terms outranks one that
   does not; without `query`, newest first.
7. Site mode: after three commits editing one function, the chain returns them
   newest first with `introduced` and `last_changed` set and `omitted:`
   containing `renames not followed`; a regex with two matches and no
   `occurrence` reports the K-locations line in `omitted:`.
8. Pickaxe mode returns the commit that introduced the string and the one that
   removed it, and nothing else.
9. `limit` cap: `limit: 500` returns at most 100 records and the `omitted:`
   count is exact.
10. Errors: `site` with `paths`, malformed `site`, bad `since`, invalid glob.
11. Output: the compact text ends with an `omitted:` line in every case; the
    JSON shape matches the schema above.
12. `toolSchemaRequiresSessionKey("rationale.query")` is true; the tool is
    listed in `tools/list`.

## Playbook Text

Insert verbatim. Template variables use the playbook's existing namespace
tokens.

### `ticket-fact-populator.md`

In step 3, after the `Under ## Constraints` bullet and its example paragraph,
add this bullet:

```
   - Write the `## Prior Decisions` section (below), replacing it whole if
     present.
```

After the `## Route Facts` section (before `## Constraints`), add this section:

```
## Prior Decisions

A section written exactly `## Prior Decisions`, placed immediately before
`## Route Facts`. It carries the recorded decisions that bear on this ticket's
paths and subject, as pointers into git history and other tickets, so the
reviewer and the worker read them without searching. Fill it from
`{{.McpNamespace}}/rationale.query` with `exclude_stem: <this stem>`:

1. `paths:` every path the ticket names, no `query`, `limit: 30`.
2. `query:` the distinctive terms of the ticket title, no `paths`.
3. `site:` at most four of the `path#Lstart-Lend` references or quoted code
   lines the ticket cites, choosing the ones its unfinished phases edit.

Six calls per ticket at most. Merge the results by pointer. Keep at most 8
threads, preferring threads whose paths overlap the ticket's, then the most
recent. Write one line per thread, the quote collapsed to one line and never
beginning with `#`:

    - <stem or short hash> (<date>, <section or "commit">): "<quoted record, at most 200 characters>" — bearing: <supports|constrains|contradiction-candidate>

`bearing` is your read of the thread's newest record: `supports` when the
ticket builds on it, `constrains` when the ticket must respect it,
`contradiction-candidate` when the ticket's plan or `## Decisions` appears to
reverse it and the ticket does not say so. A contradiction with a recorded
decision is a design choice, not a factual claim: never rewrite the ticket's
plan or decisions over it; list it under `prior_contradictions:` in the
report. When every query returns nothing, write the section with the single
line `- none found (queried <YYYY-MM-DD>)`.
```

In `## Route Facts`, replace the sentence

```
before `## Phases` — or, when the ticket has no phases, before the first `##`
heading after its body prose.
```

with

```
before `## Phases` — or, when the ticket has no phases, before the first `##`
heading after its body prose and its `## Prior Decisions` section.
```

In `## Output`, after the count line `relations: <N>`, add the count line
`prior_contradictions: <N>`, and after the `unverified:` list add:

```
prior_contradictions:
  - pointer: <stem or short hash>
    reverses: <the ticket sentence or decision that appears to reverse it>
```

### `ticket-reviewer-design.md`

At the end of Process step 2, add:

```
   Read the ticket's `## Prior Decisions` section as a third contradiction
   anchor. For each `contradiction-candidate`, open the pointed commit with
   {{.McpNamespace}}/git.log(range: "<hash>^..<hash>", include_body: true), or
   resolve the pointed stem with {{.McpNamespace}}/tickets.query(ticket_stem:
   <stem>, include_done: true, include_dropped: true) and read that ticket;
   confirm the quote and check whether a later record in the same section
   reverses it. Then establish commit-level currency with one bounded explorer
   question (**Autonomous Exploration** below): whether a later commit on the
   quote's paths reverses the decision; the explorer calls
   {{.McpNamespace}}/rationale.query(kinds: ["commit"], paths: <the quote's
   paths>, since: <the quote's date>) and nothing wider; put every candidate
   into that one question rather than one spawn each. A reversed decision
   makes the candidate stale, not a finding. A ticket with no
   `## Prior Decisions` section was populated incompletely: report that in
   `omitted:` and continue.
```

In `## Constraints`, after the bullet beginning "Use host-native explorers",
add:

```
- A pointer in the ticket's `## Prior Decisions` section may be opened at its
  current path whatever its status, including `.done/`, and its commit-level
  currency checked with a commit-only `rationale.query`; this opens exact
  pointers and commits only, never a ticket directory.
```

In `## Checklist`, add item 7:

```
7. **Unacknowledged reversal**: Does the ticket's plan or a `## Decisions`
   entry reverse a verified, still-current recorded decision without naming
   it? Naming it (`supersedes <hash or stem>: <reason>`) is a legitimate change
   of direction and never a finding. An unnamed reversal is `important` with
   `resolution: missing`, which the thresholds below turn into `block`: which
   contract holds is a policy choice the implementer cannot make.
```

In `## Batch review boundary`, in the delta-review paragraph after
"previously passed tickets are accepted baseline context, not fresh targets.",
add:

```
Re-check Checklist item 7 only for `changed_stems`.
```

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | new package `agents-plugin-tool/internal/wsrationale/`; `agents-plugin-tool/internal/mcp/server.go` (dispatch, schema, `toolSchemaRequiresSessionKey`); `agents-plugin/runtime.json`, `agents-plugin-wsflow/runtime.json`, `agents-plugin-pi/runtime.json`; `agents-plugin/rsrc/ticket-fact-populator/ticket-fact-populator.md`; `agents-plugin/rsrc/ticket-reviewer-design/ticket-reviewer-design.md`; `agents-plugin-pi/rsrc/` (hand copy); five pinned test files |
| scope.surface | public-interface | adds MCP tool `rationale.query` to `tools/list` and the three `runtime.json` contracts (agents-plugin-tool/internal/mcp/server.go#L3934-3943 shows the sibling `toolSchemaRequiresSessionKey` list `rationale.query` joins) |
| scope.new_public_symbol | yes | `rationale.query` |
| scope.new_type_contract | yes | new input schema (query/paths/site/occurrence/pickaxe/since/until/stems/exclude_stem/kinds/order/limit/format) and new JSON output shape (threads/records/site/scanned_commits/truncated/omitted) |
| scope.test_surface | new-files | Phase 1 adds twelve acceptance tests in the new `wsrationale` package fixture; Phase 2 updates five existing pinned test files (playbook_tools_test.go, playbook_render_surface_test.go, ticket_review_design_test.go, ticket_batch_review_test.go, agents-plugin/tests/test_skill_dispatch_contracts.py — all confirmed to already reference these two playbooks) |
| complexity.reuse_points | confirmed | `LogArgs`/`ParseLog` in agents-plugin-tool/internal/wsgit/git.go#L363,381; `toolSchemaRequiresSessionKey` entry pattern next to `git.log` in internal/mcp/server.go#L3934-3943; ticket directory walk and frontmatter parsing in internal/wsdoc/tickets.go (TicketsFind, scanTicketsWithBodies) |
| complexity.side_effect_risk | moderate | Phase 1's tool is read-only by Constraints, but Phase 2 changes what `ticket-fact-populator` writes into every future ticket and adds design-reviewer Checklist item 7, which can produce an `important` finding that blocks ready promotion — a workflow-wide behavior change, not a contained code change |
| risk.correctness | moderate | the stem regex decision is settled (generic category segment, measured 3,405 vs 2,920 commits); residual risk sits in the bullet-or-paragraph extraction rule on free-form commit bodies and in site mode, whose `git log -L` chain cannot distinguish a rename from a creation and whose regex can match several locations (Tool Specification > Record model, Site mode; twelve acceptance tests pin these) |
| risk.fit | low | Decisions 1-9 and the epic's already-`.done` 260909-refactor-retire-spec-mental-model-layers establish the read-side, no-new-document direction this ticket follows; Prior Art reuses git.log and tickets.query shapes directly |
| risk.test | moderate | twelve new acceptance tests plus five existing pinned test files updated in lockstep, and Phase 2 requires a manual (non-regenerated) byte-identical copy step to `agents-plugin-pi/rsrc/`, a drift-prone step the automated wsflow-mirroring regen commands do not cover |
| risk.security_or_contract | moderate | new read-only MCP tool follows the `git.log` session-key precedent (low on its own), but Phase 2 also changes the design-review gate contract by adding Checklist item 7, which can newly block a ticket's promotion to ready |

## Phases

### Phase 1: `rationale.query` tool

Implement the Tool Specification in a new package
`agents-plugin-tool/internal/wsrationale/` (extraction, stem linkage, path
matching, BM25, site and pickaxe drivers, text and JSON formatting), wired
through `internal/mcp/server.go` the way `git.log` is: dispatch case, schema
entry, session-key requirement, root resolution, compact text default. Add the
tool to the `tools` contract in `agents-plugin/runtime.json`,
`agents-plugin-wsflow/runtime.json`, and `agents-plugin-pi/runtime.json` at
the same version range as `git.log`. Write the twelve acceptance tests.

Verification: `go test ./... -count=1` in `agents-plugin-tool/` green; a manual
run of the three Phase 3 queries against this repository returns non-empty
threads (record the outputs' first lines in the Result).

Deferred from this phase: cache, `all_branches`, rename following.

### Phase 2: populator and reviewer prose

Insert the Playbook Text verbatim into
`agents-plugin/rsrc/ticket-fact-populator/ticket-fact-populator.md` and
`agents-plugin/rsrc/ticket-reviewer-design/ticket-reviewer-design.md`. Apply
the skill-authoring invariant checklist to every changed line. Regenerate
mirrors with the two mandatory commands from `wsflow-mirroring.md` (both with
`-count=1`) and copy the two playbooks and manifest to `agents-plugin-pi/rsrc/`
by hand; confirm the pi mirror test passes. Update pins that name these two
playbooks in `agents-plugin-tool/internal/mcp/playbook_tools_test.go`,
`playbook_render_surface_test.go`, `ticket_review_design_test.go`,
`ticket_batch_review_test.go`, and
`agents-plugin/tests/test_skill_dispatch_contracts.py`; add a pin for the
`## Prior Decisions` section text, the populator's `prior_contradictions:`
report line, and the reviewer's item 7. Confirm the route-facts reader still
parses a ticket carrying a `## Prior Decisions` section before `## Route Facts`
(existing route tests plus one fixture ticket with both sections).

Verification: `go test ./... -count=1`, the wsflow package tests, and
`agents-plugin/tests` all green; the three copies of each playbook
byte-identical.

### Phase 3: validation on recorded reversals

Run the tool against this repository, without editing any closed ticket, and
record precision. Each case mirrors one populator recipe query and is judged
"target present in the top 8 threads":

1. Query `session.note`, no paths: the targets are the refoundation epic's
   Cross-Child Decision 16 (`session.note` as the carry-over record) and the
   lead-run diet ticket's decision declaring its removal. Then paths
   `agents-plugin/rsrc/lead-run`, no query: the target is the diet ticket's
   implementation commit whose AI Context explains dropping the record.
2. Query `spec-address gate`, no paths: the target is the retirement ticket's
   decision to delete the ready spec-address gate. Then paths
   `agents-plugin-tool/internal/wsdoc/tickets_mutate.go`, no query: the
   target is the commit that retired that gate.
3. Site `agents-plugin-tool/internal/mcp/server.go:/^func \(s \*Server\) resolveToolRoot/,+20`:
   the chain has nine members as of 2026-09-15, listed here in no particular order (`git log -w --no-patch -L
   '/^func (s \*Server) resolveToolRoot/,+20:agents-plugin-tool/internal/mcp/server.go'`
   returns 79602bf5, 6ef9c100, 24d7dfb8, bf06f7c2, 6499533d, 50e7d7d0,
   79fe8bfa, 24569308, 6762aaf5) and the target is the
   2026-06-11 member "require session keys for root resolution", whose AI
   Context records removing every silent root fallback. A regex without the
   receiver (`^func resolveToolRoot`) matches nothing and must return the
   zero-match error, not an empty chain.

For each case record: target present (yes/no), rank of the target thread,
number of relevant threads among the top 8 by your reading, wall-clock time.
Pass when all three targets are present; otherwise tune the populator recipe
or ranking in an Edition of Phase 1 or 2 before closing. The Result also
states, with these numbers as evidence, whether each follow-up candidate below
should be ticketed.

## Follow-up candidates (not in scope)

- A worker-playbook line: before editing a function, `site:` it once.
- A standalone `rationale-discovery` delegate for on-demand orientation
  (`lead-discuss`, downstream bootstrap).
- `all_branches: true` to include unmerged impl branches during goal runs.
- A HEAD-keyed untracked cache if a downstream repository scans above 2 s.
- Embedding-based ranking, only if Phase 3 shows misses that path and site
  addressing do not cover.
- Reconcile `ai-docs/spec/pi-adapter-runtime.md`, the one live spec-shaped
  file this repository still maintains, against the retired-layer rule; it is
  evidence for the demand this ticket serves and is outside this ticket.
