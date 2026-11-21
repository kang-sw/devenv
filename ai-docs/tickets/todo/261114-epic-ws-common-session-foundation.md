---
title: "공통 세션 기반: 역량 카탈로그를 통한 hosted 우선 하네스 공통화"
sage-review-design: required
related:
  261110-epic-ws-dashboard-agent-host: "existing hosted-agent product scope; this epic supplies a shared foundation without replacing its child plans"
  261113-bug-ws-hosted-session-sentinel-bootstrap-mints-foreign-key: "identity-adoption regression evidence, not authority to restrict root issuance"
  261114-bug-ws-fork-parent-identity-loss: "lineage evidence; clerk design (705=b) was later dropped, see Child Tickets"
  261114-research-ws-clerk-session-role: "dropped; kept as history of the clerk-role direction, not a live child"
  261116-research-ws-hosted-session-first-tier: "evidence child: the hosted-first research whose Confirmed Decisions this epic carries"
  261117-research-ws-daemon-file-touch-map: "release-gate file-touch map; stage-2 children must follow its rule (not edited from here)"
---

# Common session foundation

## Scope

Hosted-first (01)=a: the daemon-hosted session is the reference tier. Common
interface work targets hosted sessions (MCP, daemon injection, playbooks,
daemon-owned `ws/agent.*`). The common harness interface is a declarative
capability catalog (07); it is not a wider external spec. Removal work (the Pi
extension, CLI-tier features) is approved per item, never bundled.

Delivery is four stages:

1. **Foundation (done).** The common interface Phase 1 and Phase 2 and the
   Pi/Claude parity Phase 1 are in `.done`; owner (674)=a with (676)-(680)=ok
   authorized the first interface ticket, and (682) put current context/state
   parity adoption in that ticket's Phase 2.
2. **Hosted-first migration.** The reference implementation is Claude (25).
   - Capability catalog first: define it and fill the Claude values, with
     fixtures.
   - Compaction becomes a common command delivered from outside the harness.
   - `ws/agent.*` multi-level delegation; native subagents are removed in hosted
     sessions (leads and workers).
   - `ws/exec` starts with one holder per job; the single session holder is an
     idea-only future consolidation, not a prerequisite.
   - Move plugin/extension features into the daemon: MCP plus injection,
     playbooks instead of skills, `prompts/list` slash commands.
   - The Claude CLI plugin is frozen; the Pi extension is retired after hosted
     parity, each removal separately approved.
3. **Tier-1 harnesses.** Codex, then Antigravity (02). Each fills the catalog
   values per `harness@version` range and implements its backend. Then Pi as a
   port of the Codex adapter (25).
4. **Extensions.** Audit store/view, authority/permission UI, voice, live
   reasoning, and `ws/agent.*` session lightweighting (the long-term cost answer
   from (06)).

Working order (owner 2026-11-17 22:10, (70)=b, (69), superseded for the holder
dependency by owner (1872)=a on 2026-11-20): catalog -> Codex -> Antigravity ->
exec, with Pi last. Exec starts with per-job holders; the single session holder
is no longer on the prerequisite path.

Independent, not stage-bound: the mailbox in-memory-first store. It gates the
agent-tools ticket's Phase 2 (mail routing to daemon-owned addresses), per the
mailbox ticket's Decision 6.

## Non-Scope

This map does not authorize a replacement universal runtime, new approval powers
or root-key issuance restrictions. Existing MCP root issuance removal/blocking is
unchosen and may be T2; the owner must settle it separately. Existing session
keys and historical state are preserved. Removing the Pi extension or any CLI-tier
feature is not authorized by this map; each concrete removal needs the owner's
explicit approval (04).

## Child Tickets

Stage 1 (done):

- `261114-refactor-ws-common-session-interface` - done: Phase 1 defined the
  minimal shared access/identity/event-adapter boundary; Phase 2 adopted current
  access with state/behavior parity. Prior art: the landed
  `261114-refactor-ws-hosted-common-contract-pi-claude-parity#phase1`.

Stage 2 (hosted-first migration, reference implementation Claude):

- `261117-feat-ws-harness-capability-catalog` - first child; defines the catalog
  (admission conditions, graded items, version policy), fills Claude values,
  HTTP MCP with per-session token, startup check, fixtures, and compaction as a
  common command. Phase 1 starts with the Pi HTTP MCP probe.
