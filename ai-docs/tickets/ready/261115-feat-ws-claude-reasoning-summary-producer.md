---
title: "Show public Claude reasoning summaries through interactive and hosted producers"
related:
  261114-feat-ws-live-reasoning-summary: "reuse the existing lead-only projection, immutable natural-turn history and display contract; its Pi-only admission is not Claude support"
  261115-feat-ws-permission-profile: "MAIN queue ordering only; no permission implementation contract is consumed by this ticket"
  261115-feat-ws-worker-profile-session-spawn: "MAIN queue ordering only; no worker-profile implementation prerequisite is invented"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 4fbfddb5c738fb49
sage-review-completeness-reviewed: 4fbfddb5c738fb49
---

# Show public Claude reasoning summaries through interactive and hosted producers

## Background

Owner check (1074) saw no reasoning-summary text in an external interactive
Claude Code MAIN session. The existing implementation accepts Pi's verified
OpenAI Responses summary sources, not Claude thinking chunks. Restarting or
re-registering that Claude session does not add a producer.

MAIN relayed owner **(1090)=a**, approximately **2026-11-15 09:17Z**, as
"proceed to an implementation ticket". Owner **(1085)=a** authorized one bounded
experiment, already completed by MAIN; it does not authorize another experiment
while authoring this ticket. MAIN **09:23:25** requires interactive TUI/mod-path
verification in Phase 1, performed later under its own worker dispatch.

MAIN **09:33:26** relayed owner **(1093)=a** plus
`hosted에서만 자동으로 켜주는 방향으로 가죠`: external interactive Claude sessions
select summarized mode per session; existing sessions and global settings stay
unchanged; dashboard-launched ws-hosted Claude sessions automatically receive
`--thinking-display summarized`. MAIN **09:35:39** clarified the outcome: hosted
sessions must **show summaries**, not merely receive an invisible launch flag.
External TUI/mod delivery comes first; a later useful hosted producer and its
auto-enable wiring belong to this same ticket. Hosted/headless delivery is not
out of scope.

This ticket captures that confirmed outcome. Fact population does not run
Claude, change a setting, implement a producer, or promote itself. MAIN
10:07:22 assigns ready promotion followed by Phases 1+2 as one implementation
head; hosted Phases 3+4 remain in this ticket but are not this run. Permission
P2/D14 and worker-profile Phase 1 links are queue ordering only, not
prerequisites or `blocked-by` claims (current MAIN assignment supplied with
this fact-population request).

## Decisions

1. **Owner (1090)/(1093), MAIN 09:35:39 — both useful delivery paths.** Deliver
   public reasoning-summary text from external interactive Claude leads first,
   then ws-hosted Claude leads through the existing dashboard activity/history
   display. Pair hosted automatic summarized-mode launch wiring with the hosted
   delivery slice; a cosmetic flag-only landing does not satisfy the outcome.
2. **Owner (1093) — activation boundary.** External interactive activation is a
   per-session user choice. Leave existing sessions and global settings
   unchanged. Automatically select `--thinking-display summarized` for newly
   dashboard-launched ws-hosted Claude sessions. Do not retrofit a running
   session, restart it, or edit an installed/global configuration to acquire
   summaries. The precise supported external session control is a Phase 1
   evidence requirement, not an assumed settings API.
3. **MAIN's binding provenance gate.** Send text only when the actual source
   path establishes public-summary provenance. A nonempty `thinking` chunk,
   selected model name, producer opt-in, launch flag alone, or a settings read
   does not establish effective per-request mode. Where that evidence is
   missing, ambiguous, unsupported or contradicted, keep ordinary activity and
   report summary unavailability; never send raw thinking, signatures,
   redacted/opaque data, request bodies or reconstructed reasoning. Apply this
   gate independently to external and hosted transports.
4. **Existing 675 owner contract — leads and natural history.** Subagents remain
   outside scope. Reuse lead-only current-turn projection, real natural-turn
   settlement, exact observed interrupted/failed prefixes and immutable
   completed history. Preserve actual aborted/failed/incomplete status; do not
   manufacture completion, a new turn, or missing summary text. Process death
   before the immutable commit may lose the in-flight summary. Mutable disk
   snapshots remain summary-free; reopen starts with no live summary until a
   new admissible event, while committed history remains readable.
