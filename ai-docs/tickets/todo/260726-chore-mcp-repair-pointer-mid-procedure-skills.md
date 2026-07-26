---
title: mcp-server-repair pointers reach only front-door skills, not the skills a procedure lands in
related:
  260624-epic-pre-release-cleanup: item 8 · repair-pointer coverage gap
  260726-feat-ws-cli-call-stdin-payload: sibling CLI-fallback ergonomics ticket from the same MCP-down dogfooding class
  260726-feat-ws-cli-lenient-tool-name-resolution: sibling CLI-fallback ergonomics ticket from the same MCP-down dogfooding class
sage-review-design: required
---

# mcp-server-repair pointers reach only front-door skills, not the skills a procedure lands in

## Background

Reported downstream (wsflow 0.36.1) as "no local fallback when the MCP server
drops mid-procedure". **Two of the report's premises are false and must not be
acted on:**

- *"`rsrc/` ships `impl-playbook.md` and `sample-playbook/`, but not the `lead-*`
  playbooks."* False. Every `lead-*` playbook ships in `rsrc/`, verified in
  source (`agents-plugin/rsrc/lead-write-ticket/`), in the wsflow mirror, and in
  an installed plugin cache. The reporter appears to have read the top-level
  listing and mistaken two entries for the whole set. **Do not bundle anything.**
- *"There is no documented degraded path."* False in current source. The
  `mcp-server-repair` skill exists precisely for this: it makes no MCP call, maps
  every `ws/x.y(a: b)` to `ws-cli call x.y '{"a": "b"}'`, covers cold start, and
  carries a verbatim reconnect-steps relay for the user.

The report is explained almost entirely by distribution lag. `mcp-server-repair`
landed in `adbf5ec3` on 2026-07-25; the 0.36.1 release the reporter ran was
`62d9a1a1` on 2026-07-24. It missed the release by one day.

The genuine residue is pointer coverage. `adbf5ec3` added front-door pointers to
`lead-discuss`, `lead-sprint`, `lead-proceed`, and `lead-revive` — entry-point
skills. `lead-write-ticket` and `lead-write-spec` have none; their SKILL.md still
ends at "If the playbook cannot be loaded, stop and report that blocker."
Downstream died at exactly that line, mid-procedure, after routing.

## Decisions

- **Scope is a pointer sweep, nothing else.** No bundling, no new fallback
  mechanism, no softening of the stop-and-report default beyond naming the repair
  route.
- **Cover skills reached after routing, not just entry points.** A transient
  transport failure should not read as a hard stop on user-requested work when a
  documented CLI fallback exists one pointer away.

## Prior Art

- `adbf5ec3` established the pointer wording; reuse it verbatim rather than
  inventing a second phrasing.

## Phases

### Phase 1: Extend the repair pointer to mid-procedure skills

- Add the existing front-door pointer wording to `lead-write-ticket`,
  `lead-write-spec`, and any other skill whose SKILL.md delegates to
  `playbook.print` without naming the repair route.
- Mirror into `agents-plugin-wsflow/skills/` so the two trees do not drift.
- Register this ticket as item 8 in `260624-epic-pre-release-cleanup`.

Verification boundary: every SKILL.md that can fail on `playbook.print` names the
repair route; the wsflow mirror matches.
