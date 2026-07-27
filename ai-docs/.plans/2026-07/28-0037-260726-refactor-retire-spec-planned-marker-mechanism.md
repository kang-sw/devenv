# Plan: 260726-refactor-retire-spec-planned-marker-mechanism — Phase 1: Legacy-marker compat note

## Relevant Ticket Contract

- Add an advisory note on four surfaces — `specs.list`, `specs.status`,
  `specs.find` (query path), `project_tree` — when a spec file still carries a
  legacy `🚧` planned marker. The note names the retired mechanism and gives the
  resolution.
- **Detect** by reusing `markerContext` / `specMarkerContexts`
  (`spec_discovery.go:263`, reached from `:206`), filtered to `🚧`. Do **not**
  build on `specStats` (`project_tree.go:169`) — it reads `features:` frontmatter
  that zero spec files declare.
- **Resolve ticket → spec, never spec → ticket.** Scan live tickets (`idea/`,
  `todo/`, `ready/`) for `spec:` / `spec-remove:` frontmatter and `## Spec Impact`
  body text referencing the marker-carrying spec.
- **Two resolution branches.** Live tickets found → name them with statuses,
  instruct "move the marker text into their `## Spec Impact`, then strip the
  marker". None found → orphaned; strip it, keeping the described behavior as an
  ordinary implemented entry if it shipped.
- **Advisory, never blocking.** Legacy markers are a migration state, not an
  error. The note routes; it never fails a commit.
- `markerContext`'s own looseness is **retained as-is** — its surviving consumer
  is `specs.find` match scoring (`spec_discovery.go:85`). The advisory does not
  inherit that looseness.
- Do **not** delete the existing marker's content. Phase 1 adds the note; Phase 2
  removes the mechanism and resolves the marker.
- **Document the advisory in `ai-docs/spec/mcp-tools.md` in this phase**, at both
  the `specs.*` and `project_tree` contracts. Sweep rather than trusting offsets.
- **Record two things in `### Result`**: the retained detection surface (2.1 may
  extend but never shrink it) and the compat note's removal condition (deleted
  one bootstrap ratchet cycle after 2.7's v0045/v0006 ship).
- Rejected, do not re-litigate: resolving the marker's anchor to a ticket stem;
  hosting the check in `tickets.verify`; blocking a commit on a legacy marker.
- Nothing planned goes into a spec doc as a `🚧` entry — that is the mechanism
  being retired.

### Lead deviations from the ticket text (binding; both re-verified below)

**D1 — the advisory predicate is a marker SHAPE at line start, not a bare `🚧`
contains-check.** The ticket's `## Decisions` ("the advisory keys on the emoji
alone") rests on a false premise and is unsatisfiable as written. Verified:
`ai-docs/spec/documentation-system.md` contains **6** `🚧` occurrences, so a bare
contains-check fires on it and directly fails this phase's own verification
clause 4. The predicate must key on the marker's syntactic shape at line start.

**D2 — ticket→spec matching is scoped to the MARKER's own anchor plus the exact
spec file path.** The ticket's Phase 1 wording ("an anchor stem belonging to that
file") is over-broad and contradicts its own expected outcome. Verified:
`ai-docs/spec/ws-web-dashboard/index.md` carries **72** anchors and **15** live
ticket `spec:` entries name anchors that resolve into that file. File-level anchor
matching would report ~15 unrelated dashboard tickets for a
workspace-root-prune-policy marker.

## Out of Scope

- All of Phase 2 (Go removal, embedded conventions, rsrc playbooks, judge, repo
  spec corpus sweep, resolving the live marker, bootstrap ratchet, regens/bump,
  ticket drop).
- Removing or altering `markerContext` / `specMarkerContexts` / `specStats` /
  `SpecInfo.TicketRefs`. Phase 1 only adds.
- Stripping or rewriting `ai-docs/spec/ws-web-dashboard/index.md:231` — that is
  2.6.
- `documentation-system.md` / `workflow-skills.md` prose edits — that is 2.5.
  Phase 1 touches only `mcp-tools.md`.
- Promoting `260723-feat-ready-spec-address-hard-gate` (explicitly not a
  prerequisite).

