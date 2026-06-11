---
title: render-minted child-key splice is unreachable -- no shipped playbook declares a delegate role
related:
  260609-refactor-ws-spawn-runtime-deletion-session-auth: 2c implemented the mint/splice mechanism; this is the missing asset that makes it reachable
---

# render-minted child-key splice is unreachable -- no shipped playbook declares a delegate role

## Background

Dogfooding 2c's render-minted child-key path on the Claude plugin install:
`ws.lead.login` (lead key) + `playbook.render(session_key, name)` succeeds and
writes the rendered prompt, but the **minted child session key is never spliced
into any shipped playbook** because the splice is gated on the playbook's
frontmatter `role`.

## What is wrong

`renderPlaybookBody` (`agents-plugin-tool/internal/mcp/playbook_tools.go`) only
mints + splices a child key when:

1. the caller is a lead (`mintRoot != ""`), AND
2. `childRoleForPlaybookRole(meta.Role)` returns ok -- i.e. the playbook's
   frontmatter `role:` is one of `implementer | reviewer | delegate | leaf`.

The mechanism is complete and unit-tested (`mercenary_surface_test.go`,
`session_auth_test.go` use in-memory role fixtures; `loader.go` parses the
`role:` frontmatter key correctly). But **no playbook in the shipped `rsrc/`
tree declares a `role:` field at all**:

```
$ grep -rl '^role:' agents-plugin/rsrc/*/*.md   # -> no matches
```

So with every real shipped playbook, `meta.Role == ""`,
`childRoleForPlaybookRole("")` is false, and the credential block is never
prepended. `delegate-sample` sets `delegates: true` (gets the delegation tip)
but has no `role:`, so it does not mint either.

## Why it matters

2c's stated contract (ticket 260609, "single self-contained prompt from
`playbook.render`", "scope mercenary to implementer/reviewer roles only",
"render-minted child keys ... when `session_key.role == lead`") requires a
render-able implementer/reviewer prompt that carries the minted child key. The
runtime half exists; the **asset half (a `role: implementer` / `role: reviewer`
delegate playbook in `rsrc/`) is missing**, so the end-to-end delegate handoff
the milestone describes cannot actually be exercised. The feature reads as
implemented (code + tests green) but is unreachable through the shipped surface.

## Possible follow-ups

- Add the implementer/reviewer delegate playbook(s) to `rsrc/` with the proper
  `role:` frontmatter (and a manifest regen) so `playbook.render` produces the
  self-contained, pre-keyed prompt 2c describes. Confirm whether this was
  intended to land in 2c or is deferred to Phase 3.
- Add an end-to-end test (or a dogfood check) that renders a shipped delegate
  playbook with a lead key and asserts the credential block is present -- the
  current unit tests pass with in-memory fixtures and would not catch the
  missing shipped asset.
- Consider a manifest/lint gate: if the milestone requires at least one
  delegate-role playbook, fail the build when none is present.

## Notes

- Surfaced alongside two sibling dogfood findings from the same session:
  `260611-bug-launcher-repair-failure-opaque-mcp-error` (already filed) and
  `260611-bug-rsrc-load-unknown-playbook-misleading-error`. The WS_RSRC_ROOT
  manifest-resolution bug found in the same session was fixed in 379ff5e5.
