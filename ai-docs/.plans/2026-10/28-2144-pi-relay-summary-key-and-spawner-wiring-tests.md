# Pi: relayed batch summary key collision and spawner relay wiring tests

## Outcome

Two Important findings from the release-gate sweep
`50e88c50a..afe75c38f`, now on develop after the batch row redesign merge
(`d01b13b31`):

1. **B1: relayed summary keys collide after a relaunch.**
   `batchSummaryKey(agentId, seq)` (`agents-plugin-pi/src/batch-summary.ts:77`)
   keys a summary `<agentId>:<seq>`, and the widget looks summaries up the
   same way (`agents-plugin-pi/src/agent-widget.ts`, `summaryFor` near line
   1284). When an intermediate agent is relaunched (dormant resume), its
   revived descendant's batch buffer restarts `seq` at 1, so the root shows
   the old summary for the new batch 1. Make the key unique across launches:
   carry a launch identity the buffer already has or can cheaply get (for
   example a per-buffer or per-launch epoch) into `BatchView` and the key, so
   a relaunched agent's batches never reuse a prior launch's key. Keep the
   agent-view relay carrying whatever the key needs. Pin with a test: a
   summary recorded for launch 1's batch 1 is not shown for launch 2's
   batch 1.
2. **B2: the spawner's relay wiring is untested.** Add spawner tests (in the
   existing spawner test file's style) that pin:
   - `clearLiveState` (`agents-plugin-pi/src/spawner.ts:2237`) closes the open
     batch and drops `record.descendantViews` on stop, exit, and a failed
     launch;
   - `observeAgentView` attachment in `spawnAgent` and in `sendToAgent`
     (`spawner.ts` around 3743 and 4022): an `agent-view` message arriving on
     the channel lands on `record.descendantViews`.
   Each test must fail when the wiring line it pins is removed; check that by
   reverting the line locally once.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing.
- Minimal change; do not refactor surrounding code. Follow the test file
  idioms already in `agents-plugin-pi/test/`.
- Verification: `npm test` in `agents-plugin-pi/` for the touched test files
  plus the full suite; report the failure count against the known base (the
  full suite has pre-existing environment failures; none may be new).

## Boundary

- Work on `develop` in the root checkout `/Users/kang-sw/devenv`. Another
  delegate is editing `agents-plugin/hooks/claude-mod/` and
  `agents-plugin-wsflow/hooks/claude-mod/` there concurrently: never stage,
  revert, or stash those files, and never run `git add -A`, `git stash`, or
  `git checkout -- .`. Stage only your own `agents-plugin-pi/` files and this
  plan file. Leave untracked `latest` and `debug.log.1` alone.
- Out of scope: the 13 Minor follow-ups, any Claude mod change, version bumps.
  Return to the lead if B1 needs an agent-view protocol shape change beyond
  adding one field.
