---
title: "Unify dashboard quick-open command bar, custom command buttons, and keyboard shortcuts on one command bus"
parent: 260711-epic-ws-dashboard-command-surface
related:
  260710-epic-ws-dashboard-terminal-ux-polishing: sibling UX-polish board; this idea's parent epic was split out from it because its Non-Scope excludes new product surfaces
  260711-idea-dashboard-agent-facing-mcp-control-surface: agent/MCP-facing half of custom commands, kept under 260622 instead of this epic
  260711-idea-dashboard-workroot-scoped-artifact-consolidation: storage substrate this ticket's custom command definitions depend on (.ws-dashboard/scripts/)
  260722-feat-dashboard-hotkey-config-framework: hotkey configuration framework this ticket's Phase 1 shortcut-capture layer now binds onto, per the Agenda A restructure note below
related-mental-model:
  - ws-web-dashboard
---

# Unify dashboard quick-open command bar, custom command buttons, and keyboard shortcuts on one command bus

> **Restructure note (2026-07-22):** the global shortcut-capture layer this
> ticket's Phase 1 plans now sits on top of the hotkey configuration
> framework decided in Agenda A UX review and tracked as
> `260722-feat-dashboard-hotkey-config-framework` (tmux-style leader-only
> model, `Ctrl+Space` leader, leader-sub bindings first-class). This ticket's
> prefix grammar and phased plan below are unaffected; only the underlying
> shortcut-capture mechanism gains a formal binding registry to integrate
> with. See the parent epic's Restructure section for the full layer
> ordering.

## Background

Three previously-separate feature requests turn out to share one
underlying gap and one underlying reusable asset:

1. A VSCode-style generic command bar (prefix characters like `%`, `@`,
   `!`, `#`, `:` triggering different actions: full-text search, go-to-file,
   run-arbitrary-command-in-workroot, go-to-line).
2. Custom command buttons registrable in the sidebar/topbar, per work
   root, executable by click, by shortcut, and (later) by an agent through
   a dashboard-exposed MCP surface (see the separate
   `260711-idea-dashboard-agent-facing-mcp-control-surface` ticket for the
   agent-facing/MCP half of this — this ticket only covers the
   human-facing UI and the shared dispatch mechanism).
