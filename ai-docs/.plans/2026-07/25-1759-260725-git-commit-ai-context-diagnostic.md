# Plan: 260725-idea-ws-git-commit-rename-and-payload-rejections — Phase 2: Report the real ai_context constraint

## Relevant Ticket Contract

- Diagnostic-first: do not assume a fixable server-side size limit exists. The
  originating report captured no evidence the server ever received the call.
- Deliverable: record received `ai_context` argument sizes (e.g. via
  `runtime.debug_events`) so **absent-field**, **empty-array**, and
  **all-blank-entries** become distinguishable at the point the emptiness error
  is raised; make the error name which of those actually occurred instead of
  collapsing them into `ai_context requires at least one entry`.
- If the audit surfaces a real limit, report it with the limit and the
  offending field. Otherwise do not resolve this by asking callers to write
  less — `## AI Context` is the project's decision-rationale tier.
- Independent of Phase 1 (already closed via `78bf2e11`) — no ordering
  dependency.
- Verification is behavioral only, because the originating payload cannot be
  reproduced byte-for-byte: a large valid array commits; an absent field, an
  empty array, and an all-blank-entry array each report their own distinct
  condition; a regression test pins whichever behavior is chosen.

## Out of Scope

- Phase 1 (staged ticket rename verifier fix) — already resolved and
  independent; not touched here.
- Chasing a host- or model-side transport/tool-input truncation cause outside
  this repo. The ticket explicitly frames this as unprovable from here; the
  deliverable is making the server's own received-state distinguishable and
  honestly reported, not proving where upstream truncation happens.
- Raising or otherwise changing the `bufio.NewScanner` 64KiB token cap
  (`internal/mcp/server.go:143`). Already tracked as Technical Debt in
  `ai-docs/mental-model/mcp-runtime.md:133`, and the ticket's own arithmetic
  rules it out as this bug's cause (eight prose bullets are far below 64KiB,
  and hitting the cap kills the connection rather than producing a JSON-RPC
  error response at all, so it cannot be the source of the observed
  `ai_context requires at least one entry` reply).
- Adding a new, real size ceiling on `ai_context`. Confirmed absent today (see
  Codebase Findings) and the ticket forbids inventing one as the fix.
- Changing the underlying requirement that `ai_context` have at least one
  non-blank entry — that requirement itself is correct and stays; only the
  error's specificity changes.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/server.go:3849-3865` — `git.commit`'s
  `inputSchema` has no `maxLength`/size constraint on `ai_context`
  (`stringArrayProperty` is a plain array-of-string schema). Confirms the
  ticket's audit: no schema-level cap.
- `agents-plugin-tool/internal/mcp/server.go:143` — `ServeStdio` calls
  `bufio.NewScanner(in)` with no `scanner.Buffer(...)` override anywhere in
  the file (confirmed via grep), so the default `bufio.MaxScanTokenSize`
  (64KiB) cap is live. Per the ticket's own math this is not the trigger for
  the reported failure and is out of scope to change.
- `agents-plugin-tool/internal/mcp/server.go:4794-4807` — `stringList(value any) []string`:
  returns `nil` (early return) when the type assertion `value.([]any)` fails —
  which is exactly what happens both when the JSON key is absent from
  `params.Arguments` and when it is explicitly `null`. When the value **is**
  a JSON array (including `[]`), it returns a non-nil `make([]string, 0, len(items))`,
  filtering only exact empty-string items (`text != ""`), not whitespace-only
  ones. **This means the nil-vs-non-nil-empty distinction between "field
  absent" and "field present as `[]`" already exists at this call site and is
  preserved into `CommitOptions.AIContext`** (direct in-process struct
  assignment at `server.go:1047`, no JSON round-trip) — it is simply never
  read again downstream. This is the reusable signal the fix should key off,
  not a size measurement.
