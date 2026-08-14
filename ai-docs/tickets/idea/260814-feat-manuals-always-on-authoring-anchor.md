---
title: Make # Manuals an always-on authoring anchor
related-mental-model:
  - mcp-runtime
---

# Make # Manuals an always-on authoring anchor

## Background

The `ai-docs/manuals/` tier is asymmetrically under-exposed as an *authoring*
convention. An audit (this session) found:

- The consumed schema is deliberately a single optional field, `summary:`
  (`agents-plugin-tool/internal/wsdoc/manuals.go:10-16,49`).
- There is **no `manuals-conventions.md`** returned by `ws/convention.read`
  (`agents-plugin-tool/internal/wsdoc/conventions.go:14-18`), unlike
  spec/ticket/mental-model tiers. An agent reaching for the canonical
  authoring-discovery path finds nothing for manuals.
- Authoring guidance exists only inside the `lead-bootstrap` skill surface and
  the ambient nudge — not on a generally-discoverable channel.

Meanwhile the ambient `# Manuals` block
(`agents-plugin-tool/internal/mcp/manuals_announcement.go`) is injected only
into **lead** `workflow_manual` output (subagents cannot bootstrap
workflow_manual — `workflow_manual.go:31-33`), and it early-returns `""` when
zero manuals exist, so the block vanishes exactly when a project has not yet
started writing manuals — the moment authoring guidance would help most.

Goal: turn `# Manuals` into an **always-present authoring anchor** for the lead
session that (a) teaches where shared project procedures live and how to
summarize them, and (b) teaches the local/tracked split so credentials, IPs,
and other machine-local details are written to a gitignored `*.local.md`
sibling rather than committed into a tracked manual. This is deliberately a
different treatment from `# Notes` (which is presence-gated): `# Manuals` should
be an ever-present convention anchor, not a memory dump.

## Decisions

Resolved:

- **Always render `# Manuals`.** Drop the `len(manuals)==0 -> ""` early return
  in `computeManuals`; the block renders header + a fixed authoring-guidance
  paragraph unconditionally, then either the manual list or a `- (none yet)`
  placeholder. Cost is lead-bootstrap-only (not per-subagent), so the standing
  overhead is small and lands on the right audience (the lead curates manuals).
- **Channel is the ambient block, not `convention.read`.** Per user direction,
  the guidance lives in the always-on `# Manuals` block. A
  `manuals-conventions.md` `convention.read` doc is explicitly out of scope for
  this ticket (the consumed schema is one field; the ambient anchor covers the
  authoring need for the lead audience).

Open (need user's call before Phase 1):

- **[open] `.local.md` in the ambient list.** `ManualsList` currently lists any
  `*.md`, so `ai-docs/manuals/*.local.md` are listed *and* nagged for a missing
  `summary:` (`manuals.go:40`, `manuals_announcement.go:31-32`). Options:
  (a) exclude `*.local.md` from the ambient `# Manuals` catalog (recommended —
  the block advertises shared/tracked manuals; local files should not be nagged
  for summaries), keeping `manuals.list`/`manuals.find` behavior unchanged; or
  (b) exclude `*.local.md` from `ManualsList` entirely (also affects
  `manuals.list`/`manuals.find`); or (c) keep current behavior (list them).
  Note: workflow_manual is lead-local, so this is a clarity/nag question, not a
  cross-clone leak.
- **[open] Guidance wording.** Draft below; confirm tone/scope.

Draft guidance text (English, ambient block; final wording pending):

```
# Manuals
Shared project procedures (build, deploy, env setup, …) live here: one markdown
file per procedure under `ai-docs/manuals/`, each opening with a one-line
`summary:` frontmatter. Keep machine-local details (credentials, IPs, hostnames)
out of tracked manuals — write them to a sibling `*.local.md` (gitignored).
- <path> — <summary>
```

## Prior Art

- `computeNotes` / `wsnote.Compute` (`agents-plugin-tool/internal/wsnote/inject.go`)
  — the presence-gated sibling block; `# Manuals` intentionally diverges by
  being always-on.
- `.gitignore:16` already carries `ai-docs/**/*.local.md`, so the `*.local.md`
  convention has gitignore backing (existing `ai-docs/_index.local.md`,
  `_continue.local.md`).

## Spec Impact

Observable `workflow_manual` behavior change (the `# Manuals` block now always
renders for lead sessions). Update the manuals/workflow_manual behavior contract
in `ai-docs/spec/` (mcp-tools.md or plugin-runtime.md, whichever owns the
`# Manuals` ambient-block contract) to state: always-on for lead sessions,
fixed authoring-guidance paragraph, `- (none yet)` placeholder when empty, and
the resolved `.local.md` listing rule.

## Phases

### Phase 1: Always-on # Manuals authoring anchor

Goals:

- Remove the empty-list early return in `computeManuals`; render header +
  fixed guidance paragraph + list-or-placeholder.
- Apply the resolved `.local.md` rule (open decision above).
- Update tests: `manuals_announcement`-level rendering (empty, non-empty,
  no-summary, `.local.md` handling) and the `note_workflow_manual` /
  workflow_manual assembly tests that assert the `# Manuals` block.
- Update the spec behavior contract per `## Spec Impact`.

Constraints:

- Do not add a `manuals-conventions.md` `convention.read` doc in this ticket.
- Keep the change scoped to `computeManuals` (+ `ManualsList` only if the
  chosen `.local.md` rule requires it); do not alter `# Notes` behavior.
- Guidance text is AI-authored English.
