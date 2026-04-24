# Changelog

## v1.0.0 — 2026-04-24

### Added
- `ws-call-agent` — `claude -p` wrapper with permission bypass; `--agent` flag for deterministic UUID sessions (create-or-resume), `--session-id`, `--uuid`, `--system-prompt`
- `ws-agent` — deterministic UUID v5 from repo-root + branch + name
- `ws-declare-agent` — clears session files to scope agent slots to a run
- `ws-call-agent` formatted output: `[info]`/`[warn]` context window line (shown when `--agent` used and fill ≥50%; warn at ≥70% of 150K)

### Changed
- `ws:implement` internal orchestration rewritten — `TeamCreate`/`SendMessage`/`TeamDelete` replaced with `ws-call-agent`/`ws-declare-agent`
- `ws-*` scripts moved from `claude/infra/` to `claude/bin/` (PATH-accessible)

### Removed
- `/parallel-implement` skill — split-scope work now handled via split tickets + `/implement`
- `/team-lead` skill — no longer needed without `TeamCreate` machinery

## v0.6.0 — 2026-04-24

### Added

- `/add-rule` skill: classify an incoming rule as cross-cutting (→ `CLAUDE.md ## Architecture Rules`) or domain-scoped (→ `ai-docs/mental-model/<domain>.md ## Domain Rules`). Autonomous when clear, interactive when ambiguous. Append-only — never modifies existing rules.
- Ship config (`ai-docs/ship/ws.md`): version strategy, CHANGELOG procedure, pre-flight, tag, and push steps for the `ws` plugin.

### Changed

- **2-layer Architecture Rules split**: `CLAUDE.md ## Architecture Rules` is now scoped to cross-cutting invariants only. Domain-scoped rules belong in `ai-docs/mental-model/<domain>.md ## Domain Rules`.
- `mental-model-conventions.md`: added `## Directory Hierarchy` (flat vs `<domain>/index.md` + children), ancestor loading invariant, and `## Domain Rules` section with authorship and modification constraints.
- `ai-docs/mental-model.md`: updated index with directory hierarchy and Domain Rules sections.
- `claude/bin/list-mental-model`: rewritten with tree output (`├─`/`└─` glyphs); ancestor `index.md` auto-emitted alongside matching direct-child sub-domain in filtered mode.
- `mental-model-updater`: gains `/forge-mental-model` authority — creates new domain docs and splits flat docs to `<domain>/index.md` + children when diff shows code-structure change. Domain Rules promotion-only (upward during splits, never downward, never content-modified). Stale rules flagged in `## Stale Rules` output block; never edited autonomously.
- `executor-wrapup.md`: added `§Ancestor Loading` contract (3-step procedure) and Invariant bullet for one-level hierarchy reads.
- `edit`, `implement`, `parallel-implement` skills: propagate ancestor-loading contract to Invariants and spawn prompts, bounded to one-level hierarchies (`<domain>/<sub>.md` only).
- `CLAUDE.template.md` v0028: tightened `## Architecture Rules` inclusion test (explicitly excludes domain-scoped rules; directs authors to `/add-rule`); added v0028 migration item (reclassify existing Architecture Rules entries that are domain-scoped).
- `_index.md`: stale per-commit version-bump rule removed (ship config is now the authority); `/add-rule` added to skill inventory.

## v0.5.0 — 2026-04-24

### Added

- `project-survey` agent: pre-invocation context survey; returns `[Must|Maybe]`-tiered spec/mental-model/ticket reference list for a given brief.
- Auto-invoke integration: `edit`, `implement`, `parallel-implement`, `discuss` each spawn `project-survey` at step 0.