5. **Existing owners remain authoritative.** Use the current mod observation,
   terminal intake, hosted backend/holder, live reducer, summary commit and
   renderer owners. Preserve stream chunks/results and tool/permission/input
   behavior. A source label or flag is not a slot/registration/holder ownership
   proof. Do not add another input owner, transcript scraper, renderer, runtime,
   generated summary model or alternate conversation store.
6. **Phase and authorization boundary.** Verify the external interactive path
   before enabling its producer; verify the hosted stream path before enabling
   its producer. A successful headless experiment is not interactive TUI
   verification. Authoring and ready review authorize none of those live checks
   during fact population. MAIN 10:07:22 explicitly authorizes bounded external
   interactive TUI/mod verification in Phase 1 and the selected session-local
   `--thinking-display summarized` launch; hosted Phase 3 still needs a later
   bounded dispatch. No existing/global changes, installs/downloads, shared
   daemon swap or unrelated runtime operation are authorized (current MAIN
   assignment supplied with this fact-population request).
7. **Directional compatibility — design grades, not completed acceptance.**
   MAIN 09:44:13 accepted **T0** for unchanged legacy calls/ordinary display,
   gated optional calls and summary-free mutable snapshots, and **T1
   display-degraded** for an old validator hiding retained Claude-source history
   without rewriting it. The source trace below pins the actual old reader and
   writer. Delivery fixtures must prove unchanged bytes and Claude-capable new
   reopen/display; neither optional fields nor retained bytes alone prove that
   result. No T2 overwrite recovery or T3/T4 connection/schema boundary is
   authorized. A failed directional fixture returns to the lead rather than
   silently changing the grade.
8. **(1201), MAIN 09:44:13 — session-local external activation.** Use an
   explicitly selected launch `--thinking-display summarized` path only after
   Phase 1 verifies interactive support and observable effective request mode.
   Rejected: settings mutation or another ws mode-control UI; the launch
   argument has a session lifetime and leaves existing/global state unchanged.
   Flag parsing alone is not provenance. A missing supported public surface
   returns to the lead, not a substitute setting write.
9. **(1202), MAIN 09:44:13 — source-specific negotiated support.** Reuse the
   bounded v1 projection/history types. Require both `reasoning_summary_v1`
   and `reasoning_summary_claude_v1` before external optional text. Source pairs
   are `claude-code/claude-code-mod-summarized` for external delivery and
   `claude-code/claude-code-stream-json-summarized` for hosted delivery. They
   identify the observed host/transport, not a guessed upstream vendor; each
   remains gated on its own verified provenance. Retain existing
   provider/API/model fields, strict ownership and one bounded summary event
   per intake record. These are approved future literals, not implemented API.
   Rejected: generic Pi capability as Claude support, OpenAI source spoofing,
   a new event kind, or a hello/protocol/schema bump.
10. **(1203), MAIN 09:44:13 — exact evidenced eligibility.** Initially admit
    only exact host/model/provider combinations whose public summarized
    provenance and request/attempt mapping the relevant phase demonstrates,
    beginning with Claude Code 2.1.325 evidence. Rejected: arbitrary-version
    or generic-thinking inference; an evidence-backed allowlist has an
    explicit unavailable failure mode. Phases 1 and 3 record actual observed
    identifiers and supported source surfaces, not identifiers invented from
    experiment display labels. Their reviewed output fixes exact eligibility
    and mapping before delivery; substantive changes require settlement and
    review before runtime edits. Never invent attempt 1 or an unexplained
    retry interpretation for a response/block reference.
11. **(1204), MAIN 09:44:13 — preserve observed cadence.** Keep the existing
    mod flush cadence and ordinary-event scheduling, with isolated bounded
    summary projections. Rejected: faster timers or artificial delta pacing;
    the short Opus burst does not establish incremental generation throughout
    the thinking interval.
12. **MAIN 10:07:22 — current acceptance and run scope.** Ready promotion and
    implementation cover Phases 1+2 as one head only; hosted Phases 3+4 stay
    deferred in this ticket. A summarized Claude lead must show each observed
    public summary paragraph in existing live activity and retained history
    after reload, within the retained bounds. Omitted summary option and
    empty/redacted/signature-only thinking must show no summary row, only
    ordinary activity, with tests. Preserve MCP tool/schema/stored-format
    semantics; Decision 7's directional T0/T1 targets remain binding, and stop
    if delivery would exceed T0/T1. Attach 1440-wide light and dark screenshots
    of positive live, positive reload and negative ordinary-only states for
    MAIN's pre-merge review. This supersedes the old no-screenshot constraint,
    not the existing display design. If interactive source lacks public
    summaries, stop at Phase 1 with redacted evidence; never fabricate text
    (current MAIN assignment supplied with this fact-population request).

