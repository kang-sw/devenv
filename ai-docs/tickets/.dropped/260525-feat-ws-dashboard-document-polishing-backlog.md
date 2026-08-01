---
title: ws dashboard document polishing backlog
parent: 260514-epic-ws-web-dashboard-mvp
spec:
  - 260524-ws-dashboard-document-viewer-mode
  - 260524-ws-dashboard-document-translation-overlay
  - 260524-ws-dashboard-document-edit-save-fanout
---

# ws dashboard document polishing backlog

## Background

The dashboard document MVP is complete enough for the current milestone:
Markdown rendering, read/edit mode separation, CodeMirror-backed source viewing,
raw edit/save fan-out, translation-ready block overlays, pathref copying, and
basic Markdown selection actions are implemented.

This ticket keeps remaining document quality work out of the MVP critical path.
Future sessions should pull concrete discomforts from dogfood use into this
ticket or split them into smaller implementation children when the boundary is
clear.

## Phases

### Phase 1: Markdown reader quality pass

Improve the dedicated Markdown viewer only where dogfood shows visible friction.
Candidate areas include selection-rail ergonomics, block hover actions, copy
output fidelity for additional Markdown constructs, code block visual tuning,
callout variants, table/task-list edge cases, and footnote/tooltip behavior.

Do not expand raw HTML rendering, generalized document plugins, or translation
provider behavior in this phase unless a later ticket promotes that scope.

Verification should include focused document viewer tests and at least one
browser gate that exercises Markdown selection/copy behavior.

### Phase 2: Source editor polish

Tune the CodeMirror read/edit substrate as on-demand issues appear. Candidate
areas include language mapping gaps, keyboard affordances, search discoverability,
read-only source viewer chrome, scroll behavior, and save/revert feedback.

This phase should preserve the current product decision that CodeMirror is the
dashboard source substrate while LSP/completion and full IDE parity remain
outside MVP scope.

Verification should include the document editor test suite, frontend build, and
browser coverage for any user-visible editor behavior changed.
