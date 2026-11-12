# The Claude mod holds dashboard messages while the conversation compacts

## Goal

Ticket `261112-bug-ws-claude-mod-dashboard-message-during-compaction-submits-as-follow-up`
(idea; read it). User report 2026-11-12: in a Claude CLI session running the
ws mod, a dashboard message sent while the context compacts is always
submitted at once and lands as a settled follow-up. User decision (board
item 378 = a, 2026-11-12): hold such messages exactly as during a running
main-thread turn, and submit them once the compaction settles, after any
compaction continuation.

## Decisions

- While a main-conversation compaction runs, the mod's board drain treats
  the session as busy: dashboard messages stay held in the inbox (so the page
  keeps them editable / cancellable and promotable to steering, as during a
  turn) instead of being submitted through `$.prompt.submit`.
- When the compaction settles (landed, failed, or vetoed), held messages are
  delivered by the same path that delivers them at a turn end. A compaction
  continuation the mod itself queues (the compaction lever's `continue`) goes
  first; held messages follow it (as the turn-end leftovers of that
  continuation's turn, or submitted after it when no continuation runs).
- A message promoted to steering during the compaction is delivered at the
  next main-thread tool boundary as today; if none comes, it falls back like
  any carried steer.
- Mailbox mail behaviour during compaction is unchanged unless the same
  busy reading already covers it naturally; do not widen scope to mail.
- Compatibility tier T0 (mod behaviour only, no MCP / store shape change).

## Where

- `agents-plugin/hooks/claude-mod/watcher.ts` (`turnRunning` edge, the
  board drain at ~558, turn-end flush), `register.ts` (session.compact hook
  / compaction start and end, which `dashboard.ts` already tracks as
  `compactStarted` / `compactEnded`), and `compaction.ts` for the
  continuation ordering.
- Mirror byte-identically to `agents-plugin-wsflow/hooks/claude-mod/`
  (read `ai-docs/manuals/wsflow-mirroring.md` first).
- Update the header comments that say messages are held only while a
  main-thread turn runs.

## Verification

- Extend the mod's existing tests (watcher.test.ts / engine tests, run the
  way the existing claude-mod tests are run in this repo - find the runner
  from package or test files; they are TypeScript tests, not Go): a board
  wake during a compaction submits nothing; the compaction end delivers the
  held messages; with a continuation queued, the continuation goes first;
  a compaction that fails or is vetoed still releases the hold.
- No Go build/test locally. If any Go test is affected, use
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test -count=1 <pkgs>`.

## Constraints

- Preserve: the dashboard interrupt abort, the tool-boundary steer drain and
  its bound, dropped-submit fallback rows, hosted-session mode (mailbox wait
  off), the compaction lever and advisory turns.
- Commit on branch `delegate/mod-hold-during-compaction` in this worktree;
  do not merge or push. Update the ticket only through its commit's
  `## Ticket Updates` if relevant; leave its status to the lead.
