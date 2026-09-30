---
title: "Disconnect the pi owner-question wiring: no mid-run questions from agents"
related:
  260908-research-ws-pi-ws-ask-removal: prior decision ("redesign, not removal") that this ticket reverses at the wiring level
  260911-feat-ws-pi-async-question-queue: implementation whose entry points this ticket disconnects; its code stays
  260923-bug-pi-fork-owner-thread-windows-instability: fork-raised owner threads stop being created, which removes that bug's trigger; not closed here
  260909-research-ws-refoundation-evidence-audit: binding anchor; stop-condition verdict (workers batch decisions into the stop report)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 14f2c566db4a49b0
sage-review-completeness-reviewed: 14f2c566db4a49b0
---

# Disconnect the pi owner-question wiring: no mid-run questions from agents

## Background

The pi extension (`agents-plugin-pi/`) lets agents raise questions to the
owner through two entry paths into one thread registry in `src/ask.ts`:

- **Lead-raised:** the top lead calls `ws-queue-question` /
  `ws-withdraw-question`; `/answer` opens a sequential prose-modal queue.
- **Fork-raised:** a `ws-fork` child calls `ws-report-to-lead` with
  `kind:"question"` and ends its turn; the lead's `onForkQuestion` hook binds an
  owner thread to the live fork (`threadBound`), the agent widget shows
  `⚠ OWNER ACTION · /answer qN`, and `/answer` / `/done` attach and close it.

Owner report (2026-09-30): the OWNER ACTION surface has low practical use,
while a single agent question tends to break the whole workflow run. The
structural cause is visible in code: a `threadBound` record is excluded from
fan-in, alias reuse, parking and eviction, so a questioning fork parks outside
the lead's wait set until the owner acts.

`260908-research-ws-pi-ws-ask-removal` settled "redesign, not removal" on
2026-09-11 on cost grounds (the discussion fork). The owner now reverses that
at the wiring level on different grounds: the question itself stalls the
workflow. This matches the binding anchor's stop-condition verdict: a worker
stops only on a closed list and batches its own decisions into the stop report,
rather than stopping mid-run to ask.

## Decisions

- **No mid-run questions from spawned agents (option b).** A fork or worker
  decides and proceeds, recording assumptions under the final answer's
  `Decisions:` field; when it genuinely cannot proceed it settles with the
  blocker under `Blockers:` instead of parking on a question. The lead resumes
  the same agent with `ws-agent-send`, so the turn count is effectively the
  same as a question round-trip.
  Rejected: (a) keep `ws-report-to-lead` `kind:"question"` and route every
  question to the lead through the headless `ws-agent-question` baseline — the
  owner surface would disappear but the ask-and-park pattern would survive,
  merely moved to the lead.
- **Disconnect wiring; keep the code.** `src/ask.ts`, `startForkFinish`, the
  thread registry, and the related record fields stay in the tree; only the
  entry points that create new questions are cut.
  Rejected: deleting the feature outright (roughly 3,000 source lines plus
  `test/ask.test.ts`) in this ticket. Whether to delete the dormant code later
  is a separate, undecided question.
