---
title: Left-nav work-root rows — two-line layout with open-surface counts, and open-vs-closed de-emphasis
related:
  260710-epic-ws-dashboard-terminal-ux-polishing: parent board for dashboard-centric UX polish
  260725-research-ws-dashboard-pty-agent-pivot: supplies the unresolved SurfaceKind question that blocks the agent counter in the sub-row
  260523-research-ws-dashboard-persistable-ui-state-map: adjacent; the counts read from the same browser-local workbench state this research maps
parent: 260710-epic-ws-dashboard-terminal-ux-polishing
sage-review-design: completed
sage-review-completeness: completed
---

# Left-nav work-root rows — two-line layout with open-surface counts, and open-vs-closed de-emphasis

## Background

Owner UX request, 2026-07-25. Two changes to the left-nav work-root row, both
in the same component and the same CSS block:

1. **Two-line title cell.** The row becomes a 2-column × (2-row left, 1-row
   right) matrix: the left cell stacks a larger title line over a smaller
   secondary line, while the right-hand action cell stays a single
   vertically-centred row. Requested proportion roughly 65:35 between the two
   left-hand lines. The secondary line shows how many surfaces that work root
   currently has open — terminals, agents, documents.
2. **Open-vs-closed de-emphasis.** Whether a work root is open (the same
   condition that shows the row's `X` close affordance) should be legible from
   the row's background. Requested treatment: reduce *saturation* on
   not-open rows, and drop brightness only very slightly, so closed rows read
   as receded rather than blacked out.

## Prior Art

Verified against source at `507f99d1`.

**Read the later CSS block, not the first one.** `styles.css` defines
`.resource-row`, `.resource-row-select`, `.resource-row-main`, and
`.resource-row-meta` **twice** — around 1030-1110 and again from 2727. The
later block wins, and its values differ from the earlier one in ways that
change this ticket's reasoning. Everything below cites the *effective*
values.

- `.resource-row` is a `display: grid` with
  `grid-template-columns: minmax(0, 1fr) auto` — the requested 2-column matrix
  already exists, and `.resource-row-actions` (`display: flex;
  align-items: center`) keeps the right-hand cell a centred single row for
  free once the left cell grows a second line. This is the one Prior Art claim
  that is identical in both blocks.
- Effective `.resource-row` background is
  `var(--ws-color-surface-context)`, **not** `transparent`
  (`styles.css:2729`). This helps change 2: the preferred background
  `color-mix` has something real to mix against.
- Effective `min-height` is **34px**, not the 46px in the earlier block
  (`styles.css:2730`, and again for
  `[data-resource-presentation="workRoot"]` at 2735). Consequence: there is
  little vertical slack, so a reserved second line will **grow every nav
  row** rather than fit inside the existing box. Budget for that rather than
  assuming the row absorbs it.
- `.resource-row-select` is a grid holding exactly one child
  (`.resource-row-main`), and the later block adds
  `grid-template-columns: minmax(0, 1fr)` + `align-content: center`
  (`styles.css:2760-2763`). The second line is an added grid child, not a
  layout rewrite.
- `.resource-row-main` is **not** the flex rule from the earlier block. It is
  `display: grid; grid-template-columns: 20px minmax(0, 1fr) auto`
  (`styles.css:2765`) — a 20px glyph column, then the title. So adopting
  `.resource-row-meta` as-is for the new line leaves it starting at x=0 while
  the title starts after the glyph column; the new line needs its own
  alignment to sit under the title.
- `.resource-row-meta` exists, is unreferenced by `App.tsx`, and also has a
  second block (`styles.css:2788`, `justify-content: flex-start`) beyond the
  1096-1110 range. It is still the natural home for the secondary line, but
  inherit deliberately rather than by assumption.
- `ResourceRow` (`App.tsx:7323`) already receives `isOpenWorkRoot`, used to
  decide whether the `X` close affordance renders (`canCloseWorkRoot`,
  `App.tsx:7402`). It is not currently emitted as a `data-*` attribute, while
  `data-resource-presentation` / `-kind` / `-activation` / `-availability`
  already are (`App.tsx:7433-7437`).
- A per-root secondary line has precedent: `App.tsx:7263` renders a
  `.nav-secondary-context` div reading `"N pinned main surface(s)"` beneath a
  work-root row.
- Closing a work root deletes its per-root workbench entries
  (`App.tsx:1366-1380`, `1439-1455`), so "a closed root has no workbench
  state" holds.

### Where the counts actually live — the non-trivial part

The counts are browser-local, not daemon data, but **not** where a first read
suggests. Do not derive them from `paneOrderByRoot`: its in-source CONTRACT
(`App.tsx:4040-4048`) states it "only tracks agent/activity pane order for
this root; a dockview group can also host readonly-file panes and terminal
panes, whose order lives separately in (flat, cross-root)
`readOnlyFilePaneOrderByGroup` / `terminalPaneOrderByGroup`".
`workbenchGroupsByRoot` holds only `{id, label}` group refs and no panes at
all. Deriving from either ships zeros for both kinds.

Real sources:

- **Documents — straightforward.** `readOnlyFilePanes` (`App.tsx:488`) is
  App-level state and each pane carries `workRootId`/`serverRoute`, so the
  nav can group by root directly.
- **Terminals — needs plumbing.** `terminalPanes` (`App.tsx:3618`) is
  `WorkbenchShell`-local, while `ResourceNavigation` renders from `App()`
  (`App.tsx:1871`) as a *sibling* of `WorkbenchShell` (`App.tsx:1960`). There
  is no path from one to the other today. Either lift `terminalPanes` or add
  an upward count callback — precedent for the callback shape already exists
  in `onWorkbenchGroupsByRootChange` (`App.tsx:1969`).

Pane kinds come from `SurfaceKind`
(`frontend/src/workbench/surfaceRegistry.ts`): `persistentTerminal`,
`editor`/`viewer` for documents, and — relevant below — both `agent` and
`agentChat`.

## Constraints

- **Panes only exist while a work root is open.** A closed work root has no
  workbench state, so its secondary line has nothing to show. This composes
  well with change 2 (closed ⇒ receded and count-less), but the row's
  height must not visibly jump between open and closed rows — reserve the
  secondary line's height rather than conditionally removing the element.
- **There are three competing visual axes, not two.** Selection is
  `.resource-row-selected` (`styles.css:1081`: accent gradient + inset
  shadows, and `border-left-color`). Openness is the new axis. But a third
  already exists and already owns *both* channels the obvious split would
  use: `resourceRowTone` (`resourcePresentation.ts`) emits
  `.resource-row-ready` / `-muted` / `-error`, which set `border-left-color`
  (`styles.css:2746-2757`) — the same property selection uses — and
  `.resource-row-error` additionally sets
  `background: var(--ws-color-notice-error)`, which an openness background-mix
  would fight on exactly the rows where availability matters most. So
  "selection on the rail, openness on the background" is not a free split;
  resolve all three explicitly.
- **The foreground route does not inherit.** `.row-title` sets its own
  `color: var(--ws-color-text-primary)` (`styles.css:1143`) and
  `.resource-kind-glyph` its own `var(--ws-color-text-disabled)`
  (`styles.css:2786`), so a `color` change on `.resource-row` will not reach
  them. Desaturating the foreground needs explicit per-descendant selectors.
- **Scope the change across the three presentations.** `ResourceRow` serves
  `workRoot`, `compactWorkRoot`, and `workspace`, and all three already
  receive `isOpenWorkRoot` (`App.tsx:7182`, `7220`, `7247`) — so a `data-*`
  attribute plus a CSS rule hits all three by default, which is probably not
  what is wanted. Two concrete problems on the `workspace` row: its
  `isOpenWorkRoot` reflects only `baseRoot` (`App.tsx:7220-7223`), so a
  workspace whose child worktrees are open renders as a receded parent above
  bright open children; and it spans N roots with no single `rootKey`, so it
  has no well-defined count. Decide per presentation which gets the secondary
  line and which gets the de-emphasis, and state it — the request was phrased
  for work-root rows.
- Prefer emitting `isOpenWorkRoot` as a `data-*` attribute over branching in
  the class string: it keeps the treatment CSS-only, matches the four `data-`
  attributes the row already exposes, and gives the browser acceptance suite
  a selector.
- Prefer a token-level `color-mix` on the row's own background/foreground
  over `filter: saturate()` on the row element. `filter` applies to every
  descendant (glyphs, the kind icon, the `X` button) and establishes a new
  stacking context, which can interact with the drag-over affordance
  (`.resource-row-drag-over`'s `inset` `box-shadow`, `styles.css:1071`).
- Express the 65:35 proportion through type scale and the existing grid
  `gap`, not `grid-template-rows` percentages — a percentage split breaks as
  soon as the type scale changes. Note the effective `min-height` is 34px
  (not the 46px in the overridden earlier CSS block), so there is almost no
  slack to absorb the second line: expect nav rows to get taller, and check
  that the resulting left-nav density is still acceptable rather than
  assuming the change is height-neutral.
- These rows are drag-reorderable (`draggable`, `onDragStart`/`onDrop`,
  `App.tsx:7440-7500`). The added line must not become a drag dead-zone or
  swallow the row's `resource.select` click.

## Deferred scope

**The agent counter is deferred, not dropped.** Its data source is genuinely
undecided right now, and picking one here would pre-empt a decision owned
elsewhere:

- `SurfaceKind: "agent"` already exists and is wired, but to the daemon-
  discovered singleton main-instance projection (`workbench/editorGroups.ts`,
  handled at `App.tsx:5909`/`5931`) — that is what the existing
  `"N pinned main surface(s)"` line counts.
- `SurfaceKind: "agentChat"` is the structured chat surface, currently
  **suspended** behind `AGENT_GUI_SUSPENDED` and un-spawnable.
- Under `260725-research-ws-dashboard-pty-agent-pivot`, a PTY agent is
  expected to be a terminal running an agent CLI, whose surface identity is
  that ticket's open question #1 (a `kind`/profile flag on the terminal pane
  versus a thin wrapper).

Ship terminal and document counts in Phase 1. Add the agent counter once the
pivot settles what an agent pane *is*; until then a counter would either
count the wrong thing or need rewriting immediately.

**Forward note (owner discussion, 2026-07-25) — what the deferred slot will
carry.** The pivot's notification design now claims this slot explicitly (see
`260725-research-ws-dashboard-pty-agent-pivot` `## Notification Path`), so
Phase 1 should leave room for it rather than treat it as an unknown shape:

- The agent counter will be a *split* count, not a single number: working N
  (spinner) / ready M (blinking orange bell glyph). Budget secondary-line
  width for two labelled sub-counts, not one.
- The pivot also wants a Windows-11-style orange attention flash on the row
  itself. That flash must be an independent overlay layer (e.g. a
  pseudo-element), NOT an animation on `background` — `resourceRowTone`
  already owns `background` and `border-left-color` (styles.css 2746-2757,
  with `-error` setting `background` outright). Phase 1 does not implement the
  flash, but it must not consume the row's only overlay affordance either.

This is a compatibility note, not added scope: Phase 1 still ships terminal and
document counts only.

## Spec Impact

Target spec area: `ai-docs/spec/ws-web-dashboard/index.md` —
`#260516-ws-web-dashboard-inspectable-navigation-shell`, which currently
describes left-nav rows in terms of label composition (compact rows, shared
workspace/workRoot labels, sibling drag reorder) with no secondary line and
no open-state presentation rule.

Expected caller-visible change: left-nav work-root rows gain a secondary
information line describing that root's currently open surfaces, and encode
open-versus-closed state visually rather than only through the presence of
the close affordance. No API, route, payload, or command-id change.

Contract-first spec: no. This is presentation refinement over an already
specified nav surface; the exact wording follows the implemented result, and
planned spec text would restate the phase.

