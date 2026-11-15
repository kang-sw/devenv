---
title: "New whiteboard topics default to discussing and updates summarize applied operations"
related:
  261115-feat-ws-whiteboard-topic-state-badge: "landed consumer of authoritative state; preserve legacy absence, diagnostics and close/reset behavior"
  261115-feat-ws-revive-topic-state-summaries: "parallel structured-state consumer; legacy absent state remains none, while new topics receive an explicit default"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: f7c577b0ad26f917
sage-review-completeness-reviewed: f7c577b0ad26f917
completed: 2026-11-15
---

# New whiteboard topics default to discussing and updates summarize applied operations

## Background

MAIN's 2026-11-15 02:39:25 UTC message relays the owner's request: every
newly added whiteboard topic defaults to `discussing`, and whiteboard updates
return short summaries of applied operations, including the assigned default
state. Currently topic creation need not declare a state, and successful
`whiteboard.update` output reports IDs, page/embed locations and optional
information rather than topic-operation summaries.

MAIN's 03:49:38 UTC instruction authorizes this independent documentation lane
without a settings-implementation dependency. This capture uses landed source
at `bffec047c`; it is not product implementation or the readiness fact-population
gate. The lead owns later fact population, design/completeness review,
promotion and commit.

## Decisions

MAIN's 2026-11-15 04:02:16 UTC technical calls settle summary selection and
explicit-creation representation below; no further owner asks or local IDs
are required.

- Every newly added topic defaults to the existing `discussing` state. An
  explicit `wb:state` in the same batch overrides that default. This is
  authoritative topic state, not a badge-only fallback or inference from logs,
  asks or activity. Reuse the existing state mechanism rather than introducing
  another enum or state store. For explicit creation, emit
  `topic added: <stem> (state: <effective>, explicit)` and do not retain an
  intermediate default event. Ordered validation is unchanged: `wb:state`
  before the activity that creates its topic still refuses the batch.
- Existing topics are not backfilled. Keep ordinary legacy absence distinct
  from malformed/unsupported state diagnostics. Updating, renaming or adopting
  an existing topic is not authority to assign it a default. Preserve the
  existing close/reset and reopen lifecycle rather than treating reopening as
  creation.
- Append short per-topic applied-operation summary lines to successful update
  output. Preserve existing `ids:`, `full:` and `embed:` lines and their values
  byte-for-byte; `ids:` remains conditional on added items. The settled literal
  examples are `topic added: <stem> (state: discussing, default)` and
  `topic updated: <stem>`. The creation summary identifies the default when it
  was assigned; do not report a default as the effective state when an explicit
  declaration overrides it.
- Emit one summary per changed resolved topic, in first-touch order, selected
  from net semantic changes rather than intermediate command transitions.
  Added wins over updated for a newly created topic changed again in the same
  batch. No-ops and render-only calls emit no topic summary.
- Updated summaries include item answers/patches, state/ETA changes,
  close/reopen and adoption/rename when they produce net semantic changes.
  Attribute root `wb:answer` commands to each affected item's resolved topic.
  Include compact kind tags, as in
  `topic updated: <stem> (answered 2, closed)` and
  `topic updated: <stem> (renamed from <old>)`. These examples preserve the
  approved shapes; other counts/tags and their compact deterministic forms
  remain implementation choices, not an additional normative vocabulary.
- Illustrative default-creation result, with runtime-dependent values
  represented by placeholders (not a fixture fixing IDs or paths):

  ```text
  ids: (01)
  full: <unchanged full URL or path>
  embed: <unchanged embed filename or path>
  topic added: rollout (state: discussing, default)
  ```

  Existing `info:` and `warning:` output retains its behavior. The selection
  and ordering rules above also apply when several topics or operations share
  a batch.
- Existing refusal output already lists problems and remains unchanged. Reject
  replacing it with a new error protocol or reporting requested operations as
  applied when the batch was refused. Preserve existing committed-but-publication-
  failed diagnostics and render-only recovery rather than instructing callers
  to resend accepted commands.
