# Plan: Implement the ws Pi-native MVP — Phase 1: Self-built MCP stdio bridge extension

## Relevant Ticket Contract

- Build a Pi extension in a **new `agents-plugin-pi/` sibling root** (this
  ticket is the authorizing ticket for that root per AGENTS.md) that spawns
  the ws-mcp launcher as a subprocess, speaks a minimal self-owned
  JSON-RPC-over-stdio MCP client, and re-registers every ws-mcp tool via
  `pi.registerTool` under its **verbatim `ws/...` name** so existing
  `SKILL.md` prose (which already writes literal `ws/playbook.print(...)`
  calls) works with zero rewriting.
- Own subprocess lifecycle: spawn on load/`session_start`, close on
  `session_shutdown`.
- Expose `agents-plugin/skills/` through `resources_discover`.
- Prefer the minimal self-owned stdio client over `@modelcontextprotocol/sdk`
  to keep the dependency surface empty.
- **`session_key` stays optional and caller-controllable on every keyed
  tool** — never stripped. The bridge default-fills its own startup
  auto-login key only when a call omits `session_key`, and forwards an
  explicit `session_key` verbatim (subagent lineage / lead multi-track
  orchestration both depend on this — out of scope to build the
  subagent/lead-track machinery itself in this phase, but the wiring must not
  break it later). `ferrule` and other key-management tools stay exposed,
  unmodified.
- **Golden rule**: ws-mcp Go source is never modified; dependency is
  one-directional (adapter → ws-mcp). All new behavior lives inside
  `agents-plugin-pi/`.
- The adapter validates the ws-mcp `runtime.json` plugin version on startup
  and **fails loudly** on mismatch (pin-and-fail).
- **Gate**: a keyed ws-mcp tool (e.g. `ws/workflow_manual`) round-trips
  end-to-end through the bridge against a live model — `session_key`
  default-filled when omitted, forwarded verbatim when supplied.

## Out of Scope

- `ws-agent-spawn/continue/wait` and `explore` (Phase 2 — the async
  subprocess-spawner/registry, tool-group table, `--tools`/`--model`
  curation).
- Model catalog curation + tier map + bootstrap warning (Phase 3).
- The `/ws-discuss` PoC command (Phase 4).
- Depth-bounded recursion / dual-defense tool hiding (all Phase 2+; nothing
  in Phase 1 spawns a `pi` subprocess other than the ws-mcp launcher itself).
- Distribution packaging (`pi install git:...`) — Phase 1's gate is local dev
  loop only (`pi -e ./agents-plugin-pi/src/index.ts` or
  `.pi/extensions/`), not the git-package installability chain from the
  anchor's "Resolved this session" section.
- Any change to `agents-plugin/`, `agents-plugin-tool/`, or
  `agents-plugin-wsflow/` beyond reading them as reference/copy sources.

## Codebase Findings