## Codebase Findings

### Verified line numbers (the ticket carries three drifted offsets)

- `agents-plugin-tool/internal/wsdoc/spec_discovery.go#L263-L270` —
  `markerContext`; match condition at **:266** = `Contains "🚧" || lower Contains
  "planned" || lower Contains "wip"`. Ticket-accurate.
- `agents-plugin-tool/internal/wsdoc/spec_discovery.go#L252-L261` —
  `specMarkerContexts`, splits full text (frontmatter included) line-by-line.
- `agents-plugin-tool/internal/wsdoc/spec_discovery.go#L206` — call site
  `info.MarkerContexts = specMarkerContexts(text)`; `:196` has `text`, `:199` has
  `info.Path` (repo-relative, slash-normalized).
- `agents-plugin-tool/internal/wsdoc/spec_discovery.go#L210-L229` —
  `specAnchorsInText`; `:224` sets `MarkerContext: markerContext(line)`. This is
  where a marker line's **own anchor** is already available (`match[1]`).
- `agents-plugin-tool/internal/wsdoc/spec_discovery.go#L24-L50` — `SpecInfo`
  (`MarkerContexts` at `:31`), `SpecAnchorInfo` (`MarkerContext` at `:43`),
  `SpecAnchorStatus` at `:46`.
- `agents-plugin-tool/internal/wsdoc/spec_discovery.go#L85` — `specs.find` match
  scoring joins `MarkerContexts` into the searchable field set. **This is
  `markerContext`'s protected consumer; do not touch it.**
- `agents-plugin-tool/internal/wsdoc/spec_tools.go#L13` — `specAnchorRE` =
  `\{#([0-9]{6}-[a-z0-9-]+)\}`. Reuse for per-marker anchor extraction.
- `agents-plugin-tool/internal/mcp/server.go#L2508-L2539` — `formatSpecs`; marker
  render at **:2536** (`writeIndentedLines(&b, "  marker: ", spec.MarkerContexts)`),
  `tickets=` flag at **:2529**. **Ticket says 2526 and 2518 — both DRIFTED.**
- `agents-plugin-tool/internal/mcp/server.go#L2541-L2547` — `formatSpecFind`;
  delegates wholly to `formatDocumentFind` and emits **no** marker line.
- `agents-plugin-tool/internal/mcp/server.go#L2624-L2654` — `formatSpecStatus`;
  `MarkerContext` render at **:2637-2638**. **Ticket says 2627-2628 — DRIFTED.**
- `agents-plugin-tool/internal/wsdoc/project_tree.go#L169-L183` — `specStats`,
  frontmatter-only, structurally dead on this corpus. `:145-156` is its render
  site (`WIP n -> refs`).
- `agents-plugin-tool/internal/wsdoc/project_tree.go#L15` — `ticketRefRE` =
  `\[(\d{6}-[\w-]+/p\d+)\]`; zero matches under `ai-docs/spec/`.

### Reusable infrastructure (found; use it, do not rebuild)

- `agents-plugin-tool/internal/wsdoc/tickets.go#L59-L65` — `TicketsList(root,
  TicketListOptions{Statuses: []string{"idea","todo","ready"}})`. Exactly the live
  set. Returns `[]TicketInfo`.
- `agents-plugin-tool/internal/wsdoc/tickets.go#L35-L52` — `TicketInfo` already
  carries `Stem`, `Path`, `Status`, `Specs`, `SpecRemoves`. `:264-265` populates
  `Specs`/`SpecRemoves` from `spec:` / `spec-remove:` frontmatter. **The
  frontmatter half of the resolver needs no new parsing at all.**
- Only the `## Spec Impact` **body** read is new. `TicketInfo` has no body field;
  `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L228` shows the existing
  section-detection idiom (`strings.HasPrefix(strings.TrimSpace(line), "## Spec
  Impact")`) but only as a presence check — extraction is new.
- `agents-plugin-tool/internal/mcp/server.go#L3015-L3022` — `writeIndentedLines`,
  the existing indented-render helper the advisory should reuse.

### Threading constraint (non-obvious; shapes the design)

