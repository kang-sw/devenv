---
title: "whiteboard 토픽 텍스트 너머로 dashboard 멘션 기능 일반화"
---

# Generalize dashboard mentions beyond whiteboard topic text

## Background

Owner request (2026-11-16 22:05 KST, verbatim): "그리고 커다란 UX 건 ... 에이전트의 코멘트, 파일 미리보기 등도 다 멘션의 일부가 되는 기능을 todo 티켓으로 잡아주세요. 초안은 파일 미리보기 멘션은 파일 경로에 라인 컬럼 등 + 컨텐츠를 멘션하기, 하여간 멘션 기능의 일반화."

Today a mention is text the user selected inside a whiteboard topic card and added to the message. The message carries it as `<wb:mention topic=".." id=".." entry="eN">selected text</wb:mention>` (format and escaping: `agents-plugin-tool/internal/wsinbox/mentions.go` header comment and `Mention`). The client builds it in `agents-plugin-tool/internal/wsdashboard/assets/dashboard-bundles.js`, and `internal/mcp/server.go` references the element. The element can only point at a board topic, an item ID, and a log/panel entry.

The goal is that anything the user reads in the dashboard can be mentioned the same way. Examples: an agent's chat message or comment, and a range in a file preview. The lead should receive a precise locator plus the selected content.

## Phases

### Phase 1: Mention sources beyond board topics

Not yet designed (owner: "todo 티켓으로", "초안은"). The owner's draft, recorded as direction and not as a settled decision:

- **File preview mention.** The file path plus a line/column range, plus the selected content.
- **Agent messages and comments.** These become mentionable too.
- **One general mechanism** rather than per-surface special cases ("멘션 기능의 일반화").

Questions to settle through the Open Decision Queue at `ready/` promotion. None is decided here:

- **Element shape.** Either extend `wb:mention` with new locator attributes, or add a new element kind per source (for example a file element or a chat element). The answer sets the compatibility tier: a lead running an older plugin parses `wb:mention` with a strict regex (`mentionHead` in `mentions.go`), so new attributes may not be tolerated. Every option must state its tier per AGENTS.md `### Compatibility Tiers`.
- **Locators per source.**
  - File: whether the path is repo-relative or worktree-relative, which revision or content hash it names, and whether lines and columns are 1-based.
  - Chat or comment: a session plus a stable entry ID.
  - Panels inside a sandboxed iframe: how a selection there is located.
- **Content policy.** Size caps and truncation for large selections. Whether a file mention carries content, or only the locator for content the lead can re-read.
- **Surfaces in scope** for the first phase, and the add-to-message interaction each one uses.
- **Delivery.** Delivery to non-whiteboard sessions (hosted chat, mailbox), and how a lead without whiteboard mode reads the element.
