---
title: "Pi adapter: Down arrow at the bottom of the lead editor moves into the agent picker"
parent: 260908-epic-ws-pi-subagent-conversation-view
related:
  260908-feat-ws-pi-subagent-audit-window-and-owner-steering: lands the `/audit` picker modal and its keyboard shortcut; this ticket is only the Down-arrow affordance on top of it
  260905-feat-ws-pi-live-agent-widget: the `belowEditor` widget the owner would visually "move into"
---

# Pi adapter: Down arrow at the bottom of the lead editor moves into the agent picker

## Background

Owner wish (2026-09-08, while designing the subagent audit window): with
the live-agent widget sitting right below the editor, pressing **Down** in
the Pi editor when the cursor is already on the last line, the input history
is at its bottom, and the key has no other valid action, should move focus
"down into" the agent list, where Up/Down select a child and Enter opens its
conversation view. Two keystrokes with no command and no chord.

What the extension API (pi-coding-agent 0.84.4) offers today, as found during
that design:

- `setWidget` components only render; they never receive input. Keys go to
  the focused component — the editor, or a `ctx.ui.custom` overlay.
- `registerShortcut` binds a key globally while the editor is focused; it
  cannot be conditional on "the editor had nothing to do with this key".
  Binding Down would take the editor's own cursor/history navigation away.
- No editor fall-through hook (an "unhandled key" callback) is documented
  for extensions.

So the exact affordance is not reachable as an extension yet. The child
ticket therefore lands the picker as a separate focused modal
(`ctx.ui.custom`) reached by `/audit` or a keyboard shortcut; this ticket
keeps the Down-arrow idea alive.

## Phases

### Phase 1: Verify whether a Down-arrow hook exists, then wire it

1. Verify against the installed Pi version whether any of these exists:
   an editor "unhandled key" / fall-through callback exposed to extensions;
   a way to make a `setWidget` component focusable; or a conditional
   shortcut predicate. Record the finding here with file references.
2. If a hook exists: on Down at the editor's last line with history at its
   bottom, open the child B picker modal with the first row selected; Up on
   the first row (or Esc) returns focus to the editor. Never intercept Down
   in any other editor state.
3. If none exists: record the gap and the Pi-side change that would enable
   it, and leave this ticket in `idea/` until Pi grows the hook.