- `agents-plugin-tool/internal/mcp/server.go:1036-1052` — the `git.commit`
  case in `callTool`: `AIContext: stringList(params.Arguments["ai_context"])`
  is the sole call site that both the MCP tool path and (indirectly, via
  `wsgit.Client.Commit`) the eventual error share. This is the natural place
  to capture raw-argument diagnostics for `runtime.debug_events`, since
  `appendDebugEvent` (below) only exists in this package.
- `agents-plugin-tool/internal/mcp/server.go:263-292` — `appendDebugEvent(event string, fields map[string]any)` already exists: appends to an in-memory 256-entry
  ring (`debugEvents`, `maxDebugEvents = 256` at `:113`) and, if
  `WS_MCP_DEBUG_LOG` is set, mirrors to a file. `recentDebugEvents` (`:405`)
  and `debugEventsJSONL` (`:420-431`) back the existing `runtime.debug_events`
  tool (schema at `:3233-3242`, dispatch at `:538-540`). No new plumbing is
  needed — only a new call site with the right fields.
- `agents-plugin-tool/internal/mcp/server_test.go:1085-1116` — existing
  pattern for asserting on `runtime.debug_events` output in a server-level
  test (`byID["8"]`, checks a JSONL line via `strings.Contains(... , '"event":"request.received"')`). Reuse this pattern for a new assertion on
  the new event name/fields.
- `agents-plugin-tool/internal/wsgit/git.go:423-432` — `CommitOptions.AIContext []string`. No `mcp` import anywhere in this file (confirmed via read) —
  `wsgit` is intentionally free of an `internal/mcp` dependency (see the
  `Verifier` doc comment at `git.go:36-44` citing
  `{#260720-wsdoc-commit-boundary}` for the same layering rule applied to
  `wsdoc`). **This means `appendDebugEvent` cannot be called from inside
  `wsgit`; the debug-event capture must live in `internal/mcp/server.go`, and
  the error-message differentiation must live in `wsgit` so both entry points
  below get it.**
- `agents-plugin-tool/internal/wsgit/git.go:508-536` — `normalizeCommitOptions`:
  `opts.AIContext = trimStrings(opts.AIContext)` (line 511) runs *before* the
  emptiness check at line 522-524. `trimStrings` (`git.go:758-767`) always
  returns a fresh `make([]string, 0, len(values))` regardless of whether its
  input was nil or non-nil-empty, which is exactly where the nil/len signal
  described above gets thrown away today. This is the fix point: capture
  `opts.AIContext` (pre-trim) before line 511 overwrites it, then branch the
  line 522-524 error on that captured value's nil-ness / length instead of
  only on the post-trim length.
- `agents-plugin-tool/internal/wsgit/git.go:451-506` — `Client.Commit` is the
  single call path both the MCP tool handler and the CLI mirror below share
  (`normalizeCommitOptions` runs first, at line 452) — confirms a `wsgit`-level
  fix covers both entry points without duplicating logic.
- `agents-plugin-tool/cmd/ws-mcp/main.go:462-507` (`gitCommit`) — the CLI
  mirror. `var aiContext multiFlag` (`main.go:964-973`, a bare `[]string` with
  `Set` appending on each `-ai-context` flag) has zero value `nil`. `_ =
  fs.Parse(args)` (`main.go:481`) silently discards a parse error, leaving
  `aiContext` at its pre-parse value (`nil` if no `-ai-context` flags were
  successfully consumed before the error). This CLI path calls
  `wsgit.Client{...}.Commit(...)` directly (`main.go:492-501`) — it never goes
  through `internal/mcp/server.go`'s `callTool`, so it has **no access to
  `appendDebugEvent`/`runtime.debug_events`** at all. This is corroborating
  evidence for the ticket's "field never arrived" theory, and it is the
  concrete reason the error-message fix must live in `wsgit.normalizeCommitOptions`
  (shared) rather than only in the MCP server layer — a CLI-only bug already
  gets the correct diagnostic through the differentiated error text even
  though it can never emit a debug event.
