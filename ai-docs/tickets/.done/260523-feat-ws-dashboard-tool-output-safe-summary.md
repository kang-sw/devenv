---
title: Show bounded Codex tool output snippets
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-feat-ws-dashboard-activity-console-tail-ribbon-polish: recent transcript density polish left tool output placeholders too generic
spec:
  - 260521-ws-dashboard-activity-console-read-model
  - 260522-ws-dashboard-activity-console-transcript-expansion
related-mental-model:
  - ws-web-dashboard
---

# Show bounded Codex tool output snippets

## Background

Dogfood of a live subquery agent showed Codex tool output transcript blocks
rendering only "Tool output captured". That text is not the original tool
output; it is the dashboard transcript normalizer's conservative placeholder
for Codex `function_call_output` and `custom_tool_call_output` records.

The current behavior is more conservative than the dashboard owner model needs.
Activity Console is an authenticated owner viewer, so the primary constraint is
bounded rendering rather than redacting normal command output from the owner.
The block should show useful output context without freezing the browser or
turning the transcript into an unbounded payload dump.

## Direction

- Render bounded head/tail output snippets for authenticated owners, starting
  with roughly the first 10 lines and last 10 lines when output is longer than
  the inline budget.
- Include a clear omission marker and bounded metadata such as line count or
  byte count when middle content is omitted.
- Keep internal storage paths, transcript file paths, session ids, and
  daemon-private implementation identifiers out of browser-visible text.
- Apply one generic bounded rendering policy first. Tool-type-specific polish
  can come later only when a concrete type needs different display treatment.

## Phases

### Phase 1: Render bounded tool output snippets

Replace content-free Codex tool-output placeholders with authenticated-owner
snippets that show the first 10 lines and last 10 lines when output exceeds the
inline budget. Include a clear omitted-middle marker with bounded line and/or
byte counts.

The first implementation should use one generic rendering policy across MCP,
shell, patch, and generic function outputs. Expanded detail may use the same
head/tail policy with a larger bounded cap; unlimited raw output remains out of
scope.

Verification should cover short output, long output with omitted-middle
metadata, empty output, non-text or malformed payload degradation, hidden
internal storage/session identifiers, and browser rendering that remains bounded
for large transcript blocks.

### Result (pending) - 2026-05-24

Implemented Phase 1 for native Codex transcript tool-output records:

- `function_call_output` and `custom_tool_call_output` now render bounded owner
  snippets instead of the placeholder `Tool output captured`.
- Long outputs are summarized as first 10 lines, an omitted-middle marker, and
  last 10 lines, with `outputBytes`, `lineCount`, and
  `omittedMiddleLines` metadata.
- Empty and non-string outputs degrade into bounded text, while the existing
  native transcript text redaction still prevents internal paths/session
  details from leaking into browser-visible blocks.

Verification:

- `cargo test -p ws-dashboard-daemon work_root_activity::tests::codex_session --manifest-path ws-dashboard/Cargo.toml`
- `cargo test -p ws-dashboard-daemon --test routes work_root_activity_transcript_route_reads_codex_native_session_backfill --manifest-path ws-dashboard/Cargo.toml`
