---
title: Retire the Pi ws-claude tool and soften the compaction advisory nudge
related:
  260908-feat-ws-pi-claude-delegate-tool: reversed; the ws-claude tool it added is removed
  260914-bug-ws-claude-timeout-resume-handle: obsoleted; dropped with the tool
  260908-feat-ws-pi-claude-code-lead-provider: adjacent; it adds @anthropic-ai/claude-agent-sdk back on its own if pursued
  261002-feat-pi-lead-ws-owned-compaction: amended; its advisory preparation message wording changes
sage-review-design: skipped
sage-review-completeness: completed
sage-review-completeness-reviewed: ccdae0f030743b54
completed: 2026-10-03
---

# Retire the Pi ws-claude tool and soften the compaction advisory nudge

## Background

Two user-requested changes to `agents-plugin-pi`, bundled because both are
small and land in the same package.

- **ws-claude is too unstable to use.** The lead-only `ws-claude` tool
  (260908-feat-ws-pi-claude-delegate-tool: audit, consult, rewrite, and
  design-review presets over `@anthropic-ai/claude-agent-sdk`) has behaved
  unreliably enough in practice that the user does not use it. It is
  self-contained: `src/claude-delegate.ts`, `src/claude-sdk.ts`,
  `src/claude-design-review.ts`, `src/claude-delegate-prompts.ts`, their
  wiring in `src/index.ts`, six test files
  (`test/claude-{contract,delegate,lifecycle,resume-design,sdk,session}.test.ts`;
  `ls agents-plugin-pi/test | grep claude`), and the SDK dependency, which
  nothing else in the package imports (only `src/claude-sdk.ts` does).
- **The 50% compaction advisory reads as an order.** 261002's advisory
  preparation message (`buildPreparationMessage`, `kind: "advisory"`)
  says "prepare for compaction now ... before taking up new work". It is
  delivered as a `followUp` with `triggerTurn` at `agent_end`, so the lead
  wakes right after settling and starts preparing regardless of whether it
  is at a good point. The user wants a light nudge: the lead still notices on
  its own at the settle boundary, but judges whether now is a quiet point.

## Decisions

- **Remove ws-claude entirely.** Every preset (audit, consult, rewrite,
  design-review), its tests, its `index.ts` wiring (registration, session
  start and shutdown, the `setActiveTools` lead addition), and the
  `@anthropic-ai/claude-agent-sdk` dependency with its lockfile entries.
  Drop 260914-bug-ws-claude-timeout-resume-handle. Rejected: keeping it
  behind a flag (the user cannot use it as is, and dead optional code keeps
  its test and dependency cost).
- **Advisory delivery is unchanged.** It stays a `followUp` with
  `triggerTurn` at `agent_end`, so the lead notices without user input; the
  80% hard steer and its wording are unchanged. Rejected: `nextTurn`
  delivery (the lead must notice without the user intervening).
- **Advisory head wording.** Replace the advisory head with this text
  (percent and threshold interpolated as today, the hard point from the
  resolved hard percent), followed by the guide body as today:

  ```text
  Context usage is <P>% of the window (advisory point: <A>%). This is a light
  nudge, not an instruction to stop.

  Consider compacting now if this is a quiet point — for example, you are only
  waiting on background agents, or a piece of work just landed and what comes
  next is weakly related to what you are holding. If you are mid-task or holding
  context that would be costly to rebuild (an unsettled discussion, a
  half-applied change, a diagnosis in progress), keep going and compact at the
  next quiet point instead. You will not be nudged again before the hard point
  (<H>%), where compaction is no longer optional.

  If you decide not to compact now, end this turn without replying. If you do,
  follow the guide below.
  ```
- **The advisory trigger carries the hard percent.** The advisory
  `PreparationTrigger` gains the resolved hard percent and
  `sendPreparation`'s advisory call passes it, so the head can print `<H>`.

## Constraints

- A declined advisory must not leave the preparation state set: the lead
  ending the turn without calling the lever has to re-enable the triggers
  exactly as an unprocessed preparation does today (cleared at `agent_end`).
- The new wording promises no second advisory before the hard point. Keep
  today's behavior, which already makes that true: a declined advisory does
  not fire again while usage stays at or above the advisory point, and
  re-arms only after a compaction or when usage is seen below it
  (`advisoryFired` in `agents-plugin-pi/src/goal-loop.ts`).
- Matching manuals from AGENTS.md `### Implementation Conventions`: none
  (`agents-plugin-pi/` has no declared row).

## Prior Decisions

- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, f065de0d): "The advisory nudge is a followUp with triggerTurn so an idle lead prepares at the run boundary; Pi continues a run for messages queued by agent_end handlers." — bearing: constrains
- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, a5ff0b27): "at agent_end a queued preparation has either run or been dropped; clearing there is exact and needs no timeout" — bearing: constrains
- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, f065de0d): "firing [the hard cut] marks the advisory crossing too, so no weaker nudge follows." — bearing: constrains
- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, Result): "Each fires once per crossing, re-arms after a compaction or when usage is seen below the th[reshold]" — bearing: constrains
- 260908-research-ws-pi-claude-code-lead-provider (.done): "The distinct 260908-feat-ws-pi-claude-delegate-tool owns the Claude Code subprocess-as-subagent direction; the original provider implementation remains parked in todo" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/claude-*.ts, src/index.ts, src/lead-compaction.ts, src/goal-loop.ts, test/claude-*.test.ts, package.json, package-lock.json |
| scope.surface | internal | adapter-internal Pi package; removes the ws-claude lead tool and reworks advisory text, no ws-mcp surface change |
| scope.new_public_symbol | no | none; exports are removed (claude-delegate.ts, claude-sdk.ts, claude-design-review.ts) |
| scope.new_type_contract | yes | the new head interpolates the hard percent, but PreparationTrigger advisory (lead-compaction.ts#L443-L446) carries no hard field; buildPreparationMessage or the trigger type and sendPreparation call site (goal-loop.ts#L1183) must change |
| scope.test_surface | existing | test/lead-compaction.test.ts#L225 pins the old advisory head regex and must be rewritten; claude-* tests are deleted |
| complexity.reuse_points | confirmed | buildPreparationMessage in lead-compaction.ts#L449 and existing resolveCompactionHardPercent are reused |
| complexity.side_effect_risk | moderate | index.ts wiring (setActiveTools lead addition, session start and shutdown) and lockfile edits must be removed cleanly |
| risk.correctness | moderate | the declined-advisory trigger state (advisoryFired, preparation) must stay as today while the head changes |
| risk.fit | low | self-contained removal with an exact replacement text in the ticket |
| risk.test | moderate | deleted tests must not leave dangling imports; test/claude-lifecycle.test.ts is also named by 260921-chore-pi-stabilize-flaky-lifecycle-websearch-tests |
| risk.security_or_contract | low | removes a tool and an SDK dependency; no new input surface |

## Phases

### Phase 1: Remove ws-claude and replace the advisory wording

- Delete the ws-claude sources, tests, wiring, and SDK dependency. Besides
  the files named in Background, `test/mailbox-bootstrap.integration.test.ts`
  (L84) pins the `claudeDelegateSession.shutdown()` line of `index.ts`;
  update it to match the new shutdown sequence. Tickets and other
  `ai-docs/` history outside the package stay as they are.
- Replace the advisory head in `buildPreparationMessage` with the wording
  above; the hard and reroute heads are unchanged.
- Rewrite the advisory assertion in `test/lead-compaction.test.ts` (L225) to
  pin the full head text for fixed `<P>`, `<A>`, and `<H>` values.
- Confirm a test covers the declined-advisory behavior in Constraints (no
  re-fire while usage stays at or above the advisory point; re-arm after a
  compaction or a reading below it). Add one if none does.
- Drop 260914-bug-ws-claude-timeout-resume-handle with `tickets.close`
  (status `dropped`, reason: the ws-claude tool is retired), and name it in
  the commit's `## Ticket Updates`.

Done when:

- `npm test` in `agents-plugin-pi` passes.
- `git grep -i -e ws-claude -e claude-agent-sdk -e claude-delegate -e claudeDelegate -- agents-plugin-pi`
  returns nothing.
- `package.json` and `package-lock.json` no longer carry
  `@anthropic-ai/claude-agent-sdk`.
- A test pins the full advisory head, including the hard-point percent.

### Result (dbd56bd7c) - 2026-10-03

- Removed ws-claude sources, six tests, `index.ts` wiring, and the
  `@anthropic-ai/claude-agent-sdk` dependency; dropped
  260914-bug-ws-claude-timeout-resume-handle.
- Advisory head replaced with the ticket text; the advisory
  `PreparationTrigger` now carries `hardPercent`. Full head pinned in
  `test/lead-compaction.test.ts`; `goal-loop.test.ts` regex updated.
- Declined-advisory behavior (no re-fire, re-arm after compaction or a
  reading below) was already covered by the goal-loop trigger tests; no new test.
- `test/mailbox-bootstrap.integration.test.ts` shutdown anchor moved to
  `persistShutdownAgentSnapshots(...)`, the first remaining shutdown await.
- Verification: `npm test` in `agents-plugin-pi`: 1938 pass, 0 fail, 3 skipped;
  `git grep` for the ws-claude terms returns nothing. Lite review: clean.
- Decision: the lockfile was regenerated by `npm uninstall` (broad churn, also
  stamps version 0.46.26); npm's spurious `bundleDependencies` in package.json
  was reverted.
