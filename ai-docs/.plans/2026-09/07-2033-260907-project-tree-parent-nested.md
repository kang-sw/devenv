# Plan: 260907-feat-ws-project-tree-parent-nested-ticket-render — Phase 1: Parent-nested status/stem ticket tree with dead-anchor pruning

## Relevant Ticket Contract

- Drop status-directory grouping; render the ticket section as a single
  parent-nested tree, each node labeled `status/stem` (e.g. `ready/feat-blah`),
  nested by `parent:` like a filesystem — no inline comments on node lines.
- Each stem renders once. `related:` edges and their inline `# comment` tails
  are removed from `project_tree` entirely (moved to on-demand `tickets_query`).
- Render the entire backlog: every `idea`/`todo`/`ready` ticket emits its own
  node (nested under its parent when it has one, otherwise a root). The
  orphan-`idea/` hidden-count fold is removed — no ticket is hidden.
- Dead-parent anchoring: a `.done`/`.dropped` node renders only if it has a
  live (`idea`/`todo`/`ready`) descendant, transitively. Two-pass shape: mark
  live-descendant reachability, then render only marked dead nodes (live
  nodes always render). A dead subtree with no live descendant is omitted
  entirely (dead children never count toward keeping a dead parent visible).
- Roots and children are ordered by stem, for deterministic output.
- A `parent:` pointing at a non-existent stem renders as a single shared
  placeholder root `?/<stem>`; several tickets pointing at the same missing
  stem nest under one shared placeholder, not one each.
- Cycle guard: a malformed `parent:` cycle must not hang the call; the nodes
  in the cycle degrade to flat roots (break-and-root) instead of erroring or
  looping.
- Tests required (from ticket Phase 1 body): live child under a `.done`
  parent shows the `.done` parent as anchor; a `.done` subtree with only
  `.done` children is omitted; multi-level dead-ancestor chains keep only the
  marked chain; all orphan `idea/` tickets render (none hidden); placeholder
  and cycle cases; output contains no `related` edges.
- Verification boundary: measure rendered size against the ~1.9k-token
  projection on the current backlog; within ~20% counts as pass; a result
  near the old ~8.6k means the `related`-edge removal or fold-drop did not
  take effect.
- Spec Impact (in scope for this phase — not a future phase): revise
  `ai-docs/spec/mcp-tools.md` `{#260505-project-context-convention-tools}` on
  the three named points (parent-nested `status/stem` tree instead of
  status-directory/edge prose; no `related:` edges — reached via
  `tickets_query` on demand; orphan-`idea/` fold prose deleted, full backlog
  renders). No new anchor, no `spec-remove:`.
- Constraint: preserve the file-tree portion of `project_tree` unchanged.

## Out of Scope

- `tickets_query`'s own full-queue dump cost (~11.8k tok/call) — named as the
  sibling symptom under `## Related`, not addressed by this ticket/phase.
- Any redesign of `tickets_query` itself.
- Mental-model doc changes: `ai-docs/mental-model/documentation-system.md`'s
  "Add a ticket status" recipe already names project-tree rendering
  generically and needs no edit for this phase (confirmed by reading it; no
  other mental-model file describes the ticket-section tree shape).
- `wip` as a status: not a currently active directory (`ai-docs/tickets/`
  today has only `idea/`, `todo/`, `ready/`, plus dot-prefixed `.done/`,
  `.dropped/`), not named by the ticket, and outside `scanTickets`'s default
  status set — left untouched.

## Codebase Findings

- `agents-plugin-tool/internal/wsdoc/project_tree.go#L218-L274` — current
  `renderTickets`: walks `ready`/`todo`/`idea` status dirs directly via
  `os.ReadDir`/`frontmatter(path)`, prints `[status] stem`, a `parent:` line,
  `related:` lines with inline `# note · title` comments, and folds
  parent-less `idea/` tickets into an `orphanIdea` counter/line. This whole
  body is replaced.
- `agents-plugin-tool/internal/wsdoc/project_tree.go#L276-L293` — `titleSuffix`
  and `ticketTitle` helpers exist only to append `# Title` comments to
  `parent:`/`related:` lines; both become dead code once those lines are
  removed (grepped — no other caller in the module) and should be deleted.