- Expected compatibility tier: **T0**, grounded in additive text output and use
  of an already supported state value/record, with no tool/argument removal,
  schema/protocol change or new tracker event. Static consumer inspection below
  finds no strict whiteboard line-count/parser requirement in the repository's
  Pi bridge or Codex launcher path. This is not a claim about uninspected host
  internals or an executed compatibility test; retain old-line regression checks.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`). New emitted
  summaries and shipped playbook text must resolve in downstream projects,
  without this repository's tickets, hashes or maintenance paths.
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`). Document the changed default/result contract
  without duplicating MCP schema or deterministic post-call instructions.
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`).
  Regenerate canonical resource metadata and the byte-identical wsflow tree if
  the playbook changes; check product-mode output. Account for the packaged Pi
  resource copy through its existing copy pipeline, not an independent prose fork.
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`). Keep the default result compact text;
  this request does not select a JSON result or another API.
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`).
  Keep existing tool arguments, persisted shape, state lifecycle and protocol;
  a non-additive requirement needs lead disposition rather than silently
  changing the expected T0 classification.
- Summary production must respect batch atomicity and durable commitment.
  Preserve committed-effects conversation behavior; a transient summary is not
  authority to add a new event variant or persistent summary store.
- This preparation edits only this existing todo ticket. No source changes,
  tests/builds, browser/live probes, provider calls, downloads, shared-runtime
  actions, port 29417 use, process signals, branch synchronization, reviewers,
  stamps, promotion or commit are authorized here. The write-once plan remains
  unchanged. Later executable verification must be separately authorized and
  isolated; the lead owns shared-runtime integration.

## Prior Art and Source Observations

Static inspection at `bffec047c`, not test execution or Route Facts:

- `agents-plugin-tool/internal/wswhiteboard/store.go`: search `Apply`,
  `Outcome`, `topicKnown`, `applyAdd` and `applyClose`. Apply runs ordered commands
  against a clone and returns an empty outcome on refusal. Outcome already holds
  IDs, withdrawals, info and semantic effects, but no topic-summary field.
  Items, bodies and entries establish topic existence; a blanket fallback for
  every absent `TopicStates()` result would backfill old topics.
- `agents-plugin-tool/internal/wswhiteboard/topics.go`: search `applyGroup`,
  `applyLabel`, `setTopicTickets` and `renameTopic`. Linked tickets can persist
  a new topic without content commands; labels may rename/adopt existing
  identity. An empty unknown group is not generally topic creation. These are
  creation/identity seams to cover, not permission to change command validity.
- `agents-plugin-tool/internal/wswhiteboard/topic_state.go`: search
  `applyTopicState`, `appendStateEvent`, `TopicStates` and `structuralWarnings`.
  Existing version-1 system entries own state; an explicit declaration requires
  a known open topic. Close resets it; system events alone do not reopen it
  (`entries.go`, `reopenWithEvent`). Defaulting a content-only topic to discussing
  can now trigger the existing `discussing_without_open_items` info warning;
  do not suppress that warning or create an ask as an incidental fix.
- `agents-plugin-tool/internal/wswhiteboard/effects.go`: `semanticEffects`
  compares typed before/after values around commands, with resolved identities.
  Topic fields are presence/label/body/tickets/closed/result; operational state
  and ETA are not current effect fields, and system prose is excluded. Effects
  are useful prior art, but do not alone supply state/ETA inclusion or the
  approved net-per-topic summary selection.
- `agents-plugin-tool/internal/mcp/whiteboard_tools.go`: search
  `handleWhiteboardUpdate`, `observed.Committed` and `whiteboardRefusal`.
  Success writes optional IDs, full, embed, optional info and knob warning.
  Conversation effects are recorded only after observed commitment; apply,
  persistence and derived publication are distinct. Append summaries without
  repurposing existing lines or changing refusal/recovery paths.
- Consumer/parser findings: `agents-plugin-pi/src/mcp-stdio-client.ts`
  (`callTool`, `PendingRequestRegistry.settle`) passes the result envelope;
  `src/bridge.ts`'s raw dispatch returns `content` and `details: result`, with
  special text transforms for manual/playbook calls, not whiteboard results.
  `agents-plugin/.codex-plugin/plugin.json` points to `.mcp.json`, which starts
  `bin/ws-mcp-launcher.py`; its `McpProxy.deliver`/`output` inspect JSON-RPC IDs
  and forward original lines, not whiteboard text. Codex host internals are not
  in this repository. MCP `whiteboard_tools_test.go`'s `resultLine` selects by
  prefix and its render test checks exact IDs and path shapes, not a fixed
  success line count. Pi `test/mcp-stdio-client.test.ts` covers generic envelope,
  Unicode and response matching; launcher `tests/test_ws_mcp_launcher_proxy.py`
  covers generic forwarding/reconnect. These are not dedicated appended-summary
  compatibility regressions yet.

The completed `261115-feat-ws-whiteboard-topic-state-badge` consumes declared
state, preserves legacy absence/diagnostics and clears closed/run displays;
this request changes the producer default, not badge/timer design.
`261115-feat-ws-revive-topic-state-summaries` selects `state: none` for absent
state and `state: unknown (<raw value>)` for diagnostics. Keep those approved
cases: a newly defaulted topic has declared discussing state, while an existing
state-less topic still has none. Neither related ticket needs its approved
behavior silently rewritten, and this capture adds no hard dependency on it.

## Prior Decisions

- 261114-feat-ws-whiteboard-topic-state-and-lead-nudges (2026-11-15, Result (0203cff95)): "Scoped cumulative `wb:eta` preserves original start, run identity, owner and initial estimate, validates strictly increasing totals and records every extension reason/count." — bearing: constrains
- 261115-feat-ws-revive-topic-state-summaries (2026-11-15, Decisions): "Reuse existing structured state/run/ETA and item authority." — bearing: constrains
- 240de91d (2026-11-15, commit): "Recovery/render-only calls never backfill missing effects." — bearing: constrains
- 5b2b01b5 (2026-11-15, commit): "A newly labeled empty topic group is provisional until it gains content/items/tickets; pruneLabels may discard it." — bearing: supports
- 206a5830 (2026-11-15, commit): "Preserve incoming per-command and final effects capture, plus stampBodyTimeProofs(origins) immediately after stampChanges." — bearing: constrains
- 261113-feat-ws-whiteboard-update-refusal-cost (2026-11-13, commit 5975183e): "validation and atomicity unchanged (per-command apply stays in todo 261113-feat-ws-whiteboard-update-refusal-cost)." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | The phase names agents-plugin-tool/internal/wswhiteboard/, agents-plugin-tool/internal/mcp/whiteboard_tools.go, agents-plugin/rsrc/whiteboard/whiteboard.md, generated wsflow/Pi resources and consumer tests. |
| scope.surface | cross-module | The model's accepted outcome feeds MCP text and committed conversation effects; Pi and the Codex launcher forward that result (agents-plugin-tool/internal/mcp/whiteboard_tools.go#L284-L338; agents-plugin-pi/src/bridge.ts#L1189-L1226; agents-plugin/bin/ws-mcp-launcher.py#L1619-L1630). |
| scope.new_public_symbol | unknown | No new named exported Go symbol is selected. Whether Outcome gains an exported summary field is an implementation choice; its current fields are Effects, IDs, Withdrawn and Info (agents-plugin-tool/internal/wswhiteboard/store.go#L306-L322). |
| scope.new_type_contract | yes | Decisions select additive topic-added/topic-updated text, explicit/default attribution and net-change first-touch ordering. They require no new persisted type or state enum (agents-plugin-tool/internal/wswhiteboard/topic_state.go#L18-L25, #L342-L369). |
| scope.test_surface | existing | Existing seams include agents-plugin-tool/internal/wswhiteboard/{topics,topic_tickets,topic_state,topic_state_history,effects}_test.go, agents-plugin-tool/internal/mcp/{whiteboard_tools,whiteboard_topic_state,whiteboard_publication}_test.go, agents-plugin-pi/test/mcp-stdio-client.test.ts and agents-plugin/tests/test_ws_mcp_launcher_proxy.py. New assertions are required; none were executed here. |
| complexity.reuse_points | confirmed | Read Apply's ordered clone/refusal boundary, topicKnown, applyGroup/setTopicTickets/applyLabel/renameTopic, item/body creation and panel/log append paths (agents-plugin-tool/internal/wswhiteboard/store.go#L385-L393, #L600-L716, #L854-L896, #L1110-L1134; topics.go#L90-L110, #L134-L142, #L153-L232; entries.go#L86-L163). Existing state events and the MCP commitment gate are reusable, not an existing summary implementation. |
| complexity.side_effect_risk | high | A default becomes durable system history and changes warning/state consumers; it must not backfill legacy/adopted/reopened topics or leak from a refused batch (agents-plugin-tool/internal/wswhiteboard/topic_state.go#L94-L132, #L342-L420; store.go#L701-L716; entries.go#L84-L91). |
| risk.correctness | high | Ordered state validation, identity adoption and root-answer attribution must coexist with net-per-topic selection. applyTopicState requires a known open topic; applyAnswer resolves each ID to its actual item topic (agents-plugin-tool/internal/wswhiteboard/topic_state.go#L281-L313; store.go#L902-L963; topics.go#L153-L232). |
| risk.fit | moderate | Existing effects intentionally retain intermediate command transitions and exclude system prose; their topic fields omit state/ETA. Summaries must not replace that committed-effects contract (agents-plugin-tool/internal/wswhiteboard/effects.go#L25-L40, #L68-L72, #L148-L178; agents-plugin-tool/internal/mcp/whiteboard_tools.go#L284-L301). |
| risk.test | high | Existing prefix/path assertions and publication-recovery fixtures do not prove the new default/history or net-summary matrix. Publication fixtures currently pin entry counts that a legitimate creation event will change (agents-plugin-tool/internal/mcp/whiteboard_tools_test.go#L123-L133, #L274-L285; whiteboard_publication_test.go#L66-L101). Generic consumer tests are not appended-summary regressions. |
| risk.security_or_contract | moderate | T0 depends on preserving existing text lines, grammar and immutable-state encoding. Inspected Pi dispatch and launcher paths forward result content without parsing whiteboard lines; uninspected host internals and executed compatibility remain outside this static check (agents-plugin-pi/src/mcp-stdio-client.ts#L154-L166, #L325-L327; src/bridge.ts#L1189-L1226; agents-plugin/bin/ws-mcp-launcher.py#L1535-L1542, #L1619-L1630; ai-docs/manuals/release-compatibility.md). |

## Phases

### Phase 1: Default newly created topics and append applied-operation summaries

Implement the producer default and successful update summaries as one behavior
slice using the existing topic identity, state and transaction seams. Cover
every accepted creation path, including item-, body-, log-, panel- and
linked-ticket-created topics. An explicit same-batch state wins, uses the
explicit-creation summary literal and leaves no intermediate default event;
state-before-creation validation still refuses. Existing topics,
adoption/rename, close/reset, reopen, diagnostics and run/ETA authority retain
their behavior.

Select one summary per changed resolved topic from net semantic changes in
first-touch order, with added taking precedence over updated. Include answers
attributed through their items, state/ETA changes, close/reopen and
adoption/rename, using compact kind tags; omit summaries for no-ops and
render-only calls. Keep any additional tag/count forms compact and deterministic
without extending the approved examples into an invented fixed vocabulary.
Update the shared whiteboard playbook's default/result description and generated
package resources through the existing pipelines.

Verification expectations for later implementation (none executed by this
capture):

- Model tests alongside `topics_test.go`, `topic_tickets_test.go`,
  `topic_state_test.go`, `topic_state_history_test.go` and `effects_test.go`:
  new-topic defaults across creation paths; explicit overrides with no
  intermediate default event; atomic refusal of state before creation;
  repeated groups; legacy absent/diagnostic topics without backfill; identity
  adoption/rename; close/reopen and implementing-run/ETA preservation.
  Check persistence/reload through existing folder-store fixtures. Preserve atomic refusal with neither
  default state nor success summary leaking from a rejected batch. Use genuine
  legacy fixtures for absence tests rather than expecting new topics to remain
  state-less.
- MCP tests alongside `whiteboard_tools_test.go`,
  `whiteboard_topic_state_test.go` and `whiteboard_publication_test.go`:
  default and explicit creation literals; one summary per changed resolved
  topic in first-touch order; added precedence over updated; and no summaries
  for no-ops, net-cancelled semantic changes or render-only calls. Cover mixed
  batches and repeated groups, root answers spanning multiple item topics,
  item patches, state/ETA-only changes, answer/close combinations and
  adoption/rename with the approved compact-kind examples. Preserve optional
  IDs, unchanged full/embed path-or-URL values, existing info/warnings, and
  unchanged refusal and committed-publication-failure diagnostics. Render-only
  recovery must not duplicate applied claims.
- Add an offline consumer regression showing appended summary text survives
  generic Pi dispatch and Codex-launcher forwarding without consuming or
  changing the old prefix lines. Do not substitute a live host/daemon probe
  for repository tests or claim host-internal coverage from a generic parser.
- Run the affected Go suites and the project's documented Go verification
  (`go test ./...` from `agents-plugin-tool`) within authorized isolated scope;
  disclose unrelated failures and omitted smoke checks. Run applicable Pi tests
  via its package `npm test` and resource-copy pipeline, canonical resource
  manifest/mirror guards, and wsflow package tests when those surfaces change.
  Preserve existing query/view absence and diagnostic coverage and playbook
  downstream neutrality. Record actual commands, full output and exits before
  claiming a pass; build/runtime/release checks remain outside this capture.

### Result (a0f89577d) - 2026-11-15

- Implemented authoritative creation defaults through existing version-1 state
  records, explicit same-batch suppression, and additive net-topic summaries.
  Existing topics, diagnostics, adoption/reopen, run/ETA authority, ordered
  refusal, committed effects, and publication-only recovery remain intact.
- MAIN's 04:16:28 implementation grant superseded the earlier DOCS-worker-only
  preparation constraint and full-suite wording: scoped offline verification
  only, no runtime actions or branch synchronization. The phase plan is unchanged.
- Defaults are planned against an ordinary ordered transaction and replayed on
  a fresh copy only when needed, preserving default-before-close history without
  an intermediate default for explicit creation. Summaries align resolved
  identities once, retain first-touch order, and attribute compact item kinds to
  surviving semantic field writers. Creation followed by close reports effective
  state `none`, provenance, and `closed`; in-place adoption uses `adopted`.
- Updated canonical guidance, regenerated ws/wsflow/Pi resources through existing
  pipelines, and added offline Pi/launcher forwarding regressions preserving the
  old prefix lines. Migrated default-aware entry fixtures; literal legacy
  fixtures retain absence/removal coverage rather than weakening assertions.
- Verification passed with `GOPROXY=off`, `GOSUMDB=off`, local toolchain, isolated
  cache/config, and package offline mode: 91 exact-name model tests from eleven
  relevant files (historical writer excluded); `go test -p=1 ./internal/mcp
  -run '^TestWhiteboard' -count=1 -timeout=180s`; Pi stdio-client tests (20);
  socket-free launcher regression (1); resource guards (4); wsflow package
  checks (24, two platform skips); manifest/mirror generation and byte equality.
  Final model/MCP outputs and exact selections are in
  `/tmp/ws-topic-default-summary-261115/`; support evidence is in
  `/tmp/ws-whiteboard-support.vEOywL/`.
- One independent Lite found one Important converged-adoption comparison bug,
  fixed in `a0f89577d` with model/MCP regressions; no Critical or Minor findings.
  No re-review. Earlier failures were diagnosed as stale event/legacy fixtures,
  an obsolete playbook assertion, and nudge assertions conflating structural
  warnings with nudges; exact contracts were retained in their corrections.
- Omitted by authorization: full repository/Pi/browser suites, historical-writer
  subprocess execution, live/provider/runtime checks, unrelated markup/waiting-on
  repairs, and synchronization. The lead owns the single subsequent sync and
  integration; no T1/T2 surface or unresolved finding remains.
