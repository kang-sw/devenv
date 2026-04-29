---
title: Blueprint plugin extraction
started: 2026-04-19
completed: 2026-04-19
---

# Blueprint plugin extraction

## Background

The `claude/` workflow system (skills, agents, infra) living in devenv is being packaged as a Claude Code plugin named **blueprint** (`/bp:` prefix). Goal: enable company colleagues to install the workflow via Claude Code's plugin system while the author continues to develop it in-place in devenv with zero extra sync steps.

## Decisions

- **Name**: `blueprint` — maps to `/bp:` prefix. Short, ergonomic, descriptive (blueprints = plans before building).
- **No separate repo**: devenv is already public. Full repo clone is intentional (author convenience over bandwidth). Plugin lives at `claude/` subdirectory; external users install via `/plugin marketplace add kang-sw/devenv` + `/plugin install blueprint@blueprint`.
- **Local install mechanism**: `extraKnownMarketplaces` with `directory` source in `~/.claude/settings.json`, followed by `claude plugin install blueprint@blueprint`. Plugin files are **copied to cache** (not loaded live); `claude plugin update blueprint@blueprint` is the author's maintenance cost after changes.
- **infra scripts → `claude/bin/`**: `ask.sh`, `merge-branch.sh`, `list-mental-model.py` become PATH-accessible bare executables (`ask`, `merge-branch`, `list-mental-model`).
- **infra docs stay in `claude/infra/`**: `impl-playbook.md`, `ticket-conventions.md`, `mental-model-conventions.md`, `_subagent-rules.md`.
- **`blueprint-infra` helper**: New script in `claude/bin/`. Self-locates plugin root via `dirname` chain, cats any infra doc by name. Required because `$CLAUDE_PLUGIN_ROOT` is available in skill bash injections but **not** in agent execution context.
- **Reference pattern by context**:
  - Skills (bash `!` injection): `${CLAUDE_PLUGIN_ROOT}/infra/<doc>.md`
  - Agents (Bash tool): `blueprint-infra <doc>.md`
- **Rejected**:
  - Separate GitHub repo — submodule sync overhead across machines
  - Symlink into plugin cache — unsupported, gets overwritten on update
  - Home-dir install via `install.sh` copy — superseded by plugin system

## Phases

### Phase 1: Restructure claude/ into plugin layout

- Move `claude/infra/ask.sh` → `claude/bin/ask` (executable, no extension)
- Move `claude/infra/merge-branch.sh` → `claude/bin/merge-branch`
- Move `claude/infra/list-mental-model.py` → `claude/bin/list-mental-model`
- Create `claude/bin/blueprint-infra` helper:
  ```bash
  #!/bin/bash
  PLUGIN_ROOT="$(dirname "$(dirname "$(readlink -f "$0")")")"
  cat "${PLUGIN_ROOT}/infra/$1"
  ```
- Add `claude/.claude-plugin/plugin.json`:
  ```json
  { "name": "blueprint", "version": "0.1.0", "description": "Structured AI-assisted development workflow" }
  ```
- Add `devenv/.claude-plugin/marketplace.json`:
  ```json
  { "name": "blueprint-marketplace", "plugins": [{ "name": "blueprint", "source": "./claude" }] }
  ```

Success: `claude/` has valid plugin structure; `claude/bin/` contains all executables; `claude/infra/` contains only `.md` docs.

### Phase 2: Update skill/agent infra references

Update all references across `claude/skills/` and `claude/agents/`:

| Old reference | New reference (skills) | New reference (agents) |
|---|---|---|
| `~/.claude/infra/ask.sh "..."` | `ask "..."` | `ask "..."` |
| `~/.claude/infra/merge-branch.sh` | `merge-branch` | `merge-branch` |
| `python3 ~/.claude/infra/list-mental-model.py` | `list-mental-model` | `list-mental-model` |
| `~/.claude/infra/impl-playbook.md` | `${CLAUDE_PLUGIN_ROOT}/infra/impl-playbook.md` | `blueprint-infra impl-playbook.md` |
| `~/.claude/infra/ticket-conventions.md` | `${CLAUDE_PLUGIN_ROOT}/infra/ticket-conventions.md` | `blueprint-infra ticket-conventions.md` |
| `~/.claude/infra/mental-model-conventions.md` | `${CLAUDE_PLUGIN_ROOT}/infra/mental-model-conventions.md` | `blueprint-infra mental-model-conventions.md` |
| `~/.claude/infra/agents/_subagent-rules.md` | `${CLAUDE_PLUGIN_ROOT}/infra/subagent-rules.md` | `blueprint-infra subagent-rules.md` |

Files to update (from prior audit):
- Skills: `implement/SKILL.md`, `write-mental-model/SKILL.md`, `write-ticket/SKILL.md`, `parallel-implement/SKILL.md`, `delegate-implement/SKILL.md`, `write-skeleton/skeleton-writer.md`, `write-plan/plan-writer.md`
- Agents: `implementer.md`, `parallel-implementer.md`, `clerk.md`, `mental-model-updater.md`, `reviewer.md`

Success: zero `~/.claude/infra/` references remain in any skill or agent file.

### Phase 3: Update install.sh

Patch `~/.claude/settings.json` to add:
```json
{
  "extraKnownMarketplaces": {
    "blueprint": {
      "source": { "source": "directory", "path": "<absolute-path-to-devenv>" }
    }
  },
  "enabledPlugins": {
    "blueprint@blueprint": true
  }
}
```

The `<absolute-path-to-devenv>` must be resolved at install time (not hardcoded — machines differ).
Remove or gate the old `~/.claude/` file-copy behavior; plugin system supersedes it.
After writing `settings.json`, also run `claude plugin install blueprint@blueprint` (idempotent on re-runs: check `installed_plugins.json` before running).

Success: `install.sh` on a clean machine registers the marketplace, installs the plugin to cache, and leaves `/doctor` clean.

### Phase 4: Validation (new session required)

Must be run in a fresh Claude Code session after `install.sh` has been applied:

- Plugin loads: `blueprint` appears in Claude Code plugin list
- Skill prefix resolves: `/bp:proceed`, `/bp:discuss`, `/bp:ship` are invocable
- Infra doc access (skill context): invoke `/bp:write-ticket` — verify `ticket-conventions.md` is injected correctly
- Infra doc access (agent context): trigger a flow that spawns an implementer agent — verify `impl-playbook.md` is read via `blueprint-infra`
- Bare executables reachable: a skill or agent calls `ask` or `merge-branch` — no "command not found"
- `list-mental-model` works standalone: `list-mental-model` with no args returns mental model doc list
