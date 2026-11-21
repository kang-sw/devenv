---
title: "hosted 프롬프트 조립: 데몬 소유 workflow manual, playbook 본문, 주변 컨텍스트 주입, 렌더링 가능한 에이전트용 playbook"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-harness-capability-catalog: "landed (85ef24622); value source for the native-subagent steer (its native subagent tool name per harness) and the skill list/injection items"
  261117-feat-ws-hosted-antigravity-adapter: "its Phase 2 Forward names this ticket as the fix for a keyless agy lead"
  261117-feat-ws-agent-tools-mailbox-routing: "agent.spawn surface (candidate C: spawn delivers a system prompt and resolves tier; rendering stays in playbook.render); Decision 6 pairs native-tool removal with this ticket's Never-use steer"
  261117-feat-ws-hosted-compaction-lifecycle: "sibling stage-2 child; shares the daemon injection path (nudge and restore bundle ride the same assembly)"
  261116-research-ws-hosted-session-first-tier: "evidence and Confirmed Decisions"
  261116-feat-ws-daemon-plugin-package-select: "hosted-lead plugin package selection; Decision 12 removes the plugin bundle from hosted Claude, which then runs the daemon's own package ws-mcp (Phase 1)"
  261115-bug-ws-pi-bootstrap-elides-undelivered-manual: "Pi extension manual-injection readiness, which Decision 21 relies on"
  261115-feat-ws-worker-profile-session-spawn: "ready; owner-created worker-profile sessions keep its short worker manual and are exempt from native-subagent removal"
  261115-feat-ws-whiteboard-agent-card: "ready; card children keep its fixed card bootstrap; its keyless-attempt tests meet Decision 10"
  261115-feat-ws-hosted-codex-adapter-contract: "ready; recommends keeping plugin skills and hooks in hosted Codex, the opposite of Decision 12, outside this ticket's scope (Decision 15)"
  261120-feat-ws-pi-common-interface-rebuild: "late-stage epic child rebuilding Pi on the common hosted interface and retiring its native extension after the unresolved transition gate"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 1e60dd1b53252259
sage-review-design-reviewed: 1e60dd1b53252259
completed: 2026-11-21
---

# Hosted prompt assembly

## Background

Owner 18:35 proposed an "application layer" on top of the common harness
interface: the daemon, not the harness plugin, assembles what a hosted session
reads. This ticket carries four of its parts, placed in stage 2 ("hosted prompt
assembly") by (28)=a:

- (2) the daemon owns the workflow manual;
- (3) the daemon owns the skill body: it inserts the playbook body directly,
  removing today's double pointer (skill invocation, then `playbook.read`);
- B (lead addition, accepted as a candidate by (28)=a): per-turn ambient
  injection by the daemon replaces plugin hooks;
- C (same, revised by owner 18:56): `agent.spawn` is playbook-unaware; it
  delivers `system_prompt_path`/`system_prompt` as an appended system prompt
  ((39), (44)) and resolves tier server-side ((38)=d). Rendering stays only in
  `playbook.render`, which returns the path plus the recommended tier.

It also adds the native-subagent "Never use" steer from (29)=ok's owner
addition, delivered through daemon-injected text (Decision 24). Today the shipped playbooks steer delegation toward whatever spawn
tool the CLI plugin offers; once hosted workers lose their native subagent tool
too, sessions must still be steered away from a native subagent tool that cannot
be removed (Codex and Antigravity removability is unprobed, (08)=c, stage 3)
toward `ws/agent.*`.

## Decisions

Confirmed (cite IDs; no other behavior is decided here). Decisions 4 and 5 were
queued and acknowledged by the owner on 2026-11-17 ("32..37=ok").

1. **Application layer parts 2, 3, B, C are stage-2 children (28)=a**, owner
   proposal 18:35. Parts 2 and 3 are the daemon-owned manual and daemon-owned
   skill body; B and C were lead additions accepted as candidates by (28)=a.
2. **Native subagent tool removal extends from hosted leads to hosted workers
   (29)=ok**, a consequence of (06) "delegation is always `ws/agent.*`
   regardless of harness". The removal itself is delivered in
   `261117-feat-ws-agent-tools-mailbox-routing` (Decision 6, its Phase 3,
   `ai-docs/tickets/.done/261117-feat-ws-agent-tools-mailbox-routing.md#L825-L846`;
   its Phase 2 is mailbox routing).
3. **Playbooks state, per harness, "Never use {{NativeToolScheme}}" (owner
   addition with (29)).** Mechanism superseded by Decision 24 (the steer is
   daemon-injected text, not a playbook render variable); the steer itself
   stands. As first settled, `{{NativeToolScheme}}` was a render variable naming
   that harness's native subagent tool, so harnesses whose native subagent
   tools cannot be removed are still steered to `ws/agent.*`. Its source is the
   catalog's native-subagent item per harness; the catalog ticket is therefore a
   prerequisite.
4. **(32), owner ack 2026-11-17: the "Never use {{NativeToolScheme}}" line
   renders only in hosted sessions; the CLI tier renders it empty.** Narrowed
   by Decision 23 and carried by Decision 24's daemon-injected text, which the
   CLI tier never receives. Rejected:
   rendering it everywhere, because the CLI plugin is frozen and CLI-tier leads
   still delegate through the native Agent tool on the shared playbooks.
5. **(33), owner ack 2026-11-17: (28)=a's single "hosted prompt assembly" child
   is split into two tickets:** prompt assembly (2, 3, B, C; this ticket) and
   compaction lifecycle (1, A, ws/compact;
   `261117-feat-ws-hosted-compaction-lifecycle`). Rejected: one ticket, because
   ws/compact is a separate runtime mechanism (session replacement, segment
   chain, record-shape compatibility tier) with different verification.

Decisions 6-17 were settled through the Open Decision Queue on 2026-11-18
(owner answers "(107) ok", "(108)", "109=a", "110", "116,117=a",
"111..114,118..120=ok").

6. **(107) Phase order: the daemon-delivered session key and workflow manual
   come first (Phase 1); the native-subagent steer and daemon-owned playbook
   body follow (Phase 2).** No code dependency orders them (the catalog has
   landed; the steer is unused by the manual work), and the owner
   prioritized the hosted-lead key gap ((102)=a). Rejected: the original order.
7. **(108) Hosted sessions get the full keyed workflow manual injected always,
   through the launch-time appended system prompt**: Claude
   `--append-system-prompt-file`, agy `_meta.antigravity.systemPrompt.append`.
   A Pi lead already meets this: its extension appends the manual on every
   `before_agent_start` (`agents-plugin-pi/src/lead-bootstrap.ts#L335-L350`)
   and takes its key from the hosted key env; only the bridge's fallback path
   (extension failed to load, `pi_lead_bridge.mjs#L50-L56`) lacks the manual,
   so the daemon adds it there (Decision 21). Owner note: the manual is "how to work" itself and must
   not depend on whether the model happens to call a tool. The injected text is
   a launch-time snapshot; the session calls `workflow_manual` again for fresh
   session state. Rejected: a short card plus first-turn pull; MCP initialize
   `instructions` (unprobed per harness).
8. **(118), revised by (123)=a on 2026-11-18: children get no lead workflow
   manual.** An `agent.spawn` child's "how to work" is its rendered playbook,
   which `agent.spawn` already delivers always as an appended system prompt,
   plus the key sentence `composeInstructions` adds today
   (`agents-plugin-tool/internal/wsagent/runner_process.go#L717-L742`); this
   meets the owner's (108) intent that a subagent's how-to-work arrives
   always, never through a tool call. Owner-created worker-profile sessions
   and card children keep the manuals their ready tickets define
   (`261115-feat-ws-worker-profile-session-spawn`,
   `261115-feat-ws-whiteboard-agent-card`). Rejected: appending the manual
   `workflow_manual` renders for the child's key (it is the lead-only
   orchestration manual, `internal/mcp/server_inventory.go#L30`, and
   `agent.spawn` children hold parent-less delegate keys that cannot re-read
   it); a new short delegate manual for every child (a new shipped artifact
   to maintain).
9. **(120) No fixed byte budget for the injected manual.** Manual-size
   optimization is separate work; this settles research Open Question 10 for
   this ticket.
10. **(109) On a hosted lead connection, a tool call that omits `session_key`
    resolves server-side to that connection's hosted key.** Rejected: relying
    on the prompt text alone.
11. **(117) `session_key` stays an optional argument with a permission check.**
    On a hosted lead connection: omitted resolves to the connection's key
    (10); an explicit key must be the connection's own key or a key of one of
    its descendant sessions; any other key (a peer lead's, or a peer's
    descendant's) is refused with an error naming the rule. Scope per (124)=a
    (2026-11-18): these rules apply to hosted lead connections only, because
    child connections carry no hosted identity
    (`agents-plugin-tool/internal/wsagent/runner_process.go#L140-L163`,
    `#L309-L317`) and a lead has no parent; refusing a child's use of its
    parent's key needs a child-connection identity and is
    `261118-feat-ws-hosted-child-connection-identity`. Rejected for now:
    giving child connections a hosted identity in this ticket. The CLI path is
    unchanged (frozen tier). Rejected:
    dropping `session_key` from every tool schema, which breaks the CLI and
    older plugins (effectively T3 with a minor bump).
12. **(110) Hosted Claude sessions launch without the ws/wsflow plugins** and
    use the agent-v2 adapter path like agy and Pi: the daemon injects the
    manual and key (7), and skills are exposed through a playbook whitelist plus
    a dashboard-injected slash-command list (14). The Claude plugin and the
    `kang-sw-devenv` marketplace stay as they are for the CLI path. Rejected:
    keeping the plugin's mod in hosted Claude alongside daemon injection.
    This owner direction supersedes, for hosted Claude, the catalog's rejection
    of "dropping the bundle entirely" (`261117-feat-ws-harness-capability-catalog`
    Decision 12, owner (65)) and the daemon-bundled plugin of
    `261113-feat-ws-hosted-lead-bundled-plugin-only`: the hosted Claude lead
    gets no `--plugin-dir` bundle, and the record's `plugin_isolation` field
    stays in place but no longer affects hosted Claude launches (removing it is
    out of scope). The mod's in-process `mcp__ws__request_*` tools (such as
    `request_compaction`) leave with the plugin; hosted compaction belongs to
    `261117-feat-ws-hosted-compaction-lifecycle` (ws/compact).
