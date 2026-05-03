---
title: agents-plugin spec skill drafts
parent: 260503-epic-agents-plugin-skill-porting
related:
  260429-research-host-neutral-ws-plugin: research anchor for host-neutral ws plugin architecture
  260503-feat-agents-plugin-runtime-boundary: MCP/runtime boundary that unblocks draft helper references
---

# agents-plugin spec skill drafts

## Background

The `agents-plugin` candidate has draft front-of-pipeline skills and a usable
read-oriented MCP runtime boundary. The next porting slice is the direct
spec/documentation track: `write-spec` and `update-spec`.

The Claude versions still assume slash-command chaining, `$ARGUMENTS`, direct
`ws-*` helper availability, and Claude-specific commit snippets. The Codex-first
drafts should preserve the durable workflow behavior while making helper access
explicit through MCP surfaces or documented CLI fallbacks.

## Constraints

- Do not mutate `claude-plugin/skills/` during this port.
- Keep skill text host-neutral and self-contained.
- Keep `ws-*` helpers as CLI fallbacks, not as the only shared contract.
- Do not claim operational parity for spec index mutation until MCP write
  surfaces or fallback execution semantics are explicitly available.
- Preserve the rule that all AI-authored spec content is English.

## Phases

### Phase 1: write-spec draft

Add `agents-plugin/skills/write-spec/SKILL.md` as a host-neutral draft that:

- loads spec conventions from the current authoritative document
- routes no-public-behavior cases away from spec authoring
- handles create and update flows
- explains anchor generation through the available MCP/fallback boundary
- requires spec index verification after edits
- keeps accuracy checks lead-driven unless delegation is explicitly available

Success criteria:

- The skill follows `ai-docs/ref/skill-authoring.md`.
- The skill does not depend on `$ARGUMENTS`, Claude slash commands, or implicit
  plugin PATH injection.

### Phase 2: update-spec draft

Add `agents-plugin/skills/update-spec/SKILL.md` as a host-neutral draft that:

- audits a commit range for caller-visible behavior changes
- adds missing implemented spec entries
- strips completed `🚧` markers only after confirmation
- handles `removed: <stem>` commit body markers
- commits spec changes when it makes modifications

Success criteria:

- The skill follows `ai-docs/ref/skill-authoring.md`.
- The skill makes helper requirements explicit and does not assume Claude named
  agents or plugin PATH injection.

### Phase 3: port verification

Verify the draft skills as plugin content.

Success criteria:

- The new skill files are present under `agents-plugin/skills/`.
- Frontmatter is minimal and parseable.
- `claude plugin validate agents-plugin` still passes for manifest-level
  compatibility.
- Codex visibility remains a human-in-the-loop plugin cache refresh item, not a
  local CLI claim.
