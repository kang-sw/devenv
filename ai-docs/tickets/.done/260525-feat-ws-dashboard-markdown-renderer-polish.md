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
completed: 2026-05-25
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

### Result (3eb29bc) - 2026-05-25

Implemented a Markdown render grouping layer that keeps adjacent top-level
list-item blocks in shared semantic `ul` or `ol` containers while preserving
each item's block id, ordinal, pathref, translation lookup, and selection
state. The renderer now supports nested lists, task-list checkboxes, ordered
list `start` metadata, compact list spacing, and conventional inline-code
token styling. Raw HTML remains inert.

Focused document-viewer tests now cover inline code, unordered and nested
lists, task lists, non-default ordered-list starts, inert raw HTML, and list
unit grouping. Browser acceptance fixture Markdown was expanded to exercise
inline code, nested lists, ordered lists, and task lists.

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

### Result (3eb29bc) - 2026-05-25

Moved Markdown block selection and block copy actions from body-click behavior
into a left-side interaction rail on each rendered block. The body no longer
has a block-toggle click handler, so rendered Markdown remains normal
text-selectable document content while the rail owns select/deselect,
shift-range selection, visible-text copy, translated-copy availability, and
pathref copy. The rail appears through subtle hover/focus/selected chrome and
keeps block actions outside the document text flow.

Tests cover rail selection toggling, shift-range selection, and the absence of
the old global action strip. Browser acceptance verifies that body clicks do
not select a block, rail selection does select it, and pathref copy still
returns a workRoot-relative reference.

#### Edition (b34f1fe) - 2026-05-25

Tightened the browser acceptance nested-list assertion after semantic list
rendering made both the outer and nested unordered lists match the same class.
The test now scopes the assertion to the nested unordered-list descendant and
keeps the renderer implementation unchanged.

#### Edition (422bdbd) - 2026-05-25

Moved Markdown copy actions out of the per-block rail after dogfood text
selection showed rail glyphs such as `V`, `T`, and `@` could be copied with the
document body. The rail now only owns block selection and range selection, while
a single selected-block toolbar near the document surface owns visible-text,
translated-text, and pathref copy actions for the selected block set.

The follow-up also removed selected-state glyph text from the rail, marked rail
and toolbar chrome as non-selectable, and softened inline-code styling by
removing the visible border and using a subtler warm background. Verification
covered document-viewer tests, e2e TypeScript compilation, and frontend build.
The full Playwright gate passed in the implementer run, while a later local
rerun hit the separately tracked sticky agent tab close confirmation issue
before reaching the Markdown step.

#### Edition (7528758) - 2026-05-25

Adjusted `Copy visible` serialization so selected list, task-list, and code
blocks copy closer to their rendered/source Markdown shape. Untranslated list
items now preserve bullet, task, numbering, nested indentation, and adjacent
same-family list items are joined with single newlines instead of blank lines.
Untranslated code blocks copy their source fenced block so whitespace and line
breaks survive.

Translated blocks still use the translated markdown returned for that block
without inventing list markers. Focused document-viewer tests cover mixed
heading, paragraph, unordered/task list, ordered list, translated list, and
fenced code visible-copy formatting.
