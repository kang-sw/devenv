---
title: ws-execute workers cannot reach their approval execution tool
---

# ws-execute workers cannot reach their approval execution tool

## Background

Pi dogfooding could not exercise the approval-card path because an agent spawned through `ws-execute` reported that `ws-worker-exec` was not exposed in its session. The lead-facing `ws-execute` call succeeded and the child started, but it settled without issuing an approval request. This was reproduced while asking the worker to propose only `printf 'approval-card-probe\n'`.

The observation does not establish whether the defect is in worker tool-profile construction, runtime compatibility, or another adapter boundary. Preserve that uncertainty during investigation.

## Phases

### Phase 1: Restore the execute-worker approval path

Trace execute-worker session creation and custom-tool registration, then ensure a `ws-execute` child can invoke its gated execution tool and deliver a `ws-agent-approval` request to the lead. Preserve the command-by-command approval contract: no command runs before an explicit matching `ws-approve` decision, and stale or mismatched command IDs remain rejected.

Verification must cover tool exposure in the spawned worker, a harmless approved command, denial, `run-instead`, stale command IDs, worker completion after approval, and a live Pi approval-card check. Include diagnostics that distinguish profile/tool-registration failure from an unavailable external ws-mcp runtime.