3. Keyboard shortcut registration (e.g. `Ctrl+`` `` to open/focus a
   terminal).

**Shared asset**: `commands.ts` already defines a `DashboardCommand`
discriminated-union dispatch system (`DashboardCommandId`/
`DashboardCommandPayload`, consumed via `executeCommand` at `App.tsx:884`
and dispatch sites like `App.tsx:4021`). Any of the three surfaces above
just needs to resolve to a `commandId` + payload and hand it to this
existing bus — the bus itself does not need to be reinvented.

**Shared gap**: there is no app-level global-shortcut-capture layer today.
`App.tsx` has exactly two global `keydown` listeners:
`useDismissableMenu`'s `dismissOnEscape` (`App.tsx:1198`, Escape-only) and
the terminal input-forwarding fallback (`App.tsx:6711-6768`, which
special-cases and consumes several Ctrl+letter combos as terminal control
bytes and is not a general shortcut system). A new global shortcut layer
must coexist with the terminal fallback's focus/IME/`isActivePane` guards
without breaking terminal keystroke passthrough — that fallback's guard
pattern (`offsetParent` visibility check, `isComposing` check, skip when
target is `input`/`textarea`/`contentEditable`, skip when focus is already
inside the terminal container) is the closest existing precedent for how
to avoid a new shortcut layer eating terminal input.

## Findings: VSCode Quick Open prefix grammar (reference, verified against VS Code source)

| Prefix | Meaning |
|---|---|
| *(none)* | Go to File (fuzzy path match) |
| `>` | Command Palette |
| `@` | Go to Symbol in current file |
| `@:` | Go to Symbol in current file, grouped by symbol kind |
| `#` | Go to Symbol in workspace (all files) |
| `:` | Go to Line (and column) |
| `::` | Go to character offset (not line) |
| `?` | Help — lists all available prefixes |
| `%` | Quick text search (file-content grep), additive to the dedicated Ctrl+Shift+F search view |

Notes relevant to our design:
- Prefixes are not freely composable. The only compound form is `@:`
  (grouped symbols) and the `filename:line:column` suffix syntax parsed
  by a shared `extractRangeFromFilter()` helper reused from the `:`
  Go-to-Line matcher — this is a suffix parse on the plain (no-prefix)
  file box, not a nested-prefix combination.
- Full-text content search has two paths in VSCode: the dedicated search
  view (`Ctrl+Shift+F`, full-featured: regex, include/exclude globs,
  replace) is primary; `%` inside Quick Open is a newer, lighter
  additive entry point into the same underlying search. We should decide
  up front whether the dashboard needs both or only the Quick-Open-style
  entry.
- The prefix-to-provider mapping in VSCode is a hardcoded core registry
  (`IQuickAccessRegistry`), not user-remappable via settings, though
  internally other first-party features (like `%`) register against the
  same internal API. We are not obligated to mirror this rigidity, but it
  is a reasonable precedent for keeping the prefix set small and fixed
  rather than user-configurable, at least for an initial version.

## Findings: what the daemon does and does not support

- No daemon-side full-text-search-across-workroot endpoint exists today
  (only git/worktree/discovery/server routes were found). A `%`-style
  prefix needs either a new daemon endpoint or client-side search of
  already-fetched content (insufficient for large repos).
- No one-shot "run arbitrary command in workroot, capture output" daemon
  endpoint exists; only PTY-backed terminal session creation and
  git/ssh subprocess calls for specific features. A `!`-style prefix
  either reuses the terminal-session mechanism (spawn a session and feed
  it the command) or requires a new bounded exec endpoint.
- `@`-style go-to-file has a plausible existing surface
  (`workRootFiles.ts`, `documentRawEditor.tsx`, `documentViewer.tsx`), but
  go-to-line support inside those viewers was not confirmed and needs a
  follow-up check before committing to the `:` suffix design.

## Decisions

- Custom command definitions are stored per-workroot at
  `<workroot>/.ws-dashboard/scripts/` (owner, 2026-07-11), not in the
  global `DashboardStateStore`. Files are conditionally git-tracked: a
  `*.local.*`-style filename marker means local-only (gitignored),
  everything else tracks normally in the repo — mirroring the
  `.env`/`.env.local` convention, so team-shareable custom commands can
  be committed while personal overrides stay local. `scripts/` turns out
  to need **no physical symlink/junction at all**: since only the
  dashboard daemon itself reads these files, it can resolve the root
  workroot path pragmatically (via `git rev-parse --git-common-dir`'s
  parent) from any worktree and read `.ws-dashboard/scripts/` there
  directly — it never needs the directory to physically exist inside
  each worktree. The `.ws-dashboard/` top-level directory, the separate
  `.ws-dashboard-shared` path reserved for content that *does* need a
  physical link (currently reserved for build/dependency artifacts, not
  custom commands), and related open questions are tracked in
  `260711-idea-dashboard-workroot-scoped-artifact-consolidation`, a
  sibling ticket this one now depends on for the storage substrate.
- **Scope baseline: mirror VSCode Quick Open UX, but single-workroot only**
  (owner, 2026-07-11): the command bar's prefix grammar and behavior
  should follow the VSCode reference table above as the starting shape
  (no prefix = go to file, `@` = go to symbol, `#` = go to symbol in
  workspace, `:` = go to line, `%` = text search, etc.), but every one of
  these operates scoped to **the single work root currently being
  viewed** — not across all work roots the dashboard has open. This
  matches VSCode's own single-workspace-folder Quick Open behavior (multi-
  root workspace search is a separate, heavier VSCode feature we are not
  adopting) and avoids designing a cross-workroot index/search fan-out for
  v1. If cross-workroot search is ever wanted, it should be a distinct,
  later idea, not baked into this ticket's baseline.

## Phases

