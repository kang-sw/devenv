---
title: "whiteboard.update: one malformed command throws away a large batch"
related:
  261113-feat-ws-whiteboard-topic-panels-and-log: "append proposal superseded; folder transaction and entry model are now the baseline"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 39fb2abec55d50e8
sage-review-completeness-reviewed: 39fb2abec55d50e8
---

# whiteboard.update: one malformed command throws away a large batch

## Prior Decisions

- 59a9da6c (2026-11-13, commit): "move to an incremental apply-what-validates model with a failed-command XML tree" — bearing: supports
- 261113-feat-ws-whiteboard-topic-panels-and-log (2026-11-13, Resolution): "Note-only answer mapping/status remains owned by `261113-feat-ws-whiteboard-update-refusal-cost`, not this ticket." — bearing: supports
- 7322118e (2026-11-08, commit): "Fixed by diffing against the batch-start state" — bearing: constrains
- fc0ca459 (2026-11-16, commit): "Not changed: the class open-only gate." — bearing: constrains
- 261115-feat-ws-whiteboard-topic-default-and-update-summaries (2026-11-15, Result): "ordered refusal, committed effects, and publication-only recovery remain intact." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wswhiteboard mutation/parser, folder persistence, query/view/history; MCP result and publication integration, with existing localization as needed. Playbooks are excluded. |
| scope.surface | cross-module | Persisted Item.Status, whiteboard.update first-text results, query and dashboard rendering. |
| scope.new_public_symbol | yes | Technical status spelling `commented` requires a status constant; no new MCP tool or argument. |
| scope.new_type_contract | yes | Command-local acceptance/failure outcomes and one additional existing-schema status value. |
| scope.test_surface | existing | wswhiteboard commands/store/entries/folders/query/view/history tests; MCP whiteboard tools/summary/effects/publication tests. Add regression cases there. |
| complexity.reuse_points | confirmed | Apply/ordered grammar; FolderStore plan/replay; existing system entries, MCP text/error envelope and publication observation (citations below). |
| complexity.side_effect_risk | high | Roll back failed command mutations, events and counters; persist/publish accepted results once. |
| risk.correctness | high | Multi-ID rollback, topic-scoped close guards, parser boundaries and accepted-only defaults. |
| risk.fit | moderate | Preserve board 43/44; all mapping, spelling and compatibility choices below are attributed technically. |
| risk.test | high | Phase 1 requires mixed-success, lifecycle, old-write/new-reopen and stamp/publication matrices; no implementation verification is claimed by this draft. |
| risk.security_or_contract | high | Preserve legacy calls, schema 2, sandbox boundaries and uncertain-commit inspection; no automatic replay. |

## Background

