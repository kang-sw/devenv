---
title: regenerate devenv's own root AGENTS.md / WORKFLOW.md to clear retired Ticket Focus references
related:
  260710-bug-project-index-ticket-focus-stale-status: completes the deferred post-reinstall regeneration of this ticket's Phase 1
---

# regenerate devenv's own root AGENTS.md / WORKFLOW.md to clear retired Ticket Focus references

## Context

`260710-bug-project-index-ticket-focus-stale-status` Phase 1 removed the
`## Ticket Focus` section and retired every source-tree surface that reads,
writes, maintains, or describes it (rsrc bodies, specs, the Go-embedded
convention, both `AGENTS.template.md` markers bumped to `v0044` (ws) / `v0005`
(wsflow)). What it could **not** land on that branch is the regeneration of this
repo's own *managed* consumer files:

- root `AGENTS.md` (~:199) still carries the `Check '## Ticket Focus' …` reader
  instruction.
- `ai-docs/WORKFLOW.md` (:47, :107, :120) still carries the Ticket Focus
  semantics / keep-list / routing mentions.

Per 260710's contract these must be cleared by **bootstrap regeneration, never a
hand-edit** (a hand-edit is re-added on the next bootstrap upgrade).

## Blocked

A correct regeneration requires the rebuilt-and-reinstalled plugin carrying the
new `v0044` template. The currently installed binary still ships the `v0043`
template (which still contains the Ticket Focus reader line), so running
`lead-bootstrap` today would *reintroduce* the removed text rather than clear it
— the same installed-binary-vs-branch-source divergence that already affects
`ws/convention.read`. This item is therefore gated on 260710 shipping to `main`
and devenv re-installing the plugin.

## Next Step (once unblocked)

After the plugin with template `v0044` is installed:

1. Run the `lead-bootstrap` upgrade path so devenv's managed root `AGENTS.md` and
   `ai-docs/WORKFLOW.md` regenerate from the `v0044` template without the Ticket
   Focus reader instruction, keep-list membership, or routing mention.
2. Verify: a repo-wide `grep -ri 'ticket focus'` then returns only immutable
   migration-history entries (the `v0041` / `v0004` bullets and the `v0044` /
   `v0005` entries that name the section they retire), `CHANGELOG.md`, and ticket
   bodies — no live reader/semantics reference in `AGENTS.md` / `WORKFLOW.md`.
3. Note the v0044 migration entry's section hint says the reader bullet lives
   under `## Project Knowledge` (true in the template); in devenv's own generated
   `AGENTS.md` the equivalent line sits under `## Ticket System`, so a
   section-scoped regen must still catch it there.
