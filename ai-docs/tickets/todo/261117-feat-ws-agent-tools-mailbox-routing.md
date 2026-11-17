---
title: "ws/agent.* delegation tools with unified mailbox routing, relation from lineage, and native subagent removal in hosted sessions (leads and workers)"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-harness-capability-catalog: "steer-capability lookup for parent-to-child delivery reads the catalog's mid-turn steer item"
  261116-research-ws-hosted-session-first-tier: "evidence and Confirmed Decisions"
  261115-feat-ws-worker-profile-session-spawn: "worker-profile caps apply to agent.spawn"
  261115-feat-ws-session-hire-approval: "hire stays a separate peer-creation path; lineage authority was rejected there"
  261115-feat-ws-permission-profile: "common permission intent; tool-whitelist based spawn depends on it"
  261117-research-ws-daemon-file-touch-map: "release-gate file-touch constraint; not edited from here"
  261112-epic-ws-mcp-pi-subagents: "done; pi.* is the prior art"
  261117-feat-ws-hosted-prompt-assembly: "delivers the playbooks' Never-use {{NativeToolScheme}} steer that complements native subagent removal (Decision 6)"
---

# ws/agent.* tools with unified mailbox routing

## Background

Delegation is always `ws/agent.*` regardless of harness (06): hosted sessions (leads and workers, (29)=ok) no
longer use the harness's own spawn tool, so subagent tracking comes from the
daemon. Today `pi.*` is the daemon-owned spawn path (children, depth cap
`MaxSpawnDepth = 2` in `agents-plugin-tool/internal/wsagent/children.go`), and
hosted Claude sessions are launched without a tool whitelist, so the native Agent
tool is available. This ticket introduces the final `agent.*` surface and
unifies parent/child/peer communication on a single `mailbox.send`.

The surface below supersedes an earlier draft that had `agent.transcript` and
`agent.wait`.

## Decisions

1. **Surface (06)=a, (09)=a.**
   - `agent.spawn(prompt, tier, system_prompt_path? | system_prompt?,
     authority?, cwd? | worktree?, label?) -> {address, status}`.
   - `agent.stop(address)` closes deepest-first (existing behavior kept).
   - `agent.list()` returns child states plus the last report summary (a
     superset of `pi.list`).
   - `agent.tiers()` (new) lists the valid `tier` values (Decision 11).
   - No `wait` and no `transcript`: completion arrives by push (a system mail
     starts a new turn); humans read transcripts in the dashboard.
   - No `agent.send`: a single `mailbox.send(to: <address>)` serves children,
     peers and other leads.
2. **In-memory routing for daemon-owned addresses (12).** The daemon Runner
   routes daemon-owned agent addresses in memory, not through mailbox files;
   persist only what restore needs. The mailbox file path stays only for
   external sessions and CLI leads. Reason: the release gate on daemon file
   touches.
3. **Relation is computed, not stored (11).** Parent/child/peer is derived from
   lineage at delivery. Rejected: a stored Envelope relation field, because of
   whole-file loss of a new Envelope field on an old binary (11).
4. **Delivery timing by relation.** Parent to child: steer if the catalog
   supports mid-turn steer for that harness@version, else next turn. Child to
   parent: report as today's Pi worker report. Peer: next turn.
5. **Report-hold rule kept.** A child's final report is an automatic system mail
   to its parent, held while the child's own children are open.
6. **Native subagent removal in hosted leads and hosted workers (06)=a, (11),
   (29)=ok.** Hosted Claude leads launch with `--disallowedTools Agent` and
   `Task` (init reports Agent as `Task`); the removal extends to hosted workers
   (29)=ok, a consequence of (06) "delegation is always `ws/agent.*` regardless
   of harness". Fix the existing tests that assert the flag's absence
   (`claude_lead_plugins_test.go:90`, `lead_claude_test.go:1182`). The
   complementary measure is the playbooks' per-harness "Never use
   {{NativeToolScheme}}" steer (owner addition with (29)), delivered by
   `261117-feat-ws-hosted-prompt-assembly`, so harnesses whose native subagent
   tools cannot be removed (Codex/Antigravity removability is unprobed, (08)=c,
   stage 3) are still steered to `ws/agent.*`.
7. **No depth cap (10)=c.** Remove `MaxSpawnDepth` (`wsagent/children.go`, the
   constant and its enforcement). Runaway is bounded by a per-root-tree
   concurrent-agent cap exposed as a config key. The default is chosen by
   measurement in this ticket (a guess of about 8 was floated and is uncertain)
   (12).
