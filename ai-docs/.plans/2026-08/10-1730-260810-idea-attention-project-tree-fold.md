# Plan: 260810-feat-idea-ticket-attention-policy — Phase 2: Fold orphan idea/ in project_tree, keep parented idea and ready/todo

## Relevant Ticket Contract
- `renderTickets` keeps rendering `ready/` and `todo/` in full, unchanged.
- For `idea/`: render in full ONLY tickets carrying a `parent:` frontmatter key
  (epic children = planned decomposition). Fold the remaining orphan idea
  tickets into a single hidden-count line, e.g. `idea: N orphan hidden —
  tickets.list status=idea to view`.
- The fold is keyed strictly on `parent:` presence — `related:`-only ideas
  still fold (Decision: "Parented-idea exception").
- Count line over full omission: the existence signal (`N`) must survive even
  though bodies are omitted; full idea bodies stay reachable via
  `tickets.list(status:"idea")` / `tickets.find`.
- Verification boundary (ticket's own Phase 2 Verification): project_tree
  renders every parented idea (with `parent:`/`related:` suffixes) and all
  ready/todo exactly as today; orphan idea tickets collapse to one count line
  whose N equals the orphan idea count; folded stems are absent from the tree
  body but returned by `tickets.list(status=idea)`.
- Spec Impact names the exact amendment surface: `spec/mcp-tools.md`,
  `project_tree` ticket inventory section, anchor
  `{#260505-project-context-convention-tools}` — "the inventory renders
  parented idea plus all ready/todo in full and folds orphan idea tickets into
  a hidden-count line; full idea bodies remain reachable via the discovery
  tools."

## Out of Scope
- Phase 1 surfaces (already landed, do not re-touch): `lead-scope-worktree`
  playbook/sparse-checkout pattern, `.gitkeep` forcing, `ws/git.commit`
  `--sparse` staging (`wsgit.CommitOptions.SparseScopeActive`), and
  `agents-plugin-tool/internal/mcp/scope_announcement.go`'s counted-status
  list.
- `ticketScopeAnnotation` (`agents-plugin-tool/internal/mcp/server.go:3069`)
  and its call site (`server.go:1119`) — this is the sparse-checkout
  hidden-count annotation appended by the `project_tree` tool case *after*
  `wsdoc.ProjectTree(root)` returns. It is a separate mechanism from this
  phase's orphan-idea fold (scope-hidden vs. render-folded) and is untouched.
  Both annotations may appear in the same response; that is expected, not a
  conflict.
- `260807-refactor-dissolve-project-index` (`ai-docs/tickets/todo/260807-refactor-dissolve-project-index.md`,
  currently in `todo/`, blocked on prerequisites, not `ready/`): its Decisions
  name `project_tree` as the eventual generated-table destination for
  `_index.md`'s derivable content, but the ticket body contains no edit to
  `renderTickets`'s idea-handling logic — its scope is `_index.md`
  dissolution and a `spec/documentation-system.md` rewrite. This phase's
  orphan-idea fold is a narrow, additive change to the same function
  (`renderTickets`) and does not alter `_index.md`, the generated-table
  destination story, or pre-empt any future 260807 change to the same
  function — the two remain orthogonal edits to a shared file, not competing
  designs.
- `spec/workflow-skills.md` (Phase 1's amendment surface) and the
  `workflow_manual` scope banner text — neither is touched by this phase.
- Mental-model updates: `ai-docs/mental-model/documentation-system.md` only
  mentions ticket status directories generically (line 26) and carries no
  `project_tree`/`renderTickets` rendering detail to update; no mental-model
  edit is in scope for this phase.

## Codebase Findings
- `agents-plugin-tool/internal/wsdoc/project_tree.go#L218-L265` — `renderTickets`
  is the sole target. It loops `for _, status := range []string{"ready", "todo", "idea"}`,
  and for every `.md` entry in each status dir it (a) writes
  `"  [%s] %s\n"` immediately, THEN (b) reads frontmatter via
  `frontmatter(filepath.Join(statusDir, entry.Name()))` and appends
  `parent:`/`related:` suffix lines if present. **Parent detection is already
  read on every ticket today (line 234) — no new frontmatter read is needed**,
  only a reorder: frontmatter must be read *before* the "render vs. fold"
  decision, not after the header line is already written.
- `agents-plugin-tool/internal/wsdoc/project_tree.go#L235` — existing parent
  read: `if parent, _ := fm["parent"].(string); parent != "" { ... }`. Reuse
  this exact frontmatter-key access for the fold predicate
  (`status == "idea" && parent == ""` → fold).
- `agents-plugin-tool/internal/wsdoc/project_tree.go#L262-L264` — the
  `anyTicket` bool guards the `(none)` fallback line at the end of
  `renderTickets`. An orphan-idea ticket that gets folded must still count
  toward `anyTicket` (a folded idea is not "no tickets exist").
- `agents-plugin-tool/internal/wsdoc/project_tree.go#L267-L284` — `titleSuffix`
  and `ticketTitle` scan all status dirs by stem directly (`for _, status :=
  range []string{"ready", "todo", "idea", "wip", ".done", ".dropped"}`) to
  resolve a referenced ticket's title. These are independent of the fold: a
  `related:` line on a `ready`/`todo` ticket that points at a *folded* orphan
  idea ticket still resolves and prints that idea's title via `ticketTitle`,
  because that lookup reads the file directly rather than depending on
  `renderTickets`'s own idea-branch output. No change needed here.
- `agents-plugin-tool/internal/wsdoc/frontmatter.go#L8` — `frontmatter(path)
  map[string]any` is the existing helper already used at line 234; reuse
  as-is, no signature change.
- `agents-plugin-tool/internal/wsdoc/project_tree_test.go#L12-L40` —
  `TestProjectTreeRendersCoreSections` is the existing test to extend. Its
  fixture ticket `ai-docs/tickets/idea/260503-research-demo.md` has **no**
  `parent:` frontmatter, so under the new fold it becomes an orphan (count 1)
  rather than a rendered `[idea]` line. The test's current assertion list
  does not check for a `[idea] 260503-research-demo` line's presence, so it
  will not spuriously pass/fail on that account, but it also does not yet
  assert the new fold behavior (count line presence, folded stem absence,
  parented-idea full-render) — extend it (or add a sibling test) to cover:
  (1) a parented idea ticket renders in full with its `parent:`/`related:`
  suffixes, (2) one or more orphan idea tickets do NOT appear as `[idea]
  <stem>` lines, (3) the count line `idea: N orphan hidden — tickets.list
  status=idea to view` appears with the correct N, (4) `ready:`/`todo:`
  tickets are byte-for-byte unaffected.
- `agents-plugin-tool/internal/mcp/server.go#L1106-L1121` — the `project_tree`
  tool case calls `wsdoc.ProjectTree(root)` then appends
  `ticketScopeAnnotation(...)` (a distinct sparse-checkout hidden-count
  mechanism, Phase 1 surface). No change needed at this call site; the
  orphan-idea fold lives entirely inside `wsdoc.ProjectTree` → `renderTickets`.
- `ai-docs/spec/mcp-tools.md#L934-L942` — anchor
  `## Project Context And Convention Tools {#260505-project-context-convention-tools}`,
  specifically the paragraph "`project_tree` renders the project document map,
  spec inventory, and active ticket inventory for the current repository...."
  is the exact amendment surface named by the ticket's Spec Impact bullet.
  Add a sentence describing the parented-idea-full / orphan-idea-folded
  behavior and the count-line recovery path.
- Risk signal: none found. This is a narrow, additive, single-function change
  with an already-available frontmatter read; no reuse gap, no mock data, no
  fallback/temporary path.

## Implementation Plan
1. `agents-plugin-tool/internal/wsdoc/project_tree.go` — rewrite
   `renderTickets` (lines 218-265):
   - Add an `orphanIdea int` counter alongside `anyTicket`.
   - Inside the entry loop, move the `frontmatter(...)` read (currently line
     234) to occur immediately after the `.md` extension check, before any
     `fmt.Fprintf` header write.
   - Extract `parent, _ := fm["parent"].(string)` right after the frontmatter
     read.
   - When `status == "idea" && parent == ""`: increment `orphanIdea`, set
     `anyTicket = true`, and `continue` (skip writing the `[idea] <stem>`
     header and its parent/related suffix lines for this entry).
   - Otherwise (ready, todo, or a parented idea): keep existing behavior
     unchanged — write `"  [%s] %s\n"`, then the `parent:` suffix line (now
     using the already-extracted `parent` var instead of re-declaring it),
     then the existing `related:` block, unchanged.
   - After the outer `for _, status := range [...]` loop finishes (idea is
     last in status order, so this is equivalent to emitting right after the
     idea branch, but placing it after the full loop is simplest and
     order-independent), if `orphanIdea > 0`, write:
     `fmt.Fprintf(b, "  idea: %d orphan hidden — tickets.list status=idea to view\n", orphanIdea)`.
   - Keep the existing `if !anyTicket { b.WriteString("  (none)\n") }` as the
     final step, unchanged in position (still after the orphan-count line, so
     a repo with only folded orphan ideas does not also print `(none)`).
2. `agents-plugin-tool/internal/wsdoc/project_tree_test.go` — extend
   `TestProjectTreeRendersCoreSections` (or add a new
   `TestProjectTreeFoldsOrphanIdeaTickets`, preferred for isolation):
   - Fixture: one `ready/` ticket (unchanged, keep asserting full render),
     one `idea/` ticket WITH `parent:` (assert it renders in full with its
     `parent:`/`related:` suffix lines, same as a ready/todo ticket today),
     and two or more `idea/` tickets with NO `parent:` (orphans).
   - Assert: the parented idea's `[idea] <stem>` line and its suffixes are
     present; the orphan idea stems do NOT appear as `[idea] <stem>` lines
     anywhere in the output; the line `idea: 2 orphan hidden — tickets.list
     status=idea to view` (count matching fixture) is present; `ready:`/`todo:`
     output is unaffected (reuse the existing ready-ticket assertions as a
     baseline).
   - Optionally add a case with zero orphan idea tickets to confirm no count
     line is emitted (matches the "count line only when N > 0" doctrine —
     avoids clutter, mirrors the `!anyTicket` / `(none)` pattern already in
     the function).
3. `ai-docs/spec/mcp-tools.md` — amend the paragraph at
   `{#260505-project-context-convention-tools}` (around line 936-942): after
   the existing "active ticket inventory" sentence, add a sentence stating
   that `ready/` and `todo/` render in full, `idea/` tickets carrying
   `parent:` render in full (epic children), and remaining orphan `idea/`
   tickets fold into a single hidden-count line pointing at
   `tickets.list(status:"idea")` for full bodies. Match this plan's ticket
   Spec Impact bullet wording; do not invent new anchor stems (this is an
   amendment to the existing anchor, not a new one — ticket confirms "no
   single new stem covers this policy").
4. Do not touch `agents-plugin-tool/internal/mcp/scope_announcement.go`,
   `ticketScopeAnnotation`, `wsgit.CommitOptions`, or the
   `lead-scope-worktree` playbook — all Phase 1, already landed.

## Verification Plan
- `go test ./agents-plugin-tool/internal/wsdoc/... -run TestProjectTree` (or
  the project's standard Go test invocation for the `wsdoc` package) —
  confirms the new/extended fold test and the existing
  `TestProjectTreeRendersCoreSections`/`TestProjectTreeSkipsGitIgnoredEntries`
  still pass.
- Full package build/vet for `agents-plugin-tool` per repo convention (`go
  build ./...`, `go vet ./...`) since `renderTickets` is a shared internal
  function with no other call site changes expected.
- Manual/functional check matching the ticket's own Phase 2 Verification: on
  a fixture tree with mixed ready/todo/idea (some idea parented, some
  orphan), `project_tree` output shows parented idea and all ready/todo
  exactly as before, orphan idea stems absent from the body, and the count
  line's N equals the orphan idea count; `tickets.list(status:"idea")`
  (via `wsdoc`/MCP tool, not exercised by this Go unit test but worth a
  reviewer sanity check) still returns the folded stems.

## Escalations
- None.