- `agents-plugin-tool/internal/mcp/server.go#L1140-L1177` — the handlers call
  `wsdoc.SpecsList(root)` / `SpecsFind(root, …)` / `SpecsStatus(root, …)` **with**
  root, but then call `formatSpecs(result)` / `formatSpecFind(query, result)` /
  `formatSpecStatus(result)` **without** it. The formatters cannot scan tickets.
  → Compute the advisory in `wsdoc` (which has root) and carry it on the result
  structs; the formatters only render. This matches the existing
  `MarkerContexts` compute-in-wsdoc / render-in-server split.
- `agents-plugin-tool/internal/wsdoc/project_tree.go#L32` + `#L123-L128` —
  `renderSpecs(b, specRoot)` / `renderSpecDir(b, root, indent)` receive only the
  **spec dir**, not the repo root, and `renderSpecDir`'s `root` param shadows the
  repo root. Threading the repo root (or a prebuilt resolver) through both is
  required.
- The resolver must scan tickets **once** per tool call, not once per spec file.

### D1 evidence — re-verified against the real corpus

- `grep -rc "🚧" --include=*.md ai-docs/spec/` → only two files:
  `ws-web-dashboard/index.md` = 1, `documentation-system.md` = **6**.
  `mcp-tools.md` = 0, `workflow-skills.md` = 0. (The ticket's "22 lines across 4
  files" is the **loose** predicate; the emoji-only predicate hits 2 files.)
- `ai-docs/spec/documentation-system.md` — all six occurrences read and confirmed
  prose/inline-code, **none** in heading or callout form at line start:
  - `:98` ``Contract-first planned spec behavior uses `🚧` markers only when …``
  - `:100` ``… a heading such as `## 🚧 Feature Name {#YYMMDD-slug}`, and a …``
  - `:101` ``… uses a `> [!note] Planned 🚧` callout. Entries``
  - `:102` ``without `🚧` are treated as implemented and must be verified …``
  - `:236` ``… writes implemented entries or contract-first `🚧` entries, …``
  - `:240` ``… strips `🚧` markers when implementation has landed, …``
  **Nuance that makes line-start anchoring load-bearing:** `:100` and `:101`
  contain the literal marker *shapes* (`## 🚧 Feature Name {#…}` and
  `> [!note] Planned 🚧`) inside inline code, mid-line. A "contains the shape
  anywhere" predicate would still fire. The predicate must anchor at line start.
- `ai-docs/spec/ws-web-dashboard/index.md:231` (sole live marker, confirmed):
  `> [!note] Planned 🚧 {#260524-dashboard-workspace-root-prune-policy}`

### D2 evidence — re-verified against the real corpus

- The marker's own anchor is `260524-dashboard-workspace-root-prune-policy`.
  Grep across `idea/` + `todo/` + `ready/`: the **only** hits are inside
  `ai-docs/tickets/ready/260726-refactor-retire-spec-planned-marker-mechanism.md`
  (this ticket itself, at `:95`, `:421`, `:431`, `:504`, `:594` — all in
  `## Decisions` / `## Phases` / review tables; **none** in its `## Spec Impact`,
  which spans `:220-242` and names only `documentation-system.md`,
  `workflow-skills.md`, `mcp-tools.md`) and
  `ai-docs/tickets/idea/260726-research-spec-planned-marker-management-cost.md:94`.
- **Confirmed as predicted**: the research-ticket hit sits under
  `## Measured 2026-07-26` (heading at `:87`). That file has **no** `## Spec
  Impact` section at all — its headings are `Owner Statement`, `Why this is
  research, not a change ticket`, `Standing evidence`, `Measured 2026-07-26`,
  `Resolved 2026-07-26 — retire`, `Resolution direction (owner, 2026-07-26)`,
  `Topics`, `Non-Scope`. It must **not** match.
- Path `ai-docs/spec/ws-web-dashboard/index.md`: same two tickets only, same
  non-`## Spec Impact` sections.
- → **Orphaned case holds** under D2 at authoring time.
- **Why D2 matters, quantified**: `ai-docs/spec/ws-web-dashboard/index.md` has
  **72** anchors. Live tickets whose `spec:` entries resolve into that file include
  `260525-feat-ws-dashboard-document-polishing-backlog` (`260524-ws-dashboard-document-viewer-mode`,
  `…-translation-overlay`, `…-edit-save-fanout`),
  `260620-feat-ws-dashboard-agent-client-activity-sources`
  (`260521-ws-dashboard-activity-console-read-model`, `…-ui-shell`),
  `260517-bug-ws-dashboard-windows-terminal-control-keys` (three
  `260516-ws-web-dashboard-terminal-*` stems), and
  `260525-bug-ws-dashboard-agent-tab-close-confirmation-sticky`
  (`260516-ws-web-dashboard-browser-ui-acceptance-gate`) — **15** live `spec:`
  entries in total. Each sampled stem was confirmed to live in
  `ws-web-dashboard/index.md`. File-level anchor matching flips clause 1 from
  orphaned to "move it into these 15 unrelated tickets".

### Contradictions with the ticket, surfaced

1. **The `## Decisions` predicate justification is factually wrong** (see D1).
   The ticket claims all three prose files match "only because they describe the
   mechanism", implying a bare `🚧` check is safe. `documentation-system.md`
   carries six real `🚧` characters; a bare check fails clause 4.
2. **Phase 1's match rule contradicts the ticket's own `## Decisions`.** Phase 1
   says "the exact spec file path or an anchor stem belonging to that file";
   `## Decisions` says exact matching is what makes clause 1's orphaned case hold.
   Under the Phase 1 wording, clause 1 **fails** (15 matches). D2 resolves this in
   favor of `## Decisions`.
3. **Three drifted line numbers** in the ticket (`server.go` 2526 → 2536,
   2627-2628 → 2637-2638, 2518 → 2529). The ticket's own round-3 review notes it
   "has already carried two wrong line numbers"; these are more.
4. **`features:`-frontmatter marker form is unhandled by Phase 1 as written.**
   Phase 2's 2.7 explicitly requires v0045 cover `- 🚧 <name> [stem/pN]` entries in
   spec `features:` frontmatter, because a downstream project pinned at v0022 can
   still hold that form. Phase 1's advisory is the in-tree half of the same
   migration, but D1 names only heading and callout forms. Recommendation folded
   into step 1a (safe: `documentation-system.md` has no line-start `- 🚧`).

## Implementation Plan

1. **New file `agents-plugin-tool/internal/wsdoc/legacy_marker.go`** — one home
   for the shape predicate and the resolver, so nothing duplicates across
   `spec_discovery.go` and `project_tree.go`.

   a. `legacyMarkerLines(text string) []legacyMarker` — walk lines, keep only
      **line-start marker shapes** (trim leading whitespace first, then match at
      position 0). Three shapes:
      - heading: `^#{1,6}\s+🚧` (`## 🚧 Feature Name {#stem}`)
      - callout: `^>\s*\[![A-Za-z]+\]\s*Planned\s+🚧`
        (`> [!note] Planned 🚧 {#stem}`)
      - frontmatter/list: `^-\s+🚧\s` (`- 🚧 pending [stem/p1]`) — see
        contradiction 4; drop this shape only if the lead rejects it.
      Each `legacyMarker` carries the raw trimmed line and its own anchor,
      extracted with the existing `specAnchorRE` (`spec_tools.go:13`); empty when
      the line carries no anchor.
      **Do not call `markerContext` for the predicate** — reuse its
      trim-then-inspect structure only. `markerContext` stays byte-identical.

   b. `newLegacyMarkerResolver(root string) *legacyMarkerResolver`:
      - `TicketsList(root, TicketListOptions{Statuses: []string{"idea","todo","ready"}})`
        — one scan per tool call.
      - Per ticket, collect references from `info.Specs` + `info.SpecRemoves`
        (already parsed, `tickets.go:264-265`) **plus** tokens read from the
        ticket body's `## Spec Impact` section only (read from `info.Path`; stop at
        the next `^## ` heading). Extract both `ai-docs/spec/….md` paths and
        `{#YYMMDD-slug}` / bare `YYMMDD-slug` anchor stems from that section.
      - Failure to read a ticket body is non-fatal: skip it. The advisory is
        advisory.

   c. `func (r *legacyMarkerResolver) Advise(specPath string, markers []legacyMarker) string`
      — **D2 matching**: a ticket matches when its collected reference set
      contains the exact `specPath` (slash-normalized, repo-relative) **or** one
      of the `markers`' own anchor stems. Nothing else — no substring, no area
      prefix, no other anchor in the file. A marker with no anchor falls back to
      path matching only.
      Returns `""` when `markers` is empty. Otherwise the note, always advisory:
      - matched: `legacy planned marker (retired mechanism): N marker(s); live
        tickets referencing this spec: <stem> [<status>], … — move the marker text
        into the ticket's ## Spec Impact, then strip the marker. Advisory only;
        this never blocks a commit.`
      - unmatched: `legacy planned marker (retired mechanism): N marker(s); no
        live ticket references this spec — the marker is orphaned; strip it,
        keeping the described behavior as an ordinary implemented entry if it
        shipped. Advisory only; this never blocks a commit.`
      Sort matched tickets by stem for deterministic output.

