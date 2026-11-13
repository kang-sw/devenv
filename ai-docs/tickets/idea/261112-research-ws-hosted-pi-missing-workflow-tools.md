---
title: "Hosted Pi dogfood: workflow tools absent from the lead's active surface"
related:
  261110-epic-ws-dashboard-agent-host: hosted Pi lead dogfood
---

# Hosted Pi workflow tools absent during dogfood

## Observation

On 2026-11-12, the user opened a dashboard-hosted Pi lead and requested
`lead-discuss` plus a whiteboard update. The lead received only the native
read/bash/edit/write tools and no ws bridge or delegation tools. It could
update the board only by starting the bundled MCP launcher from bash and
calling `whiteboard.query` / `whiteboard.update` over stdio with the existing
hosted key. No new session key was minted.

The session registration and agent record agree on `harness: pi`,
`profile: lead-pi`, `hosted: true`, and `host_pid: 242829` (reply ID
`theaterbotchsubatomicpolar`). These are runtime evidence, not portable
reproduction constants.

## Investigation

`agents-plugin-tool/internal/wsagent/pi.go` `piLeadArgs` loads ambient Pi
extensions rather than explicitly adding the ws Pi adapter. This machine's
`~/.pi/agent/settings.json` lists a local `pi-agent-dev` package and
`npm:pi-openai-usage`; whether that local package includes or loads the ws
adapter was not established. Missing installation, resource discovery, or
adapter startup are hypotheses, not a diagnosed runner defect.

Compare ordinary terminal and dashboard-hosted Pi startup on the same
machine, inspect extension diagnostics and effective active tools, and decide
whether this is a setup/documentation gap or a missing hosted preflight.
The fallback restores board access but does not validate ws bootstrap,
delegation, hosted-mode suppression of duplicate input/feed, or compaction
revival.

No behavior or compatibility-surface change is authorized by this capture.

## Verified Findings (pi.spawn subagent path, 2026-11-13)

Scope note: the Observation above concerns a hosted Pi lead. The report that
prompted this pass concerns `pi.spawn` subagents, which run the builtin
`pi-worker` profile. The two take different startup paths; the lead path
(`piLeadArgs`) stays undiagnosed here.

- **Cause is a deliberate but too-narrow allowlist, not a startup bug.**
  `agents-plugin-tool/internal/wsagent/builtin.go:75` defines
  `PiWorkerWSTools` as `git.status`, `git.diff`, `git.log`, `git.merge_base`,
  `git.commit` plus the six `pi.*` tools; `builtin.go:129` wires it as the
  `pi-worker` profile's `WSTools`. The doc comment (`builtin.go:60-73`) says
  "the few ws tools a delegated implementation or review prompt names" - the
  list was sized for commit plus spawning, never for the ticket workflow.
- **Mechanism.** `piToolSurface` (`wsagent/pi.go:143-165`) turns each
  `WSTools` entry into a `--tools mcp__ws__<tool>` entry; `piArgs`
  (`pi.go:241-267`) passes `--no-extensions` plus only the MCP bridge and
  `--tools <that list>`. The runner also sets `WS_MCP_ALLOWED_TOOLS` on the
  attached ws-mcp to the same list (`wsagent/runner.go:1880`), and
  `Server.toolAllowed` (`mcp/server.go:5621-5628`) refuses anything else. So
  both the Pi tool list and the server enforce the allowlist; the ws-mcp is
  attached and healthy (the git tools work), it simply advertises 11 tools.
- **Not a missing registration, tier filter, or lead-vs-worker role bug.** A
  hosted Pi lead takes `piLeadArgs` (`pi.go:283-300`): ambient extensions and
  the user's own Pi setup, no `--tools`, a lead key. A Claude worker gets
  the full server through the plugin's MCP launch. Only the Pi worker profile
  has an explicit ws allowlist.
- **Key scope is separate from the allowlist.** Agent keys are minted
  delegate-scope (`mcp/agent_keys.go:11-30`). `roleAllowsTool`
  (`mcp/server.go:5650`) lets a delegate reach everything except `session.*`
  and `config.*` writes, and `isLeadOnlyTool` (`server.go:140`) blocks it from
  the lead-only set. `tickets.query/acquire/release/close/move/verify/
  checklist` are not lead-only, so adding them to the allowlist is enough.
- **Tools that must stay excluded (lead-only or key-minting):**
  `ferrule` and `workflow_manual` (rejected by `leadKeyMintingTools`,
  `wsagent/profile.go:260`, since they mint a lead key); `workflow_state`,
  `tickets.sage_stamp`, `tickets.index_init`, `git.merge`, `worktree.*`,
  `lead.*`, config-writer tools (`isLeadOnlyTool`); `note.write/erase/mute/
  unmute` (denied to any non-top-level-lead key, `server.go:733`), and the
  whiteboard and tracker write tools (`server.go:736-744`). `note.query` is a
  read and is allowed.