The technical settlement IDs above were remapped from the draft's conflicting
(1094)–(1097) to (1201)–(1204). They do not reinterpret the owner-board answers
using the old numbers.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)

- This ticket does not authorize a new skill/agent.
- Retain the current 8 KiB text, 24 KiB projection, 49,152-byte JSONL frame,
  16-attempt and 64-part aggregate bounds, UTF-8 validation and truncation
  accounting. Retain positive safe request/attempt counters, monotonic revision,
  producer retirement, current open lead-turn identity and stale/overlap refusal.
  Reuse first-v1 ingress ordering and actual deferred-write fencing.
- Keep RAM-only live state, receiver-owned retry of the immutable natural-turn
  candidate, byte-preserving old ring append, normal paging/pruning and no
  replay/recount of ordinary replies/tools/lifecycle events.
- Unsupported version/source, absent summarized output, omitted mode, empty or
  signature-only blocks, redacted data, uncertain provenance, missing attempt
  identity and unsupported receiver are ordinary activity plus truthful summary
  unavailability, not launch failure and not permission to drop privacy gates.
- A hosted lead is not an external terminal producer. The mod is intentionally
  disabled for hosted sessions; the later hosted slice must use the existing
  backend stream mapper and actual holder/turn ownership, not turn that mod
  feed on or emit a duplicate producer. Hosted automatic activation is useful
  only with the later hosted summary delivery slice in this ticket.
- No global mode/config mutation, automatic migration of existing sessions,
  unrelated launch permission change, model/provider replacement, install,
  SDK upgrade or unapproved host launch. Read-only source inspection and fake
  protocol fixtures are not evidence of real interactive delivery.
- No new visual design/reference was approved. Reuse the existing text-only
  summary display and user-fold state. MAIN 10:07:22 now requires 1440-wide
  light and dark screenshots of positive live, positive reload and negative
  ordinary-only states for pre-merge review; no new visual reference is
  approved (current MAIN assignment, Decision 12).
- All retained test material is synthetic or explicitly redacted structure and
  aggregates. Do not commit raw thinking, signatures, account data, prompts,
  private transcript paths, full provider events or request/error bodies.
- MAIN owns develop integration and shared daemon replacement. Broad/long/full
  or Windows checks require the shared heavy slot. Scoped affected fixtures do
  not authorize a live provider launch or another experiment during capture.

## Prior Art and Source Evidence

Repository source rechecked at `67571a86bb4ab290aa91f5b689a5cb54ab3db268`
(previous inspection: `87e9d75dc`); `G/` below is
`agents-plugin-tool/internal/`, `M/` is `agents-plugin/hooks/claude-mod/`.
Evidence is not a completed acceptance run.

- Durable feasibility reference:
  `ai-docs/ref/claude-code-reasoning-summary-feasibility-261115.md`, committed
  `4760ec209`. Its narrative supplies supporting evidence, not implementation
  authority. The confirmed owner/MAIN decisions above supply authority.
- MAIN's completed bounded experiment, redacted `/tmp/think-exp/results.md`:
  Claude Code **2.1.325**, headless `-p --thinking-display summarized`, yielded
  Opus **43 deltas / 519 characters**, a burst in approximately **11 ms**;
  Sonnet **49 deltas / 606 characters** over approximately **2 seconds**.
  Omitted mode yielded zero; `showThinkingSummaries:true` alone in `-p` yielded
  zero. These aggregates are captured here so the ticket does not depend on
  retaining that temporary file. They prove neither interactive TUI delivery,
  per-request mode observability nor an arbitrary model/provider contract.
- `M/register.ts#L1687-L1707` taps `turn.step` using `${e.turnId}/${e.index}`;
  `tapStep` at `195–225` forwards chunks/results and isolates observer errors.
  `M/dashboard.ts:374–395,1579–1590` uses thinking chunks for phase/rate, while
  only text chunks become replies. The prior installed-source inspection found
  chunk block indexes but no effective thinking-display field on the step
  event; public retry-attempt attribution is also unverified.