## Phases

### Phase 1: Two-line work-root row with open-surface counts and open-state de-emphasis

Both changes land together: they modify the same component
(`ResourceRow`/`WorkspaceGroup` in `App.tsx`) and the same CSS block, and the
secondary line's emptiness on closed roots is only coherent alongside the
open-state treatment.

- Add the secondary line as a second grid child of `.resource-row-select`,
  starting from the unreferenced `.resource-row-meta` rule but giving it its
  own alignment so it sits under the title rather than under
  `.resource-row-main`'s 20px glyph column. Reserve its height so open and
  closed rows keep a uniform row height.
- **Plumb the counts from their real sources** (see Prior Art — *not*
  `paneOrderByRoot`, which by its own in-source CONTRACT does not track
  terminal or readonly-file panes): documents group directly off App-level
  `readOnlyFilePanes`; terminals need `terminalPanes` lifted out of
  `WorkbenchShell` or surfaced through a new upward count callback modelled
  on `onWorkbenchGroupsByRootChange`. This plumbing, not the CSS, is the
  substantial part of this phase. Agent counts are deferred per
  `## Deferred scope`.
- Emit the existing `isOpenWorkRoot` value as a `data-*` attribute and drive
  the de-emphasis from CSS, resolving the three-way axis collision and the
  non-inheriting foreground colours per `## Constraints`.
- Decide and state which of the three `ResourceRow` presentations
  (`workRoot`, `compactWorkRoot`, `workspace`) receive the secondary line and
  the de-emphasis, per `## Constraints`. Do not let the `data-*` rule apply
  to all three by default.
- Reconcile with the existing `.nav-secondary-context`
  `"N pinned main surface(s)"` line (`App.tsx:7262`), which now sits directly
  beneath a row that has grown its own secondary line, so the row does not
  end up with two stacked sub-lines by accident. **Default: keep them
  separate in this phase.** That line counts the daemon-discovered singleton
  main-instance projection — conceptually the same thing as the agent counter
  this ticket defers — so folding it into the new line now would silently
  pre-empt the deferred decision in `## Deferred scope`. Fold it in when the
  agent counter lands and the pivot has settled what an agent pane is. If the
  two stacked lines look wrong in practice, prefer suppressing the older line
  for rows that render the new one over merging their contents.

Verification: the dashboard browser acceptance gate
(`npm run test:browser`), asserting the secondary line's content for a root
with known open surfaces and the open/closed attribute for both states. Note
that `frontend/e2e/dashboard-acceptance.spec.ts` runs
`test.describe.configure({ mode: "serial" })`, so a new step's placement
affects what later steps still run on failure.

Exact type scale, colour-mix ratios, and secondary-line wording are
implementation-time visual choices; the owner asked for roughly 65:35 line
proportion, reduced saturation, and only a very slight brightness drop.
