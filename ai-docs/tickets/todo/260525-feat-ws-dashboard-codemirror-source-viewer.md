---
title: Use CodeMirror for source document viewing
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260525-feat-ws-dashboard-codemirror-edit-mode: initial editable CodeMirror substrate
  260525-feat-ws-dashboard-markdown-renderer-polish: Markdown keeps a dedicated rendered view mode
spec:
  - 260516-ws-dashboard-readonly-text-pane
  - 260524-ws-dashboard-document-edit-save-fanout
related-mental-model:
  - ws-web-dashboard
---

# Use CodeMirror for source document viewing

## Background

CodeMirror is now the document edit-mode substrate, but files without a custom
viewer still render read mode through the older raw preformatted text surface.
That means non-Markdown source files only benefit from CodeMirror after the
owner explicitly enters edit mode, even when they only wanted to inspect a file
with syntax highlighting, line numbers, search, and stable source scrolling.

The desired model is to keep the system-level `view | edit` split, while making
CodeMirror the source document surface for files that do not have a richer
custom read-only viewer. Markdown remains special: view mode uses the dedicated
Markdown renderer, and edit mode uses raw Markdown CodeMirror editing.

## Decisions

- Keep `view | edit` mode as the document pane lifecycle model. Do not collapse
  source files to edit-only, because future custom viewers such as Excalidraw,
  draw.io, HTML, or other format-aware panels need the same read/edit boundary.
- Use CodeMirror in read-only mode for source-like text files without a custom
  viewer. CodeMirror should be configured as non-editable and should not mark
  drafts dirty from view mode.
- Use the same language detection and theme foundation for read-only and
  editable CodeMirror surfaces.
- Remove the focused CodeMirror panel outline/border chrome. Focus should keep
  keyboard behavior correct without drawing the current bright blue editor
  border, because it visually competes with the document pane chrome.
- Broaden bundled syntax highlighting beyond the first implementation's core
  set, but keep loading lazy and conservative so the dashboard does not pay for
  every language on initial load.
- Keep LSP, completion, diagnostics, formatting, go-to-definition, and IDE
  code intelligence out of scope.

## Phases

### Phase 1: Add read-only CodeMirror source view

Render non-custom text documents through CodeMirror read-only view mode instead
of the legacy `<pre>` raw text surface. Markdown view mode must continue to use
the dedicated Markdown renderer, while Markdown edit mode continues to use raw
Markdown CodeMirror editing.

The read-only CodeMirror surface must preserve pane identity, mode switching,
save/revert behavior, stale/conflict handling, and same-source fan-out. It must
scroll inside the document pane, expose line numbers and search, and never
trigger dirty draft state while in view mode.

As part of this phase, remove the bright focused CodeMirror outline/border
effect from both read-only and editable CodeMirror surfaces.

### Phase 2: Expand source language coverage

Extend the language mapping and lazy-loaded highlight support for common source
and workflow files beyond the initial set. Prioritize direct CodeMirror 6
language packages where available, then use `@codemirror/legacy-modes` through
`StreamLanguage.define(...)` for well-known modes.

Initial target set:

- TOML.
- XML.
- SQL.
- Diff and patch files.
- INI, properties, and dotenv-style config.
- Dockerfile.
- Makefile.
- Go.
- Java.
- C and C++.
- PHP.
- Ruby.
- Lua.

Workflow-specific candidates may be added when they are low-risk and available:
Nix, Typst, Mermaid, Justfile, gitignore, and env files. Unknown extensions must
continue to fall back to plain-text CodeMirror.

Verification should cover read-only CodeMirror rendering for at least one
non-Markdown file, Markdown still using the dedicated viewer in view mode,
editable CodeMirror still saving correctly, focus chrome staying visually
quiet, TOML highlighting selection, fallback plain text, and browser evidence
that read-only and editable source surfaces scroll inside the pane.
