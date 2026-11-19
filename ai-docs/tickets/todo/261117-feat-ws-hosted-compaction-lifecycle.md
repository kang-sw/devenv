---
title: "Hosted compaction lifecycle: daemon compaction nudge, session-state restore bundle and ws/compact (summarize, new harness session, inherited ws key)"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-harness-capability-catalog: "prerequisite; its compaction item grade (native / ws-substitute) selects the fallback; its Decision 6 defers the ws-substitute build to this ticket"
  261117-feat-ws-single-session-holder: "reattach after daemon restart targets the newest harness-session segment"
  261117-feat-ws-exec-tools: "daemon-owned exec jobs survive compaction; the restore bundle lists them"
  261117-feat-ws-agent-tools-mailbox-routing: "children and lineage keyed by the ws key; compaction applies to children too"
  261117-feat-ws-hosted-prompt-assembly: "sibling stage-2 child; shares the daemon injection path"
  261116-research-ws-hosted-session-first-tier: "evidence and Confirmed Decisions"
---

# Hosted compaction lifecycle

## Background

Part of owner 18:35's "application layer" on top of the common harness
interface, placed in stage 2 by (28)=a: (1) a daemon-owned compaction nudge, and
lead addition A, a daemon session-state restore without the lead-revive ritual.
This ticket also builds `ws/compact`, the common compaction command. Per owner
(31)=a the catalog ticket (Decision 6) names this ticket the owner of
`ws/compact`, the compaction nudge and the `ws-substitute` (summarize and
continue in a new session); `ws/compact` is the default compaction method for
hosted sessions. Phase 1 stages it on native compaction; Phase 2 changes the
backend to the new-session chain and retains native grades as fallback inputs.

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
`ws/compact`, the default-method policy (31)=a, and the nudge/restore.

1. **Scope placement (28)=a.** The nudge (1) and state restore (A) are stage-2
   children of the epic.
2. **Nudge and restore apply to child agents too (owner 18:39).** Not only
   leads.
3. **Default compaction method (31)=a.** For hosted sessions on every harness
   the default is `ws/compact`: summarize in our format (today's Pi extension
   compaction format), open a NEW harness session, inherit the ws session key,
   and inject the summary as its first prompt. In Phase 2 native harness
   compaction is the fallback when the session cannot write the summary itself;
   Phase 1 is the explicitly staged native implementation under (186).
4. **(33), owner ack 2026-11-17: (28)=a's single "hosted prompt assembly" child
   is split into two tickets:** prompt assembly (2, 3, B, C;
   `261117-feat-ws-hosted-prompt-assembly`) and compaction lifecycle (1, A,
   ws/compact; this ticket). Rejected: one ticket, because ws/compact is a
   separate runtime mechanism (session replacement, segment chain, record-shape
   compatibility tier) with different verification.
5. **(34), owner ack 2026-11-17: ws/compact runs only at an idle turn
   boundary.** Rejected: running it mid-turn, because in-flight harness tool
   calls and harness-side background shells die with the old session
   (daemon-owned ws/exec jobs survive).
6. **(35), owner ack 2026-11-17: one ws session owns a chain of harness-session
   segments** (resume and holder reattach target the newest segment; the
   dashboard stitches transcripts). Rejected: minting a new ws session per
   compaction, because agenda, whiteboard and child lineage are keyed by the ws
   key, and inheriting it is the point.
7. **(36), owner ack 2026-11-17: when the session cannot write the lever prose
   (context overflow), fall back to native compaction where the catalog grades
   it native, otherwise a cheap headless summarizer session over the
   transcript.** Rejected: a daemon-side model API call, because the daemon
   holds no model credentials.
8. **(37), owner ack 2026-11-17: the summary format is the Pi lever prose
   sections plus daemon-assembled fixed sections.** Rejected: a new format,
   because the existing Pi format is proven on Pi.

9. **(186)=a, owner 2026-11-19 15:14:** if a compaction command can be
   sent, implement `ws/compact` in Phase 1 by delivering the native signal.
   This is staging, not reversal of (31)=a: Phase 2 replaces the backend with
   (31)/(35)'s new-session chain, and (36) remains its fallback.
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
    not disable hard compaction. Restore means re-render and inject the same
    hosted session-start manual/identity injection, not the Phase 3 bundle.
    These choices reuse existing context and avoid a new persistent shape.
12. **Execution:** Pi worker (owner 2026-11-19 14:33); board topic
    `daemon-compaction`.

## Constraints

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
  and token observation must not add per-event file writes; segment-chain
  records are appended, not rewritten. Related, not edited from here:
  `261117-research-ws-daemon-file-touch-map`.
- Shipped playbooks stay harness-neutral and must not name this repository's
  specifics; the Claude CLI tier is frozen (its existing lever and `lead-revive`
  path are untouched).
