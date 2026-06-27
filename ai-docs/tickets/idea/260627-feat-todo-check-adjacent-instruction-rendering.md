---
title: Todo check adjacent instruction rendering
---

# Todo check adjacent instruction rendering

## Background

`ws.todo.check` renders the todo list after marking an item complete. Once todo
items can carry long `instruction` text, rendering every instruction at each
checkpoint can flood the output. Rendering no instructions loses the immediate
guidance that makes the checkpoint useful.

The desired behavior is a focused checkpoint rendering mode: after a todo item is
checked, include full `instruction` text only for unfinished todo items
immediately adjacent to the checked item, meaning the previous and next list
items. All other items should render compactly as `{key} title`.

The purpose is to guide the next actionable instruction without turning every
checkpoint into a full workflow manual.

## Investigation Notes

- Confirm where `ws.todo.check` chooses its post-check rendering mode.
- Confirm whether adjacency is based on the current todo order before or after
  marking the checked item done. The likely intended rule is list position after
  the state update, while preserving stable todo order.
- Confirm how this interacts with existing compact/full todo rendering modes,
  workflow manual rendering, and checkpoint/status output.
- Preserve compact rendering for completed adjacent items; only unfinished
  adjacent items should receive full instruction text.

## Promotion Criteria

Promote this ticket once the implementation surface is identified and the
contract can be stated against the existing todo rendering spec.

Expected implementation-ready scope:

- Add or reuse a todo rendering mode for post-check checkpoint output.
- Render full `instruction` text for only the unfinished previous and next todo
  items adjacent to the checked item.
- Render all other todo items compactly as `{key} title`.
- Cover edge cases for first item, last item, adjacent completed items, and
  missing `instruction` values.
- Update the relevant todo rendering spec or mental model if the behavior is
  caller-visible.
