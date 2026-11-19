---
title: "호스티드 compaction 수명주기: native ws/compact 레버, 감지, compaction 후 컨텍스트 재주입, 데몬 넛지"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-harness-capability-catalog: "prerequisite; its compaction item grade (native / ws-substitute) selects the fallback; its Decision 6 defers the ws-substitute build to this ticket"
  261117-feat-ws-single-session-holder: "reattach after daemon restart targets the newest harness-session segment"
  261117-feat-ws-exec-tools: "daemon-owned exec jobs survive compaction; the restore bundle lists them"
  261117-feat-ws-agent-tools-mailbox-routing: "children and lineage keyed by the ws key; compaction applies to children too"
  261117-feat-ws-hosted-prompt-assembly: "sibling stage-2 child; shares the daemon injection path"
  261116-research-ws-hosted-session-first-tier: "evidence and Confirmed Decisions"
  261119-feat-ws-new-session-compaction-segment-chain: "idea split out by the owner redirect of 2026-11-19 15:18: the Pi-style new-session method, segment chain and (31)/(35)/(36)/(37) background"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: dff1690277918e94
sage-review-completeness-reviewed: dff1690277918e94
---

# Hosted compaction lifecycle

## Background

Part of owner 18:35's "application layer" on top of the common harness
interface, placed in stage 2 by (28)=a: (1) a daemon-owned compaction nudge, and
lead addition A, a daemon session-state restore without the lead-revive ritual.
This ticket also builds `ws/compact`, the common compaction lever. Owner
redirect 2026-11-19 15:18 (Decision 13) makes harness-native compaction THE
method: `ws/compact` triggers the harness's native compaction, and the work here
is that lever, compaction detection, the nudge, and post-compaction
re-injection. The Pi-style new-session method (summary in our format, new harness
session, segment chain) moved to the idea ticket
`261119-feat-ws-new-session-compaction-segment-chain`.

Owner 18:39 goal: the nudge and the state restore apply to child agents too, not
only leads. Subagent compaction timing, notably for Pi children, was hard; the
expected gains are subagent quality and token efficiency.

Evidence of today's state:

- Pi compaction (`agents-plugin-pi/src/lead-compaction.ts`, `goal-loop.ts`):
  the session writes prose through the `ws-compact` lever under seven `###`
  sections (`LEAD_PROSE_SECTIONS`: User preferences and style; Agreed working
  practices; Decisions not yet recorded; Current work; Immediate next step; Mood
  and rapport; Residual details). The adapter adds fixed sections: `## Session`
  (ws key, ticket, playbook), `## Carried forward by the lead`, child agents in
  flight and finished, background tasks, `## Dialog` near-raw under a byte
  budget, required re-reads inlined, on-demand references, and `## Resume`.
  `session_before_compact` returns the extension's own compaction, bypassing
  Pi's summarizer; without lever prose a separate tool-less model call is the
  fallback (`FALLBACK_SYSTEM_PROMPT`). Compaction is in place (`ctx.compact()`);
  no new-session path exists. Agenda and todos are not in the summary
  (`lead-revive` restores them).
- The daemon has no mechanism to start a new hosted harness session that
  inherits an existing ws session key. Nearest: Claude `--resume spec.SessionID`
  (`wsagent/claude.go`) and handoff TakeOver/HandOff (`wsagent/handoff.go`). The
  daemon only observes compaction levers and `compact_boundary`
  (`wsagent/backend.go`, `claude.go`).

## Decisions

Confirmed. Decisions 4-8 were queued and acknowledged by the owner on
2026-11-17 ("32..37=ok").

**Scope boundary.** The catalog ticket grades compaction per harness and routes
the existing `/compact` command through the catalog; this ticket owns
`ws/compact` as the native-compaction lever (Decision 13), and the nudge, detection and restore.

1. **Scope placement (28)=a.** The nudge (1) and state restore (A) are stage-2
   children of the epic.
2. **Nudge and restore apply to child agents too (owner 18:39).** Not only
   leads.