- `M/dashboard.ts:94–118,1245–1362` has `dashboard_intake`, adapter `claude`,
  one-/two-second cadence and response-driven `accepts:` recovery. Do not assume
  existing status negotiation enables summaries. `M/register.ts#L1218-L1221`
  keeps that mod feed off for hosted sessions to avoid duplicate observation.
- `G/wsagent/claude.go:102–130` builds hosted print/stream-json arguments,
  currently without partial-message output; `1226–1277` maps assistant text and
  tool use, not summary text. A later hosted producer therefore requires a
  separately verified partial-stream contract, not only a launch flag.
- `G/mcp/summary_owner.go:11–43`, `live_snapshot.go:483–509,608–613` require an
  owned non-hosted Pi terminal receiver and adapter `pi`; generic admission
  cannot simply be widened without Claude-specific source/ownership gating.
  `G/wsagent/runner_summary.go:13–45,72–114` similarly binds hosted summaries
  to Pi leads, actual conversation generation and observed holder/turn state.
- `G/wslive/reasoning_summary.go:13–25,88–153` defines bounds and validates only
  the two existing OpenAI Responses source pairs. `195–303` governs live
  freshness. `G/wsdashboard/reasoning_summary.go:28–46` renders only validated,
  nonempty live/history summaries. Current Claude text would fail that source
  validation; a settings flag is not a display implementation.
- `G/wslive/ring.go:173–207` writes separate ring entries rather than rewrites;
  `summary_commit.go` owns frozen natural-turn retry. Existing historical
  fixture `summary_compat_test.go` pins `9854ff350` for the earlier Pi addition.
  It is an anchor to extend, not already a Claude-source skew test.

### Directional compatibility source trace

The selected old-side pin for the future Claude addition is
`87e9d75dcac7e1fa392725fe42934fe531105734` (prior inspected checkout), not the pre-Pi fixture pin
`9854ff350fa1b0eccfb5c71649c5190c405925b9`. References in this trace describe
the pre-Claude mechanisms, rechecked in the current source; the Claude-capable
new side and its directional fixtures
remain delivery targets, not completed acceptance tests. The design grades are
settled in Decision 7; their runtime acceptance remains required.

- **Old producer to new receiver, T0 target:** preserve ordinary Claude/Pi
  events and calls. The current Claude mod sends adapter `claude` and negotiates
  optional fields only from answered `accepts:` evidence
  (`agents-plugin/hooks/claude-mod/dashboard.ts#L94-L117`,
  `agents-plugin/hooks/claude-mod/dashboard.ts#L1245-L1362`). No Claude summary
  producer exists at these sites. A future receiver must demonstrate unchanged
  ordinary writes and reopen; optional fields alone do not prove that result.
- **New external producer to current receiver, T0 gated fallback target:**
  `summaryCapable` requires a real owned slot, Pi harness and non-hosted context;
  intake additionally rejects optional text unless adapter is `pi`, and only
  advertises the generic token for a capable, available receiver
  (`agents-plugin-tool/internal/mcp/summary_owner.go#L16-L19`,
  `agents-plugin-tool/internal/mcp/live_snapshot.go#L483-L509`,
  `agents-plugin-tool/internal/mcp/live_snapshot.go#L608-L613`). Withhold Claude
  projections rather than trigger that refusal. The approved future extra capability
  is absent from this current receiver, not already an implemented API. Hosted
  Claude has no equivalent producer or launch option here: `claudeLeadArgs`
  requests whole stream-json output, and `claudeMapper.assistant` handles text,
  tool calls and usage, not summary text
  (`agents-plugin-tool/internal/wsagent/claude.go#L102-L130`,
  `agents-plugin-tool/internal/wsagent/claude.go#L1226-L1277`).
- **Mutable snapshots, T0 data-preservation target:** `Snapshot.ForDisk` clears
  only live `Lead.ReasoningSummary`; `liveSession.finish`, `liveSession.fire`
  and `liveWriter.writeSnapshot` use it while RAM publication keeps the full
  projection (`agents-plugin-tool/internal/wslive/snapshot_disk.go#L3-L9`,
  `agents-plugin-tool/internal/mcp/live_snapshot.go#L308-L332`,
  `agents-plugin-tool/internal/mcp/live_snapshot.go#L725-L739`,
  `agents-plugin-tool/internal/wsagent/record.go#L481-L509`). Thus the current
  old writer encounters no persisted live Claude summary if the new writer
  retains this boundary. New live reopen must deliberately be blank, not
  reconstructed from history. The existing Pi-oriented reopen fixture is
  `agents-plugin-tool/internal/mcp/summary_snapshot_test.go#L156-L192`;
  Claude-path coverage remains a target.
