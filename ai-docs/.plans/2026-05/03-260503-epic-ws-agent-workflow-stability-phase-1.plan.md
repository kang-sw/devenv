# Plan: ws agent workflow stability Phase 1

## Implementation Steps

1. Add a process-tree cancellation abstraction in `internal/wsagent`.
   - Unix: kill the async worker process group using negative PID, matching the
     `Setpgid` setup in `configureAsyncCommand`.
   - Windows: keep a conservative PID kill fallback for now.
   - Route `Manager.Cancel` through the abstraction and append runtime events
     for attempted, succeeded, failed, and cleanup-needed outcomes.

2. Make status text lifecycle-aware without breaking current consumers.
   - Keep existing `agent_status`, `call_status`, `pid`, and timestamp lines.
   - Add explicit `active`, `cleanup_needed`, `output_path`,
     `runtime_log_path`, and `follow_up` lines where they are useful.
   - Preserve completed/failed/cancelled state reconciliation before printing.

3. Make wait timeout output unambiguous.
   - Append a runtime event on timeout.
   - Return a bounded text response that says the call is still active when it
     is active and lists safe follow-up commands.
   - Do not mark the call failed just because the host-side wait timed out.

4. Expand tests.
   - Unit-test timeout output and runtime diagnostics.
   - Unit-test cancel success, cancel failure, and cleanup-needed reporting with
     injected process cancellation and liveness behavior.
   - Keep existing async completion, dead-worker reconciliation, and panic tests
     passing.

5. Verify.
   - `cd agents-plugin-tool && go test ./...`
   - `python3 -m json.tool agents-plugin/runtime.json >/dev/null`
   - `claude plugin validate agents-plugin`
   - `git diff --check`

## Review Notes

This slice should not try to solve all context compression. The target is that a
lead can distinguish "still running, wait again later" from "failed",
"cancelled", or "cleanup needed" without reading full tail output.