- `agents-plugin-tool/internal/wsdoc/tickets.go#L44-L65,192-196,349-424` —
  reusable component: `TicketInfo{Stem, Status, Parent, Title, ...}` and
  `scanTickets(root, ticketScanOptions{IncludeDone, IncludeDropped})` already
  load the whole board (default `Statuses` gives `ready`/`todo`/`idea`, plus
  `.done`/`.dropped` when included) with `Parent` pre-parsed via
  `frontmatterFromText`. This directly replaces the ad hoc per-file
  `frontmatter(path)` walk and gives `Status` already normalized
  (`.done`/`.dropped` with the dot). No `Resolve`/scope handling is needed
  (leave `Resolve` at its zero value `resolveOff`) — that matches current
  `renderTickets`'s filesystem-only behavior; sparse-checkout-hidden tickets
  are handled separately by the scope-count annotation appended in
  `internal/mcp/server.go` (see below), not by `project_tree` itself.
- `agents-plugin-tool/internal/wsdoc/tickets_graph.go#L79-L133,202-231` —
  `loadTicketGraph` is the established pattern for this exact kind of
  whole-board parent/child load (`scanTickets(root, ticketScanOptions{
  IncludeDone: true, IncludeDropped: true})`, then a `children map[string][]string`
  built from `Parent`), already referenced by
  `ai-docs/mental-model/documentation-system.md:41`. Reuse the same
  `scanTickets` call shape for consistency; do not reuse `walkAncestors`
  itself, since it is tuned for verify-advisory ancestor-chain reporting
  (returns nil chain + cycle path) rather than the mark-and-render tree this
  phase needs — but it confirms the established idiom for detecting a
  `parent:` cycle by walking with a `seen` set is fine to mirror.
- `agents-plugin-tool/internal/wsdoc/project_tree.go#L14-L35` — `ProjectTree`
  calls `renderTickets(&b, filepath.Join(aiDocs, "tickets"))` only when
  `isDir(...)` is true. `renderTickets`'s signature must change to take the
  repo `root` (already in scope in `ProjectTree`) instead of `ticketsRoot`,
  since `scanTickets` joins `ai-docs/tickets` itself; `renderTickets` should
  also start returning `error` so a `scanTickets` failure propagates through
  `ProjectTree`'s existing `(string, error)` signature instead of being
  swallowed.
- `agents-plugin-tool/internal/mcp/server.go#L1022-L1037` — the
  `project_tree` tool case appends `ticketScopeAnnotation(root, []string{
  "ready", "todo", "idea"})` after `wsdoc.ProjectTree(root)`'s text. That
  function is independent (no dependency on `titleSuffix`/`ticketTitle`) and
  needs no behavior change, but its comment at L1029-1034 explicitly narrates
  the now-removed "title suffix" / `related:` nicety ("a hidden parent:/
  related: target still renders without its title suffix") — this comment
  goes stale the moment this phase lands and should be corrected in the same
  change (docs-on-contact) to describe the parent-nested-tree behavior
  instead.
- `agents-plugin-tool/internal/wsdoc/project_tree_test.go#L12-L113` — four
  existing tests assert the old `[status] stem` / `parent:` / `related:` /
  orphan-fold shape and must be rewritten:
  `TestProjectTreeRendersCoreSections` (ticket-section assertions only; keep
  the file-tree/spec assertions), `TestProjectTreeFoldsOrphanIdeaTickets`,
  `TestProjectTreeOrphanOnlyIdeaSuppressesNone`,
  `TestProjectTreeNoOrphanIdeaCountLineWhenZero`.
- `ai-docs/spec/mcp-tools.md#L1178-L1191` — the exact prose the Spec Impact
  section names; current text describes status-directory rendering, the
  epic-child `idea/` inclusion rule, and the orphan-fold with a
  `tickets.query(statuses: ["idea"])` pointer. Replace with prose describing:
  single parent-nested tree of `status/stem` nodes; no `related:` edges
  (reachable via `tickets_query` on demand instead); full backlog renders
  (fold prose deleted); `.done`/`.dropped` nodes appear only as dead-parent
  anchors for a live descendant.
- `agents-plugin-tool/internal/mcp/server_test.go#L2342-L2357` and other
  `ticketGraphAdvisoryFixture`-based tests near L2345-2482 exercise
  `TicketVerify`'s commit-time board advisories (a different, untouched code
  path — `tickets_graph.go`), not `project_tree`; confirmed no overlap, no
  edit needed there.
