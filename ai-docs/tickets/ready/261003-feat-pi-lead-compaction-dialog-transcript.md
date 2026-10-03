---
title: Carry the raw lead dialog across Pi lead compaction instead of tool noise
related:
  261002-feat-pi-lead-ws-owned-compaction: amended; its user-message section and kept-tail pass-through are replaced
  261002-feat-ws-config-adapter-schema-extension: adjacent; the budget keys it declared for Pi are renamed in the Pi manifest
sage-review-design: skipped
sage-review-completeness: completed
sage-review-completeness-reviewed: 7ad632e428210a1c
---

# Carry the raw lead dialog across Pi lead compaction instead of tool noise

## Background

261002-feat-pi-lead-ws-owned-compaction replaced Pi's native summarizer for
the lead with a ws-authored summary. Dogfooding shows it working, but what
crosses the compaction boundary is weighted the wrong way:

- **The summary carries only the human side of the discussion.**
  `## User messages` holds human-typed user messages only
  (`selectHumanMessages`, `agents-plugin-pi/src/lead-compaction.ts`). It is
  budgeted at 8000 tokens with a 1500-token per-message cap that keeps only
  the head of a long message. The lead's own replies are not carried.
- **The raw kept tail carries tool noise.** `buildLeadCompactionResult`
  (`agents-plugin-pi/src/goal-loop.ts`) passes Pi's
  `preparation.firstKeptEntryId` through unchanged. Pi then keeps roughly
  `keepRecentTokens` (default 20000) of raw recent entries after the summary,
  tool calls and full tool results included.

The user wants the opposite weighting. The discussion between the user and
the lead should cross as close to raw as a budget allows, and tool output
should not cross at all. Tool output stays recoverable because Pi's session
JSONL is append-only and still holds every pre-compaction entry.

## Decisions

- **One dialog transcript section replaces `## User messages`.** It holds
  human-typed user messages, the lead's assistant text, and one line per tool
  call, in chronological order.
  - Human text is classified exactly as today (`humanTextOf`).
  - Assistant thinking parts are excluded.
  - Custom messages (push batches, mailbox, preparation messages) stay
    excluded, as today.
  - Rejected: user messages only (as in 261002), because the lead's side of
    a discussion is half of what the next session needs.
- **The budget is in bytes: 40 KiB by default.** UTF-8 bytes approximate
  tokens across languages (about 4 bytes per token in English, and in Korean
  3 bytes per character at lower token efficiency), so one byte budget lands
  near 10k tokens either way.
  - The two Pi keys `pi.compaction_user_messages_budget_tokens` and
    `pi.compaction_user_message_cap_tokens` are replaced by one key,
    `pi.compaction_dialog_budget_bytes`: integer, minimum 1, default 40960.
    The old keys never shipped in a release (they landed after v0.46.26), so
    no migration or alias is needed.
  - Rejected: a character budget, which undercounts Korean tokens about
    threefold.
  - Rejected: keeping a token budget behind the `CHARS_PER_TOKEN` heuristic,
    for the same reason.
- **Selection is newest-first within the byte budget.** The section is
  always a contiguous newest run, as today. Its header states how many older
  items were omitted and that the prose carries them.
  - The budget counts every byte the section renders for an item: its label
    line, its text after elision, markers, and fold lines. The section header
    is not counted.
  - An item that does not fit whole is dropped whole, and selection stops
    there, as today.
  - Tool-run folding happens before selection, so a fold line is one item.
- **Rendering.** The section heading is `## Dialog`, and items render as:

  ```text
  --- user (<timestamp>) ---
  <text>
  --- assistant (<timestamp>) ---
  <text>
  → <tool name> <arguments JSON>
  → <tool name> <arguments JSON> ✗failed
  → (+23 more: Bash×15, Read×8)
  ```

  The section header reads, for example, "The newest 42 of 57 dialog items
  (older ones are carried by the prose below)."; when nothing is omitted, it
  reads "All 57 dialog items of this session.". Fold-line counts are ordered
  by count, highest first.
