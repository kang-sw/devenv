# Plan: 260824-feat-review-release-gate-policy — Phase 2: Mandatory release gate (devenv ship) — decoupled marker model

## Relevant Ticket Contract

- Four deliverables, no new MCP tool (`## Sign-off (2026-08-30)`, revised `## Decisions`, Phase 2 body):
  1. **Frontier resolver** — the review frontier is the last *clearing* entry
     (`pass`/`concern`); `block`/`routed` do not advance it; `bootstrap` is the
     resolvable floor. One shared internal resolver, consumed by both
     `CheckpointNudge` and the ship gate.
  2. **`lead-ship` gate branch (R5)** — un-omittable, user-overridable. If
     `release-boundary: present`, before `develop`→`main`: read the frontier
     **head SHA** from `review.marker` (bare/structured output, no
     string-scraping) and compute `git rev-list --count <frontier-head>..HEAD`.
     Empty → proceed. Non-empty → trigger `lead-review` over the range; if
     still not clear, surface a **strong recommendation** and stop for an
     **explicit** user decision. Override → proceed; marker stays unadvanced
     (single-writer invariant: only `lead-review`'s step 7 stamps); no audit
     record is required or defined by the mechanism.
  3. **Drop `release-tag-glob`** — remove the field from
     `ReadAgentsReviewPolicy` and devenv's `AGENTS.md`; remove the (never
     consumed) `git describe --match` machinery entirely. Keep
     `release-boundary` and `rendezvous-backend`.
  4. **devenv wiring (`ai-docs/ship/ws.md`)** — place the gate in ship
     pre-flight ordering; host-neutral branch must actually be exercised.
- **R4 pin-and-re-assert (retained)**: record the reviewed through-SHA;
  re-assert `git rev-parse develop` immediately before `git merge --ff-only
  develop`; a moved tip aborts and re-runs the gate over the delta. Name it
  generically (host-neutral) as well as concretely in `ws.md`.
- **Doc-map** (ticket body): `lead-review.md` step 7 gets the single-writer
  invariant + which verdicts advance the frontier (extends existing text, does
  not restructure it); `lead-ship.md` gets the gate branch + the negative
  invariant "ship never advances the marker"; `ai-docs/ship/ws.md` gets
  concrete pre-flight placement; **not** `lead-workflow-manual` (marker
  behavior is skill-specific, not a general primitive); spec `mcp-tools.md`
  gets the frontier contract + new head output + drops tag-glob refs, and must
  reconcile the Checkpoint Nudge anchor's "latest entry" wording; spec
  `workflow-skills.md` gets the lead-ship gate + lead-review sole-writer, and
  must clean the Phase-1 entry of `review.gate`/tag-glob traces; mental-model
  `review-watermark-ledger.md` gets frontier=clearing-entry + rationale.
- Editing `agents-plugin/rsrc/lead-ship/lead-ship.md` or `lead-review.md`
  requires the wsflow rsrc mirror regen (both are shipped wsflow skills).
- Verification boundary (ticket, Phase 2 `Verification:`): ship pre-flight
  stops for an explicit decision when `<marker>..HEAD` is non-empty and the
  triggered review doesn't clear it; proceeds silently when empty; override
  proceeds with marker unadvanced, no audit record required; through-SHA
  re-asserted at ff-merge time; no-boundary project's ship path unchanged;
  frontier resolver does not advance past a `block` entry.

## Out of Scope

- Phase 1's config surface (done, `ac3b7356`) — do not re-touch
  `ResolveTrack`, the `workflow_manual` nudge wiring, or the three-config-home
  split, except to read/trim `ReadAgentsReviewPolicy`.
- `review.gate` MCP tool, block-entry enumeration, routed-ticket-status
  cross-check as a gate-clearing mechanism — all DROPPED by the frontier
  redesign (superseded plan
  `ai-docs/.plans/2026-08/30-1751-260824-review-release-gate-policy-p2.md`).
  The retained ledger `block`-requires-`ref` invariant stays as fix-tracking
  bookkeeping only; whether to relax it is explicitly an open question **not**
  settled by this phase.
- Platform-backend (GitHub branch-protection) rendezvous hardening — devenv is
  `canary`.
- `AGENTS.template.md` bootstrap-template propagation — deferred (Phase 1
  Result), not this phase.
- Any change to ledger `Append`/`ParseLatest`/`entryLineRE` or entry format —
  the frontier resolver is a new read-side filter, not a format change.
- Durable audit-record home for overrides — the ticket explicitly says the
  mechanism defines none.

## Codebase Findings