Owner question 2026-11-13 ("a whiteboard format error is expensive because
the bodies are large"). `whiteboard.update` validates a batch whole: any
problem refuses all of it, the lead must resend every body, and the refusal
now lists problems and a playbook pointer, without reprinting syntax or the
command section (VERIFIED: `agents-plugin-tool/internal/mcp/command_syntax.go#L66-L80`;
the earlier ~10 KB cost is a historical session report, not a current measured size). In one session three refusals came from three patterns:

1. `wb:edit` anchor ambiguity: a generic `wb:old` (`## `) matched three
   places in a body the lead had not queried first.
2. `wb:edit` on a topic that has no body yet ("set one with `<wb:body>`").
3. A note-only board answer to a policy item. The answer block allows
   `41=^<note>` (a note alone), but `wb:answer` on a policy item with nonempty options requires the
   value to start with an option letter (VERIFIED:
   `agents-plugin-tool/internal/wswhiteboard/store.go#L944-L947`), so the lead had to invent a letter
   and mark it assumed.

The compact-refusal hotfix is already present, not remaining work
(`agents-plugin-tool/internal/mcp/command_syntax.go#L66-L80`). The former
append proposal was superseded by panels/logs. The historical three-match
incident and invented-letter workaround above are session evidence, not
reproduced fixtures or approved behavior.

## Decisions

- Owner 2026-11-13 (board 43=b): the atomic unit is one command, not a
  topic group. A wrong answer must not discard large bodies; the lead sees
  the failed-command tree and resends only those next turn. Lead technical
  guard: a `wb:close` is skipped (reported as failed) when an earlier
  command in the same batch for the same topic failed, so a close never
  withdraws an item whose answer just failed.
- Owner 2026-11-13 (board 44=b): a note-only answer to a policy item gets a
  new status meaning "user commented, lead's move": it leaves the user's
  waiting list until the lead re-asks, records an option letter, or
  withdraws it. Compatibility: a new item status in the whiteboard store is
  a store change; settle its tier at promotion.

### Technical settlements (2026-11-16; not additional owner answers)

- **MAIN, under owner 44=b:** note-only policy answers use explicit `value=""`,
  with the note in the existing required verbatim body/quote field. This matches
  wsinbox's empty parsed Value for `NN=^note`
  (`agents-plugin-tool/internal/wsinbox/answers.go#L156-L165`,
  `agents-plugin-tool/internal/wsinbox/answers.go#L192-L207`). A missing value
  remains an error; only the exact empty string selects this mapping, not
  whitespace-only values. Empty non-policy values retain today's refusal.
  Nonempty policy values retain option-letter validation; invalid letters remain
  errors. No option is fabricated.
- **Lead technical consequence of 44=b:** use the additional status string
  `commented`, not new fields or a schema. Store `Answer=""`, the verbatim note
  in `AnswerQuote`, and its time in `AnsweredAt`. The empty policy value always
  selects `commented`; `assumed` qualifies nonempty answers, not comments (its
  syntax still validates). Use the call's Changed mark/time and allocate a settle
  order for rotation as with assumed items, but do not include `commented` in
  IsSettled/archive eviction. Advance AnswerSeq for this actual user input, not
  as evidence of an option choice; its legacy fallback must recognize comments
  too. An identical repeated comment is a semantic no-op: no new event, time or
  settle/answer mark. These are lead technical bookkeeping consequences, not
  owner policy: reuse existing reply/order fields without calling a comment an
  answer or assumption. Keep it queryable and visible as "user commented,
  lead's move", out of open-user
  counts, and retained until the lead acts (not archive-evictable as a settled
  decision). Re-ask via `status="open"` clears answer fields and stamps the
  existing re-ask markers; a valid letter records answered/assumed normally;
  explicit withdrawal/supersession preserves the note as history. Closing a
  topic still withdraws only open items, not comments. Emit truthful comment
  text through the existing system-entry kind, not an answered/assumed event.
- **Lead technical consequence of 43=b:** retain one numeric call sequence
  shared by accepted commands and one recoverable durable transaction, not a
  transaction or publication per command. All-failed calls leave state/stamp
  unchanged; accepted no-ops and empty render-only calls retain existing
  sequence/embed behavior. Diff marks remain relative to the batch-start body;
  failed commands contribute no marks, defaults, effects or bookkeeping.
- **MAIN accepted unsupported-feature behavior:** a new caller sending
  `value=""` to the old server receives its existing nonblank-value refusal
  with no write. This is not a new schema, hello, or write boundary. The owner's
  later playbook pass will teach "re-ask or record a letter"; this ticket does
  not edit playbooks or silently substitute a letter.

### Directional compatibility targets

These are bounded technical targets, not verified implementation results.
"Old" is the inspected pre-feature source in this checkout; "new" is the
implementation this ticket requires. The highest target is T1. Release must
verify the actual old-write/new-reopen trace. A necessary T3/minor boundary,
new field/schema/event kind, or further product choice stops back to the lead.

| Surface / direction | Old read/write evidence | Required new reopen / result | Target |
|---|---|---|---|
| New writer stores `commented` in schema 2; old reader displays it; old writer saves unrelated changes | Item.Status is a string and quote/time are existing fields (`agents-plugin-tool/internal/wswhiteboard/store.go#L118-L151`). Load validates identity/topic, not status (`agents-plugin-tool/internal/wswhiteboard/folders.go#L384-L411`). Clone preserves those known fields, metadata save copies entire Items, and archive eviction only drops IsSettled (`agents-plugin-tool/internal/wswhiteboard/store.go#L357-L377`, `agents-plugin-tool/internal/wswhiteboard/store.go#L577-L604`; `agents-plugin-tool/internal/wswhiteboard/folders.go#L582-L641`, `agents-plugin-tool/internal/wswhiteboard/folders.go#L689-L704`). Old waiting counts require `open`; history defaults to an answered label and recent-settled/query summary omit the unknown semantics (`agents-plugin-tool/internal/wswhiteboard/store.go#L164-L175`; `agents-plugin-tool/internal/wswhiteboard/history_render.go#L41-L55`; `agents-plugin-tool/internal/wswhiteboard/view.go#L914-L946`; `agents-plugin-tool/internal/wswhiteboard/query.go#L386-L400`, `agents-plugin-tool/internal/wswhiteboard/query.go#L453-L466`). | Read both old statuses and `commented` in unchanged schema 2; after an old unrelated write, recover the identical status, quote and time directly from topic metadata, not reconstructed events (`agents-plugin-tool/internal/wswhiteboard/folders.go#L306-L323`, `agents-plugin-tool/internal/wswhiteboard/folders.go#L476-L490`). Old display/query degradation is accepted; no incidental normalization or data loss. Old explicit re-ask, valid answer, withdraw/supersede are intended transitions, not damage to resurrect: patch starts from the whole previous Item, re-ask clears answers, withdrawal keeps history; answer replaces it deliberately (`agents-plugin-tool/internal/wswhiteboard/store.go#L1053-L1059`, `agents-plugin-tool/internal/wswhiteboard/store.go#L1110-L1134`, `agents-plugin-tool/internal/wswhiteboard/store.go#L935-L969`). | **T1**, same-schema status-string addition with preservation. No new Item fields: fixed-struct rewrites would drop them. No T2 recovery claim from append-only logs. |
| New server receives legacy calls; old caller reads mixed-success text | Existing MCP success/error envelopes carry one text item; error sets `isError: true` (`agents-plugin-tool/internal/mcp/server.go#L4395-L4415`). Old Pi surfaces that first text on error without retrying (`agents-plugin-pi/src/bridge.ts#L1188-L1193`); launcher reconnect replays initialization, reports lost sent calls, and forwards only unsent requests (`agents-plugin/bin/ws-mcp-launcher.py#L1645-L1663`). Existing successful calls remain accepted; all-success output retains IDs, links and summaries (`agents-plugin-tool/internal/mcp/whiteboard_tools.go#L308-L334`). | Keep the envelope and legacy arguments. Any recoverable command failure uses the existing error envelope, but its **first text** states accepted effects and failed commands truthfully, with the complete failed tree and retry instruction. Only all-failed may say "nothing was applied". New caller to old server gets whole-batch refusal/no failed tree, not partial success (`agents-plugin-tool/internal/mcp/whiteboard_tools.go#L257-L292`). Output is not durable retry state: after uncertainty, inspect committed state; reopening cannot reconstruct a lost response. | **T0** for legacy transport/calls under the evidenced no-automatic-machine-replay path. No promise that blindly replaying an entire mixed batch is safe. |
| New partial-apply writer; old numeric sequence/browser reader and subsequent writer | Seq is the existing render number; Apply increments it once, BeginRender also advances it, and successful commands share its mark (`agents-plugin-tool/internal/wswhiteboard/store.go#L568-L575`, `agents-plugin-tool/internal/wswhiteboard/store.go#L637-L643`, `agents-plugin-tool/internal/wswhiteboard/store.go#L749-L755`). Folder session State retains it; replay publishes session metadata last and load recovers pending intent first (`agents-plugin-tool/internal/wswhiteboard/folders.go#L49-L56`, `agents-plugin-tool/internal/wswhiteboard/folders.go#L582-L594`, `agents-plugin-tool/internal/wswhiteboard/folders.go#L727-L819`, `agents-plugin-tool/internal/wswhiteboard/folders.go#L306-L323`). `wbStamp` emits that number and old JS reloads on inequality (`agents-plugin-tool/internal/mcp/whiteboard_tools.go#L508-L521`; `agents-plugin-tool/internal/wswhiteboard/assets/live.js#L18-L55`). | Preserve numeric fields, accepted-call +1, shared marks, LastEmbedSeq and batch-start baseline semantics. An old write advances the same fields; new reopen uses the persisted values, with no alternative reconstruction. Failed-only calls leave all unchanged; accepted no-ops/empty calls keep existing behavior (`agents-plugin-tool/internal/wswhiteboard/store_test.go#L755-L770`). Publish accepted state once after durable commit; repair publication without replaying mutations. | **T0** for retained one-call stamp and durable publication contract, independently of the T1 status display. |

## Implementation contract

### Ordered command acceptance and failed tree

- Apply commands in input order against accepted prior state. Roll back the
  **entire** failing command, including partial multi-ID changes, generated
  entries, item/panel/entry/settle counters, effects, scope adoption and summary
  bookkeeping. Later commands can use accepted earlier creations, never failed
  ones. Existing `closedBy` protection for answer-after-close stays.
- A recoverable parser/validation failure belongs to its soundly delimited
  command and must not discard unrelated valid commands. If malformed input
  cannot be partitioned into sound commands (for example an unterminated body
  or group), refuse the whole input without writes; do not apply its parsed
  prefix. Keep diagnostics for what can be identified without guessing intent.
- Treat a group's label/tickets mutation as one command, not atomicity for all
  children. If its metadata/scope fails, roll it back and report dependent
  children as failed; continue unrelated groups/root commands. Resolve one
  accepted topic identity for its children. Default topic-state inference and
  same-batch explicit declarations must consider accepted commands only.
- Track earlier failures by resolved topic, including every known target of a
  multi-ID answer/patch and failed group metadata. A later close of any affected
  topic fails even if an intervening correction succeeds. Unrelated topics may
  close; an unresolvable nonexistent ID does not poison unrelated topics.
- First-text output carries diagnostics plus a `<wb:failed>` result tree with
  only failed commands in original order, retaining full bodies, fields,
  attributes and necessary `wb:topic` scoping. Include full rejected payloads,
  not tag-only skeletons, so correction/resend needs no accepted large bodies.
  Diagnostics/locators must not replace payloads or become input attributes.
  `wb:failed` is an output wrapper, not a new input command. Resend its corrected
  command contents only; do not resend accepted commands. Unsplittable malformed
  input instead gets an explicit whole-input/no-write refusal.

### Persistence, effects and comment lifecycle

Reuse the landed panels/log folder authority and existing system events.
Stage only accepted commands into one FolderStore intent; failed commands emit
no durable event or conversation effect. Observe commitment separately from
publication: a committed publication failure identifies accepted work and calls
for empty-command render-only repair, not mutation replay; unconfirmed commitment
requires inspection first. Preserve current best-effort conversation projection
semantics, not a new retry store or exactly-once projection guarantee.

Integrate `commented` into existing status queries/counts, item summaries,
history/timeline, file/served/embed views and retention without implying an
option was chosen or an assumption confirmed. Preserve original options and
verbatim quote retrieval. The bookkeeping settlement above follows the current
non-assumption wb:answer marker and assumed-item retention/order patterns
(`agents-plugin-tool/internal/wswhiteboard/store.go#L37-L48`,
`agents-plugin-tool/internal/wswhiteboard/store.go#L262-L268`,
`agents-plugin-tool/internal/wswhiteboard/store.go#L532-L560`,
`agents-plugin-tool/internal/wswhiteboard/store.go#L950-L969`); it changes no
owner decision. Further caller-visible choices stop back to the lead.

## Verified implementation baseline (2026-11-16)

- Apply is currently whole-batch atomic over a private copy; dispatch is ordered
  and grouped answers can mutate before a later ID fails
  (`agents-plugin-tool/internal/wswhiteboard/store.go#L608-L643`,
  `agents-plugin-tool/internal/wswhiteboard/store.go#L667-L755`,
  `agents-plugin-tool/internal/wswhiteboard/store.go#L918-L969`). `wb:close`
  withdraws only open items and today's guard protects answer-after-close, not
  close-after-failure (`agents-plugin-tool/internal/wswhiteboard/store.go#L702-L707`,
  `agents-plugin-tool/internal/wswhiteboard/store.go#L758-L790`,
  `agents-plugin-tool/internal/wswhiteboard/store.go#L935-L942`). This is why command-local rollback and the new guard are needed.
- Grammar already supports body/edit/panel/log/state/eta, not append/section
  (`agents-plugin-tool/internal/wswhiteboard/commands.go#L22-L42`). Group extra
  attributes become a Command (`agents-plugin-tool/internal/wsxmlcmd/wsxmlcmd.go#L204-L237`);
  `applyGroup` can change label/tickets before a later error, so it needs rollback
  too (`agents-plugin-tool/internal/wswhiteboard/topics.go#L90-L117`). Parser
  recoverable errors continue, fatal errors return even a parsed prefix; that
  prefix is not sufficient permission to write (`agents-plugin-tool/internal/wsxmlcmd/wsxmlcmd.go#L282-L320`).
- Old `value` validation is nonblank before per-item policy validation, and
  quote body is required (`agents-plugin-tool/internal/wswhiteboard/store.go#L918-L947`;
  `agents-plugin-tool/internal/wsxmlcmd/check.go#L21-L39`). Existing generic
  system entries hold text and ItemID, needing no new event kind for comments
  (`agents-plugin-tool/internal/wswhiteboard/entries.go#L16-L39`,
  `agents-plugin-tool/internal/wswhiteboard/entries.go#L75-L90`).
- The handler currently refuses any problem and records effects only after
  observed commitment (`agents-plugin-tool/internal/mcp/whiteboard_tools.go#L257-L303`).
  Render preparation sets embed baseline and stamp before the folder transaction
  publishes (`agents-plugin-tool/internal/mcp/whiteboard_tools.go#L452-L524`);
  accepted-only state must reach that path. Body time
  proofs are minted against the batch-start origin only for changed bodies
  (`agents-plugin-tool/internal/wswhiteboard/bodytime.go#L107-L116`).
- These sources and existing atomic-refusal tests describe the old baseline,
  not a pass for this feature. No build, test or runtime probe was run for this
  documentation draft.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/); emitted text must be downstream-neutral.
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/); keep default results compact text in the existing envelope.
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/); full directional preservation/reopen evidence is required. Keep schema 2; its existing guard would make a schema bump a T3 boundary (`agents-plugin-tool/internal/wswhiteboard/folders.go#L238-L303`). Stop rather than select T3/minor or a new policy/scope.
- Keep exact-match `wb:edit` and its empty-body refusal. No section-scoped edit, changed empty-body semantics, `wb:append`, server retry token, new protocol argument, broad heuristic or new event kind.
- Playbook changes are the owner's later pass, outside this ticket. Their skill-authoring/mirroring manuals were read for this draft but do not authorize guidance changes here. Preserve existing sandbox isolation and legacy rollback snapshots; no additional UI redesign or unrelated fact-note feature.
- No visual reference is approved for this ticket; the predecessor's panel mockup is not an approval for a new design here.