- **New Claude history to current old reader/append to new reopen, T1 display
  target with retained bytes:** `ReadRing`/`ReadRingPage`/`readRingEntry` decode
  optional `SummaryHistory` without source validation or a read-time rewrite
  (`agents-plugin-tool/internal/wslive/ring.go#L94-L95`,
  `agents-plugin-tool/internal/wslive/ring.go#L287-L290`,
  `agents-plugin-tool/internal/wslive/ring.go#L309-L366`). The old dashboard's
  `ConvEntry.Summary` calls `ValidateSummaryHistory`, whose shared source check
  admits only the two OpenAI pairs, so a Claude-source history is hidden
  (`agents-plugin-tool/internal/wsdashboard/reasoning_summary.go#L42-L46`,
  `agents-plugin-tool/internal/wslive/reasoning_summary.go#L118-L119`,
  `agents-plugin-tool/internal/wslive/reasoning_summary.go#L138-L160`). Old
  `AppendRing`/`writeRingEntry`/`writeRingRaw` create separate temp/rename files;
  they do not serialize the old read record back over its original bytes
  (`agents-plugin-tool/internal/wslive/ring.go#L173-L207`,
  `agents-plugin-tool/internal/wslive/ring.go#L220-L243`). Ordinary pruning
  still removes oldest records above 2,000; live pages contain 50
  (`agents-plugin-tool/internal/wslive/ring.go#L249-L260`,
  `agents-plugin-tool/internal/wslive/snapshot.go#L79-L84`). For an unpruned
  record, the new reader must decode those identical bytes and its new validator
  must admit the verified Claude source before the existing renderer can show
  it. That new validator/reopen/display assertion is not implemented yet. This
  is display degradation, not T2 recovery from an old overwrite.
- **Fixture boundary:** `TestSummaryHistoricalReaderWriterPreservesOriginalBytes`
  builds the complete old module at `9854ff350`, reads and appends through its
  real `ReadRing`/`AppendRing`, compares original bytes, then reopens through the
  current reader and checks retained attempt/commit metadata
  (`agents-plugin-tool/internal/wslive/summary_compat_test.go#L18-L100`,
  `agents-plugin-tool/internal/wslive/summary_compat_test.go#L180-L195`). Its
  separate `54b252036f430ddd0886af90ba8de767bc4b1a6a` mapper pin tests additive
  Pi widgets, not a Claude producer (`agents-plugin-tool/internal/wslive/summary_compat_test.go#L104-L138`).
  Extend with the exact current old-side pin above and a future Claude-capable
  new reader/render assertion. Existing renderer negative-source and ordinary
  fallback assertions are anchors, not completed Claude skew acceptance
  (`agents-plugin-tool/internal/wsdashboard/reasoning_summary_test.go#L61-L104`).

## Prior Decisions