- `agents-plugin-tool/internal/wsreview/ledger.go#L90-104` — `ParseLatest` is
  the only read function today; returns the last matching entry regardless of
  verdict. The frontier resolver is a new sibling function, not a modification
  of this one (`ParseLatest`/`Read` stay as-is; `Bootstrap`'s own idempotency
  check at `#L225-231` uses `Read`, i.e. "any entry exists," which is a
  different question from "what's the frontier" — leave it alone, flag only).
- `agents-plugin-tool/internal/wsreview/ledger.go#L33-39,77` — verdict tokens
  `pass|concern|block|routed|bootstrap`; `entryLineRE` is the single parse
  source of truth. Frontier classification: `{pass, concern, bootstrap}`
  clearing/floor, `{block, routed}` skipped (never overwrite "latest seen").
- `agents-plugin-tool/internal/wsreview/checkpoint.go#L49-53` — `CheckpointNudge`
  calls `Read(root)` (raw latest) as its first step; must switch to the new
  frontier resolver per deliverable 1. Rest of the function (track resolution,
  `commitsAheadOfMarker`) is unaffected — it already consumes `entry.Head`
  generically.
- `agents-plugin-tool/internal/mcp/server.go#L1253-1275` — `review.marker`
  handler: the bootstrap-branch "already exists" report (`L1266`, via
  `wsreview.Bootstrap`'s internal `Read`) and the bare-read branch
  (`L1268-1275`, via `wsreview.Read`) both currently report the raw latest
  entry. Both must report the frontier. Simplification: after the bootstrap
  call (created or not), re-resolve via the frontier function for the reported
  entry — a just-created bootstrap entry is itself a clearing/floor verdict, so
  this is a no-behavior-change simplification for that path, not a special
  case.
- `agents-plugin-tool/internal/mcp/server.go#L4164-4173` — `review.marker`'s
  `tools()` schema has one optional bool property (`bootstrap`); no output-format
  knob. `tickets.status` (`L4140-4149`) is the precedent for adding one:
  `"format": stringProperty('Optional output format. Use "json" for structured
  compatibility output.')`, dispatched via `wantsJSON(params.Arguments)`
  (`L1705-1708`) and `toolJSONResponse` (`L3349-3358`). Reuse this exact
  pattern for review.marker's new bare/structured head output instead of
  inventing a new mechanism — satisfies "must not string-scrape" without a new
  MCP tool.
- `agents-plugin-tool/internal/wsreview/agents_config.go#L21-23,42-43,59-61,117-119`
  — `DefaultReleaseTagGlob`, `releaseTagGlobLineRE`, the `ReleaseTagGlob`
  struct field + doc comment, and its parse block are the entire surface to
  remove. **Grep-confirmed: no `git describe --match`/`--tags` code exists
  anywhere in the codebase** — only this doc comment mentions it. "Remove the
  machinery" is therefore exactly this field removal; nothing else consumes
  `ReleaseTagGlob`.
- `agents-plugin-tool/internal/wsreview/agents_config_test.go#L16-100` — four
  `want` struct literals include `ReleaseTagGlob` (`L22,36,50,65`); two fixture
  strings embed `release-tag-glob:` lines (`L59,74`);
  `TestReadAgentsReviewPolicyParsesPlatformBackend` (`L72-82`) asserts
  `ReleaseTagGlob` directly and needs its glob-specific fixture/assertion
  dropped (keep the platform-backend assertion).
- `agents-plugin-tool/internal/wsreview/track_test.go#L76` — fixture AGENTS.md
  string also embeds `release-tag-glob: v*`.
- `agents-plugin-tool/internal/mcp/review_track_alarm.go#L27` — the
  once-per-session nudge text lists `release-tag-glob` among the fields to
  declare; drop it from the parenthetical.
- `AGENTS.md#L107-123` (devenv's own config, Phase 1 Result) — `release-tag-glob:
  v*` line (`L113`) and its prose sentence (`L122-123`) to remove.
- `agents-plugin/rsrc/lead-ship/lead-ship.md#L9-14` — Invariants include "The
  ship config is the single source of truth; do not improvise steps not listed
  there" (`L13`); must be amended with a carve-out for the mandatory gate.
  `### 2. Execute` step 1 is "Pre-flight — run any listed checks" (`L35`); the
  gate branch is new playbook content, not a Pre-flight bullet (R5: bullet-only
  is defeatable by omission, which is why it's a playbook branch).
- `agents-plugin/rsrc/lead-review/lead-review.md#L58` — step 7 is already the
  sole marker writer (`review.stamp` call), and already states "the
  invocation's own `<base>..<head>` ... record it verbatim, never the marker
  entry's Base." This is the extension point for the single-writer invariant +
  frontier-verdict classification; extend, don't restructure.
- `ai-docs/ship/ws.md#L20-49` (Pre-flight) and `#L63-77` (Publish) — Pre-flight
  has no review step today (insertion point for deliverable 4); Publish
  already runs `git checkout main && git merge --ff-only develop` (`L69`) —
  the R4 pin-and-re-assert goes immediately before this line.
- `ai-docs/manuals/wsflow-mirroring.md#L48,52,263-267` — both `lead-ship` and
  `lead-review` are shipped wsflow skills (thin `playbook.print` shims over the
  shared rsrc). After-edit checklist: `WSRSRC_REGEN=1 go test
  ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest` then
  `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run
  TestRegenerateWsflowRsrcMirror` (both `-count=1` mandatory — cached `ok`
  silently skips the write side effect).
- `ai-docs/spec/mcp-tools.md#L1880-1929` (`{#260830-review-watermark-ledger-tools}`)
  — `L1897` states "`review.marker` reads the ledger's latest entry"; must
  change to frontier semantics + document the new `format: json` output.
  `#L1930-1952` (`{#260830-review-watermark-checkpoint-nudge}`) — `L1941`
  "reads the ledger's latest entry (`wsreview.Read`...)" needs the same
  frontier reconciliation per the ticket's explicit doc-map note ("a tail
  `block` changes the nudge origin").
- `ai-docs/spec/workflow-skills.md#L1180-1222` (`{#260830-review-policy-config-surface}`)
  — `release-tag-glob` bullet at `L1208-1209` to remove; `release-boundary`
  bullet's forward-reference "a subsequent release-gate capability keys its
  mandatory review off" (`L1199-1201`) is now realized and should describe the
  actual mechanism. `L1327-1329` is the one-paragraph `lead-ship` spec entry —
  gains the gate description.
- `ai-docs/mental-model/review-watermark-ledger.md#L77-84` — current text: "the
  marker resolves purely from ledger content (the last parseable entry)."
  Becomes imprecise once frontier ≠ last-parseable-entry; needs a
  clarifying edit alongside the new frontier/single-writer paragraph.

## Implementation Plan

1. **Frontier resolver** (`agents-plugin-tool/internal/wsreview/ledger.go`):
   add `ParseFrontier(content string) (Entry, bool)`, mirroring `ParseLatest`
   (`#L90-104`) but skipping any line whose verdict is `block` or `routed`
   (never overwrite the running "latest clearing" entry), and `Frontier(root
   string) (Entry, bool, error)` mirroring `Read` (`#L109-119`) via
   `ParseFrontier`. Leave `ParseLatest`/`Read`/`Bootstrap` unchanged.
2. Switch `CheckpointNudge` (`checkpoint.go#L50`) from `Read(root)` to
   `Frontier(root)` — no other change needed in that function.
3. Switch the `review.marker` handler (`server.go#L1253-1275`) to report the
   frontier entry from both branches: after the bootstrap call, re-resolve via
   `wsreview.Frontier(root)` for the reported entry text (works for both
   `created` and no-op cases, since a just-created bootstrap entry is itself
   the frontier); the bare-read branch calls `wsreview.Frontier(root)` instead
   of `wsreview.Read(root)`.
4. Add a `format` schema property to `review.marker`'s `tools()` entry
   (`server.go#L4164-4173`), copying `tickets.status`'s pattern
   (`#L4140-4149`). In the handler, when `wantsJSON(params.Arguments)`, return
   `toolJSONResponse` with a small struct (`base`, `head`, `verdict`, `ref`,
   `found`) built from the frontier `Entry` — this is the "bare-SHA/structured
   head" output the gate branch consumes without string-scraping.
5. `agents-plugin/rsrc/lead-ship/lead-ship.md`: add a gate branch (new
   numbered section, invoked before the Execute step that promotes
   `develop`→`main` — i.e. ahead of or as part of `### 2. Execute` step 1
   Pre-flight) that: reads the project's `AGENTS.md` `### Review Policy`
   `release-boundary` field as plain prose (no new MCP tool — this mirrors how
   the rest of `lead-ship.md`/`ws.md` already reads project config as text);
   if `present`, calls `review.marker(format: json)` for the frontier head,
   runs `git rev-list --count <frontier-head>..HEAD`; empty → proceed;
   non-empty → trigger `lead-review` over `<frontier-head>..HEAD`; if still not
   clear, surface a strong recommendation and stop for an explicit decision;
   override → proceed (never calls `review.stamp` — ship never advances the
   marker). Amend the `L13` Invariant to carve out this un-omittable-but-
   overridable gate. Also add a generic (host-neutral) one-line mention of the
   pin-and-re-assert idiom (R4) near the promotion step, since `ws.md` supplies
   the concrete version.
6. `agents-plugin/rsrc/lead-review/lead-review.md#L58`: extend step 7's
   existing text with the single-writer invariant ("only this step, via
   `review.stamp`, ever advances the marker") and the frontier-verdict
   classification (`pass`/`concern` advance; `block`/`routed` do not).
7. Regen the wsflow mirror after steps 5-6:
   `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run
   TestGenerateRealManifest && WS_REGEN_WSFLOW_RSRC=1 go test
   ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`.
8. Drop `release-tag-glob`:
   - `agents_config.go`: remove `DefaultReleaseTagGlob` (`#L21-23`),
     `releaseTagGlobLineRE` (`#L42-43`), the `ReleaseTagGlob` field + doc
     comment (`#L59-61`) and its parse block (`#L117-119`); trim the doc-comment
     example at `#L71`.
   - `agents_config_test.go`: drop `ReleaseTagGlob` from all four `want`
     literals (`#L22,36,50,65`), strip `release-tag-glob:` from the two fixture
     strings (`#L59,74`), and remove the glob-specific fixture line +
     assertion in `TestReadAgentsReviewPolicyParsesPlatformBackend`
     (`#L72-82`), keeping its `RendezvousBackend` assertion.
   - `track_test.go#L76`: strip `release-tag-glob: v*` from the fixture.
   - `review_track_alarm.go#L27`: drop `release-tag-glob` from the nudge text.
   - `AGENTS.md#L107-123`: remove the `release-tag-glob: v*` line and its
     prose sentence.
9. `ai-docs/ship/ws.md`: add the gate to Pre-flight (host-neutral branch
   wired concretely — read `release-boundary` from `AGENTS.md`, resolve the
   frontier head via `review.marker`, `git rev-list --count`); add the R4
   pin-and-re-assert prose to the Publish section (`#L63-77`) immediately
   before `git checkout main && git merge --ff-only develop` (`#L69`): record
   the frontier-through-SHA at gate time, re-assert `git rev-parse develop`
   equals it right before the merge, abort-and-re-gate on mismatch.
10. Doc pass:
    - `ai-docs/spec/mcp-tools.md#L1897-1908,1941`: rewrite `review.marker`'s
      contract to frontier semantics (last clearing pass/concern/bootstrap
      entry; block/routed skipped) and document the new `format: json` output;
      reconcile the Checkpoint Nudge subsection's "latest entry" wording.
    - `ai-docs/spec/workflow-skills.md#L1199-1209,1327-1329`: remove the
      `release-tag-glob` bullet; reword the `release-boundary` bullet's
      forward-reference to describe the realized gate; extend the `lead-ship`
      paragraph with the gate description (un-omittable, overridable, keyed to
      `<marker>..HEAD`, `lead-review` sole marker advancer).
    - `ai-docs/mental-model/review-watermark-ledger.md`: add frontier =
      last-clearing-entry classification + single-writer/decoupling rationale
      to `## Module Contracts`; clarify `#L77-84`'s "last parseable entry"
      language now that frontier ≠ raw latest.

## Verification Plan

- `cd agents-plugin-tool && go build ./... && go vet ./...`
- `go test ./internal/wsreview/... ./internal/mcp/...` — extend with:
  - `wsreview`: frontier unit tests — trailing `block` holds the frontier at
    the prior `pass`/`concern`; trailing `routed` likewise; `bootstrap` alone
    resolves as the floor; `pass`/`concern` advance normally (mirrors existing
    `ledger_test.go` `ParseLatest` coverage shape).
  - `wsreview`: `CheckpointNudge`'s existing tests (`checkpoint_test.go`) still
    pass unmodified (they only use `VerdictPass`, unaffected by the frontier
    filter) — add one case with a trailing `block` to pin the nudge now
    originates from the last clearing entry, not the raw tail.
  - `mcp`: extend `review_watermark_checkpoint_test.go` — a
    pass-then-block sequence where `review.marker` still reports the `pass`
    entry (frontier, not latest); a `format: json` case asserting structured
    `base`/`head`/`verdict` fields.
- wsflow: `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run
  TestGenerateRealManifest && WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc
  -count=1 -run TestRegenerateWsflowRsrcMirror`, then `python3 -m unittest
  discover agents-plugin-wsflow/tests`.
- Spec index sanity after doc edits (`ws/spec_index_verify` or the bundled
  equivalent).
- Manual dry-run on devenv (ticket's own Verification block): ship pre-flight
  stops for an explicit decision when `<marker>..HEAD` is non-empty and the
  triggered review doesn't clear it; proceeds silently when the range is
  empty; an override proceeds and leaves the marker unadvanced; the reviewed
  through-SHA is re-asserted at ff-merge time; a no-boundary project's ship
  path is unchanged (exercise by temporarily setting `release-boundary:
  absent`); the frontier resolver does not advance past a `block` entry.

## Escalations

- None.
