---
title: "ws-cli — MCP-independent CLI fallback surface for Windows disconnects"
related:
  260724-bug-windows-mcp-mid-session-disconnect: root-cause hardening this works around; that ticket fixed the confirmed crash trigger, this one keeps the workflow usable when the connection is gone anyway
  260523-bug-ws-mcp-launcher-runtime-repair-race: already-fixed concurrent-repair safety that makes CLI-triggered runtime repair acceptable
  260703-chore-prefer-subagent-verify-discussion-inline-mirror: introduced the substitution-mirrored skill generation this extends with a ws-cli pattern
  260630-epic-skill-playbook-diet: front-door token budget constraint on the entry-point pointer line
  260513-feat-runtime-binary-staging-copy: adjacent runtime-binary placement work; this ticket deliberately does not stage a binary into bin/
sage-review-design: completed
sage-review-completeness: completed
---

# ws-cli — MCP-independent CLI fallback surface for Windows disconnects

## Background

On Windows the ws/wsflow MCP server disconnects mid-session often enough to
block normal workflow use. `260724-bug-windows-mcp-mid-session-disconnect`
landed four phases of root-cause hardening (request-goroutine panic recovery
with an always-on crash log, launcher abnormal-exit breadcrumb, Windows
parent-death self-termination, SQLite point-read retry + WAL re-assert) and
`260713-bug-mcp-ping-idle-disconnect` restored protocol `ping`, but disconnects
are still observed downstream.

Two host-side facts make the disconnect worse than a transient error:

- Claude Code does **not** auto-reconnect a dropped ws MCP server. Once the
  server is gone, its tools stay absent from the tool list for the rest of the
  session.
- The manual re-enable path is not discoverable, so a user who hits this has no
  obvious recovery action.

The result is a total workflow stop: every ws skill's front door begins with an
MCP call, so with the server down the session cannot even bootstrap. This ticket
adds a fallback path that does not depend on the MCP connection at all, plus the
discovery surface that makes an agent actually take it.

### Enabling facts (source-verified during design)

- Claude Code puts a plugin's `bin/` directory on `PATH`. Verified live:
  `/Users/kang-sw/.claude/plugins/ws-plugin/ws/bin` is present in `PATH` and
  `ws-mcp-launcher.py` resolves from it.
- The launcher is already argv-transparent — `ws-mcp-launcher.py:883` builds
  `[str(binary), *sys.argv[1:]]` and execs it. No new argument plumbing is
  needed; `serve --stdio` is only what `plugin.json` happens to pass.
- Session keys are file-backed, not in-memory: `sessionStore.readRecord`
  (`internal/mcp/session_auth.go:359`) reads one JSON record per key from the
  keys directory. A fresh CLI process can therefore reuse a `session_key` minted
  by the MCP server process. This is what makes the fallback able to continue an
  in-flight session rather than start over.
- `Server.callTool` (`internal/mcp/server.go:485`) is a single dispatch entry
  point keyed by tool name, and `Server.filteredTools()` (`server.go:4368`)
  already produces the profile-aware `name`/`description`/`inputSchema` list that
  `tools/list` returns.

### Measured invocation cost (macOS arm64, warm, 2026-07-25)

| Path | Time |
|---|---|
| Binary direct (`version`) | 7.8 ms |
| Launcher-mediated (`version`) | 84.9 ms |
| `python3 -c pass` (reference) | 21.4 ms |
| Real work: `tickets list` (binary direct) | 13 ms |
| Real work: `git status` (binary direct) | 46 ms |

Attribution of the 77 ms launcher overhead: ~21 ms interpreter startup, ~22 ms
`import urllib.request`, ~6 ms launcher logic (`runtime.json` read 0.1 ms +
local-devenv source fingerprint rglob 2.2 ms + `detect_project_root` 3.7 ms), the
remainder residual stdlib imports. The compatibility stamp does not hash the
12 MB binary — `compatibility_stamp_payload` (`ws-mcp-launcher.py:316`) uses
`stat` size + mtime only — and the source fingerprint walk is likewise stat-only,
so the warm path is dominated by Python process startup, not by launcher work.

Conclusion: a real fallback call costs ~90-130 ms on macOS, which is noise
against a Bash tool round-trip. Windows is expected to be materially worse
(slower interpreter startup, AV scanning, `subprocess.call` instead of `exec`)
but was not measured — see Constraints.

## Decisions