13. **(116) The plugin leaves hosted Claude in Phase 1, together with manual
    and key injection.** Until Phase 2 lands, hosted Claude has no skill
    shortcuts (the injected manual still names `playbook.read`), and until
    Phase 3 lands it has no plugin-hook ambient context. Both gaps stay inside
    `track/agent-v2`, whose develop merge is held ((115)=b) until the
    follow-up work accumulates. Rejected: removing the plugin at the end of
    Phase 3, which needs throwaway dedupe code while the mod and the daemon
    both inject the manual.
14. **(119) Skill exposure folds into Phase 2**: the playbook whitelist, the
    dashboard slash-command list, and the daemon's expansion of a command into
    its playbook body are the same command-to-body mechanism as the
    daemon-owned playbook body. Rejected: a separate phase.
15. **(111) Harness scope is hosted Claude, agy, and Pi.** Codex is excluded:
    it is not a registered hosted backend; its adapter is separate work.
16. **(113) No separate injection seam.** Implement the launch-time append
    only; ws/compact segment restarts refill the same `StartSpec.Instructions`
    field (a restart is a launch), and Phase 3 ambient text rides the
    harness's per-turn input, since `StartSpec.Instructions` applies only at
    process start (design review 2026-11-18). Rejected: a general injection layer
    now, which is speculative while the reuse point exists.
17. **(112) Compatibility tiers.**
    - Lead `StartSpec.Instructions` is in memory only: T0. No record, store,
      event, `/mcp` hello, or tool-argument shape changes.
    - `session_key` fallback and permission check (10, 11): argument shape
      unchanged, T0. Calls a hosted connection made with a peer's or parent's
      key now fail by design.
    - Hosted Claude launch without plugins (12): launch arguments are built in
      memory per start, T0. Downgrade direction: a plugin-less lead launched by
      a new daemon and rejoined by an older one keeps running, but the old
      lead result mapper accepts only `plugin:<pkg>:<pkg>` servers
      (`agents-plugin-tool/internal/wsagent/claude.go#L1500-L1511`), so its
      result-carrying ws tools (tracker, cards, UI) lose their mapped display:
      T1, which still permits a patch.
    - A lead launched by an older daemon and rejoined by a newer one stays
      keyless until restart: accepted T0 caveat. Rejected: persisting the
      injected text in the record.
    - Phase 2 whitelist: if it adds a manifest field, the field is optional
      and an old binary ignores it (T0); a required field is out of bounds.
      The dashboard slash list is served by the same daemon, T0.

Decisions 18-21 close the fact populator's decision gaps of 2026-11-18 on the
lead's technical judgment (no owner round; implementation detail under the
already-confirmed Decisions 7-12).

18. **A plugin-less hosted Claude lead reaches ws through a daemon-supplied
    `--mcp-config` without `--strict-mcp-config`, server name `ws`**, the same
    shape children use (`claudeMCPConfig`, `agents-plugin-tool/internal/wsagent/claude.go#L232-L245`),
    so the user's other plugins and MCP servers keep loading. The lead result
    mapper (`claudeMapper.resultTool`, `claude.go#L1500-L1511`), which today
    maps result-carrying ws tools for leads only from `plugin:<pkg>:<pkg>`,
    also accepts the `ws` server for hosted leads. Rejected: a plugin-style
    server name (needs a fake plugin wrapper); `--strict-mcp-config` (drops
    every user and claude.ai server, catalog Phase 2 probe).
19. **The descendant check walks the Runner lineage (`Record.Parent`, the
    authority parent of epic (84)=a) from the presented key's session up to
    the connection's session**, read through the caller-record path
    internal/mcp already uses (`callerRecord`, 6f4da285d); key-record parents
    are not the source, because `agent.spawn` and worker-profile keys are
    minted with an empty key parent (`runner.go#L793`, `agent_keys.go#L28`).
    It applies to every key-taking tool
    on a hosted lead connection, including `workflow_manual`, whose current gate
    admits any parent-carrying key (`agents-plugin-tool/internal/mcp/session_context.go#L118-L123`).
    The fallback and check sit at the top of `callTool`
    (`server_dispatch.go#L21-L89`) using `Server.hostedSessionKey()`
    (`process_env.go#L276-L279`). An omitted key or the connection's own key
    short-circuits by string comparison; only an explicit other key pays for
    the walk's record reads, so the common dispatch path does no file I/O.
    For hosted lead connections this supersedes ff312f48's allowance for
    keys minted through `ferrule(parent_session_key: <connection key>)`:
    such a key has no Runner record and is refused (no current caller: hosted
    Claude leads have native Agent removed, Pi children are not hosted
    connections). Rejected: a children index (new state to
    keep consistent; chains are short); the key-record parent chain (misses
    `agent.spawn` children).
20. **The lead manual render reaches the lead `StartSpec` through an injected
    `RunnerOptions` function supplied by the daemon**, next to `WSMCP`
    (`agents-plugin-tool/internal/wsagent/runner.go#L134`), because
    `internal/mcp` imports `internal/wsagent` and not the reverse. It serves
    leads only (Decision 8). Rejected: moving the render into
    `internal/wsagent` (pulls the server's session and config state across the
    package boundary).
21. **Pi keeps its extension's manual injection; the daemon adds the manual
    only on the bridge's fallback path**, where it already appends the key
    (`pi_lead_bridge.mjs#L50-L56`). Rejected: a daemon copy on every Pi lead
    plus suppressing the extension's block under `hostedMode()`, which changes
    `agents-plugin-pi/src` for no behavior gain and still needs dedupe.

Decisions 22-23 apply the design review's `autonomous` findings of 2026-11-18
on the lead's judgment.

22. **A plugin-less hosted Claude lead gets a daemon allow rule for the `ws`
    server's tools**, the same rule children get (`claudeAllowedRules`,
    `agents-plugin-tool/internal/wsagent/claude.go#L197-L225`), so ws calls do
    not each turn into a dashboard prompt under a non-auto permission mode.
    The tool names change from `mcp__plugin_<pkg>_<pkg>__*` to `mcp__ws__*`,
    so a user's allow, ask, or deny rule written against the plugin names no
    longer matches in hosted Claude leads; this is accepted and stated in the
    Phase 1 Result. Rejected: translating user rules into the new names
    (reads and rewrites user settings at every launch for a corner case).
23. **The "Never use" steer keys on the session's native-subagent policy,
    not on "hosted"**: a session under the removal policy
    (`wsagent.RemovesSubagents`, `internal/wsagent/agent_tools.go#L59`, which
    is harness-independent; not the Claude-only `subagentRemovedRecord`,
    `internal/mcp/playbook_tools.go#L162`) gets it,
    including on a harness whose native tool cannot be physically removed;
    exempt hosted sessions such as owner-created worker-profile sessions
    (`261117-feat-ws-agent-tools-mailbox-routing` Decision 6 exemption) and
    the CLI tier get none. This narrows Decision 4's "hosted".

Decisions 24-25 were settled through the Open Decision Queue on 2026-11-18
(owner answers "121,123,124=a", "122=ok"; Decision 8 and Decisions 10-11
above carry (123) and (124)).

24. **(121) The native-subagent "Never use" steer is daemon-injected text, not
    a playbook render variable**: the lead gets it in its Phase 1 appended
    manual, an `agent.spawn` child in its appended prompt next to the key
    sentence, keyed on Decision 23's policy and naming the harness's native
    subagent tool from the catalog's per-harness item. No shared playbook text
    changes, so the steer is T0. Rejected: a `{{.NativeToolScheme}}` render
    variable in shared playbooks, because playbook text comes from the
    connecting plugin and an older same-minor daemon fails closed on an
    unknown or unprovided variable (`ErrUndeclaredVar`/`ErrUnprovidedVar`,
    `agents-plugin-tool/internal/mcp/playbook_tools.go#L1085-L1125`; skew per
    `ai-docs/manuals/release-compatibility.md#L14-L26`); shipping daemon
    support first and the text later (still fails closed under skew); a minor
    bump (T3).
25. **(122) Phase 4 is T0**: `playbook.render` already returns the path plus
    `recommended-tier`/`recommended-model`/`recommended-reasoning-effort`
    (`playbook_tools.go#L529-L565`); a new playbook resource that uses only
    variables older binaries know loads on them (no name allowlist,
    `internal/wsrsrc/loader.go#L90-L114`); the embedded builtin profile text
    (`internal/wsagent/instructions/explore.md`, `builtin.go#L139-L146`) stays
    the fallback when the plugin lacks the playbook. Rejected: replacing the
    embedded text, which would drop the profile for an old plugin under a new
    daemon.

