---
title: "Pi ticket worker가 ticket-worker playbook 도구를 갖고 spawn되게 하기"
related:
  261113-feat-ws-pi-mini-lead-profile: prior decision; defined pi-mini-lead as the bounded ticket-workflow Pi profile
  261116-bug-ticket-handler-split-gaps: origin; its first Pi worker stopped for missing tools
  261116-bug-ws-pi-worker-worktree-tool-profile-gaps: prior decision; mini-lead is the profile for a full ticket-lifecycle worker
  261030-bug-workflow-manual-sentinel-mints-lead-key-for-subagents: origin of the workflow_manual profile bar
  261116-feat-ws-subagent-worktree-lease: landing-order; edits the same builtin.go profile lists and fixtures
  261116-feat-orchestration-profile-foundation: same track; changes workflow_manual file selection by profile
---

# Pi ticket worker가 ticket-worker playbook 도구를 갖고 spawn되게 하기

## Background

On hold (owner, 2026-11-18): the agent-management rework on
`track/agent-v2` (`ws/agent.*` with an authority enum replacing `pi.spawn`
profiles) is expected to supersede the decisions below; revisit after it
lands instead of settling (15)-(20) against `pi.spawn`.

The same gap covers `review-handler` and `ticket-handler` (playbook diet,
v0.50.8 release sweep): their playbooks call ledger and Sage tools no Pi
profile allows, and they spawn large-tier reviewers above their own medium
tier. Until this ticket lands, `lead-review` and `lead-ticket` tell a lead on
such a host to perform the rendered handler prompt itself.

When the implementation harness is Pi, `lead-run` step 6 tells the lead to
spawn the ticket worker through `{{.SpawnIdiom}}`. For Pi that idiom renders
as `<ns>/pi.spawn (tier: ...)` with no `profile`
(`agents-plugin-tool/internal/mcp/playbook_tools.go:170-177`), so the worker
gets the default `pi-worker` profile (`internal/mcp/pi_tools.go:265-269`).

The ticket-worker playbook (`agents-plugin/rsrc/ticket-worker/ticket-worker.md`)
calls `workflow_manual` (line 26), `route.resolve_implement` (62),
`tickets.acquire` (70), `playbook.render` (86) and `tickets.close` (100);
`ticket-worker-elevated.md` additionally calls `config.resolve_agent` (97).

- `pi-worker` (`internal/wsagent/builtin.go:97-106,168-177`) has none of
  those.
- `pi-mini-lead` (`builtin.go:110-114,178-187`) has all of them except
  `workflow_manual` and `config.resolve_agent`.
- `workflow_manual` cannot be listed in any profile's `ws_tools`:
  `leadKeyMintingTools` (`internal/wsagent/profile.go:61-70`) bars it and
  `ParseProfile` refuses it (`profile.go:261-262`), because it mints a lead key
  for a keyless caller.

Dogfood: during `261116-bug-ticket-handler-split-gaps`, the first Pi worker
(default profile) stopped because the tools were missing; the lead respawned
it with `profile: pi-mini-lead` and authorized a reduced workflow.

## Open Decision Queue
Last ID: (20)

(15) [open] [technical] The Pi ticket worker is spawned with `pi-mini-lead`, extended with the ticket-worker tools it still lacks, and the Pi rendering of lead-run's worker spawn names that profile.
- Extend `pi-mini-lead` over a new builtin `pi-ticket-worker` profile or widening default `pi-worker` - `pi-mini-lead` already exists as the bounded ticket-workflow profile (261113), so the name is known to every shipped daemon (an older daemon spawns it with its older list instead of refusing an unknown profile name); widening `pi-worker` would hand ticket mutation tools to every plain Pi delegate.

