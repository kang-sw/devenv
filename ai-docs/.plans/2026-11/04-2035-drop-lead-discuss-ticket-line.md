# Drop the `Ticket: <path>` output-line rule from lead-discuss

## Outcome

The lead-discuss playbook's `## Output` section no longer asks the lead to end
with a `Ticket: <path>` line. The section body becomes just `Conversation.`

## Rationale

- User decision (89a), 2026-11-04: remove the rule from lead-discuss only.
- No Go code, hook, Pi source, or other playbook parses the line (grep over
  `agents-plugin/rsrc`, `agents-plugin/skills`, `agents-plugin/hooks`,
  `agents-plugin-tool/internal`, `agents-plugin-pi/src`). It is a leftover of
  the pre-2026-09 proceed handoff "artifact line" convention (`faf7d3265`,
  `49283a547`), carried into lead-discuss by `ce5f30ef5`.
- lead-discuss only re-points at a ticket the user already knows.

## Scope

Edit:

- `agents-plugin/rsrc/lead-discuss/lead-discuss.md` (`## Output`, lines ~59-62)
- `agents-plugin-wsflow/rsrc/lead-discuss/lead-discuss.md` (generated mirror;
  update per `ai-docs/manuals/wsflow-mirroring.md`)
- `agents-plugin-pi/rsrc/lead-discuss/lead-discuss.md` (Pi copy; update per
  whatever mechanism keeps the Pi rsrc tree in sync)

Out of scope, keep unchanged:

- `lead-ticket.md`'s `End with Ticket: ...` rule (user kept it, (89a)).
- `lead-run.md:84` `Ticket: <ticket path> (stem ...)` (worker brief content).
- `ai-docs/tickets/todo/261030-feat-session-whiteboard-mode.md` (its
  `Ticket: <path>` example still holds through lead-ticket).

## Verification

- wsflow mirror drift test and Pi rsrc tree drift test pass.
- Any test that pins lead-discuss text passes.

## Return boundary

Stop and report if a test or a consumer depends on the line, or if the edit
needs any file beyond the three listed.