3. **[superseded by Decision 13] (31)=a.** Was: the default is a Pi-format
   summary, a NEW harness session and an inherited ws key. Moved to the idea
   ticket `261119-feat-ws-new-session-compaction-segment-chain` as background.
4. **(33), owner ack 2026-11-17: (28)=a's single "hosted prompt assembly" child
   is split into two tickets:** prompt assembly (2, 3, B, C;
   `261117-feat-ws-hosted-prompt-assembly`) and compaction lifecycle (1, A,
   ws/compact; this ticket). Rejected: one ticket, because compaction is a
   separate runtime mechanism with different verification.
5. **(34), owner ack 2026-11-17: ws/compact runs only at an idle turn
   boundary.** Rejected: running it mid-turn, because in-flight harness tool
   calls and harness-side background shells die with the old session
   (daemon-owned ws/exec jobs survive).
6. **[superseded by Decision 13] (35)** segment chain: moved to the idea ticket.
7. **[superseded by Decision 13] (36)** overflow fallback to native or a headless
   summarizer: moot while native compaction is the method; background in the idea
   ticket.
8. **[superseded by Decision 13] (37)** Pi-format summary: background in the idea
   ticket. The nudge no longer asks for summary prose (Decision 13).

9. **(186)=a, owner 2026-11-19 15:14:** if a compaction command can be
   sent, implement `ws/compact` in Phase 1 by delivering the native signal.
   Originally staging toward (31)=a; Decision 13 makes it the final method.
10. **Thresholds, owner 2026-11-19 15:15–15:16:** four common parameters:
    lead advisory, lead hard limit, worker advisory, worker hard limit. Resolve
    each per harness and per normal config layer, with one shared global
    default set and no per-harness defaults. Preserve the existing threshold
    grammar and old Claude/Pi keys via migration reads. Lead's reading:
    advisory nudges at the next idle boundary; hard limit makes the daemon
    issue `ws/compact` at the next idle boundary.
11. **Implementation details (autonomous):** use scalar keys
    `compaction.lead_advisory`, `compaction.lead_hard`,
    `compaction.worker_advisory`, `compaction.worker_hard`, harness-bucketed
    in existing string overrides using `<key>.<harness>` for a harness
    override and `<key>` for an unbucketed layer value; global defaults 50%,
    80%, 50%, 80%, respectively. 50% and 80% are Pi's current defaults and 50% is Claude's advisory;
    Claude's native automatic point is dynamic, not an 80% knob. Shared
    defaults avoid harness-specific policy. Explicit common values win over
    legacy keys at the same layer; otherwise read the matching legacy lead key
    at that layer before falling through. Workers use shared defaults unless
    explicitly tuned; do not map lead-only legacy settings to workers.
    Preserve Claude's advisory-enabled opt-out for hosted advisory; it does
    not disable hard compaction. Restore content is Decision 16, not the Phase 2 bundle.
    These choices reuse existing context and avoid a new persistent shape.
12. **Execution:** Pi worker (owner 2026-11-19 14:33); board topic
    `daemon-compaction`.
13. **Owner reversal, 2026-11-19 15:18 (supersedes (31)=a and (35)).** Gist of
    the owner's words: since the native-compaction fallback must be kept in mind
    anyway, the compaction nudge keeps only "write it to your agenda", the
    Pi-style method (summary in our format + new harness session + segment chain)
    moves to an idea ticket, native compaction is used (the lever is provided),
    and the focus is compaction detection and post-compaction context
    re-injection. Consequences: (a) native compaction is the method; `ws/compact`
    is the common lever that triggers the harness's native compaction, still only
    at an idle boundary (34). (b) The nudge text tells the session to write
    durable state to its agenda before compacting; it asks for no Pi-format
    summary prose. (c) The segment chain and new-session phase are removed from
    this ticket into `261119-feat-ws-new-session-compaction-segment-chain`,
    which carries (31)/(35)/(36)/(37) as unconfirmed background, with the segment
    chain's compatibility question. (d) The restore bundle (agenda, ticket/plan,
    children, exec jobs) is now Phase 2 and no longer waits on a segment chain.
