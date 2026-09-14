---
title: Pi cannot resume a settled execute-worker when its owned session home is unavailable
---

# Pi cannot resume a settled execute-worker when its owned session home is unavailable

## Background

After an `ws-execute` worker successfully settled, `ws-agent-send` to the same registered agent failed with `ws-pi-agent: owned session home is unavailable during resume`. The Pi lead guide promises that sending to a dormant agent transparently resumes it from its cached session file, and does not exclude execute-workers.

This forced the lead to spawn a replacement execute-worker for the second half of one stash/restore operation. Determine whether execute-worker session homes are being removed too early, whether the resume contract intentionally differs for this worker class, or whether the lead guide overstates support.

## Phases

### Phase 1: Restore or correct execute-worker continuation

Reproduce the settled execute-worker continuation failure, identify the lifetime mismatch, and either make dormant execute-workers resumable as documented or narrow the exposed contract consistently. Add regression coverage for the chosen behavior.
