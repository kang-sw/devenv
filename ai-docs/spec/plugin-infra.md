---
title: Plugin Infrastructure
summary: Claude Code plugin manifest, bin tool suite, and hook provided by the `ws` plugin under `claude-plugin/`.
---

# Plugin Infrastructure

The `ws` plugin lives under `claude-plugin/` and is delivered through the Claude Code plugin system. It exposes a plugin manifest, a set of PATH-accessible bin tools, and a session hook.

## Plugin Manifest

### `plugin.json` {#260421-plugin-json}

Declares the plugin identity consumed by the Claude Code runtime.

- **Name:** `ws`
- **Version:** `0.3.0`
- **Author:** `kang-sw`

No explicit `skills:`, `commands:`, or `bin:` declaration — skill discovery is convention-based. The runtime scans the `skills/` directory by naming convention.

### `marketplace.json` {#260421-marketplace-json}

Directory-source marketplace definition at the repo root. Points to `./claude` as the plugin source path, enabling `claude plugin install ws@ws` to resolve the plugin from the local repository without a remote registry.

`install.sh` writes this entry to `~/.claude-plugin/plugins/known_marketplaces.json` before invoking `claude plugin install`, ensuring the CLI resolves the marketplace on fresh machines before Claude Code processes `settings.json`.

> [!note] Constraints
> - Local-path source only — not a published registry entry.

## Bin Tools

Executables under `claude-plugin/bin/` are placed on PATH by the Claude Code plugin system. Agents and skills invoke them by bare name from any working directory.

### `ws-print-infra` {#260421-ws-print-infra-tool}

Reads and outputs any file from the plugin's `infra/` directory by name.

```
ws-print-infra <doc-name>
```

Resolves the plugin root via dirname chain from the script's own location — CWD-independent. Needed because `$CLAUDE_PLUGIN_ROOT` is available in skill bash injections but absent in the agent Bash tool context.

### `ws-merge-branch` {#260421-ws-merge-branch-tool}

Merges an implementation branch into a base branch and deletes the impl branch.

```
ws-merge-branch <original-branch> <impl-branch> <commit-msg>
```

Strategy selection:
- Single-commit impl branch → squash merge into `<original-branch>`.
- Multi-commit impl branch → `--no-ff` merge.

Deletes `<impl-branch>` after a successful merge.

### `ws-review-path` {#260421-ws-review-path-tool}

Returns temp paths for review findings files.

```
ws-review-path <stem1> [<stem2> ...]
```

Prints one path per stem under `/tmp/claude-reviews/`. Creates the parent directory if absent. Path format: `/tmp/claude-reviews/<pwd-hash>-<run-id>-<stem>.md`. `pwd_hash` scopes paths to the current project; `run_id` prevents collisions across concurrent invocations. {#260424-ws-review-path-non-deterministic}

> [!note] Constraints
> - Caller must capture all output lines from a single invocation — paths are not reproducible after the call returns.
> - Always pass all stems in one call; separate calls produce different `run_id`s and break co-invocation grouping.

### `ws-subquery` {#260421-ws-subquery-tool}

Spawns a headless `claude -p` subprocess with a structured-report system prompt.

```
ws-subquery [--deep-research] "<question>"
```

Default model: `haiku`. With `--deep-research`: switches to `sonnet`. Output is a self-contained answer in structured-report format.

> [!note] Constraints
> - Non-interactive — no tool use, no follow-up turns. The question must be answerable in one pass.

### `ws-list-mental-model` {#260421-ws-list-mental-model-tool}

Lists `ai-docs/mental-model/` domain documents as a YAML map.

```
ws-list-mental-model [path ...]
```

No args: emits all domain docs as `domain: path` pairs.
With path args: filters to docs whose `sources:` frontmatter field overlaps with the given paths.

### Spec Tooling

`ws-generate-spec-stem`, `ws-list-spec-stems`, and `ws-spec-build-index` are also part of the bin suite. Full behavioral specs are in [Spec System](spec-system.md) under Stem Tooling.

## Hooks

### `teammate-idle-token-tracker` {#260421-teammate-idle-token-tracker}

Fires on `TeammateIdle` events. Reads the most recent subagent `.jsonl` transcript for the named teammate, sums `input_tokens + cache_creation_input_tokens + cache_read_input_tokens` from the last assistant message, and writes a `<pct>%/150K` usage entry to `~/.claude-plugin/usage/<team-name>.md`.

Registered automatically in `~/.claude-plugin/settings.json` as a `TeammateIdle` hook by `install.sh`.

> [!note] Constraints
> - Measures the last assistant turn's input context only — not cumulative session usage.
> - Output path `~/.claude-plugin/usage/<team-name>.md` is machine-local; not committed.
