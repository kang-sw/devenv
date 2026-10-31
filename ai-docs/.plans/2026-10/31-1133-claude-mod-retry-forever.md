# Plan: claude-mod retry-forever (mailbox push loop crash, MCP watchdog cap)

## Intent

Owner preference (2026-10-31): mailbox push and MCP recovery must never
silently give up. A long backoff (up to about 10 minutes) is acceptable;
stopping for good is not.

## Scope

1. `agents-plugin/hooks/claude-mod/watcher.ts`: today a push loop that throws
   outside its guarded calls is caught by `bind()`'s
   `.catch((err) => edges.log("... loop crashed ..."))` and push stays dead
   until a rebind. Change: restart the loop for the same key and slug after
   the existing exponential backoff (`nextBackoff`, `BACKOFF_MIN_MS` ..
   `BACKOFF_MAX_MS`). Skip the restart when the loop's signal aborted or a
   stop or rebind superseded it (a crash in a superseded loop must never
   start a second loop beside the current one). Show one user notice per
   crash streak, not one per crash. Add tests in the watcher tests.
2. `agents-plugin/hooks/claude-mod/reconnect.ts`: the MCP watchdog stops
   attempting after `WATCHDOG_MAX_FAILURES` (3) consecutive failures until
   the server resolves again. Change: keep attempting once per interval
   (`claude.mcp_watchdog_interval_minutes`, default 10) indefinitely. Keep
   one transcript notice per failure streak. Update the file header comment
   and the tests. Remove `WATCHDOG_MAX_FAILURES` if nothing else uses it.
3. Mirror both files and their tests byte-identical to
   `agents-plugin-wsflow/hooks/claude-mod/`.

## Out of scope

- The session.start `python3 --version` probe (owner: leave it as is).
- The backoff cap (stays 60 s).
- The Pi adapter.
- Config manifest defaults.

## Verification

`claude plugin test` and `claude plugin validate` in both `agents-plugin` and
`agents-plugin-wsflow`. Confirm the mirror with `diff -r`.

## Boundary

Stop and report if the restart cannot be made safe against duplicate loops,
or if the watchdog change needs a config or manifest change.
