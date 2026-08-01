---
title: Orca plugin surfacing the ws workflow board and TODO state
related:
  260730-research-ws-dashboard-drop-for-orca: origin — dropping ws-dashboard leaves exactly this gap
---

# Orca plugin surfacing the ws workflow board and TODO state

## Background

Dropping ws-dashboard (`260730-research-ws-dashboard-drop-for-orca`) hands the
generic ADE shell to Orca. Orca covers terminals, worktrees, git, documents, and
agent attention natively, and ws/wsflow keeps working unchanged inside its
terminals. One capability has no replacement: **visual ws workflow state** —
the ticket/spec board, TODO list, and running session identity.

The intent is an Orca plugin that reads ws workflow artifacts from the
filesystem and renders them as a board, and if reachable, retrieves the live
session key to display the current TODO list.

This ticket records the feasibility survey. It does not commit to building.

## Feasibility Findings

Read 2026-07-30 against `stablyai/orca` at `main`. Orca is MIT, so these are
source facts, not documentation claims.

**The plugin worker is unsandboxed plain Node.** `src/main/plugins/plugin-host-process.ts`
forks with `execArgv: []` and no Node permission-model flags;
`plugin-host-runtime.ts` loads the entry with a plain dynamic `import()`; there
is no `vm`, SES, or lockdown anywhere in the path. `src/main/plugins/plugin-worker-env.ts`
scrubs the environment to an allowlist (`PATH`, `HOME`, locale, Windows
essentials) but that is secret hygiene, not isolation. **`node:fs` works.** The
capability model gates `orca.host.call(...)` — access to *Orca's own state* —
not the operating system.

**The panel is fully network-isolated.** `src/shared/plugins/plugin-panel-shell.ts`
prepends a CSP to every panel document:

```
default-src 'none'; connect-src 'none'; script-src 'unsafe-inline'; ...
```

A panel can never reach a local daemon or any network endpoint. A second CSP
meta from plugin content can only tighten, never loosen.

**Panels cannot call their own worker.** `src/shared/plugins/plugin-panel-bridge.ts`
admits only host API v0 actions (`isPluginPanelAction`). There is no
panel-to-worker command path.

**Capability set v0 is a closed enum** (`src/shared/plugins/plugin-capabilities.ts`):

```
workspace:read · terminal:send · notifications:show · storage · secrets
events:subscribe · settings:own
```

Relevant host API v0 shapes (`src/shared/plugins/plugin-host-api.ts`):

- `workspace.readContext` → `{ branch, displayName, terminals: [{ id }] }` of
  the **focused** worktree, nullable.
- `terminal.sendText` → `{ terminalId, text, enter }`; explicitly never "the
  active terminal", so a focus change cannot redirect a delayed write.
- `storage.get/set/delete/keys` → 256 KB per value, 5 MB total, 1024 keys.

Manifest shape (`examples/plugins/hello-orca/orca-plugin.json`): `manifestVersion: 1`,
`pluginApi: 1`, `engines.orca`, `main`, and `contributes` with `panels`,
`commands`, and `events` (`worktree.created`, `agent.status.changed`).

## Workable Architecture

Given the above, the only shape that works today:

```
worker (unsandboxed Node)  read ai-docs/tickets/** and spec/** via node:fs
                           → project to JSON → storage.set
panel  (sandboxed HTML)    storage.get → render board (poll; no push path)
commands                   worker registers → Orca command palette + keybindings
events                     worktree.created/removed, agent.status.changed → refresh
notifications              attention cue
```

Session key and TODO retrieval ride the same worker path — being unsandboxed
Node, it can read wsstate, spawn a ws CLI, or call a local endpoint.

## Blocking Friction

**`workspace:read` returns no worktree path.** It gives `branch`,
`displayName`, and terminal ids only. The worker can read any file but cannot
learn *which directory* to read. Two workarounds, neither clean:

- `settings:own` with a user-configured root — honest and stable, but manual
  per project.
- Reading Orca's private on-disk state to resolve the focused worktree — this
  is **Hack tier** in the sense used by the dashboard harness research
  (undocumented private state, no stability contract).

This is the single largest obstacle and the first thing a spike should measure.

## Phase 3 Reconciliation

- The loss of compact, fork, steer, and goal control applies to Orca-owned
  sessions only. For ws-owned Codex app-server sessions, those capabilities are
  Passthrough, so the earlier "permanently given up" claim was over-scoped.
- The `storage` limits — 256 KB per value and 5 MB total — cannot hold a board
  of roughly 100 live tickets together with its `related:` graph in one value.
- `contributes.events` has no filesystem-change event. Worker re-projection and
  panel refresh timing are therefore unspecified, and may block a useful board
  more directly than worktree-path discovery.
- For one to three personal projects, `settings:own` with a manually configured
  root is adequate. The Hack-tier approach of reading Orca's private state is
  avoidable.

## Risk

The worker's lack of sandboxing is a current implementation fact, not a
contract. `plugin-capabilities.ts` states v0 is deliberately a closed set and
announces `net:fetch` hosts and `process:exec` globs as future *scoped* kinds —
so the intent to gate this surface already exists. A plugin built on raw
`node:fs` depends on a hole the vendor may close.

`plugin-host-api.ts` further marks the surface `EXPERIMENTAL: additive-only
within pluginApi major 1 once frozen; no stability promises before then.`

## Proposed First Step

A half-day spike, scoped to answering the friction question rather than
producing a usable plugin: worker reads `ai-docs/tickets/**` via `node:fs`,
projects to JSON, `storage.set`; panel renders a board from `storage.get`. The
deliverable is a verdict on how the worktree-path problem behaves in practice,
not a shipped board.

## Upstream Ask

Not yet filed, and not yet decided. The candidate ask is that `workspace:read`
expose the focused worktree's path, or that a scoped filesystem capability be
added. Either would move the whole approach off Hack tier onto a supported
path, which is why it is recorded here rather than left to be rediscovered.
