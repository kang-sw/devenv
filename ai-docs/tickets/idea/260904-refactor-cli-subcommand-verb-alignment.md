---
title: "Align CLI subcommand verbs (and residual planning-ticket prose) with the canonical MCP verb set"
parent: 260903-epic-mcp-tool-surface-affordance-reduction
related:
  - 260903-refactor-mcp-verb-vocabulary-unification
---

## Background

Layer ④ (`260903-refactor-mcp-verb-vocabulary-unification`) renamed six
read-surface **MCP tool** names to the canonical verb set (`find`/`search`→
`query`, `print`/`info`→`read`). Its frozen name map covered the MCP tool surface
only. During implementation two adjacent surfaces were found to still use the old
verbs and were deliberately left out of ④'s scope:

1. **CLI subcommand surface** — `agents-plugin-tool/cmd/.../main.go` uses
   `case "find":` in `ticketsCommand`/`specsCommand`/`mentalModelsCommand`, and
   `runtime.json`'s `"commands"` section + `runtimeCapabilityCommandNames()`
   carry the same dotted-name literals. These share the strings with the MCP tool
   surface but are a separate consumer surface not addressed by ④'s map.
2. **Planning/research ticket prose** — tickets `260703`, `260731`, `260723`
   reference the old MCP names (`ws/playbook.print`, `ws/specs.find`, …) as design
   prose. Left as point-in-time records rather than rewritten.

## Open Questions

- Should the CLI subcommand verbs be aligned to the canonical set (`find`→`query`,
  etc.) for a consistent human CLI vocabulary, or is the CLI surface intentionally
  independent (e.g. `find` reads better as a shell subcommand)? Decide before any
  rename.
- If aligned: does `runtime.json`'s `"commands"` section and
  `runtimeCapabilityCommandNames()` need a coordinated update + a capability test?
- Planning-ticket prose: refresh the three tickets' tool-name references only if/
  when each is next actioned, or sweep them proactively? (Low value either way.)

## Notes

- Zero test-breakage either direction today — the CLI strings are self-consistent
  as-is.
- Scope guard: this does NOT reopen ④'s settled MCP tool-surface rename.