Decisions 26-30 settle (1900)-(1904) from the owner's 2026-11-20 answer
"(1900)=a", meaning all recommended options.

26. **(1900) Phase 3 covers hosted Claude, Codex, agy, and Pi.** Owner
    (1900)=a, 2026-11-20. This supersedes Decision 15 for Phase 3 only:
    Codex is now a registered hosted backend, and the ambient unit sits at the
    backend-neutral send boundary. Rejected: inventory Codex while excluding
    it from ambient injection, which would split the hosted prompt contract.
27. **(1901) Preserve Decision 21 during Phase 3.** Owner (1901)=a,
    2026-11-20. Pi's native `lead-bootstrap`, `model-prompts`, `lead-skills`,
    `goal-loop` hooks and the bridge remain enumerated exceptions;
    Phase 3 delivers only daemon-owned ambient input and does not migrate those
    hooks. Owner context, 2026-11-20: Pi's extension will instead be retired
    and Pi rebuilt from scratch against the common interface in the late epic
    child `261120-feat-ws-pi-common-interface-rebuild`. Rejected: migrating all
    Pi per-turn prompt and follow-up injection paths into the daemon in this
    phase; those paths include live skill discovery and model/fork behavior
    beyond the short ambient block.
28. **(1902) Inject only the canonical instruction resolved from
    `workflow.lang`, for hosted leads only.** Owner (1902)=a, 2026-11-20.
    An unset value adds nothing. Rejected: adding `lead.turn_reminder` here,
    which creates another config contract; mirroring arbitrary host user-hook
    output for every hosted role, which would leak lead reminders into workers.
29. **(1903) Set the renderable `explore` brief's recommended bare tier to
    `small`.** Owner (1903)=a, 2026-11-20. Callers may override the returned
    recommendation. Rejected: `medium`, because bounded read-only exploration
    fits the small tier.
30. **(1904) Repurpose the existing `explore` stem as child-facing appended
    instructions and remove `delegates: true`.** Owner (1904)=a, 2026-11-20.
    Rejected: retaining the caller-facing stem and adding another rendered
    stem, which would duplicate the existing named profile/playbook contract.

## Constraints

- Go tests run on remote w5090 only, never local `go test`.
- Shipped playbooks stay harness-neutral: per-harness differences are resolved
  by the daemon (the native-subagent steer is daemon-injected text, Decision
  24), never harness names in playbook text. Shipped text must not depend on anything only this
  repository has.
- Read before editing playbooks, skills, `agents-plugin/rsrc/` or
  `agents-plugin-wsflow/`: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md` (apply its invariant checklist to every
  changed Invariants/Constraints line) and `ai-docs/manuals/wsflow-mirroring.md`.
- Read before editing `agents-plugin-tool/`: `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`; for `agents-plugin-tool/internal/mcp/`
  also `ai-docs/manuals/ws-mcp.md`.
- Release-gate file-touch rule (owner, 2026-11-17): minimize daemon file touches
  at the root; no per-turn file writes or per-event O(N) rereads for injection;
  interactive paths never wait on file I/O. Related, not edited from here:
  `261117-research-ws-daemon-file-touch-map`.
- The Claude CLI tier is frozen: keep current features, add none. Hosted-only
  behavior must not change what the CLI plugin renders.
- Any cross-version surface touched (an MCP tool or argument, the `/mcp` hello,
  a render-variable manifest field) states its compatibility tier in
  `## Decisions` before `ready/` (AGENTS.md `### Compatibility Tiers`).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Decisions

