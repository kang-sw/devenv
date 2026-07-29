import { expect } from "@playwright/test";
import { readFileSync } from "node:fs";
import path from "node:path";

// Shared agent turn-state test helpers, lifted out of
// `agent-attention-indicator.spec.ts` when `agent-attention-notification.spec.ts`
// needed the same two operations. Both were module-local there and closed over
// that file's `stateHome` / `daemon.baseUrl`; here the closed-over values are
// leading parameters instead, and each spec binds them back with a thin
// module-local wrapper so its own call sites stay unchanged.
//
// Sibling of `daemonHarness.ts`, and deliberately NOT a `*.spec.ts`: Playwright
// would otherwise collect it as a test file with zero tests.

// Reads the daemon-written per-terminal callback token straight off disk.
// Never over HTTP (there is no route that serves it, by design), never
// logged, never echoed into an assertion message. Path construction mirrors
// `agent_token_store.rs::token_store_path`.
export function readCallbackToken(
  stateHome: string,
  terminalId: string,
): string {
  const tokenPath = path.join(
    stateHome,
    "terminal-tokens",
    `${terminalId}.json`,
  );
  const parsed = JSON.parse(readFileSync(tokenPath, "utf8")) as {
    token?: string;
  };
  expect(
    typeof parsed.token === "string" && parsed.token.length > 0,
    "the hooked test profile must have made spawn mint a callback token",
  ).toBe(true);
  return parsed.token as string;
}

// Drives the Phase 4 callback route the way a vendor hook would: from
// OUTSIDE the browser, with no owner session cookie (that route is
// registered outside `require_owner_auth` and is authorized by the
// per-terminal token alone).
export async function postTurnState(
  baseUrl: string,
  terminalId: string,
  token: string,
  state: "working" | "ready" | "idle",
) {
  const response = await fetch(
    new URL(`/api/dashboard/terminals/${terminalId}/turn-state`, baseUrl),
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token, state }),
    },
  );
  expect(
    response.status,
    `turn-state POST for state '${state}' must be accepted`,
  ).toBe(204);
}