- `agents-plugin-tool/internal/wsgit/git_test.go:468-477` —
  `TestCommitRequiresAIContextAndRelativePaths` is the existing regression
  test for the current generic message; its first case
  (`CommitOptions{Paths: []string{"src"}, Title: "feat: x"}`, `AIContext`
  field left unset ⇒ nil) already exercises the "absent" condition and only
  asserts a generic `strings.Contains(err.Error(), "ai_context")`. Extend or
  replace this test with three explicit sub-cases (absent/nil, `AIContext:
  []string{}`, `AIContext: []string{"   ", "\n"}`) each asserting a distinct
  message.
- `agents-plugin-tool/internal/wsgit/git_test.go:224-268` — existing pattern
  for a `TestCommit*` success case (`TestCommitStagesExplicitPathsAndBuildsMessage`)
  with a real `wsgit.Client{Runner: <fake>, Verifier: <stub>}` — reuse this
  shape for the "large valid array commits" behavioral test, with an
  `AIContext` slice of many long entries (e.g. 50+ entries, several KB total)
  asserting no error and that the commit message contains all entries.
- `ai-docs/spec/mcp-tools.md:1125-1182` — the `git.commit` spec section and
  its anchor convention (e.g. `{#260725-git-commit-verify-excludes-delete-side-paths}`
  from the Phase 1 closeout). Phase 2 needs an analogous new anchor
  documenting that `ai_context` has no size limit and that the emptiness
  error now distinguishes absent/empty/blank, per the ticket's own Spec
  Impact section (non-contract-first: corrects the spec to match the fixed
  tool at closeout).
- `ai-docs/mental-model/mcp-runtime.md:131-133` — existing Technical Debt
  bullet about the `bufio.Scanner` cap; no change needed, just confirms this
  survey's out-of-scope call is consistent with already-recorded project
  knowledge.

## Implementation Plan

1. **`agents-plugin-tool/internal/wsgit/git.go`, `normalizeCommitOptions`
   (~508-536)**: before line 511 overwrites `opts.AIContext`, save the
   pre-trim value (e.g. `rawAIContext := opts.AIContext`). Keep the existing
   `opts.AIContext = trimStrings(opts.AIContext)` line unchanged. Replace the
   single `if len(opts.AIContext) == 0 { return ..., fmt.Errorf("ai_context requires at least one entry") }`
   at lines 522-524 with a three-way branch on `rawAIContext`:
   - `rawAIContext == nil` → field was absent (or explicitly `null`): error
     names that condition (e.g. `"ai_context is required: no ai_context field was received"`).
   - `rawAIContext != nil && len(rawAIContext) == 0` → present but an empty
     array: error names that condition (e.g. `"ai_context requires at least one entry: received an empty array"`).
   - otherwise (`len(rawAIContext) > 0` but all entries trimmed away) →
     error names that condition and includes the entry count (e.g.
     `"ai_context requires at least one non-blank entry: received %d entr%s, all blank"`,
     `len(rawAIContext)`). Use a small local pluralize helper or inline
     ternary; do not add a new exported helper unless reused elsewhere.
   Keep all three messages prefixed distinctly enough that
   `strings.Contains(err.Error(), "ai_context")` (used by existing/adjacent
   tests) still matches all three, and that no message text overlaps with
   `"paths requires at least one path"`.

2. **`agents-plugin-tool/internal/mcp/server.go`, `git.commit` case (~1036-1052)**:
   before building `wsgit.CommitOptions`, capture raw `ai_context` argument
   diagnostics directly off `params.Arguments` (not off the post-`stringList`
   value): presence via comma-ok map lookup, raw entry count when it is a
   JSON array, and total UTF-8 byte length of the raw string entries. Emit one
   `appendDebugEvent("git.commit.ai_context_received", map[string]any{...})`
   call unconditionally for every `git.commit` invocation (matching the
   existing always-on `"request.received"` precedent at `server.go:178`, so
   the ring buffer captures both successful and rejected calls without
   depending on error-string matching). Suggested fields: `"present"` (bool),
   `"raw_entry_count"` (int, -1 or omitted if not an array), `"raw_bytes"`
   (int), `"post_trim_entry_count"` (int, computed from the same
   `stringList`+trim result already produced for `CommitOptions.AIContext`).
   This directly implements the ticket's "record received argument sizes ...
   via `runtime.debug_events`" ask, and is additive — it does not change
   `git.commit`'s return value or error text; that comes entirely from step 1.