2. **`spec_discovery.go`** — add `LegacyMarkerAdvisory string` (json
   `legacy_marker_advisory,omitempty`) to `SpecInfo` (near `MarkerContexts`,
   `:31`) and to `SpecAnchorStatus` (`:46`). Populate in `SpecsList` /
   `SpecsFind` / `SpecsStatus` (`:52`, `:56`, `:117`), each of which already has
   `root`: build the resolver once per call, then set the field per spec using
   `legacyMarkerLines(text)`. Leave `markerContext`, `specMarkerContexts`,
   `specTicketRefs`, and the `:85` match-scoring join untouched.

3. **`server.go` — four render points; `formatSpecFind` inherits nothing.**
   - `formatSpecs` (`:2508-2539`): after the existing `marker:` line at `:2536`,
     emit `writeIndentedLines(&b, "  legacy-marker: ", …)` when the advisory is
     non-empty. Covers `specs.list` (`:1144`) and `specs.find`'s no-query fallback
     (`:1163`).
   - `formatSpecFind` (`:2541-2547`): **must be changed explicitly.** It delegates
     wholly to `formatDocumentFind`, which knows nothing of `SpecInfo`. Append the
     advisory lines after the delegated body, iterating `specs` for non-empty
     `LegacyMarkerAdvisory`, prefixed with the spec path so the note is
     attributable. Covers `specs.find`'s query path (`:1161`).
   - `formatSpecStatus` (`:2624-2654`): emit the advisory after the `files:` block
     (`:2643-2652`), from `SpecAnchorStatus.LegacyMarkerAdvisory`. Do not disturb
     the per-location `MarkerContext` render at `:2637-2638` — 2.1 owns that.
   - `project_tree`: see step 4.

