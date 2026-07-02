# Survey: 22-260622-create-ticket-tool

## Reusable Components
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L18-L45` — `TicketCloseOptions`/
  `TicketMoveOptions`/`TicketMutateResult`: the options+result struct shape the brief's
  `TicketCreateOptions`/`TicketCreateResult` should mirror (incl. `Today` for testability).
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L171-L173` — `ticketRelPath(statusDir, stem)`:
  builds `ai-docs/tickets/<dir>/<stem>.md` forward-slash path. Reuse to compute the return path
  (pass `<Today>-<Stem>` as stem). Eliminates re-deriving the path-join convention.
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L38-L45` — `statusDirs` map: `idea/todo/ready`
  map to identical dir names, so no special-casing is needed for the create directories.
- `agents-plugin-tool/internal/mcp/format.go#L45-L47` — `FormatTicketMutate` exported wrapper over
  the unexported `formatTicketMutate`: this is the exact pattern the brief's `FormatTicketCreate`
  should follow (exported wrapper in format.go).

## Existing Patterns
- MCP dispatch: `agents-plugin-tool/internal/mcp/server.go#L828-L871` (`tickets.close`/`tickets.move`)
  — resolve root via `s.resolveToolRoot(params.Arguments, params.Meta)`, extract string args with
  `params.Arguments["k"].(string)`, call wsdoc, return `toolTextResponse(req.ID, formatX(...), err)`.
  Note: `tickets.move` reads `session_key` only to resolve `sage_review` config; create needs no
  config lookup, so the simpler `tickets.close` dispatch shape is the closer template.
- Tool schema: `agents-plugin-tool/internal/mcp/server.go#L2553-L2564` (`tickets.move` schema block)
  — `stringProperty(...)` per field, `"required": []string{...}`.
- `rootAwareToolSchemaRequiresSessionKey`: `agents-plugin-tool/internal/mcp/server.go#L2775-L2782`
  — add the new tool name to this string switch list (currently includes `tickets.close`, `tickets.move`).
- CLI mirror: `agents-plugin-tool/cmd/ws-mcp/main.go#L610-L648` (`ticketsClose`/`ticketsMove`) use
  `flag.NewFlagSet`, a `--root` flag with `defaultRoot(*root)`, positional fallback
  (`if *stem=="" && len(fs.Args())>0`), then `printTextOrFatal`. Add a `create` case at
  `main.go#L506-L523` (`ticketsCommand` switch) and update the usage string at `#L528-L529`.
- CLI command registry: `agents-plugin-tool/cmd/ws-mcp/main.go#L198-L239` `runtimeCapabilityCommandNames()`
  uses dotted names (`tickets.close`, `tickets.move`), NOT the CLI argv form (`tickets close`).

## Relevant Interfaces
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L171-L173` — `ticketRelPath`.
- `agents-plugin-tool/internal/mcp/server.go#L2775-L2782` — `rootAwareToolSchemaRequiresSessionKey`.
- `agents-plugin-tool/cmd/ws-mcp/main.go#L198-L239` — `runtimeCapabilityCommandNames`.
- `agents-plugin/runtime.json#L8-L37` (tools) and `#L81-L107` (commands); wsflow mirror
  `agents-plugin-wsflow/runtime.json#L9` (`"match": "exact"`), tools `#L31-L35`, commands `#L57-L61`.

## Constraints
- The contract test `agents-plugin-tool/cmd/ws-mcp/main_test.go#L140-L173` compares the LIVE
  `runtime capabilities` output (derived from `tools()` + `runtimeCapabilityCommandNames()`)
  against `agents-plugin-wsflow/runtime.json` keys EXACTLY. The runtime.json **tools** key must
  equal the MCP tool name registered in `tools()`; the **commands** key must equal the entry added
  to `runtimeCapabilityCommandNames()`. The same surface also feeds the full-ws contract test
  (`main_test.go#L48`, reads `agents-plugin/runtime.json`), so BOTH runtime.json files must carry
  matching entries or one of the two contract tests fails.
- `runtimeCapabilityCommandNames` and the runtime.json `commands` section use **dotted** names
  (e.g. `tickets.close`), while the CLI argv form is space-separated (`tickets close`). The
  command-registry/runtime.json entry for the new CLI mirror is therefore `tickets.create`, not
  the argv string and not the tool name.
- `frontmatter` parser (`agents-plugin-tool/internal/wsdoc/frontmatter.go#L40-L44`) reads
  `title: ""` as an empty map, not the string `""`. The brief's tests assert raw file substrings
  (`title: ""` present / `sage-review` present/absent), so they pass; but any future code reading
  the stub's title via `frontmatter(...)["title"].(string)` (e.g. `project_tree.go#L246`) gets the
  empty-string zero value from a failed assertion, not a literal empty title — benign here.
- `tickets_mutate.go` validates stems against `ticketStemRE = ^\d{6}-[\w-]+$` (`tickets.go#L12`).
  Create receives only the semantic stem (no date), so it must NOT validate the raw `Stem` against
  this regex; the brief's contract (reject only empty stem) is correct. No date-prefix collision
  guard exists beyond the brief's "error if path exists" rule.

## Risk Signals
- `ai-docs/.plans/2026-06/22-260622-create-ticket-tool.brief.md#L134-L145` — Possible contract/naming
  risk: the brief instructs adding `"create_ticket"` to BOTH the `tools` and `commands` sections of
  both runtime.json files. Every existing ticket entry in the **commands** section uses the dotted
  CLI form (`tickets.close`, `tickets.move`; `agents-plugin/runtime.json#L97-L101`), and the CLI
  mirror the brief defines is `tickets create` → registry name `tickets.create`. Adding
  `create_ticket` to the **commands** section would not match `runtimeCapabilityCommandNames()` and
  would fail the exact-match contract test. The **tools** section entry `create_ticket` is correct
  only if the tool is genuinely registered top-level as `create_ticket` (vs. the `tickets.*` family
  convention used by every other ticket tool). Lead/planner should confirm the intended tool name
  (`create_ticket` vs `tickets.create`) and the correct commands-section key before implementation.
- `ai-docs/.plans/2026-06/22-260622-create-ticket-tool.brief.md#L101-L110` — Possible contract risk:
  the tool name `create_ticket` is inconsistent with the established `tickets.{list,find,status,
  close,move}` namespace (`server.go#L2541-L2564`). Not a logic defect, but a public-surface naming
  decision; if `tickets.create` is preferred, every brief reference to `create_ticket` (schema,
  dispatch, runtime.json tools key, `rootAwareToolSchemaRequiresSessionKey`) shifts accordingly.

## Opinion
- The brief is otherwise self-contained and the pure-logic layer is low-risk (stdlib-only file
  write, no git, mirrors `TicketClose` shape). The single material ambiguity is the tool/command
  naming (`create_ticket` vs `tickets.create`) and the runtime.json **commands** key, which the
  exact-match contract test will hard-fail if mis-keyed. Confirm naming before coding; everything
  else is mechanical and well-specified.