8. **Hire stays separate (11).** `session.request_hire` (peer, no lineage) is not
   merged into `agent.spawn` (child, lineage); the hire ticket rejected lineage
   authority (768=c, 775=a).
9. **Caps by tier rank and authority (46), (52)=a.** A child never exceeds its
   parent's tier or authority (1014). The tier cap compares rank
   (small<medium<large<xlarge) regardless of harness; aliases resolve to
   `harness.tier` first (existing rank check, search `children.go` for the tier
   rank check). The authority cap: a child's `authority` never exceeds the
   parent's.
10. **`pi.*` spawn tools are removed, not aliased** (`pi.spawn`, `pi.send`,
    `pi.list`, `pi.stop`, `pi.wait`, `pi.transcript`; owner 2026-11-17 19:16,
    reversing (11)'s alias decision). (54)=b: hidden from `tools/list` now
    (T0: new sessions never see them, old installed-plugin sessions keep working
    because the handlers still answer) and the handlers are deleted in the next
    minor. Rejected: removing the handlers in the same release with a T3
    boundary/minor bump. `agent.*` replaces them and shipped playbook/skill
    references to `pi.*` move to `agent.*` in the same change.
    The Pi backend remains reachable as `agent.spawn(tier: "pi.small")`.
11. **Tier namespace (38)=d, (45), (47), (48), (49).**
    - `tier` is one string: bare `small|medium|large|xlarge` = the parent's
      harness; `<harness>.<tier>` with harness in claude|codex|agy|pi (for
      example `pi.small`, `codex.large`); or a user alias defined in user
      settings and editable in the dashboard (for example `mockup` ->
      `agy.large`).
    - Resolved server-side to model/effort through per-harness tier tables,
      generalizing today's pi bucket `wsconfig.HarnessTiers`
      (`ResolveAgentTierForHarness`, `config.go`).
    - No model or effort arguments at all (45); pinning a specific model is done
      only through a user alias. Rejected: an override object {harness, model,
      effort} ((38) option a), a separate `agent.spawn_ex` (b), flat optional
      harness/model/effort (c).
    - `agent.tiers()` returns native (bare tiers on the parent harness), ext
      (`<harness>.<tier>` entries usable now: harness installed and the catalog
      grades the worker role available) and aliases with their resolution.
      Spawning an unknown or unusable tier fails with the valid list. Rejected:
      a static list (47).
    - The alias map is a new user-scope config key, dashboard-editable (48);
      compatibility expected T0 (old binary ignores the unknown key).
    - Shipped playbooks and `playbook.render` recommendations use bare tiers
      only; aliases and `harness.tier` are for lead/user choice (49,
      Architecture Rule 4).
12. **System prompt source (39), (44).** At most one of `system_prompt_path` or
    `system_prompt`, delivered through the harness's append-system-prompt (not
    the first user message as the Pi spawn path does today, search `spawn.go`; not full
    replacement). Spawn is playbook-unaware (owner 18:56): `playbook.render`
    keeps rendering and returns the path plus the recommended tier, which the
    caller passes as `tier`. Rejected: a `playbook`/`context` argument on spawn.
    Spawn reads `system_prompt_path` once at spawn and keeps the text in memory
    for re-injection when a new harness segment opens (ws/compact) (44).
    Rejected: re-reading the path per segment (render outputs are temporary;
    file-touch rule).
13. **`authority` replaces `profile` (52)=a.** `authority?: read-only |
    workdir-write | full`; default workdir-write; capped by the parent's
    authority. The enum is the common permission intent of
    `261115-feat-ws-permission-profile` (reused; that ticket unchanged).
    Harness/model come only from tier, instructions only from the system
    prompt. Each level maps to a daemon-defined ws tool set; per-harness native
    tool lists derive through the catalog's tool-removal value range. Existing
    `agent.profile.<name>` profiles stay internal: hosted leads (lead-claude/pi),
    dashboard features (search-answer-*, tracker-support-*). Agent-facing builtins are expressible: pi-worker = `pi.<tier>` +
    workdir-write; explore = read-only + explore.md as system prompt;
    pi-mini-lead = full. Rejected: renaming to `authority_profile` with today's
    contents (conflicts with (38)=d because a profile fixes backend/model, and
    duplicates the system prompt source); keeping `profile`.

## Open Decision Queue

Last ID: (54)

(53) [open] [policy] Whether `agent.spawn` gets a named `preset?` (config-defined {tier, authority, system_prompt_path} bundle).
- Context: explicit args would override the preset; the three agent-facing builtins (pi-worker, explore, pi-mini-lead) would ship as default presets and be listed with `agent.tiers`.
- Option a: no presets; builtins become "playbook.render + spawn args" recipes.
- Option b: presets, for convenience.
- Recommendation: a (keeps spawn without an instruction source, per owner 18:56); b acceptable.
- Lead reading of the owner's 19:16 reply: a (no presets; with pi.* removed no old-name resolution remains) - awaiting confirmation.

