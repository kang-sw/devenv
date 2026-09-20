---
title: "ws MCP input-shape coercion traps: create_empty stem + git ai_context"
---

# ws MCP input-shape coercion traps

Parent epic: `260909-epic-ws-worker-interpreter-refoundation` (tooling
ergonomics for the worker/lead surface).

## Surprise

Two recurring caller mistakes during dogfooding, both with the same root
cause: an MCP tool demands an input shape that diverges from the shape the
same concept carries everywhere else in the workflow, and then *silently
coerces* a mismatched input instead of rejecting it clearly.

### Trap 1 — `tickets.create_empty` doubles the date prefix

- Contract: `stem` is a *dateless semantic* stem (`feat-foo-bar`);
  `TicketCreate` unconditionally prepends today's date
  (`fullStem := today + "-" + stem`, `wsdoc/ticket_create.go:59`).
- Failure mode: a caller passing `260920-feat-foo` (the "stem" as it appears
  in filenames, `git log --grep`, and every ticket reference) yields
  `260920-260920-feat-foo.md`. No guard; the wrong artifact is created
  silently.
- Why it recurs: "stem" is overloaded. AGENTS.md's own "Reference tickets by
  stem" example is `260429-research-host-neutral-ws-plugin` (date included),
  as are all on-disk names and grep references. The word means "dated full
  stem" everywhere except this one tool's input, where it means "dateless
  semantic stem".

### Trap 2 — `git.commit` / `git.merge` `ai_context` string vs. array

- Contract: `ai_context` is `array<string>`, one bullet per element
  (`server.go:3727,3743` declare it as such).
- Failure mode: a caller passing a single dash-bulleted prose string (the
  exact shape a commit message's `## AI Context` block takes everywhere) hits
  `stringList`/`stringListKeepBlank` (`server.go:4534,4556`), which return
  `nil` for any non-`[]any` value. The server then reports "requires nonempty
  ai_context" — it says *empty* when the real fault is *wrong type*.
- Why it recurs: the surface concept (`## AI Context` = dash-bulleted prose)
  and the tool's wire shape (JSON array of strings) disagree, and the error
  message points at the wrong axis (emptiness, not type).

## Proposed direction (not yet approved — API-semantics change)

- **Trap 1 (recommended, matches the user's instinct):** if `stem` begins
  with a `\d{6}-` prefix:
  - prefix `== today` → strip it (harmless dedup) and proceed;
  - prefix `!= today` → reject with an explicit message rather than silently
    re-dating (respects AGENTS.md "Creation-date prefixes are stable; never
    rename to change the date" — a differing embedded date is ambiguous
    between intentional backdating and a mistake, so it must not be silently
    stripped-and-re-prepended). A leading `\d{6}-` is never a valid semantic
    stem (categories are feat/bug/... never numeric), so detection is
    unambiguous.
- **Trap 2:** make `stringList`/`stringListKeepBlank` (or the commit/merge
  arg validation) distinguish "wrong type" from "empty": when `ai_context` is
  present but not an array, reject with "ai_context must be an array of
  strings (one bullet per element), got <type>" instead of coercing to `nil`
  and reporting emptiness. Optionally, leniently accept a single string by
  splitting on newline-leading `- ` bullets — but a clear rejection is the
  minimum and the safer default.

## Scope / notes

- Both fixes are host-neutral MCP tooling changes (Arch Rule 4 clean); they
  ship in `ws` proper.
- Consider auditing other `stringList`-fed array args for the same
  silent-nil-on-wrong-type trap (`updated_tickets`, `stems`, note/todo
  arrays) — the coercion helper is shared, so the fix at the helper (or a
  typed-reject variant) may cover several tools at once.
- Open question: helper-level typed rejection (one change, broad reach) vs.
  per-call validation (surgical, tool-specific messages). Decide at todo
  promotion.