- `agents-plugin/.mcp.json` (whole file) — canonical launcher invocation used
  by the existing Claude/Codex plugin: `command: "python3"`,
  `args: ["./bin/ws-mcp-launcher.py", "serve", "--stdio"]`, `cwd: "."`,
  `startup_timeout_sec: 30`, `tool_timeout_sec: 600`. The bridge's spawn call
  should mirror this shape (spawn `python3 <launcher> serve --stdio` with
  `cwd` set to the launcher's own plugin directory).
- `agents-plugin/bin/ws-mcp-launcher.py#L835-L900` (`main()`) — the launcher
  resolves/installs a compatible runtime binary, then **execs it**
  (`os.execvpe`, L900) passing through `sys.argv[1:]` and inheriting stdio
  (Windows falls back to `subprocess.call`, L896). All diagnostics go through
  `note()`/`fail()` (L67-75), which write to **stderr only** — confirms
  "stdout before exec corrupts JSON-RPC" is already an enforced invariant of
  the launcher, not something the bridge needs to defend against separately.
- `agents-plugin/bin/ws-mcp-launcher.py#L219-L256` (`tools_compatible`) — the
  launcher's own internal compatibility probe shows the reference JSON-RPC
  handshake: an `initialize` line followed by a `tools/list` line, each a
  single JSON object terminated by `\n`, sent as one batched stdin write.
- **Live probe (this session, read-only, via a throwaway `git init` tmp
  root)**: `printf '<initialize>\n<tools/list>\n' | python3
  agents-plugin/bin/ws-mcp-launcher.py serve --stdio --root <tmp>` confirms:
  - Wire format is **newline-delimited JSON-RPC, one object per line — no
    Content-Length header framing.** This is the exact shape the self-built
    stdio client must parse/write.
  - `tools/list` returns 60 tools with **bare dotted names and no `ws/`
    prefix** — `runtime.info`, `ferrule`, `playbook.print`,
    `workflow_manual`, `tickets.list`, etc.
  - `tools/call` result shape is `{"content":[{"type":"text","text":"..."}]}`
    — this maps almost 1:1 onto `pi.registerTool`'s expected `execute()`
    return shape (`{content, details}`), so the bridge's `execute()` can pass
    `result.content` straight through.
  - A tool-level failure sets `"isError":true` alongside populated `content`
    (e.g. `workflow_manual` called with no `session_key` →
    `{"content":[{"type":"text","text":"workflow_manual: a valid session_key
    is required"}],"isError":true}`) — no exception/JSON-RPC-error path for
    ordinary tool failures.
  - `ferrule({root, format:"json"})` returns the minted `session_key`
    **embedded inside the JSON-stringified `content[0].text`**, not a
    separate top-level protocol field — the bridge must
    `JSON.parse(content[0].text).session_key`.
  - `initialize`'s response carries
    `serverInfo:{name:"ws-mcp","version":"0.43.4"}`, exactly matching
    `agents-plugin/runtime.json`'s `plugin_version` — a free, no-extra-call
    version-pin check point right after the handshake.
- `ai-docs/spec/mcp-tools.md#L1925-L1936` — `McpNamespace` (renders as
  `ws/<tool>`) is a **reserved template variable** used when the ws-mcp
  playbook renderer produces shared playbook prose. Combined with
  `agents-plugin/skills/lead-add-rule/SKILL.md#L8` and
  `agents-plugin/skills/lead-backfill-docs/SKILL.md#L9-L10`, which hard-code
  the **resolved literal** call syntax `ws/playbook.print(...)`,
  `ws/workflow_manual(...)`: since the raw MCP tool names carry no prefix
  (previous finding), **the bridge must register each tool under
  `"ws/" + rawName`**, not the raw name verbatim. This is the correct
  reading of the ticket's "verbatim `ws/...` name" — "verbatim" means "don't
  mangle the dotted segment" (no `.`→`_` conversion, no server-name
  prefixing à la `pi-mcp-adapter`), not "the wire name already has the
  prefix." Registering bare names (`playbook.print` instead of
  `ws/playbook.print`) would silently break every `SKILL.md`'s literal
  tool-call prose — this is a genuine shortcut-risk signal if missed.
  Independently confirmed by the design anchor's own spike, which
  deliberately registered a tool literally named `ws/playbook.print`
  (`260802-research-ws-pi-native-framework.md`, "Resolved by runtime spike"
  section).
- `ai-docs/spec/mcp-tools.md#L19-L29` — protocol surface is `initialize`,
  `ping`, `tools/list`, `tools/call`; tool-level runtime failures return a
  normal MCP envelope with `isError:true` (confirmed above), while unknown
  methods/profile-rejected tools return real JSON-RPC errors.
- `ai-docs/spec/mcp-tools.md#L82-L92` — `session_key` is mandatory on
  root-aware tools; `ferrule(root)` is the sole bootstrap verb (lead-only,
  the only tool accepting `root`); `mandatory_session_key` /
  `unknown_session` recovery contracts are server-side — the bridge forwards
  them as-is, it does not need to reimplement any of that logic.
- `ai-docs/tickets/.done/260617-refactor-ws-session-bootstrap-obscurity.md` —
  historical rename `ws.lead.login` → `ws.ferrule`. The implementation
  ticket's own Constraints section still says "session_key (minted via
  `ws/lead.login` or `ferrule`)" — stale naming; only `ferrule` exists today.
  Minor prose drift, not a contract conflict (no `lead.login` tool to call).