Owner decision (2026-07-11): `%`/`!` are an implementation goal, not
deferred indefinitely, but land through a phased frontend → backend →
frontend split rather than all at once, since the daemon has no
supporting endpoints yet (see Findings above) and shipping the
client-only-supportable prefixes first avoids blocking on backend design.

### Phase 1: Frontend shell, client-only prefixes, shortcut layer

- Quick-open modal and prefix parser, dispatching through the existing
  `DashboardCommand`/`executeCommand` bus (`commands.ts`, `App.tsx:884`).
- No-prefix go-to-file and `:` go-to-line, the two prefixes Findings above
  confirm have a plausible existing surface
  (`workRootFiles.ts`/`documentRawEditor.tsx`/`documentViewer.tsx`) —
  confirm go-to-line support in those viewers as part of this phase, not
  before.
- New global keyboard-shortcut-capture layer, coexisting with the
  terminal input-forwarding fallback's guard pattern (`App.tsx:6711-6768`)
  per the Background note above. Used both to invoke the command bar
  itself and for other dashboard shortcuts (e.g. `Ctrl+`` `` to
  open/focus a terminal).
- Custom command button UI shell (registration form, sidebar/topbar
  placement, click-to-execute wiring) for commands already expressible
  through the existing `DashboardCommand` bus — this phase does not yet
  require `.ws-dashboard/scripts/`-backed custom scripts to exist.
- `@`/`@:`/`#` symbol search are deferred past this phase: no existing
  symbol-indexing surface was found in Findings, so they need their own
  design (whether via an existing viewer's language support or new
  daemon-side indexing), not just a UI slot.

### Phase 2: Backend — daemon endpoints for `%` and `!`

- `%`: new daemon endpoint for full-text search across the single
  currently-viewed work root (per the single-workroot scope Decision
  above). Backend choice (e.g. shelling out to `ripgrep` vs. an in-process
  search) is an implementation detail for this phase, not decided here.
- `!`: new support for running an arbitrary command in the current work
  root and capturing its output — either by reusing the existing
  PTY-backed terminal-session mechanism (spawn a session scoped to that
  work root and feed it the command) or a new bounded one-shot exec
  endpoint if a quick command shouldn't need a full PTY session. Decide
  based on whether the quick-open UX wants streaming output or a
  fire-and-capture result.
- Daemon-side list/read support for `.ws-dashboard/scripts/` custom
  command definitions, per the sibling storage ticket
  (`260711-idea-dashboard-workroot-scoped-artifact-consolidation`).

### Phase 3: Frontend — wire `%`/`!` and ship custom commands end-to-end

- Wire the `%` and `!` prefixes into the quick-open bar against the Phase
  2 endpoints.
- Wire custom-command button execution against `.ws-dashboard/scripts/`-
  backed definitions (human-click-triggered only; the agent/MCP-facing
  path stays out of scope for this epic per Non-Goals below).
- Re-evaluate `@`/`@:`/`#` symbol search once Phase 1-3 ship; treat as a
  stretch/later phase if it still needs its own indexing design.

## Open Points

- ~~Epic ownership~~ — resolved 2026-07-11: split out as its own epic,
  `260711-epic-ws-dashboard-command-surface`, sibling to `260710` and
  `260622`, since bundling agent-harness/session-key concerns with
  human-facing command-bar UX was judged too broad a stretch.
- ~~Whether `%`/`!` need daemon API work up front~~ — resolved 2026-07-11:
  yes, both are implementation goals, sequenced via the Phases above
  (frontend-first, then backend, then frontend wiring) rather than
  deferred indefinitely.
- Whether the command-bar UI on this large (8000+ line) `App.tsx` should
  be extracted into its own module from the start, given the existing
  file's size and the "Responsibility check" code standard.

## Non-Goals

- The agent/MCP-facing half of custom command buttons (agents triggering
  registered commands) — tracked separately in
  `260711-idea-dashboard-agent-facing-mcp-control-surface`.
- Redesigning `DashboardCommand`/`executeCommand` itself; this idea only
  proposes new front-ends that dispatch into it.
