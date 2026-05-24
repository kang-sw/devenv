---
title: ws dashboard editor and WorkRoot management workset
related:
  260514-epic-ws-web-dashboard-mvp: umbrella dashboard MVP board
  260524-feat-ws-dashboard-document-viewer-editor-substrate: editor and markdown viewer track
  260524-feat-ws-dashboard-add-git-worktree-ui: WorkRoot management Git worktree creation slice
  260524-feat-ws-dashboard-git-aware-workroot-toolbar: WorkRoot toolbar Git status and branch controls
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard editor and WorkRoot management workset

## Scope

This workset groups the next discussed dashboard implementation area after the
visual-polishing line. It is a recovery board for a short run of related child
tickets, not an implementation target itself.

The workset covers:

- a reusable markdown/document viewer and raw-text editor substrate;
- daemon-backed whole-document translation overlay support;
- workspace-menu Git worktree creation;
- selected-WorkRoot Git-aware toolbar chips, branch controls, and safe sync
  actions.

## Non-Scope

- Implementing this workset directly as one branch or one phase.
- Multi-server management, agent view panel redesign, diagnostics/task panels,
  broader terminal UX redesign, or dashboard-wide visual-system refresh.
- Git worktree removal, force push, merge/rebase pull, conflict resolution,
  branch deletion/rename, remote-branch tracking UX, generic file-manager
  operations, or broad filesystem watcher correctness.
- Full IDE/editor parity, rich markdown editing, collaborative editing,
  Excalidraw/draw.io/HTML renderer support, or general translation-provider UI.

## Included Tickets

- `260524-feat-ws-dashboard-document-viewer-editor-substrate` - done; editor
  polishing track. Completed reusable Markdown/document viewing,
  daemon-backed translation overlay, raw-text edit/save fan-out, and document
  events.
- `260524-feat-ws-dashboard-add-git-worktree-ui` - ready; first WorkRoot
  management slice. Replace the workspace trash affordance with an overflow
  menu, keep remove workspace in that menu, and add Git worktree creation with
  daemon-resolved auto branch/path preview.
- `260524-feat-ws-dashboard-git-aware-workroot-toolbar` - todo; companion
  WorkRoot toolbar slice. Add Git-aware branch/status chips for selected Git
  workRoots, including fetch, plain push, and fast-forward-only pull controls.

## Cross-Ticket Decisions

- Preserve the existing dashboard resource model: workspaces own root
  workRoots, linked Git worktrees remain child workRoots, and public API/routes
  use `workRootId` rather than Git-worktree identity names.
- Keep host paths as authenticated request data only. Command payloads, logs,
  browser-visible errors, copied pathrefs, and diagnostics should avoid private
  absolute host paths.
- Route visible controls through stable dashboard command ids where the current
  command model applies.
- Browser-visible UI changes require browser-level evidence against the
  daemon-served production frontend, not only unit tests or Vite builds.
- Git operations should follow Git's own safety behavior. The dashboard should
  not invent force, stash, merge/rebase, conflict-resolution, or branch policy
  semantics in these first slices.
- Translation is a viewer overlay over immutable content hashes, not mutation
  of source files. The daemon owns provider configuration, model discovery,
  prompting, bounded parsing, and cache behavior.
- File watching or Git watching may be used as freshness hints only when a
  child ticket defines the scope. Correctness should come from daemon reads,
  content hashes, resource refresh, or explicit command results.

## Completion Criteria

- Done: the three included tickets are implemented or split into more precise
  completed tickets with equivalent editor/translation and WorkRoot Git UX
  coverage.
- Dropped: the dashboard direction moves away from browser-native document
  panels or from Git-aware WorkRoot management in the top-level dashboard UI.
- Deferred: multi-server forwarding of Git/document operations, agent panel
  redesign, diagnostics/task panels, generalized provider configuration UI,
  and advanced Git workflows belong to later tickets.