- **Consequence for ticket-worker text.** `ticket-worker.md` calls
  `workflow_manual(session_key)`, `route.resolve_implement`,
  `tickets.acquire`, `playbook.render`, and `tickets.close`. A Pi worker can
  never call `workflow_manual` (key-minting, stays excluded), so the worker
  prompt a spawner renders must carry its context another way; this is a
  separate gap from the allowlist.
- **Existing overrides.** A higher-scope `agent.profile.pi-worker` replaces
  the builtin whole (`builtin.go` BuiltinProfiles comment), so widening the
  builtin does not reach a user who already overrides the profile.

## Proposals (non-authoritative)

- **Widen `PiWorkerWSTools`** with the ticket-workflow tools a worker calls:
  `tickets.query`, `tickets.acquire`, `tickets.release`, `tickets.close`,
  `tickets.move`, `tickets.verify`, `tickets.checklist`, and read-only
  `note.query`; optionally `playbook.read`/`playbook.render` and
  `path.generate`. Keep every excluded tool above out.
- **Update the doc surfaces that enumerate the list:** the `builtin.go`
  comment, `ai-docs/manuals/ws-mcp.md` (the `pi-worker` paragraph) and
  `builtin_test.go` expectations.
- **Tier: T0.** The change adds tools to a builtin profile's default
  allowlist: additive, no tool, argument, store, tracker event, or manifest
  field changes (`release-compatibility.md` "Tiers": new tools, arguments and
  optional fields are T0). Running children keep their old list until their
  process restarts. The ticket should still record the tier under
  `## Decisions` per AGENTS.md.
- **Hosted Pi lead observation** (the original capture) still needs the
  terminal-versus-hosted comparison; this pass does not close it.

## Profile Design Evidence (2026-11-13)

Reopened by the owner: an elevated worker needs playbook access, so the
allowlist may differ per spawn instead of widening one list for everyone.

- **Profiles already exist.** `agent.profile.<name>` is the family
  (`wsagent/profile.go:34`); a profile bundles `tools`, `ws_tools`,
  `mcp_servers`, `plugins`, `skills`, `instructions` (`profile.go:74-123`).
  Resolution is project, repo, global, then the builtin floor
  (`BuiltinProfiles`, `builtin.go:106`); a higher scope's profile of the same
  name replaces the builtin whole. `config-manifest.json` has no `agent.*`
  key: the family is generated in `mcp/config_registry.go:514`. A repo-scope
  profile may set `ws_tools` (only `mcp_servers`, `plugins`, `lead` are
  refused there, `profile.go:328-348`); `ws_tools` may never name `ferrule`
  or `workflow_manual` (`profile.go:260`).
- **Spawn hardcodes one profile.** `handlePiSpawn` passes
  `wsagent.PiWorkerProfile` (`mcp/pi_tools.go:307`) and a token holder's
  `spawnFor` does the same (`wsagent/children.go:310`), so every child has
  its parent's tool set and escalation is impossible today. Caps that exist:
  depth `MaxSpawnDepth = 2` (lead 0, worker 1, child 2; `children.go:63-65,
  275-277`) and tier no higher than the holder's (`children.go:285`). The
  child's profile is resolved by name at every process start from its record
  (`runner.go:1869`), so an override applies at restart.
- **Allowlist is not authority.** Spawned Pi keys are delegate-scope
  (`mcp/agent_keys.go:11-30`). The keyed gate refuses lead-only tools to any
  non-lead key even when allowlisted (`server.go:731-734`, `isLeadOnlyTool`
  `server.go:140`: `git.merge`, `worktree.*`, `tickets.sage_stamp`,
  `workflow_state`, config writers). A delegate key also cannot call
  `config.resolve_agent` (`roleAllowsTool`, `server.go:5661-5665`). A broader
  "lead authority" Pi child therefore needs a key-scope change, not just a
  longer `ws_tools`. Claude `role: worker` playbooks already get a
  lead-scope child key (`playbook_tools.go:507-517`, mint at `:926-933`);
  note, whiteboard and tracker writes stay top-level-lead only
  (`server.go:735-745`).
