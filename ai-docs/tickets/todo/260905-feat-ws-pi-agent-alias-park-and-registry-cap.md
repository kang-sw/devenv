---
title: "Pi adapter: agent alias and title, park idle children, cap the registry"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260905-feat-ws-pi-push-only-child-reports: sibling — its status line now carries only a running count; this ticket names the children and changes when the line is present
  260905-feat-ws-pi-live-agent-widget: consumer — the always-visible widget should show alias/title, not the uuid
  260903-feat-ws-pi-subagent-rpc-ux: origin of `ws-agent-spawn` / `ws-agent-list` / `ws-agent-stop` and the RPC agent registry this ticket extends
related-mental-model:
  - plugin-runtime
spec:
  - pi-adapter-runtime
sage-review-design: recommended
---

# Pi adapter: agent alias and title, park idle children, cap the registry

## Background

Owner request (2026-09-05, after the second live run of the push-only report
channel). Three gaps in how the lead and the owner see and keep children:

- **Children are named only by uuid.** `ws-agent-spawn` takes
  `system_prompt_path`, `prompt`, `model_name`, `model_effort`; every push
  reads `[ws-agent-report] agent 5e134551-…`, `ws-agent-list` returns
  `{agent_id, status, last_report_at}`, and the widget ticket would render
  uuids too. The fan-in status line used to list running ids, which the owner
  judged noise and which duplicated `ws-agent-list`; the sibling ticket's
  Edition dropped it.
- **Idle children keep a Pi process.** A worker that filed `final` and
  settled stays live until the lead stops it or the session ends. The
  process holds memory and a model context for nothing, and the delegated
  set only grows across a session. A dormant child is already resumable from
  its session file (`ws-agent-send` re-hydrates it), so keeping the process
  buys nothing but resume latency.
- **The registry and the sidecar grow without bound.** No path removes a
  registry entry; the shutdown sidecar re-registers every entry at the next
  `session_start`. Today the sidecar skips records without a client, so
  stopped children silently vanish across `/reload`; once idle children are
  parked, every finished child takes that path.

## Decisions

- **`alias` and `title` are optional `ws-agent-spawn` parameters.** `alias`
  is a short identifier the lead chooses (`/` allowed, e.g. `review/a-b-c`,
  so a prefix groups a fan-out); `title` is one human-readable line. Both
  are persisted on the record and in the sidecar. The adapter never derives
  either from the prompt.
- **Every `agent_id` parameter accepts a uuid or an alias.** `ws-agent-send`,
  `ws-agent-stop`, `ws-agent-transcript` resolve the alias against the
  registry; spawn returns both `agent_id` and `alias`. Pushed-message heads,
  the TUI renderer, `ws-agent-list` rows and the orphan roll-call print the
  alias when there is one, followed by the uuid.
- **Alias reuse overwrites.** A new spawn with an alias already held by a
  dormant or idle record takes it: the previous holder's alias is cleared and
  it stays reachable by uuid. Reuse is rejected only when the holder is
  `running` — a child the lead is still waiting on must not be renamed
  underneath its plan. No `ws-agent-close`: parking plus the cap below cover
  what an explicit close would, and the owner preferred fewer tools.
- **Idle children are parked at once.** When a child's turn settles with
  nothing queued, the adapter stops its process silently (the existing
  silent-stop path) after the settle bookkeeping — the deferred `final` push
  is composed, the idle-without-final advisory is judged, `lastReportAt` is
  recorded — and the record becomes dormant. No `ws-agent-settled` is pushed
  for the park itself. Children blocked on a question or an approval are not
  idle in Pi (the tool call holds the turn open) and are unaffected. No grace
  period: one rule, and a follow-up right after a `final` costs one resume.
- **Status-line presence keys on the registry, not on live processes.** The
  `N delegated agents still running` line is present whenever the registry
  holds any non-thread-bound member (dormant included) and omitted only when
  it holds none; N is unchanged. Without this the last `final` of a fan-out
  would park its sender and drop the line instead of reading `0 …`. The
  orphan roll-call therefore ends with the `0 …` line, which is accurate for
  a set that was just re-registered dormant.
