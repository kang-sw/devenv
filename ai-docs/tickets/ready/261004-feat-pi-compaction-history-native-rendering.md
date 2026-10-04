---
title: Native-style Markdown rendering for previous conversation history
related:
  261004-feat-pi-compaction-interactive-ux: completed display-only history implementation extended visually
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 60b2557b6b9a2863
sage-review-completeness-reviewed: 60b2557b6b9a2863
---

# Native-style Markdown rendering for previous conversation history

## Background

The user likes the restored previous-conversation block and requests Pi
Markdown rendering with background-colored user messages, resembling native
conversation. The user authorizes implementation in an isolated worktree and
merge to develop. The change is limited to presentation and its tests.

## Decisions

- Render each restored user/assistant text body with Pi Markdown. User messages
  use the native theme's userMessageText foreground and userMessageBg
  background; assistant messages have no user-message background. Reuse host
  Markdown/theme primitives rather than native message objects whose prompt
  navigation markers would add behavior beyond this visual change.
- Keep the explicit previous-conversation/display-only header and separator,
  chronological order and latest twenty combined user/assistant message limit.
  Persist original text unchanged and sanitize terminal controls at display time.
- Preserve plain custom-entry persistence, model-context exclusion and no extra
  model wake. Selection, branch isolation, deduplication and supported live/reload
  placement remain unchanged.

## Constraints

- Resolve UI primitives through the existing loadHostPiTui seam; the repository
  development TUI and installed host TUI can be different physical instances.
- Respect the active theme, including themes with transparent user backgrounds.
  User Markdown presentation should follow native user preservation options
  where supported, rather than rewriting original message text.
- Read ai-docs/manuals/shipped-surface-boundary.md before editing shipped UI text.
  Pi API work requires complete relevant installed Pi docs and Markdown
  cross-references, especially docs/extensions.md, docs/tui.md and docs/themes.md.
- No compaction scheduling/resume changes, host patch, new persisted schema,
  tool/thinking/image replay, theme configuration or shared ws/wsflow changes.

## Prior Art

- agents-plugin-pi/src/compaction-history.ts: registerCompactionHistory currently
  returns one Text component with role labels and literal message bodies.
- agents-plugin-pi/src/pi-tui.ts: loadHostPiTui runtime UI resolution seam.
- agents-plugin-pi/test/compaction-history.test.ts: real SessionManager fixtures
  and renderer assertion currently expecting literal Markdown.
- Installed Pi's native user-message component uses userMessageBg and
  userMessageText; entry renderers can return a Container of Components.

## Relations

| stem | declared as | status |
|---|---|---|
| 261004-feat-pi-compaction-interactive-ux | related | done |

## Prior Decisions

- 261004-feat-pi-compaction-interactive-ux (2026-10-04, commit 04c163c4): "Recent twenty combined user/assistant bodies are persisted as display-only custom entries excluded from model context, preserving retain-none. The disclosed live/reload ordering difference is accepted." — bearing: constrains
- 261004-bug-pi-identical-summary-compaction-event (2026-10-04, commit 04c163c4): "One Important lite review finding was fixed with an active-branch compaction boundary resolution and real SDK regression" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/compaction-history.ts, agents-plugin-pi/test/compaction-history.test.ts; agents-plugin-pi/src/pi-tui.ts only if the existing seam needs a primitive export |
| scope.surface | internal | adapter-local entry renderer and TUI seam; no extension tool, lifecycle or shared ws/wsflow interface changes |
| scope.new_public_symbol | no | existing registerCompactionHistory and loadHostPiTui suffice; no new public API required |
| scope.new_type_contract | no | existing version-1 CompactionHistory data remains unchanged; Markdown/Component composition uses existing host types |
| scope.test_surface | existing | agents-plugin-pi/test/compaction-history.test.ts contains real SessionManager fixtures and literal-renderer narrow/wide-width assertions |
| complexity.reuse_points | confirmed | loadHostPiTui in agents-plugin-pi/src/pi-tui.ts; installed Markdown default color/bgColor and preservation options; exported getMarkdownTheme and Container |
| complexity.side_effect_risk | low | change is restricted to render components and tests; selection and appendEntry observer remain untouched |
| risk.correctness | moderate | Markdown styling, padding, ANSI sanitization and narrow-width wrapping must preserve complete readable bodies |
| risk.fit | low | approved visual change uses the same Markdown color/bgColor and preservation options as native user rendering, without native navigation markers |
| risk.test | moderate | existing fixtures cover storage/context/no-wake and wrapping; new Markdown/background assertions needed, and live terminal/reload appearance remains unobserved |
| risk.security_or_contract | moderate | sanitize stored text before Markdown rendering and preserve plain-custom-entry context exclusion; never introduce user-message navigation control sequences |

Source/API evidence: `agents-plugin-pi/src/compaction-history.ts` selects text in branch order with `slice(-20)`, persists raw version-1 custom data, strips terminal sequences only in its Text renderer and has no sendMessage/continuation call. The installed host root is `/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/` (package version 1.0.0); its `dist/modes/interactive/components/user-message.js` uses Markdown with userMessageText/userMessageBg, preserveOrderedListMarkers and preserveBackslashEscapes, then adds OSC 133 navigation markers in render. Its `dist/core/extensions/types.d.ts#L1123-L1127` permits any Component from an entry renderer, and `#L1210-L1211` specifies custom-entry context exclusion. Installed nested pi-tui `dist/components/markdown.d.ts` and repository development `agents-plugin-pi/node_modules/@earendil-works/pi-tui/dist/components/markdown.d.ts` both expose the color/bgColor and preservation options. Installed `docs/extensions.md`, `docs/tui.md`, `docs/themes.md`, `docs/sessions.md`, `docs/session-format.md`, `docs/message-types.md`, `docs/compaction.md` and `examples/extensions/entry-renderer.ts` were read in full. These are source/API checks, not test execution or live terminal acceptance. AGENTS.md declares no Implementation Conventions row matching the named agents-plugin-pi paths.

## Phases

### Phase 1: Render display-only history like native conversation

Replace the literal aggregate renderer with host-compatible per-message
Markdown components and themed user background blocks, preserving the
historical boundary and unchanged display-only storage/selection semantics.
Keep scope within the history renderer, supporting host UI seam if required,
and tests. No exact native navigation/copy parity is requested.

Verification: exercise real Markdown rendering for headings/emphasis/code and
user versus assistant background behavior, padding/wrapping at narrow widths,
terminal-control sanitization and native-style user text preservation. Retain
history persistence, model exclusion, no-wake, latest-twenty, branch and
repeat-compaction coverage. Run targeted tests, the full Pi suite and diff
checks. Report live terminal appearance/reload as unverified unless actually
observed; do not compact or reload the lead session to manufacture acceptance.
