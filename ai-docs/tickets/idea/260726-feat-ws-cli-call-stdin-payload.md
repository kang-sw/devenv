---
title: "ws-cli call: accept JSON payload from stdin"
sage-review-design: required
related:
  260726-feat-ws-cli-lenient-tool-name-resolution: sibling CLI-fallback ergonomics ticket from the same MCP-down dogfooding session
---

# ws-cli call: accept JSON payload from stdin

## Background

During MCP-down recovery dogfooding, the CLI fallback path
(`ws-cli` -> `ws-mcp call`) proved workable for reads and bootstrap, but
complex root-aware **write** payloads are painful: the payload must be
inline-quoted in the shell. Multi-line bodies with embedded double quotes are
the concrete pain case — `git.commit` messages carrying a `## AI Context`
block, and `agenda.set` prose. Shell quoting of such payloads is error-prone
exactly when the operator is already in an outage and least able to absorb
extra failure modes.

Verified current behavior:

- `callCommand` at `agents-plugin-tool/cmd/ws-mcp/main.go:1000` hard-requires
  exactly two args: `if len(args) != 2` -> `usage: ws-mcp call <name> '<json>'`
  and `os.Exit(2)`.
- It then reads `rawArguments := args[1]` and `json.Unmarshal`s that string
  directly.
- There is **no** `os.Stdin` read and **no** `-` / `@file` sentinel handling on
  this path.

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
  same dogfooding session, also improving CLI-fallback ergonomics on the
  `ws-mcp call` path. Coordinate touch points in `callCommand` if both land
  near each other; do not fold the two scopes together.
- `agents-plugin/bin/ws-mcp-launcher.py:208` `run_binary` already demonstrates
  the pattern of feeding a payload to the Go binary over stdin
  (`input=input_text`), so the receiving side is the only missing half.

## Phases

### Phase 1: stdin payload source in callCommand

Teach `callCommand` (`agents-plugin-tool/cmd/ws-mcp/main.go:1000`) to resolve
its payload from stdin when `args[1] == "-"`.

Goals:

- Keep the `len(args) != 2` guard and the two-arg shape untouched.
- Read all of `os.Stdin` when the payload arg is exactly `-`, then feed the
  resulting bytes into the existing `json.Unmarshal` probe and
  `request.Params.Arguments` assignment. The downstream request-building,
  `runMCPLine`, and error-surfacing code should not need to change.
- Update the usage string to advertise both forms, e.g.
  `usage: ws-mcp call <name> '<json>'|-`.
- Error legibility: empty/whitespace-only stdin gets its own message (payload
  source named, e.g. "empty JSON payload on stdin"); malformed JSON keeps the
  existing `malformed JSON arguments: %w` wording so operators see the same
  diagnostic regardless of source.

Constraints:

- No new dependencies; `io.ReadAll(os.Stdin)` is sufficient.
- Do not add a read timeout. A blocked read with no stdin attached is the
  standard unix behavior for `-` and should not be papered over.

Rejected alternative: adding a `--payload-file` / `--stdin` flag. It expands
the arg arity, forces flag parsing into a deliberately flag-free command, and
would break the `len(args) != 2` invariant that keeps this path trivial.

### Phase 2: verify stdin pass-through across the shim and launcher

The `ws-cli` / `wsflow-cli` bin shims delegate through
`ws-mcp-launcher.py tools` / `call`, so Phase 1 is only reachable if stdin
survives that hop. Treat this as an explicit phase concern, not an assumption —
it is the real risk point.

Anchors to verify:

- `agents-plugin/bin/ws-cli:6` — `exec python3 "$script_dir/ws-mcp-launcher.py" "$@"`.
  `exec` preserves fd 0; confirm no shim variant wraps or redirects stdin.
- `agents-plugin/bin/ws-mcp-launcher.py:900` — `os.execvpe(str(binary), args, os.environ)`
  on POSIX; `:896` `subprocess.call(args)` on Windows. Both should inherit fd 0
  unchanged.
- **Probe subprocesses that run before the handoff and inherit fd 0 because
  `run_binary` is called without `input_text`, leaving `subprocess.run(input=None)`
  to inherit the launcher's stdin**: `ws-mcp-launcher.py:272` (commands
  compatibility), `:680` (`runtime capabilities`), `:719` (`version`). If any
  probe reads stdin, the heredoc payload is drained before the `execvpe`
  handoff and Phase 1 silently receives an empty payload. Also check
  `:599` (local `go build`, stdin inherited, no capture) on the
  local-devenv rebuild path.

Goals:

- Determine whether any pre-handoff subprocess can consume stdin; if so, pass
  an explicit empty stdin (`input=""` or `stdin=subprocess.DEVNULL`) to the
  probes so the operator payload is preserved for the real invocation.
- Verify end to end on both the cached-runtime fast path and a path that
  triggers install/repair or a local-devenv rebuild, since those paths run more
  probes before the handoff.

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

## Spec Impact

Not required at `todo/`. Before this ticket is promoted to `ready/`, address the
caller-visible CLI contract change (payload sources for `ws-mcp call`) against
the MCP/plugin-runtime spec area — `ai-docs/spec/mcp-tools.md` or
`ai-docs/spec/plugin-runtime.md`, whichever owns the `ws-mcp` command surface.
Expected caller-visible change: `ws-mcp call <name>` accepts `-` as the payload
argument to read JSON from stdin, in addition to the existing inline JSON
string. Contract-first spec: no.