- **Expose the fallback as a generic passthrough, not as hand-written
  per-tool subcommands.** Two new subcommands on the existing `ws-mcp` binary:

  ```
  ws-cli tools                  # mapping rule + tool name/description list (the fallback's help entry point)
  ws-cli tools <name>           # that tool's inputSchema
  ws-cli call <name> '<json>'   # invoke, printing the tool's text content to stdout
  ```

  Both read from `filteredTools()` and `call` routes through `callTool`, so the
  documented mapping rule collapses to one line: `ws/x.y(a: b)` becomes
  `ws-cli call x.y '{"a":"b"}'`. Coverage is total and drift is structurally
  impossible.

- **Bare `tools` prints names and descriptions only; schemas are per-tool via
  `tools <name>`.** Measured against the staged 0.36.1 binary, the full
  `tools/list` payload is 49,954 bytes across 64 tools (~13k tokens) while
  name+description alone is 12,378 bytes. `tools` is read once per recovery and
  is the entry point an agent lands on with no other context, so dumping every
  `inputSchema` there would spend ~13k tokens to deliver mostly-unused schemas.
  Playbooks that depend on a specific tool's schema fetch it individually.

- **Rejected: documenting the existing hand-written subcommands as the fallback
  surface.** `main.go`'s switch covers `git|tickets|specs|mental-models|
  references|config|path|runtime|mercenary` but not `ferrule`,
  `workflow_manual`, `playbook.print`/`render`, `convention.read`,
  `project_tree`, `todo.*`, `agenda.*`, or `enter.*` — i.e. not the tools needed
  to bootstrap a session at all. Documenting it would also require a hand-
  maintained flag-to-schema mapping that drifts from the MCP schemas on every
  tool change.

- **The existing hand-written subcommands stay** (they are load-bearing for
  tests), but they are explicitly **not canonical**. Only `tools` and `call` are
  the documented fallback surface.

- **The generic usage text is not the fallback entry point.** `main.go` has no
  `--help` case at all — an unrecognized argument falls through to `usage()` and
  exits 2 — and that usage line interleaves the non-canonical hand-written
  subcommands with the fallback surface. `ws-cli tools` with no argument is the
  entry point instead: it prints the mapping rule and the tool list, and the
  hand-written subcommands never appear there because they live in the `main.go`
  switch rather than in the tool list. The existing `usage()` text is left
  untouched.

- **Ship a `bin/` shim, not a copied binary.** `bin/ws-cli` (POSIX shebang
  script, resolvable from git-bash) plus `bin/ws-cli.cmd` (for `cmd`/PowerShell,
  which will not resolve a bare extensionless name), both delegating to the
  existing launcher. Rejected: renaming/copying the runtime binary into `bin/` —
  the real binary lives under `.runtime/<platform>/` under contract + stamp
  management, and duplicating it there would fork the staging/repair logic and
  collide with `260513-feat-runtime-binary-staging-copy`. The "extra logic"
  originally feared for the shim approach does not exist, because the launcher
  already resolves the binary and forwards argv.

- **Shim names are namespace-scoped: `ws-cli` for ws, `wsflow-cli` for wsflow.**
  Both plugins currently ship an identically-named `bin/ws-mcp-launcher.py`, so
  with both installed a shared shim name would resolve by `PATH` order to an
  arbitrary plugin.

- **Each shim bakes in its own environment.** wsflow's `WS_MCP_NO_AGENT=1`,
  `WS_MCP_NAMESPACE=wsflow`, and `WS_MCP_SETUP_TOOL=setup` are supplied only by
  the `env` block of its `plugin.json` `mcpServers` entry. A `PATH`-invoked shim
  inherits none of it, so without baking, a wsflow user's fallback would run in
  full-ws mode under namespace `ws` and expose the mercenary surface that the
  agentless product deliberately hides.

- **CLI-triggered runtime repair is allowed; a `--no-repair` fast path was
  rejected.** A CLI-only session must still be able to update its runtime, and
  making the user run an install script is a far more expensive failure mode
  (cognitively, and in practice) than a slow first call. The concurrent-repair
  hazard that motivated the alternative was already fixed in `fb8e156d`
  (`260523-bug-ws-mcp-launcher-runtime-repair-race`): contract-addressed cache
  binary names, process-unique temporary files, and best-effort replacement with
  compatible-target fallback. The residual cost is latency on a stale-runtime
  first call, not corruption.

- **Discovery goes through a dedicated skill, `/ws:mcp-server-repair` (mirrored
  as `/wsflow:mcp-server-repair`), not through the workflow manual.** The manual
  is delivered *by* the `workflow_manual` MCP tool, so a fallback paragraph
  living there is unreachable in exactly the situation it addresses. Skills are
  read from disk by the host and stay listed when the server is down. Rejected:
  editing the downstream bootstrap `AGENTS.md`, which would force every
  downstream project to re-bootstrap.

- **The repair skill body must be fully self-contained.** Every other ws skill's
  front door delegates to `ws/playbook.print`; this one cannot, or it dies at the
  moment it is needed. Mapping rule, `ws-cli tools` usage, the cold-start
  sequence, the PATH-independent invocation, and the manual reconnect procedure
  go directly in `SKILL.md`.

- **The skill must state the cold-start `session_key` sequence explicitly.** The
  motivating failure is that the session cannot bootstrap at all, and every
  root-aware tool hard-fails without a key (`mandatory_session_key`,
  `server.go:2926`) while `handleWorkflowManual` (`workflow_manual.go:210`)
  rejects a keyless call outright. The working sequence is
  `ws-cli call workflow_manual '{"session_key":"obsidian-latch","root":"<abs worktree>"}'`,
  which mints and returns a lead key to reuse for every later call. It is
  derivable from `lead-revive`, but a self-contained skill that omits it leaves
  the primary scenario unrecoverable.

- **The skill must also give the PATH-independent form.** `PATH` injection of a
  plugin's `bin/` is verified on macOS only (see Constraints), so the skill
  documents `python3 <plugin-root>/bin/ws-mcp-launcher.py tools` as the fallback
  to the fallback when the shim name does not resolve.

- **`mcp-server-repair` breaks the `lead-` prefix deliberately.** Every existing
  skill in both plugins is `lead-*`. This one is a recovery utility rather than a
  step in the lead workflow, and the name is the user-chosen invocation surface
  (`/ws:mcp-server-repair`), so the break is intentional and not an accident of
  phase text.

- **The skill has two audiences.** For the agent: keep working through `ws-cli`.
  For the user: the concrete steps to re-enable the MCP server, since that path
  is otherwise undiscoverable. The agent cannot re-enable the server itself and
  must surface the procedure.

- **A pointer line goes in the four session entry-point front doors** —
  `lead-discuss`, `lead-sprint`, `lead-revive`, `lead-proceed` — pointing at
  `/ws:mcp-server-repair` if the front door's MCP call fails.

  These four are **not** substitution-mirrored: `substitutionMirroredSkills`
  (`internal/wsrsrc/skills_mirror_test.go:15`) is a curated list of exactly
  `lead-goal-step`, `lead-prefer-subagent`, and `lead-verify-discussion`. The
  wsflow copies of the four entry points are hand-curated and already diverge
  from the ws copies, so **eight files** need the edit, and the wsflow four must
  be hand-written with `wsflow-cli` and `/wsflow:mcp-server-repair`.

  On the wsflow side the line **replaces an existing dead end**: wsflow's
  `lead-proceed` ends with "If the playbook cannot be loaded, stop and report
  that blocker." and its `lead-revive` with the equivalent "If the tool cannot be
  loaded, stop and report that blocker." (the same sentence appears in roughly a
  dozen other wsflow skills, which stay out of scope here). wsflow's
  `lead-discuss` and `lead-sprint`, and all four ws copies, carry no such line
  and simply gain one.

  Rejected: all 17 skills. Rejected: the originally-proposed criterion "skills
  that load `workflow_manual`", which selects only `lead-discuss`, `lead-revive`,
  and `lead-sprint` — `lead-proceed` calls `playbook.print` only, and omitting it
  would leave the "start from a ticket" path uncovered. The remaining 13 skills
  are only invoked once a session is already running, so the entry points filter
  them.

  Rationale for having the line at all, given the standalone skill exists:
  noticing that tools are *absent* from a tool list is a weak signal, whereas an
  MCP call returning an error is the highest-attention moment available, and the
  only text in front of the model at that moment is that skill's front door.
  `lead-revive` matters most — its description already positions it as the
  pre-everything recovery entry, yet its body is a `workflow_manual` call that
  dead-ends when the server is down.

- **`skills_mirror.go` gains a narrow `\bws-cli\b` -> `wsflow-cli` pattern, and
  `mcp-server-repair` joins `substitutionMirroredSkills`.** Current substitution
  covers only `\bws:` and `\bws/`, so a mirrored wsflow skill would instruct the
  agent to run `ws-cli` — the exact wrong-namespace bug the split naming exists
  to prevent, and one the `disqualifyingTokens` guard does not catch. The pattern
  must stay narrow: `ws-mcp` and `ws-plugin` are real names that must not become
  `wsflow-*`. The repair skill's text is namespace-only, so its wsflow copy is
  generated rather than hand-written — unlike the four entry-point front doors.

- **`import urllib.request` becomes lazy in the launcher.** It costs 22 ms of the
  85 ms warm path (~26%) and is only needed on the download-install path. Folded
  into this ticket as a pure gain, with a larger relative payoff on Windows.

## Constraints

- **The `PATH` enabling fact is verified on macOS only.** The whole ticket
  targets Windows, but the live check that a plugin's `bin/` lands on `PATH` was
  run against `/Users/kang-sw/.claude/plugins/ws-plugin/ws/bin`. If Claude Code
  does not inject plugin `bin/` on Windows, the shims are unreachable there and
  Phases 2-3 ship nothing usable on the target platform — which is why the skill
  must carry the PATH-independent `python3 <plugin-root>/bin/ws-mcp-launcher.py`
  form. Windows `PATH` resolution is a downstream dogfood acceptance item, not a
  local gate.
- Windows per-call cost is **unmeasured**. Interpreter startup is typically
  several times slower than macOS, AV scanning adds to it, and the Windows
  launcher branch uses `subprocess.call` rather than `exec`, so the estimate is
  roughly 250-400 ms per call. Measuring it would fit naturally as a timing step
  in the existing `windows-smoke` CI job, but that is **out of scope here** — no
  Windows test environment is available to this work. Recorded so a later session
  does not treat the macOS numbers as cross-platform.
- The fallback keeps the launcher's existing `python3`-on-`PATH` requirement. If
  the MCP failure is itself caused by a broken Python, the fallback fails too.
- Launcher edits must be applied to `agents-plugin/bin/ws-mcp-launcher.py` and
  kept byte-identical with the `agents-plugin-wsflow/` mirror.
- Text intended for substitution mirroring must clear
  `guardSubstitutionEligible`'s denylist, which includes the bare token `ws.`
  matched by naive substring containment — so any sentence-ending word such as
  "flows.", "shows.", or "knows." disqualifies the source. Mirrored skill text
  must avoid them.
- **Keyless privilege escalation is a pre-existing non-goal, not a regression to
  fix here.** `callTool` deliberately leaves keyless callers ungated
  (`server.go:517`), so `ws-cli call ferrule '{"root":...}'` mints a lead key for
  anything with shell access, including a subagent whose ws MCP tools the host
  filtered out. The identical escalation is already reachable today by piping
  JSON-RPC into `ws-mcp serve --stdio`, so the CLI makes it convenient rather
  than newly possible. Recorded so a later session does not mistake it for a
  defect this ticket introduced; closing it is separate scope.
- The entry-point pointer must stay to a single short line. Only one skill loads
  per invocation, so the practical cost is ~15 words per invocation, but
  `260630-epic-skill-playbook-diet` governs the front-door budget.

## Prior Art

- `Server.callTool` (`internal/mcp/server.go:485`) — single name-keyed dispatch
  with the session-scope and product-profile gates already applied; reuse rather
  than reimplement, so CLI callers inherit identical authorization behavior.
- `Server.filteredTools()` (`internal/mcp/server.go:4368`) — profile-aware tool
  list with schemas, already reflecting agentless mode and mercenary hiding.
- `ws-mcp-launcher.py:883` — existing argv passthrough.
- `internal/wsrsrc/skills_mirror.go` — `GenerateWsflowSkillBody` plus
  `guardSubstitutionEligible`.
- `internal/mcp/session_auth.go:359` — file-backed session records.

## Spec Impact

- Target spec areas: `mcp-tools.md` for the `ws-cli tools` / `ws-cli call`
  passthrough contract and its equivalence to the MCP tool surface;
  `plugin-runtime.md` for the `bin/` shim names, per-plugin env baking, `PATH`
  exposure, and CLI-triggered repair behavior; `workflow-skills.md` for the
  `mcp-server-repair` skill and the entry-point pointer rule.
- Expected caller-visible change: every MCP tool becomes invocable as a
  subprocess with the same name and schema; two new `PATH`-resolvable
  executables per plugin (`ws-cli`/`ws-cli.cmd`, `wsflow-cli`/`wsflow-cli.cmd`);
  one new skill per namespace; a pointer line in eight existing skills (four per
  namespace).
- Contract-first spec: no. Tool names and schemas are already specified in
  `mcp-tools.md` and the CLI is a transport mirror of them rather than a new
  contract; the shim naming and skill behavior are reflected into the specs at
  closeout.

## Phases

### Phase 1: Generic CLI passthrough (`tools` / `call`)

Add `tools` and `call` subcommands to `cmd/ws-mcp/main.go`, routing through the
existing `Server` rather than duplicating handler logic.

`tools` with no argument prints the mapping rule (`ws/x.y(a: b)` ->
`ws-cli call x.y '{"a":"b"}'`) followed by `filteredTools()` reduced to name and
description; `tools <name>` prints that tool's `inputSchema`. `call <name>
'<json>'` builds the same request shape `callTool` consumes and writes the
resulting text content to stdout, with a non-zero exit and a readable message on
tool error, unknown tool name, or malformed JSON (`isError` on the tool response
is the signal for the error exit).

Output must remain profile-correct: an agentless (`WS_MCP_NO_AGENT=1`) invocation
must not list or dispatch agent-backed tools, and session-scope gating in
`callTool` must apply unchanged — a non-lead `session_key` must not reach
lead-only tools through the CLI. `--help` is deliberately left alone.

Verification: `tools` lists exactly the tool names `tools/list` returns for the
same profile in both full-ws and agentless mode, and carries no `inputSchema`;
`tools <name>` matches that tool's schema from `tools/list`; a `call` round-trip
against a real tool using a `session_key` minted by a separate process returns
the same text the MCP path returns; the documented cold-start call
`call workflow_manual '{"session_key":"obsidian-latch","root":"<abs>"}'` mints a
usable lead key; error paths exit non-zero.

### Result (6b1038b7) - 2026-07-25

Added `tools` and `call` subcommands to `cmd/ws-mcp/main.go`. Both route through
`mcp.NewServer(...).ServeStdio` fed a synthetic single-line JSON-RPC request —
the same mechanism `serve()`/`runSmoke()` already use — so `filteredTools()` and
`callTool` profile and session-scope gating are inherited unchanged, with **zero**
`internal/mcp` export changes (the two methods are unexported; the ServeStdio
round-trip is the sanctioned reuse path). Bare `tools` prints the mapping rule
plus each tool's name/description only (no `inputSchema`); `tools <name>` prints
that one tool's `inputSchema`; `call <name> '<json>'` dispatches and writes the
tool's text content to stdout, exiting non-zero on tool error, unknown tool, or
malformed JSON.

Error-shape handling confirmed against source: an unknown tool name returns a
JSON-RPC-level `errorResponse(-32602, "unknown tool: ...")` (`server.go:1720-1721`,
the `callTool` switch `default:`), not a tool-level `isError`, so `callCommand`
checks both shapes — `resp.Error != nil` first, then `result.IsError` — before
extracting `content[0].text`. Malformed JSON is caught by a pre-dispatch
`json.Unmarshal` probe, not surfaced as a `-32700` from `ServeStdio`. Arg-count
usage errors exit 2 (matching `usage()`); tool/runtime/parse errors exit 1.

Verification: from `agents-plugin-tool/`, `go build ./...` and `go vet ./...`
clean; `go test ./cmd/ws-mcp/... ./internal/mcp/...` green. Eight new
`main_test.go` cases cover all Verification Plan bullets — bare-list parity
(full + agentless, no `inputSchema`), `tools <name>` schema parity, unknown-tool
for both `tools` and `call`, malformed JSON, cross-process session-key round trip
(two subprocesses sharing `WS_CACHE_HOME`), non-lead scope gating (delegate key
rejected from `ferrule`/`workflow_manual`), and the cold-start `obsidian-latch`
mint. All assertions compare against an independent in-process `ServeStdio` drive
rather than the CLI's own output.

Deviations: the cross-process round-trip test exercises `git.status` rather than
`tickets.list` — an empty temp repo has no `ai-docs/tickets`, which would trip the
tool-level error path instead of a clean round trip. No other deviation; all eight
verification bullets landed.

Review: partitioned correctness/fit/test, all clean — **0 critical, 0 important**.
Six minor items, all accepted (no remediation): generic `-32602 invalid params`
message for a valid-but-non-object JSON argument; latent tool-`description`-with-
newline listing/parse fragility (no current tool has one) in both the CLI output
and the bare-list test; the JSON-RPC-error branch using an inline
`fmt.Fprintf+os.Exit(1)` rather than `fatal()` to keep the two error branches
parallel and surface tool text verbatim; `mcpLineResponse`'s unread `JSONRPC`/`ID`
wire-mirror fields; and `CombinedOutput()` vs the more precise `.Output()` in the
round-trip golden check.

Deferred (unchanged from ticket): spec reflection into `mcp-tools.md` /
`plugin-runtime.md` / `workflow-skills.md` and any durable mental-model invariant
stay closeout-only per the ticket's "Contract-first spec: no" decision — captured
once Phases 2-3 settle the full fallback surface. Phases 2-3 not started; the
ticket remains in `ready/`.

### Phase 2: `bin/` shims, namespace env baking, and launcher warm-path trim

Add `bin/ws-cli` + `bin/ws-cli.cmd` to `agents-plugin/`, and `bin/wsflow-cli` +
`bin/wsflow-cli.cmd` to `agents-plugin-wsflow/`, each delegating to its own
plugin's launcher and exporting that plugin's environment (`WS_MCP_NO_AGENT`,
`WS_MCP_NAMESPACE`, `WS_MCP_SETUP_TOOL` for wsflow) before doing so. Extend
`skills_mirror.go` with the narrow `\bws-cli\b` -> `wsflow-cli` pattern. Make
`import urllib.request` lazy in the launcher, keeping the two launcher copies
byte-identical.

Depends on Phase 1: the shims front commands that must already exist.

Verification (local): both shims resolve as bare names on `PATH` from a POSIX
shell; the wsflow shim yields namespace `wsflow` with the agentless profile and
no mercenary tools in `wsflow-cli tools` output; mirror generation rewrites
`ws-cli` to `wsflow-cli` while leaving `ws-mcp` and `ws-plugin` untouched;
launcher warm path improves by roughly the measured 22 ms; launcher `diff`
between the two plugin copies stays empty.

Verification (downstream dogfood, not a local gate): bare-name resolution from
`cmd`/PowerShell on a real Windows host, per the Constraints note that no Windows
test environment is available to this work. Do not block the phase on it; record
the result when a Windows session next runs.

### Result (1281b11a) - 2026-07-25

Added the four shims — `agents-plugin/bin/ws-cli` (+x, **env-free**),
`agents-plugin/bin/ws-cli.cmd`, `agents-plugin-wsflow/bin/wsflow-cli` (+x, bakes
`WS_MCP_NO_AGENT=1` / `WS_MCP_NAMESPACE=wsflow` / `WS_MCP_SETUP_TOOL=setup`), and
`agents-plugin-wsflow/bin/wsflow-cli.cmd` — each delegating to its own plugin's
`ws-mcp-launcher.py` via `exec python3 "$script_dir/ws-mcp-launcher.py" "$@"`
(the launcher already resolves the binary and forwards argv+`os.environ`, so no
new plumbing). Confirmed the env-baking asymmetry against source: `ws` `plugin.json`
has no `env` block and `RuntimeNamespace()`/`NoAgentMode()` default to full-ws, so
`ws-cli` correctly exports nothing; only `wsflow-cli` carries the three vars. Made
`import urllib.request` lazy inside `download_file()` (the sole `urlopen` caller,
on every download path) in both launcher copies, kept byte-identical. Extended
`internal/wsrsrc/skills_mirror.go` with `wsCliPattern` (`\bws-cli\b` -> `wsflow-cli`,
a literal-token rule that does not touch `ws-mcp`/`ws-plugin` and does not collide
with the existing `\bws:`/`\bws/` patterns) plus a pinning `TestWsCliSubstitutionPattern`.

Verification: from `agents-plugin-tool/`, `go build ./...` / `go vet ./...` clean;
`go test ./internal/wsrsrc/... ./cmd/ws-mcp/...` green including the new test and
the unchanged `TestWsflowSkillsMirrorUpToDate`; `diff` of the two launcher copies
empty. Local shim resolution driven end-to-end (via `WS_MCP_BOOTSTRAP_BINARY`,
since the source tree is not itself a recognized local-devenv-runtime cache path —
a verification-harness detail, not a defect): `ws-cli runtime capabilities` reports
the full surface (`mercenary.*`, `config.workflow_prefer_mercenary` present) while
`wsflow-cli runtime capabilities` omits them, proving the baked
`WS_MCP_NO_AGENT=1`/`WS_MCP_NAMESPACE=wsflow` env reaches the subprocess.

Deviations: none from the plan contract; `substitutionMirroredSkills`,
`cmd/ws-mcp/main.go`, and the Phase 3 surface were left untouched. A stray untracked
`agents-plugin-wsflow/.runtime/` produced by local shim verification was removed
before commit (pre-existing `.gitignore` gap, out of scope to fix here).

Review: partitioned correctness/fit/test, all clean — **0 critical, 0 important**.
Two minor items accepted (no remediation): (1) `wsflow-cli.cmd`'s three `set`
statements lack an enclosing `setlocal`, so an interactive Windows `cmd` session
would leak the three vars after the launcher returns — functional behavior is
correct (the vars still reach `python3`), the leak is interactive-Windows-only, and
the plan explicitly scopes `.cmd` correctness as downstream dogfood; wrapping in
`setlocal`/`endlocal` for exec-no-leak parity with the POSIX shim is a good polish
to fold into the Windows dogfood pass. (2) `TestWsCliSubstitutionPattern` exercises
the left/standalone boundary (`ws-mcp`/`ws-plugin` untouched) but not the right
boundary (`ws-client`), which is protected by the pattern's `\b` — an optional
coverage top-up.

Concurrency note: implemented under strict explicit-pathspec isolation from a
concurrent unrelated session's five uncommitted files (`agents-plugin{,-wsflow}/rsrc/lead-write-ticket/lead-write-ticket.md`,
both `rsrc/manifest.json`, `ai-docs/_index.md`); commit `1281b11a` contains exactly
the eight Phase 2 files and none of them.

Deferred (unchanged from ticket): spec reflection into `mcp-tools.md` /
`plugin-runtime.md` / `workflow-skills.md` stays closeout-only. Phase 3 (the
`mcp-server-repair` skill and eight front-door pointers) remains in `ready/`.

### Phase 3: `mcp-server-repair` skill and entry-point pointers

Author `agents-plugin/skills/mcp-server-repair/SKILL.md` with a description that
names the trigger explicitly (ws tools absent from the tool list, or a ws tool
call failing to connect) and a fully self-contained body covering: `ws-cli tools`
/ `ws-cli tools <name>` / `ws-cli call` usage, the mapping rule, the cold-start
`workflow_manual` + `obsidian-latch` sequence, the PATH-independent
`python3 <plugin-root>/bin/ws-mcp-launcher.py` form, the note that a stale
runtime may make the first call slow because repair runs, and the manual
reconnect procedure to relay to the user. The body must make no MCP call.

Register the new skill on every surface that enumerates skills, not just the
mirror: `substitutionMirroredSkills` (`internal/wsrsrc/skills_mirror_test.go`),
the hash manifest `agents-plugin/skills/manifest.json` (guarded by
`internal/wsrsrc/skills_manifest_test.go`), `EXPECTED_SKILLS` in
`agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`, and the shipped-skill
list in `ai-docs/ref/wsflow-mirroring.md`. Confirm the source passes
`guardSubstitutionEligible`, then generate the wsflow mirror.

Add the one-line pointer to eight front doors: `lead-discuss`, `lead-sprint`,
`lead-revive`, and `lead-proceed` in **both** `agents-plugin/skills/` and
`agents-plugin-wsflow/skills/`. The wsflow four are hand-written (they are not in
`substitutionMirroredSkills`) and use `wsflow-cli` /
`/wsflow:mcp-server-repair`; in wsflow's `lead-proceed` and `lead-revive` the
pointer replaces the existing "stop and report that blocker" sentence rather than
being added beside it.

Depends on Phases 1 and 2: the skill documents the invocation form they define.

Verification: the skill is listed by the host with the ws MCP server stopped; its
body makes no MCP call; mirror generation succeeds and produces `wsflow-cli` in
the mirrored body; `go test ./internal/wsrsrc/...` and the wsflow bundle test both
pass with the new skill registered; each of the eight front doors changes by
exactly the pointer line (plus, in the two wsflow cases, removal of the replaced
sentence).

### Result (adbf5ec3) - 2026-07-25

Authored `agents-plugin/skills/mcp-server-repair/SKILL.md` as a Choreography-layout
skill (Invariants -> On: X handlers -> Templates -> Doctrine), fully self-contained
with **no MCP call** anywhere in its body — the whole point is to keep working after
the MCP server drops. It covers the `ws-cli tools` / `ws-cli tools <name>` /
`ws-cli call <name> '<json>'` surface, the mapping rule
(`ws/x.y(a: b)` -> `ws-cli call x.y '{"a": "b"}'`), the cold-start
`ws-cli call workflow_manual '{"session_key": "obsidian-latch", "root": "<abs worktree>"}'`
sequence, the PATH-independent `python3 <plugin-root>/bin/ws-mcp-launcher.py` fallback,
the stale-runtime slow-first-call note, and a "relay verbatim to the user" reconnect
template. The reconnect steps are described functionally (plugin-provided stdio server,
not auto-reconnected: run `/mcp`, toggle the server off/on or `/reload-plugins`, else
restart the session) rather than hardcoding the `plugin:ws-plugin:ws` identifier — the
literal identifier does not survive the `ws:`/`ws/`/`ws-cli` substitution and would
leave a wrong string in the wsflow mirror, so a namespace-neutral description keeps both
mirrors correct.

Registered on all four enumeration surfaces: appended `"mcp-server-repair"` to
`substitutionMirroredSkills` (`internal/wsrsrc/skills_mirror_test.go`); regenerated the
wsflow mirror via `WS_REGEN_WSFLOW_SKILLS=1 ... TestRegenerateWsflowSkillsMirror`
(produces `wsflow-cli`/`wsflow/` in the generated body; `ws-mcp-launcher.py` correctly
stays verbatim because the wsflow package ships that same filename); regenerated
`agents-plugin/skills/manifest.json` via `WSRSRC_REGEN_SKILLS=1 ...
TestGenerateRealSkillsManifest` (skills manifest only — the rsrc regens were never run);
and added `mcp-server-repair` to `EXPECTED_SKILLS` + `EXPECTED_INLINE_SKILLS` in
`test_wsflow_skill_bundle.py` and to both the Shipped-Skills list and the
Substitution-Mirrored curated list in `ai-docs/ref/wsflow-mirroring.md`. Confirmed the
source is namespace-only: zero lowercased `ws.` substring and none of the other
disqualifying tokens, so `guardSubstitutionEligible` passes.

Added the one-line pointer to eight front doors. The six that carried no prior blocker
line simply gain it — ws `lead-discuss`/`lead-sprint`/`lead-revive`/`lead-proceed` gain
`` If this call fails to connect, run `/ws:mcp-server-repair`. ``, and wsflow
`lead-discuss`/`lead-sprint` gain the `` /wsflow:mcp-server-repair `` form. The two
wsflow front doors that ended in a "stop and report that blocker" sentence
(`lead-proceed`, `lead-revive`) have that sentence **replaced** by the pointer. Updated
the two hardcoded `re.fullmatch` bundle tests with per-skill exact tails (not loosened
optional groups): `test_skill_files_are_thin_playbook_shims` excludes `lead-proceed` and
gains a dedicated `test_lead_proceed_shim_carries_repair_pointer`;
`test_parallel_init_skill_files_are_playbook_shims` uses an explicit per-skill
`pointer_tail` map so a missing pointer on discuss/sprint fails loudly.

**New skill discoverability (recorded here because the `ai-docs/_index.md` refresh was
skipped this round — a concurrent unrelated session holds uncommitted edits there):** the
new `mcp-server-repair` skill is invocable as `/ws:mcp-server-repair` (full ws) and
`/wsflow:mcp-server-repair` (wsflow). It is the CLI-fallback recovery skill for when the
MCP tools vanish or a tool call fails to connect.

Verification: from `agents-plugin-tool/`, `go build ./...` / `go vet ./...` clean;
`go test ./internal/wsrsrc/... ./cmd/ws-mcp/...` green (including
`TestWsflowSkillsMirrorUpToDate` now covering `mcp-server-repair` and
`TestSkillsManifestDriftIsVisible`); from repo root `python3 -m unittest discover
agents-plugin-wsflow/tests` green (10 tests, including the two updated exact-match tests
and the new dedicated lead-proceed check). Diff-checked all eight front doors: each
changed by exactly the pointer line, with the two wsflow replacements also removing the
old blocker sentence.

Deviations: none from the plan contract. Committed under strict explicit-pathspec
isolation from the concurrent session's five uncommitted files
(`agents-plugin{,-wsflow}/rsrc/lead-write-ticket/lead-write-ticket.md`, both
`rsrc/manifest.json`, `ai-docs/_index.md`) — none were staged. The implementation
landed in `adbf5ec3`; this Result is recorded in the follow-up docs commit per the
Phase 1/2 convention.