- `261117-feat-ws-agent-tools-mailbox-routing` - `ws/agent.*`, single
  `mailbox.send`, in-memory routing for daemon-owned addresses, relation from
  lineage, native subagent removal in hosted Claude sessions (leads and
  workers).
- `261117-feat-ws-single-session-holder` - idea-only PC-wide holder
  consolidation; no longer a prerequisite of exec (owner (1872)=a,
  2026-11-20).
- `261120-research-ws-daemon-supervision-http-attach` - stage-2 research into a
  long-lived daemon supervisor and direct HTTP MCP attachment for user-started
  sessions; separable from holder consolidation.
- `261117-feat-ws-exec-tools` - `ws/exec.*` with one holder per job, reusing the
  landed holder-per-process model; native process-tool removal and the
  never-auto-terminate rule. The single session holder is not a prerequisite.
- `261117-feat-ws-exec-mux-terminal` - stage 2, after exec tools: PTY screen
  sessions (`exec.shell(mux: true)`, `exec.mux.read`) ((51)=a). Prerequisite:
  `261117-feat-ws-exec-tools`.
- `261117-feat-ws-hosted-prompt-assembly` - application layer 2, 3, B, C
  ((28)=a): daemon-owned workflow manual and playbook body, per-turn ambient
  injection replacing plugin hooks, `agent.spawn` delivering a system prompt
  path/text and resolving tier server-side (spawn is playbook-unaware), and the
  `{{NativeToolScheme}}` render variable. Prerequisite: `261117-feat-ws-harness-capability-catalog`.
- `261117-feat-ws-hosted-compaction-lifecycle` - application layer 1, A
  ((28)=a): daemon compaction nudge, session-state restore replacing the
  lead-revive ritual, and `ws/compact` ((31)=a), for leads and children.
  Prerequisite: the catalog ticket; compatibility tier (segment chain) settled
  before ready.
- `261117-feat-ws-hosted-child-session-entry` - application layer 4, late stage
  2 ((28)=a): enter any `ws/agent.spawn` child's hosted session from the
  dashboard, every harness. Prerequisite: `261117-feat-ws-agent-tools-mailbox-routing`.
- Candidates, no ticket yet: E, usage/budget per agent tree (late stage 2); D,
  tool-result size budget/paging (stage-2 candidate, unplaced by (28)); R,
  split the oversized `agents-plugin-tool/internal/wsagent` sources this epic
  grew past the 1000-line size guard (`agent_tools_test.go`, `runner_conv.go`,
  with the earlier `runner.go`, `claude.go`, `pi.go`, `children.go`,
  `lead_claude_test.go`, `lead_fake_test.go`), plus
  `agents-plugin-tool/internal/wsdashboard/daemon.go` (1010 lines after
  `dec0d7d6a`; owner (1874), 2026-11-20). Owner (94)=a: defer to this
  follow-up and do not raise the `.gitattributes` threshold.
- Planned (not yet cut): the remaining plugin/extension moves into the daemon
  (playbooks instead of skills, `prompts/list` slash commands); open questions
  are in `261116-research-ws-hosted-session-first-tier`.

Stage 3 (tier-1 harnesses; each prerequisite: the catalog ticket):

- `261115-feat-ws-hosted-codex-adapter-contract` - Codex first; range-based
  catalog values and one hosted lead conversation. Protocol: the Codex
  app-server (owner answered (03), 2026-11-17 22:10).
- `261117-feat-ws-hosted-antigravity-adapter` - Antigravity after Codex; revives
  the dropped `261114-feat-ws-hosted-antigravity-acp-adapter` as a new stem;
  account access confirmed (owner 2026-11-17 22:10).
- `261120-feat-ws-pi-common-interface-rebuild` - late-stage Pi child, after
  Claude, Codex, and agy complete the common hosted interface: rebuild Pi from
  scratch as its backend adapter, fit beneath the unchanged daemon application
  layer, MCP tools, and prompt assembly, and retire the existing Pi extension
  through a separately settled transition/compatibility gate (owner directive,
  2026-11-20). This supersedes the prior planned Codex-adapter port (25).

Stage 4 (extensions):

- `261119-epic-ws-conversation-audit` - conversation audit sub-epic ((1331)=a,
  2026-11-19): policy and envelope, pre-filter capture and per-harness
  adapters, SQLite store (after `261119-epic-ws-daemon-sqlite-storage`),
  query API, dashboard view. It supersedes the dropped
  `261114-feat-ws-conversation-audit-event-store` and
  `261114-feat-ws-dashboard-conversation-audit-view` and carries approvals
  545, 546, 550, 594-611 and 681. The foundation gate (interface Phase 2, 682)
  is satisfied.