3. **`agents-plugin-tool/internal/wsgit/git_test.go` (~468-477)**: replace
   `TestCommitRequiresAIContextAndRelativePaths`'s first assertion with three
   explicit sub-tests (or `t.Run` subtests) pinning the exact new messages
   for: `AIContext` field unset (nil), `AIContext: []string{}`, and
   `AIContext: []string{"   ", "\n"}`. Keep the existing
   `"../outside"`/`"inside the repository"` assertion in the same test (or
   split it out) since it is unrelated to this phase.

4. **`agents-plugin-tool/internal/wsgit/git_test.go`** (new test near the
   other `TestCommit*` success cases, e.g. after line 268): add a "large
   valid array commits" behavioral test — call `Client.Commit` with an
   `AIContext` of many long, non-blank entries (well past any prior 64KiB
   confusion, e.g. several KB total) through the existing fake
   `Runner`/`Verifier` test doubles, and assert no error and that the
   resulting commit message includes the entries. This pins the ticket's
   first verification bullet ("a large valid array commits") at the `wsgit`
   level, independent of any transport.

5. **`agents-plugin-tool/internal/mcp/server_test.go`** (near the existing
   `git.commit` coverage around lines 1708-1116 and the `runtime.debug_events`
   assertion pattern at 1085-1116): add or extend a test that issues a
   `git.commit` `tools/call` with `"ai_context": []` and asserts the JSON-RPC
   error text names the empty-array condition (not the old generic message),
   and a second call with the `ai_context` key omitted entirely asserting the
   absent-field condition. Optionally follow either with a
   `runtime.debug_events` call asserting the new
   `"git.commit.ai_context_received"` event is present with matching
   `present`/`raw_entry_count` fields, mirroring the existing
   `byID["8"]`/`request.received` assertion pattern.

6. **`ai-docs/spec/mcp-tools.md`** (`git.commit` section, ~1125-1182): at
   closeout, add a new anchored bullet documenting that `ai_context` has no
   size limit in the tool itself, and that the emptiness error now
   distinguishes absent-field, empty-array, and all-blank-entries conditions
   by name. Follow the existing anchor-id convention (date-prefixed,
   kebab-case, e.g. `{#260725-git-commit-ai-context-condition-reporting}`).
   Update `ai-docs/mental-model/mcp-runtime.md` only if the new
   `appendDebugEvent` call site needs a one-line invariant note; the existing
   Technical Debt bullet about the scanner cap needs no change.

## Verification Plan

- `go test ./agents-plugin-tool/internal/wsgit/... ./agents-plugin-tool/internal/mcp/...`
  (or `cd agents-plugin-tool && go test ./...`) covering the new/updated
  tests in steps 3-5.
- `go build ./... && go vet ./...` from `agents-plugin-tool/`.
- Behavioral checks pinned by the new tests, matching the ticket's
  verification boundary exactly (payload cannot be reproduced byte-for-byte,
  so verification is behavioral, not a byte-for-byte repro):
  - A large valid `ai_context` array commits successfully (step 4).
  - An absent `ai_context` field, an empty array, and an all-blank-entry
    array each produce their own distinct, named error condition — not the
    same collapsed "requires at least one entry" text (steps 1, 3, 5).
  - A regression test exists per condition so the emptiness message cannot
    silently start covering more than one condition again (steps 3, 5).
- Manual/optional: a live MCP session calling `git.commit` with a genuinely
  large `ai_context` (dozens of long entries) followed by `runtime.debug_events`
  to visually confirm the recorded `raw_bytes`/`raw_entry_count` look sane —
  useful as a sanity check but not required for the automated verification
  boundary above.

## Escalations

- None.
