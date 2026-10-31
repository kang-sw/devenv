# Plan: claude-mod drained mail never vanishes on a post-drain crash

## Intent

Owner request (2026-10-31): close the gap left by the retry-forever merge
(fe9377835). In direct drain mode, `recv` removes mail from the server queue
before the mod submits it. Every *expected* failure after the drain already
routes the batch to `keep()` (transcript record + debug log), but an
*unexpected* throw between the drain and the hand-off to `steer`/`deliver`
(for example inside `batchPrompt`, `steer`, `deliver`, or the `lost`/`keep`
callbacks) escapes `loop()`, the supervisor restarts the loop, and the drained
mail is gone without a trace.

## Scope

1. `agents-plugin/hooks/claude-mod/watcher.ts`, in `loop()`: guard the section
   from a successful non-empty drain through the delivery hand-off so that any
   unexpected throw first records the drained mail through `keep()` and then
   rethrows, keeping the existing crash path (supervisor restart, backoff,
   one notice per crash streak) unchanged.
   - When `batchPrompt` itself threw, `keep()` gets a plain fallback rendering
     of the raw drained mail (for example `JSON.stringify` of each item), so the
     content survives even when formatting is what failed.
   - Mail already handed to `steer` or `deliver` successfully must not also be
     recorded (no duplicate rows on the normal path).
   - A throwing `edges.record` inside `keep()` must not drop the mail: the
     debug-log line (`edges.log`) still carries the full text, and the failure
     is logged.
2. Tests in `watcher.test.ts`: a throw in the delivery path after a
   non-empty drain records the mail (record and log carry its text), the loop
   restarts as before, and the normal success path records nothing.
3. Mirror both files byte-identical to `agents-plugin-wsflow/hooks/claude-mod/`.

## Decision

Record-then-rethrow over buffer-and-redeliver: a crash in the delivery path is
likely deterministic for that batch, so redelivering the same mail after the
restart would crash again in a loop; `keep()` is the existing, documented
fallback for drained mail that cannot be submitted.

## Out of scope

- Process or session death between `recv` and submission (would need a
  server-side peek/ack protocol in ws-mcp).
- Notice mode, the wait/peek path, backoff values, Pi.

## Verification

`claude plugin test` and `claude plugin validate <path>` in both
`agents-plugin` and `agents-plugin-wsflow`; `diff -r` of the two
`hooks/claude-mod` trees.

## Boundary

Stop and report if the guard cannot avoid duplicate delivery-plus-record, or if
the fix needs a change outside watcher.ts and its tests (other than the mirror).
