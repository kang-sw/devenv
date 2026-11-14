---
title: "Worktree page reusing session cards and the file explorer without agent panels"
related:
  261114-feat-ws-dashboard-reusable-file-explorer-card: "prerequisite component reuse approved by (785); source capture 3d2e3463d"
  261114-refactor-ws-dashboard-whiteboard-asset-split: "new UI starts pause during the owner-approved mechanical split"
---

# Worktree page reusing session cards and the file explorer without agent panels

## Background

Clicking a develop-branch/worktree name in the sidebar should open a worktree-focused page with live file viewing and terminal creation. Owner (784)=b settles this as a **new page**, not a mode added to a session's conversation/agent panels.

The owner's clarification, translated from MAIN's verbatim relay: create a new page, reuse exactly the current session screen's mechanisms, including cards and pinning, and remove only the left conversation panel and agent panel, leaving the file explorer. File viewing and terminal spawning remain live.

This is an idea capture only. It does not authorize implementation, a new session, a route/API design or a separate explorer implementation.

## Decisions

- **New worktree page — (784)=b.** The sidebar's develop-branch/worktree name is the entry point. The page is rooted in the selected worktree rather than presented as a conversation with an agent.
- **Reuse the session mechanism.** Keep the existing card and pinning mechanisms; omit the conversation and agent panels. Do not implement a second card system or a separate simplified pinning model.
- **Reuse the explorer — (785)=ok.** Consume the reusable explorer component owned by `261114-feat-ws-dashboard-reusable-file-explorer-card` (the approved capture was committed as `3d2e3463d`). The worktree page is another consumer, not a competing explorer or workspace-picker UI.
- **Worktree-bound terminals — (785)=ok.** Terminals created from this page spawn with `cwd` equal to the selected worktree path. File viewing and terminal operations remain live rather than being a static directory snapshot.
- **Respect the split pause.** This is one of the new UI starts paused while `261114-refactor-ws-dashboard-whiteboard-asset-split` is active. Capturing the idea does not bypass MAIN's cut-line scheduling or start signal.

## Constraints

- Preserve the explorer's existing read-only file-viewing scope and guarded-root/host-action boundaries. A selected worktree and a new page do not authorize broader filesystem access, filesystem mutation, new symlink permissions or a new terminal authority model.
- Preserve the reusable explorer component's state and attachment boundaries rather than copying its internals. This idea does not override its approved feature scope or select new storage fields, libraries or component event names.
- A worktree page is not authority to create an agent session, enable a root flag, control another session or add a new conversation/agent panel under a different name.
- No URL shape, endpoint, wire schema, persistence model, additional view policy or new public component API is selected by this capture. Those remain outside this settled page concept until separately designed.
- Before future tooling implementation, read the matching `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md` rules. This documentation-only idea changes no cross-version surface.

## Phases

### Phase 1: Deliver the approved worktree-focused page after its prerequisites

When the idea is separately prepared for implementation and MAIN schedules its start, reuse the current session page's card/pinning mechanisms and the approved reusable explorer component. Open the page from a sidebar develop-branch/worktree name, omit conversation and agent panels, keep live file viewing and terminal creation, and bind terminal `cwd` to that selected worktree.

Future verification must distinguish two worktrees: selecting one opens its own files and starts terminals in its exact path, without accidentally using the source session's worktree. Check reused card/pinning behavior, absence of conversation/agent panels, live file viewing, and unchanged explorer/file/terminal authority boundaries. Implementation design, tests, screenshots, runtime acceptance and any cross-version surface choices belong to later work, not this idea capture.
