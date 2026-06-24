---
title: ws dashboard managed vendor CLI terminal
parent: 260622-epic-ws-dashboard-session-key-realignment
related:
  260514-epic-ws-web-dashboard-mvp: predecessor dashboard MVP board whose reusable PTY/workbench surface this ticket extends
  260525-feat-ws-dashboard-server-scoped-operation-forwarding: server-scoped route and identity prerequisite for new terminal-like operations
  260622-research-ws-dashboard-ferrule-session-binding: settled ferrule-backed top-level harness binding model this ticket must follow
  260620-feat-ws-dashboard-agent-client-activity-sources: deferred structured Activity adapter track; this ticket provides the nearer terminal-first milestone
  260517-bug-ws-dashboard-windows-terminal-control-keys: existing Windows control-key risk for PTY-backed dashboard terminal surfaces
  260605-research-ws-native-subagent-pivot: ferrule/session-key and mercenary boundary context
  260624-feat-ws-dashboard-managed-cli-recent-sessions: deferred vendor-history-backed recent sessions follow-up
related-mental-model:
  - ws-web-dashboard
  - mcp-runtime
sage-review: pending
---

# ws dashboard managed vendor CLI terminal

## Background

The dashboard session-key realignment epic keeps the browser dashboard but moves
agent and harness integration onto ferrule-backed, daemon-private session
bindings. The richer agent-client direction through Codex app-server and OpenCode
ACP remains useful, but that milestone is too far from a dogfoodable interactive
surface. The nearer accepted backlog is a terminal-first layer for vendor agent
CLIs: the dashboard daemon starts and owns PTY-backed Codex, Claude, OpenCode, or
similar CLI sessions, while the browser renders the output as a terminal and adds
a web-native prompt composition affordance.

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
- Model the surface as a new workbench surface kind, not as a titled variant of
  `persistentTerminal`. Shared PTY process I/O should be factored below the
  surface boundary, while composer, vendor profile, bootstrap, restore prompt,
  and Activity behavior remain managed-CLI semantics.
- Managed CLI panes are primary work surfaces. They should use pinned primary
  placement like terminal/agent panes rather than support/opened placement.
- Keep terminal output terminal-first. Xterm rendering is the primary display;
  transcript/search/export extraction is out of scope for the first milestone.
  Later UI may recognize visible ticket/spec stems in terminal text and route to
  those project artifacts, but it must not introduce semantic turn/tool/activity
  parsing in this ticket.
- Commonize the painful PTY text I/O substrate once: spawn argv/env/cwd,
  output ring and cursoring, stdin write, resize, status, close/reap, WebSocket
  forwarding, and bounded fallback reads should be reusable by pure terminals
  and managed CLI terminals.
- Keep vendor differences in thin profiles: executable, args, environment,
  cwd behavior, submit policy, bootstrap text policy, and optional extraction
  hints. Do not build a universal agent protocol over ANSI output.
- Treat ferrule/bootstrap injection as explicit and auditable. Top-level
  dashboard-launched harness sessions follow
  `260622-research-ws-dashboard-ferrule-session-binding`: the target daemon may
  call `ws.ferrule(root)`, store the returned `wsSessionKey` only in
  daemon-private binding state, emulate the `lead-workflow-manual` rendered
  context plus ferrule result as a system-prompt-style launch context, and inject
  that context into the managed CLI through the selected vendor profile. Session
  keys and bootstrap text must not become browser route identity, Activity ids,
  command logs, transcript metadata, pane titles, tooltips, localStorage restore
  descriptors, or other ordinary browser chrome/state.
- The composer opens from a dashboard command surface, with `Ctrl+G` as the
  intended keyboard affordance for managed CLI panes. Pure shell terminals keep
  their raw terminal input contract and do not gain managed CLI hotkey semantics.
- Composer editing is paste-only by default: `Enter` inserts a newline inside
  the browser editor, `Ctrl+Enter` submits the finalized draft to the daemon, and
  submit writes text to the CLI prompt without sending a final Enter unless a
  profile explicitly opts into that behavior.
- Closing a managed CLI pane terminates the vendor CLI process. Browser refresh
  may reattach to daemon-live sessions, but daemon restart should show a restore
  prompt in the remembered pane instead of silently relaunching the vendor CLI.
- Opening a managed CLI starts from a visible agent affordance adjacent to the
  existing terminal action. The action opens a profile/context chooser; advanced
  profile or start-parameter edits can sit behind a gear/settings affordance.
- Managed CLI sessions stay separate from the Activity Console for the first
  implementation. Future Activity provider adapters are tracked by
  `260620-feat-ws-dashboard-agent-client-activity-sources`.