- **Cut points:**
  1. The top lead's active-tool list no longer gains `ws-queue-question` /
     `ws-withdraw-question` (the `addAskToolsIfLead` step in the lead
     bootstrap). Tool registration (`registerAsk`) stays. Because a fork
     inherits the lead's active-tool snapshot, forks lose them too.
  2. The fork-raised owner-thread hook is not armed: `onForkQuestion` is no
     longer passed to `registerFork` nor to `armForkRoleWiring` on orphan
     revival, so `record.onQuestionReport` is never set and no fork-raised
     thread is created.
  3. Agent-facing text stops inviting questions: the fork directive and
     initial message in `src/fork.ts` no longer tell the fork to call
     `ws-report-to-lead` with `kind:"question"` and end its turn, and instead
     direct it to decide-and-record or settle with `Blockers:`; the worker
     lifecycle guide appended to delegating workers (`WORKER_LIFECYCLE_GUIDE`
     in `src/spawner.ts`, "ws-report-to-lead is only for progress or a question
     before settlement") gets the same treatment (only the
     directive names `kind:"question"`, `agents-plugin-pi/src/fork.ts#L121`;
     the initial message says to use `ws-report-to-lead` "only for a question
     or progress before settlement", `agents-plugin-pi/src/fork.ts#L163`); the
     `ws-report-to-lead` schema drops the `kind` parameter so the tool is a
     progress/finding report only.
  4. Shipped text that names the disconnected surface is updated (see
     "Shipped text updated in step" below for the list).
- **`kind` leaves `ws-report-to-lead` for every caller role** (fork, worker,
  explore, execute-worker), not only forks: option (b) covers forks and
  workers alike, and the tool is registered once for all these roles.
- **Dormant question handling stays.** The `kind:"question"` branch in
  `applyRpcEvent`, the `ws-agent-question` push family, and its hold/batch
  handling remain as unreachable code, consistent with keeping the code.
- **A residual `kind` is ignored at the report site.** Dropping `kind` from
  the schema does not stop a caller from sending it: the schema does not set
  `additionalProperties: false`, and `applyRpcEvent` reads `args.kind` from the
  tool-call event directly. The likely sender is a pre-change fork revived from
  its `--session` file, whose history still holds the old "call with
  `kind:"question"` and end the turn" directive. `applyRpcEvent` therefore
  treats every `ws-report-to-lead` call as a plain report (`ws-agent-report`
  push) regardless of `kind`, which is what makes the question branch
  unreachable. The stale fork then settles, and the lead reads the report and
  resumes it with `ws-agent-send`, the same flow as option (b).
  Rejected: closing the schema with `additionalProperties: false` — whether Pi
  validates arguments before emitting `tool_execution_start` is unverified, so
  the push could still fire; accepting the residual `ws-agent-question` steer
  push as transitional — that is the rejected option (a) behavior, and the
  updated lead guide would no longer describe it.
- **Shipped text updated in step:** `pi-lead-guide.md` (the
  `ws-queue-question` / `ws-withdraw-question` rows, the `ws-agent-question`
  push row, the owner-side question paragraph that opens with how a question
  was raised, and the question wording in the `ws-fork` row and the
  `ws-agent-report` row), the `ws-fork` tool description in `src/fork.ts`, the
  `ws-agent-spawn` description in `src/spawner.ts` ("its reports, questions and
  completion arrive"), and the fork input frame in `src/fork-context.ts`, so no downstream lead or spawned agent is told about
  behavior it no longer has (Architecture Rule 4). The sweep covers every
  role's agent-facing text, not only the fork's. The list above is what
  authoring-time review found (plus the `kind` parameter description in the
  `ws-report-to-lead` registration, which goes away with the parameter); an
  authoring-time grep missed `WORKER_LIFECYCLE_GUIDE`, so the implementer
  re-sweeps `src/` string literals, `*-guide.md`, and `rsrc/` for any remaining
  invitation to ask a question mid-run before closing.
- **Spec note only:** the side-thread owner question surface section of
  `ai-docs/spec/pi-adapter-runtime.md` gains a short note that its entry points
  are disconnected; the section is not rewritten, since the binding anchor
  retires the spec layer.
- **Leftover persisted threads drain through the existing commands.** `/answer`,
  `/thread`, the `ctrl+shift+a` shortcut, and `.ws-threads.json` hydration stay
  registered, so any pending thread persisted before this change can still be
  answered or closed; no new thread can be created.
  Rejected: cutting hydration and the commands too — a hydrated pending thread
  would still render an OWNER ACTION row with no way to answer it.

## Constraints

- `/audit` owner steering is out of scope: its `idle-awaiting-owner` state,
  the AWAITING OWNER banner, the attention animation, and the bare
  `⚠ OWNER ACTION · <name>` row variant are shared with `/audit` and stay.
- `ws-execute` approval routing (`awaiting-approval`, `ws-approve`) is a
  separate feature and stays untouched.
- `ws-fork` itself, `ws-report-to-lead` as a progress tool, and the push
  families stay registered.
- Stale `ws-ask` mentions in other open tickets
  (`260906-workset-ws-pi-dogfood-ux`,
  `260908-feat-ws-pi-claude-code-lead-provider`) are out of scope; those
  tickets are edited when next worked.
- Verification: the `agents-plugin-pi` typecheck and test suite pass. Tests
  that pin the old surface are updated, and assertions are added that the top
  lead's active tools exclude `ws-queue-question` and `ws-withdraw-question`,
  that fresh and revived forks arm no `onQuestionReport`, that
  `ws-report-to-lead` has no `kind` parameter, that a `ws-report-to-lead` event
  carrying `kind:"question"` yields a `ws-agent-report` push (not
  `ws-agent-question`), and that neither the fork
  directive, the fork initial message, nor the worker lifecycle guide invites
  the agent to ask a question. No live Pi TUI dogfood
  is required; the earlier hide of the same tools (`ac998f77` / `a8cf1183`)
  was verified with focused helper tests only.

## Prior Decisions

- 260911-feat-ws-pi-async-question-queue (2026-09-11, commit 467e7996): "Rename to ws-queue-question is a contract change (async/non-blocking/answer-when-ready, Codex queued-question shape), not cosmetic; the blocking path stays ws-report-to-lead(kind:question)." — bearing: superseded by this ticket (question path disconnected per option b)
- 260908-research-ws-pi-ws-ask-removal (2026-09-11, commit 467e7996): "Rename to ws-queue-question is a contract change ... Reverses 260904's 'no fork-less quick-answer path / discussions dominate' on dogfood evidence." — bearing: constrains
- 260904-feat-ws-pi-side-thread-fork-question-surface (2026-09-04, Decisions): "`ws.ask` / `ws.resolve` are removed too: a fork's only question path is `ws-report-to-lead(kind: \"question\")` (§1)." — bearing: superseded by this ticket (question path disconnected per option b)
- 3ec8d5af (2026-09-08, commit): "Fork cache reuse depends on the lead's exact post-handler prompt and callable registration order ... Role safety is execution-time refusal, not schema hiding: forks retain the lead's provider-visible tools" — bearing: constrains
- 260913-bug-ws-pi-settled-agent-falsely-remains-running (2026-09-13, commit 6bafc76f): "Keep ws-report-to-lead informational and question-oriented, removing final-report parsing, adequacy validation, and report-tool completion dependencies from workers and forks." — bearing: superseded by this ticket (question path disconnected per option b)
- 260905-bug-ws-pi-fork-question-lead-notice-dropped (2026-09-06, Resolution): "Owner-run acceptance scenario E re-run on 2026-09-06 passed: the fork's question was registered as a thread (`registered as thread q2`) and the final report carried the owner's answer" — bearing: constrains
- 834fa99a (2026-09-06, commit): "Anchor ids (`{#260904-pi-report-to-lead-channel}`, `{#260905-pi-side-thread-owner-question-surface}`) are unchanged — wording-only amendment, no renamed-spec trailer needed." — bearing: constrains
- 260906-bug-ws-pi-lead-cannot-see-or-load-skills (2026-09-06, commit 5b8d956c): "lead-bootstrap.ts's buildWsBlock grew a third skillsBlock parameter (order: manual snapshot, guide text, skills block)." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/lead-bootstrap.ts#L242 addAskToolsIfLead call, agents-plugin-pi/src/index.ts#L828-L889 onForkQuestion wiring, agents-plugin-pi/src/fork.ts, agents-plugin-pi/src/spawner.ts#L4310-L4327 ws-report-to-lead schema, agents-plugin-pi/src/fork-context.ts#L234, agents-plugin-pi/pi-lead-guide.md, ai-docs/spec/pi-adapter-runtime.md#L1399, tests |
| scope.surface | public-interface | ws-report-to-lead tool schema loses kind for every role; top lead active tools lose ws-queue-question and ws-withdraw-question; shipped pi-lead-guide.md text |
| scope.new_public_symbol | no | none; the change removes entry points only |
| scope.new_type_contract | no | none added; the kind parameter is removed from the ws-report-to-lead schema |
| scope.test_surface | existing | agents-plugin-pi/test/lead-bootstrap.test.ts, fork.test.ts, spawner.test.ts, ask.test.ts, agent-sidecar.test.ts, fork-lifecycle.integration.test.ts, fork-prefix.integration.test.ts, fork-review-regressions.test.ts, agent-widget.test.ts, ownership-contention.test.ts reference the old surface |
| complexity.reuse_points | not-applicable | disconnection only; no component is reused to build new behavior |
| complexity.side_effect_risk | moderate | the lead tool list and report-tool schema feed the fork's inherited callable surface and prompt-cache prefix, and orphan revival re-arms forks |
| risk.correctness | moderate | dormant ask.ts paths, persisted .ws-threads.json hydration, and /answer draining must keep working while no new thread can be created |
| risk.fit | moderate | shipped pi-lead-guide.md, ws-fork description, and fork-context frame must all drop the disconnected surface under Architecture Rule 4 |
| risk.test | moderate | ten test files pin kind question, onForkQuestion, or the ask tools and must be updated without deleting dormant-code coverage |
| risk.security_or_contract | moderate | removes the kind parameter from a tool contract shared by fork, worker, explore, and execute-worker roles |

## Phases

### Phase 1: Disconnect question entry points and update agent-facing text

Apply the four cut points in `## Decisions`, update the shipped text and spec
note named there, and adjust tests per `## Constraints`. The dormant code in `src/ask.ts` and
the fork-finish machinery stays compiled and its direct unit tests keep
passing.
