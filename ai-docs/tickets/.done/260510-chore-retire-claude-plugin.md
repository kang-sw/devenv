---
title: retire frozen claude plugin fallback
related:
  260429-research-host-neutral-ws-plugin: host-neutral plugin architecture now treats claude-plugin as frozen legacy fallback
spec:
  - 260510-claude-plugin-retirement-freeze
related-mental-model:
  - claude-compatibility
  - workflow-skills
completed: 2026-05-11
---

# retire frozen claude plugin fallback

## Background

`claude-plugin/` is no longer an active mirror target for Codex-first workflow
changes. Keeping it as a parallel edit surface doubles skill and prompt updates
without improving the current Codex/MCP workflow.

The prior freeze-before-retire policy is complete enough to retire the source
tree. `infra.read` has been fixed to read bundled runtime infra docs, so active
Codex/Agents workflows no longer require downstream projects or plugin installs
to carry `claude-plugin/`.

This ticket removes the frozen legacy tree and updates installer, runtime, and
documentation references so `agents-plugin/` and `agents-plugin-tool/` are the
active migration surface.

## Decisions

- Treat `claude-plugin/` as retired source, not frozen fallback.
- Do not keep Claude-only skills, prompts, bins, hooks, or plugin manifests in
  the live tree.
- Keep root `CLAUDE.md` only as the `@AGENTS.md` compatibility shim.
- Preserve the former global Claude home instructions as a historical reference
  under `ai-docs/ref/`, not as installer-managed live config.
- Preserve historical references under `ai-docs/ref/old-spec/`; they are not live
  dependencies.

## Phases

### Phase 1: Remove installer and runtime dependencies

Remove live references that still require the legacy tree:

- stop linking `claude-plugin/CLAUDE.home.md` from `install.sh`;
- stop linking `claude-plugin/hooks/*.sh` from `install.sh`;
- remove stale installer comments that describe copying `claude-plugin/`;
- remove runtime health checks that require a `claude-plugin/` directory;
- preserve the former `CLAUDE.home.md` text as a historical reference under
  `ai-docs/ref/`.

Success criteria:
- `install.sh` can run without a repository `claude-plugin/` directory.
- `ws-mcp doctor` no longer requires `claude-plugin/`.
- The preserved Claude home text is clearly marked historical, not installed.

### Result (8ad181b) - 2026-05-11

Removed installer links to `claude-plugin/CLAUDE.home.md` and
`claude-plugin/hooks/*.sh`, added cleanup for old repo-owned Claude home and
hook symlinks, and stopped `ws-mcp doctor` from requiring a `claude-plugin/`
directory. The former Claude home instructions moved to
`ai-docs/ref/claude-home-legacy.md` as historical reference.

### Phase 2: Delete legacy source and update active docs

Delete the `claude-plugin/` tree. Update active repository guidance to describe
`agents-plugin/` plus `agents-plugin-tool/` as the supported plugin/runtime
surface, with `CLAUDE.md` limited to the root shim.

Success criteria:
- `AGENTS.md`, `README.md`, `ai-docs/_index.md`, ship docs, specs, and mental
  models no longer describe `claude-plugin/` as a live fallback.
- Active workflow docs no longer instruct agents to read or edit
  `claude-plugin/` for normal work.
- Historical references remain only where they explain prior migration context.

### Result (8ad181b) - 2026-05-11

Deleted the `claude-plugin/` source tree, including legacy skills, prompts,
infra docs, bins, hooks, and manifest. Updated root guidance, README, specs,
mental models, ship docs, and MCP references so `agents-plugin/` plus
`agents-plugin-tool/` are the active plugin/runtime surface and root
`CLAUDE.md` remains only an `@AGENTS.md` shim.

### Phase 3: Verify retirement

Run the retirement verification set:

Success criteria:
- `rg claude-plugin` outside `ai-docs/ref/old-spec/` returns only deliberate
  historical, changelog, or removal notes.
- `go test ./...` passes under `agents-plugin-tool/`.
- `ws/spec_index.verify` passes.
- Downstream simulation smoke passes: a temp Git root with only
  `ai-docs/_index.md` can call `infra.read("executor-wrapup")` through
  `ws-mcp serve --stdio` without a `claude-plugin/` directory.

### Result (8ad181b) - 2026-05-11

Verification passed: `bash -n install.sh`, `go test ./...` under
`agents-plugin-tool/`, `ws-mcp doctor --root /home/swkang/devenv`,
`ws/spec_index.verify`, downstream-root `infra.read("executor-wrapup")` through
`ws-mcp serve --stdio`, and temp-HOME `install.sh update` without a repository
`claude-plugin/` directory.
