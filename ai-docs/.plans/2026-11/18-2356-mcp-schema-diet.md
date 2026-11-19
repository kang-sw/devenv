# MCP tool schema diet (branch `schema-diet`)

## Outcome

Every model-visible ws MCP tool fits a 2,000-byte budget, enforced by a test,
and two adapter-only tools leave the model tool list while staying callable.
All decisions below are settled with the owner (whiteboard items 141-146);
apply them, do not redesign them.

Worktree: `/Users/kang-sw/.ws-worktrees/devenv/crumpled-kinswoman-retail`,
branch `schema-diet`, based on `77a778437` (track/agent-v2 tip). Commit on
`schema-diet` only. Do not push and do not merge anything; the lead does both.

## Work items

1. **Apply ten drafted descriptions verbatim.** `/tmp/schema-diet/all-drafts.json`
   maps tool name -> `{description, properties}`. `description` replaces the
   tool description. Each `properties` key is an input property whose
   `description` it replaces; a dotted key `a.b` is property `b` inside the
   array items of property `a` (git.commit `followups.content`,
   `followups.level`; wiki.update `images.data`, `images.name`). Change only
   the description strings: types, enums, required lists, defaults, minimum
   and maximum stay as they are. Tools: tickets.query, wiki.update,
   config.tune, wiki.query, git.merge, git.commit, worktree.acquire,
   rationale.query, playbook.render, tickets.acquire. Most live in
   `agents-plugin-tool/internal/mcp/server_schemas.go`; grep for the others
   (wiki.* is defined elsewhere). The drafts were sized against a dump taken
   from an earlier develop; if a tool's schema on this base gained or lost a
   property, keep the new property's existing description and report it.
2. **session_key batch.** Every input property `session_key` whose
   description is exactly `Caller's ws session key (see ws:workflow-manual).`
   becomes `Your ws session key.` (probably one shared constant or helper;
   change it at the source, not per tool). Leave differently worded
   session_key descriptions alone except where item 1 sets them.
3. **Unlisted-but-callable mode.** `dashboard.intake` and
   `dashboard.inbox_drain` (`live_snapshot.go` `dashboardIntakeTool` and its
   sibling) are called by adapters by name
   (`agents-plugin/hooks/claude-mod/dashboard.ts`, `agents-plugin-pi/src/*`),
   never by the model. Add a registry-level mode in which a tool is omitted
   from every model-facing tool list but its calls are still accepted.
   - The existing hidden-tool mechanism (`WS_MCP_ALLOWED_TOOLS`,
     `TestHiddenToolsListedOnlyWhenAllowlisted`; tracker.show, tracker.ui,
     git.search) also REFUSES calls, so it is not this mode. Do not change
     its behavior.
   - The Codex and agy alias surfaces on this branch build their tool lists
     from the same registry (agy's underscore aliases come from the harness
     descriptor, see commit e69a4a620). An unlisted tool must stay off those
     alias lists too, and a call through its alias must still be accepted.
   - First verify the adapter call paths: read every caller of these two
     tools in `agents-plugin/hooks/` and `agents-plugin-pi/src/` and confirm
     none discovers them through tools/list (or a tool-list cache) before
     calling. If one does, stop and report it as `blocked` with file:line;
     do not change adapter code.
   - Tests: the two tools are absent from tools/list for every harness
     variant and from the Codex and agy alias lists; a canonical-name call
     and an alias call each still reach the handler; a hidden-tool call is
     still refused (existing behavior intact).
4. **Size guard test.** A test in `internal/mcp` that builds the tool list
   the way tools/list serves it (see `callToolsList` in server_test.go) and
   asserts, for every listed tool, `len(json.Marshal(struct{Name,
   Description, InputSchema}))` <= 2000 (Go `encoding/json` defaults, so
   `<`, `>`, `&` count as 6 bytes). Unlisted tools are outside the guard.
   On failure, list every violating tool with its size, largest first, so
   one run shows the whole picture. If the alias surfaces serve separate
   tool lists, cover them too, or explain in the report why they cannot
   exceed the canonical size.
   - Expected: `agent.*` tools (notably agent.spawn) may still exceed 2,000
     B on this base. Another lead's worker is trimming them on a separate
     branch. Do NOT touch any `agent.*` description and do not exempt them;
     let the guard fail on them and report the exact list. Any non-agent.*
     tool over the cap after items 1-2 is yours: trim it under the
     criterion below and report the before/after text.

## Criterion for any trimming you must do yourself

Keep: what the tool does and when to call it; argument semantics that change
behavior; refusals and guards the caller must know. Drop: result-field lists,
feature-flag detail (shrink to "where enabled"), implementation internals,
defaults and ranges the schema already states. English, plain sentences.
These strings ship to downstream agents: read
`ai-docs/manuals/shipped-surface-boundary.md` first.

## Constraints

- Read before editing: `ai-docs/manuals/ws-mcp.md`,
  `ai-docs/manuals/release-compatibility.md`,
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/performance.md`.
- Compatibility tier: T0 for all of it (description-only edits; unlisting
  keeps the call path). Record that in the commit's AI Context. If anything
  you find makes a change non-T0, stop and report.
- Old description text may be mirrored in golden files, fixtures, docs, or
  tests (grep for distinctive phrases of each old description, and for
  `see ws:workflow-manual`). Update test expectations that assert the old
  strings; report any non-test mirror (Pi sources, rsrc, manuals) instead of
  guessing.
- Surgical changes; follow existing style.

## Verification

- `cd agents-plugin-tool && go build ./... && go vet ./internal/mcp`.
- `go test ./internal/mcp -timeout 60m` (it takes ~15+ minutes; run it in
  the background and wait). Expected: all pass except the size guard on
  agent.* tools, if any. Any other failure: check whether it also fails on
  `77a778437` before attributing it.
- `go test` for any other package you touch.
- If you touch `agents-plugin-pi/`: `cd agents-plugin-pi && npm ci
  --ignore-scripts && npm test`; compare failing test names against the base,
  17 failures are pre-existing.

## Return

Commit (one or two logical commits, `## AI Context` included) on
`schema-diet`, then end with the delegate-report block giving: commit SHAs;
per-tool before/after sizes for every tool you changed; the guard's failing
list (expected: agent.* only); the adapter call-path verification with
file:line; any mirrors found; test results with the commands run.