14. **Worker native delivery (autonomous).** Where a harness's native command is
    available to workers, `ws/compact` and the hard limit use it; the Pi
    lead-only gate (`pi.go:555`) is lifted only if the Pi RPC `/compact`
    is safe for a worker, otherwise that role reports unsupported through the
    descriptor. Unsupported is reported explicitly and the worker still
    receives the advisory nudge. This reuses the descriptor capability rather
    than adding a worker-only path. Supersedes the worker-scope line of
    `261002-feat-pi-lead-ws-owned-compaction` ("spawned worker and explore sessions
    keep Pi's native compaction"): workers now get the advisory nudge and a
    daemon-issued native compaction where supported; the compaction itself stays
    native, so no contract of that ticket is reversed.
15. **Shared threshold resolver and phase boundary (autonomous).** The four
    keys are read by one layer-major resolver (at each layer: harness bucket,
    bare common value, applicable legacy key; then the next layer; shared default
    last) used by both `config.get` and the daemon's nudge, with `config.tune` still writing
    `<key>.<harness>` entries for keys that are not `HarnessBucketed()` (split the
    read and write flag if needed); they are not routed
    through the bucket-major `resolveHarnessBucketed` path
    (`mcp/config_registry.go:217-224`), which would let a global harness bucket
    beat a project bare value. Duplicate-advisory suppression is hosted-only,
    keyed off the existing hosted-session discriminator; the CLI-tier hooks and
    `lead-revive` stay untouched. If a probe finds no lifecycle signal for a
    harness, that harness is reported unsupported for detection and the phase
    still completes for the others after the gap is reported to the lead.
16. **(187)=a, owner 2026-11-19 15:30: post-compaction re-injection content.**
    A short note on every harness (ws key, "context was compacted", pointer to
    `workflow_state`); the full hosted manual only where the harness descriptor
    says its prompt does not persist through compaction. The flag is verified by
    the Phase 1 probe. Rejected: always the full manual (duplicates the persisted
    appended prompt, ~25 KB per compaction, consumes one-shot manual nudges);
    nothing. This replaces Decision 11's "re-inject the same session-start
    manual" wording and Phase 1 step 3's content.
17. **(188)=a, owner 2026-11-19 15:30: hosted Pi extension.** Hosted mode
    suppresses only the Pi extension's threshold nudges and hard-cut steer (the
    daemon owns them); the `ws-compact` lever and `session_before_compact` stay
    until `261119-feat-ws-new-session-compaction-segment-chain` decides.
    `agents-plugin-pi/src` joins Phase 1's span. Rejected: daemon defers to the
    extension; suppressing the whole extension path.
18. **(189)=a, owner 2026-11-19 15:30: `ws/compact` contract.** Args: `target`
    (optional; default self. Implementation detail: it accepts the spawn address
    `id:<id>` the caller owns, resolved and ownership-checked the way
    `agent.interrupt`/mailbox control resolves a control key, because
    `agent.spawn`/`pi.spawn` return only that address and never the child's ws
    session key (`wsagent/agent_tools.go:136-139,226`, `mcp/pi_forward.go:172`);
    a session key is also accepted where the caller holds one. On child
    connections, which carry no hosted identity, self/descendant checks rest on
    the presented key: the known gap of `261118-feat-ws-hosted-child-connection-identity`,
    not a new authority hole) and `resume` (bool, default true: after confirmed
    compaction and re-injection the daemon starts one continue turn; `resume`
    defaults true only for a session's own `ws/compact` call. Daemon-issued
    hard-limit, dashboard `/compact` and native-auto compactions inject the
    Decision 16 note and start no turn, since they were issued at an idle
    boundary and interrupted no work; each case is tested). Result
    `queued | unsupported | error` plus a reason. Old daemons refuse the unknown
    tool; callers gate on that. Rejected: no target/no resume; no target.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/performance.md (declared for agents-plugin-tool/)
- On the office Windows PC, Go builds/tests run only on remote w5090 via
  `D:/sys_lock.sh remote w5090 run ...`; no local `go test`.
