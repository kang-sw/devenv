# mcp-precheck-tool-arguments

## Outcome

A ws MCP `tools/call` whose arguments miss a declared `required` property, or
carry a top-level argument name the tool's `inputSchema.properties` does not
declare, is refused before its handler runs. The refusal is an `isError` tool
result that names every missing required property and every unknown argument,
then dumps the failed tool's own `inputSchema` as JSON (only that tool, never
the full tool list), so the calling agent can retry correctly in one step.

## Why

Agents (notably when a host passes calls through without client-side schema
validation, e.g. deferred tools called before their schema was loaded) hit
one-line handler errors such as `title is required`, fix one field, and fail
again on the next; unknown argument names (`message` for `title`) are silently
ignored today. User decisions (38)/(39a), 2026-10-30: dump the failed tool's
schema only; delegate rather than ticket.

## Scope

- One central check in `Server.callTool`
  (`agents-plugin-tool/internal/mcp/server.go`), after the existing profile /
  role gates and before the handler `switch`, sourcing the schema from the
  same definitions `tools/list` serves for the current profile/namespace (no
  second copy of any schema).
- Top-level only: `required` presence and unknown names against `properties`.
  No type validation, no nested validation.
- A schema whose top level sets `additionalProperties` to anything other than
  `false`/absent-with-properties semantics is exempt from the unknown-name
  check; decide and comment the rule at the site.
- Existing handler-level errors stay unchanged.
- Before enabling the unknown-name check, audit handlers for arguments they
  read but their schema does not declare (legacy aliases, `format`,
  `session_key`, test-only fields). Each such argument is either added to its
  schema (when it is a real caller-facing input) or the finding is reported
  back; never weaken the check silently.

## Verification

- New tests in `internal/mcp`: missing-required (all missing names listed,
  schema JSON present and parseable, handler not run), unknown-argument,
  a valid call unaffected, and the dump contains only the called tool.
- Full `go test ./internal/mcp ./internal/wsrsrc -count=1 -timeout 40m` green
  (internal/mcp takes ~5-8 min); existing tests that send undeclared arguments
  are fixed by declaring the argument or correcting the test, with the reason
  in the commit.

## Boundary

Return to the lead before committing if the audit finds a handler argument
whose right resolution is unclear (caller-facing vs internal), or if the
change would alter any shipped playbook/skill text.