- **A long message keeps its head and tail.** A user or assistant message
  over 2560 bytes keeps its first 1024 bytes and its last 1024 bytes, with
  `[... N bytes skipped ...]` between them, where N is the elided byte count.
  - Context in the middle of a long paste is accepted as lost.
  - These sizes are fixed constants, not settings.
  - Rejected: head-only truncation (today's behavior), because a paste can
    sit before or after the user's own words.
- **A tool call is one line: tool name plus its arguments.**
  - The arguments are rendered as JSON and elided the same way when they
    exceed 300 bytes: the first 150 and last 150 bytes, with the marker
    between them.
  - Tool results are never carried; a failed call is marked `✗failed`.
  - The `ws-compact` lever call itself is excluded, because its arguments are
    the prose the summary already holds.
  - Rejected: dropping tool calls entirely, because which tools ran, and
    which failed, is cheap context the next session uses.
- **At most 8 tool lines between two dialog messages.**
  - A run of more consecutive tool calls keeps its last 8 lines. The earlier
    calls fold into one line counting them by tool name, for example
    `→ (+23 more: Bash×15, Read×8)`.
  - Tool lines count against the same byte budget as dialog messages.
- **No raw kept tail.** Every compaction this adapter authors, lever-
  initiated or fallback, sets `firstKeptEntryId` so that no raw entry from
  before the compaction is kept after the summary. Rejected: keeping Pi's
  kept tail, which is where the tool noise crosses today.
- **Tool output is reachable by search.** The summary names the session file
  (`ctx.sessionManager.getSessionFile()`) and says that full tool output and
  every earlier message remain there to search, for example with grep. A
  dedicated search tool is out of scope.

## Constraints

- Byte slicing must not split a UTF-8 code point: cut at a character
  boundary at or inside the byte limit.
- The deterministic sections other than `## User messages` (session, child
  agents, lead prose, resume instruction) are unchanged.
- Text that describes the old section must be updated to the new contract:
  - the `ws-compact` tool description, which says the adapter adds
    "human-typed user messages" itself;
  - `agents-plugin-pi/lead-compact-guide.md`, if it names the section or its
    budget;
  - `config-manifest.json` key descriptions.
- Spawned worker, explore, and fork sessions keep Pi's native compaction and
  its kept tail, as in 261002.
- Out of scope: microcompaction (rewriting old tool results before each model
  call through Pi's `context` event). It may become its own ticket.
- Matching manuals from AGENTS.md `### Implementation Conventions`: none
  (`agents-plugin-pi/` has no declared row).

## Prior Decisions

- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, Decisions): "Kept raw tail. The compaction result uses `preparation.firstKeptEntryId` unchanged; Pi computes valid cut points and its `compaction.keepRecentTokens` setting tunes the size." — bearing: contradiction-candidate
- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, Decisions): "The human-typed user-message section keeps messages newest-first within about 8k tokens, and caps any single message at about 1.5k tokens with a truncation marker" — bearing: contradiction-candidate
- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, Decisions): "No system- or developer-role injection. ws messages stay user-role." — bearing: constrains
- 261002-feat-pi-lead-ws-owned-compaction (94cdf8bd, commit): "Human messages are selected newest-first under the budget and displayed chronologically; "Goal armed:" stays as human text because it carries the user's goal." — bearing: constrains
- 261002-feat-pi-lead-ws-owned-compaction (94cdf8bd, commit): "The prose is also passed as customInstructions so a degraded native compaction (summary-build failure) is still steered by it." — bearing: constrains
- 261002-feat-ws-config-adapter-schema-extension (2026-10-02, Result): "config-manifest.json declares nine pi.* knobs: ... compaction_hard_percent, compaction_user_messages_budget_tokens, compaction_user_message_cap_tokens." — bearing: constrains
- 261002-feat-ws-config-adapter-schema-extension (1949b0e5, commit): "A test pins every manifest default to resolve identically to an absent knob, so the manifest and code cannot drift" — bearing: constrains
- 505cd037 (2026-10-03, commit): "Fixed elision constants (1 KiB head and 1 KiB tail over 2560 bytes; tool args 150+150 over 300 bytes) per the user's call that they need not be settings" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/lead-compaction.ts, agents-plugin-pi/src/goal-loop.ts, agents-plugin-pi/config-manifest.json, agents-plugin-pi/lead-compact-guide.md |
| scope.surface | cross-module | config key rename in manifest, goal-loop config resolution and ws-compact tool description change the adapter's caller-visible contract |
| scope.new_public_symbol | yes | pi.compaction_dialog_budget_bytes config key (exported helper names in lead-compaction.ts not fixed by the ticket) |
| scope.new_type_contract | yes | UserMessageBudgets in goal-loop.ts#L138-L141 replaced by a single byte-budget field; buildLeadCompactionResult input changes |
| scope.test_surface | existing | agents-plugin-pi/test/lead-compaction.test.ts, test/goal-loop.test.ts |
| complexity.reuse_points | confirmed | humanTextOf and messageText in lead-compaction.ts, existing selection/summary builder and getSessionFile use in src/ask.ts |
| complexity.side_effect_risk | moderate | firstKeptEntryId change alters what Pi reloads after compaction; whether Pi can express an empty kept tail is unverified |
| risk.correctness | high | UTF-8 boundary slicing, newest-first byte budgeting, tool-run folding, and empty kept tail against Pi session-manager reload |
| risk.fit | moderate | amends the done 261002 contract; consistent with its pass-through hooks but reverses two of its recorded decisions explicitly |
| risk.test | moderate | many new pinned behaviors but a test file and node --test harness already exist |
| risk.security_or_contract | moderate | config key rename and tool description contract; lead dialog text and tool arguments now cross into the summary |

## Phases

### Phase 1: Dialog transcript section and no kept tail

- Replace `collectHumanMessages`/`selectHumanMessages` and the
  `## User messages` section with the dialog transcript described in
  Decisions.
- Replace the two budget keys with `pi.compaction_dialog_budget_bytes`
  everywhere they are declared, resolved, and tested.
- Change `buildLeadCompactionResult` so no raw entry is kept. Pick the
  `firstKeptEntryId` value from Pi's compaction and session-manager code.
  Verify that it keeps nothing and that the session reloads cleanly
  afterwards. If Pi cannot express an empty kept tail, stop and report rather
  than keeping tool results.
- Add the session-file search line to the summary.

Done when:

- `npm test` in `agents-plugin-pi` passes.
- Tests pin:
  - the chronological interleave of user, assistant, and tool lines;
  - newest-first byte-budget selection and its omitted-count header;
  - head/tail elision of a long message and of long tool arguments,
    including a multi-byte boundary case;
  - the 8-line tool-run fold;
  - exclusion of thinking parts, tool results, adapter traffic, and the
    `ws-compact` call;
  - `✗failed` marking;
  - a compaction result whose kept tail holds no raw pre-compaction entry;
  - a reload check: after appending that compaction through Pi's own
    session manager, the rebuilt context (Pi's context-building path, such
    as `buildContextEntries`) holds the summary and no raw entry from before
    it.
