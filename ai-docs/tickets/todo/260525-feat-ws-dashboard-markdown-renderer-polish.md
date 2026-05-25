---
title: Polish dashboard Markdown renderer context
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260524-feat-ws-dashboard-document-viewer-editor-substrate: initial document viewer substrate that exposed the rendering gaps
  260524-epic-ws-dashboard-editor-workroot-workset: completed editor and WorkRoot workset whose follow-up polish should stay scoped
spec:
  - 260524-ws-dashboard-document-viewer-mode
  - 260524-ws-dashboard-document-translation-overlay
related-mental-model:
  - ws-web-dashboard
---

# Polish dashboard Markdown renderer context

## Background

The dashboard Markdown viewer now has a reusable AST-backed document model,
block selection, translation overlays, pathrefs, GFM tables, task-list support,
and Obsidian-style callout scaffolding. Dogfood review showed that the visible
renderer still feels rough: inline code has no conventional token chrome,
unordered lists lose natural bullets and nesting, ordered lists lose numbering,
and adjacent list items inherit document-block spacing rather than compact
intra-list rhythm. The block selection interaction also feels too heavy because
ordinary body clicks currently double as document-block selection, making the
rendered document feel less like normal selectable text.

The cause is architectural rather than only CSS. The viewer currently flattens
top-level Markdown lists into separate selectable `listItem` blocks before
rendering, and the React renderer emits list items as generic divs. That keeps
translation/pathref units addressable, but it discards rendered list context
that normal Markdown readers expect.

## Decisions

- Keep CodeMirror and raw-text edit mode out of this ticket. This ticket
  polishes read-only Markdown view mode only.
- Preserve block identity, selection, translation overlay, and pathref copying.
  A visual list-context fix must not collapse all list items into an
  unaddressable monolithic block.
- Render Markdown lists with semantic list structure where possible:
  `ul`/`ol` containers, `li` items, ordered `start` metadata, nested list
  indentation, and task-list checkbox alignment.
- Treat the current block flattening as an implementation detail that may need
  a view grouping layer. Adjacent top-level list-item blocks can remain
  independently selectable while sharing a rendered list context.
- Move block selection and block-level actions into a dedicated left-side
  interaction rail. The rendered Markdown body should behave like normal text:
  dragging or selecting inside the body should select text, not toggle document
  blocks.
- The rail should appear or strengthen on hover/focus and provide a natural
  checkbox or handle affordance for selecting blocks, range-selecting adjacent
  blocks, and exposing copy/pathref/translated-copy actions without visually
  dominating the document.
- Style inline code as a conventional Markdown token with monospace font,
  subtle background, border, padding, and restrained warm accent color.
- Keep raw HTML disabled or inert; this polish does not reopen the deferred
  sanitized/sandboxed HTML decision.

## Phases

### Phase 1: Restore list context and inline code polish

Update the Markdown viewer so unordered, ordered, nested, and task lists render
with natural Markdown semantics while preserving selectable block actions and
translation overlay behavior. Add explicit renderer support for Markdown `list`
nodes, render list items as list items rather than standalone divs where the
visual context requires it, and tune list CSS for compact intra-list spacing,
nested indentation, marker readability, and task checkbox alignment.

Also add inline-code-specific styling so code spans look like conventional
Markdown tokens rather than unstyled text. Verification should include focused
frontend tests or fixtures for inline code, unordered lists, nested lists,
ordered lists including non-default starts when available, and task lists, plus
browser evidence for a representative Markdown pane.

### Phase 2: Move block selection into an interaction rail

Replace body-click block selection with a left-side Markdown block rail that
appears naturally on hover or focus. The rail owns block selection, range
selection, and block action affordances such as visible-text copy, translated
copy when available, and pathref copy. The document body remains readable and
text-selectable, so dragging across rendered Markdown text behaves like normal
browser text selection rather than block toggling.

The rail should feel integrated with the document surface rather than like a
literal form checkbox column. It may use a subtle vertical ribbon, check/handle
glyphs, hover chrome, and selected-state markers, but it must keep the main
Markdown layout stable and avoid stealing space from list markers or nested
list indentation. Verification should include text selection behavior, block
range selection through the rail, pathref copy from selected blocks, and
translation-overlay action availability when translated blocks are present.
