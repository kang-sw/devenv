---
title: Add CodeMirror document edit mode
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260524-feat-ws-dashboard-document-viewer-editor-substrate: initial raw edit/save substrate and view/edit pane split
  260525-feat-ws-dashboard-markdown-renderer-polish: adjacent read-only Markdown viewer polish that must stay separate from edit mode
spec:
  - 260516-ws-dashboard-readonly-text-pane
  - 260524-ws-dashboard-document-edit-save-fanout
related-mental-model:
  - ws-web-dashboard
completed: 2026-05-25
---

# Add CodeMirror document edit mode

## Background

Document panes now separate format-aware read-only view mode from raw-text edit
mode, but edit mode still uses a plain browser textarea. The dashboard needs a
browser-native editor layer that feels competent for workflow files without
turning the dashboard into a full IDE.

The accepted direction is CodeMirror 6 rather than Monaco for this first
editor substrate. The dashboard's near-term editing needs are embedded
raw-text editing, custom command/keybinding integration, syntax highlighting,
and theme consistency inside pane chrome. LSP, completion, symbol navigation,
and IDE-grade diagnostics are non-goals for this project unless a later ticket
creates a separate code-editor product direction.

## Decisions

- Replace the document edit-mode textarea with CodeMirror 6 inside the existing
  document pane; do not create a separate tab or surface identity for editing.
- Keep Markdown view mode as the dedicated document renderer. CodeMirror edits
  raw Markdown text only and does not own translation overlays, block
  selection, pathref copy, or rendered Markdown semantics.
- Preserve the dashboard command model. Mode switching, save, revert, dirty
  state, and stale/conflict presentation continue to use the existing document
  command ids and source-identity fan-out behavior.
- Add a small language-extension mapping by file extension or language hint for
  common workflow files such as Markdown, TypeScript/JavaScript, JSON, CSS,
  HTML, YAML, Python, Rust, shell, and plain text. Missing language packages
  fall back to plain text rather than blocking editing.
- Scope first-pass editor affordances to syntax highlighting, line numbers,
  search, bracket matching, basic indentation behavior, selection/focus
  stability, and dashboard dark-theme integration.
- Keep custom keybinding room open, including a future Vim-like modal layer or
  ws-specific command bindings, but do not require a complete modal editing
  system in the first implementation.
- Explicitly defer LSP, completion, go-to-definition, diagnostics, formatting,
  multi-file refactors, and file-tree-wide code intelligence.

## Phases

### Phase 1: Install CodeMirror as raw edit substrate

Introduce CodeMirror 6 dependencies and a small reusable document editor
component for raw-text edit mode. Wire the component to the existing document
pane state so edits update the draft, clean/dirty/stale state remains correct,
and save/revert command buttons continue to operate through the current command
dispatcher and write API.

The editor must fit within the existing pane body without page-level scroll,
match the dashboard dark visual system, keep the document ribbon height stable,
and preserve source-identity fan-out for same-file multi-pane saves.

### Result (1726c1b) - 2026-05-25

Implemented a reusable CodeMirror 6 raw editor inside the existing document
pane edit mode. The pane still owns view/edit switching, draft state, save,
revert, stale/conflict presentation, and same-source save fan-out through the
existing command ids and write API.

The editor replaces the browser textarea without creating a new workbench
surface identity. It uses dashboard dark-theme styling, fills the pane body,
scrolls internally, keeps the ribbon chrome stable, and preserves the
read-only Markdown viewer as the separate format-aware view mode.

### Phase 2: Add language and editing affordances

Add extension-based syntax highlighting and core editing affordances: line
numbers, search, bracket matching, basic indentation, and a fallback plain-text
mode. Keep the mapping conservative and dependency-light; unsupported file
types should still open and save as text.

Verification should cover Markdown/raw text save behavior, dirty/revert/stale
state, same-source fan-out, at least one highlighted language, plain-text
fallback, and browser evidence that the editor scrolls inside the pane and
does not interfere with view/edit mode switching.

### Result (1726c1b) - 2026-05-25

Added conservative language detection from language hints, extensions, and
paths for Markdown, TypeScript/JavaScript, JSON, CSS, HTML, YAML, Python, Rust,
shell, and text fallback. Language packages load asynchronously and unsupported
types remain plain text.

The first-pass CodeMirror affordances include line numbers, search keymaps,
bracket matching, history, indentation behavior, active-line highlighting,
selection handling, and dashboard-styled search/tooltips. Browser coverage now
edits Markdown through CodeMirror content, verifies the Markdown language
selection and line-number gutter, checks internal scrolling, saves, and returns
to rendered Markdown view.

Verification passed:

- `npm run test:document-viewer`
- `npx tsc -p tsconfig.e2e-tests.json`
- `npm run build`
- `npm run test:browser` after one retry; the first run stopped before the
  CodeMirror step at the separately tracked
  `260525-bug-ws-dashboard-agent-tab-close-confirmation-sticky` path.
