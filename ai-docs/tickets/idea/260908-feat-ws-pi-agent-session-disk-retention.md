---
title: "Pi adapter: bound subagent session files on disk (evict to disk, prune by age, drop empty spawn dirs)"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260905-feat-ws-pi-agent-alias-park-and-registry-cap: owns the in-memory registry cap and `evictForCapacity`
  260908-feat-ws-pi-subagent-audit-window-and-owner-steering: reads `record.sessionPath` for history; a pruned record must read as gone, not as a parse error
---

# Pi adapter: bound subagent session files on disk (evict to disk, prune by age, drop empty spawn dirs)

## Background

Owner ask (2026-09-08): a subagent that has not been referenced for a long
time, or that falls off the registry cap, should be deleted from disk as
well, not only forgotten in memory.

What the adapter does today (`agents-plugin-pi/src/spawner.ts`,
`agent-sidecar.ts`, verified on the 2026-09-08 tree):

- **The registry is an in-memory `Map`.** The cap (`WS_PI_AGENT_REGISTRY_CAP`,
  default 256) bounds only that map, and only lazily: `evictForCapacity`
  runs at spawn time when `size >= cap`, removes the dormant
  (`!client && !running && !threadBound`) record with the oldest
  `max(lastLeadPromptAt, last reportLog entry, lastReportAtOverride)`, and
  by design "only forgets the registry entry (never touches the evicted
  agent's on-disk session file)". The only other removal is the explore
  leaf's self-reap after its first harvest. Stop and automatic park keep
  the record.
- **Dormant records do not outlive the lead session.** The shutdown sidecar
  (`<lead session>.ws-agents.json`) persists every non-`threadBound`
  record and is deleted on the next `session_start` of the same lead
  session file. A fresh lead session starts with an empty registry, so the
  cap never sees the previous sessions' children at all; "256" is not a
  disk bound in any sense.
