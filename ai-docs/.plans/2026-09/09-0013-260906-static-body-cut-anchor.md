# Plan: 260906-bug-ws-pi-workflow-manual-static-body-cut-never-matches — Phase 1: Anchor cut and real-shaped fixtures

## Relevant Ticket Contract

- Replace `cutStaticBody`'s exact-substring cut with an anchor cut: start
  anchor = the session-start snapshot's first non-empty line, matched as a
  whole line at its first occurrence in the response; end anchor = the
  literal `## Session Key` heading line.
- `cutStaticBody` keeps its `(response, staticBodySnapshot)` signature and
  purity; its return gains a `reason` (`"start-anchor" | "end-anchor" |
  "order" | "no-body"`) beside `text`/`found`.
- Fallback (unchanged trigger shape, changed condition): exactly one anchor
  missing, or end anchor precedes start anchor → dispatch `workflow_state` as
  today, `notifyMappingDegraded(reason)` fires, warning text names the
  missing anchor instead of "renderer drift".
- Neither anchor present (`reason: "no-body"`) → forward the original
  `workflow_manual` response unchanged (fixed mapping line prepended, no
  `workflow_state` dispatch, no warning notification). This is ws-mcp's
  no-restorable-state notice shape.
- Replace the synthetic `"STATIC-BODY"` fixtures with a pair **captured from
  a real ws-mcp render** (`playbook.read("lead-workflow-manual")` +
  `workflow_manual` under one key against this repo), trimmed on the
  session-state/notes tail, committed as fixtures.
- Reshape the two advisory-keying tests that currently rely on a same-line
  substring cut with no `## Session Key` in the fixture, so they keep
  covering the cut-hit path rather than silently sliding into fallback.
- Fallback-dispatch tests must keep passing unmodified.
- Amend spec `pi-adapter-runtime` `{#260905-pi-workflow-manual-state-mapping}`
  Primary/Fallback bullets — **this is the lead's doc pre-pass, not part of
  this implementation's commit.**
- Adapter-only: `agents-plugin-pi/`; no `agents-plugin-tool/` (ws-mcp) change.

## Out of Scope

- Any ws-mcp (`agents-plugin-tool/`) change — `stripModeGatedRegion`,
  `injectSessionKeyLine`, the CONTINUE/FRESH branches in
  `handleWorkflowManual` stay exactly as they are; they are read-only
  evidence for this plan.
- The system-prompt ws block (`{#260905-pi-lead-bootstrap-system-prompt}`) —
  unaffected, carries the full response not the snapshot.
