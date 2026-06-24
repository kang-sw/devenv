---
title: ws dashboard managed vendor CLI terminal
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260620-feat-ws-dashboard-agent-client-activity-sources: deferred structured Activity adapter track; this ticket provides the nearer terminal-first milestone
  260517-bug-ws-dashboard-windows-terminal-control-keys: existing Windows control-key risk for PTY-backed dashboard terminal surfaces
  260605-research-ws-native-subagent-pivot: ferrule/session-key and mercenary boundary context
related-mental-model:
  - ws-web-dashboard
  - mcp-runtime
sage-review: pending
---

# ws dashboard managed vendor CLI terminal

## Background

The dashboard's richer agent-client direction through Codex app-server and
OpenCode ACP remains useful, but that milestone is too far from a dogfoodable
interactive surface. The nearer accepted backlog is a terminal-first layer for
vendor agent CLIs: the dashboard daemon starts and owns PTY-backed Codex,
Claude, OpenCode, or similar CLI sessions, while the browser renders the output
as a terminal and adds a web-native prompt composition affordance.

The long-text input surface is a browser-side composer, not an editor embedded
inside the PTY. It may use a React/CodeMirror-style floating editor, snippets,
or bootstrap previews, then deliberately submits the final text to the daemon so
the daemon writes it to the managed CLI session's stdin. The PTY receives only
the final byte stream.

Structured Activity adapters for Codex app-server, OpenCode ACP, and later
provider protocols are intentionally deferred to
`260620-feat-ws-dashboard-agent-client-activity-sources`. This ticket should
not parse terminal output into source-neutral Activity as its success criterion.

## Decisions

- Add a distinct managed vendor CLI terminal surface, also acceptable as an
  "agent TUI pane" in UI vocabulary. It is separate from the existing pure shell
  terminal surface.
- Keep terminal output terminal-first. Xterm rendering is the primary display;
  output extraction is best-effort support for copy/search/export or later
  adapters, not a structured transcript contract.
- Commonize the painful PTY text I/O substrate once: spawn argv/env/cwd,
  output ring and cursoring, stdin write, resize, status, close/reap, WebSocket
  forwarding, and bounded fallback reads should be reusable by pure terminals
  and managed CLI terminals.
- Keep vendor differences in thin profiles: executable, args, environment,
  cwd behavior, submit policy, bootstrap text policy, and optional extraction
  hints. Do not build a universal agent protocol over ANSI output.
- Treat ferrule/bootstrap injection as explicit and auditable. Session keys and
  bootstrap text must not become browser route identity, Activity ids, command
  logs, or transcript metadata. A daemon-side `ws.ferrule` call belongs behind a
  later spec decision if it becomes necessary.
- Preserve the pure terminal contract: it stays a shell terminal substrate and
  does not gain Codex, Claude, OpenCode, bootstrap, or composer semantics.

## Constraints

- The dashboard may own PTY lifecycle and stdin delivery for managed CLI
  sessions, but it must not become the canonical ws MCP root, model loop,
  permission runtime, provider session authority, or ws agent runtime.
- Browser composer submission must be a deliberate action. Editing, IME
  composition, snippets, and previews happen in browser UI; no PTY-side editor
  or adapter editor is introduced.
- Long prompt submission must handle payloads larger than the existing terminal
  raw-input fallback cap. The implementation should define chunking, bracketed
  paste, plain paste, final-Enter, and CR/LF behavior per platform and profile
  instead of relying on accidental shell behavior.
- Text extraction starts from daemon-owned PTY chunks and sequence cursors.
  ANSI-stripped text, screen snapshots, or scrollback export may be added as
  bounded best-effort surfaces, but semantic turn/tool/activity parsing stays
  out of this ticket.
- Pure terminal tests must stay green and must prove that shell input fidelity,
  resize bounds, close behavior, and route identity did not change.

## Prior Art

- `ws-dashboard/crates/daemon/src/terminal.rs` already owns daemon-held PTY
  sessions, output ring cursoring, stdin writes, resize, close/reap, and
  terminal WebSocket forwarding.
- `ws-dashboard/frontend/src/terminals.ts` and `frontend/src/App.tsx` already
  model xterm-backed terminal panes, fallback HTTP reads, WebSocket transport,
  raw input, focus fallback, and bounded resize forwarding.
- `ai-docs/spec/ws-web-dashboard/index.md` explicitly describes pure terminal
  panes as shell terminal substrate only; this ticket adds a sibling managed
  surface rather than changing that contract.

## Phases

### Phase 1: Shared PTY text I/O substrate

Factor or wrap the existing terminal implementation so pure shell terminals and
managed CLI terminals can share the host-process PTY substrate without sharing
product semantics. The reusable layer should cover command spawn from explicit
argv/env/cwd, output ring storage with sequence cursors, stdin writes, resize,
status transitions, close/terminate/reap, WebSocket output/input/resize frames,
HTTP backfill, and bounded error handling.

This phase should preserve the existing pure terminal routes and frontend
behavior. It may add internal types or helpers, but no vendor CLI UI needs to be
visible yet.

Verification boundary: existing Rust route tests, frontend terminal tests, and
browser terminal acceptance should remain green. Add focused tests for the new
substrate boundary, including command profile spawn configuration, output
cursoring, oversize input handling, close/reap, and resize bounds.

### Phase 2: Managed vendor CLI terminal and browser composer

Add a separate managed CLI terminal resource and workbench surface. It should
spawn a configured vendor CLI in a daemon-owned PTY, render output through the
same terminal-first xterm path, and expose a browser-side floating composer for
long text. The composer must be ordinary web UI, not a PTY editor. Submitting the
composer writes the finalized text to the managed CLI session through an
explicit submit path.

The visible surface should be distinct from pure terminals in command ids,
surface kind, pane identity, title/icon treatment, close confirmation, and
restore policy. Overlay focus must suspend terminal refocus behavior while the
composer is open, and IME composition inside the composer must not leak partial
keystrokes to the PTY.

Verification boundary: route tests for create/list/socket/submit/resize/close
and auth; TypeScript tests for surface identity, composer open/submit/cancel,
focus isolation, and payload sizing; browser acceptance for a local installed
CLI profile when available, with a fixture or harmless command profile fallback
when the vendor binary is absent.

### Phase 3: Vendor profiles and bootstrap/submit policy

Define the first vendor profile contract for Codex, Claude, and OpenCode-style
CLIs. Each profile should declare executable resolution, args, environment,
working-directory behavior, startup health or missing-binary degradation,
long-text submit strategy, newline/final-Enter handling, optional bracketed
paste support, interrupt/close expectations, and bootstrap text policy.

Bootstrap/ferrule behavior starts as explicit visible text insertion or
submission. The policy may offer a "prepare ws bootstrap" action that composes
the workflow instructions and root-specific context for the user to inspect
before sending. Direct daemon-side `ws.ferrule` minting is not required in this
phase unless a separate spec decision authorizes the dashboard to hold and inject
that credential.

Verification boundary: profile fixture tests for command construction and
submit encoding, bounded missing-binary errors, no leakage of `session_key` or
host paths into browser-visible identities/loggable command payloads, and at
least one dogfood smoke for the primary Codex CLI path when available.
