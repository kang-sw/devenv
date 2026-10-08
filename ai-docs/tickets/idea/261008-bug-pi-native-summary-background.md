---
title: Preserve native tool backgrounds in display summaries
related:
  261007-feat-pi-display-summary: original display-summary feature
---

# Preserve native tool backgrounds in display summaries

## Background

During dogfooding, the user observed that a native Pi `edit` call has its
normal background before summarization but becomes transparent when replaced
by the summary renderer. The user requested a bounded delegated repair.

## Constraints

- Preserve the native tool frame and status background when showing summaries.
- Preserve the existing summary spacing, intention indentation, result color,
  optional context, and expanded/raw rendering behavior.
- This capture records the observed defect; it does not authorize broader host
  UI or persistence changes.

## Phases

### Phase 1: Preserve native backgrounds during summary replacement

Repair the native-tool summary wrapper and verify that summarized self-framed
tools retain their status backgrounds without changing default-shell tools or
expanded native renderers. Include regression coverage for the reported edit
case and the existing summary layout.