- Risk signal: none found that blocks light planning. The one genuine design
  gap is that the ticket text scopes dead-parent anchoring explicitly to
  `.done`/`.dropped` ("anchoring ... is needed only for done/dropped
  ancestors") and never states whether a placeholder (`?/<stem>`) root must
  also satisfy the live-descendant-required rule when every ticket naming
  that missing parent turns out to be a dead-with-no-live-descendant subtree.
  This plan applies the same live-descendant rule to placeholders too (never
  render a placeholder with a fully-pruned subtree), for consistency with the
  anchor rule's intent and because it is the conservative choice (never
  invents extra visible noise); flagged below as a minor default rather than
  a scope reduction, since it does not implement less than the ticket asks —
  it fills an edge case the ticket's prose does not address either way.

## Implementation Plan

1. `agents-plugin-tool/internal/wsdoc/project_tree.go` — change the
   `ProjectTree` call site (`~L32-34`) to call `renderTickets(&b, root)`
   (repo root, not `ticketsRoot`) and propagate its error:
   ```go
   if isDir(filepath.Join(aiDocs, "tickets")) {
       if err := renderTickets(&b, root); err != nil {
           return "", err
       }
   }
   ```
2. Replace `renderTickets` (current `L218-274`) with a `(b *strings.Builder,
   root string) error` implementation:
   - Load the whole board: `tickets, err := scanTickets(root,
     ticketScanOptions{IncludeDone: true, IncludeDropped: true})` (default
     `Statuses` gives `ready`/`todo`/`idea`; mirrors
     `loadTicketGraph`, `tickets_graph.go#L85`). Return the error unchanged.
   - Index by stem: `byStem := map[string]TicketInfo{}` (one entry per stem;
     collisions cannot occur here the way `loadTicketGraph` guards for, since
     `scanTickets` here draws each stem from exactly one status directory on a
     normal board — no need to replicate the graph's first-wins guard, but do
     skip a duplicate stem defensively by keeping the first occurrence, to
     never crash on an abnormal board).
   - Cycle-aware effective-parent resolution, run once over `byStem` (a
     functional graph: each ticket has at most one `Parent` edge). Use a
     three-color (white/gray/black) walk per unvisited ticket, following
     `Parent` only while it resolves to another key in `byStem`:
     - white -> gray, push stem onto the current walk's path, follow `Parent`.
     - if `Parent` is empty or does not resolve in `byStem`: every stem on the
       current path is a normal (non-forced-root) node; color them black and
       stop. (An unresolved `Parent` is handled by placeholder attachment in
       the next step, not by this cycle pass.)
     - if `Parent` resolves to a stem already black: same as above (attach
       normally) — that ancestor chain was already resolved as non-cyclic (or
       already forced-root) by an earlier walk.
     - if `Parent` resolves to a stem currently gray (on the current path):
       cycle found. Every stem from that gray stem's position to the end of
       the path is a forced-root (color black, mark `forcedRoot[stem] = true`);
       every stem earlier on the path (leading into the cycle) is normal
       (color black, not forced-root).
   - Build `children map[string][]string` and `roots []string`:
     - For each ticket: if `forcedRoot[stem]` or `Parent == ""`, it is a root.
       Else if `Parent` resolves in `byStem`, it is a child of that stem. Else
       (`Parent` set but unresolved) it is a child of a synthetic placeholder
       keyed e.g. `"?" + Parent` — create the placeholder node once per
       distinct missing `Parent` value the first time it is referenced, and
       add the placeholder key itself to `roots` at creation time (dedup — a
       second ticket naming the same missing stem reuses the existing
       placeholder, does not add a second root).
     - Track a small `placeholders map[string]string` (placeholder key ->
       missing stem) so the renderer knows to print `?/<stem>` instead of
       `status/stem` for that node.
   - `isLiveStatus(status string) bool`: `status == "idea" || status == "todo"
     || status == "ready"`.
   - `displayStatus(status string) string`: strip the leading `.` from
     `.done`/`.dropped` (-> `done`/`dropped`); placeholders render `?`
     directly (handled separately, see below); other statuses pass through.
   - Memoized post-order `shouldRender(key string) bool` over `children`,
     computed via one DFS from `roots` (or a simple recursive memo map,
     guarded against re-entering a node already computed — the tree is
     acyclic by construction after the cycle pass, so no re-guard is needed
     beyond memoization for efficiency):
     - For a real ticket: `isLiveStatus(status) || any(shouldRender(child) for
       child in children[stem])`.
     - For a placeholder: `any(shouldRender(child) for child in
       children[placeholderKey])` (placeholders are never themselves "live");
       this is the plan's edge-case default noted above — a placeholder with
       an entirely pruned subtree is omitted, matching the anchor rule's
       spirit.
   - Render: `sort.Strings(roots)`, then DFS each root at indent 0, printing
     `strings.Repeat("  ", depth) + displayLabel + "\n"` (label = `"?/" +
     placeholders[stem]` for a placeholder, else `displayStatus(status) + "/"
     + stem`) only when `shouldRender(stem)` is true; when false, prune the
     whole branch without recursing (safe: `shouldRender(stem) == false`
     implies every descendant is also non-rendering, since `shouldRender` is
     exactly "isLive OR any child renders"). Sort each node's own children by
     stem before recursing, for deterministic sibling order.
   - Preserve the section header `"tickets:\n"` and the empty-state line: emit
     `"  (none)\n"` when zero nodes were ever printed (replaces the old
     `anyTicket` bool with a `printed` bool set on each successful render).
