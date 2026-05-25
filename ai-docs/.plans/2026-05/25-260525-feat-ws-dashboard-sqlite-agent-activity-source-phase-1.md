# Plan: 260525-feat-ws-dashboard-sqlite-agent-activity-source Phase 1

## Survey Result

The current dashboard Activity projector discovers agents by scanning `<cache>/proj/<worktreeKey>/agents/*/agent.json`. Phase 1 must switch current role discovery to `<cache>/proj/<worktreeKey>/state.sqlite` `agent_defs`, while continuing to read payload files from `agents/<state_path>/`.

## Steps

1. Add read-only SQLite dependency and adapter code for `state.sqlite`.
2. Replace current-agent discovery with `agent_defs` query results.
3. Reuse existing projection/transcript readers by passing a resolved payload directory.
4. Update route test fixtures to seed SQLite rows without `agent.json`.
5. Verify missing/incompatible registry soft-degrade behavior.
6. Run filtered daemon Activity tests and formatting.

## Acceptance Checks

- `agent_defs` rows project into unchanged `agents` and current `items` shapes.
- Payload directories without `agent.json` still provide current-call, output, and native transcript availability.
- Transcript lookup resolves the current role through SQLite `state_path`.
- Missing or incompatible SQLite state does not fail the route.
- No frontend source changes.
