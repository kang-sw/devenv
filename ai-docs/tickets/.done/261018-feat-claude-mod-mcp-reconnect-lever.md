---
title: Model-invoked MCP reconnect lever in the Claude mod
related:
  261018-research-claude-mod-ws-mcp-reconnect: source research; its Verified Findings are this ticket's evidence, and its Proposals became the Decisions below through the user's 2026-10-18 approval
  261017-feat-claude-mod-lead-compaction-timing: the single Claude mod this lever joins, and the request-then-act-at-turn-end lever shape it copies
  261017-feat-claude-mailbox-active-push-mod: first Claude mod; main-session-only tool refusal and server resolution precedent
  261018-feat-claude-mod-ws-manual-injection: sibling feature in the same mod; runs after this ticket (both edit claude-mod/register.ts)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0fa63f5bd3ef1b3d
sage-review-completeness-reviewed: 0fa63f5bd3ef1b3d
completed: 2026-10-18
---

# Model-invoked MCP reconnect lever in the Claude mod

## Background

On Windows the ws stdio MCP server sometimes drops, and Claude Code does not
bring a dead plugin stdio server back in an interactive session. Today the
lead falls back to `ws:mcp-server-repair`, which relays manual `/mcp` steps to
the user and keeps working through `ws-cli`.

The research `261018-research-claude-mod-ws-mcp-reconnect` verified on macOS
with Claude Code 2.1.302 (probe runs cited there):

- In an interactive session a killed plugin stdio server is not restarted: its
  tools leave `$.tool.list` within about 1.5 s and no process starts.
- `$.mcp.connect` never restarts a dead server.
- `$.command.run({ command: "mcp", args: "reconnect <server>" })` from
  `turn.complete` or a timer restores the server (new PID, tools back, the
  model's next call succeeds), including a copy of the real ws plugin. From a
  hook the turn waits on (`tool.call`, `prompt.submit`) the host refuses it,
  and a `tool.call` hook does not fire for a tool that has left the list.
- Headless sessions restart a killed server lazily; a failed restart sticks
  for the session, and `/mcp reconnect` is unavailable headless.

## Decisions

- **A model-invoked lever, not a watchdog.** The single Claude mod
  (`agents-plugin/hooks/claude-mod/`) registers a tool the model calls when
  the ws MCP server is gone; the mod reconnects at the next main-thread turn
  end and submits a continuation. Rejected: a `tool.call` hook that catches the
  transport error and retries in place (the hook does not fire once the tool
  has left the list, and the host refuses `/mcp reconnect` from it);
  `$.mcp.connect` as the reconnect (never restarts a dead server). A watchdog
  (`$.clock.every` counting the server's tools) is out of scope (see
  Constraints). (user, 2026-10-18)
- **Shipped in both packages.** Unlike compaction timing, the lever registers
  in `ws` and `wsflow`; package identity comes from `$.plugin.name` at runtime
  as the mod already does, and the wsflow `claude-mod/` copy stays
  byte-identical. (user, 2026-10-18)
- **The reconnect runs at turn end.** At the next main-thread `turn.complete`
  after a request, the mod schedules the reconnect step to run after the hook
  has returned (as the compaction lever schedules its compact step through
  `after`): the step runs `/mcp reconnect <server>` through `$.command.run`,
  then the tool-list wait, then the continuation, none of it inside the hook's
  time bound (the probe saw an awaited `$.command.run` take 17 s to settle
  while a turn ran). Rejected: running it from the tool call (host refuses it
  from a hook the turn waits on). (user, 2026-10-18)
- **Server name resolved at runtime, never spelled.** The server argument is
  the `server` that `$.mcp.connect($.plugin.name)` answers, resolved while the
  server is up and cached, the way the mailbox feature resolves its server
  (`useServer`). Rejected: hard-coding `plugin:ws:ws` (wrong for wsflow and
  for a renamed install). (user, 2026-10-18)
- **Success is judged by the tool list.** After the reconnect command
  settles, the mod checks `$.tool.list` for the server's tools (the
  `mcpToolName(server, ...)` prefix) and the continuation states which outcome
  happened. Rejected: trusting the `$.command.run` result alone (whether it
  separates success from failure is unverified). (user, 2026-10-18)
