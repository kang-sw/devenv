---
title: Map persistable ws dashboard UI state
parent: 260710-epic-ws-dashboard-terminal-ux-polishing
related:
  260523-feat-ws-dashboard-persist-open-workroots: first daemon-local persisted resource seed
  260523-feat-ws-dashboard-workroot-registry-activation: durable workspace/workRoot spine for later UI persistence
  260523-feat-ws-dashboard-terminal-tab-restore: first browser descriptor replay surface
  260523-feat-ws-dashboard-readonly-file-pane-restore: concrete next descriptor replay surface
---

# Map persistable ws dashboard UI state

## Background

Dogfood restart loops now show that "opened workRoot" is only the first layer
of useful dashboard continuity. The dashboard has several state classes that
could be persisted, but they have different ownership and risk profiles:
daemon resource seeds, browser presentation state, local acknowledgement state,
and potentially sensitive runtime/session details.

This research ticket captures candidate surfaces before splitting additional
implementation tickets. The default posture should remain conservative:
persist logical, user-visible descriptors and preferences; avoid host paths,
daemon-private ids, backend session paths, process ids, raw transcripts, raw
terminal output, or anything that would make browser state more authoritative
than daemon/resource state.

## Candidate Surfaces

- **Selected workRoot / selected resource**: restore the last selected workRoot
  and resource row after `/api/dashboard/resources` loads. Useful and low
  risk, but must degrade when the resource is no longer present.
- **File explorer tree state**: expanded directories, selected row, scroll
  position, and maybe last visited directory per workRoot. Useful for repeated
  navigation; should store only workRoot-relative paths.
- **Read-only file panes**: preview and pinned pane descriptors. Concrete child
  ticket exists as `260523-feat-ws-dashboard-readonly-file-pane-restore`.
- **Workbench layout**: Dockview groups, tab order, active pane per group,
  split proportions, and selected group per workRoot. High value, but must be
  sanitized against current registry/resource state and should not store surface
  kind or daemon ids as authority.
- **Workbench pane visibility**: whether WorkRoot Activity is open, agent pane
  manually closed, and support panes that are reversible. Medium value; should
  be keyed by logical surface keys and current resource availability.
- **Activity Console local state**: selected activity item, dirty/read
  acknowledgement timestamps, ribbon scroll, transcript scroll/tail-follow
  state, and load-more cursor hints. High dogfood value, but acknowledgements
  need timestamp semantics so refresh can distinguish "new since last seen"
  from ordinary live updates.
- **Terminal tab descriptors**: already implemented as new-session replay.
  Future improvement could add explicit PWD capture if a safe shell integration
  appears.
- **Terminal UI affordances**: selected terminal tab, font size, scrollback
  visual position, and shell profile preference. Avoid persisting raw output or
  pretending scrollback survives daemon restart unless a separate bounded
  output-history design exists.
- **Command palette / keybinding preferences**: keymap profile, recent
  commands, and pinned commands. Relevant to Tmux-like bindings; should be
  logical command ids only.
- **Dashboard chrome preferences**: sidebar width, collapsed sections, density,
  theme, split orientation preference, and toolbar visibility. Low risk and
  useful, but should not compete with the main persistence milestones.
- **Root picker history**: recent manually opened paths or labels. Useful but
  privacy-sensitive because it can expose host paths; should remain opt-in or
  daemon-local, not loggable command payload.
- **Linked worktree visibility choices**: whether an auto-discovered sibling
  worktree was acknowledged, hidden, or opened. This should stay coupled to
  `260523-feat-ws-dashboard-linked-worktree-discovery`.
  ~~The prior direction rejected invisible worktrees: known workRoots stayed
  visible with live-derived availability until a future explicit
  forget/remove action existed.~~ **Superseded 2026-07-22:** owner finalized
  "Agenda B — worktree UX" and reversed this — direction is now a plain UI
  hide (worktree stays on disk, branch untouched; hiding only affects the
  dashboard's visible list), restored via the root workRoot's "..." settings
  menu → a "hidden worktrees" submenu, where clicking a hidden entry
  un-hides it. See `260525-feat-ws-dashboard-workroot-polishing-backlog`
  Phase 3 for the full, final spec (this also covers the paired removal
  confirmation modal and branch-delete checkbox, which are new decisions not
  previously tracked in this candidate list).

## Risk Boundaries

- Persist descriptors, not live authority. On restore, the daemon/resource/file
  APIs revalidate every item.
- Use command ids and logical targets for replayable controls so keyboard
  bindings and mouse actions converge.
- Keep browser-local state browser-local unless the state must be visible before
  a browser opens; opened workRoot seeds are the exception already handled by
  daemon-local state.
- Prefer explicit unavailable/error UI over silently dropping a restored surface
  when the underlying resource changed.
- Treat raw output, transcripts, host paths, backend session paths, pids, and
  cache paths as non-persistence candidates unless a later ticket states a
  bounded, privacy-reviewed format.

## Split Candidates

- Feature: persist selected workRoot/resource and file explorer tree state.
- ~~Feature: persist sanitized workbench layout and split proportions per
  workRoot.~~ Taken up as Phase 5 of
  `260703-feat-dashboard-workroot-session-keepalive`, alongside terminal
  visual-buffer restore (a "Terminal UI affordances" candidate below,
  narrowed to scrollback/cursor/scroll-position restore on reload rather
  than font size or shell profile preference).
- Feature: persist WorkRoot Activity pane, selected item, dirty acknowledgement,
  and transcript scroll state.
- Feature: persist dashboard command/keybinding preferences.
- Feature: persist root picker history with explicit privacy boundaries.
