// Shared timeout wrapper for daemon-facing fetch calls.
//
// A bare `fetch()` never settles if the daemon accepts the connection but
// stalls before sending a response body (the exact condition a git
// index-lock stall can induce — see
// `260724-bug-dashboard-git-diff-index-lock-stuck-activity-badge` Phase 2).
// Several pollers hold an `inFlight` boolean guard that is only cleared in
// the fetch promise's `.finally`, which never runs if the promise never
// settles — so the poller wedges permanently at "loading", with no retry.
// Wrapping the fetch in a bounded `AbortController` timeout guarantees the
// promise always settles (by rejecting with `AbortError`), letting the
// existing `.finally`/`.catch` handlers release the guard and recover.
export const DEFAULT_FETCH_TIMEOUT_MS = 8_000;

/**
 * Drop-in replacement for `fetch` that aborts and rejects after `timeoutMs`
 * if the request has not otherwise settled. A caller-supplied `init.signal`
 * is preserved (merged, not clobbered) so an unmount/cancel abort still
 * takes effect alongside the timeout abort.
 */
export async function fetchWithTimeout(
  input: RequestInfo | URL,
  init: RequestInit = {},
  timeoutMs: number = DEFAULT_FETCH_TIMEOUT_MS,
): Promise<Response> {
  const controller = new AbortController();
  const signal = init.signal
    ? AbortSignal.any([init.signal, controller.signal])
    : controller.signal;
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(input, { ...init, signal });
  } finally {
    clearTimeout(timer);
  }
}
