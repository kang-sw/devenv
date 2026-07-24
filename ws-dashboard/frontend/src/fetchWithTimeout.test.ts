import { DEFAULT_FETCH_TIMEOUT_MS, fetchWithTimeout } from "./fetchWithTimeout.js";
import { createResourceRefreshCoordinator } from "./resourceRefresh.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
  }
}

async function assertRejectsWithAbort(
  action: () => Promise<unknown>,
  label: string,
) {
  try {
    await action();
  } catch (error) {
    const name = error instanceof DOMException ? error.name : undefined;
    if (name !== "AbortError") {
      throw new Error(`${label}: expected AbortError, got ${String(error)}`);
    }
    return;
  }
  throw new Error(`${label}: expected rejection`);
}

// A realistic fetch stand-in: it settles only when the request signal is
// aborted (rejecting with the same AbortError shape a real aborted `fetch`
// produces), and otherwise never settles. This is the non-vacuous half of
// the contract — if `fetchWithTimeout` stopped wiring the abort signal
// through to the underlying `fetch` call, this mock would never reject and
// every test below built on it would hang instead of passing.
function neverSettlesUnlessAborted(): typeof fetch {
  return (async (_input, init) => {
    return new Promise<Response>((_resolve, reject) => {
      const signal = init?.signal;
      if (!signal) {
        return;
      }
      if (signal.aborted) {
        reject(new DOMException("Aborted", "AbortError"));
        return;
      }
      signal.addEventListener("abort", () => {
        reject(new DOMException("Aborted", "AbortError"));
      });
    });
  }) as typeof fetch;
}

const originalFetch = globalThis.fetch;
const originalSetTimeout = globalThis.setTimeout;
const originalClearTimeout = globalThis.clearTimeout;
try {
  assertEqual(
    DEFAULT_FETCH_TIMEOUT_MS,
    8_000,
    "default timeout stays bounded at ~8s",
  );

  // Happy path: a fast response passes straight through untouched.
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ ok: true }), {
      status: 200,
      headers: { "content-type": "application/json" },
    })) as typeof fetch;
  const fastResponse = await fetchWithTimeout("/api/fast", {}, 50);
  assertEqual(fastResponse.status, 200, "happy-path response passes through unchanged");
  assertEqual(
    (await fastResponse.json()).ok,
    true,
    "happy-path response body is unaffected by the timeout wrap",
  );

  // A fast response must clear its timer rather than leaving it dangling.
  {
    let setCount = 0;
    let clearCount = 0;
    globalThis.setTimeout = ((fn: (...args: unknown[]) => void, ms?: number) => {
      setCount += 1;
      return originalSetTimeout(fn, ms);
    }) as typeof setTimeout;
    globalThis.clearTimeout = ((handle: Parameters<typeof clearTimeout>[0]) => {
      clearCount += 1;
      return originalClearTimeout(handle);
    }) as typeof clearTimeout;

    await fetchWithTimeout("/api/fast", {}, 50);

    assertEqual(setCount, 1, "each call arms exactly one timeout timer");
    assertEqual(
      clearCount,
      1,
      "a fast response clears its timeout timer in `finally` so no dangling timer leaks",
    );
    globalThis.setTimeout = originalSetTimeout;
    globalThis.clearTimeout = originalClearTimeout;
  }

  // Stall: the underlying fetch never resolves on its own. Non-vacuous per
  // the helper above — this can only reject because `fetchWithTimeout`
  // itself aborts after `timeoutMs`.
  globalThis.fetch = neverSettlesUnlessAborted();
  {
    const startedAt = Date.now();
    await assertRejectsWithAbort(
      () => fetchWithTimeout("/api/slow", {}, 20),
      "a stalled fetch rejects with AbortError once the bounded timeout elapses",
    );
    const elapsedMs = Date.now() - startedAt;
    if (elapsedMs < 20) {
      throw new Error(
        `fetchWithTimeout aborted before its ${20}ms timeout elapsed (after ${elapsedMs}ms)`,
      );
    }
    if (elapsedMs > 2_000) {
      throw new Error(
        `fetchWithTimeout took ${elapsedMs}ms to abort a 20ms-timeout request — timer likely not wired`,
      );
    }
  }

  // Signal merge: a caller-supplied signal must remain effective alongside
  // the timeout's own controller, not get clobbered by it.
  globalThis.fetch = neverSettlesUnlessAborted();
  {
    const callerController = new AbortController();
    const startedAt = Date.now();
    const promise = fetchWithTimeout(
      "/api/slow",
      { signal: callerController.signal },
      5_000,
    );
    setTimeout(() => callerController.abort(), 10);
    await assertRejectsWithAbort(
      () => promise,
      "caller-supplied signal still aborts the request alongside the timeout signal",
    );
    const elapsedMs = Date.now() - startedAt;
    if (elapsedMs > 1_000) {
      throw new Error(
        `caller signal abort took ${elapsedMs}ms — fetchWithTimeout's internal 5000ms timeout fired instead of the merged caller signal`,
      );
    }
  }

  // Integration proof: routed through the highest-severity deadlock-prone
  // poller family (resourceRefresh's coordinator), a stalled fetch's
  // `inFlight` guard is released once the wrap times out, and a subsequent
  // refresh is accepted rather than permanently skipped. Non-vacuous: with
  // a bare `fetch()` in place of `fetchWithTimeout`, the stalled mock above
  // never settles, `refresh("poll")` never resolves, and this block hangs
  // rather than completing.
  globalThis.fetch = neverSettlesUnlessAborted();
  {
    const coordinator = createResourceRefreshCoordinator({
      fetchResources: () =>
        fetchWithTimeout("/api/dashboard/resources", {}, 20).then(
          (response) => response.json(),
        ),
      applyResources: () => {},
    });

    assertEqual(coordinator.isInFlight(), false, "coordinator starts idle");
    const firstResult = coordinator.refresh("poll");
    assertEqual(
      coordinator.isInFlight(),
      true,
      "coordinator marks inFlight while the timeout-bound fetch is outstanding",
    );

    const result = await firstResult;
    assertEqual(
      result.status,
      "failed",
      "a stalled fetch times out into a failed refresh result instead of hanging forever",
    );
    assertEqual(
      coordinator.isInFlight(),
      false,
      "inFlight guard is released once the wrapped fetch times out and its `.finally` runs",
    );

    const secondResult = coordinator.refresh("poll");
    assertEqual(
      coordinator.isInFlight(),
      true,
      "a refresh issued after the timeout-release is accepted rather than skipped, proving the guard was truly released for reuse",
    );
    await secondResult;
  }
} finally {
  globalThis.fetch = originalFetch;
  globalThis.setTimeout = originalSetTimeout;
  globalThis.clearTimeout = originalClearTimeout;
}