- 261117-feat-ws-harness-capability-catalog (2026-11-17, Decisions 12): "Rejected: adding HTTP alongside the plugin's stdio server (two ws servers); dropping the bundle entirely (hooks and skills vanish before prompt assembly replaces them)." — bearing: contradiction-candidate
- 261117-feat-ws-harness-capability-catalog (2026-11-17, commit 4e8add8d, Phase 2 Result): "`--strict-mcp-config` (option 3) drops every plugin, user and claude.ai MCP server ... (option 2) names the server `ws`, not `plugin:ws:ws`" — bearing: constrains
- 261113-feat-ws-hosted-lead-bundled-plugin-only (2026-11-13, commit 876dd14b0, Background): "dashboard-hosted sessions (Claude and Pi) should use the ws/wsflow plugin bundled with the daemon and exclude any user-installed ws/wsflow copy" — bearing: contradiction-candidate
- 261110-feat-ws-hosted-session-mode-flag (2026-11-11, commit ff312f48): "workflow_manual lets a key whose record carries a parent through under the hosted flag ... child mints keep working for subagents on the same connection" — bearing: constrains
- 261113-bug-ws-hosted-session-sentinel-bootstrap-mints-foreign-key (2026-11-15, Resolution): "Hosted sessions already reject other keys in workflow_manual; ff312f485." — bearing: constrains
- 261018-feat-claude-mod-ws-manual-injection (2026-10-18, commit 04ee5872): "user chose (11)=D, minting a per-conversation lead key at session start ... key-less playbook_read render differs from the keyed manual (User preferences overrides), so a plain cut would drop key-specific text" — bearing: constrains
- 260904-feat-pi-lead-bootstrap-system-prompt (2026-09-04, Decisions): "### 2. Bridge session-key normalization — narrow, mechanical" — bearing: supports
- 261117-feat-ws-hosted-antigravity-adapter (2026-11-18, commit e83316ae): "No Record field added (an old binary drops unknown fields): T0 for the daemon's stores; an old daemon reading an agy row is T1." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | whole ticket (Phases 1-4): agents-plugin-tool/internal/wsagent/runner_process.go, claude.go, claude_lead_plugins.go, agy.go, pi_lead_bridge.mjs; agents-plugin-tool/internal/mcp/server_dispatch.go, session_context.go, workflow_manual.go; agents-plugin-pi/src/lead-bootstrap.ts (Pi dedupe); Phase 2-3 add agents-plugin/rsrc/, agents-plugin-wsflow/, agents-plugin/hooks/, wsdashboard composer; phase text is a plausible breakdown refined at ready promotion |
| scope.surface | cross-module | internal/mcp imports internal/wsagent and not the reverse (agent_tools.go#L11; search for internal/mcp in wsagent non-test files returned nothing), so the manual render handleWorkflowManual (workflow_manual.go#L311-L493) reaches wsagent only through a new injected RunnerOptions seam beside WSMCP (runner.go#L134) and LeadKeyStore (runner.go#L36-L48); also touches the Pi package and the dashboard |
| scope.new_public_symbol | unknown | Phase 1 likely adds an exported runner option for the lead manual render (Decision 20) and a hosted-key fallback in callTool; neither is named; no new playbook render variable (Decision 24) |
| scope.new_type_contract | unknown | the render-seam signature, the descendant-key rule (session_key admitted today for any parent-carrying key under the hosted flag, session_context.go#L118-L123, not only descendants) and the Phase 2 whitelist manifest field are unsettled |
| scope.test_surface | new-files | existing hosted tests agents-plugin-tool/internal/mcp/hosted_session_test.go, internal/wsagent/lead_claude_test.go, lead_http_test.go, lead_isolation_test.go, lead_bundled_plugin_test.go cover launch args and hosted keys; Phase 1 permission cases extend them, Phase 2 variable and whitelist tests and Phase 4 render tests are new; Go tests run on w5090 only |
| complexity.reuse_points | confirmed | StartSpec.Instructions is delivered by agy.go#L248-L254 and, for children only, claude.go#L193-L194 (claudeLeadArgs, claude.go#L103-L136, has none); composeInstructions (runner_process.go#L717-L742) already writes the child key sentence; AgentKeyStore.MintLead (mcp/agent_keys.go#L38-L44) mints the lead key; hostedSessionKey (mcp/process_env.go#L276-L279) is the per-connection hosted identity; claudeMCPConfig (claude.go#L232-L245) and enabledPlugins false (claude_lead_plugins.go#L53-L64) exist; dashboard slash lists already come from adapters (claude.go#L1194-L1229, agy.go#L957-L971, pi.go#L1099-L1106) |
| complexity.side_effect_risk | high | hosted Claude without the plugin: the lead mapper maps result-carrying ws tools only from plugin:<pkg>:<pkg> servers (claude.go#L1500-L1511) so a daemon-supplied server named ws loses tracker/card/UI results for leads; the mod's in-process mcp__ws__request_* tools (claude-lead-compaction.jsonl fixture) go with it; the central callTool fallback (server_dispatch.go#L21-L89) touches every hosted call |
| risk.correctness | high | new descendant check on an explicit session_key and a keyless fallback in the one central dispatch, plus exactly-one-manual dedupe across Claude plugin mod, Pi extension (lead-bootstrap.ts#L335-L350 already injects the static manual on every turn) and daemon copy |
| risk.fit | moderate | Decision 12 reverses the catalog's owner (65) Decision 12 rejection of dropping the bundle before prompt assembly lands, deliberately (owner (110), (116)=a) with the gap accepted in Decision 13; MCP name, mapper, permission walk and render seam settled in Decisions 18-21 |
| risk.test | moderate | launch-arg and permission-matrix tests are unit-reachable; first-turn agent.spawn needs a live hosted harness per kind, agy needs an account-backed smoke, and a no-plugin Claude launch needs a version probe |
| risk.security_or_contract | high | the permission check changes which keys a hosted connection may use (peer and parent refused), the lead key goes into model-visible prompt text, and a plugin-less launch changes which MCP servers and hooks a user's Claude loads |

## Phases

### Phase 1: Daemon-delivered session key and workflow manual

The daemon, not a harness plugin, gives every hosted session its session key
and workflow manual from the first turn (Decisions 7-13, 15-17).

- Hosted Claude and agy leads: the lead's `StartSpec.Instructions` carries
  the full workflow manual rendered for the lead's own key, minted by
  `Runner.CreateLead` / `AgentKeyStore.MintLead`
  (`agents-plugin-tool/internal/mcp/agent_keys.go#L38-L44`), through the
  daemon-supplied render function (Decision 20). Claude delivers it with
  `--append-system-prompt-file`: the develop hotfix merge 5c7797981 (impl
  fff872331, v0.50.7, synced into the track before this phase starts) already
  has `startLead` write a key-only `instructions.md` through
  `composeLeadInstructions` and `claudeLeadArgs` pass it with that flag, so
  this phase replaces that text with the rendered manual (the key sentence
  stays in it) rather than adding the flag. agy delivers it with
  `_meta.antigravity.systemPrompt.append` (`agy.go#L248-L254`).
- Hosted Pi leads (Decision 21): unchanged where the bundled extension loads
  (it appends the manual on every `before_agent_start`,
  `agents-plugin-pi/src/lead-bootstrap.ts#L335-L350`, and takes the key from
  the hosted key env, `agents-plugin-pi/src/bridge.ts#L1018-L1021`); on the
  bridge's fallback path (`pi_lead_bridge.mjs#L50-L56`, extension failed to
  load) the bridge appends the daemon-rendered manual next to the key it
  appends today. Each hosted session ends up with exactly one copy.
- Child sessions: unchanged by this phase; `agent.spawn` children keep the
  key sentence plus their appended playbook prompt (Decision 8).
- Hosted Claude launches without the ws/wsflow plugins (Decisions 12-13, 18):
  no `--plugin-dir` bundle, so no plugin skills, hooks, mod, or mod
  in-process `request_*` tools; installed ws/wsflow copies stay disabled with
  the existing `--settings` `enabledPlugins` false
  (`claude_lead_plugins.go#L53-L64`). ws tools reach the session through a
  daemon-supplied `--mcp-config` server named `ws`, without
  `--strict-mcp-config`, and the lead result mapper accepts that server
  (`claude.go#L1500-L1511`). That server runs the daemon's own package
  ws-mcp, the same `RunnerOptions.WSMCP` children use. Every launch through
  `claudeLeadArgs` is plugin-less, including owner-created worker-profile
  sessions (`Runner.CreateLead` with a non-lead mint); confirm their
  short-manual delivery (`261115-feat-ws-worker-profile-session-spawn`) does
  not depend on the plugin mod, and report it if it does. The user's CLI installation, the Claude plugin,
  and the `kang-sw-devenv` marketplace are not modified. If a live launch on
  the installed Claude version does not expose the `ws` server alongside the
  user's other MCP servers, stop and report what it does instead of choosing
  another mechanism.
- `session_key` on a hosted lead connection (Decisions 10-11, 19): omitted
  resolves to the connection's key; an explicit key must be the connection's
  own or a descendant session's (Runner lineage walk); any other key is
  refused with an error naming the rule, `workflow_manual` included. Child
  and non-hosted connections behave as today.

Dogfood evidence (2026-11-18, track/agent-v2): a hosted lead starts with no
session key and no workflow manual (true of an agy lead and of a Claude lead
whose bundled plugin does not deliver them: the daemon builds a lead's
`StartSpec` with no `Instructions`, `agents-plugin-tool/internal/wsagent/runner_process.go#L265-L326`,
and `agy.go#L248-L254` appends only `spec.Instructions`; a native-extension Pi lead gets
its key from the hosted key env and re-login (`agents-plugin-pi/src/bridge.ts#L1018-L1021`,
`#L1258-L1261`) and a fallback Pi lead from `pi_lead_bridge.mjs#L52-L53`, a Claude lead with the plugin loaded adopts it
through the mod, `agents-plugin/hooks/claude-mod/manual.ts#L555-L568`). On a scratch daemon a hosted Claude lead's
`agent.spawn` failed with "session_key is required", and the agy Phase 2 live
lead recovered only by calling `ferrule` itself
(`261117-feat-ws-hosted-antigravity-adapter` Phase 2 Result, Forward).

Deferred: skill shortcuts in hosted Claude (Phase 2), plugin-hook ambient
context (Phase 3).

Verification:

- Go tests on remote w5090: each hosted lead kind's start carries the keyed
  manual in its appended prompt; a child's appended prompt is unchanged; the
  hosted Claude launch arguments load no ws/wsflow plugin
  and carry the `ws` `--mcp-config`; the lead mapper maps results from it;
  a fallback-path Pi lead's bridge prompt carries the manual;
  `session_key` omitted, own, descendant (an `agent.spawn` child), and peer
  cases on a hosted lead connection, and unchanged child and non-hosted
  connections; the ws allow rule (Decision 22) is present.
- A live hosted lead per available harness (Claude, Pi; agy when an account is
  available) calls `agent.spawn` successfully on its first turn without
  calling `ferrule` and without passing `session_key`.
- No per-turn file writes: the manual is written at most once per session
  start.

### Result (f4ff30388) - 2026-11-18

Commits:
- c7e7b3114: the spine.
- 00e68753a: the refusal names its ws-mcp cause.
- 2ede7478b: tests.
- f4ff30388: lite-review fixes.

Design record:

- Render path (Decision 20).
  - `wsagent.Options.LeadManual` (`runner.go:143`) is supplied by the daemon as `renderLeadManual` (`cmd/ws-mcp/agent_mcp.go:78`).
  - It resolves the hosted package env the same way `agentMCPCommand` does:
    - the daemon env, plus the package env, plus `WS_MCP_PROJECT_ROOT`;
    - mailbox identity cleared;
    - `callerHello` defaults applied.
  - It calls `mcp.RenderLeadManual` (`workflow_manual.go:545`). That function strips the hosted-session variables and the harness PID, so the render is never a hosted session itself, and it returns the CONTINUE manual without the read-full notice.
  - The harness is detected as `claude` for a Claude lead and as none otherwise.
  - The throwaway server is closed once its session-start tracker check settles.
- Lead prompt (Decisions 7, 8, 16).
  - `startLead` writes `instructions.md` through `composeLeadInstructions(dir, rec, manual)` (`runner_process.go:256,760`). This happens on every non-attach Claude and agy launch, `--resume` included, before `writeRegistration`.
  - The file holds the key sentence, then "The workflow manual below is binding; its session state is as of this session's start", then the manual.
  - A render error or an empty render keeps the old key-only text and is logged (`leadManual`, `:779`).
  - Claude passes the file with `--append-system-prompt-file`; agy passes it as `_meta.antigravity.systemPrompt.append` (unchanged code).
  - A Pi lead on the bridge fallback embeds `LeadSpec.Manual()` as `workflowManual` (`pi_lead.go:141`, `pi_lead_bridge.mjs`). The native-extension path neither renders nor embeds it (Decision 21).
  - Children's prompts are unchanged.
  - The render happens once per process start, so there are no per-turn writes.
- Plugin-less hosted Claude (Decisions 12, 18, 22).
  - `claudeLeadPluginArgs` (`claude_lead_plugins.go:21`) now only disables enabled `ws`/`wsflow` ids through `--settings`. There is no `--plugin-dir` bundle.
  - The hosted HTTP-variant code is deleted (`claude_hosted_variant.go`), along with the builtin-plugin seed.
  - `claudeLeadMCPArgs` (`:49`) adds `--mcp-config {"mcpServers":{"ws":…}}`, without `--strict-mcp-config`:
    - the HTTP route when the start has a session;
    - otherwise the daemon's stdio ws-mcp (`spec.WSMCP`, set in `startLead` from `Lead.WSMCP`).
  - `claudeLeadArgs` ends with `--allowedTools mcp__ws` (`claude.go:148`).
  - `claudeMapper.resultTool` (`claude.go:1512`) maps `mcp__ws__*` for leads. It still accepts `plugin:<pkg>:<pkg>` for a lead an older daemon started.
- Key rule (Decisions 10, 11, 19).
  - `hostedSessionKeyRule` (`mcp/hosted_key.go:26`) runs first in `callTool` (`server_dispatch.go:36`), on hosted lead connections only.
  - An omitted, null, or blank `session_key` is filled with the connection's key when the tool's schema declares the argument (`ferrule` does not).
  - An explicit key passes when it is the connection's own key or a Runner-lineage descendant's (`hostedDescendant`, `:72`). The walk follows `Record.Parent` reply-ids through `callerRecord`/`ReadRecord`, bounded at 16 levels and failing closed.
  - Any other key is refused with an error naming the tool, both keys, `WS_HOSTED_SESSION_KEY`, and the descendant rule.
  - `workflow_manual`'s gate (`session_context.go:121`) applies the same rule.
  - A key minted by `ferrule(parent_session_key: <hosted>)` has no Runner record and is refused on the hosted connection.
  - Omitted and own keys are settled by string comparison and read no file.
- Tiers, as Decision 17 settled them: T0 for the prompt, the key rule, and the launch args; T1 for the downgrade display (an older daemon's mapper on a rejoined plugin-less lead).

Worker decisions (veto points):

- A Claude lead with no ws-mcp at all (no HTTP route and no resolvable launcher) is refused with `claude lead: no ws-mcp to attach: <cause>`. It is not started without ws tools. This follows the agy precedent.
- `plugin_isolation` no longer applies to a Claude lead. With no bundle, the only plugin override left is disabling the installed ws packages. `isolated` no longer disables the user's other plugins or the builtins, and an unknown value no longer refuses the launch.
- Per Decision 22, a user's allow, ask, or deny rule written against `mcp__plugin_<pkg>_<pkg>__*` no longer matches in a hosted Claude lead. The tools are now `mcp__ws__*`, and the daemon allows the whole `ws` server.
- Worker-profile sessions (`261115-feat-ws-worker-profile-session-spawn`) are not in code yet. Their planned short-manual delivery is the same daemon-side prompt path, not the plugin mod, so they do not depend on the plugin.
- An agy lead keeps its native subagents (`RemovesSubagents` is honored only by Claude). A child minted through `ferrule(parent_session_key: <hosted>)` on its connection is now refused (Decision 19), so Decision 19's "no current caller" premise holds for Claude and Pi but not agy.

Verification:

- On w5090:
  - `go build ./...` passed.
  - `go vet` of `internal/wsagent`, `internal/mcp`, and `cmd/ws-mcp` passed.
  - `go test ./internal/wsagent/... ./internal/mcp/... ./cmd/ws-mcp/... -count=1` passed for all three trees at 2ede7478b. A targeted mcp rerun (`Hosted|RenderLead|WorkflowManual|ResolveSession|Tracker`) passed at f4ff30388.
- The Pi bridge JS tests (`pi-lead.test.mjs`, `pi-lead-health.test.mjs`) passed under local node.
- Lite review: clean with 5 minors. Two of them are fixed in f4ff30388.
- Not done: the live hosted-lead check per harness (first-turn `agent.spawn` with no `ferrule` and no `session_key`), and the check that the installed Claude exposes the `ws` server alongside the user's other MCP servers. The owner rule forbids running ws executables on the worker's PC.

Forward:

- Run the live check before Phase 2 builds on the plugin-less launch. Phase 1's stop rule applies if the `ws` server is not exposed.
- The render consumes one-shot manual nudges (the review-track nudge and the daily cleanup slot) even when the launch is then refused.
- On the no-HTTP stdio fallback, the ws-mcp env (which includes `WS_HOSTED_SESSION_KEY`) appears in the `--mcp-config` argv. This is the same shape as a worker's inline server.

### Phase 2: Native-subagent steer, daemon-owned playbook body, and skill exposure

Add the native-subagent "Never use <tool>" steer to the daemon-injected text
(Decisions 23-24): the lead's appended manual and an `agent.spawn` child's
appended prompt, for sessions under the removal policy, naming the harness's
native subagent tool from the catalog's per-harness item (harnesses without a
catalog value yet get none until their stage-3 values land). No shared
playbook text changes. Update the catalog's agy lead-row `Source` text,
which still says a per-harness playbook line covers the steer
(`internal/wsagent/catalog.go#L420-L421`), to name the daemon-injected text.

The daemon inserts a playbook body directly into a hosted session's prompt
when a command is invoked, removing the skill-invocation-to-`playbook.read`
double pointer. Skill exposure uses the same mechanism (Decision 14): a
whitelist of user-invocable playbooks, the dashboard composer offering that
whitelist as a slash-command list for hosted sessions, and the daemon
expanding a sent command into its playbook body plus the user's arguments.
The whitelist is the package's user-invocable lead skills (the `lead-*` skill
shims the CLI plugin exposes), held daemon-side; it follows Decision 17's tier
rule. Only user-sent commands lose the double pointer: a playbook the model
chooses to use mid-turn is still read through `playbook.read`.

Deferred: ambient injection, `agent.spawn` system prompt delivery.

Verification: Go tests on remote w5090 for the steer per harness and policy
(a removal-policy lead and child carry it with their harness's tool name; an
exempt worker-profile session and the CLI tier carry none), for whitelist
resolution, and for command expansion into the body without a
`playbook.read` round trip; playbook mirroring/shim drift tests per
`wsflow-mirroring.md`; a recorded hosted turn (Claude without plugins) showing
a slash command from the dashboard list expanded into its body.

Dogfood additions (owner (160)=a, 2026-11-19). The Phase 1 Forward live check
ran on a hosted Claude lead (daemon `dcc8dc31e`): the `ws` server is exposed
(79 tools as `mcp__ws__*`) and the key fill works. It surfaced these defects,
which this phase also fixes:

1. The hosted render names tools `wsflow/<tool>` and skills `wsflow:lead-*`,
   but the hosted server is `ws` and the session has no skills. The hosted
   render uses the connection's server name for tool references and the
   playbook route (command expansion or `playbook.read`) instead of skill
   names.
2. The manual arrives twice: in the appended system prompt and again from a
   `workflow_manual` call (about 25 KB), with different harness wording (the
   prompt copy is Claude-specific, with a "Claude delegate spawn" section and
   "the Explore agent"; the tool copy is generic). One source; a hosted
   session is not told to fetch what its prompt already carries.
3. Text points at tools a hosted Claude lead does not have: the native Agent
   tool (removed there) and the user-preferences `pi.*` workers (hidden;
   `agent.*` replaces them). The hosted render steers to `agent.*`.
4. The rendered manual duplicates the tier phrase: "a `wsflow/agent.spawn`
   subagent at tier small with read-only authority at the tier the question
   needs".

The injection path stays harness-neutral (descriptor-driven, no harness-name
branch in common code), so a hosted agy lead receives the same expansion and
render; tests cover an agy lead alongside Claude. Codex is out of scope until
its hosted adapter lands.

### Result (e13aa9d5d) - 2026-11-19

Commits:
- e2aca179c: the phase.
- a552c0b89: review round 1 fixes.
- e13aa9d5d: review round 2 fixes.

Design record:

- Steer (Decisions 23-24).
  - `Runner.subagentSteer` (`wsagent/hosted_prompt.go`) returns "Never use the native `<tool>` tool. Start a subagent only with the ws `agent.spawn` tool, and continue one with the ws `mailbox.send` tool."
  - It applies only under `RemovesSubagents(rec)`, and only when the backend catalog's `subagent_removal` item names a tool for the role (`HarnessProfile.NativeSubagentTool`, any grade).
  - Values: Claude lead and worker `Agent`; Codex lead `spawn_agent`; agy none yet (its lead-row `Source` now names this line); Pi has no catalog.
  - `composeLeadInstructions` puts it after the lead key sentence. `composeInstructions` appends it to a worker's key sentence.
  - There is no harness-name branch, and no shared playbook text changed.
- Playbook commands (Decision 14).
  - `BackendRegistration.PlaybookCommands` is set for Claude and agy.
  - The whitelist is derived, not hand-listed. `wsrsrc.PreloadCommands` returns the package's skill shims whose `<ws-preload playbook="X">` names their own directory, which excludes `lead-revive` (inline) and `lead-proceed` (alias). `mcp.HostedPlaybookCommands` then drops `lead-workflow-manual`, because the prompt carries the manual.
  - The runner reads the list once per runner (a failed read is cached and logged once), and stamps each entry `Source: "ws"`.
  - `withPlaybookCommands` merges the list into a lead's dashboard commands at creation (seed), on every harness-reported list (`KindCommands`), at caps changes, and whenever a lead conversation opens (rejoin, takeover, reopen). Harness commands of the same name, and earlier runners' `ws` entries, are replaced.
  - `conv.kick` sends a whitelisted command as one turn: `playbookCommandTurn`, which is the command line, its arguments, and `<playbook name=…>body</playbook>`. There is no `playbook.read` round trip.
  - The body is `mcp.RenderLeadPlaybook` for the lead's key. It shares `hostedRenderServer` with `RenderLeadManual`, and its text equals `playbook.read` on the lead's own connection (`Server.readPlaybook`, extracted from the dispatch case).
  - A failed render sends a pointer turn to `playbook.read` and is logged.
  - The page's user entry stays the typed line. Other commands still run as the harness's own.
- Dogfood defects.
  - (1) On a lead connection whose backend has `PlaybookCommands` (lead only; a worker renders as before):
    - `runtimeNamespace()` is the reserved `ws` server;
    - `{{.SkillNamespace}}:` refs and `{{ns}}:` lines render as `/name`;
    - delegation renders as `agent.spawn`.
  - (2) The lead prompt tells such a lead that the manual it carries is the only copy. For fresh state, and in place of `/lead-revive`, it calls `workflow_state`. The throwaway render server lists agent tools (`leadRender`), so the prompt copy and the tool copy share the same wording, and no "Claude delegate spawn" overlay or "the Explore agent" text appears.
  - (3) The native Agent line is now the steer. The `pi.*` worker text in that report comes from the user's own `UserPreferenceSection` override, which is user config, so the daemon render does not rewrite it. Fix it in user config.
  - (4) The daemon-agents `ExploreAgent` value no longer carries a tier ("a read-only `ns/agent.spawn` subagent"). This matches the tierless host values every site already renders with. It also removes "tier small" from the other daemon-agents sites (Pi MCP-only, and agent.spawn workers' renders).
- Tiers: T0. Every addition is a runtime-only descriptor or Options field, or prompt text. No store, hello, tracker, or manifest shape changed.

Worker decisions (veto points):

- The expansion renders on the conversation loop, once per command run. This adds one throwaway server's config, session-store, and package reads before the turn starts, instead of the session's own `playbook.read` during the turn. It was not measured on the Windows baseline (performance.md Rule 7) because the owner rule forbids ws executables here. Rejected alternative: rendering at the dashboard HTTP route, which blocks the request.
- A Codex lead (registered, catalog `spawn_agent`) now gets the steer line through its catalog value. It gets no playbook commands and no namespace change.
- The Pi bridge-fallback lead's daemon-rendered manual now uses `agent.spawn` wording (`leadRender` lists the agent tools), matching its real MCP-only connection.
- The exempt-session case is tested with a tracker-support profile, since worker-profile sessions are not in code yet. Agy is covered by unit tests (steer, connection descriptor, manual and playbook render), not by a runner-level agy lead test.
- Unused `formatPromptOverrideListing` still hard-codes `ws:lead-tune`. It has no callers, so it was left alone.

Verification:

- On w5090 at e13aa9d5d / a552c0b89:
  - `go build ./...` passed.
  - `go vet` of `internal/wsagent`, `internal/mcp`, `internal/wsrsrc`, and `cmd/ws-mcp` passed.
  - `go test ./internal/wsagent/... ./internal/mcp/... ./internal/wsrsrc/... -count=1` passed. The new tests were repeated with `-count=5`.
  - `go test ./cmd/ws-mcp/...` passed except `TestHostedLauncherSurvivesReplacementAndRestartsRecordedBuild`. That test clears `WS_RSRC_DEV_CHECKOUT` and needs the gitignored embedded package assets (`generated/inventory.json`), which the remote checkout does not generate. This is environmental and unrelated to this phase.
- New tests:
  - `TestSubagentSteerFollowsPolicyAndCatalog`
  - `TestAgentChildPromptCarriesTheSteer`
  - `TestHostedLeadPlaybookCommands`: exact prompt, seed and merge, expansion, pointer, harness command, ring entries, one read, rejoin refresh under a held scan.
  - `TestPlaybookCommandsFailedReadOffersNone`
  - `TestPlaybookCommandTurnText`
  - `TestPreloadCommands*`: both shipped packages, plus a synthetic tree.
  - `TestHostedPlaybookCommandRender`: Claude and agy, with wsflow as the package.
  - `TestHostedPlaybookCommandReadMatchesTheConnection`
  - `TestHostedPlaybookCommandsList`
  - Descriptor assertions for agy lead and worker connections.
- No playbook or skill text changed, so the mirroring and shim drift checks have nothing new. The wsrsrc mirror tests passed.
- Review: two rounds by fresh independent reviewers. Round 1 (correctness, and fit/test) found no blockers and 6 should-fix items, which are fixed in a552c0b89 or recorded above. Round 2 found no blockers and 1 should-fix, fixed in e13aa9d5d.
- Not done: a recorded hosted turn (Claude without plugins) showing a dashboard slash command expanded into its body. The owner rule forbids ws executables on the worker's PC.

Forward:

- Record the live hosted turn, and measure a command run's file operations on the Windows baseline.
- A hosted lead on the stdio ws-mcp fallback (no daemon runner) has no descriptor `playbookCommands`, so its own `playbook.read` still renders package names. The daemon expansion is unaffected.

#### Edition (this-commit) - 2026-11-20

- Supersedes the second Forward item above: there is no hosted-lead stdio
  fallback. The launchers are daemon proxies and explicitly never select a
  second server when the peer is missing, incompatible, or hung
  (`agents-plugin/bin/ws-mcp-launcher.py:952-984`, verified on `develop` at
  `06e615956`). Direct `ws-mcp serve --stdio` is an unsupported raw-binary
  diagnostic case, not a hosted lead; its lack of descriptor
  `playbookCommands` does not describe a supported hosted prompt path.

### Phase 3: Per-turn ambient injection replacing plugin hooks

The daemon adds one backend-neutral ambient block to each hosted lead turn's
input across hosted Claude, Codex, agy, and Pi (Decision 26). That block is the
canonical instruction resolved from `workflow.lang` (Decision 28); an unset
value adds nothing. It does not copy arbitrary user hooks or introduce
`lead.turn_reminder`. `StartSpec.Instructions` remains launch-only (Decision
16), and Pi's native prompt hooks remain the enumerated Decision 21 and 27
exceptions until the late epic rebuild.

#### Current hook and ambient inventory

| source | what reaches model context | hosted recipients today | Phase 3 disposition |
|---|---|---|---|
| `agents-plugin/hooks/hooks.json` | The only command hook is the `SessionStart` matcher for `compact`, which prints the revive nudge. | No plugin-less hosted Claude lead; the ws/wsflow CLI plugin only. | Explicitly excluded; `261117-feat-ws-hosted-compaction-lifecycle` owns it. |
| `agents-plugin/hooks/claude-mod/register.ts` manual/preload rewrites | A keyed workflow-manual row on lead-skill expansion, repeat-read collapse, and manual adoption notices around `workflow_manual` / `playbook_read`. | None in hosted Claude after Phase 1 removed the plugin. | Already replaced by Phases 1-2, not Phase 3 ambient. |
| `register.ts` mailbox/dashboard steering | Mail and promoted dashboard text can be appended at a main-thread tool boundary; prompt/turn hooks otherwise observe the stream for feed, captions, status, compaction, and reconnect state. Hosted mode disables the mailbox watcher and dashboard feed (`register.ts#L984-L993`, `#L1217-L1223`). | No hosted backend receives this mod injection. | No text to reproduce. The daemon already joins claimed mail and child reports into the next ordinary input (`runner_conv.go#L1129-L1133`, `#L1212-L1219`). UI/feed observers are not model ambient. |
| Claude mod compaction/reconnect paths | Context-window advisory and milestone prompts, an optional post-compaction continuation (`timing.ts#L145-L169`, `#L196-L204`; `compaction.ts#L29-L65`), and MCP reconnect success/failure plus optional continuation prompts (`reconnect.ts#L71-L80`, `#L340-L377`). | None in hosted Claude after Phase 1 removed the plugin. | Do not reproduce. `SessionStart(compact)` remains separately excluded, and these plugin-owned lifecycle prompts are not the short daemon ambient block. |
| owner user-level Claude hooks | `UserPromptSubmit` and main-agent `PostToolUse` add `(응답 언어: 한국어)` as `additionalContext` in the current owner settings. These are user hooks, not shipped ws hooks. | Hosted Claude, because the plugin-less launch still keeps user settings. | Do not inspect or emulate arbitrary user hook output. The `workflow.lang` instruction is the ws-owned replacement (Decision 28); the owner can remove the duplicate local hook. |
| `agents-plugin/.codex-plugin/hooks.json` | A `Stop` command can block completion and ask the model to drain unread mail from its named `WS_MAILBOX` inbox. | The Codex plugin hook is preserved because hosted Codex keeps plugins and user hooks (`codex.go#L224-L248`), but it is silent in daemon-hosted sessions because `runner_process.go#L355-L357` blanks `WS_MAILBOX` and `WS_MAILBOX_AUTO`; the daemon owns mail delivery. No response-language or other per-turn ws ambient is supplied. | No replacement text. Codex receives the backend-neutral language block (Decision 26); user Codex hooks remain host-owned. |
| agy adapter | No plugin/hook ambient facility. `_meta.antigravity.systemPrompt.append` carries launch-time instructions, and later `Send` calls pass turn text directly (`agy.go#L296-L311`, `#L364-L378`). | Hosted agy. | New ambient block rides the common turn input; no agy-specific file. |
| Pi `lead-bootstrap.ts` | Every lead/fork `before_agent_start` appends the static manual snapshot, session-state pointer, Pi guide, live `<available_skills>`, and Pi agent-compat guide (`lead-bootstrap.ts#L1-L28`, `#L304-L350`). | Native-extension hosted Pi leads and forks; not worker/explore roles. | Preserve under Decision 27 / Decision 21. It is not duplicated in the daemon ambient block. |
| Pi `model-prompts.ts` | Every turn replaces a length-framed `<ws-model-prompts>` block with user-configured general and lead-only text for the selected model (`model-prompts.ts#L9-L31`, `#L68-L103`). | Native-extension hosted Pi leads/forks, subject to role/model config. | Preserve under Decision 27; model-specific and fork-inheritance semantics stay extension-owned. |
| Pi `lead-skills.ts` expansion | Typed `/skill:` input and `ws-skill` results can replace a shim preload with a rendered playbook body or a pointer to its visible copy (`lead-skills.ts#L310-L360`, `#L377-L429`). | Native-extension hosted Pi leads/forks when the skill tool is active; typed expansion requires interactive or RPC input. | Preserve under Decision 27. This is explicit skill invocation, not daemon ambient. |
| Pi `goal-loop.ts` follow-up | While a lead goal remains active, settlement can submit a follow-up reminder with achieve/block/compact directions, context usage, and carried-forward text (`goal-loop.ts#L533-L550`, `#L1072-L1119`, `#L1412-L1445`). Its separate advisory, milestone, and hard-cut compaction preparation is disabled in hosted mode (`goal-loop.ts#L1477-L1503`). | Hosted Pi lead, conditional on an active goal. | Preserve under Decision 27. Goal lifecycle remains extension-owned and is not duplicated in the daemon ambient block. |
| Pi generated bridge | `before_agent_start` appends extension-health/version warnings; only the fallback path also appends hosted identity and the daemon-rendered manual (`pi_lead_bridge.mjs#L46-L59`, `pi_lead.go#L146-L171`). | Hosted Pi; identity/manual only when the native extension failed and MCP fallback is active. | Preserve. Warnings and fallback identity/manual are not language ambient. |
| Pi active-tool hook | `index.ts#L547-L554` filters the active tool set before each turn. | Hosted Pi roles with delegation policy. | Not model context and outside this phase. |

There is no shipped Codex, agy, or Claude `UserPromptSubmit` response-language
equivalent to remove. The owner's user hook is the observed use case; the
daemon must own only the equivalent ws configuration, not undiscoverable host
configuration. SessionStart(compact) remains excluded throughout this phase.

#### Daemon seam and ambient unit

`conv.kick` is the only ordinary hosted-turn send seam. It drains dashboard
input, child reports, and claimed mail, then sends either their joined text at
`runner_conv.go#L1212-L1215` or an expanded playbook body at `#L1215-L1219`.
Both paths currently call `compactInput`; `compaction.go#L420-L425` prepends and
consumes a queued restore/advisory note once. Adapter `Process.Send` methods then
translate that one string into their native turn APIs. The dashboard user row
is built later from the original parts (`runner_conv.go#L1281-L1297`), so a
model-only wrapper does not create a fake visible user message.

Add `internal/wsagent/ambient.go` with one `turnInput` composition unit and use
it at both `Process.Send` calls. The unit must:

- apply to hosted leads only, after the ordinary/playbook body is chosen;
- preserve `compactInput`'s one-shot prefix, then append one delimiter-framed
  ambient block so the response-language instruction remains last and salient;
- resolve `workflow.lang` through the conversation's existing rooted/session
  config view, use the same canonical instruction text as playbook/manual
  rendering, and cache the rendered block in memory when the conversation is
  restored/created, outside `Process.Send`;
- refresh affected open conversations from the successful `config.tune`
  set/reset path and publish their fully rendered blocks into the Runner in
  memory before the next turn. The turn path never resolves config, polls a
  generation, reads/writes a file, or starts an adapter subprocess; an external
  file edit takes effect on daemon restart unless it goes through
  `config.tune`;
- keep the original turn text for page/history events and retries, and avoid a
  block entirely when the resolved value is empty;
- add no record, event, MCP argument, manifest field, or on-disk shape. This is
  T0 in both directions: an old daemon keeps sending the unwrapped input, and a
  new daemon stores no state an old daemon can drop.

Frozen file list under Decisions 26-28:

- `agents-plugin-tool/internal/wsagent/runner_conv.go` - yes: add loop-owned
  ambient cache fields and route both send sites through `turnInput`.
- `agents-plugin-tool/internal/wsagent/ambient.go` - new: resolver, canonical
  language instruction, cache invalidation, and composition.
- `agents-plugin-tool/internal/wsagent/ambient_test.go` - new: backend/role,
  ordinary/playbook, compaction ordering, empty value, retry, and no-reread
  cases.
- `agents-plugin-tool/internal/wsagent/runner.go` - yes: seed the in-memory
  ambient cache as each conversation is restored/created.
- `agents-plugin-tool/internal/mcp/playbook_tools.go` - use the ambient unit's
  canonical language formatter so playbook/manual and turn text cannot drift.
- `agents-plugin-tool/internal/mcp/agent_backends.go` - add the narrow optional
  in-memory ambient refresh capability exposed by the daemon Runner.
- `agents-plugin-tool/internal/mcp/server_dispatch.go` - signal ambient-cache
  refresh after successful `workflow.lang` set/reset, on the tune path rather
  than the next turn.
- `agents-plugin-tool/internal/mcp/config_workflow_lang_test.go` - cover live
  set/reset invalidation through the real `config.tune` path.

`runner_process.go` is **not touched** (launch-time instructions are the wrong
seam); `children.go` is **not touched** (its reports already enter the joined
turn); `agent_tools.go` is **not touched**; no `rsrc/manifest.json` or other
manifest is touched. Pi extension/bridge, `agents-plugin-pi/src/lead-skills.ts`,
`agents-plugin-pi/src/goal-loop.ts`, and generated package artifacts belong to
the late epic rebuild, not this phase (Decision 27).

Verification: remote w5090 Go tests exercise each of Claude, Codex, agy, and Pi
through ordinary and expanded-playbook turns; lead versus
worker; set, reset, and empty `workflow.lang`; a compaction note plus ambient
block exactly once and in order; retries without duplicated ambient text; and
the unchanged dashboard-visible user text. The Windows baseline records file
operations and send-to-processing latency before/after: steady-state turns add
zero file operations and no writes. Live smoke confirms the response language
without the owner's duplicate hook enabled.

### Result (75274d98) - 2026-11-21

Commits:
- `e18d84bde`: runner-owned ambient cache, canonical formatter, and shared
  ordinary/expanded-playbook `turnInput` seam.
- `0985f2aeb`: successful `workflow.lang` set/reset refresh through the
  daemon runner's optional in-memory capability.
- `9dff34aa6`: real hosted-lead lifecycle coverage for refresh, retry,
  expanded playbooks, and model-only history.
- `75274d98f`: Lite-review fix adding real Claude, Codex, agy, and Pi send-path
  coverage.

Design record:

- `wsagent.WorkflowLanguageInstruction` is the one canonical formatter used by
  ambient and playbook/manual rendering (`internal/wsagent/ambient.go:12-20`,
  `internal/mcp/playbook_tools.go:331-335`). An empty resolved value emits no
  block.
- Each conversation resolves its fully rendered `<ws-ambient>` block once at
  creation/restoration through the existing rooted config options and optional
  session reader (`ambient.go:31-63`, `runner.go:646-658`). Non-lead records
  short-circuit before any config read.
- Both ordinary and daemon-expanded playbook turns call `turnInput`; it applies
  the one-shot compaction prefix first and appends ambient last
  (`ambient.go:65-76`, `runner_conv.go:1232-1239`). Queue, page, history, and
  retry state keep the original unwrapped text.
- A successful `workflow.lang` set/reset synchronously calls the runner's
  optional `RefreshAmbient` capability before `config.tune` returns
  (`mcp/agent_backends.go:13-27`, `server_dispatch.go:536-543,630-637`). Failed
  validation/writes and unrelated knobs do not refresh. Refresh republishes
  every open conversation in memory so global, repo, and session-scope changes
  all settle before the next turn; workers still short-circuit without reads
  (`ambient.go:79-91`). The send path performs no config resolution, file I/O,
  generation polling, or subprocess work.
- Pi's native extension, model prompt, skill, goal, and bridge behavior remains
  untouched per Decisions 21 and 27. No plugin hook, `runner_process.go`, Pi
  extension, resource, manifest, or generated artifact changed.

Compatibility:

- T0 in both directions. The change adds only runtime-only fields, an optional
  Go interface, and model prompt text. It changes no MCP tool or argument,
  `/mcp` hello, record/store/event shape, config/rsrc manifest, or persisted
  value. Old and new binaries continue reading the existing `workflow.lang`
  key; an old daemon simply sends the pre-Phase-3 unwrapped input, while a new
  daemon persists no ambient state for an old daemon to drop.

Verification:

- `GOCACHE=/private/tmp/ws-hosted-prompt-go-cache GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsagent -run 'Ambient|TestHostedLeadPlaybookCommands|TestLeadFailedSendShowsTheMessageOnce|TestCompactionAutoDoesNotStartContinue' -count=1` passed.
- `GOCACHE=/private/tmp/ws-hosted-prompt-go-cache GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/mcp -run 'TestConfigTuneWorkflowLang|TestPlaybook.*WorkflowLang' -count=1 -timeout 45m` passed.
- The first local test attempt did not compile because the sandbox refused the
  default Go build-cache path; setting the scratch `GOCACHE` above resolved the
  environment failure. No implementation/test failure repeated.
- Lite review found one Important gap (real backend send coverage was
  Claude-only); `75274d98f` fixed it for Claude, Codex, agy, and Pi. Per Lite
  policy there was no second review. The generated findings file remained
  empty because the read-only reviewer could not write it; the delivered
  finding was applied from the reviewer completion report.

Not run under the caller's constraints: live provider dogfood, daemon swap,
and the Windows file-operation/latency baseline. The implementation-level
steady-state test proves repeated `turnInput` calls make no config reads; the
platform syscall count and live response-language smoke remain follow-up
evidence, not an implementation gap.

### Phase 4: Renderable agent-facing playbooks for `agent.spawn`

No rendering happens inside `agent.spawn` (playbook-unaware, owner 18:56).
`agent.spawn` delivers `system_prompt_path`/`system_prompt` as an appended system
prompt and resolves tier server-side ((39), (44), (38)=d); that surface is owned
by `261117-feat-ws-agent-tools-mailbox-routing` (phase 1). The production paths
already exist:

- `agents-plugin/rsrc/explore/explore.md` is already `kind: render`, declares
  only the established `ExploreAgent`, `SpawnIdiom`, and `ContinueIdiom`
  variables, and has exact wsflow and Pi rsrc mirrors. Its current body is a
  caller-facing fallback brief (`explore.md#L9-L38`), and `delegates: true`
  makes rendering append another caller continuity tip
  (`playbook_tools.go#L514-L528`). It also lacks a frontmatter `tier`, so
  `playbook.render` omits the recommendation.
- `internal/wsagent/builtin.go#L194-L212` embeds
  `internal/wsagent/instructions/explore.md` and maps it to `ExploreProfile`;
  the builtin profile selects `builtin:explore` at `builtin.go#L290-L299`.
  `runner_process.go#L791-L825` already appends builtin/profile instructions
  and caller `system_prompt_path`/`system_prompt` at process start.
- `agent_tools_test.go#L653-L675` already pins one-time relative-path reading,
  cached delivery across restarts, and mutual exclusion with inline prompt;
  the same suite pins tier-rank and authority caps. `playbook.render` already
  returns path plus recommended tier/model/effort when a tier exists
  (`playbook_tools.go#L531-L565`).

This phase rewrites the existing `explore` body as instructions addressed to
the spawned child, removes `delegates: true` so rendering carries no caller
continuity tip, and adds the recommended bare tier `small` (Decisions 29-30).
The embedded one-line profile remains unchanged as the legacy profile path when a
caller/plugin has no renderable playbook. "Fallback" does not mean that a
failed `playbook.render` silently loads builtin text; the two paths remain
independent, which is the T0 behavior Decision 25 approved. The native-subagent
steer still comes from daemon-composed child instructions (Decision 24), not
this playbook. Depends on
`261117-feat-ws-agent-tools-mailbox-routing#phase1`.

Frozen file list (the resource regeneration is mandatory):

- Authoritative/mirrored rsrc: `agents-plugin/rsrc/explore/explore.md`,
  `agents-plugin/rsrc/manifest.json`,
  `agents-plugin-wsflow/rsrc/explore/explore.md`,
  `agents-plugin-wsflow/rsrc/manifest.json`,
  `agents-plugin-pi/rsrc/explore/explore.md`, and
  `agents-plugin-pi/rsrc/manifest.json`. All three manifests regenerate because
  the content hash changes; the two mirrors remain byte-identical to ws.
- Pi workflow bundles: `agents-plugin-pi/workflow-bundles/manifest.json`,
  `agents-plugin-pi/workflow-bundles/ws.zip`, and
  `agents-plugin-pi/workflow-bundles/wsflow.zip`.
- Native-Pi first-party copies and inventories:
  `agents-plugin/native-pi/first-party/workflow-bundles/manifest.json`,
  `agents-plugin/native-pi/first-party/workflow-bundles/ws.zip`,
  `agents-plugin/native-pi/first-party/workflow-bundles/wsflow.zip`,
  `agents-plugin/native-pi/first-party-manifest.json`,
  `agents-plugin-wsflow/native-pi/first-party/workflow-bundles/manifest.json`,
  `agents-plugin-wsflow/native-pi/first-party/workflow-bundles/ws.zip`,
  `agents-plugin-wsflow/native-pi/first-party/workflow-bundles/wsflow.zip`,
  and `agents-plugin-wsflow/native-pi/first-party-manifest.json`. These are the
  required outputs of `prepare-workflow-bundles.py` followed by
  `prepare-native-payload.py --first-party-only`, not optional release noise.
- Tests: `agents-plugin-tool/internal/mcp/playbook_render_surface_test.go`
  (shipped explore render returns path plus bare tier/model/effort, directly
  addresses the spawned child, and carries no caller continuity tip) and
  `agents-plugin-tool/internal/wsagent/agent_tools_test.go` (the rendered file's
  exact body reaches the child while tier/authority caps still apply).

`runner_conv.go`, `runner_process.go`, `children.go`, and `agent_tools.go` are
**not touched** by Phase 4; the delivery and cap code already exists. Only
`agent_tools_test.go` changes in that package. `builtin.go` and
`builtin_test.go` are also not touched: the existing builtin test already pins
the embedded fallback's evidence requirements. No config manifest regenerates;
only the three rsrc manifests and the listed Pi bundle/native inventories do.

Verification: remote w5090 tests prove the real shipped `explore` render returns
a path and the selected bare-tier recommendation plus config-resolved model and
effort; the rendered body addresses the spawned child and carries no
caller-facing continuity tip; spawning with that path and tier delivers its
exact rendered body as the appended system prompt, with tier-rank and authority
caps unchanged; the existing builtin-profile test still passes with the shipped
playbook hidden.
Run the ws/wsflow/Pi rsrc mirror and manifest drift guards and both Pi bundle
freshness checks after regeneration.

### Result (6ceff7e97) - 2026-11-21

Commits:

- `c5e6d6d7e`: exact wsflow/Pi resource mirrors, all three rsrc manifests,
  both Pi workflow bundles and native first-party inventories, plus the
  `agent.spawn` delivery/cap regression.
- `6ceff7e97`: child-facing `explore` body, `tier: small`, removal of
  `delegates: true`, and the shipped `playbook.render` surface regression.
- `a553e6df3`: Lite-review fixes that pin the real delegation-tip marker and
  deliver the real shipped explore resource under its selected small/read-only
  caps.

Design record:

- `explore` remains the existing render stem and declares only the established
  `ExploreAgent`, `SpawnIdiom`, and `ContinueIdiom` variables. Its body now
  addresses the spawned read-only worker directly, defines evidence and stop
  conditions, and contains no caller fallback or continuity instruction.
- `playbook.render` remains the only rendering owner and returns the existing
  path plus config-resolved `recommended-tier`, `recommended-model`, and
  `recommended-reasoning-effort` lines. `agent.spawn` remains playbook-unaware:
  the test passes the rendered file as `system_prompt_path` and asserts its
  exact body precedes the daemon-composed key and parent-mailbox suffix.
- The embedded builtin explore profile and all production spawn, delivery,
  authority, and tier-cap code remain unchanged. The real shipped-resource
  regression proves nested workdir-write and medium requests are refused from
  the small/read-only child.
- The authoritative ws resource, wsflow mirror, and Pi mirror are byte-identical.
  The mandatory workflow-bundle and native first-party generators refreshed
  only the frozen Phase 4 output list.

Compatibility:

- T0 in both directions, as Decision 25 settled. This changes only a resource
  body and adds an existing frontmatter tier; the variables and
  `playbook.render` response fields already exist, resource names have no
  allowlist, no MCP argument/protocol/store/manifest shape changes, and the
  unchanged embedded builtin profile remains the fallback for an older plugin
  that lacks the resource.

Verification:

- `python3 agents-plugin-pi/scripts/prepare-workflow-bundles.py` and
  `python3 agents-plugin-pi/scripts/prepare-native-payload.py
  --first-party-only` passed; the first command's `--check` form passed both
  before and after native refresh. A second native refresh left the worktree
  clean.
- `GOCACHE=/private/tmp/ws-p4-go-cache GOPROXY=off GOSUMDB=off
  GOTOOLCHAIN=local go test ./internal/wsagent -run
  'TestAgent(SystemPromptReachesEveryStart|RenderedPlaybookPromptReachesChildAndKeepsCaps|SpawnCapsAuthorityAndTier)'
  -count=1 -v` passed (0.902 s).
- `GOCACHE=/private/tmp/ws-p4-go-cache GOPROXY=off GOSUMDB=off
  GOTOOLCHAIN=local go test ./internal/mcp -run
  'TestRenderShippedExploreForAgentSpawn|TestRenderReturnsFrontmatterRecommendedTier|TestWithRecommendedRenderBinding'
  -count=1 -timeout 45m -v` passed (0.908 s).
- `GOCACHE=/private/tmp/ws-p4-go-cache GOPROXY=off GOSUMDB=off
  GOTOOLCHAIN=local go test ./internal/wsrsrc -run
  'Test(WsflowRsrcMirrorUpToDate|PiMirrorUpToDate|ShippedManifestUpToDate)'
  -count=1 -v` passed (0.277 s), and `TestPiBundleCommittedIsFresh`
  passed (0.546 s). The parent lead independently ran the whole
  `internal/wsrsrc` package and the pre-review focused wsagent/MCP commands;
  all passed.
- The first worker Go invocation reached no compiler or test because the
  sandbox refused the default macOS Go cache. Setting the isolated `GOCACHE`
  above resolved the environment failure; no implementation/test failure
  repeated.
- The required one-pass fresh-reader audit found four Important and five Minor
  wording issues; `6ceff7e97` fixed all of them before implementation review.
  Lite review found two Important test gaps; `a553e6df3` fixed both. Per Lite
  policy there was no second review.

Dogfood surprises:

- `route.resolve_implement` proposed renaming this Phase 4 branch to the deleted,
  already-merged Phase 3 branch `impl/develop/prize-pupil-anger`; the parent
  vouched `impl/develop/prompt-assembly-p4` and identified the known stale-branch
  resolution gap tracked by `261121-bug-ws-dogfood-context-routing-gaps`.
- `workflow_manual` and `config.resolve_agent` were absent from this worker's
  exposed ws tool surface. The parent confirmed the manual gap is tracked by
  `261121-bug-ws-dogfood-context-routing-gaps`; this run used `playbook.read`,
  `playbook.render`'s resolved recommendation, and `agent.tiers` without
  restarting or swapping the daemon.
- The generation leaf's first ad hoc manifest write emitted a literal `\\n`;
  it corrected the serialization immediately, then all manifests parsed and
  passed exact hash/mirror validation before bundle generation. No malformed
  artifact was committed.

Not attributed to Phase 4: the optional
`workflow-bundle-consumer.test.mjs` check fails on both this branch and
untouched `develop` because its immutable older consumer rejects the current
Claude launcher/config reference set; the first macOS run also exposed the
`/var` to `/private/var` identity difference. The mandatory generators,
inventory checks, native bundle freshness test, and Go drift guards above all
pass.
