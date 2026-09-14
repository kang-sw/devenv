# Plan: A fork-raised owner question never reaches the lead as the promised thread notice — Phase 1: Deliver the notice

## Relevant Ticket Contract
- In `spawner.ts`'s `applyRpcEvent`, when `record.onQuestionReport` returns a
  string, return
  `{ push: { family: "ws-agent-advisory", payload: { advisory: "fork-question-thread", detail: notice }, deliverAs: "followUp" } }`
  instead of `{}`.
- `undefined` from the hook still yields the headless
  `ws-agent-question`/`steer` baseline; a throwing hook still degrades to that
  same baseline.
- Rewrite the comment block above the question branch to describe the notice
  push, not the suppression.
- Invert the three landed suppression tests (two in `test/spawner.test.ts`,
  one in `test/fork.test.ts`) to assert the new push.
- Amend `ai-docs/spec/pi-adapter-runtime.md`: the owner-question surface
  anchor's "Attach to a live task fork" bullet, the report-channel anchor's
  family table, and the two "thread-bound pushes no settle or advisory"
  sentences (report-channel anchor and side-thread anchor) with the
  registration-notice carve-out.
- The record becomes thread-bound inside the hook (`handleForkRaisedQuestion`
  in `ask.ts`, called from `index.ts`'s `onForkQuestion`) *before* the notice
  string is returned, so this one push is emitted for an already-thread-bound
  record — this is the single carve-out to "thread-bound records push no
  settle or advisory."

## Out of Scope
- `attachEventListener`'s settle-branch gate (`spawner.ts:1972`,
  `!record.threadBound && !record.terminalThisTurn`) and `fork.ts`'s
  anti-bleed loop (`wireAntiBleedLoop`, `fork.ts:401-559`) — both stay
  untouched per the ticket's Decisions.