## Phases

### Phase 1: Command-local acceptance, complete retries and note-only policy comments

Implement the Decisions and Implementation contract as one coherent feature,
including status consumers and MCP/persistence integration. Reuse existing
ordered parsing, system entries and recoverable folder publication; do not
introduce a second store or per-command durable commits. Deferred scope is the
owner's later playbook pass and the exclusions above.

Acceptance and verification (future implementation, not run by this draft):

1. **Atomicity/order:** mixed valid/invalid body, edit, add, answer, patch,
   panel/log and topic-metadata commands keep valid effects in input order.
   Multi-ID failure rolls back every target, event, counter, ID and summary;
   a later dependent command cannot see a failed creation. Cover metadata
   rollback/blocked children, accepted-only default-state inference, independent
   topics, and existing answer-after-close protection. Recoverable unknown tags,
   bad attributes and anchor errors fail locally; unsplittable malformed XML
   (including a valid prefix) changes no board/store/publication bytes.
2. **Close guard/retry:** a failed answer/patch/scoped command blocks every later
   close of its resolved topic(s), even after a correction; other topics close.
   Include mixed-ID known/missing targets, group metadata failure and a failure
   after an already accepted close. Full failed-tree output retains whitespace,
   XML-like/HTML bodies, fields, attributes and scopes in order. Correct and resend
   only that tree's commands: accepted panels/logs/IDs must not duplicate.
