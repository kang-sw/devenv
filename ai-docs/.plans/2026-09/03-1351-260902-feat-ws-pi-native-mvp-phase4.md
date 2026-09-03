# Plan: 260902-feat-ws-pi-native-mvp — Phase 4: Proof-of-concept command

## Relevant Ticket Contract

- Phase 4 gate (authority): register ONE PoC command (e.g. `/ws-discuss`) via
  `pi.registerCommand` that (a) loads the ws skill, (b) calls a ws-mcp tool
  through the bridge, and (c) drives one spawn round-trip — end-to-end on Pi,
  proving **skills-load + bridge + spawner compose**. This validates the MVP
  before feature expansion.
- Depends on Phases 1–3, all landed: the bridge re-registers every ws-mcp tool
  as `ws__*` (Phase 1 Result `0d47b71f`); the spawner registers
  `ws-agent-spawn` / `ws-agent-continue` / `ws-agent-wait` / `explore` (Phase 2
  Result `7fcce4e3`); the model catalog + `ws-model-catalog-list` command
  (Phase 3 Result `2daacf6d`) is the working `pi.registerCommand` precedent.
- Golden rule (Constraints): ws-mcp Go source (`agents-plugin-tool/`) and the
  other package roots are NEVER modified. All Phase 4 work is confined to
  `agents-plugin-pi/`. Verified untouched in every prior phase.
- AI-authored content is English; human-facing UI strings exempt.

## Out of Scope

- Durable depth-2 recursion, always-visible TODO, goal-loop, compaction hooks
  (Phase 4 body explicitly defers these to follow-up tickets under the epic).
- Any second command or expanding `ws-model-catalog-list`; Phase 4 asks for
  ONE PoC command only.
- Modifying the bridged tool set, the spawner engine, the model catalog, or the
  hand-synced `bin/`/`runtime.json`/`rsrc/` copies — Phase 4 composes the
  existing surfaces, it does not add to them.
- The `AGENTS.md` `## Project Orientation` `agents-plugin-pi/` root entry
  (deferred by every prior phase "until the ticket integrates off this tracking
  branch"); the dedicated Pi-adapter mental-model domain (Phase 3 Result defers
  it to "once Phase 4 closes the MVP" — a lead/doc step, not implementer code).

## Codebase Findings

- `agents-plugin/skills/lead-discuss/SKILL.md#L1-11` — the discuss skill body
  itself instructs the model to call `ws/playbook.print(name:"lead-discuss", …)`
  **and** `ws/workflow_manual(…)` in parallel. Both are bridged ws-mcp tools
  (registered `ws__playbook_print` / `ws__workflow_manual`). So loading this
  skill and letting the model run it satisfies gate (a) skills-load AND gate (b)
  "calls a ws-mcp tool through the bridge" without any imperative tool call in
  the handler. The skill's `/ws:mcp-server-repair` reference is prose the model
  maps as usual.
- `.../pi-coding-agent/docs/skills.md#L76-83` — `resources_discover` skill paths
  register as `/skill:<dirname>` commands; args after the command are appended
  to skill content as `User: <args>`. ws skill dirs are hyphen-form
  (`lead-discuss`), matching Pi's skill-name charset (spec
  `pi-adapter-runtime` `{#260903-pi-bridge-skill-exposure}`), so
  `/skill:lead-discuss <topic>` loads the discuss skill with `<topic>` as the
  user prompt. Confirmed exposed via `index.ts#L61-63`
  (`resources_discover → skillPaths:[skillsDir]`).
- `.../docs/extensions.md#L1439-1467` — `pi.sendUserMessage(content, { expandPromptTemplates: true })`
  is THE documented way a command triggers model work while expanding
  `/skill:` and prompt templates (`#L1458` shows `/review …` expansion). When
  the agent is idle it sends immediately and triggers a turn; while streaming it
  requires `deliverAs`. This is how `/ws-discuss` "loads the ws skill" and
  drives the composed run — the command triggers model work, it does not run the
  workflow imperatively.
- `.../docs/extensions.md#L1109-1136, #L96-99` — command handler signature is
  `handler: async (args: string, ctx: ExtensionCommandContext) => …`. `ctx`
  carries `ui.notify`, `isIdle()`, `sessionManager`, and session-control
  methods; `pi.sendUserMessage` lives on the `ExtensionAPI` closure (not `ctx`).
  Registration API: `pi.registerCommand(name, { description, handler,
  getArgumentCompletions? })` (`#L1525-1557`).
- `examples/extensions/send-user-message.ts#L18-35` (installed Pi build) —
  canonical pattern: guard `if (!ctx.isIdle()) { ctx.ui.notify("busy…"); return; }`
  then `pi.sendUserMessage(args)`. Direct precedent for a skill/prompt-injecting
  command.
- `agents-plugin-pi/src/index.ts#L69-77` — the working `pi.registerCommand`
  precedent (`ws-model-catalog-list`): registered at the top level of the
  extension factory (NOT inside `session_start`), imperative handler using
  `ctx`. Phase 4 adds a sibling `pi.registerCommand("ws-discuss", …)` here.