- Any change to `agents-plugin-tool/` or `agents-plugin/skills/`.
- The live acceptance re-run (owner-run, out-of-band; not part of this
  phase's automated verification).
- Headless behavior, `onFinalReport`, approval-gateway branches — unaffected.

## Codebase Findings
- `agents-plugin-pi/src/spawner.ts#L1830-L1856` — `applyRpcEvent`'s
  `tool_execution_start`/`REPORT_TO_LEAD_TOOL_NAME` branch, `kind === "question"`
  sub-branch. Current code:
  ```ts
  if (kind === "question") {
    // A defined return means the owner surface consumed it (TUI); only
    // the headless `undefined` case reaches the lead. A throwing hook
    // degrades to the headless baseline rather than dropping the report.
    let consumed = false;
    if (record.onQuestionReport) {
      try {
        consumed = record.onQuestionReport(record, message) !== undefined;
      } catch {
        consumed = false;
      }
    }
    return consumed ? {} : { push: { family: "ws-agent-question", payload: { question: message }, deliverAs: "steer" } };
  }
  ```
  Change: capture the hook's return value itself (a string or `undefined`),
  push the advisory when it is a string, keep the existing baseline branch
  otherwise. The doc block at `L1798-L1804` ("260904 Phase 2 ... nothing is
  pushed at all") is the comment to rewrite.
- `agents-plugin-pi/src/spawner.ts#L1757-L1762` — `RpcEventOutcome.push` shape:
  `{ family: PushFamily; payload: Record<string, unknown>; deliverAs: PushDeliverAs }`.
  `"ws-agent-advisory"` is already a member of `PUSH_FAMILIES`
  (`spawner.ts#L1081-L1088`); no type change needed.
- `agents-plugin-pi/src/fork.ts#L438-L457` and `#L500-L558` — every existing
  `ws-agent-advisory` push shape to match: `payload: { advisory: "<name>",
  detail: "<text>" }` (plus `transcript_tail` only for `"stalled"`), always
  `deliverAs: "followUp"`, sent via `pushToLead(pi, rpcRegistry, record,
  "ws-agent-advisory", payload, "followUp")`. `applyRpcEvent` itself is pure
  (no `pi`/registry) and only *describes* the push via `RpcEventOutcome`;
  `attachEventListener` (`spawner.ts#L1958-L1963`) is what actually calls
  `pushToLead` for whatever `outcome.push` names — so the sender-label
  `<alias> (<id>)` formatting and status-line composition are already handled
  generically there and need no new code.
- `agents-plugin-pi/src/ask.ts#L129-L135` — `buildForkQuestionLeadNotice`:
  builds the exact notice string that must become `detail`. No change needed
  here; it already returns the right text.
- `agents-plugin-pi/src/index.ts#L326-L330` — `onForkQuestion` callback: calls
  `handleForkRaisedQuestion` (which thread-binds the record) *then*
  `buildForkQuestionLeadNotice` in TUI mode, `undefined` in headless. This is
  the hook wired onto `record.onQuestionReport` via `registerFork`/
  `armForkRoleWiring`; confirms the record is already thread-bound by the time
  `applyRpcEvent` sees the hook's string return. No change needed.
- `agents-plugin-pi/src/spawner.ts#L1958-L1963` (`attachEventListener`) and
  `#L1972` (settle-branch gate `!record.threadBound && !record.terminalThisTurn`)
  — confirmed untouched surface for this phase. `record.terminalThisTurn` is
  already set to `true` for any defined `kind` (question or final,
  `L1836-L1841`) before the question branch runs, independent of this
  change.
- `agents-plugin-pi/src/fork.ts#L401-L481` — anti-bleed loop's own
  `questionTurn`/`threadBound` handling: a question turn is already a "valid
  stop" (no nudge, no advisory) and `record.threadBound` suppresses the
  no-signal nudge/stalled path independently. Neither branch touches the new
  push; confirmed no interaction.
- `agents-plugin-pi/src/push-render.ts#L1-L80` (full file has no per-advisory
  switch) — the TUI renderer (`buildPushRenderLines`) splits pushed content
  generically into head/body/status lines by shape, not by `details.advisory`
  value; grepped for `final-report-shape`/`expects-commit`/`stalled` in
  `push-render.ts` with no hits. The new `advisory: "fork-question-thread"`
  needs **no** renderer change.
- `agents-plugin-pi/test/spawner.test.ts#L733-L745` — test 1 to invert:
  "onQuestionReport returning a string SUPPRESSES the push entirely — the TUI
  owner surface consumed the question (§1)"; asserts `result` deepEqual `{}`.
- `agents-plugin-pi/test/spawner.test.ts#L1528-L1533` — test 2 to invert: "a
  hook-consumed question (the TUI owner surface) is not pushed at all";
  asserts `h.pi.sent` deepEqual `[]` (this is the `attachEventListener`-level
  listener-harness describe block, driven through the real listener rather
  than `applyRpcEvent` directly).
- `agents-plugin-pi/test/fork.test.ts#L577-L588` — test 3 to invert: "I6
  (260905): a hook return SUPPRESSES the question push — the owner surface
  consumed it"; asserts `outcome` deepEqual `{}`. Sibling tests at
  `#L590-L630` (`undefined` -> baseline, throwing -> baseline, `final` never
  reaches hook, no-hook -> baseline) already assert the correct behavior and
  need no change.
- `ai-docs/spec/pi-adapter-runtime.md#L496-L550` (`### Child→lead report
  channel {#260904-pi-report-to-lead-channel}`) — family table entry for
  `ws-agent-question` at `#L522-L524` ("In TUI the question is consumed by the
  owner question surface instead.") needs the push-notice fact added; the
  `ws-agent-advisory` entry at `#L528-L530` needs `fork-question-thread` named
  alongside the existing advisories; the suppression sentence at `#L549-L551`
  ("A record that is **thread-bound**... pushes no settle or advisory...")
  needs the registration-notice carve-out.
- `ai-docs/spec/pi-adapter-runtime.md#L826-L897` (`## Side-thread owner
  question surface {#260905-pi-side-thread-owner-question-surface}`) —
  "Attach to a live task fork (fork-raised threads)" bullet at `#L885-L893`
  currently says "hands the lead a **notice** in place of the question text"
  (still true) but "Registration marks the fork **thread-bound** (outside the
  pushed status line, no settle or advisory pushed) until the thread closes"
  at `#L891-L892` needs the same carve-out (the notice itself IS pushed, as
  `ws-agent-advisory`/`followUp`).

## Implementation Plan
1. `agents-plugin-pi/src/spawner.ts` — in the `kind === "question"` branch
   (`L1843-L1856`), replace the boolean `consumed` with the hook's actual
   return value:
   ```ts
   let notice: string | undefined;
   if (record.onQuestionReport) {
     try {
       notice = record.onQuestionReport(record, message);
     } catch {
       notice = undefined;
     }
   }
   return notice !== undefined
     ? { push: { family: "ws-agent-advisory", payload: { advisory: "fork-question-thread", detail: notice }, deliverAs: "followUp" } }
     : { push: { family: "ws-agent-question", payload: { question: message }, deliverAs: "steer" } };
   ```
2. `agents-plugin-pi/src/spawner.ts` — rewrite the "260904 Phase 2" doc
   paragraph at `L1798-L1804` (above the `kind === "question"` handling) to
   describe: a defined hook return is now pushed to the lead as a
   `ws-agent-advisory`/`fork-question-thread` notice (not the question text
   itself — the lead is told a thread exists, not asked to answer it);
   `undefined` (headless) still yields `ws-agent-question`/`steer`; a
   throwing hook still degrades to that same baseline. Keep the reference to
   the record already being thread-bound by the hook.
3. `agents-plugin-pi/test/spawner.test.ts#L733-L745` — invert: rename to
   reflect the push, and assert
   `result` deepEqual
   `{ push: { family: "ws-agent-advisory", payload: { advisory: "fork-question-thread", detail: "[ws] registered as thread q1" }, deliverAs: "followUp" } }`.
   Keep the `seen`/`reportLog.length` assertions.
4. `agents-plugin-pi/test/spawner.test.ts#L1528-L1533` — invert: rename, and
   assert `h.pi.sent` carries one message with `customType: "ws-agent-advisory"`
   and `details.advisory === "fork-question-thread"` (match this file's
   existing `families(h.pi)`-style helper if present nearby; otherwise assert
   directly on `h.pi.sent[0]`).
5. `agents-plugin-pi/test/fork.test.ts#L577-L588` — invert: rename, and
   assert `outcome` deepEqual
   `{ push: { family: "ws-agent-advisory", payload: { advisory: "fork-question-thread", detail: "[ws] thread T1 — the owner answers this." }, deliverAs: "followUp" } }`.
   Leave `L590-L630` (undefined/throwing/final/no-hook cases) unchanged — they
   already assert the retained baseline.
6. `ai-docs/spec/pi-adapter-runtime.md`:
   - Family table, `ws-agent-question` entry (~`L522-524`): add that a
     TUI-consumed question instead produces the one `ws-agent-advisory`
     notice below, rather than silently vanishing.
   - Family table, `ws-agent-advisory` entry (~`L528-530`): add
     `fork-question-thread` (a fork-raised question was registered as an
     owner thread) to the named advisories list.
   - Suppression sentence (~`L549-551`, report-channel anchor): add the
     carve-out — the registration notice is the one advisory a thread-bound
     record pushes; all later settles/advisories for it stay suppressed.
   - "Attach to a live task fork" bullet (~`L885-893`, side-thread anchor):
     state the notice is delivered as a pushed `ws-agent-advisory`
     (`fork-question-thread`, `followUp`) rather than only "hands the lead a
     notice" in the abstract, and amend the thread-bound parenthetical
     ("outside the pushed status line, no settle or advisory pushed") with
     the same one-notice carve-out.

## Verification Plan
- `cd agents-plugin-pi && npm test`
- Out-of-band (owner-run, not part of this phase): repeat acceptance scenario
  E and confirm the lead sees the notice before the owner opens `/answer`.

## Escalations
- None.