3. **Comments:** `value=""` plus a required verbatim note on policy maps to
   `commented` even with `assumed="true"`, no chosen/assumed answer; missing/
   blank quote, missing value, whitespace-only value, empty non-policy value
   and invalid nonempty letters fail without effects.
   Cover repeated comments/no-op, multi-ID rollback, re-ask, letter answer,
   withdrawal/supersession, topic close, quote/time retrieval, waiting counts,
   query filter/count/summary, history/timeline, full/embed visibility and archive
   retention. Pin answer/change/settle bookkeeping and truthful system text.
4. **Directional store/call compatibility:** new writer -> old source fixture
   read -> unrelated old mutation/save -> new reopen retains `commented`,
   quote/time/options and IDs exactly, including archive-pressure saves. Old
   explicit re-ask/letter/withdraw transitions remain authoritative on new
   reopen. New reader also opens old statuses/history. Confirm old display-only
   degradation and unchanged schema; do not claim recovery from logs. New
   empty-value caller -> old-server fixture gets the existing nonblank refusal
   with unchanged board bytes. Legacy nonempty calls still work on the new
   server; mixed results keep the MCP envelope and complete first-text applied/
   failed information, including error-path first-text extraction without replay.
5. **Sequence/publication:** compare baseline all-success with mixed success,
   all-failed, accepted no-op and empty render-only calls. Accepted calls advance
   Seq once and share marks; all-failed leave state/stamp/embed baseline unchanged.
   Preserve batch-start diff marks, AnswerSeq/settle counters and LastEmbedSeq
   semantics. Inject commit/replay/publication failures: accepted-only durable
   events/effects, one intent/publication, recovery with no duplicate logs/panels,
   and empty-command publication repair. Unconfirmed commitment says inspect
   before retry; reopening never fabricates a lost result or projection.

Extend the existing wswhiteboard commands/store/entries/folders/query/view/history
and MCP tools/summary/effects/publication suites rather than adding a parallel
harness. During implementation, run the applicable focused package cases with
`-count=1` and explicit `-run` selections covering the matrices above; include
race coverage for mutation/persistence concurrency. Every Go invocation uses
the absolute owned module cwd, isolated HOME/config/runtime/temp, warm caches,
and `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. Full MCP/dashboard/all-package
runs and long tests need separate lead authorization. Read the full selected
outputs and recheck the tier table against actual writes/reopen before claiming
these targets met.
