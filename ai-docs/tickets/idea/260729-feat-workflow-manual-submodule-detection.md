---
title: Announce ws-aware submodules in workflow_manual output
related:
  260729-bug-dashboard-submodule-workroot-empty-projection: sibling submodule-support gap
---

# Announce ws-aware submodules in workflow_manual output

## Background

`wsstate` now resolves a git submodule working tree as an independent ws project,
so a lead can `ferrule` a submodule root and run the full workflow inside it. The
tooling works, but nothing tells the lead the submodule is there or that it is a
separate board with a separate key.

A lead that does not know this either treats the submodule's files as part of the
parent project (wrong root, wrong board) or discovers the split only when a tool
returns something surprising. `workflow_manual` is the right place to close that
gap: its load rate is high, it already carries the "each key binds to one
canonical repository root" rule, and it already has conditional-injection
machinery (`injectSkepticalPosture`, `injectBootstrapStalenessWarning`,
`injectDocCoverageWarning` in `agents-plugin-tool/internal/mcp/workflow_manual.go`).

## Decisions

- **Detection, not federation.** The output announces that a ws-aware submodule
  exists and that it is a separate root needing its own key. It does not merge
  boards, resolve cross-root references, or teach any `<submodule>#<stem>` syntax.
  A cross-root reference surface was considered as an epic and rejected: it would
  have to breach the one-key-one-root mapping in `resolveToolRoot`
  (`internal/mcp/server.go`), which is an isolation invariant, and the main thing
  it would enable is the anti-pattern below.
- **Ticket ownership rule must travel with the announcement.** A submodule's
  implementation tickets belong on the submodule's own board, never the parent's.
  Ticket tracking is `git log --grep=<stem>` against the repo that holds the
  commits, so a parent-board ticket for submodule work is structurally
  untrackable. A downstream patch is either an upstream ticket on the submodule
  board, or not ticketed at all and expressed as the parent's pin-bump commit.
- **Marker for "ws-aware".** Prefer the `<!-- Template Version: vNNNN -->` line in
  the submodule's `AGENTS.md` over the presence of `ai-docs/WORKFLOW.md`. The
  bootstrap staleness alarm already parses that marker, so detection reuses an
  existing parse rather than inventing a second convention.

## Constraints

- `.gitmodules` parse plus a stat of each submodule path; must stay cheap enough
  for a tool this frequently loaded, and must not shell out per submodule on a
  hot path.
- Must be silent when there are no submodules, and when submodules exist but none
  are ws-aware. An always-present empty section is context cost for the common case.
- Must not fail the `workflow_manual` call when `.gitmodules` is missing,
  malformed, or the submodule is uninitialized — the announcement is advisory.
- Injection follows the existing `inject*` pattern; manual prose belongs in the
  rsrc, not in handler string literals (`workflow_manual.go` currently owns only
  scaffolding strings, and that boundary should hold).

## Open Questions

- Does the announcement belong in every mode, or only fresh/continue? A lead deep
  into a session may not need it re-stated on each reload.
- Should it name the submodule's own board status (e.g. ready-ticket count), or
  only its existence? Reading the child's `ai-docs` costs more and duplicates what
  the lead gets after `ferrule`-ing that root.