- Read before editing `agents-plugin-tool/`: `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`; for `agents-plugin-tool/internal/mcp/`
  also `ai-docs/manuals/ws-mcp.md`. If playbooks, skills, `agents-plugin/rsrc/`
  or `agents-plugin-wsflow/` text is touched, also read
  `ai-docs/manuals/skill-authoring.md` and `ai-docs/manuals/wsflow-mirroring.md`.
- Release-gate file-touch rule (owner, 2026-11-17): minimize daemon file touches
  at the root; files for backup/restore only; deltas (append) not rewrites; no
  heartbeat rewrites; no per-event O(N) rereads; interactive paths never wait on
  file I/O; judged by measured syscalls/s on the owner's PC. The nudge's turn
  and token observation must not add per-event file writes; any state this
  ticket adds is appended, not rewritten. Related, not edited from here:
  `261117-research-ws-daemon-file-touch-map`.
- Shipped playbooks stay harness-neutral and must not name this repository's
  specifics; the Claude CLI tier is frozen (its existing lever and `lead-revive`
  path are untouched).
- Identity, authority, lineage and mailbox delivery ownership are preserved
  across compaction: the compacted session keeps its ws key and never gains
  authority.

## Compatibility

**Phase 1 target: T0, conditional on the specified implementation and skew
verification.** No session record, event, or config-manifest schema shape is
changed; new scalar threshold values must use existing string `overrides`.
The old config reader loads that string map
(`agents-plugin-tool/internal/wsconfig/config.go:37-47,83-90`); old resolver
writes change individual map entries (`agents-plugin-tool/internal/wsconfig/resolver.go:193-213,299-310`),
and repo writes retain other entries (`agents-plugin-tool/internal/wsconfig/repo.go:93-136`). Legacy thresholds remain readable and writable;
new reads resolve them without destructive migration. New `ws/compact` is an
additive command: old calls stay unchanged and new callers must gate against
older daemons (unknown calls are refused with -32602 "unknown tool": `mcp/server_dispatch.go:189-190,1697-1698`; line 1662 is a workflow.lang resolve, not the refusal).
Reopening with the new daemon reads the retained scalar values, not a rebuilt
session record. **Do not claim T0 solely from no session-shape change:** verify
old global/project writers preserve new harness-bucketed scalar entries and
that existing old-key writes still affect the new resolver; if either fails,
re-grade before release. No new manifest fields: the old reader rejects them
(`mcp/config_manifest.go:172-185`).

Source trace (track/agent-v2): old `setOverrideInFileRMW` (`wsconfig/resolver.go`,
`setOverrideInFile`) mutates only `cfg.Overrides[itemKey]` inside the locked
`updateConfigFile`, and `deleteOverrideInFile` deletes only its key, so an old
writer keeps new `compaction.*` entries; the new resolver reads legacy keys at
the same layer, so old-key writes still take effect. Phase 1 is T0 on that
trace; the implementer re-confirms with an old/new round-trip test.

Phase 2 (restore bundle) adds no session-record shape; settle its tier before
implementation. The segment chain's record-shape tier left with
`261119-feat-ws-new-session-compaction-segment-chain`.

## Prior Decisions

- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, Decisions): "Scope: the lead session only. Spawned worker and explore sessions keep Pi's native compaction; they are structurally short-lived and rarely compact." — bearing: contradiction-candidate
- 261113-feat-ws-compaction-before-queued-followup (2026-11-13, Decisions): "Scope: hosted sessions and claude-mod sessions' dashboard-delivered (board) messages ... Pi's own ws-compact lever ... is not changed here" — bearing: constrains
- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, Decisions): "Two thresholds. At the advisory threshold (compaction_advisory_percent, default 50) the adapter nudges the lead once, at a turn boundary (agent_end). At the hard threshold (compaction_hard_percent, default 80) ..." — bearing: supports
- 261016-feat-pi-compaction-token-thresholds (2026-10-16, Decisions): "Lead-facing nudge and reminder text reports the threshold in both tokens and percent" — bearing: constrains
- 261017-feat-claude-mod-lead-compaction-timing (2026-10-17, Decisions): "No threshold message while a lever request is pending." — bearing: constrains
- 261117-feat-ws-harness-capability-catalog (2026-11-17, Decisions): "Compaction is a common command delivered from outside the harness (07)... Claude and Pi are graded native" — bearing: supports
- 261114-epic-ws-common-session-foundation (2026-11-17, 6615ab3f): "(31)=a: ws/compact (Pi summary format + new harness session + key inheritance) is the default compaction on every harness; native compaction is only the fallback." — bearing: superseded by this ticket's Decision 13 (owner reversal 2026-11-19 15:18); the epic's text needs a knock-on edit by the lead
- 261112-bug-ws-hosted-context-window-ignores-compaction-window (2026-11-12, d6598b44): "post-/compact token count still updates only at the next model call." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/{wsagent,wsconfig,mcp,wsdashboard}, agents-plugin/config-manifest.json, agents-plugin-pi/config-manifest.json, agents-plugin-pi/src, claude-mod hooks |
| scope.surface | cross-module | new ws/compact common command (target, resume), four compaction.* config keys, descriptor/mapper fields across wsagent, wsconfig, mcp |
| scope.new_public_symbol | yes | ws/compact command; compaction.lead_advisory, compaction.lead_hard, compaction.worker_advisory, compaction.worker_hard |
| scope.new_type_contract | yes | descriptor compaction command/lifecycle fields; ws/compact input (target, resume) and result fixed by Decision 18; descriptor lifecycle fields remain open |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent (114 test files on track/agent-v2, e.g. agy_fixture_test.go, agy_test.go), mcp/config_manifest_test.go; new live fixtures also required by step 1 |
| complexity.reuse_points | confirmed | wsagent/catalog.go ItemCompaction, claude.go compact_boundary mapping, pi.go compaction_start/end, mcp/config_registry.go resolveHarnessBucketed (reference only; Decision 15 uses a layer-major resolver instead) |
| complexity.side_effect_risk | high | daemon injects into live sessions, may auto-issue compaction at hard limit, must add no per-event file writes |
| risk.correctness | high | latching, exactly-once restore, stale usage, simultaneous crossings; agy and Codex lifecycle signals unproven |
| risk.fit | moderate | descriptor-only dispatch required; worker native compaction unsupported on Claude and Pi today |
| risk.test | high | needs live agy and Codex probes and syscalls/s measurement; Go tests only on remote w5090 |
| risk.security_or_contract | moderate | new config keys through existing string overrides (T0 conditional), additive command, gated on older daemons |

## Phases

Phase 1 is the implementation contract below; Phase 2 is todo-level and staged
later. Phase 1 delivers native `ws/compact`, detection, the nudge to write the
agenda, and post-compaction re-injection (Decision 16).

### Phase 1: Native ws/compact, detection, re-injection and daemon nudge

1. **Probe first.** Record reproducible live agy and Codex app-server fixtures
   for native command and compaction start/end, including autonomous native
   compaction and failure. Existing evidence is not proof of all signals:
   agy sends `/compact` but its fixture records no event
   (`agents-plugin-tool/internal/wsagent/agy_fixture_test.go:309-333`,
   `agy_test.go:322-333`); Codex sends `thread/compact/start`
   (`wsagent/codex.go:381`) and maps `thread/compacted`
   (`wsagent/codex_mapper.go:189`). Also record, per harness, whether the appended system prompt survives native compaction (the descriptor flag of Decision 16). If no native command exists, advertise
   unsupported rather than simulate it. If no reliable lifecycle signal is
   observed, do not fabricate completion or claim automatic detection; record
   the limitation and bring the gap to the lead before implementation.