- Preserve the pure terminal contract: it stays a shell terminal substrate and
  does not gain Codex, Claude, OpenCode, bootstrap, or composer semantics.

## Constraints

- The dashboard may own PTY lifecycle, stdin delivery, and daemon-private
  session binding for managed CLI sessions, but it must not become the canonical
  ws MCP root, model loop, permission runtime, provider session authority, or ws
  agent runtime.
- New managed CLI route helpers, pane keys, command payloads, restore records,
  WebSocket URLs, and daemon APIs must carry `serverId` or explicitly remain a
  `server-local` compatibility alias. Do not add new bare local-only API debt;
  if `260525-feat-ws-dashboard-server-scoped-operation-forwarding` has not
  landed the needed route substrate, this ticket should either depend on that
  phase or introduce server-scoped shapes from the start.
- Browser composer submission must be a deliberate action. Editing, IME
  composition, snippets, and previews happen in browser UI; no PTY-side editor
  or adapter editor is introduced.
- While the composer is open, terminal refocus, fallback raw-key forwarding, and
  xterm input should be suspended for that pane. PTY output may continue to
  render behind or beneath the overlay.
- Long prompt submission must handle payloads larger than the existing terminal
  raw-input fallback cap. The implementation should define chunking, bracketed
  paste, plain paste, final-Enter, and CR/LF behavior per platform and profile
  instead of relying on accidental shell behavior.
- Text extraction beyond visible stem recognition is out of scope. Claude and
  other vendor CLIs may render full TUIs where scrollback parsing is unreliable;
  do not promise copy/search/export/transcript behavior until a later ticket
  defines a bounded extraction source.
- Pure terminal tests must stay green and must prove that shell input fidelity,
  resize bounds, close behavior, and route identity did not change.

## Deferred Follow-Ups

- Vendor-history-backed "recent sessions" browsing is tracked by
  `260624-feat-ws-dashboard-managed-cli-recent-sessions`. It should parse vendor
  history stores later to show a flat list across workspaces, Git roots, and
  worktrees, but it is not part of the first managed CLI terminal milestone.

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
surface kind, pane identity, title/icon treatment, primary pinned placement,
close confirmation, and restore policy. Overlay focus must suspend terminal
refocus behavior while the composer is open, and IME composition inside the
composer must not leak partial keystrokes to the PTY.

The first visible launch affordance should sit next to the existing terminal
action and open a profile/context chooser. The composer should be invokable from
the managed CLI pane through a dashboard command and the intended `Ctrl+G`
keyboard path. In the composer, `Enter` inserts a newline, `Ctrl+Enter` submits,
and submit defaults to paste-only without a final Enter. Closing the pane
terminates the CLI process; after daemon restart, a remembered managed CLI pane
should render a restore prompt such as `Restore conversation <title>?` with
explicit yes/no actions instead of silently relaunching.

Verification boundary: route tests for create/list/socket/submit/resize/close
and auth; TypeScript tests for surface identity, composer open/submit/cancel,
focus isolation, payload sizing, primary placement, restore prompt behavior, and
server-scoped route/helper identity; browser acceptance for a local installed CLI
profile when available, with a fixture or harmless command profile fallback when
the vendor binary is absent.

### Phase 3: Vendor profiles and bootstrap/submit policy

Define the first vendor profile contract for Codex, Claude, and OpenCode-style
CLIs. Each profile should declare executable resolution, args, environment,
working-directory behavior, startup health or missing-binary degradation,
long-text submit strategy, newline/final-Enter handling, optional bracketed
paste support, interrupt/close expectations, and bootstrap text policy.

Bootstrap/ferrule behavior starts as explicit, auditable profile policy governed
by the session-key realignment model. For a top-level dashboard-launched harness
CLI, the target daemon may prepare the workflow instructions by emulating the
`lead-workflow-manual` result, call `ws.ferrule(root)` in its local ws
environment, store the resulting `wsSessionKey` only in daemon-private binding
state, and inject the approved launch/bootstrap context as a
system-prompt-style context for the selected CLI. Profiles that do not support ws
bootstrap yet must degrade explicitly rather than silently starting with an
untracked authority model.

Verification boundary: profile fixture tests for command construction and
submit encoding, bounded missing-binary errors, no leakage of `session_key`,
bootstrap text, provider session ids, vendor history paths, or host paths into
browser-visible identities, pane metadata, restore records, or loggable command
payloads, and at least one dogfood smoke for the primary Codex CLI path when
available.