- `/home/linuxbrew/.linuxbrew/lib/node_modules/@earendil-works/pi-coding-agent`
  — Pi is installed locally at **0.84.4**, exactly the version range the
  design anchor's spikes ran against ("pi 0.83.0–0.84.4"). `docs/extensions.md`
  is the real, current extension API reference (verified directly, not
  fabricated):
  - `docs/extensions.md#L1953-2011` — `pi.registerTool(definition)`:
    `execute(toolCallId, params, signal, onUpdate, ctx)` returns
    `{content, details}`; throwing from `execute` is how a tool signals
    `isError: true` (returning a value never sets it).
  - `docs/extensions.md#L372-387` — `resources_discover` event returns
    `{skillPaths, promptPaths, themePaths}`; confirmed live by
    `examples/extensions/dynamic-resources/index.ts`
    (`pi.on("resources_discover", () => ({skillPaths: [...]})`) — a direct,
    reusable pattern for exposing `agents-plugin/skills/`.
  - `docs/extensions.md#L516-526` — `session_shutdown` fires "before a
    started session runtime is torn down… clean up resources opened from
    session_start" — the exact hook for killing the ws-mcp subprocess.
  - `docs/extensions.md#L220-224` — extensions must **not** start background
    processes (subprocess, sockets, timers) from the top-level factory —
    only from `session_start` or first tool/command use. The bridge must
    spawn the ws-mcp subprocess in `session_start`, not at module load.
  - No built-in MCP client anywhere under `docs/` — self-build is confirmed
    as the only option (matches the anchor's decision, re-confirmed against
    the real, installed docs rather than the anchor's paraphrase).
- `examples/extensions/subagent/index.ts#L344-420` — reference pattern for
  `node:child_process.spawn` + line-buffered stdout JSON parsing
  (`buffer += data.toString(); const lines = buffer.split("\n"); buffer =
  lines.pop() || ""`). Reusable for the bridge's stdout reader, though the
  bridge additionally needs bidirectional stdin writes and
  request/response `id` correlation (the example is one-shot, fire-and-drain
  only).
- `agents-plugin-wsflow/bin/ws-mcp-launcher.py` — byte-identical to
  `agents-plugin/bin/ws-mcp-launcher.py` (`diff` confirms no output),
  establishing existing repo precedent: a sibling package root carries its
  **own copy** of the launcher + its own `runtime.json`, not a cross-root
  relative reference. No sync tooling under `agents-plugin-tool/scripts/`
  currently keeps these in lockstep — it's a manual-copy convention today.
- `agents-plugin/skills/` — 17 skill directories, each hyphen-form
  (`lead-add-rule`, `lead-proceed`, …) with a `SKILL.md`, already matching
  Pi's skill-name charset — no renaming needed for `resources_discover`
  exposure.
- Pi's own `node_modules` bundles `typebox@1.3.7` and `jiti@2.7.0`
  (`pi-coding-agent/package.json:60,64`); single-file example extensions
  (`examples/extensions/question.ts`, `truncated-tool.ts`) import `typebox`
  directly with **no local `package.json`/`node_modules` of their own** —
  confirms `agents-plugin-pi/`'s `package.json` needs no `dependencies` entry
  at all for typebox (available for free via Pi's own resolution), keeping
  the "self-owned client, empty dependency surface" preference achievable
  with zero npm installs.

## Implementation Plan

1. Scaffold `agents-plugin-pi/` (new sibling root, authorized by the ticket's
   Constraints):
   - `agents-plugin-pi/package.json` — `name`, `version`, `"type":"module"`,
     `"pi": {"extensions": ["./src/index.ts"]}`, no `dependencies` (see
     Codebase Findings on `typebox`).
   - Copy `agents-plugin/bin/ws-mcp-launcher.py` →
     `agents-plugin-pi/bin/ws-mcp-launcher.py` and `agents-plugin/runtime.json`
     → `agents-plugin-pi/runtime.json` verbatim, mirroring the
     `agents-plugin-wsflow` precedent. Leave a code comment noting these must
     be kept hand-synced with `agents-plugin/` until a shared-copy mechanism
     exists (same open gap `agents-plugin-wsflow` already has).

2. `agents-plugin-pi/src/mcp-stdio-client.ts` — minimal self-owned
   JSON-RPC-over-stdio client (no `@modelcontextprotocol/sdk`):
   - `spawn("python3", [launcherPath, "serve", "--stdio"], {cwd: pluginDir,
     stdio: ["pipe", "pipe", "pipe"]})`, mirroring `.mcp.json`'s
     `command`/`args`.
   - Line-buffered stdout reader (pattern from
     `examples/extensions/subagent/index.ts#L390-395`) parsing one JSON-RPC
     message per `\n`-terminated line — confirmed newline-delimited, no
     Content-Length framing.
   - Outgoing requests: monotonically increment an id, write
     `JSON.stringify(msg) + "\n"` to stdin, keep a `Map<id, {resolve,
     reject}>`; resolve/reject on the matching response id. Must support
     **concurrent in-flight calls** — `ai-docs/spec/mcp-tools.md#L48-52`
     states only `ws.setup`-class calls are order-fenced, implying ordinary
     `tools/call` responses are not guaranteed in send order.
   - Pipe stderr straight to a diagnostic sink (`console.error` and/or
     `ctx.ui.notify` on hard failure) — never parse it as protocol data.
   - `initialize({protocolVersion: "2025-03-26", capabilities: {},
     clientInfo: {name: "ws-pi-bridge", version: <package.json version>}})`
     on connect; retain `result.serverInfo.version` for the pin-and-fail
     check in step 3.
   - Expose `listTools()` (wraps `tools/list`) and `callTool(name, args)`
     (wraps `tools/call`, returns `{content, isError?}`).

3. `agents-plugin-pi/src/version-check.ts` — pin-and-fail: read the bundled
   `runtime.json`'s `plugin_version`; immediately after `initialize`,
   compare it to `serverInfo.version`; on mismatch, throw synchronously so
   the extension load fails loudly (no tools registered, no silent
   fallback) — satisfies the ticket's pin-and-fail Constraint using a value
   already returned by the handshake, no extra `runtime.info` call needed.

4. `agents-plugin-pi/src/bridge.ts` — tool re-registration:
   - In `session_start`: construct the client, run `initialize` + version
     check (steps 2-3), then `listTools()`. For each `{name: rawName,
     description, inputSchema}` returned, call
     `pi.registerTool({ name: "ws/" + rawName, label: rawName, description,
     parameters: <rawName's inputSchema>, async execute(toolCallId, params) {
     const result = await client.callTool(rawName, resolveSessionKey(params));
     return { content: result.content, details: result }; } })`. The
     `"ws/"` prefix is load-bearing (see Codebase Findings) — this is the
     one place a naming mistake would silently break every skill.
   - `session_key` fill-or-forward (`resolveSessionKey(params)`): if
     `params.session_key` is undefined/empty, splice in the bridge's
     default-filled key (step 5); otherwise pass `params.session_key`
     through unchanged. Never add a synthetic `session_key` to the
     registered tool's `parameters` schema beyond what ws-mcp's own
     `inputSchema` already declares — root-aware tools already advertise it.
   - In `session_shutdown`: kill the subprocess (idempotent — guard against
     double-invocation), matching `docs/extensions.md#L516-526`.

5. Default-fill key bootstrap: after tools are registered in
   `session_start`, call `client.callTool("ferrule", {root: ctx.cwd, format:
   "json"})`, `JSON.parse(content[0].text).session_key` (per the live-probe
   finding — the key is embedded in text, not a top-level field), and store
   it in module state as the default-fill key. If `ferrule` itself fails,
   log via `ctx.ui.notify`/stderr and leave the default-fill key unset — a
   subsequent omitted-`session_key` call then surfaces the server's own
   `mandatory_session_key` guidance rather than the bridge swallowing the
   failure silently.

6. `agents-plugin-pi/src/index.ts` — the extension's default factory:
   - `pi.on("resources_discover", () => ({ skillPaths: [<absolute path to
     the existing `agents-plugin/skills/` directory, resolved relative to
     this file>] }))`, following the confirmed
     `examples/extensions/dynamic-resources/index.ts` pattern. Point at the
     existing `agents-plugin/skills/` directory directly (sibling root, same
     repo) rather than copying skills — the ticket phase text names that
     directory literally, unlike the launcher (step 1), which has repo
     precedent for copying instead.
   - Wire `session_start` → steps 2, 3, 5 (spawn client, version-check,
     register tools per step 4, bootstrap default key).
   - Wire `session_shutdown` → step 4's subprocess kill.

7. In-flight spike (not a planning blocker, flagged so the executor isn't
   surprised): confirm whether `pi.registerTool`'s `parameters` field
   accepts a raw JSON-Schema object (ws-mcp's `inputSchema` as returned by
   `tools/list`) directly, or requires wrapping through `typebox`'s
   `Type.*` builders. Resolve by registering one passthrough tool early and
   checking Pi's validation behavior; if raw JSON-Schema isn't accepted,
   write a small JSON-Schema→typebox shim (ws-mcp's schemas are plain
   `{type, properties, required}` objects per the live probe, not deeply
   nested — a small shim is bounded work either way).

## Verification Plan

- Manual JSON-RPC reproduction (already exercised read-only during survey,
  safe to rerun against the copied launcher):
  `printf '<initialize>\n<tools/list>\n' | python3
  agents-plugin-pi/bin/ws-mcp-launcher.py serve --stdio --root <tmp-git-root>`
  to confirm the wire format still holds after copying.
- Gate (ticket-defined): load the extension
  (`pi -e ./agents-plugin-pi/src/index.ts`, or drop it under
  `.pi/extensions/` for auto-discovery) in a trusted worktree; in an
  interactive or `-p` session, call `ws/workflow_manual` with no
  `session_key` argument and confirm it round-trips against a live model
  using the bridge's default-filled key; call it again with an explicit
  `session_key` (obtained via a manual `ws/ferrule` call in the same
  session) and confirm that exact key is forwarded (observable via the
  returned content or via `ws/session.children` showing lineage).
- Confirm `resources_discover` wiring: invoke `/skill:lead-add-rule` (or any
  other skill) in the same Pi session and confirm it loads with its literal
  `ws/playbook.print(...)` prose intact (no rewriting needed, and the call
  actually dispatches).
- Confirm pin-and-fail: temporarily edit the copied
  `agents-plugin-pi/runtime.json`'s `plugin_version` to a wrong value,
  reload the extension, and confirm it fails loudly (visible error, zero
  tools registered) rather than degrading silently; revert afterward.

## Escalations

- None. The one open implementation detail (step 7 — raw JSON-Schema vs.
  typebox for `parameters`) is a bounded, cheap-to-resolve spike during
  coding, not a strategy or contract question.
