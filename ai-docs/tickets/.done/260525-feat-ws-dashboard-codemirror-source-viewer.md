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
completed: 2026-05-25
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

### Result (8feb467) - 2026-05-25

Non-Markdown document view mode now renders through the shared CodeMirror
source surface in read-only mode instead of the legacy preformatted text
surface. Markdown view mode remains the dedicated Markdown renderer, while edit
mode continues to use raw CodeMirror editing.

`DocumentRawEditor` now supports editable and read-only configurations. The
read-only configuration keeps keyboard focus and text selection available but
does not publish draft changes or dirty the pane. The visible focused
CodeMirror outline was removed for both read-only and editable surfaces so
focus no longer draws the bright blue editor border.

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

### Result (8feb467, 0078e7f) - 2026-05-25

Expanded language detection and lazy highlighting for TOML, XML, SQL,
diff/patch, INI/properties/env, Dockerfile, Go, Java, C/C++, PHP, Ruby, and
Lua. Workflow-specific candidates such as Makefile, Nix, Typst, Mermaid,
Justfile, and gitignore now receive stable language ids where useful, with
plain CodeMirror text fallback when no safe highlighter is wired.

Browser coverage now verifies TOML read-only CodeMirror viewing, quiet focus
chrome, source viewer line numbers, read-only source scrolling, CodeMirror edit
save, fallback text viewing, and Markdown view mode staying on the dedicated
Markdown renderer. The follow-up test commit accounts for CodeMirror's
virtualized DOM by scrolling before asserting the last long-file line.

Verification passed:

- `npm run test:document-viewer`
- `npx tsc -p tsconfig.e2e-tests.json`
- `npm run build`
- `npm run test:browser`

Two earlier browser attempts stopped at the separately tracked
`260525-bug-ws-dashboard-agent-tab-close-confirmation-sticky` path before the
source viewer step; a later run reached the source viewer and exposed the
virtualized-line assertion, and the final browser gate passed after the test
fix.