- Identity, authority, lineage and mailbox delivery ownership are preserved
  across compaction: the new segment inherits the ws key and never gains
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
older daemons (unknown calls are refused: `mcp/server_dispatch.go:1662`).
Reopening with the new daemon reads the retained scalar values, not a rebuilt
session record. **Do not claim T0 solely from no session-shape change:** verify
old global/project writers preserve new harness-bucketed scalar entries and
that existing old-key writes still affect the new resolver; if either fails,
re-grade before release. No new manifest fields: the old reader rejects them
(`mcp/config_manifest.go:172-185`).

Phase 2's segment chain changes the session record shape (one ws key maps to an
ordered list of harness-session segments). Settle its separate tier with
source-backed old/new writer and reopen traces before implementation; T3 needs
owner approval, T4 never ships (`ai-docs/manuals/release-compatibility.md`).

## Phases

Phase 1 is the implementation contract below; Phases 2–3 remain todo-level.
Phase 2 needs its compatibility tier settled before implementation.

### Phase 1: Native ws/compact, detection and daemon nudge

1. **Probe first.** Record reproducible live agy and Codex app-server fixtures
   for native command and compaction start/end, including autonomous native
   compaction and failure. Existing evidence is not proof of all signals:
   agy sends `/compact` but its fixture records no event
   (`agents-plugin-tool/internal/wsagent/agy_fixture_test.go:309-333`,
   `agy_test.go:322-333`); Codex sends `thread/compact/start`
   (`wsagent/codex.go:381`) and maps `thread/compacted`
   (`wsagent/codex_mapper.go:189`). If no native command exists, advertise
   unsupported rather than simulate it. If no reliable lifecycle signal is
   observed, do not fabricate completion or claim automatic detection; record
   the limitation and bring the gap to the lead before implementation.
2. **Common command.** Add `ws/compact` to the hosted common command/dispatch
   surface; resolve command capability and native delivery via harness
   descriptor/catalog, never a harness-name conditional in common code.
   Queue at an idle turn boundary, for leads and workers; do not kill or steer
   in-flight work. Claude uses existing `/compact` routing
   (`wsagent/catalog.go:362`, `wsagent/claude.go:1004-1012`); Pi's existing
   RPC `/compact` maps to its compact command (`wsagent/pi.go:543-571`),
   and the extension's in-process lever uses `ctx.compact()`. Route unsupported
   and errors explicitly; no blind retry or duplicate command.
3. **Detect and restore.** Put per-harness native start/end mapping in the
   descriptor/mappers, including auto compaction. Claude's status/compact
   boundary are mapped at `wsagent/claude.go:1004-1012`; Pi already maps
   `compaction_start`/`compaction_end` (`wsagent/pi.go:959-968`). Drive the
   existing dashboard compacting phase (`wsdashboard/assets/dashboard.tmpl:189`)
   and clear it on success/failure. On confirmed successful end, re-render and
   inject the hosted session-start context for that same ws key, once, before
   the next ordinary queued message. Use the existing renderer/instruction
   assembly (`wsagent/runner_process.go:260,801-804`); do not mint a key.
   Do not inject a Phase 3 agenda/children/exec restore bundle here.
4. **Thresholds.** Implement the four Decision 11 values and per-layer,
   per-harness resolution in the config registry/resolver and tune/catalog
   surfaces (`mcp/config_registry.go:98-108,138-139`,
   `wsconfig/config.go:37-47`). Respect the usual layer precedence: at each
   layer use explicit common harness bucket, then explicit unbucketed common
   value, then applicable explicitly set legacy key, then proceed to the next
   layer; shared global default last.
   Keep old keys declared and functional for old clients and CLI tier; no
   destructive migration. Use the daemon's turn/usage observation
   (`wsagent/claude.go:1252-1259`) for both roles and all harnesses, not
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

### Phase 2: Session segment chain and ws/compact

Build `ws/compact` per (31)=a: the session writes the summary in the Pi format
(37), the daemon opens a new harness session as a new segment under
the same ws key, injects the summary as its first prompt, and retargets
resume/holder reattach to the newest segment. Native compaction is the fallback
when the session cannot write the summary (36). Applies to leads and
children. Records the compatibility verdict.

Verification: fixture-driven tests per harness available at that
time (Claude first): new segment created, ws key inherited, agenda/whiteboard/
lineage intact, children still reachable, resume after daemon restart lands on
the newest segment, fallback path on overflow; a recorded end-to-end hosted
compaction of a lead and of a child.

### Phase 3: State restore bundle

After compaction and after a daemon restart, the daemon injects a restore
bundle: ws key, agenda, ticket/plan, children, exec jobs. This replaces the
`lead-revive` ritual for hosted sessions and applies to children as well as
leads. Depends on Phase 2.

Verification: tests that a compacted session and a
restart-recovered session each receive the bundle with every listed field, with
no `lead-revive` call; CLI-tier `lead-revive` unchanged.