- 261114-feat-ws-live-reasoning-summary (2026-11-14, Decisions): "Live data uses an optional stage-free `agent.reasoningSummary` projection; terminal final data accompanies the existing real `turn_end`." — bearing: supports
- 83ba68d5 (2026-11-15, commit): "Decision967 requires RAM-only live reasoning summaries until immutable natural-turn history commits." — bearing: constrains
- 39052e47 (2026-11-15, commit): "The new 1.5.2 native bundle explicitly opts in via capabilities on the existing status widget; summary payload presence is never capability evidence." — bearing: constrains
- 5d018785 (2026-11-14, commit): "Fence optional mutation and candidate registration with the actual daemon slot generation plus the same registration lock used by terminal/hosted takeover." — bearing: constrains
- a5b05c09 (2026-11-14, commit): "Freeze accepted natural-turn candidates in receiver-owned session RAM and serialize immutable temp+rename retries independently of client slots." — bearing: constrains
- 72c9d851 (2026-11-11, commit): "An idle after a result is that turn's own and is ignored (mapper `open`), so it cannot close the next turn a flushed send already began." — bearing: constrains
- c2fd2c99 (2026-11-10, commit): "Chunks count only while the record's buffer streams that step, and begin() drops spans of a request that never finished" — bearing: constrains
- e4a68658 (2026-11-06, commit): "The feed is observe-only: every handler is guarded, turn.step is tapped only while the feed runs" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | This run: agents-plugin/hooks/claude-mod/register.ts and dashboard.ts, wsflow mirrors, agents-plugin-tool/internal/mcp/live_snapshot.go and summary_owner.go, internal/wslive/reasoning_summary.go and existing tests; internal/wsagent/claude.go and runner_summary.go belong to deferred Phases 3+4 |
| scope.surface | cross-module | External mod, MCP adapter intake, shared live/history validation and dashboard display; no MCP tool/schema/stored-format semantic change authorized |
| scope.new_public_symbol | yes | Decision 9's future reasoning_summary_claude_v1 capability and claude-code/claude-code-mod-summarized source pair; not currently implemented |
| scope.new_type_contract | yes | Claude-specific provenance, eligibility and request/attempt attribution extend source admission within existing bounded v1 types; Phase 1 evidence gates Phase 2 |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/dashboard.test.ts and dashboard-engine.test.ts; agents-plugin-tool/internal/mcp/summary_intake_test.go, summary_snapshot_test.go and intake_order_test.go; internal/wslive/summary_compat_test.go; internal/wsdashboard/reasoning_summary_test.go |
| complexity.reuse_points | confirmed | Read tapStep, createDashboardFeed negotiation, summaryCapable/lockSummaryOwner, SummaryCommits, AppendRing/ReadRing, ForDisk and ConvEntry.Summary; current Claude producer/admission absent |
| complexity.side_effect_risk | high | External stream observation and intake ordering affect ordinary activity, natural-turn settlement and retained history; hosted launch changes are deferred |
| risk.correctness | high | External effective mode and attempt mapping remain unverified; stale ownership or misattributed chunks could contaminate immutable history |
| risk.fit | moderate | Existing observer and display are reusable, but Pi-only gates require separately verified Claude source/ownership admission, not blanket widening |
| risk.test | high | Claude interactive provenance, explicit negative-row tests, old/new directional fixtures and six 1440-wide acceptance screenshots remain outstanding |
| risk.security_or_contract | high | Generic thinking is not public-summary evidence; privacy gate and T0/T1 directional proof must hold without MCP tool/schema/stored-format semantic changes |

## Phases

### Phase 1: Verify the external interactive public-summary source contract

This is the first worker-dispatched slice, explicitly authorized by MAIN
10:07:22 for bounded external interactive TUI/mod verification and the selected
session-local `--thinking-display summarized` launch, not an experiment
performed by fact population. Use that bounded interactive Claude
TUI/mod verification to establish effective session activation, public-summary
provenance, actual chunk timing, outer turn/request/block identity and supported
attempt boundaries. Observe the existing stream without altering its chunks,
results, permissions or input owner. Exercise a verified summarized session and
negative omitted/unavailable/empty cases; do not use a private thinking stream as
a test oracle or copy its contents into artifacts.

Deliver a redacted, reproducible evidence record with exact installed version,
actual model/provider identifiers, selected versus **effective** request mode,
source/transport callback shapes and ordering, retry/abort/continuation behavior,
and the specific supported public surface that proves provenance. Show what
survives reconnect and how unprovable overlap becomes unavailable. The headless
experiment is supporting evidence only. A mode-setting read, CLI argument or
nonempty generic thinking callback alone cannot pass this gate.

Before Phase 2, the lead settles any newly exposed contract choice and reviews
the now-checkable eligibility/source/attempt mapping. Missing public mode or
attempt evidence leaves Phase 2 unexecutable and returns the exact unsupported
surface to the lead; do not create an invented attempt, scrape a transcript or
weaken the gate. Verification and a gated no-go result are useful phase output,
not permission to silently discard the promised producer outcome.

Verification: record only redacted structure, counts, mode evidence and timings;
check existing-session/global settings remain unchanged; prove the observer does
not alter the stream or create another input owner. State every unverified case
and the bounded dispatch used. No passing fake fixture is called interactive
acceptance. This phase does not land hosted auto-enable or a cosmetic UI.

### Phase 2: Deliver the external interactive lead producer and guarded intake