(54) settled as b by the owner at 19:18; moved to Decision 10.

## Constraints

- Go tests run on remote w5090 only, never local `go test`.
- Read before editing `agents-plugin-tool/`: `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`; for `agents-plugin-tool/internal/mcp/`
  also `ai-docs/manuals/ws-mcp.md`. If playbooks, skills, `agents-plugin/rsrc/` or
  `agents-plugin-wsflow/` text is touched (for example spawn-idiom routing), also
  read `ai-docs/manuals/skill-authoring.md` and `ai-docs/manuals/wsflow-mirroring.md`.
- Release-gate file-touch rule (owner, 2026-11-17): minimize daemon file touches
  at the root; files for backup/restore only; deltas (append) not rewrites; no
  heartbeat rewrites; no per-event O(N) rereads; interactive paths never wait on
  file I/O; judged by measured syscalls/s on the owner's PC. Daemon-owned agent
  routing must not add per-message file writes.
- Shipped playbooks must not name this repository's specifics
  (`ai-docs/manuals/shipped-surface-boundary.md`); route per-tier spawn idiom
  through generic hooks.
- Removing `MaxSpawnDepth` and blocking native tools in hosted sessions
  (leads and workers) are observable behavior changes, already owner-confirmed
  by (06), (10), (18), (29).
- `pi.*` spawn tools are hidden now and deleted next minor (Decision 10); the Claude CLI tier is frozen and keeps
  its native spawn.

## Compatibility

New `agent.*` tools are additive (T0), including `agent.tiers` (T0). The alias
map is a new user-scope config key (T0 expected: the old binary ignores the
unknown key; verdict with source lines in Phase 1's Result (48)). Hiding
`pi.*` from `tools/list` is T0 for this release ((54)=b: handlers still answer,
so old sessions keep working); the handler deletion next minor is judged in that
release. The source-line verdict is recorded in Phase 1's Result. Relation
computed at delivery rather than stored is T0 (no new Envelope field, so no
whole-file loss on an old binary). The concrete verdict cites source lines for
which version writes and reads what, per
`ai-docs/manuals/release-compatibility.md`, recorded in Phase 1's Result. Removal
of the depth cap and native-tool blocking in hosted sessions (leads and workers) are observable
behavior changes owner-confirmed as above.

## Phases

### Phase 1: agent.spawn/stop/list, mailbox routing and relation delivery

Add `agent.spawn`, `agent.stop`, `agent.list`, `agent.tiers` over the existing
Runner spawn path generalized to harness-selected backends with server-side tier
namespace resolution (bare, `<harness>.<tier>`, user alias via the new
user-scope alias config key and per-harness tier tables), `authority` mapped to
daemon tool sets, and system prompt delivery through the harness's
append-system-prompt (path read once, text kept in memory for re-injection); route
daemon-owned addresses in memory via the Runner; accept `mailbox.send` to a
daemon-owned address; compute relation from lineage at delivery with the timing
rules and the report-hold rule; apply tier-rank and authority caps; hide the `pi.*` spawn tools from `tools/list` (handlers deleted next minor,
(54)=b) and move shipped playbook/skill `pi.*` references to `agent.*`; remove `MaxSpawnDepth` and add the per-root-tree concurrent-agent cap
config key with a default chosen by measurement (record the measurement).

Deferred: exec tools, single holder, any stage-3 harness backend.

Verification: Go tests on remote w5090 covering spawn/stop deepest-first/list,
tier resolution (bare, `harness.tier`, alias, unknown/unusable failing with the
valid list), `agent.tiers` output, system prompt delivery via append-system-prompt
and re-injection from memory on a new segment, mail between parent, child and
peer with each timing, the report-hold rule, tier-rank and authority cap
enforcement and caps inheritance, `pi.*` absent from `tools/list` but still callable, with no shipped text referencing `pi.*`, and no mailbox file write
for daemon-owned addresses; the recorded cap-default measurement.

### Phase 2: Native subagent removal in hosted Claude leads and workers

Launch hosted Claude leads and hosted Claude workers with `--disallowedTools
Agent` and `Task`; fix the two tests that assert the flag's absence; confirm via
the init event that the native tool is absent and that delegation works through
`agent.spawn`.

Verification: updated tests on remote w5090; a recorded hosted-lead init stream
and a recorded hosted-worker init stream, each showing the tools removed; a
hosted lead delegating through `agent.spawn` end to end.
