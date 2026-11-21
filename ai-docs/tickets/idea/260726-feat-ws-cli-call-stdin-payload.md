---
title: "ws-cli call: stdin에서 JSON 페이로드 입력 지원"
sage-review-design: required
related:
  260726-feat-ws-cli-lenient-tool-name-resolution: sibling daemon-hosted CLI ergonomics ticket from the same host-MCP-down, daemon-available dogfooding session
---

# ws-cli call: accept JSON payload from stdin

## Background

During recovery dogfooding where the host MCP connection was down but the
daemon remained available, the daemon-hosted CLI path (`ws-cli` -> launcher ->
daemon `hostedCLI.mcpCommand`) proved workable for reads and bootstrap, but
complex root-aware **write** payloads are painful: the payload must be
inline-quoted in the shell. Multi-line bodies with embedded double quotes are
the concrete pain case — `git.commit` messages carrying a `## AI Context`
block, and `agenda.set` prose. Shell quoting of such payloads is error-prone
exactly when the operator is already in an outage and least able to absorb
extra failure modes.

Verified current behavior:

- The launcher captures stdin only for `mailbox codex-stop-hook` and `git
  commit --description-file=-`; `call <name> -` is not included
  (`agents-plugin/bin/ws-mcp-launcher.py:2424-2431`).
- Captured one-shot input is carried as the CLI hello's optional `stdin` field
  (`ws-mcp-launcher.py:2242-2248`;
  `internal/wsdashboard/proxy.go:54-64`).
- The daemon's `hostedCLI.mcpCommand` hard-requires two call args and
  unmarshals `args[1]` directly (`cmd/ws-mcp/hosted_cli.go:527-539`). Although
  `hostedCLI.input()` can read the hello's stdin (`hosted_cli.go:40-46`),
  `mcpCommand` does not use it and has no `-` / `@file` sentinel handling.

Consequence: `echo '{...}' | ws-cli call git.commit` fails with the usage
error, because the payload argument is simply absent.

## Decisions

- **Add a stdin payload source using the standard unix convention**: when the
  payload argument is exactly `-`, read the whole payload from stdin instead of
  treating `-` as JSON.
- This deliberately **preserves the existing 2-arg shape** (`<name>` + `-`), so
  the `len(args) != 2` guard and every existing invocation stay unchanged. No
  new arg arity, no flag parsing, no change to the usage contract for the
  inline form.
- A `@path` file-source variant is a **nice-to-have**: optional/secondary scope,
  not a requirement. Ship the stdin form first; add `@path` only if it costs
  little.

Target usage to make work:

```sh
ws-cli call git.commit - <<'JSON'
{"session_key":"...","message":"feat(x): ...","ai_context":"..."}
JSON
```

## Constraints

- Malformed-JSON and empty-stdin error messages must stay as legible as the
  current inline-arg failure. An empty or whitespace-only stdin payload must
  produce a distinct, actionable message rather than a bare JSON syntax error.
- The inline form must remain byte-for-byte compatible; a literal payload that
  happens to be `-` is not valid JSON today, so reinterpreting it is not a
  behavior regression.
- Exit codes keep their current meaning: `2` for usage errors, `1` for
  JSON-RPC-level or tool-level (`isError`) failures.

## Prior Art

- `260726-feat-ws-cli-lenient-tool-name-resolution` — sibling ticket from the
  same dogfooding session, also improving daemon-hosted CLI ergonomics on the
  `ws-mcp call` path. Coordinate touch points in `hostedCLI.mcpCommand` if both
  land near each other; do not fold the two scopes together.
- The launcher and proxy hello already carry stdin for selected one-shot
  commands (`agents-plugin/bin/ws-mcp-launcher.py:2242-2248,2424-2431`), and
  `hostedCLI.input()` already consumes that field (`hosted_cli.go:40-46`). The
  missing pieces are selecting `call <name> -` for capture and reading it in
  `mcpCommand`.