- Planned (not yet authored): authority/permission UI, voice, live reasoning,
  `ws/agent.*` session lightweighting.
- Candidates ((28)=a), no ticket yet: F, daemon-owned fork/rewind. G, audit
  unification, is now `261119-epic-ws-conversation-audit`.

Independent:

- `261117-feat-ws-mailbox-memory-first-store` - daemon-internal storage change
  shipped as a patch; T1/T2 downgrade judgment in its Phase 1. Its store (Phase
  2) lands before `261117-feat-ws-agent-tools-mailbox-routing` Phase 2 (mail
  routing), per its Decision 6.

Evidence child:

- `261116-research-ws-hosted-session-first-tier` - the hosted-first research;
  its Open Questions remain open there (injection routing table budget, curated
  slash list, `prompts/list` probe, Pi extension retirement parity criteria, the
  hosted-lead Claude mod, per-tier spawn idiom routing).

Dropped (history only): `261114-research-ws-clerk-session-role` (research
dropped) and `261114-feat-ws-hosted-antigravity-acp-adapter` (replaced by the
new Antigravity stem above).

### Extension dependency map

Foundation prerequisite for every extension below is already met
(`261114-refactor-ws-common-session-interface#phase2`).

| Follow-on | Consumer | Additional prerequisite |
|---|---|---|
| Authority UI | Not yet authored | No additional prerequisite settled |
| Additional harnesses | Codex, then Antigravity, then Pi (stage 3) | `261117-feat-ws-harness-capability-catalog`; Codex's first implementation also needs its settled range-based contract and separate execution approval |
| Voice | Not yet authored | No additional prerequisite settled |
| Live reasoning | Not yet authored | No additional prerequisite settled |
| Audit (capture, store, API) | `261119-epic-ws-conversation-audit` children 1-4 | Store: `261119-epic-ws-daemon-sqlite-storage` precedent ((1331)=a); order inside the sub-epic |
| Audit UI | `261119-feat-ws-dashboard-audit-view` | `261119-feat-ws-audit-query-api` and `261119-feat-ws-audit-capture-harness-adapters#phase1` |
| Exec | `261117-feat-ws-exec-tools` | No open single-holder prerequisite; starts with per-job holders using the done `261120-refactor-ws-agent-workers-under-holders` pattern |

## Cross-Child Decisions

Board IDs `(NN)` refer to the 2026-11-17 lead-discuss session; earlier IDs
(594-611, 674-682, 705, 720-722) are from the prior captures.

- **Hosted-first (01)=a.** The hosted session is the reference tier; Claude is
  the reference implementation (25). Common interface work targets hosted
  sessions.
- **Non-hosted tiers (04)=a.** The Claude CLI plugin is frozen: keep current
  features, add none. The Pi extension is retired after hosted parity, per-item
  approval; Pi remains only as a hosted lead/worker harness.
- **Harness order (25).** Implementation and verification order is Claude, Codex,
  Antigravity, Pi. Pi is driven only with GPT models, so the Pi hosted backend is
  a port of the Codex adapter and catalog profile, not a separate Pi-first design.
  Recorded as assumed-ok by the lead; the owner may veto. The working order
  across stages is catalog -> Codex -> Antigravity -> exec, with Pi last
  ((70)=b, (69), 2026-11-17 22:10; holder dependency superseded by owner
  (1872)=a, 2026-11-20). Exec starts with per-job holders; the single session
  holder is idea-only.
- **Codex and Antigravity are both tier-1 hosted harnesses (02).** The Codex
  protocol is the app-server (owner answered (03), 2026-11-17 22:10,
  superseding the unconfirmed (799)=a); Antigravity account access is
  confirmed (owner 2026-11-17 22:10).
- **Capability catalog is the common interface (07), (08)=c, (20)-(22), (24).**
  Items are declared `native` / `ws-substitute` / `unsupported` plus a range, per
  `harness@version` with `since` / `until`. Admission conditions: headless JSON
  event streaming, and HTTP MCP server injection. Startup reads the version and
  lightly checks the init shape; out-of-range versions (above or below) run
  loudly with the nearest-version profile. Verification is per-version recorded
  stream fixtures; fixtures and Go tests run locally on the macOS home machine
  ((68), 2026-11-17 22:10). Details live in
  `261117-feat-ws-harness-capability-catalog`.
