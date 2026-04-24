---
title: Plugin Infrastructure
summary: Claude Code plugin manifest, bin tool suite, and hook provided by the `ws` plugin under `claude/`.
features:
  - Plugin Manifest
    - `plugin.json`
    - `marketplace.json`
  - Bin Tools
    - `load-infra`
    - `merge-branch`
    - `review-path`
    - `subquery`
    - `list-mental-model`
    - Spec Tooling
  - Hooks
    - `teammate-idle-token-tracker`
---

# Plugin Infrastructure

The `ws` plugin lives under `claude/` and is delivered through the Claude Code plugin system. It exposes a plugin manifest, a set of PATH-accessible bin tools, and a session hook.

## Plugin Manifest

### `plugin.json` {#260421-plugin-json}

Declares the plugin identity consumed by the Claude Code runtime.

- **Name:** `ws`
- **Version:** `0.3.0`
- **Author:** `kang-sw`

No explicit `skills:`, `commands:`, or `bin:` declaration — skill discovery is convention-based. The runtime scans the `skills/` directory by naming convention.

### `marketplace.json` {#260421-marketplace-json}

Directory-source marketplace definition at the repo root. Points to `./claude` as the plugin source path, enabling `claude plugin install ws@ws` to resolve the plugin from the local repository without a remote registry.

`install.sh` writes this entry to `~/.claude/plugins/known_marketplaces.json` before invoking `claude plugin install`, ensuring the CLI resolves the marketplace on fresh machines before Claude Code processes `settings.json`.

> [!note] Constraints
> - Local-path source only — not a published registry entry.

## Bin Tools

Executables under `claude/bin/` are placed on PATH by the Claude Code plugin system. Agents and skills invoke them by bare name from any working directory.

### `load-infra` {#260421-load-infra-tool}

Reads and outputs any file from the plugin's `infra/` directory by name.

```
load-infra <doc-name>
```

Resolves the plugin root via dirname chain from the script's own location — CWD-independent. Needed because `$CLAUDE_PLUGIN_ROOT` is available in skill bash injections but absent in the agent Bash tool context.

### `merge-branch` {#260421-merge-branch-tool}

Merges an implementation branch into a base branch and deletes the impl branch.

```
merge-branch <original-branch> <impl-branch> <commit-msg>
```

Strategy selection:
- Single-commit impl branch → squash merge into `<original-branch>`.
- Multi-commit impl branch → `--no-ff` merge.

Deletes `<impl-branch>` after a successful merge.

### `review-path` {#260421-review-path-tool}

Returns a deterministic, stable temp path for a review findings file.

```
review-path <stem>
```

Prints `/tmp/claude-reviews/<stem>.md`. Creates the parent directory if absent. Spaces in `<stem>` are converted to hyphens. Reviewer agents use this to write structured findings to a file instead of transmitting large `SendMessage` payloads.

> [!note] Planned 🚧
> Will switch to non-deterministic paths incorporating a pwd hash and a per-call run ID: `/tmp/claude-reviews/<pwd-hash>-<run-id>-<stem>.md`. Multi-stem support added: callers pass all stems in one invocation and receive one path per line sharing the same run ID. Contract change: paths are no longer reproducible across calls — the caller must capture all output paths from a single Bash invocation and hold them as literals in agent context for the duration of the run. {#260424-review-path-non-deterministic}

### `subquery` {#260421-subquery-tool}

Spawns a headless `claude -p` subprocess with a structured-report system prompt.

```
subquery [--deep-research] "<question>"
```

Default model: `haiku`. With `--deep-research`: switches to `sonnet`. Output is a self-contained answer in structured-report format.

> [!note] Constraints
> - Non-interactive — no tool use, no follow-up turns. The question must be answerable in one pass.

### `list-mental-model` {#260421-list-mental-model-tool}

Lists `ai-docs/mental-model/` domain documents as a YAML map.

```
list-mental-model [path ...]
```

No args: emits all domain docs as `domain: path` pairs.
With path args: filters to docs whose `sources:` frontmatter field overlaps with the given paths.

### Spec Tooling

`generate-spec-stem`, `list-spec-stems`, and `spec-build-index` are also part of the bin suite. Full behavioral specs are in [Spec System](spec-system.md) under Stem Tooling.

## Hooks

### `teammate-idle-token-tracker` {#260421-teammate-idle-token-tracker}

Fires on `TeammateIdle` events. Reads the most recent subagent `.jsonl` transcript for the named teammate, sums `input_tokens + cache_creation_input_tokens + cache_read_input_tokens` from the last assistant message, and writes a `<pct>%/150K` usage entry to `~/.claude/usage/<team-name>.md`.

Registered automatically in `~/.claude/settings.json` as a `TeammateIdle` hook by `install.sh`.

> [!note] Constraints
> - Measures the last assistant turn's input context only — not cumulative session usage.
> - Output path `~/.claude/usage/<team-name>.md` is machine-local; not committed.