- **Failure and headless share one path.** When the tools do not come back,
  including headless where `/mcp reconnect` is unavailable, the continuation
  reports the failure and points the lead at `mcp-server-repair`'s manual
  reconnect steps and its CLI fallback. Rejected: detecting headless
  separately (another branch for the same outcome). (user, 2026-10-18)
- **`mcp-server-repair` gains a Claude branch.** In both packages' copies of
  the skill: when the harness lists the reconnect request tool, the lead calls
  it first; the Reconnect Steps are relayed to the user only when the tool
  reports failure or is absent. Failure arrives in the failure continuation
  (the tool's own result only says the request was recorded), so the skill
  says: after a failure continuation for this drop, go straight to the
  Reconnect Steps and the CLI fallback without calling the tool again. The
  invariant "The agent cannot re-enable the
  MCP server itself" is reworded so it no longer claims this where the tool
  exists. The CLI fallback steps are unchanged. (user, 2026-10-18)

- **The tool and its prose.** The tool is `request_mcp_reconnect`, taking a
  required one-line `{ continuation: string }` submitted after the reconnect
  attempt; its description, result text, and both continuation prompts are
  pinned below and asserted verbatim. Rejected: a no-argument tool (the lead's
  interrupted next step would not survive the turn end in its own words).
  (user, 2026-10-18; compaction lever precedent)
- **Lever rules copy the compaction lever.** A call carrying `agentId` is
  refused (main session only); an empty or multi-line `continuation` is a tool
  error; a repeat call while a request is pending replaces its continuation
  (latest wins); a pending request survives an `aborted` or subagent turn end
  and acts at the next non-aborted main-thread turn end (`checkLeverCall` in
  `timing.ts`, `lever` / `turnComplete` in `compaction.ts`). (user, 2026-10-18)
- **Always reconnect when a request is pending**, even if the server's tools
  are listed. Rejected: skipping when they are listed (a listed-but-failing
  server, the headless failure shape, is exactly the case a skip misses).
  (user, 2026-10-18)
- **Bounded wait on the tool list.** After `$.command.run` settles, the mod
  polls `$.tool.list` for about 10 s for the server's tools. Rejected: one
  check right after the command (can race the tool list update). (user,
  2026-10-18)
- **The server is resolved at `session.start`** (and again after a successful
  reconnect) and cached in one place that other features of the mod reuse
  (261018-feat-claude-mod-ws-manual-injection needs the same name); a request made before any resolution reports failure
  without reconnecting. The mailbox feature resolves only at bind and restore;
  resolving at start is required here because a dead server cannot be resolved
  later (`$.mcp.connect` reports `failed`). (user, 2026-10-18)
- **Independent of the compaction lever.** A compaction whose `compact()` is
  rejected because the reconnect's continuation turn started first retries at
  the next turn end under its existing rule. Rejected: a combined scheduler (new
  coupling the existing retry already makes unnecessary). (user, 2026-10-18)
- **Session end clears a pending request** in its own isolated step, before
  the awaited `$.store` delete (the 1ed6398be ordering), so a request never
  carries into the next conversation after `/clear` or resume. (user,
  2026-10-18)
- **Skill invariant wording.** The skill's first invariant ("This skill makes
  no MCP call") is reworded to say it makes no call to the dropped server's
  tools, since the reconnect tool is served by the plugin itself. (user,
  2026-10-18)

## Constraints

- Out of scope: a watchdog that reconnects without the model; handling of
  Claude Code's 15-minute MCP failure cache; Windows-specific diagnosis of
  why ws-mcp drops. Windows behavior is checked by a post-merge dogfood on the
  Windows smoke host, not as a merge gate. (user, 2026-10-18)
- Stop condition: the lever's premise is that a mod-registered tool (served by
  the mod, listed as `mcp__<plugin>__*` apart from `mcp__plugin_<plugin>_<server>__*`)
  stays callable after the ws MCP server dies; the research inferred this from
  the naming and did not probe it. If Phase 1's live check shows the tool
  leaves the list or fails with the server, stop and report before building
  further. (user, 2026-10-18)