(16) [open] [policy] How the Pi ticket worker gets its own workflow context, given `workflow_manual` is barred from profile `ws_tools`.
- Context: the bar exists to stop a keyless non-lead caller from minting a lead key: only the FRESH branch (sentinel key plus `root`) mints (`workflow_manual.go#L424-L480`); an existing child key takes the CONTINUE path and mints nothing (`#L305-L346`). A Pi worker is spawned with a render-minted child key, so it never needs the minting path. ticket-worker.md:26-28 says the worker must call `workflow_manual` itself and that context quoted by the caller is no substitute. Prior decision 261113 explicitly kept `workflow_manual` out of `pi-mini-lead`; alternative a reverses that exclusion.
- Alternative a: allow `workflow_manual` in a profile's `ws_tools`; the handler refuses the FRESH sentinel on allowlist-restricted connections (`explicitAllowedTools()` non-empty, `server_inventory.go#L117`) and leaves CONTINUE open; `ParseProfile` stops refusing it in the same commit. Add it to `pi-mini-lead`. Tier T0: today those connections cannot call `workflow_manual` at all, so the new refusal narrows nothing that works.
- Alternative b: keep the bar; the Pi rendering of ticket-worker drops the self-call and the rendered prompt carries the manual instead.
- Alternative c: keep the bar; ticket-worker skips orientation when the tool is absent.
> a: one playbook path for every harness, and the change touches only what the bar was for (minting); b forks the playbook per harness, c silently loses the context the playbook says is required.

(17) [open] [announced] Compatibility tier T0: builtin profile lists are code, not stored data; an older daemon keeps its own `pi-mini-lead` list (degraded spawn, no data change), and new playbook text names only a profile every shipped daemon knows - AGENTS.md Compatibility Tiers; follows from (15).

