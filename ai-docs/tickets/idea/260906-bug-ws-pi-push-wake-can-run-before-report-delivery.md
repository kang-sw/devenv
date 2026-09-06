---
title: Pi push wake can run the model before the waiting report is delivered
related:
  260906-bug-ws-pi-goal-reminder-races-child-push-at-settle: follow-up finding from Phase 2
---

# Pi Push Wake Can Run Before Report Delivery

## Background

Read-only investigation during owner dogfooding found an untested cost and
ordering concern in the user-preflight wake introduced by `438f2f0b`. The wake
line says `N ws messages waiting; process the incoming reports.` Reports are
then flushed at confirmed `agent_start` with their recorded delivery modes.

Installed Pi 0.85.1 source checks the steering queue before a model response,
but checks the follow-up queue after the current work finishes. A held report
using `followUp` can therefore arrive only after the wake has already caused
a model response, potentially including tool work. The effect is not necessarily
just one added input line. The earlier ticket's immediate-report explanation
and fake-loop tests do not establish delivery before the first model response.

Evidence: `agents-plugin-pi/src/spawner.ts` user wake and confirmed-start flush;
installed `pi-agent-core/dist/agent-loop.js` steering/follow-up queue ordering.
This is source-level evidence, not a measured provider trace. The owner suspects
this contributes substantially to usage; its actual share and cache impact
remain unmeasured. Earlier independent implementation reviews missed this gap.

The preflight also repairs a real, separate defect: a direct idle custom-message
run bypasses `before_agent_start`, and Pi can restore its base system prompt
on the second model iteration, dropping ws's system-prompt extension. Do not
assume removing the wake alone safely fixes both issues.

## Phases

### Phase 1: Validate actual delivery ordering and settle a bounded correction

Reproduce wake plus a held follow-up report against Pi's real loop or a faithful
integration harness, observing the first model request, report arrival, and
extra responses/tool work. Measure rather than infer the usage impact. Use the
result to discuss a correction with the owner before source changes; no delivery
mode change or replacement architecture has been approved. Keep system-prompt
continuity and existing hold/priority requirements explicit in that discussion.

Capture only: the owner stopped goal draining and authorized finishing only the
already-running tier-warning phase and its review. This follow-up is not ready
for implementation and must not restart the drain.