- The 261017 compaction and mailbox Decisions stay in force for the existing
  features of the mod.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/skills/, agents-plugin-wsflow/skills/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)

## Pinned Prose

Asserted verbatim by tests. `{plugin}` is `$.plugin.name`, `{continuation}`
the tool argument, `{reason}` a short failure reason the implementer words per
case (no resolved server, the command's error, the tools not back within
10 s); tests assert the frame with the reason the code produces.

Tool description:

```text
Reconnect the {plugin} MCP server when the current turn ends. Use it when that server's tools have left your tool list or a call reports the server not connected. `continuation` is one line naming what to do next; it is sent as your next prompt after the reconnect attempt, with the outcome. Main session only.
```

Tool result:

```text
Reconnect requested. End your turn now without further tool calls; the reconnect runs when the turn ends, and your continuation resumes the work.
```

Continuation after success:

```text
[{plugin} mcp] The {plugin} MCP server reconnected; its tools are available again.
Continue: {continuation}
```

Continuation after failure:

```text
[{plugin} mcp] The {plugin} MCP server did not come back ({reason}). Follow the {plugin}:mcp-server-repair skill: keep working through its CLI fallback and relay its reconnect steps to the user.
Continue: {continuation}
```

## Prior Decisions

- 261017-feat-claude-mod-lead-compaction-timing (2026-10-18, Result): "A repeat lever call replaces the continuation and keeps the compact-step state; a lost continuation submit is recorded to the transcript (the only holder of that text)" — bearing: supports
- 2a4cfcf5 (2026-10-18, commit): "the declarations say $.session.compact runs through every hook but the calling one; a new engine test confirmed the feature's session.compact hook does not see the lever's own call" — bearing: constrains
- 1ed6398b (2026-10-18, commit): "if the session.end time bound cut the awaited delete short, compaction.end never ran and a pending request or latch carried into the next conversation after /clear or resume" — bearing: constrains
- 261017-feat-claude-mailbox-active-push-mod (2026-10-17, Decisions): "The mod resolves its server through `$.mcp.connect(<own plugin name>)`; the ws and wsflow server names equal their plugin names. No server name is hardcoded." — bearing: supports
- 261017-feat-claude-mod-lead-compaction-timing (2026-10-17, Decisions): "No `$.session.version()` gate. A broken mod API leaves no lever and native auto compaction untouched" — bearing: constrains
- 75c8601c (2026-10-18, commit): "a lever request compacted after turn end and delivered exactly one landed-form continuation" — bearing: supports
- adbf5ec3 (2026-07-25, commit): "Skill is deliberately NOT a lead-*/playbook.print shim: it must be readable with the MCP server down, so a playbook.print body would die exactly when the skill is needed." — bearing: constrains
- adbf5ec3 (2026-07-25, commit): "Reconnect steps written functionally ... rather than hardcoding plugin:ws-plugin:ws: that literal identifier does not survive ws:/ws//ws-cli substitution and would leave a wrong string in the wsflow mirror." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/register.ts plus a new lever module and tests, the byte-identical agents-plugin-wsflow/hooks/claude-mod/ copy, agents-plugin/skills/mcp-server-repair/SKILL.md, agents-plugin-wsflow/skills/mcp-server-repair/SKILL.md (generated), agents-plugin/skills/manifest.json hash |
| scope.surface | public-interface | new model-callable request_mcp_reconnect tool with pinned model-facing prose, and reworded mcp-server-repair invariants in both packages |
| scope.new_public_symbol | yes | request_mcp_reconnect (mod-registered tool); not yet present in the tree (search of agents-plugin/hooks returned nothing) |
| scope.new_type_contract | yes | tool input with a required one-line continuation string, mirroring checkLeverCall in agents-plugin/hooks/claude-mod/timing.ts |
| scope.test_surface | unknown | existing engine-backed tests are agents-plugin/hooks/claude-mod/engine.test.ts and compaction-engine.test.ts; the ticket does not say whether the new tests extend them or add files |
| complexity.reuse_points | confirmed | lever shape in agents-plugin/hooks/claude-mod/compaction.ts#L198-L256, checkLeverCall in timing.ts, useServer and mcpToolName in register.ts#L55-L58 and L250-L253, byte-identical mirror test in agents-plugin-tool/internal/wsrsrc/wsflow_hooks_mirror_test.go |
| complexity.side_effect_risk | moderate | queues a host /mcp reconnect command and submits continuation prompts at turn end from the shared session lifecycle; $.command.run and $.tool.list are used nowhere in the current mod (search of agents-plugin/hooks returned nothing) |
| risk.correctness | high | the lever's premise (a mod-registered tool stays callable after the server dies) is unprobed and carries a stop condition, and success depends on host behavior seen only in a macOS probe |
| risk.fit | moderate | follows the compaction lever shape, but ships in both packages unlike the ws-only compaction gate and edits a generated wsflow skill copy plus manifest hash |
| risk.test | moderate | engine kit coverage of $.command.run and tool-list changes is unconfirmed, and the decisive checks are a live interactive kill-and-reconnect run |
| risk.security_or_contract | moderate | model-facing pinned prose is asserted verbatim, a subagent-call refusal is required, and the mod runs a host slash command on the model's request |

