# Brief: agenda and todo MCP tool descriptions (description-only, T0)

## Outcome

Tidy the agent-facing descriptions of the `agenda.*` and `todo.*` MCP tools so
an agent can tell them apart and knows what each is for. Tool names, argument
names, argument schemas, and behavior stay exactly as they are (T0: text only).

## Settled changes (owner-approved)

1. `agenda.set` (agents-plugin-tool/internal/mcp/server_schemas.go ~line 82):
   - Delete the dangling sentence "Freeform fallback for cases not covered by
     a typed enter.* tool." No `enter.*` tool is registered.
   - State plainly that agenda blobs are this session's working memory (what
     is being done, why, next step, pending question), restored with the
     session key after compaction or resume (via `workflow_state`), and
     cleared when the work ends.
   - Add a one-line boundary with `todo`: an ordered checklist of steps is
     `todo`; context, rationale and pending state are agenda.
   - Keep the existing `note.write` boundary and the `merge` explanation.
   - Drop the jargon "mode context". Also drop "reminded at workflow-manual
     load" unless you confirm it is still accurate, and then say it plainly.
2. `todo.*` descriptions (todo.add, check, list, read, reorder, erase, clear):
   - Evidence: `todo` is used almost only as a worker's step checklist. The
     implement route pre-installs the items (see
     agents-plugin/rsrc/worker-stop-protocol.md ~line 63, "Its todo list is
     your skeleton"), and workers mark them with check/list/read. Agents
     never call add/erase/clear directly.
   - Lead with that purpose on the most-seen todo descriptions, at least
     `todo.list` and `todo.check`. Add a one-line boundary saying session
     context and rationale belong in `agenda.set`, not `todo`.
   - Keep the argument descriptions accurate. Do not remove tools or
     arguments, and do not change validation.
3. Check whether `agenda.list` and `agenda.clear` need a matching wording
   touch for consistency. Change them only if they contradict the new text.

## Inputs to read first

- ai-docs/manuals/shipped-surface-boundary.md: these strings ship to
  downstream agents, so no repo-specific references.
- ai-docs/manuals/skill-authoring.md: the reader model, for terse,
  current-generation wording.
- ai-docs/manuals/release-compatibility.md: confirm this is T0, text only.
- Find every place these description strings are duplicated or pinned
  (golden tests, wsflow mirrors, the Pi package, generated docs) with a
  repo-wide grep for the current sentences. Update the mirrors and pins
  together.

## Permitted actions

- Edit the description strings and any test or golden file that pins them.
- Commit on this branch (impl/develop/agenda-todo-descriptions), one commit,
  with `## AI Context`, and no Co-Authored-By line.
- Do not push, switch branches, touch daemons, run Electron, or run the real
  agy binary.

## Verification

- Prefix every go command with GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local and
  run it from agents-plugin-tool/.
- Run `go build ./...`.
- Run the narrow tests that pin tool descriptions or schemas. Find them by
  grepping for the old strings and for the tools/list golden. Never run the
  whole ./internal/mcp or ./internal/wsdashboard package. If the descriptions
  feed a tools/list snapshot test, run that one test with a -run filter and
  -timeout 20m.
- Run `go vet ./internal/mcp`.
- If you touched agents-plugin-pi, run its suite:
  `npm ci --ignore-scripts && npm test` in agents-plugin-pi. Compare failing
  test names against develop; there are 17 known pre-existing failures.

## Return boundary

Return after the single commit, or earlier if any change would alter a tool
name, an argument, or behavior. That is out of scope: report it instead.
Report the commit sha, the before and after text of each description, every
command run with its result, and every mirror or pin you updated.