4. **`project_tree.go` — new body-level detection.** Thread the repo root through
   `ProjectTree` (`:17`) → `renderSpecs` (`:123`) → `renderSpecDir` (`:128`);
   rename `renderSpecDir`'s existing `root` param (it is the spec dir and would
   shadow). Build the resolver once in `ProjectTree`. In the per-file loop
   (`:130-166`), read the file body, run `legacyMarkerLines`, and when non-empty
   append a `legacy-marker` element to the existing `stats` slice (`:146`) or emit
   a following indented line. **Leave `specStats` (`:169`) and its `WIP n -> refs`
   render (`:150-156`) exactly as they are** — 2.1 removes them; Phase 1 only
   adds. Keep the existing `demo.md  - Demo  [2f, WIP 1 -> …]` output shape intact
   so `project_tree_test.go:30` still passes.

5. **`ai-docs/spec/mcp-tools.md`** — two edits, verified offsets:
   - `:807-810` (`project_tree` contract, under
     `{#260505-project-context-convention-tools}` at `:805`): add that the spec
     inventory flags spec files still carrying a legacy planned marker, advisory
     only.
   - `:828-831` (`specs.list` / `specs.find` / `specs.status`, under
     `{#260505-spec-discovery-tools}` at `:820`): add the advisory to the exposed
     surface list, stating the two resolution branches and that it never blocks.
   Sweep the file for other `specs.*` / `project_tree` output statements rather
   than editing only these two spots. Do **not** touch the `tickets=` /
   ticket-reference documentation (2.1 out-of-scope note). Do **not** write a `🚧`
   entry — record this as an ordinary implemented entry.