## Phases

### Phase 1: Reconnect lever and repair-skill branch

Add the lever to `claude-mod/` (registration in the `session.start` setup
alongside the existing features, isolated the same way: a throw in its setup
never skips another feature's; extend the existing `session.start` / `session.end` / `turn.complete` registrations rather than adding second unmatched ones, which the host refuses, and bind engine edges as closures, since the validator forbids storing `$`), its `tool.call` and `turn.complete` handling,
and the pinned prose; resync the wsflow copy byte-identically. Reword, in the same landing, the texts that list the mod's features: the ws
`hooks.json` description, the `register.ts` header comment, and the Claude Mod
paragraph of `ai-docs/manuals/wsflow-mirroring.md` (as 261017 did when it
added compaction). Update both
packages' `mcp-server-repair` skills per the Decisions, following
`skill-authoring.md`
(the wsflow copy is generated, not hand-written: edit only the ws source, which
must stay free of the substitution guard's disqualifying tokens such as `ws.`,
then regenerate the wsflow copy with `WS_REGEN_WSFLOW_SKILLS=1` and the ws
`skills/manifest.json` hash with `WSRSRC_REGEN_SKILLS=1`;
`TestWsflowSkillsMirrorUpToDate` guards the copy -
agents-plugin-tool/internal/wsrsrc/skills_mirror.go#L36-L74,
ai-docs/manuals/wsflow-mirroring.md#L175-L198).

Verification:

- `claude plugin validate` and `claude plugin test` pass for both
  `agents-plugin` and `agents-plugin-wsflow`, with engine tests for: the tool
  registered in both packages with the pinned description; refusal of a
  subagent call; empty or multi-line continuation as a tool error; repeat call
  replacing the continuation; no reconnect at an aborted or subagent turn end;
  reconnect queued at the next main-thread turn end with the resolved server;
  success and failure continuations verbatim, success judged from the tool
  list; a request with no resolved server reporting failure; the step running
  after the turn-end hook returned; the lever's setup independent of the other
  features' setups; and the existing mailbox and compaction tests still
  passing, with the two `compaction-engine.test.ts` isolation tests that pin
  the exact registered-tool list (`['request_compaction']`,
  `['mailbox_watch']`) narrowed to their own feature's tool. New tests go in
  `reconnect.test.ts` (pure core) and `reconnect-engine.test.ts` beside the
  compaction pair, one source to one test file.
- `python3 -m unittest discover` passes for `agents-plugin/tests` and
  `agents-plugin-wsflow/tests` (skill-shim and manifest drift tests), and
  `go test -race ./internal/wsrsrc/ -run TestWsflowHooksMirrorUpToDate` passes
  under `agents-plugin-tool/`.
- Live check in an interactive session (tmux, as the research probe ran):
  kill the ws-mcp process, confirm `request_mcp_reconnect` is still listed and
  callable (the stop condition above), call it, and observe the reconnect,
  the server's tools back in the list, and exactly one success continuation.
  A second run with the reconnect made to fail (for example the launcher
  unavailable) shows the failure continuation.

### Result (2615834e6) - 2026-10-18

Landed `agents-plugin/hooks/claude-mod/reconnect.ts` (pure core and the lever
state machine) with `reconnect.test.ts` and `reconnect-engine.test.ts`, wired
into `register.ts` as a third isolated feature: its own `session.start` setup,
a matched `tool.call` hook, and isolated steps in the shared `turn.complete`
and `session.end` hooks (the reconnect reset runs before the awaited `$.store`
delete). The step runs after the turn-end hook through `$.clock.after`,
issues `/mcp reconnect <server>` via `$.command.run`, polls `$.tool.list` for
the server's prefix every 500 ms for up to 10 s, re-resolves the server on
success, and submits the pinned success or failure continuation. The server
name is resolved at `session.start` into one cache (`server` beside
`useServer`) shared with the mailbox feature. Resynced the byte-identical
wsflow copy; reworded the ws `hooks.json` description, the `register.ts`
header, and the Claude Mod paragraph of `wsflow-mirroring.md`; added the
reconnect-tool branch and the two reworded invariants to `mcp-server-repair`
(wsflow copy regenerated, skills manifest hash regenerated). The two
compaction-engine isolation tests now filter to their own feature's tool.

Decisions taken:

- A `/mcp reconnect` answer whose text does not say "reconnected" is a
  failure with the command's words as the reason, checked before the
  tool-list poll. Headless answers "Reconnect, enable, and disable aren't
  available in this session." while the tools stay listed (research probe run
  6), so the tool list alone would misreport success there. The tool list
  remains the success judge; this only adds a failure signal.
- The step takes the request as it starts: a call made during a slow
  reconnect is a new request for a later turn end. A session end during the
  step drops its continuation (generation counter).
- The mailbox reload restore awaits the shared `session.start` resolution
  instead of calling `$.mcp.connect` again.

Verification:

- `claude plugin validate` passes and `claude plugin test` reports 147 pass,
  0 fail for both `agents-plugin` and `agents-plugin-wsflow`.
- `python3 -m unittest discover` passes for `agents-plugin/tests` (78) and
  `agents-plugin-wsflow/tests` (15); `go test -race ./internal/wsrsrc/`
  passes (hooks and skills mirror guards, skills manifest drift).
- Live check (tmux, Claude Code 2.1.302, macOS, scratch copy of
  `agents-plugin` via `--plugin-dir`): after `kill -9` of ws-mcp,
  `mcp__plugin_ws_ws__config_list` left the tool list while
  `mcp__ws__request_mcp_reconnect` stayed listed and callable (stop condition
  cleared). Calling it led to "Successfully reconnected to plugin:ws:ws", a
  new ws-mcp PID, exactly one success continuation, and a successful
  `config_list` call. With the launcher renamed, the transcript showed "Failed
  to reconnect to plugin:ws:ws" and the failure continuation (tools not back
  within 10 s) arrived.
- Lite review: no Critical or Important findings; the unused engine-test
  answer variant was removed (5e0db2815).