Depends on a successful Phase 1 Result and settlement of its exact source
contract. Implement the confirmed per-session activation path and external lead
summary observation through the existing mod/intake owners. Admit only the
verified source/provenance and actual lead slot/registration generation. Keep
ordinary Claude/Pi traffic unchanged. Never accept a summary merely because a
caller supplied a source label or a model produced a thinking chunk.

Carry bounded current-turn projections through the existing live reducer and
text-only display, then freeze the actual natural turn's observed summary at
settlement. Preserve failed/interrupted prefixes, revisions/attempt separation,
producer retirement, immutable history, summary-free mutable snapshots and
receiver-owned retry. Honor source-specific negotiated support; unsupported
receivers keep ordinary activity and receive no selection-bearing summary
projection. An observed reconnect/server replacement revokes support until
renegotiated. Do not claim invisible reconnects are detected without evidence.

Verification: synthetic mod/engine fixtures cover per-session activation,
effective-mode gating, no raw/opaque text, empty/omitted/redacted/unverifiable
cases, multiple blocks/requests, retries/abort/unknown overlap, limits, reorder,
late/retired producer data and observed disconnect/reconnect. Receiver fixtures
cover actual owner/slot/turn fencing, single-event admission, legacy in-flight
write barriers, no replay/recount, RAM-only live data, immutable natural history,
old-source history readers/appenders and new reopen. Extend existing display
fallback tests; existing Pi source admission and display must remain unchanged.
Show each observed public summary paragraph in existing live activity and
retained history after reload, within the retained bounds. Explicit tests must
assert no summary row and unchanged ordinary activity for omitted summary
option and empty/redacted/signature-only thinking. Attach 1440-wide light and
dark screenshots of positive live, positive reload and negative ordinary-only
states for MAIN pre-merge review (MAIN 10:07:22, Decision 12). Preserve MCP
tool/schema/stored-format semantics and stop if directional compatibility
exceeds T0/T1. This run ends after Phases 1+2 as one head; do not execute hosted
Phases 3+4. If Phase 1 finds no supported public summaries, stop there with
redacted evidence instead of implementing a fabricated producer.

### Phase 3: Verify the hosted partial-stream public-summary source contract

After the external producer slice, separately verify the exact-version hosted
Claude stream-json/partial-message path under a later explicitly bounded
worker dispatch. The completed `-p` experiment does not prove the current
Runner/holder mapper's delivery, lifecycle or ownership. Establish which public
partial events carry summarized text and how configured mode becomes trustworthy
per-request evidence, including retries/fallback/abort and message duplication.

Record the exact supported argument/event contract, actual model/provider
identifiers, request/attempt/block mapping, real holder turn identity, observed
latency and negative/unsupported behavior. Keep the hosted mod feed disabled.
Validate that any partial-event option leaves ordinary text/tool/usage/permission
behavior intact. This phase does not land automatic summarized mode by itself.

Missing public provenance or safe attempt attribution leaves Phase 4
unexecutable and returns the exact unsupported surface to the lead. Settle and
review newly checkable hosted source/mapping choices before its delivery phase;
never infer support from flag acceptance or external mod evidence.

Verification: use redacted evidence and isolated mapper/holder fixtures for
partial/final duplication, retry/continuation, stream gaps, native settlement,
stale holder generations and preserved legacy events. A later live verification
requires its own bounded dispatch; no launch or account/config mutation is
performed by authoring or ready review.

### Phase 4: Deliver useful hosted summaries with automatic session activation

Depends on a successful Phase 3 Result and settled hosted source contract.
Implement **together** the approved `--thinking-display summarized` automatic
activation for newly dashboard-launched hosted Claude leads and the verified
backend/mapper producer that makes their summaries visible. Include verified
partial-message launch wiring only as required by that contract. Keep existing
sessions and global settings untouched, preserve the existing permission/input
owner, and do not enable the external mod feed for hosted sessions.

Use the existing holder/actual conversation generation and natural-turn owners,
not terminal ownership or a serialized label. Apply the same provenance/privacy,
bounds, truthful unavailable state and immutable-history rules as the external
producer. A launch flag without displayed summary delivery does not pass this
phase. Unsupported host versions/sources must not receive an unverified flag or
produce text; settle any required behavior outside the verified support contract
with the lead rather than silently change legacy launches.