6. **Ticket `### Result`** — record exactly the two required things:
   - **Retained detection surface** (2.1 may extend, never shrink): the new
     `legacy_marker.go` predicate + resolver; `SpecInfo.LegacyMarkerAdvisory` and
     `SpecAnchorStatus.LegacyMarkerAdvisory`; the four render points; and
     `specAnchorRE` as the anchor extractor.
   - **Removal condition**: the compat note is deleted once no supported
     downstream version can still emit markers — one bootstrap ratchet cycle after
     2.7's v0045 / v0006 ship.
   Also record the two lead deviations (D1, D2) and the drifted line numbers.

## Verification Plan

- `cd /home/swkang/devenv/agents-plugin-tool && go build ./... && go test ./...`
  — existing suites must stay green, in particular `spec_discovery_test.go:33`
  (`joined(rootSpec.MarkerContexts) == "- 🚧 Root feature [260504-ticket-demo/p1]"`,
  which proves `markerContext` was not narrowed) and `project_tree_test.go:30`
  (`"  demo.md  - Demo  [2f, WIP 1 -> 260503-feat-demo/p1]"`, which proves
  `specStats` output was not disturbed).
- **New `legacy_marker_test.go` — clause 4 pin (the false-positive check).**
  Table-driven over the real shapes: assert **no** match for the six
  `documentation-system.md` line forms (prose with `🚧` in inline code, including
  the two that embed the literal marker shapes mid-line), and **match** for
  `## 🚧 Feature {#260101-x}`, `> [!note] Planned 🚧 {#260101-x}`, and
  `- 🚧 pending [260101-t/p1]`. This is the test that would fail under the
  ticket's literal bare-emoji predicate.
- **New test — the orphaned/move-the-text flip (clauses 1-3).** In a `t.TempDir()`
  corpus: a spec with a `> [!note] Planned 🚧 {#260101-anchor}` callout, plus
  sibling anchors in the same file referenced by an unrelated `todo/` ticket.
  1. No ticket references the marker's anchor or the spec path → advisory reports
     the **orphaned** case. **This also pins D2**: the unrelated ticket referencing
     a sibling anchor in the same file must not flip it.
  2. Add a `ready/` ticket with `## Spec Impact` naming the exact spec path →
     flips to the **move-the-text** case, naming stem and status `ready`.
  3. Remove that reference → back to orphaned, proving live ticket state is read
     per call, not cached.
  Assert on all four surfaces: `SpecsList`, `SpecsStatus`, `SpecsFind` with a
  non-empty query (through `formatSpecFind`, not the fallback), and `ProjectTree`.
- **Real-repo spot check** against `/home/swkang/devenv` itself: all four surfaces
  report the orphaned case for `ai-docs/spec/ws-web-dashboard/index.md`, and
  **none** reports anything for `documentation-system.md`, `mcp-tools.md`, or
  `workflow-skills.md`. Re-run the D2 grep first — the orphaned state is repo
  state, not an invariant.
- `grep -n "legacy" ai-docs/spec/mcp-tools.md` — clause 5: both the `specs.*` and
  `project_tree` contracts document the advisory.
- Manual: confirm no code path returns a non-nil error or non-zero exit because a
  marker was found (advisory, never blocking).

## Escalations

- None. Both open questions were settled by the lead (D1, D2) and both were
  re-verified against the corpus before planning; the remaining work is additive
  Go plus one spec-doc edit.
- One item for lead awareness, not a blocker: **contradiction 4** — Phase 1 as
  written does not cover the `features:`-frontmatter marker form that Phase 2's
  2.7 requires v0045 to cover. Step 1a includes it as a third shape (safe for
  clause 4: `documentation-system.md` has no line-start `- 🚧`). Drop that shape if
  the lead prefers to hold D1 to exactly two forms.