(18) [open] [announced] Tests assert behavior (the resolved profile's tool allowlist and that the Pi worker spawn names the profile), never playbook sentences; update the literal list fixtures in `wsagent/builtin_test.go` - owner's standing no-prose-verbatim-tests directive.

(19) [confirmed] [technical] Auto-confirmed: lead-run's worker spawn uses a dedicated render variable (for example `{{.WorkerSpawnIdiom}}`) that renders `pi.spawn` with `profile: pi-mini-lead` for Pi and equals `{{.SpawnIdiom}}` on other harnesses; every other spawn site keeps `{{.SpawnIdiom}}`.
- A dedicated variable over naming the profile in the shared `{{.SpawnIdiom}}` or Pi-specific literal text in lead-run - `SpawnIdiom` is substituted at every spawn site (reviewers, implementers, handler, delegates), so a profile there changes all Pi spawns; literal Pi text breaks the host-neutral playbook (Architecture Rule 3).

(20) [open] [technical] Phase 1 checks whether renders the Pi worker performs with its prompt key (reviewer wrappers) get the Pi terminology, and fixes the caller recognition if they do not.
- In this ticket over a follow-up ticket - the worker's prompt key is render-minted, not its runner key, and `callerPiMCPOnly(key)` requires `rec.WSKey == key` (`playbook_tools.go#L141-L142`), so the worker's own renders may fall back to neutral idioms; acceptance 2 cannot hold without it, and the fix sits on the same path.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Decisions

- 261113-feat-ws-pi-mini-lead-profile (2026-11-13, Decisions): "`pi-mini-lead` = `pi-worker` tools plus `tickets.query/acquire/release/close/move/verify/checklist`, ... No note, whiteboard, or tracker writes. `workflow_manual`, `ferrule`, ... stay excluded." — bearing: contradiction-candidate
- 261116-bug-ws-pi-worker-worktree-tool-profile-gaps (2026-11-16, Decisions): "The rendered ticket-worker procedure requires those mini-lead capabilities; use that profile for a worker that owns the full ticket lifecycle." — bearing: supports
- 261112-research-ws-hosted-pi-missing-workflow-tools (2026-11-12, Confirmed Decisions): "(321)=b keep builtin `pi-worker` unchanged; add builtin `pi-mini-lead`; `pi.spawn` gains an optional `profile` argument. Rejected: widening `pi-worker` alone" — bearing: supports
- f4845e28 (2026-11-08, commit; ticket 261030-bug-workflow-manual-sentinel-mints-lead-key-for-subagents): "Refuse at definition validation rather than strip silently: the profile author gets a clear error naming why." — bearing: constrains
- 261116-refactor-ws-pi-retire-native-agent-tools (2026-11-16, commit d609726e): "All cross-version surfaces remain T0/T1, with no new Profile/config/rsrc/first-party manifest fields or refusal boundary." — bearing: constrains
- 261112-feat-ws-mcp-pi-spawn-claude-lead-path (2026-11-12, Decisions): "Rejected: built-in tools only (the floor's `ws/git.commit` step would fail); every ws tool the delegate role allows (largest list; Pi context cost)." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/builtin.go, profile.go, internal/mcp/playbook_tools.go, agents-plugin/rsrc/lead-run and ticket-worker playbooks, agents-plugin-wsflow rsrc mirror, builtin_test.go |
| scope.surface | cross-module | profile tool lists (wsagent), SpawnIdiom/terminology rendering (mcp), shipped rsrc playbooks and their byte-identical wsflow mirror; ParseProfile validation if (16) alternative a |
| scope.new_public_symbol | unknown | depends on open (15)/(16): extending PiMiniLeadWSTools adds none, but a new rendering variable or profile constant is not excluded |
| scope.new_type_contract | unknown | depends on open (16): alternative a changes ParseProfile/leadKeyMintingTools semantics (profile.go#L61-L70, #L261-L262) |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent/builtin_test.go, profile_test.go, profile_spawn_test.go, internal/mcp/worker_worktree_tools_test.go, playbook_pi_mcp_only_test.go |
| complexity.reuse_points | confirmed | PiMiniLeadWSTools append chain (builtin.go#L110-L114), piMCPOnlyTerminology (playbook_tools.go#L170-L178), excessProfileTools child-subset check (profile.go#L439-L460) |
| complexity.side_effect_risk | high | SpawnIdiom is one terminology value substituted at every site (reviewer, implementer, delegate, handler), so naming a profile in it changes all Pi spawns; mini-lead edits also move profile_spawn_test.go fixtures that substitute it for pi-worker |
| risk.correctness | moderate | worker's prompt key is render-minted, not its runner key, so callerPiMCPOnly(key) (playbook_tools.go#L129-L168) may not select the Pi idiom for renders the worker itself performs |
| risk.fit | moderate | the profile-per-role split versus a shared SpawnIdiom has no existing per-site profile hook; (16) alternatives b/c fork playbook behavior per harness |
| risk.test | moderate | Go tests run on remote w5090 only; literal list fixtures in builtin_test.go and the bar assertions in profile_test.go#L71-L82,#L98 must change together |
| risk.security_or_contract | high | alternative a lets a profile list workflow_manual, whose FRESH-sentinel branch mints a parent-less lead key (workflow_manual.go#L424-L480); the bar exists to stop exactly that |

## Phases

### Phase 1: Give the Pi ticket worker the tools its playbook calls

Scope: the profile definition, the profile `ws_tools` validation if (16)
requires it, the Pi rendering of lead-run's worker spawn, mirrors and
generated manifests, and tests.

Acceptance:

- A Pi ticket worker spawned the way lead-run's Pi rendering instructs has
  every MCP tool `ticket-worker.md` and `ticket-worker-elevated.md` call.
- Subagents that worker spawns through its own Pi `SpawnIdiom` (reviewers,
  explore) still have the tools their playbooks call, within the
  child-cannot-exceed-parent rule (`internal/wsagent/spawn.go:165-174`).
- A plain `pi.spawn` with no `profile` is unchanged (`pi-worker`).

Verification: Go tests run on remote w5090 only, never locally on the
Windows PC.