3. Delete `titleSuffix` and `ticketTitle` (current `L276-293`) — dead code
   once `renderTickets` stops emitting `parent:`/`related:` comment lines.
4. `agents-plugin-tool/internal/mcp/server.go#L1029-1034` — rewrite the stale
   comment above `text += ticketScopeAnnotation(...)`: it currently narrates a
   "title suffix" nicety on `parent:`/`related:` lines that no longer exist;
   replace with a short note that the scope-hidden-ticket count is still
   appended out-of-band because `project_tree` has no JSON mode, independent
   of the parent-nested tree body. No functional change to this block.
5. `agents-plugin-tool/internal/wsdoc/project_tree_test.go` — rewrite the four
   ticket-section tests and add the ticket's required coverage:
   - Update `TestProjectTreeRendersCoreSections`: replace the `[ready]
     260503-feat-demo` / `parent:` / `related:` assertions with `status/stem`
     nested-line assertions (e.g. `"  todo/260503-epic-demo"` then
     `"    ready/260503-feat-demo"` at one deeper indent) and assert
     `"related:"` is absent from the output.
   - Replace `TestProjectTreeFoldsOrphanIdeaTickets` with a test asserting all
     orphan `idea/` tickets now render as their own root nodes (none hidden,
     no `"orphan hidden"` line).
   - Replace `TestProjectTreeOrphanOnlyIdeaSuppressesNone` /
     `TestProjectTreeNoOrphanIdeaCountLineWhenZero` with equivalents that just
     assert `"(none)"` appears only when the ticket set is truly empty, and
     never otherwise (no orphan-count concept left to test).
   - Add: a live `ready`/`todo`/`idea` ticket with `parent:` set to a
     `.done`-status stem — assert the `.done` parent renders as
     `done/<stem>` with the live child nested one level deeper.
   - Add: a `.done` ticket whose only child is another `.done` ticket (no live
     descendant anywhere) — assert neither renders.
   - Add: a 2+ level dead-ancestor chain (`.dropped` grandparent -> `.done`
     parent -> live child) — assert both dead ancestors render, in the right
     nesting, and a sibling dead branch off the same grandparent with no live
     descendant of its own does not render.
   - Add: two tickets whose `parent:` names the same non-existent stem —
     assert a single `?/<stem>` root with both as its children (not two
     placeholders).
   - Add: a 2-ticket `parent:` cycle (A parent=B, B parent=A) — assert both
     render as flat roots (not nested under each other) rather than the call
     erroring or hanging.
   - Add: an assertion (e.g. on the accumulated corpus from the other cases)
     that the full output never contains the substring `"related:"`.

## Verification Plan

- `cd /Users/kang-sw/devenv/agents-plugin-tool && go test ./internal/wsdoc/...`
- `cd /Users/kang-sw/devenv/agents-plugin-tool && go build ./...`
- Manual size check against the ticket's ~1.9k-token / ~20%-tolerance
  verification boundary: run `wsdoc.ProjectTree("/Users/kang-sw/devenv")`
  against the live repo (e.g. a short throwaway `go run` snippet, not
  committed) and measure `len(output)`; convert to a token estimate using the
  ticket's own observed ratio (~4 chars/token, from "34,415 chars / ~8,603
  tokens" in the ticket background) and confirm the result lands within ~20%
  of ~1.9k tokens (~7.6k chars), not near the old ~8.6k-token / ~34k-char
  figure — a result near the old figure means the `related`-edge removal or
  fold-drop did not take effect.
- Grep the rendered output for `"related:"` to confirm zero occurrences.

## Escalations

- None.