- **Every spawn creates `mkdtempSync(join(tmpdir(), "ws-pi-agent-"))`
  unconditionally** (`spawnAgent`, before the fork/noSession decision) and
  sets `sessionPath = <dir>/session.jsonl`. Nothing ever removes that
  directory. Persistent workers write their session there. Explore leaves
  run `--no-session`, and forks are launched with `--fork` so
  `record.sessionPath` is reassigned to the child's real file
  (`record.sessionPath = state.sessionFile`, which Pi places in its own
  `~/.pi/agent/sessions/<cwd>/` next to the lead's sessions); for both, the
  mkdtemp directory stays behind empty (it is still used as
  `WS_PI_APPROVAL_DIR`).
- **Observed on the owner machine (2026-09-08):**

  | item | value |
  |---|---|
  | `/tmp/ws-pi-agent-*` directories | 542 |
  | of which without any `session.jsonl` | 493 |
  | total size | 45M |
  | oldest present | 2026-09-07 (earlier ones removed by OS tmp policy, not by the adapter) |
  | `/tmp/ws-pi-ask-test-*` left by the test suite | 1224 dirs, 9.4M |

  Fork session files (`--fork` copies) sit in `~/.pi/agent/sessions/<cwd>/`
  interleaved with lead session files, indistinguishable by name.

So the current bound is whatever the OS applies to `tmpdir()`, which is
accidental (WSL `/tmp` survives restarts of the shell; a long-lived host
keeps everything).

## Direction

Owner question (2026-09-08, on reading the numbers above): is `tmpdir()`
a sensible home for these files at all? Assessment: no. The sidecar
revival and dormant auto-resume both relaunch from `record.sessionPath`,
so the adapter treats these files as durable, while `tmpdir()` is the one
place the OS is entitled to delete under it; a revived orphan whose file
was reaped fails for a reason the lead cannot see. It also splits one
concept across two locations (workers in `/tmp`, forks in Pi's own session
directory), and it deviated silently from a recorded decision. The research
anchor (`260802-research-ws-pi-native-framework`, "ws subagent sessions
placed at `~/.pi/agent/ws-sessions/<agentId>.jsonl` (sibling of
`sessions/`)") and the MVP Phase 2 plan text (`260902-feat-ws-pi-native-mvp`,
"`--session <ws-owned-path>` (sibling of `~/.pi/agent/sessions/`, hidden
from the `/resume` picker)") both named a durable sibling directory. The
survey plan for that phase (`acc421c7`) restated the requirement as
"ws-owned `sessionPath` (fresh temp path outside `~/.pi/agent/sessions/`)",
keeping the purpose clause (hidden from `/resume`) and dropping the
location; the implementing commit `13b4e67f` followed the plan exactly,
so plan-versus-diff review saw no deviation and the ticket's Result
"Deviations" list has none. Every later ticket (RPC resume, approval dir, sidecar, park/cap, fork readiness)
reasoned about the file and never about its directory. Postmortem
(2026-09-08, owner-requested) recorded in the commit that lands this
paragraph.

Pi already provides what a proper home needs: the lead-side
`ctx.sessionManager.getSessionDir()` (the `~/.pi/agent/sessions/<cwd>/`
directory) and the child-side `--session-dir <dir>` CLI flag, which
`SessionManager.create`/`open` honor for `--session` and `--fork` alike.

Four pieces, relocation first because it makes the retention pieces
almost trivial:

0. **Relocate to a ws-owned, lead-keyed tree.** Put every child under
   `<lead session dir>/ws-agents/<lead session id>/<agent id>/` (worker
   `session.jsonl`, approvals, and, via `--session-dir`, the fork copy),
   so one lead session owns one subtree. Retention then reads as "the
   lead session file is gone, or its subtree is older than the TTL, so
   the subtree goes"; no per-file bookkeeping and no index for forks.
   Sidecar-revival and dormant-resume paths keep reading
   `record.sessionPath` unchanged. Decide whether the lead-session-keyed
   layout should live beside the lead file (`getSessionDir()`) or under a
   sibling root such as `~/.pi/agent/ws-agents/`; the former keeps Pi's
   `/tmp`-free invariant and makes `rm` of a lead session's material
   obvious, the latter keeps Pi's own directory listing clean.
1. **Drop the spawn directory when it is not the session home.** When a
   spawn ends up `--no-session` or `--fork`, remove the mkdtemp directory
   at the point the record stops needing it as approval dir (explore
   self-reap; fork after readiness, where `rmSync(dirname(launch.contextPath))`
   already runs for the fork transport). This alone removes ~90% of the
   directories above.
2. **Cap eviction deletes what it evicts.** `evictForCapacity` currently
   returns a label; make the spawn path remove the evicted record's on-disk
   home after the registry delete succeeds: the whole `ws-pi-agent-*`
   directory for a worker, the single `sessionPath` file for a fork (never
   the lead's file, never a directory under `~/.pi/agent/sessions/` by
   scan). Same rule for the shutdown-sidecar path: an orphan the next
   `session_start` decides not to revive should be deleted, not silently
   dropped.
3. **Age-based prune across lead sessions.** Because the registry is
   per-lead-session, "not referenced for a long time" has to be decided
   from disk. Safe scope: `tmpdir()/ws-pi-agent-*` is ws-owned by prefix
   and can be swept by mtime (`session.jsonl` mtime, or directory mtime
   when empty) on `session_start`, behind a `WS_PI_AGENT_SESSION_TTL`
   style knob with a conservative default (days, not hours). Fork copies
   in Pi's directory cannot be swept safely without an index; either keep
   an append-only ws-owned index of fork session paths
   (`~/.pi/agent/ws-agents-index.jsonl` or similar) or leave forks to piece
   2 only and say so. With piece 0 landed, fork copies live in the same
   subtree and this special case disappears.

Out of scope here but worth a sibling hygiene ticket: the test suite's
`/tmp/ws-pi-ask-test-*` leak (1224 directories), which is a test-fixture
cleanup problem, not a runtime one.

## Constraints

- Never delete a file the adapter did not itself record as a child's
  session home; in particular never enumerate `~/.pi/agent/sessions/`.
- A record that is `running`, `threadBound`, or owner-held (the
  `lastWriter === "owner"` state introduced by
  `260908-feat-ws-pi-subagent-audit-window-and-owner-steering`) is never a
  prune candidate, on disk or in memory.
- Deletion is best effort and must not fail a spawn or a `session_start`;
  log through the existing notify path when a delete fails.
- The `/audit` history reader (child B of the conversation-view epic) must
  treat a missing `sessionPath` as "history gone", not as an error.

## Phases

### Phase 1: Relocate child sessions under the lead session

Piece 0. Record the chosen root here. Tests: worker, fork, and explore
spawns all resolve under the lead-keyed subtree; `--session-dir` is passed
for fork launches and the recorded `sessionPath` is inside the subtree;
sidecar revival relaunches from the new location. Live check (owner-run):
`/tmp` gains no `ws-pi-agent-*` directory across a worker, a fork, and an
explore.

### Phase 2: Drop empty spawn directories and delete on cap eviction

Pieces 1 and 2 above. Tests: explore self-reap and fork readiness leave no
`ws-pi-agent-*` directory behind; an evicted worker's directory is gone and
an evicted fork's `sessionPath` file is gone while the lead session file
is untouched; a delete failure is swallowed and the spawn still succeeds.

### Phase 3: Age-based prune on session start

Piece 3 over the lead-keyed subtree: a subtree whose lead session file no
longer exists, or whose newest child activity is older than the TTL, is
removed. Live check (owner-run): after a `session_start` with the knob set
low, only stale subtrees are gone and the current lead's subtree is intact.
