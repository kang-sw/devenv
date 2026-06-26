# Survey: 22-260620-ticket-status-tools

## Reusable Components

- `agents-plugin-tool/internal/wsdoc/tickets.go#L12` — `ticketStemRE`: `^\d{6}-[\w-]+$` compiled regexp; reuse directly in `findTicketPath` validation and the new tools' guard checks.

- `agents-plugin-tool/internal/wsdoc/tickets.go#L211-L222` — `normalizeTicketStatus`: maps "done"/".done", "dropped"/".dropped", "idea", "todo", "ready", "wip" to canonical forms; reuse to normalize `status` argument in `TicketsClose`.

- `agents-plugin-tool/internal/wsdoc/tickets.go#L144-L178` — `scanTickets` / `ticketsRoot` pattern: `filepath.Join(root, "ai-docs", "tickets")` is the anchored base; `findTicketPath` must replicate this base path with the same os.Stat check.

- `agents-plugin-tool/internal/wsdoc/tickets.go#L243-L275` — `readTicket`: reads frontmatter, phases, stems, specs; available if `TicketsMoveUpwardBlockedBySageReview` tests need to verify `sage-review` field. The `frontmatter()` parser is the production read path.

- `agents-plugin-tool/internal/wsdoc/frontmatter.go#L8-L66` — `frontmatter(path)`: read-only YAML-lite scanner; confirms the "no YAML library" approach and shows exactly how `---`/`---` fences are detected. The write helper in `tickets_mutate.go` must parse the same structure without importing this function (it may call it or replicate the fence logic — the brief says no changes to `frontmatter.go`).

- `agents-plugin-tool/internal/wsdoc/project_tree.go#L253-L270` — `sortedEntries` + `isDir`: package-private helpers usable within the new file (same package `wsdoc`).

- `agents-plugin-tool/internal/wsgit/git.go#L20-L34` — `Runner` interface + `ExecRunner{}`: exact signature `RunGit(ctx context.Context, root string, args ...string) ([]byte, error)`. The local `GitRunner` interface in `tickets_mutate.go` must match this signature to allow passing `wsgit.ExecRunner{}` from the server layer without importing `wsgit` in `wsdoc`.

- `agents-plugin-tool/internal/mcp/server.go#L460-L470` — `config.show` resolver pattern: `adapter := sessionConfigAdapter{s: s.sessions}; r := wsconfig.NewResolver(wsconfig.Options{}, nil, adapter, adapter); view, err = wsconfig.ScopedShow(...)` — this is the exact inline-resolver pattern the brief asks `tickets.move` to replicate, using `r.Get(sessionKey, "sage_review")`.

- `agents-plugin-tool/internal/wsdoc/tickets_test.go#L114-L142` — `stems`, `findTicket`, `joined` helper funcs; available for reuse in `tickets_mutate_test.go` (same package).

- `agents-plugin-tool/internal/wsdoc/project_tree_test.go#L170-L189` — `mustWrite` helper (same package, available in test file); also `runGit` helper if integration-style tests need a real git repo.

## Existing Patterns

- **MCP dispatch case pattern**: see `agents-plugin-tool/internal/mcp/server.go#L767-L827` (`tickets.list`, `tickets.find`, `tickets.status` cases) — each calls `s.resolveToolRoot(params.Arguments, params.Meta)`, extracts typed args from `params.Arguments`, calls wsdoc function, returns `toolTextResponse`. New `tickets.close` and `tickets.move` cases follow the same shape.

- **`rootAwareToolSchemaRequiresSessionKey` registration**: see `agents-plugin-tool/internal/mcp/server.go#L2696-L2712` — a flat switch case list. Both `tickets.close` and `tickets.move` must be added here so `withRootAwareToolSchemas` injects the `session_key` property and required constraint into their schemas automatically.

- **`tools()` schema list insertion point**: see `agents-plugin-tool/internal/mcp/server.go#L2471-L2485` — `tickets.status` schema ends at line 2485. New `tickets.close` and `tickets.move` entries go immediately after.