- **"Elevated worker" and "mini-lead" are one playbook.**
  `ticket-worker-elevated.md` (`role: worker`, tier large) calls itself "an
  orchestrating mini-lead" (line 15). Tools it names: `route.resolve_implement`,
  `tickets.acquire`, `tickets.close`, `playbook.render` (delegate-implementer
  leaves, reviewers), `config.resolve_agent`, `workflow_manual`, plus the
  spawn idiom. `ticket-worker.md` (medium) names the same minus
  `config.resolve_agent`. Fact-populator and the design reviewers
  (`role: delegate`/`reviewer`) read `tickets.query`, `rationale.query`,
  `git.log`. `tickets.create_empty/template`, `sage_stamp`, `whiteboard`,
  `note.write`, `git.merge` and `worktree.*` appear only in lead playbooks
  (`lead-run`, `lead-ticket`, `whiteboard`, `executor-wrapup`), never in a
  worker's. Under a delegate key the 327=b set covers every elevated step
  except `workflow_manual` (excluded by design) and `config.resolve_agent`
  (Pi tier goes through `pi.spawn`'s `tier`).
- **There is no playbook write tool.** The playbook tools are `playbook.read`
  and `playbook.render` (`server.go:5473,5486`); render writes a worktree tmp
  prompt file and mints a child key only for a lead-scope caller
  (`server.go:2240-2255`), so a Pi worker's render carries no credential
  block. `path.generate` allocates review, prompt, plan, clone artifact
  paths (`server.go:5438`). Prompt overrides are `config.tune` writes, which
  are lead-gated. "Write" today is the worker's own file tools plus these.
- **Playbook text gap (separate).** The `pi` row of the spawn idiom table is
  the legacy adapter's `ws-agent-spawn` (`playbook_tools.go:42`), not
  `pi.spawn`; `ticket-worker-elevated` also tells the worker to call
  `config.resolve_agent`, refused to a delegate key.

## Profile Proposal (non-authoritative)

- **Profiles.** `pi-worker` (default) stays exactly today's 11 ws tools.
  New builtin `pi-mini-lead` = `pi-worker` plus `tickets.query/acquire/
  release/close/move/verify/checklist`, `note.query`, `playbook.read`,
  `playbook.render`, `path.generate`, `route.resolve_implement`. No
  lead-authority profile now. A user defines further profiles (a reader, say)
  as ordinary `agent.profile.<name>` entries.
- **Selection.** `pi.spawn` gains an optional `profile` argument (name of a
  resolvable non-lead pi-backend profile); omitted means `pi-worker`.
- **No escalation.** In `Runner.Spawn`, when the parent is a conversation,
  the child's resolved `ws_tools`, `tools`, and `mcp_servers` names must each
  be a subset of the parent's resolved profile; a lead parent is unbounded.
  The depth and tier caps stay.
- **User overrides.** Today's override key `agent.profile.pi-worker` keeps
  its meaning and is not widened. `pi-mini-lead` is a separate key, overridden
  independently. A user who pinned a narrow `pi-worker` gets no silent
  widening.
- **Tier.** T0: an optional argument and a new builtin profile key.

## Outcome Ledger

### Verified Findings

- The `pi.spawn` subagent gap is a deliberate but too-narrow ws allowlist
  (`PiWorkerWSTools`, 11 tools), enforced both by the Pi `--tools` list and by
  the server (`WS_MCP_ALLOWED_TOOLS`, `Server.toolAllowed`); not a startup bug.
- `agent.profile.<name>` already exists and bundles `ws_tools`; `pi.spawn`
  hardcodes `pi-worker` (`mcp/pi_tools.go:307`, `wsagent/children.go:310`).
- Pi child keys are delegate-scope; the keyed gate refuses lead-only tools
  whatever the allowlist, so a lead-authority child needs a key-scope change.
- "Elevated worker" and "mini-lead" are one playbook
  (`ticket-worker-elevated.md`); under a delegate key the set below covers it
  except `workflow_manual` (excluded by design) and `config.resolve_agent`.
- No playbook write tool exists; read, render, and `path.generate` exist.
- The playbook text gap (legacy `ws-agent-spawn` idiom, delegate-refused
  `config.resolve_agent`) is independent of the allowlist.
- The original hosted Pi lead observation (terminal-versus-hosted comparison)
  is not closed by this pass.

### Confirmed Decisions

Owner answers 2026-11-13 (session whiteboard item IDs).

- (327)=b profile-based tool exposure; the elevated worker gets the workflow
  tool set.
- (321)=b keep builtin `pi-worker` unchanged; add builtin `pi-mini-lead`;
  `pi.spawn` gains an optional `profile` argument. Rejected: widening
  `pi-worker` alone; also shipping a lead-privilege profile now.
- (322) `tickets.query/acquire/release/close/move/verify/checklist`, (323)
  read-only `note.query`, (324) `workflow_manual`, `ferrule`, `workflow_state`,
  `git.merge`, `worktree.*`, `tickets.sage_stamp` stay excluded: these define
  the `pi-mini-lead` tool set under (321)=b.
- (381) select the existing `agent.profile.<name>` by name; no permission enum.
- (382)=a builtin set now = `pi-worker` + `pi-mini-lead`. Rejected: read-only
  `pi-reader`, lead-privilege profile.
- (383)=a defer a Pi child holding lead-scope keys; out of scope.
- (384)=a omitted `profile` always means `pi-worker`.
- (385) `profile` is a string naming any resolvable non-lead Pi profile; T0.
- (386) no-escalation is the subset check "child tools within parent tools" in
  `Runner.Spawn`; a lead parent is unrestricted.
- (387)=a an escalation request refuses the spawn, naming both profiles and
  the excess tools. Rejected: silently intersecting.
- (388)=a no new playbook-write tool. Rejected: worker override-write tool;
  deferring to a lead profile.
- (389) depth cap stays 2; a mini-lead's children are leaves.
- (390) playbook text issues go to
  `261113-fix-ws-playbook-spawn-idiom-and-delegate-resolve-agent`.
- Implementation: `261113-feat-ws-pi-mini-lead-profile`.
