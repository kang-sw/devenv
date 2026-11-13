---
title: "playbook.render with root_override splices a child key bound to the lead root"
---

# playbook.render with root_override splices a child key bound to the lead root

## Background

Dogfood 2026-11-13: the lead acquired a pooled worktree
(`worktree.acquire`, worker_key `dandruff-eggplant-satiable`, path
`.ws-worktrees/devenv/freebase-prominent-pronto`), then called
`playbook.render(name: ticket-worker, root_override: <that worktree>)` and
passed the rendered prompt to `pi.spawn(cwd: <that worktree>)`. The rendered
body carried a render-minted child key (`babied-alkalize-drizzly`) that the
Pi worker found delegate-scoped at the lead's root
(`D:\Workspace\Repos\devenv`): `git.status` with it reported `develop`, not
the worktree branch. The worker stopped (`stop: b`) before any write. It also
found `workflow_manual` and `route.resolve_implement` absent from a Pi
worker's tool list, while the ticket-worker playbook requires them.

## Idea

- Bind the render-minted key to `root_override` when it is given (the tool
  description says root_override drives child-key binding), or let render
  take the acquired worker_key so the body and the worktree lease agree.
- Decide whether a Pi worker gets the playbook-required tools or the
  playbook has a Pi path without them.

### Confirmed cause (read-only investigation, 2026-11-13)

`playbook.render` is not the mis-binding site. The handler binds the minted
key to `root_override` when it is set
(`agents-plugin-tool/internal/mcp/server.go:2243-2248`, `mintRoot =
rootOverride`; `playbook_tools.go:926-929`, `s.sessions.mint(mintRoot, ...)`),
and tests pin it (`playbook_render_surface_test.go:150-171`, `:195-226`,
`:273-276`). `resolveToolRoot` returns the key's recorded root unchanged
(`server.go:4074-4080`).

The lead-root binding comes from the Pi spawn path:

- `pi.spawn` passes `Root: entry.root` (the lead's root) and `Cwd` separately
  (`internal/mcp/pi_tools.go:294-310`); the conversation-held path does the
  same (`internal/wsagent/children.go:298-312`).
- The Pi child's own ws key is ensured against `rec.Root`, not its cwd
  (`internal/wsagent/runner.go:1848`, `Keys.Ensure(rec.Root, rec.WSKey)`), and
  its system prompt tells it to pass that key on every ws call
  (`runner.go:2320`). Its ws MCP process also starts with `cwd: spec.Root`
  (`internal/wsagent/pi.go:215`).
- So the worker holds two keys: the render-minted one in the prompt body
  (bound to the worktree) and its own system-prompt key (bound to the lead's
  root). `git.status` returning `develop` is what the lead-root key yields; the
  original report attributed it to the body key, which the code does not
  support. Not reproduced (no tests run); which key the worker used is
  inferred from the code, not observed.

The missing `workflow_manual` and `route.resolve_implement` in the Pi tool
list is a separate tool-exposure question, already tracked by
`261112-research-ws-hosted-pi-missing-workflow-tools`.

## Proposed Approach

Bind the Pi child's own session key (and its ws MCP cwd) to its cwd when the
cwd differs from the spawning root, so both keys the worker holds resolve to
the same worktree. Leave render's binding as is; only canonicalize
`root_override` before minting. Compatibility tier: T0 (no tool, argument,
store shape, or event change; a key record already carries an arbitrary
root). Record it in `## Decisions` when the queue settles.

## Decisions

- Fix the Pi child key's root binding in the Pi spawn path, not in
  `playbook.render`. Rejected: changing `playbook.render` - render already binds
  `root_override` (server.go:2243-2248, test-pinned); the lead-root key is the Pi
  worker's own, ensured against `rec.Root` (runner.go:1848).
- Bind the Pi child key to its cwd only when cwd differs from the spawning root.
  Rejected: always splitting Root from Cwd - keeps mail and config anchoring on
  the lead root unchanged and the diff small.
- Leave the Pi tool-list gap (`workflow_manual`, `route.resolve_implement`) to
  `261112-research-ws-hosted-pi-missing-workflow-tools`. Rejected: folding it
  here - it is a profile tool-exposure question with a different fix site and no
  link to key binding.
- Canonicalize `root_override` through `canonicalGitRoot` before minting.
  Rejected: storing it raw - a relative or symlinked path would otherwise bind
  whatever directory git resolves against the server cwd.
- Compatibility: T0 - no tool, argument, store-shape, or event change; a key
  record already carries an arbitrary root.

## Phases

### Phase 1: Bind Pi child key to its cwd

- Read `ai-docs/manuals/ws-mcp.md` and `release-compatibility.md` first.
- Ensure the Pi worker's key against `rec.workDir()` (`runner.go:1848`) when a
  cwd differs from `rec.Root`, and start its ws MCP with that cwd
  (`pi.go:215`); keep mail and config anchored on `rec.Root` unless the
  worker verifies they need the same change.
- Canonicalize `root_override` with `canonicalGitRoot` before the mint in
  `playbook.render`.
- Tests: a Pi spawn with a linked-worktree cwd yields a child key whose root
  and `git.status` resolve the worktree branch; render with a non-canonical
  `root_override` binds the canonical root.