- **CLI subcommand wiring**: see `agents-plugin-tool/cmd/ws-mcp/main.go#L503-L601` — `ticketsCommand` dispatch switch (`list`, `find`, `status` cases), `ticketsUsage` string, individual handler functions. `ticketsClose` and `ticketsMove` follow the same `flag.NewFlagSet` + `wsdoc.XXX` + `printTextOrFatal` pattern.

- **`runtimeCapabilityCommandNames` list**: see `agents-plugin-tool/cmd/ws-mcp/main.go#L197-L242` — sorted literal slice. New CLI mirror names `tickets.close` and `tickets.move` must be inserted here to keep the contract test green.

- **Contract test cross-check**: see `agents-plugin-tool/cmd/ws-mcp/main_test.go#L48-L92` — builds the binary, calls `runtime capabilities`, compares tools and commands to `agents-plugin/runtime.json`. Any tool or command added to the binary that is not in runtime.json fails this test; conversely, any entry in runtime.json missing from the binary also fails.

- **`agents-plugin-wsflow/runtime.json` exact-match**: see `agents-plugin-tool/cmd/ws-mcp/main_test.go#L140-L174` — same binary run under `WS_MCP_NO_AGENT=1` compared to `agents-plugin-wsflow/runtime.json` with `match: exact`. `tickets.close` and `tickets.move` are non-exec, non-mercenary tools, so they must appear in wsflow runtime.json too.

## Relevant Interfaces

- `agents-plugin-tool/internal/wsgit/git.go#L20-L22` — `Runner` interface: `RunGit(ctx context.Context, root string, args ...string) ([]byte, error)`. The local `GitRunner` in `tickets_mutate.go` must declare an identical signature. `wsgit.ExecRunner{}` satisfies both without an import.

- `agents-plugin-tool/internal/wsconfig/resolver.go#L76-L108` — `Resolver.Get(sessionKey, itemKey string) (ResolvedValue, error)`: returns `ResolvedValue{Value string, Scope Scope}`. The `sage_review` check reads `resolved.Value` and handles `""` as off.

- `agents-plugin-tool/internal/mcp/session_config_adapter.go` — `sessionConfigAdapter{s: s.sessions}` satisfies both `wsconfig.SessionReader` and `wsconfig.SessionWriter`; already used inline at five callsites in `server.go`.

- `agents-plugin-tool/internal/mcp/server.go#L2054-L2074` — `toolTextResponse` / `toolErrorTextResponse`: error responses use `isError: true` with a `content[0].text` body — this is the correct shape for guard failures (not JSON-RPC errors).

## Constraints

- **`wsdoc` must not import `wsgit`**: the package dependency direction in this repo is `mcp → wsdoc` and `mcp → wsgit`; adding `wsgit` to `wsdoc` imports would create a cycle or violate the dependency boundary. Define `GitRunner` locally in `tickets_mutate.go`.

- **`writeFrontmatterField` fence detection edge case**: `frontmatter.go#L18-L26` uses `strings.TrimSpace(lines[i]) == "---"` for the closing fence. The write helper must use the same comparison rather than a prefix-match to avoid mis-identifying a `---`-prefixed body heading as the closing fence.

- **Stem date-prefix immutability**: the destination filename is always `filepath.Base(oldPath)` — the stem is extracted from the filename and never rewritten. No caller-supplied stem transformation is allowed.

- **`git mv --force`**: the brief requires `--force`; this prevents git from rejecting the move when a working-tree rename was already done (e.g., if the file was manually moved). Without `--force`, `git mv` fails if the destination exists.

- **`atomicGitMove` path format**: brief says "both paths are repo-relative (forward slash)". The `wsgit.ExecRunner.RunGit` implementation prepends `-C root` so the git command runs from the root. Paths passed to `git add` and `git mv` must be root-relative (forward-slash), not absolute.

- **`noAgentHiddenTool` check**: `tickets.close` and `tickets.move` are non-mercenary, non-exec tools; they are NOT hidden in no-agent mode. No changes needed to `noAgentHiddenTool`.