## Phases

### Phase 1: stdin payload source in hostedCLI.mcpCommand

Teach `hostedCLI.mcpCommand`
(`agents-plugin-tool/cmd/ws-mcp/hosted_cli.go:527-539`) to resolve its payload
from `c.input()` when `args[1] == "-"`.

Goals:

- Keep the `len(args) != 2` guard and the two-arg shape untouched.
- Read all of `c.input()` when the payload arg is exactly `-`, then feed the
  resulting bytes into the existing `json.Unmarshal` probe and `tools/call`
  arguments. The downstream `mcpLine` and error-surfacing code should not need
  to change. The same code works for the daemon hello field and direct-binary
  `main.go` path, which supplies `os.Stdin` as `hostedCLI.stdin`.
- Update the usage string to advertise both forms, e.g.
  `usage: ws-mcp call <name> '<json>'|-`.
- Error legibility: empty/whitespace-only stdin gets its own message (payload
  source named, e.g. "empty JSON payload on stdin"); malformed JSON keeps the
  existing `malformed JSON arguments: %w` wording so operators see the same
  diagnostic regardless of source.

Constraints:

- No new dependencies; `io.ReadAll(c.input())` is sufficient.
- Do not add a read timeout. A blocked read with no stdin attached is the
  standard unix behavior for `-` and should not be papered over.

Rejected alternative: adding a `--payload-file` / `--stdin` flag. It expands
the arg arity, forces flag parsing into a deliberately flag-free command, and
would break the `len(args) != 2` invariant that keeps this path trivial.

### Phase 2: capture stdin into the daemon CLI hello

The `ws-cli` / `wsflow-cli` shims delegate to the launcher, and product commands
run in the daemon. Phase 1 is reachable only if the launcher reads stdin for
`call <name> -` and puts the exact text in the CLI hello before connecting.

Anchors to verify:

- `agents-plugin/bin/ws-mcp-launcher.py:2424-2431` — extend `reads_stdin` to
  include exactly `call <name> -`, then read stdin once as UTF-8.
- `ws-mcp-launcher.py:2242-2248` — `proxy_cli` puts captured text in
  `hello["stdin"]`; preserve this one-shot transport and its no-replay rule.
- `agents-plugin-tool/internal/wsdashboard/proxy.go:54-64` and
  `cmd/ws-mcp/hosted_cli.go:40-46` — the optional hello field reaches
  `hostedCLI.input()`; older hellos and commands that do not read stdin remain
  unchanged.

Goals:

- Capture stdin only after daemon/runtime preparation, at the existing
  `reads_stdin` site, so preparation cannot drain the operator payload.
- Verify end to end with the host MCP connection down and the daemon available:
  launcher capture -> hello `stdin` -> daemon `hostedCLI.mcpCommand` ->
  `tools/call`. Also cover direct raw-binary invocation, which injects
  `os.Stdin` into the same `hostedCLI` core (`cmd/ws-mcp/main.go:50-55`).

Verification: exercise the target usage with a real write tool and a payload
containing a multi-line body with embedded double quotes, confirming the tool
receives the exact bytes.

Depends on Phase 1.

### Phase 3: optional `@path` file payload source

Secondary scope only — land it if it is cheap after Phases 1-2, otherwise mark
this phase `[dropped]` rather than renumbering.

- When the payload arg starts with `@`, read the payload from the named file
  path (`@-` is not special; prefer `-` for stdin).
- Reuse Phase 1's error shape, naming the file in the message so a missing or
  unreadable path is distinguishable from malformed JSON.
- Do not add glob, multiple-file, or directory handling.

Depends on Phase 1.

## Contract Impact

This repository has no spec layer. Before promotion to `ready/`, make the
caller-visible contract explicit in the phase verification and tests:
`ws-mcp call <name> -` reads JSON from stdin through the launcher CLI hello and
daemon-hosted `mcpCommand`, in addition to the existing inline JSON string.
