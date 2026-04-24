# Ship: ws

The `ws` Claude Code plugin — structured AI-assisted development workflow skills and agents.

## Version Strategy

Semantic versioning. Source of truth: `claude/.claude-plugin/plugin.json` → `version` field.

Bump rules:
- **Minor** (0.X.0): new user-invocable skill or agent added.
- **Patch** (0.0.X): behavior change to existing skill, agent, or infra doc with no new public entry point.
- **Major** (X.0.0): breaking change to a canonical flow or public interface that requires downstream migration.

At ship time:
1. Run `git log <last-tag>..HEAD --oneline` to enumerate changes.
2. Classify each commit as minor, patch, or major using the rules above.
3. Apply the highest bump to the current version in `claude/.claude-plugin/plugin.json`.
4. Update the `Plugin: ws@<version>` line in `ai-docs/_index.md`.

## Pre-flight

- `git status --porcelain` — must be empty (clean working tree).
- `git log origin/main..HEAD` — must be empty (no unpushed commits before tagging).

## Changelog

Update `CHANGELOG.md` in the repo root before tagging:

```markdown
## v<version> — YYYY-MM-DD

### Added
- <new skill or agent>

### Changed
- <behavior change>

### Fixed
- <bug fix>
```

One entry per shipped version. Derive content from `git log <last-tag>..HEAD`.

## Build

- `claude plugin update ws@ws` — propagate changes to local plugin cache.

## Tag

Format: `v<version>` (e.g. `v0.6.0`)
Push: yes

## Publish

- `git push origin main`
- `git push origin v<version>`

## Post-ship

- Confirm plugin version with `claude plugin list | grep ws`.