- Worker/explore forwarding (`shouldMapWorkflowManual`'s role gate) — stays
  as-is, already correctly excludes non-lead/fork roles.
- Editing `ai-docs/spec/pi-adapter-runtime.md` from this implementation — the
  spec amendment is a lead-owned doc pre-pass; do not commit a spec edit as
  part of this phase's implementation commit.
- The Phase 1 "Live check (owner-run)" bullet — owner-performed in a real Pi
  lead session, not an automated test this plan drives.
- The `describe("computePiAliasTableReport")` parametrized `"mapped
  cut/fallback"` test (`agents-plugin-pi/test/bridge.test.ts:485-501`) — not
  named by the ticket's Tests list; see risk note below.
- A dedicated unit test for the `"order"` reason branch — required to be
  *implemented* (the reason enum has 4 values per the Constraints), but not
  explicitly listed in Phase 1's Tests bullet as a required test case.

## Codebase Findings

- `agents-plugin-pi/src/bridge.ts#L224-L230` — current `cutStaticBody`:
  `response.indexOf(staticBodySnapshot)`, returns `{ text, found }` only (no
  `reason`). Pure, synchronous. Its doc comment (`#L215-L223`) explicitly
  frames a miss as "renderer drift" — needs rewriting along with the code.
- `agents-plugin-pi/src/bridge.ts#L271-L280` — `WorkflowManualMappingDeps`:
  `staticBodySnapshot: string`, `notifyMappingDegraded: () => void` (no
  reason arg today).
- `agents-plugin-pi/src/bridge.ts#L301-L328` — `dispatchMappedWorkflowManual`:
  today a strict two-way branch on `cut.found`. On miss it unconditionally
  calls `notifyMappingDegraded()` then dispatches `workflow_state` with only
  `session_key` (drops `root`). This unconditional-fallback-on-any-miss shape
  is exactly what must become three-way (`found` / one-anchor-missing-or-order
  / `no-body`).
- `agents-plugin-pi/src/bridge.ts#L638-L658` — call site inside `startBridge`:
  `notifyMappingDegraded` closure (`#L647-L656`) hardcodes the "renderer
  drift" warning string and dedupes via `notifiedMappingDegraded`
  (`#L579`). The closure signature must accept the reason and word the
  message per-reason; the "no-body" case must never reach this closure (no
  warning fires for it).
- `agents-plugin-pi/src/bridge.ts#L715-L734` — session-start snapshot fetch:
  `workflow_manual` call at `#L720-L722` fills `manualSnapshotRef` (used
  elsewhere by `lead-bootstrap.ts`, untouched by this phase);
  `client.callTool("playbook.read", { name: "lead-workflow-manual",
  session_key: ... })` at `#L727-L730` fills `staticBodySnapshotRef` — this
  is the actual snapshot source (ticket text's "`playbook.print(...)`" is
  informal naming for the same `playbook.read` tool call; confirmed no real
  discrepancy, see below). Both fetches are all-or-nothing gated together —
  unaffected by this phase, just the source of the two texts to capture.
- `agents-plugin-tool/internal/mcp/workflow_manual.go#L304-L313` — CONTINUE
  branch: `stripModeGatedRegion(body, false)` → `injectSessionKeyLine` →
  `body += "\n\n## Session Key\n" + key` → `renderSessionState` →
  `computeNotes`, then `injectSkepticalPosture` (prepends
  `skepticalPostureBlock` to the *whole accumulated* body, including the
  already-appended `## Session Key`/state/notes tail).
- `agents-plugin-tool/internal/mcp/workflow_manual.go#L314-L335` and
  `bootstrap_alarm.go#L142-L146`, `doc_coverage_alarm.go#L45-L47`,
  `review_track_alarm.go#L30-L34`, `manuals_announcement.go#L43-L58` —
  every one of the remaining warning/manuals/notes injections is `warning +
  "\n\n" + body` (pure prepend to the front). Confirms the ticket's claim:
  everything ws-mcp adds beyond the raw playbook render prepends to the
  front (never re-emits the render's own opening heading line) or appends
  after the render body (`## Session Key` onward) — nothing splices a
  duplicate copy of the start-anchor line into the middle.
- `agents-plugin-tool/internal/mcp/workflow_manual.go#L216-L231` — the
  no-restorable-state FAIL-LOUD notice: `"## Session State\n(no restorable
  state for session key %q; ...)\n"` — contains neither `# Workflow Manual`
  nor `## Session Key` (note: `## Session State` ≠ `## Session Key`, no
  accidental anchor collision). This is the concrete `reason: "no-body"`
  fixture shape.
- **Live-confirmed in this session**: called `playbook.read(name:
  "lead-workflow-manual", session_key: "tackling-giant-uproot")` directly.
  Output's first non-empty line is literally `# Workflow Manual`; it carries
  the `> **Session invariant:**` blockquote and the full `<!--
  ws:fresh-only:start -->...<!-- ws:fresh-only:end -->` region verbatim,
  matching the ticket's Background claim exactly.
- **Real-fixture capture is NOT reachable from a restricted subagent MCP
  profile**: calling `workflow_manual` directly in this survey session
  returned `tool not available in current ws MCP profile: workflow_manual`
  (it is lead-only and hidden from this delegate's tool set, per
  `shouldMapWorkflowManual`'s role gate and ws-mcp's own profile filtering).
  The implementer must capture the CONTINUE-mode `workflow_manual` response
  either (a) from an actual Pi lead session, or (b) via a short throwaway
  Node script using `spawnWsMcpClient` (`agents-plugin-pi/src/mcp-stdio-client.ts:298`)
  to `initialize()`, call `ferrule` for a real key, then call `playbook.read`
  and `workflow_manual` with that key — the same technique
  `agents-plugin-pi/test/bridge.test.ts:35-36`'s comment says was already used
  once to capture `LIVE_TOOL_NAMES`. Neither this survey session nor an
  automated test run can perform (a); (b) is within the implementer's normal
  tool access and does not require ws-mcp changes.
- `agents-plugin-pi/test/bridge.test.ts#L261-L279` —
  `describe("cutStaticBody", ...)`: 3 tests on synthetic strings
  (`"STATIC BODY\n"`, `"Xabc"`), asserting exact-substring semantics. These
  are the tests the ticket's Phase 1 replaces wholesale with the real
  captured-pair test plus the reason-branch tests.
- `agents-plugin-pi/test/bridge.test.ts#L338-L359` — cut-found test: already
  includes `## Session Key\nlead-1` in its fixture text; needs no change,
  but is a useful shape template.
- `agents-plugin-pi/test/bridge.test.ts#L361-L389` — the fallback-dispatch
  test ("cut miss: falls back to workflow_state, dropping root, and notifies
  once"): fixture `"HEADER\nsomething drifted\n## Session Key\nlead-1"` has
  the end anchor but not the start anchor (`"STATIC-BODY"` never appears) —
  under the new algorithm this is unambiguously `reason: "start-anchor"`,
  still triggers fallback + one notify call. **No change needed**; confirms
  the ticket's "fallback-dispatch tests keep passing" claim.
- `agents-plugin-pi/test/bridge.test.ts#L391-L403` and `#L405-L422` — the two
  advisory-keying tests to reshape: both use fixture text
  `"HEADER\nSTATIC-BODY\nBODY"` with staticBodySnapshot `"STATIC-BODY\n"` and
  **no** `## Session Key` line. Today this is a cut-hit (exact substring
  present). Under the anchor rule this fixture has the start anchor but no
  end anchor → `reason: "end-anchor"` → **silently becomes a fallback-path
  test** even though nothing in the test changed. Fix: add a `## Session
  Key\n<key>` line to each fixture's response text so they keep exercising
  the cut-hit path (matching Phase 1's explicit instruction).
- `agents-plugin-pi/test/bridge.test.ts#L424-L438` — "throws when
  workflow_manual dispatch itself errors": never reaches `cutStaticBody`
  (throws before). No change needed.
- **Risk signal (not in ticket's Tests list, flagged for awareness only)**:
  `agents-plugin-pi/test/bridge.test.ts#L485-L501` — the parametrized
  `for (const cut of [true, false])` "mapped cut/fallback appends one
  rejection advisory" test uses `staticBodySnapshot: "STATIC"` against a
  single-line response (`"HEADER STATIC FOOTER"` for `cut: true`, `"drifted"`
  for `cut: false`). Neither fixture contains `"STATIC"` as a whole line nor
  `"## Session Key"` — under the new algorithm **both** iterations become
  `reason: "no-body"` (forward-unchanged), not one cut-hit + one fallback as
  the test name implies. The test's assertions are branch-agnostic
  (advisory shape and `original.content` identity only), so it keeps
  passing with zero code changes, but its `cut`/`fallback` parametrization
  no longer means what its name says. Not required by the ticket's Tests
  list — leave alone in this phase, but this is worth a one-line callout in
  the implementer's report for a future test-hygiene pass.
- `ai-docs/spec/pi-adapter-runtime.md#L255-L287` — anchor
  `{#260905-pi-workflow-manual-state-mapping}` confirmed. Primary bullet
  (`#L262-L268`) states "exact substring match against a static-body
  snapshot ... (the `playbook.print("lead-workflow-manual")` render...)" and
  Fallback bullet (`#L269-L274`) states "if the snapshot body is not found in
  the response (the renderer changed mid-session), the bridge dispatches
  ws-mcp `workflow_state` instead". Both need the wording changes the
  ticket's Spec Impact section describes — **owned by the lead's doc
  pre-pass, not this implementation phase**.

## Implementation Plan

1. `agents-plugin-pi/src/bridge.ts#L215-L230` — rewrite `cutStaticBody`:
   - Compute `startLine` = first non-empty line of `staticBodySnapshot`
     (split on `"\n"`, find first element with length > 0).
   - Split `response` on `"\n"` into `lines`; find the first index whose
     element equals `startLine` exactly (whole-line, not substring), and the
     first index whose element equals the literal string `"## Session Key"`
     exactly.
   - Branch: neither found → `{ text: response, found: false, reason:
     "no-body" }`; start missing only → `reason: "start-anchor"`; end
     missing only → `reason: "end-anchor"`; both found but end index ≤ start
     index → `reason: "order"`; otherwise compute character offsets for the
     start-of-start-line and start-of-end-line (reconstruct via joining the
     prefix lines, or track offsets while scanning) and return `{ text:
     response.slice(0, startOffset) + response.slice(endOffset), found:
     true }` (end anchor line itself is kept, in the retained tail).
   - Update the function's doc comment to describe the anchor rule and the
     4-way reason outcome instead of "renderer drift" substring semantics.
2. `agents-plugin-pi/src/bridge.ts#L271-L280` — widen
   `WorkflowManualMappingDeps.notifyMappingDegraded` to accept the miss
   reason (exclude `"no-body"`, since that branch never calls it): `(reason:
   "start-anchor" | "end-anchor" | "order") => void`. Export the reason
   union type (e.g. `StaticBodyCutMissReason`) for reuse by the deps type and
   by tests.
3. `agents-plugin-pi/src/bridge.ts#L301-L328` — change
   `dispatchMappedWorkflowManual`'s branch from binary to three-way on
   `cut.found` / `cut.reason`:
   - `found: true` — unchanged (existing cut-success path).
   - `reason === "no-body"` — forward the original `manualText` unchanged:
     `replaceFirstTextItem(manualResult.content,
     prependWorkflowStateLine(manualText))`, run through the same
     `maybeAppendModelCatalogAdvisory` call as today, **no**
     `notifyMappingDegraded` call, **no** `workflow_state` dispatch.
   - Otherwise (`"start-anchor" | "end-anchor" | "order"`) — existing
     fallback path unchanged in shape: `notifyMappingDegraded(cut.reason)`,
     then dispatch `workflow_state` with only `session_key`.
   - Update the function's doc comment (`#L282-L300`) to describe the
     three-way outcome instead of the current binary hit/miss framing.
4. `agents-plugin-pi/src/bridge.ts#L647-L656` — update the
   `notifyMappingDegraded` closure passed at the `startBridge` call site to
   accept the reason and word the warning without "renderer drift", naming
   which anchor was missing (or "order" for the anchors-reversed case).
   Keep the existing `notifiedMappingDegraded` once-per-session dedupe
   (`#L579`) unchanged.
5. `agents-plugin-pi/test/bridge.test.ts` — capture the real fixture pair
   per the Codebase Findings note above (spawnWsMcpClient + `ferrule` +
   `playbook.read` + `workflow_manual` against this repo, or a live Pi lead
   session), trim the `## Session State` / notes tail to a few
   representative lines, and inline both texts as template-string constants
   near the top of the file (matching this file's existing
   `LIVE_TOOL_NAMES`-style captured-fixture convention at `#L35-L59` — no
   new fixtures directory needed unless the trimmed text is too large to
   read comfortably inline, in which case follow `version-check.test.ts`'s
   `readFileSync` + `dirname(fileURLToPath(import.meta.url))` pattern
   instead).
6. `agents-plugin-pi/test/bridge.test.ts#L261-L279` — replace the 3 synthetic
   `describe("cutStaticBody", ...)` tests with:
   - One test using the real captured pair, asserting `found: true` and
     that the cut text equals the prepended-advisories-plus-tail shape
     (everything before the snapshot's `# Workflow Manual` line, concatenated
     with everything from `## Session Key` onward).
   - One test: captured response with `## Session Key` stripped out →
     `reason: "end-anchor"`.
   - One test: captured response with the start-anchor line removed/altered
     but `## Session Key` intact → `reason: "start-anchor"`.
   - One test: the no-restorable-state notice text (from
     `workflow_manual.go#L228-L230`'s shape) → `found: false, reason:
     "no-body"`.
   - Optional (not required by Phase 1's Tests list, but low-cost): one test
     for `reason: "order"` (end-anchor line placed before the start-anchor
     line).
7. `agents-plugin-pi/test/bridge.test.ts#L391-L403` and `#L405-L422` —
   reshape both advisory-keying tests' response fixture text to include a
   `## Session Key\n<key>` line after `"STATIC-BODY"`, so they keep taking
   the cut-hit path under the new algorithm (per Phase 1's explicit
   instruction).
8. Add a `dispatchMappedWorkflowManual`-level test for the `no-body` case:
   the no-restorable-state notice text in, verify output is the notice with
   `prependWorkflowStateLine` applied and nothing else, `notifyMappingDegraded`
   never called, and no `workflow_state` `callTool` invocation recorded.
9. Leave `agents-plugin-pi/test/bridge.test.ts#L361-L389` (fallback-dispatch)
   and `#L424-L438` (throws) and `#L485-L501` (flagged risk, out of scope)
   untouched.
10. Do not edit `ai-docs/spec/pi-adapter-runtime.md` as part of this
    implementation's commit — the Primary/Fallback bullet amendment is the
    lead's doc pre-pass.

## Verification Plan

- `cd agents-plugin-pi && node --test test/bridge.test.ts` (Node v22+ native
  TS type-stripping, per this file's existing header comment) — must show
  all `cutStaticBody` and `dispatchMappedWorkflowManual` tests passing,
  including the reshaped and newly added ones.
- `cd agents-plugin-pi && npm test` (or the project's standard full test
  command, if narrower than the above) to confirm no other suite regressed.
- Manual/no-tooling check: re-read the rewritten `cutStaticBody` against the
  4 reason branches by hand-tracing the real captured fixture pair plus the
  no-restorable-state notice text, confirming each maps to the intended
  reason.
- Owner-run live check (Phase 1's own verification bullet, out of scope for
  this implementation to execute): in a real Pi lead session, call an entry
  skill that invokes `workflow_manual` and confirm no warning fires, the
  response opens with the fixed mapping line, then advisories, then `##
  Session Key`, with no manual body duplicated.

## Escalations

- None.