- `agents-plugin-pi/src/spawner.ts#L737-846` — the four delegation tools
  (`ws-agent-spawn`, `ws-agent-wait`, `explore`, `ws-agent-continue`) are
  registered Pi tools the model calls; `explore({query})` is the cheapest
  one-shot spawn round-trip (recon leaf, `--no-session`, self-reaping). Gate (c)
  "one spawn round-trip" is met by the model issuing `explore` (or
  `ws-agent-spawn`+`ws-agent-wait`) during the run.
- `agents-plugin-pi/src/index.ts#L79-98` — bridge + spawner are wired in
  `session_start` (`handle`, `agentTools`); by the time a user types
  `/ws-discuss` mid-session both are live, so the model-facing `ws__*` and
  `ws-agent-*` tools are already callable. No readiness guard needed beyond the
  idle check.
- **Risk signal (design judgment, not a blocker):** the `lead-discuss` skill by
  itself drives the bridge calls (gate a+b) but does NOT inherently spawn, so a
  purely skill-driven kickoff would not reliably satisfy gate (c). The PoC
  kickoff must therefore ALSO instruct the model to dispatch one spawn round-trip
  (e.g. one `explore` leaf, harvested via `ws-agent-wait` / the sync path) so all
  three gate actions are demonstrable in one command. This makes the gate
  model-driven — the same live-gate style Phases 2-3 used successfully
  (`pi -e … --mode json -p "<orchestration>"`, model reliably issued the tools).
  Note this for the reviewer: the gate proof is a live model run, not a unit
  assertion.
- `agents-plugin-pi/test/` — no `index.test.ts` / command test exists; the
  `ws-model-catalog-list` command ships without a dedicated unit test (its
  handler is thin `ctx`/`pi` glue). Phase 4's `ws-discuss` handler is likewise
  thin (guard + one `pi.sendUserMessage`), so its real verification is the live
  end-to-end gate, matching the Phase 2-3 precedent — not a new unit test.

## Implementation Plan

1. In `agents-plugin-pi/src/index.ts`, add a second top-level
   `pi.registerCommand("ws-discuss", { description, handler })` immediately after
   the existing `ws-model-catalog-list` registration (`#L69-77`) — same
   placement (top-level factory body, not inside `session_start`), same
   `(args, ctx)` handler shape.
2. Handler body (mirror `examples/extensions/send-user-message.ts` +
   `ws-model-catalog-list`):
   - If `!ctx.isIdle()`, `ctx.ui.notify("Agent is busy — try again when idle.", "warning")` and return.
   - Build a kickoff string that (a) loads the ws discuss skill and (b) makes
     the spawn round-trip explicit, e.g.
     `` `/skill:lead-discuss ${args || "<default PoC topic>"}\n\nAfter loading the discuss procedure, also dispatch one \`explore\` recon leaf to <scoped question> and report its result, to prove the bridge + spawner compose.` ``.
     (The whole text after `/skill:lead-discuss ` becomes the skill's `User:` args, per skills.md#L83 — skill loads AND the spawn instruction rides along.)
   - `pi.sendUserMessage(kickoff, { expandPromptTemplates: true })` (idle path
     sends immediately and triggers the turn; no `deliverAs` needed since the
     handler only proceeds when idle).
   - Update the module doc-comment header (`index.ts#L20-27`) to move
     `/ws-discuss` from "out of scope" to the Phase 4 shipped surface.
3. Doc reconciliation (lead/doc step, flag it — Phase 2/3 landed spec sections
   in a separate lead-owned commit, not the implementer commit): update spec
   `ai-docs/spec/pi-adapter-runtime.md` — add a `/ws-discuss` PoC-command section
   and flip the closing Constraints note (`#L225-228`) which currently says
   `/ws-discuss` is "not-yet-implemented … not part of this contract yet."

## Verification Plan

- `cd agents-plugin-pi && npm test` (`node --test`) — must stay green (85 tests
  from Phase 3); Phase 4 adds thin glue, expected no regressions. Optional: a
  small unit test asserting the kickoff-string builder (if the kickoff is
  factored into an exported pure helper) — otherwise no new unit test, matching
  the `ws-model-catalog-list` precedent.
- Live end-to-end gate (the load-bearing proof, Phase 2-3 style): run
  `pi -e agents-plugin-pi/src/index.ts` interactively (or drive the command's
  effect via `--mode json -p`) and invoke `/ws-discuss <topic>`; confirm in one
  run: the discuss skill loads, the model calls a bridged `ws__*` tool
  (`ws__playbook_print` / `ws__workflow_manual`), and one spawn round-trip
  completes (`explore`/`ws-agent-spawn` → harvested). Capture the transcript as
  gate evidence.
- Golden-rule check: `git status` / `git diff --stat` show changes only under
  `agents-plugin-pi/` (and the spec doc); `agents-plugin/`, `agents-plugin-tool/`,
  `agents-plugin-wsflow/`, and the hand-synced
  `agents-plugin-pi/{bin,runtime.json,rsrc}` copies untouched.

## Escalations

- None. The mechanism is fully documented (skill `/skill:` expansion +
  `pi.sendUserMessage({expandPromptTemplates:true})`) and the discuss skill
  already carries the bridge calls; the one design decision (kickoff must add an
  explicit spawn instruction so gate (c) is deterministic) is a small, safe PoC
  choice, not a strategy/contract conflict. Confidence: high. The reviewer
  should accept a model-driven live gate (consistent with Phases 2-3) as the
  spawn-round-trip proof rather than expecting a unit assertion.