- **MCP transport (24).** Hosted sessions connect by HTTP direct JSON-RPC with a
  per-session token (loopback only, re-issued per process start); no intermediary
  program on the hosted path. The CLI tier keeps the stdio launcher (T0,
  additive). Whether Pi supports HTTP MCP natively without the extension is
  verified first, because the Pi extension retirement premise depends on it.
- **Delegation is always `ws/agent.*` (06)=a, (09)=a, (10)=c, (11), (12).**
  `agent.spawn` / `agent.interrupt` / `agent.list` / `agent.tiers`; no `wait`,
  `transcript` or `agent.send`; a single `mailbox.send`. Child
  `261117-feat-ws-agent-tools-mailbox-routing` Decision 1 ((60)=a) removes
  `agent.stop`: interrupt ends the current turn, then the child sleeps by TTL;
  permanent close is human-only in the dashboard, cascading deepest-first.
  Daemon-owned agent addresses route in
  memory. Relation is computed from lineage at delivery, not stored. Hire stays
  separate; worker-profile caps apply; `pi.*` alias wording superseded: hidden now, handlers removed next minor (owner 19:16, (54)=b). Spawn tree
  bounds ((72), owner 22:27, reversing (10)=c; (77)-(79)): depth cap 4 (lead =
  depth 0), per-agent width cap 99 counting awake direct children, no
  concurrent-agent cap, both caps code constants; detail in the agent-tools
  ticket. `authority` (read-only | workdir-write | full) is required on
  `agent.spawn` and capped at the parent's current level, `full` included
  without a user warning ((74)=b, (76)); the authority parent is the
  Runner spawn record ((84)=a), and every child gets one worker-role ws tool
  loadout whose mutating tools the daemon refuses for read-only ((85)=c). Native subagents are removed in hosted sessions (leads and
  workers) (29)=ok.
- **Holder and exec (16), (17)=a, (18), (19)=a; superseded in part by owner
  (1872)=a, 2026-11-20.** Exec begins with one holder per job, following the
  landed holder-per-process model; a PC-wide single holder is idea-only future
  consolidation and not a prerequisite. Exec output lives in holder memory, no
  files. Exec is daemon-approved and replaces native process tools in hosted
  leads. A session with live children or jobs is never auto-terminated;
  explicit close cascades deepest-first.
- **Release-gate file-touch rule (owner, 2026-11-17).** Minimize daemon file
  touches at the root; files for backup/restore only; deltas (append) not
  rewrites; no heartbeat rewrites; no per-event O(N) rereads; interactive paths
  never wait on file I/O; judged by measured syscalls/s on the owner's PC. Every
  stage-2 child follows it.
- **Preserve identity and authority.** Preserve identity, existing authority,
  lineage, state ownership and one owner of input/mail delivery through
  migration. Mailbox reachability and profile names do not independently grant
  approval authority.
- **Foundation stays the base.** Reuse the existing Runner/adapter boundary and
  the done common interface; audit waits for the interface's Phase 2 (682), not
  for all harnesses, voice or authority UI. Its backend/API/UI ordering and
  approvals 594-611 survive.
- **Compatibility.** New tools and endpoints are additive (T0); the 8
  old `exec.*` tools stay as aliases (T0); `pi.*` hidden from `tools/list` (T0) supersedes
  its alias (owner 19:16, (54)=b); relation computed, not stored, is T0.
  The mailbox store ships as a patch with its T1/T2 downgrade judgment in-ticket.
  The holder protocol is versioned and the new daemon speaks the old holder
  protocol. Removing `MaxSpawnDepth` and blocking native tools in hosted sessions
  (leads and workers) are observable behavior changes, owner-confirmed by (06),
  (10), (18), (29). Any other
  non-additive protocol/store/MCP change requires separate tier settlement; T2 is
  not authorized here, and a T3 boundary requires the owner's explicit approval.

## Completion Criteria

- Done: the stage-2 children are done; the tier-1 harnesses of stage 3 (Codex,
  Antigravity, Pi) are done or explicitly deferred/dropped; every captured child
  is done or explicitly deferred/dropped; and the owner closes this epic.
- Dropped: the owner drops the direction.
- Deferred: stage-4 extensions and unscheduled planned children remain proposals,
  not implicit implementation approval or whole-epic prerequisites; removal of
  the Pi extension or any CLI-tier feature stays per-item.
