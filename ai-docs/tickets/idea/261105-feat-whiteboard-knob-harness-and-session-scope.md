---
title: workflow.whiteboard_enabled - per-harness and session-scoped values
---

# workflow.whiteboard_enabled - per-harness and session-scoped values

## Background

Dogfood observation (2026-11-05): the user wanted the whiteboard on for the
Claude Code harness only, as a global preference, and found that
`workflow.whiteboard_enabled` cannot express that. The knob is global-only
(`RegisterGlobalOnly` in `agents-plugin-tool/internal/wsconfig/scope.go`)
and has no harness selector, so a global `on` also turns the whiteboard on
for Codex and Pi leads. The user also stated that it should be possible to
turn it on for a single session.

User requirements (2026-11-05):

- the value can differ per harness (for example on for `claude`, off for
  `codex` and `pi`);
- the value can be set at session scope.

## Why it is global-only today

`261030-feat-session-whiteboard-mode` declared the knob global-only after
the `workflow.prefer_subagent` precedent (technical decision (101)), with
the reason "`handleWorkflowManual` renders the manual with
`wsconfig.Options{}` and no repo root, so a repo-scoped override would be
invisible to the manual gate". `workflow.prefer_subagent` itself became
global-only in f4144db58 so that keyless bootstrap and tool-surface reads
could not diverge from session or project state.

The premise does not hold for the whiteboard knob:

- `handleWorkflowManual` (`internal/mcp/workflow_manual.go`) already resolves
  `workflow.lang` and `workflow.skeptical_posture` through
  `s.sessionResolver(key, nil)`, which reads the session, project, and repo
  scopes from the lead's session key. Only the `printPlaybook` call passes
  `wsconfig.Options{}`, and `applyWhiteboardVariants`
  (`internal/mcp/whiteboard_text.go`) reads the knob through that empty
  `configOpts` via `workflowWhiteboardEnabled`
  (`internal/mcp/whiteboard_tools.go`), which calls `resolver.Get("", ...)`.
- The whiteboard tools and the knob-on text are rendered only for a keyed
  lead session, so the keyless-read divergence that motivated
  `workflow.prefer_subagent` does not apply.

## Open questions

- Harness axis: a harness selector on the existing knob (the shape
  `prompt.<pointId>` and `agents.tier` use), or per-harness adapter settings
  (`claude.whiteboard_enabled` and siblings, the `claude.status_band_enabled`
  shape)? The original ticket rejected a package-manifest `claude.` key
  because the manual mandate is host-neutral.
- Scopes: the ordinary layered scopes (session, project, repo, global), or
  session and global only (the original ticket called side-by-side viewing
  the user's habit, not a project property)?
- Default scope for a write that names none: global, as today?
- Interaction with `261105-feat-lead-ticket-review-auto-ack` (132), whose
  rationale cites the whiteboard knob's global-only reason.