- **`filterNoAgentCommands` check**: see `agents-plugin-tool/cmd/ws-mcp/main.go#L244-L253` — filters only `mercenary.*` and `config.agents-tier`. New CLI commands pass through untouched.

- **wsflow `commands` section already contains `tickets.*`**: confirmed at `agents-plugin-wsflow/runtime.json#L55-L57`. Brief rule "add to commands if tickets.* are already there" is satisfied — both new tools must be added to wsflow `commands` as well.

## Risk Signals

- `agents-plugin-tool/internal/wsdoc/tickets.go#L211-L222` — Possible **reuse risk**: `normalizeTicketStatus` accepts "wip" as a valid status, but "wip" is not a valid close or move target per the brief. The guard in `TicketsClose` for `status not in {done, dropped}` and `TicketsMove` for `to not in {idea, todo, ready}` must be explicit rather than delegating to `normalizeTicketStatus`. Using it unchecked could silently accept "wip" as a move target.

- `agents-plugin-tool/internal/wsdoc/tickets.go#L144-L157` — Possible **path construction risk**: `scanTickets` constructs `ticketsRoot` without checking whether `root` is absolute. The `findTicketPath` helper must also accept an absolute root (passed from `resolveToolRoot`), which is always absolute per `canonicalGitRoot`. No issue in production, but test code that uses `t.TempDir()` (absolute) is consistent.

- `agents-plugin-tool/internal/mcp/server.go#L2672-L2694` — Possible **schema injection risk**: `withRootAwareToolSchemas` iterates `tools()` and injects `session_key` via `rootAwareToolSchemaRequiresSessionKey`. If `tickets.close` and `tickets.move` are omitted from that function's switch, the MCP schema will expose them without `session_key` in `required`, breaking the contract that all root-aware tools require it. This would be a silent bug — the dispatch would still work but the schema would be wrong.

- `agents-plugin-tool/cmd/ws-mcp/main_test.go#L140-L174` — Possible **wsflow exact-match contract risk**: the `TestRuntimeCapabilitiesCommandReportsWsflowContractSurface` test compares the no-agent tool list to `agents-plugin-wsflow/runtime.json` with `match: exact`. If `tickets.close`/`tickets.move` are added to the MCP `tools()` list (which is always exported by the binary) but omitted from `agents-plugin-wsflow/runtime.json`, the test will fail — even though the tools do appear in no-agent mode. This is the main correctness gate for wsflow.

- `agents-plugin-tool/internal/wsdoc/frontmatter.go#L14` — Possible **fence detection quirk**: `frontmatter.go` detects the opening fence with `strings.HasPrefix(text, "---")` (no newline check), so a file starting with `---more-text` would match. The `writeFrontmatterField` implementation should use the same lenient prefix check for the opening fence to stay consistent, but must be careful about the closing fence scan.

- Brief `## Details#Status directory mapping` line — **Typo risk signal**: the brief states `idea/ → "idea-docs/tickets/idea/"` which is clearly a typo (should be `ai-docs/`). The implementer must use `ai-docs/tickets/idea/` as confirmed by `scanTickets` at `tickets.go#L145`. This is a copy-paste error in the brief, not a codebase issue.

## Opinion

- The `writeFrontmatterField` algorithm description in the brief does not address the case where the file has no frontmatter at all (no opening `---`). The existing `frontmatter.go` silently returns nil in that case. `TicketsClose` should return an error if `writeFrontmatterField` finds no frontmatter fences, since writing `completed:` into a malformed ticket is ambiguous. The brief does not specify this guard — implementer should decide or ask lead.

- The brief specifies `context.Background()` inside `TicketsClose`/`TicketsMove`. This is consistent with the pattern in `wsgit.Client` methods but means long-running git operations cannot be cancelled by the MCP request cancellation context. This is acceptable for a local staging operation.

- The `TicketMoveOptions.Today` field is described as "for tip text, not currently used but kept for symmetry". The brief keeps it for forward-compatibility (e.g., if future versions want to write a moved-at date to frontmatter). Implementer should confirm the field is in the struct but truly unused in the initial implementation.
