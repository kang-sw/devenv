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
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 5aea625fddd7e749
sage-review-completeness-reviewed: 5aea625fddd7e749
completed: 2026-09-05
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
  `ws-agent-stop`, `ws-agent-transcript` and `ws-approve` resolve the alias
  against the registry through one helper (an execute worker never carries a
  lead-chosen alias, so `ws-approve` resolves uuids in practice, but the
  parameter contract is the same); spawn returns both `agent_id` and
  `alias`. Pushed-message heads,
  the TUI renderer, `ws-agent-list` rows and the orphan roll-call print the
  alias when there is one, followed by the uuid.
- **Alias reuse overwrites.** A new spawn with an alias already held by a
  dormant or idle record takes it: the previous holder's alias is cleared
  (its `title` stays; only the identifier moves) and it stays reachable by
  uuid. Reuse is rejected when the holder is `running` or thread-bound — a
  child the lead is still waiting on, or one the owner is talking to, must
  not be renamed underneath its plan (the same exemption class park and
  eviction use). No `ws-agent-close`: parking plus the cap below cover
  what an explicit close would, and the owner preferred fewer tools.
- **Idle children are parked at once.** When a child's turn settles with
  nothing queued, the adapter stops its process silently (the existing
  silent-stop path) after the settle bookkeeping — the deferred `final` push
  is composed, the idle-without-final advisory is judged, `lastReportAt` is
  recorded — and the record becomes dormant. No `ws-agent-settled` is pushed
  for the park itself. Children blocked on a question or an approval are not
  idle in Pi (the tool call holds the turn open) and are unaffected. No grace
  period: one rule, and a follow-up right after a `final` costs one resume.
  Two exemptions and one safety net (design review, 2026-09-05):
  - *Thread-bound records are never parked.* A record with an owner
    discussion thread open on it (`threadBound`: a `lead-ask` discussion fork
    or a fork-raised question the owner is answering) settles idle after
    every owner line and is prompted directly by the overlay `ForkChannel`;
    parking it would add a process resume to each line of an interactive
    exchange. It becomes park-eligible when the thread closes (`/done`, the
    fork's own `final`, `ws-resolve`) and settles next.
  - *Park is the last step of settle handling and is skipped when the settle
    itself re-prompted the child.* `fork.ts`'s anti-bleed nudge prompts a
    fork that went idle without a report from inside the same settle
    handling; if `record.running` is true again when park's turn comes, the
    child is not parked.
  - *The resume seam is `sendToAgent`'s dormant branch, and every prompt
    site already reaches it.* The overlay `ForkChannel` delegates to
    `sendToAgent`; `spawnAgent` prompts a client it just started; the nudge
    fires synchronously inside the fork's own `agent_settled` listener, ahead
    of the spawner's async settle body where park is appended, so
    `record.running` is already true when park's turn comes. The exemptions
    above are therefore a latency choice, not a correctness requirement.
    Defense in depth, not the mechanism: `promptAgent` itself keeps its
    live-client signature; a guard at the resume-capable seam (`sendToAgent`)
    is what any future direct caller must go through, and a test pins that
    a parked record prompted via the overlay path and via the nudge path
    comes back.
  - *Resume sees the whole transcript.* The park runs after the child's own
    `agent_settled`, by which point Pi has appended the turn to its session
    file; the test that pins it is park → resume by alias →
    `ws-agent-transcript` shows the parked turn's `final`.
- **Status-line presence keys on the registry, not on live processes.** The
  `N delegated agents still running` line is present whenever the registry
  holds any non-thread-bound member (dormant included) and omitted only when
  it holds none; N is unchanged. A member is thread-bound
  (`RpcAgentRecord.threadBound`) while an owner discussion thread is open on
  it — a discussion fork or a fork-raised question the owner is answering —
  and such a member belongs to the owner's exchange, not to the lead's fan-in. Without this the last `final` of a fan-out
  would park its sender and drop the line instead of reading `0 …`. The
  orphan roll-call therefore ends with the `0 …` line, which is accurate for
  a set that was just re-registered dormant.
- **The sidecar persists dormant entries too; the thread-bound skip stays.**
  `captureOrphans` today skips a record when it has no client *or* is
  thread-bound. Only the first condition is removed: dormant entries are
  written with `alias`, `title`, and the initial `prompt`, so `/reload` and
  `/resume` keep the full `ws-agent-list` and a revived record still names
  itself. The thread-bound skip is deliberate and kept — the owner surface's
  own `<sessionFile>.ws-threads.json` already persists and rehydrates a
  thread respondent, and writing it here too would register the same agent
  twice at `session_start` and put a thread respondent into the roll-call.
- **The registry is capped; dormant entries are evicted LRU.** Default 256
  entries (owner-set; 32 was judged too small), overridable by the
  environment variable `WS_PI_AGENT_REGISTRY_CAP` read by the lead process
  at spawn time (the adapter has no config file; its existing knobs are
  environment-driven, like the spawn role). When a spawn would exceed it, the dormant entries with the oldest
  last activity (last send or last report) are forgotten until the spawn
  fits; running and thread-bound records are never evicted, and a spawn
  that cannot fit without evicting one of those fails with an error. The spawn result
  carries one line naming what was evicted (alias or uuid) — the spawn
  caused it, so the notice belongs there, not in a separate advisory push.
  Eviction only forgets the registry entry; the session file stays on disk.
  The sidecar is written under the same cap.
- **`ws-agent-list` gains `alias`, `title`, and opt-in `include_prompt`.**
  Default rows add `alias`/`title` when set. `include_prompt: true` adds the
  initial prompt so a lead recovering from compaction or `/reload` can
  re-read what each child was asked; off by default because prompts are long
  and the common call is a liveness check. The stored prompt is
  head-truncated to 4 KB at spawn (a marker line notes the cut), which bounds
  the record, the sidecar (at most 256 × 4 KB of prompt text) and the
  `include_prompt` reply alike.
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
cap with its eviction notice. Anchors whose current prose park contradicts:
`{#260903-pi-spawner-completion-gating}` ("the child stays alive after
settling, ready for the next `ws-agent-send`" becomes "the child is parked
after settling and resumes on the next prompt"),
`{#260904-pi-report-to-lead-channel}` ("the delegated set only grows across a
session (an idle child keeps its process …)" is rewritten to the registry
presence rule), and `{#260903-pi-delegation-spawner-tools}` (the
`ws-agent-list` status vocabulary running / idle / dormant stays, with a
note that `idle` is transient). The sibling's unlanded Phase 2 test line
"children idle … → normal re-fire" describes a state that becomes transient;
N is unchanged, so that is wording only, adjusted when Phase 2 lands.

## Phases

### Phase 1: Alias, title, park, cap

- `ws-agent-spawn`: add optional `alias` and `title`; store them and the
  initial `prompt` on `RpcAgentRecord`; overwrite semantics with the
  running-holder rejection; apply the registry cap before registering and
  append the eviction line to the result.
- Alias resolution helper used by `ws-agent-send`, `ws-agent-stop`,
  `ws-agent-transcript`, `ws-approve`.
- Automatic park: as the last step of settle handling (after
  `flushPendingFinal` and the advisory/nudge judgment), when the record is
  neither `threadBound` nor `running`, stop the process silently and clear
  live state. The resume seam stays `sendToAgent`'s dormant branch;
  `onResume` role re-wiring fires there whichever caller reached it.
- `computeRunningStatusLine`: presence keyed on any non-thread-bound
  registry member.
- Sidecar: drop the `!record.client` skip in `captureOrphans` (keep the
  `threadBound` skip); persist the new fields; `reviveOrphans` restores
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
  no park for a `threadBound` record and for a record the nudge re-prompted;
  a parked record resumed through the overlay `ForkChannel` and through the
  nudge path (not only `ws-agent-send`); park → resume → transcript still
  holds the parked turn; thread-bound records excluded from eviction; prompt
  head-truncation at 4 KB; cap eviction order and the result line; sidecar round-trip for old and new
  shapes; list rows with and without `include_prompt`; head rendering.
- Verification: `cd agents-plugin-pi && npm test`; one live run with three
  aliased workers checking the push heads, that `ws-agent-list` shows them
  dormant after their finals, that `ws-agent-send <alias>` resumes one, and
  that a `/reload` keeps all of them in the list.

### Result (a8566a79) - 2026-09-05

Landed as `c5e6f9e2` (survey plan), `4060bc59` (feature), `e033c56d`
(guide and spec), `a8566a79` (review relay #1), `b17e56cf` (spec tidy), on
the implementation branch under the goal branch.

Behavioral delta:

- `ws-agent-spawn` takes optional `alias` and `title`, stores them and the
  head-truncated (4 KB, marker line) initial `prompt` on the record, and
  returns `{agent_id, alias?, evicted?}`. `runSpawnGuards` runs the alias
  guard (overwrite a dormant/idle holder, reject a running or thread-bound
  one) and the cap guard (`WS_PI_AGENT_REGISTRY_CAP`, default 256, dormant
  LRU on last send/report, running and thread-bound never evicted) and
  clears the previous holder's alias only after both pass, so a rejected
  spawn leaves no trace (review relay #1, Critical).
- One `resolveAgentId` helper resolves uuid-or-alias for `ws-agent-send`,
  `ws-agent-stop`, `ws-agent-transcript` and `ws-approve`.
- Automatic park: the last step of the spawner's settle handling stops a
  settled record that is neither `threadBound` nor running again, silently
  (no `ws-agent-settled`), after `flushPendingFinal` and the advisory
  judgment; the fork nudge runs synchronously ahead of it, so a re-prompted
  fork is not parked. `stopAgent` now clears live state before its awaits,
  so a send that lands mid-park takes the dormant-resume branch (relay #1,
  Important).
- `computeFanIn` presence keys on any non-thread-bound registry member
  (dormant included); N is unchanged, and `hasRunningAgents` still counts
  only running children. Sidecar capture keeps only the `threadBound` skip
  and persists `alias`/`title`/`prompt`; old-shape entries still revive; the
  roll-call names `alias (uuid)`. Pushed heads use the same `alias (uuid)`
  form and the TUI renderer draws it unchanged.
- `ws-agent-list` rows carry `alias`/`title` when set and the stored prompt
  under `include_prompt: true`. `pi-lead-guide.md` recommends alias and
  title on every spawn, explains parking and the cap notice, and no longer
  claims the status line is absent whenever nothing is running.
- Tests 694/694 (+46 over the 648 baseline): alias guards and
  `runSpawnGuards` ordering, cap parsing and eviction order, truncation,
  resolution on send/stop/transcript, park after settle with the `final`
  still delivered and the `0 …` line present, no park for thread-bound and
  nudge-re-prompted records, the mid-park send race, park → resume by alias
  → transcript holds the parked turn, nudge-path resume of a parked fork,
  sidecar round-trip for old and new shapes, list rows with and without
  `include_prompt`, head rendering.

Deferred: the overlay `ForkChannel` resume test is [not fixed] — the
channel is a one-line delegation into `sendToAgent`'s dormant branch, which
constructs a real `RpcClient` with no offline substitute; the `ws-agent-send`
and nudge paths pin the same seam. `ws-approve` alias resolution has no
dedicated test (two-line delegation to the tested helper). The three-worker
live run is owner-run.

Review: correctness 1 Critical [fixed, re-review resolved] + 1 Important
[fixed]; fit 1 Important [fixed] (JSDoc placement); test 1 Important, two of
its three cases [fixed], the overlay case [not fixed] as above. Minor
findings recorded in the review files only.


## Resolution (2026-09-05)

Phase 1 landed on the goal branch: alias/title on spawn with overwrite-or-reject semantics, alias resolution on every `agent_id` parameter, silent park at idle, registry-keyed status-line presence, sidecar persistence of dormant entries, `ws-agent-list` alias/title/include_prompt, and the 256-entry LRU-capped registry. Owner-run: the three-aliased-worker live run (push heads, dormant after finals, resume by alias, `/reload` keeps the list).
