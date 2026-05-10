---
title: retire frozen claude plugin fallback
related:
  260429-research-host-neutral-ws-plugin: host-neutral plugin architecture now treats claude-plugin as frozen legacy fallback
spec:
  - 260510-claude-plugin-retirement-freeze
related-mental-model:
  - claude-compatibility
  - workflow-skills
---

# retire frozen claude plugin fallback

## Background

`claude-plugin/` is no longer an active mirror target for Codex-first workflow
changes. Keeping it as a parallel edit surface doubles skill and prompt updates
without improving the current Codex/MCP workflow.

The immediate policy is freeze-before-retire: ordinary workflow edits should
target `agents-plugin/`, `agents-plugin-tool/`, embedded runtime prompts, specs,
and mental models. `claude-plugin/` should be touched only by explicit Claude
compatibility or retirement work.

This ticket tracks the later removal path. It should not delete the directory
until live installer, fallback, docs, tests, and runtime references have either
migrated or been intentionally retired.

## Decisions

- Treat `claude-plugin/` as frozen legacy fallback, not active source.
- Do not mirror `lead-*` skill semantics into `claude-plugin/skills/`.
- Keep legacy CLI and installer behavior only until explicit phases migrate or
  remove each caller surface.
- Preserve historical references under `ai-docs/ref/old-spec/`; they are not live
  dependencies.

## Phases

### Phase 1: Inventory live dependencies

Classify every non-historical `claude-plugin/` reference as one of:

- installer or local Claude home setup;
- legacy CLI fallback;
- convention or infra fallback;
- embedded prompt source or runtime fixture;
- documentation-only mention;
- removable stale reference.

Success criteria:
- The inventory distinguishes live runtime dependencies from old-spec history.
- Each live dependency has a target disposition: migrate, freeze, or remove.

### Phase 2: Migrate or retire live fallback surfaces

Move any still-needed behavior to `agents-plugin/`, `agents-plugin-tool/`, MCP
tools, embedded convention docs, or generated runtime metadata. Remove or mark
unsupported the remaining Claude-only installation and CLI paths.

Success criteria:
- `install.sh` no longer needs to snapshot `claude-plugin/` unless Claude support
  is intentionally kept.
- `agents-plugin-tool` tests and runtime docs no longer require live
  `claude-plugin/` source paths except explicit legacy fixtures.
- Active workflow docs no longer instruct agents to read or edit
  `claude-plugin/` for normal work.

### Phase 3: Remove the legacy tree

Delete `claude-plugin/` only after Phase 2 closes all live dependencies. Update
AGENTS, project memory, specs, mental models, ship docs, and installer docs in
the same change.

Success criteria:
- `rg claude-plugin` outside `ai-docs/ref/old-spec/` returns only deliberate
  historical or removal notes.
- Release and smoke verification pass without the directory.