Verification: isolated argument/mapper/Runner/holder fixtures cover automatic
selection only on supported new hosted launches, preserved old requests and
native permissions, actual source evidence versus absent/mismatched mode,
partial/final deduplication, request/attempt boundaries, abort/failure/gaps,
retired holders, current turn ownership and no duplicate input/producer. Verify
useful live and retained output, including interrupted prefixes, natural-turn
commit, reopen and old/new source compatibility. Re-run the relevant external
and Pi regression selectors. MAIN owns final merge/swap and owner live-screen
acceptance; the worker neither swaps a shared daemon nor calls the flag alone
acceptance.

## Blocked (2026-11-15)

Phase 1 stopped with worker stop (f) after two bounded interactive PTY-driver
attempts timed out (220 seconds and 40 seconds). Neither produced a
`session.start` observer aggregate; the second reached native interactive UI
but sent no input. No third launch or automatic retry was made. Interactive
public-summary provenance, effective mode and request/attempt mapping remain
unverified, so Phase 2 is not executable. This is a verification-driver failure,
not proof that every supported public surface lacks summaries or provenance.

Installed-source inspection of Claude Code 2.1.325 found `turn.step` thinking
chunks shaped as `{kind, index, text, ref}`, while internal stream envelopes
carry `thinkingDisplay`. No Phase 1 Result, runtime edits, settings mutation,
acceptance screenshots, tests, independent implementation review or Phase 2
implementation was completed. The settings before/after comparison also did
not complete; unchanged settings are not certified by this attempt.

The worker found no matching probe processes, and the second probe PID was
absent; no cleanup signals were needed. The first probe PID was not captured,
so complete descendant cleanup is not independently certified. Scratch evidence
is temporary and is not an implementation dependency. MAIN received the stop;
a new bounded verification approach or explicit re-dispatch is needed before
continuation. Hosted Phases 3+4 remain outside this assignment.

## Focused Verification Contract

These are existing anchors to extend and proposed acceptance names, **not checks
run during capture**. Read complete affected output; reject empty selector runs.
Use the installed `claude-code/testing` harness already imported by mod tests;
do not invent a Node-only runner, launch a provider, install a harness or run a
broad suite merely to discover a test command.

- Mod/engine anchors: `agents-plugin/hooks/claude-mod/dashboard.test.ts`
  (feed, observed response evidence and delivery groups) and
  `dashboard-engine.test.ts` (`turn.step`, intake recovery and observer
  forwarding fixtures). Mirror applicable shipped package surfaces under the
  mirroring manual; source drift checks are not real TUI verification.
- Go anchors: `TestSummaryIntakeNegotiatesOnlyOwnedDaemonReceiver`,
  `TestSummaryIntakeRetiredConnectionAndHostedTakeoverCannotWrite`,
  `TestSummaryIngressTracksAbandonedLegacyAndActualDeferredWrite`,
  `TestSummarySnapshotNaturalHistorySurvivesReopenWithoutLiveRecovery`,
  `TestSummaryCommitFreezesRetriesAndNeverRecounts`,
  `TestSummaryHistoricalReaderWriterPreservesOriginalBytes`,
  `TestSummaryPagingPruningAndNoResurrection` and
  `TestReasoningSummaryHistoryImmutableTurnAssociationAndAttempts`.
- Proposed new external selectors under `internal/mcp`, `internal/wslive` and
  `internal/wsdashboard`: `^TestClaudeSummary(External|Provenance|Negotiation|Ownership|Lifecycle|Compatibility)`.
  Cover both source-positive and source-negative cases; count subcases, not just
  a nonempty package pass.
- Proposed new hosted selectors under `internal/wsagent`:
  `^TestClaudeSummaryHosted(Args|Mapper|Provenance|Ownership|Lifecycle|Compatibility)`.
  Include the useful auto-enable-plus-delivery behavior and unchanged legacy
  native controls. Do not call a launch-argument assertion a producer test.
- Pin the old reader/writer source used by each directional fixture and cite
  actual functions. For new Claude history, compare original bytes after old
  read/append, then reopen with the new reader and assert identical text,
  source/attempt association and renderability. For ordinary old records,
  assert unchanged fallback. Keep live summary absent from all mutable disk
  snapshots and distinguish deliberate blank live reopen from retained history.
- Summarize external and hosted acceptance independently. Neither path's result
  certifies the other, arbitrary Claude versions/providers, a universal live
  streaming cadence or a new privacy/authentication guarantee.
