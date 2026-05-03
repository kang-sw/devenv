---
title: agents-plugin agent session runtime
parent: 260503-epic-agents-plugin-skill-porting
related:
  260425-feat-ws-agent-registry-compression: Claude-side named-agent registry and compression prior art
  260503-feat-agents-plugin-runtime-boundary: Go MCP/runtime foundation for host-neutral helper surfaces
  260503-epic-agents-plugin-skill-porting: parent roadmap; this ticket must precede write-skeleton and core orchestration ports
---

# agents-plugin agent session runtime

## Background

The next `agents-plugin` skill-porting slice cannot safely start with
`write-skeleton`. That skill is the first production workflow where the lead is
only a coordinator and a delegate owns design, file edits, and amendment rounds.
The Claude source depends on `ws-new-named-agent`, `ws-call-named-agent`,
`ws-oneshot-agent`, outbox interrupts, output files, and a registry under
`.git/ws@<repo>/agents/`.

The migration target is not a literal clone of Claude's named-agent scripts. The
shared contract should be a host-neutral sustainable session runtime built around
the common backend shape:

```text
<cli> <resume-command> <session-id> <new-prompt>
```

Claude, Codex, and future adapters such as Gemini should be backend
implementations behind the same project-local session registry and prompt
contract. The current Python implementation and its git history remain the
primary prior art for scope and trade-offs, but the durable runtime should move
toward Go so Windows and company deployments do not depend on a user-installed
Python runtime.

## Decisions

- Build the reusable project-state/path management layer first in
  `agents-plugin-tool/`. All later agent registry, review path, message queue,
  lock, session, and temporary-file logic should sit on top of that layer instead
  of inventing per-feature path rules.
- Use a single cache root for workflow state, defaulting to
  `~/.cache/ws@kang-sw-devenv/`, with an override environment variable reserved
  for tests and nonstandard deployments.
- Do not key project identity by remote origin. Local checkout identity is more
  stable for this workflow than remote identity because repos may be local-only,
  forked, mirrored, or have remotes changed.
- Compute project identity from the canonical root repository absolute path and a
  short hash. Linked worktrees share that root identity and receive a separate
  worktree identity/suffix.
- Store metadata alongside hashed paths so future tools can list active projects,
  worktrees, and agents by scanning the cache root.
- Treat accumulated text state as acceptable. Cleanup should be explicit
  (`erase`, later `gc`), not an implicit startup behavior.
- Generalize Claude model tiers out of shared skill text. The shared layer should
  use workload-depth tiers such as `light`, `core`, and `deep`; backend adapters
  map those tiers to concrete model names.

## Constraints

- The runtime must not require Python, Node, Cargo, Visual Studio Build Tools, or a
  shell helper on target user machines.
- `claude-plugin/` stays the stable Claude package while this ticket builds the
  host-neutral runtime.
- Existing Claude `ws-named-agent` behavior is prior art, not a compatibility
  promise for every file path or internal JSON field.
- Compression is optional capability in the shared contract. Claude's 120K
  auto-compression can remain Claude-only until another backend has a verified
  equivalent.
- Codex native subagent tools may become a backend fast path, but the first shared
  contract should be durable CLI-backed sessions because they generalize across
  hosts.

## Phases

### Phase 1: Project state path manager

Add Go path-management primitives under `agents-plugin-tool/` for workflow state.
The first implementation should define and test:

- cache root resolution, including the default
  `~/.cache/ws@kang-sw-devenv/` and an override for tests
- canonical git root detection
- linked-worktree root identity detection, using the common/root repository path
  as the root hash input
- project key format:
  `sha256(canonical-root-path)[:12]-<repo-basename>`
- worktree key format:
  `sha256(canonical-worktree-path)[:12]-<worktree-basename>`
- metadata files such as `project.json` and `worktree.json` with
  `schema_version`, canonical paths, ids, display names, and `last_seen_at`
- directory layout that separates root-shared state from worktree-local state

Suggested layout:

```text
~/.cache/ws@kang-sw-devenv/
  projects/
    <project-key>/
      project.json
      shared/
        locks/
      worktrees/
        <worktree-key>/
          worktree.json
          agents/
          review-paths/
          sessions/
          tmp/
```

Success criteria:

- Unit tests cover normal repos, no-origin repos, and linked worktree path
  identity.
- No code path writes runtime state under `.git/ws@...` for the new Go surface.
- The path manager can return stable locations for `agents`, `review-paths`,
  `sessions`, `locks`, and `tmp`.

### Result (d87e9f2) - 2026-05-03

Added `agents-plugin-tool/internal/wsstate` as the reusable project-state path
manager for future workflow runtime features. The package resolves the workflow
cache root, detects the canonical worktree root and common repository root through
git, derives hash-based project and worktree keys, and returns stable directories
for root-shared locks plus worktree-local agents, review paths, sessions, and
temporary files.

The implementation writes `project.json` and `worktree.json` metadata with schema
version, canonical paths, ids, display names, and `created_at`/`last_seen_at`
timestamps. Repeated `Ensure` calls preserve `created_at` and update
`last_seen_at`, so future active-project and active-agent tooling can scan the
cache root without relying on human-readable path reconstruction.

Tests cover normal repositories, repositories with no origin remote, linked
worktree identity sharing, worktree-local state separation, cache-root override
precedence, metadata timestamp behavior, and readable key sanitization. The new Go
surface does not write under `.git/ws@...`.

Verification:

- `go test ./...` from `agents-plugin-tool/`

### Phase 2: Host-neutral agent session contract

Extract the shared session surface from the current `ws-named-agent` behavior and
document it before implementing backend-specific behavior.

The contract should cover:

- `register` / `new`: named session registration, role prompt composition, tier
  selection, and metadata initialization
- `call`: resume-or-create call routing through a backend adapter
- `oneshot`: register, call, and erase lifecycle for nonpersistent delegates
- `interrupt`: queued follow-up message semantics
- `print` and `tail`: last output and session observability
- `erase`: registry and session cleanup
- status fields needed for future "active agent list" tooling
- plain-text response as the default caller-facing output contract

Success criteria:

- The contract references current Claude prior art in
  `ai-docs/spec/workflow-skills.md` and `claude-plugin/bin/ws-named-agent`.
- The contract avoids Claude-specific model names, hook names, cache paths, and
  session file formats in shared terminology.
- The contract identifies which capabilities are required for `write-skeleton`
  and which can wait for core orchestration skills.

### Result (93fccb6) - 2026-05-03

Added `ai-docs/ref/ws-agent-runtime.md` as the host-neutral session contract for
future agent orchestration work. The document extracts the reusable pattern from
the current Claude `ws-named-agent` implementation while avoiding Claude-specific
paths, model names, hook names, and session file formats in the shared contract.

The contract defines:

- `ws/<tool-name>` notation for agent MCP tools, such as `ws/agents.call`
- the cache-backed project/worktree state layout from Phase 1
- per-agent directories under `agents/<agent-name>/`
- `agent.json` registry metadata for active-agent lookup
- `inbox/` lead-to-agent message queues for interrupts, amendments, and queued
  prompts
- `outbox/` as a reserved agent-to-lead queue surface
- `output.md` and `events.jsonl` as plain-text output and lifecycle log files
- workload-depth tiers `light`, `core`, and `deep`
- planned tools `agents.register`, `agents.call`, `agents.oneshot`,
  `agents.interrupt`, `agents.print`, `agents.tail`, `agents.erase`, and
  `agents.list`

The first implementation subset is deliberately smaller than the full planned
surface: `agents.register`, `agents.call`, `agents.oneshot`, `agents.print`, and
`agents.erase` are enough to prove resume-backed delegation for `write-skeleton`.
Interrupts, tailing, active-agent listing, background execution, compression, and
agent-to-lead outbox delivery are deferred until later consumers need them.

### Phase 3: Backend adapter prototype

Implement the smallest adapter-backed session prototype needed to prove the
contract on this machine.

Minimum target:

- Codex CLI backend using `codex exec` and `codex exec resume <thread-id>` or the
  current verified equivalent.
- Registry metadata stores the backend name, workload tier, concrete model when
  known, session id, last call time, last output path, and status.
- Calls read role prompts from the plugin/runtime surface rather than from
  `claude-plugin/infra/prompts/` paths.
- Output and queued messages are stored under the Phase 1 worktree-local state
  directory.

Success criteria:

- A local smoke can register a named Codex-backed agent, call it, call it again
  through resume, print its last output, and erase it.
- A one-shot delegate call creates no persistent registry entry after cleanup.
- Failure diagnostics go to stderr and do not corrupt any future MCP stdout
  contract.

### Result (ea9042a) - 2026-05-03

Added `agents-plugin-tool/internal/wsagent` and a `ws-mcp agents ...` CLI
prototype for the minimum durable session subset. The implementation creates
worktree-local agent directories on top of the Phase 1 `wsstate` path manager,
stores `agent.json` registry metadata, writes optional materialized `system.md`
prompts, appends `events.jsonl`, persists the latest plain-text response in
`output.md`, and erases temporary agent directories explicitly.

The prototype supports:

- `ws-mcp agents register`
- `ws-mcp agents call`
- `ws-mcp agents oneshot`
- `ws-mcp agents print`
- `ws-mcp agents erase`

The Codex backend starts sessions with `codex exec --json` and resumes them with
`codex exec resume --json <thread-id>`, parsing `thread.started.thread_id` and
the last `agent_message` from JSONL stdout. A local smoke found that
`codex exec resume` does not accept `--cd`, so the adapter sets the subprocess
working directory instead of relying on a command-line cwd flag.

This phase deliberately does not publish MCP `agents.*` tools yet. Keeping the
first prototype as a CLI surface avoids accidental stdout contamination in
`serve --stdio` while the backend adapter and state files are still being tested.
Because the plugin launcher previously checked only MCP `tools/list`, this phase
also records the CLI command surface in `agents-plugin/runtime.json` and teaches
the launcher to repair stale cache-local binaries when `ws-mcp agents` subcommands
are missing.

Verification:

- `go test ./...` from `agents-plugin-tool/`
- local Codex CLI smoke: register → call → resume call → print → erase with an
  isolated `WS_CACHE_HOME`
- local Codex CLI oneshot smoke with an isolated `WS_CACHE_HOME`
- `scripts/smoke-ws-mcp.sh ..` from `agents-plugin-tool/`
- `sh -n agents-plugin/bin/ws-mcp-launcher`
- `jq . agents-plugin/runtime.json`
- launcher smoke with a temporary `WS_MCP_RUNTIME_DIR`
- `claude plugin validate agents-plugin`
- `git diff --check`

### Phase 4: First consumer skill

Port `write-skeleton` only after the session runtime has enough contract surface
for delegate ownership and amendment rounds.

Success criteria:

- `agents-plugin/skills/write-skeleton/SKILL.md` uses the shared agent session
  runtime rather than naming Claude `ws-new-named-agent` or Codex native
  `spawn_agent` directly.
- The port preserves the source skill's core flow: lead identifies contract
  directives, delegate owns skeleton design, lead reviews, lead commits.
- The ticket records which session-runtime gaps remain before `write-code`,
  `edit`, `implement`, `proceed`, and `sprint` can be ported.