- **The sidecar persists every registry entry.** Dormant entries included,
  with `alias`, `title`, and the initial `prompt`, so `/reload` and `/resume`
  keep the full `ws-agent-list` and a revived record still names itself.
- **The registry is capped; dormant entries are evicted LRU.** Default 256
  entries (owner-set; 32 was judged too small), tunable through the adapter
  config. When a spawn would exceed it, the dormant entries with the oldest
  last activity (last send or last report) are forgotten until the spawn
  fits; running records are never evicted, and a spawn that cannot fit
  without evicting a running record fails with an error. The spawn result
  carries one line naming what was evicted (alias or uuid) — the spawn
  caused it, so the notice belongs there, not in a separate advisory push.
  Eviction only forgets the registry entry; the session file stays on disk.
  The sidecar is written under the same cap.
- **`ws-agent-list` gains `alias`, `title`, and opt-in `include_prompt`.**
  Default rows add `alias`/`title` when set. `include_prompt: true` adds the
  initial prompt so a lead recovering from compaction or `/reload` can
  re-read what each child was asked; off by default because prompts are long
  and the common call is a liveness check.
- **Rejected:** auto-generated aliases or titles (the lead has the intent in
  hand at spawn; a derived name is worse than none); alias uniqueness errors
  on dormant holders (the owner chose overwrite); a live-process cap (parking
  makes it moot); a grace period before parking; a separate
  `ws-agent-close`.

## Constraints

- No ws-mcp changes: `agents-plugin-tool/` and `agents-plugin/skills/` stay
  untouched. The adapter alone owns these fields and rules.
- Sidecar compatibility: entries written by the current build (no `alias`,
  `title`, `prompt`, and only live records) must still revive.
- The `final`-at-turn-end and hold-until-settle rules from the sibling ticket
  are unchanged; parking runs after them in the same settle handling.
- `ws-agent-stop` keeps its meaning (explicit park, `reason:"stopped"`
  pushed); the automatic park is silent.

## Spec Impact

`pi-adapter-runtime`: `ws-agent-spawn` parameters and result, alias
resolution on every `agent_id` parameter, `ws-agent-list` row shape and
`include_prompt`, pushed-message head form, automatic park at idle, the
status-line presence rule, sidecar entry shape and scope, and the registry
cap with its eviction notice.

## Phases

### Phase 1: Alias, title, park, cap

- `ws-agent-spawn`: add optional `alias` and `title`; store them and the
  initial `prompt` on `RpcAgentRecord`; overwrite semantics with the
  running-holder rejection; apply the registry cap before registering and
  append the eviction line to the result.
- Alias resolution helper used by `ws-agent-send`, `ws-agent-stop`,
  `ws-agent-transcript`.
- Automatic park: in the settle handling after `flushPendingFinal` and the
  advisory judgment, stop the process silently and clear live state; verify
  `onResume` role re-wiring still fires on the next `ws-agent-send`.
- `computeRunningStatusLine`: presence keyed on any non-thread-bound
  registry member.
- Sidecar: persist every entry with the new fields; `reviveOrphans` restores
  them; old-shape entries still load. The roll-call summary uses alias/title.
- `ws-agent-list`: `alias`, `title`, `include_prompt`.
- Naming surfaces: `buildPushContent` head, `src/push-render.ts`,
  `buildOrphanSummary`.
- `pi-lead-guide.md`: recommend an alias and title on every spawn; explain
  that idle children are parked and resume on send; describe the cap notice
  and `include_prompt` as the recovery path.
- Tests: spawn with/without alias and title; alias overwrite on dormant and
  rejection on running; resolution by alias on send/stop/transcript; park
  after settle with the `final` still delivered and the `0 …` line present;
  cap eviction order and the result line; sidecar round-trip for old and new
  shapes; list rows with and without `include_prompt`; head rendering.
- Verification: `cd agents-plugin-pi && npm test`; one live run with three
  aliased workers checking the push heads, that `ws-agent-list` shows them
  dormant after their finals, that `ws-agent-send <alias>` resumes one, and
  that a `/reload` keeps all of them in the list.