2. **Common command.** Add `ws/compact` to the hosted common command/dispatch
   surface; resolve command capability and native delivery via harness
   descriptor/catalog, never a harness-name conditional in common code.
   Queue at an idle turn boundary, for leads and workers; do not kill or steer
   in-flight work. Claude uses existing `/compact` routing
   (`wsagent/catalog.go:362`, `wsagent/claude.go:1004-1012`); Pi's existing
   RPC `/compact` maps to its compact command (`wsagent/pi.go:543-571`; the dashboard command path is lead-only, `pi.go:555` `p.lead && cmd.Name == "compact"`, and a Claude worker reports no command list so the dashboard refuses `/compact`, `wsagent/catalog.go:364`: a worker's native delivery is unsupported today, so descriptor routing must report it explicitly),
   and the extension's in-process lever uses `ctx.compact()`. Route unsupported
   and errors explicitly; no blind retry or duplicate command.
3. **Detect and restore.** Put per-harness native start/end mapping in the
   descriptor/mappers, including auto compaction. Claude's status/compact
   boundary are mapped at `wsagent/claude.go:1004-1012`; Pi already maps
   `compaction_start`/`compaction_end` (`wsagent/pi.go:959-968`). Drive the
   existing dashboard compacting phase (`wsdashboard/assets/dashboard.tmpl:189`)
   and clear it on success/failure. On confirmed successful end, inject the Decision 16 re-injection
   content for that same ws key, once, before the next ordinary queued message;
   honor `resume` (Decision 18). Use the existing renderer/instruction
   assembly (`wsagent/runner_process.go:260,801-804`); do not mint a key.
   Do not inject a Phase 2 agenda/children/exec restore bundle here.
4. **Thresholds.** Implement the four Decision 11 values and per-layer,
   per-harness resolution in the config registry/resolver and tune/catalog
   surfaces (`mcp/config_registry.go:98-108,138-139`,
   `wsconfig/config.go:37-47`). Respect the usual layer precedence: at each
   layer use explicit common harness bucket, then explicit unbucketed common
   value, then applicable explicitly set legacy key, then proceed to the next
   layer; shared global default last.
   Keep old keys declared and functional for old clients and CLI tier; no
   destructive migration. The nudge text tells the session to write durable
   state to its agenda and to call `ws/compact` (not the Pi extension's `ws-compact`
   lever) before compacting and asks for no summary prose; it
   replaces any hosted advisory text that asks for lever prose; in hosted mode the Pi extension's threshold nudges and hard-cut steer are suppressed (Decision 17), and reports the
   threshold in both tokens and percent (`261016-feat-pi-compaction-token-thresholds`). Use the daemon's
   existing per-turn context-usage observation (locate it by search; 
   `wsagent/claude.go:1252-1259` is the context-window ask, not the consumer)
   for both roles and all harnesses, not
   plugin-side hosted advisory. Missing/stale usage must not cause a guessed
   hard trigger. Hard overrides advisory when both cross, latch each crossing,
   reset on confirmed compaction or usage falling below threshold, and avoid
   repeated idle commands on unsupported/error until a new crossing or explicit
   user retry. Do not add per-event file writes.

Paths abbreviated `wsagent/`, `wsdashboard/`, `mcp/` and `wsconfig/` above
are under `agents-plugin-tool/internal/`.

**Existing knobs and readers.** Claude's
`claude.compaction_advisory_percent` defaults to `50%`, and
`claude.compaction_advisory_enabled` defaults true
(`agents-plugin/config-manifest.json:6-16`); the mod reads the enable flag
(`agents-plugin/hooks/claude-mod/register.ts:1241-1260`) and advisory value
(`agents-plugin/hooks/claude-mod/timing.ts:16`, `compaction.ts:444-447`).
Claude's hard point is the harness automatic compaction point, not a separate
config knob. Pi's `pi.compaction_advisory_percent` and
`pi.compaction_hard_percent` default to `50%` and `80%`
(`agents-plugin-pi/config-manifest.json:50-61`); the Pi goal-loop extension
parses/resolves them (`agents-plugin-pi/src/goal-loop.ts:198-249,1512-1513`).
Both manifests accept percent, token count `k`/`m`, or one of each separated
by `|`, whichever occurs first. Do not confuse this separator with the four
separate settings. Existing daemon usage observation is not a threshold reader.

**Verification:** fixture tests for all four harness descriptors and both
roles, including unsupported command and measured missing signals; native and
command-triggered start/end, failure and exactly-once restore; badge transitions
and ordering before queued work; idle-only command/nudge during active turns,
children, threshold crossings, simultaneous crossings, latching/reset, stale
usage, error/unsupported. Test percent/token/combined grammar and each layer,
harness override, all four shared defaults, old-key reads and writes, old/new
config round trips and manifest parity. Confirm no duplicate hosted plugin
advisories while CLI behavior remains unchanged. Measure syscalls/s to verify
no per-event writes; run Go builds/tests only on remote w5090.

### Result (c65451174) - 2026-11-19

Implemented Phase 1 across `agents-plugin-tool/`, `agents-plugin-pi/`, `agents-plugin/`, and `agents-plugin-wsflow/`:

- **Descriptor-driven native compaction & `ws/compact`:** Added `CompactionProtocol` on `HarnessProfile` / `BackendRegistration` (`wsagent/catalog.go`, `wsagent/registration.go`, `wsagent/compaction.go`) so common code never branches on harness names. Added the additive `ws/compact` MCP tool (`mcp/compaction.go`, `mcp/server_dispatch.go`, `mcp/server_schemas.go`, `wsagent/children.go`, `agents-plugin*/runtime.json`, `agents-plugin-pi/test/bridge.test.ts`) accepting optional `target` (self key or owned descendant key / `id:<reply-id>`) and `resume` (default `true` for self, `false` for descendant), queuing at idle boundaries only and returning `queued | unsupported | error` plus a reason. Added `compact` and `agenda.set` to `PiWorkerWSTools` (propagated to `PiMiniLeadWSTools` and `AgentWSTools`).
- **Detection, badge, idle-timer guard & post-compaction restore:** Mapped native compaction start/end/failure events into `wslive.KindCompact` (driving the existing harness-neutral `phase.compacting` / `"압축 중"` dashboard badge for any harness whose descriptor declares `Lifecycle: true`). Confirmed `CompactEnd` injects the Decision 16 short note (ws key, `"Context was compacted"`, `workflow_state` pointer; full manual only when `!PromptPersists`) once before the next ordinary queued work, honoring `resume`. Fixed the worker idle-timer edge so active compaction (`c.compacting || c.compactCommand`) stays ineligible for idle shutdown until both `CompactEnd` and the command response settle (`wsagent/compaction.go:246-247`, `wsagent/runner_events.go:715-718`, `TestCompactionIsNotIdleTimeoutEligible`).
- **Shared layer-major thresholds & hosted Pi suppression:** Added `compaction.lead_advisory` (50%), `compaction.lead_hard` (80%), `compaction.worker_advisory` (50%), and `compaction.worker_hard` (80%) resolved layer-major (`session` -> `project` -> `repo` -> `global`; within each layer: `<key>.<harness>` -> `<key>` -> applicable lead legacy key) in `wsconfig/compaction.go`, `mcp/compaction.go`, `mcp/config_registry.go`, `mcp/config_manifest.go`, and `mcp/server_config.go`. Threshold observation and crossing latches stay strictly in memory with zero per-event file writes (`TestCompactionObservationMemoryOnly`: 100,000 observations, 0 instrumented file operations). Hosted Pi suppresses only the extension's threshold nudges and hard-cut steer while retaining `ws-compact` and `session_before_compact` (`agents-plugin-pi/src/goal-loop.ts`, `agents-plugin-pi/src/index.ts`).
- **Live probe evidence (`wsagent/testdata/compaction-probes.md`):**
  - **Claude 2.1.327** (`claude-compaction-probe.jsonl`): verified `status: compacting` and `compact_boundary`, `PromptPersists: true`, `LeadCommand: true`, `WorkerCommand: false`, `Lifecycle: true`.
  - **Pi 1.5.0** (`pi-compaction-probe.jsonl`, `pi-compaction-failure-probe.jsonl`): verified session-scoped RPC `compact` for lead and worker, `compaction_start`/`compaction_end` (including `aborted`/`errorMessage`), `PromptPersists: true`, `LeadCommand: true`, `WorkerCommand: true`, `Lifecycle: true`.
  - **agy 1.5.12** (`agy-1.5.12-compaction-probe-1.jsonl`, `agy-1.5.12-compaction-probe-2.jsonl`, plus a third independent live 14,845 -> 11,149-token probe with `--log-file` and transcript check): `/compact` executes real compaction (`outputTokens: 597`, context shrinkage, `cachedReadTokens: 0`) and `systemPrompt.append` persists (`PromptPersists: true`), but `agy 1.5.12` emits no compaction start/end/failure notification over ACP stdio, stderr, `--log-file`, or `hooks.json` (only `usage_update` + `stopReason: "end_turn"`; internal `SYSTEM CHECKPOINT` stays in private `transcript.jsonl`). Per Phase 1 step 1 and Decision 15, `AgyCatalog().Compaction.Lifecycle` stays `false` (no fabricated completion); `ws/compact` returns `unsupported` while legacy dashboard `/compact` and advisory nudges remain available.
  - **Codex 0.170.0** (`codex-0.170.0-compaction-probe-1.jsonl`, `codex-0.170.0-compaction-probe-2.jsonl`): both attempts failed at `thread/start` with `-32600: failed to load configuration: invalid transport in mcp_servers.ws` (probe override validation failure). `CodexCatalog().Compaction.Lifecycle` and `PromptPersists` remain `false` until measured cleanly.
- **Compatibility (T0):** No session record, event, or config-manifest schema changes (`mcp/config_manifest.go:172-185`). New `compaction.*` thresholds live in existing string `overrides` (`wsconfig/config.go:37-47,83-91`); old per-key writers preserve them (`wsconfig/resolver.go:193-214,299-314`, `wsconfig/repo.go:92-145`) and old-key writes still take effect via layer-major reads (`wsconfig/compaction.go:66-109`, verified by `TestCompactionLayerMajorAndLegacyRoundTrip`). Additive `ws/compact` is refused with `-32602` unknown tool on older daemons (`mcp/server_dispatch.go:189-191,1704-1706`).
- **Verification:**
  - Remote w5090 (`job=7417db5ece134e4da763f355f697e2fa`): `python3 scripts/generate-package-assets.py`, `go build ./...`, `go vet ./internal/... ./cmd/...`, and `go test ./internal/wsagent/... ./internal/mcp/... ./internal/wslive/... ./internal/wsdashboard/... ./internal/wsconfig/... ./cmd/ws-mcp/...` all passed (`wsagent`, `mcp`, `wsconfig`, `cmd/ws-mcp` cached from identical synced tree; `wslive` 16.186s; `wsdashboard` 278.882s).
  - Pi unit tests (`agents-plugin-pi`): `node --test test/goal-loop-compaction.test.ts test/bridge.test.ts` passed (123 pass, 0 fail, 1 skipped opt-in live test); prior filtered Pi suite passed with 25 base-failing exclusions.
- **Forwards:**
  - **agy ACP compaction lifecycle:** `agy 1.5.12` emits no compaction lifecycle signal over ACP (`usage_update` + `stopReason: "end_turn"` only). Re-probe once agy exposes an ACP compaction start/end notification, or decide explicitly whether command-response-only tracking should ever be added without autonomous detection.
  - **Codex 0.170.0 live compaction probe:** Re-run without the invalid `mcp_servers.ws` override (`-32600: invalid transport`), measure `thread/compact/start`, `contextCompaction`/`thread/compacted`, auto-compaction, failure, and `developerInstructions` persistence, then enable `CodexCatalog().Compaction.Lifecycle` and `PromptPersists`.
  - **Pi test baseline:** 25 pre-existing Pi test exclusions also fail on `track/agent-v2` base; triage separately from compaction.

### Phase 2: State restore bundle

After compaction and after a daemon restart, the daemon injects a restore
bundle: ws key, agenda, ticket/plan, children, exec jobs. This replaces the
`lead-revive` ritual for hosted sessions and applies to children as well as
leads. Depends on Phase 1's detection and re-injection path, not on a segment
chain. Settle its compatibility tier before implementation.

Verification: tests that a compacted session and a
restart-recovered session each receive the bundle with every listed field, with
no `lead-revive` call; CLI-tier `lead-revive` unchanged.
