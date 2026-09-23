---
title: ws config "project" scope resolves to one machine-wide file
---

# ws config "project" scope resolves to one machine-wide file

## Background

Dogfood surprise on 2026-09-24 during `/ws:lead-tune` (resetting project-scope
`agents.tier` mappings). The user asked to reset "this project's bucket and any
other project buckets"; there are no per-project buckets.

- `wsconfig.Path` returns `CacheRoot/config.json`, and `CacheRoot` with empty
  `Options` is `~/.cache/ws@kang-sw-devenv` (or `$WS_CACHE_HOME`). No project
  key enters the path.
- MCP callers construct `wsconfig.Options{}` (e.g. `internal/mcp/server.go`,
  `playbook_tools.go`), so every project on the machine reads and writes the
  same "project" file.
- Code comments describe the scope as "per-project, machine-local
  (`~/.ws@<id>/config.json`)" (`internal/wsconfig/scope.go`,
  `internal/wsconfig/repo.go`, `internal/wsconfig/resolver.go`), and
  `worktree_pool` defaults to project scope on the premise that it is
  per-project.

Effectively "project" scope is a second machine-wide layer above `global`
(`~/.ws/config.json`). A `config.tune` write meant for one project silently
changes every project.

## Open questions

- Intended semantics: key the project file by the project identity already
  computed in `wsstate` (`projects/<rootID>/config.json`), or rename/document
  the scope as machine-wide?
- Migration for existing `CacheRoot/config.json` content if the path moves.
- Whether `config.list` should surface which file each scope reads.

## Related observation

`config.tune` reset for `agents.tier` removes only `model_aliases` leaves; a
legacy `agents.tiers` block in the same file keeps seeding the `codex` and
`default` buckets through `effectiveAgentConfig`'s explicit-tier path, so
a full reset through catalog writers is not possible.
