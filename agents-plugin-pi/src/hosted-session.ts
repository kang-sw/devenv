/**
 * Hosted-session mode: a session the dashboard daemon spawned and feeds. The
 * daemon writes every input (dashboard messages, mailbox mail) as the
 * session's user turns and reads its output itself, so under the flag the
 * lead runs no mailbox waiter and sends no dashboard feed, and the bridge
 * re-logs in with the daemon-minted key instead of minting one. The same two
 * variables gate the Claude mod and ws-mcp. The flag without a key gates
 * nothing and only warns.
 */

/** Set to a truthy value (`1`, `true`, `yes`, `on`) by the daemon on a session it hosts. */
export const HOSTED_FLAG_ENV = "WS_HOSTED_SESSION";

/** The lead session key the daemon minted for the session it hosts. */
export const HOSTED_KEY_ENV = "WS_HOSTED_SESSION_KEY";

export type HostedMode = { mode: "off" } | { mode: "hosted"; key: string } | { mode: "missing-key" };

/** The truthy spellings ws-mcp's own env flags accept. */
export function flagOn(value: string | undefined): boolean {
  return ["1", "true", "yes", "on"].includes((value ?? "").trim().toLowerCase());
}

/** The mode an environment selects. */
export function readHostedMode(env: NodeJS.ProcessEnv): HostedMode {
  if (!flagOn(env[HOSTED_FLAG_ENV])) return { mode: "off" };
  const key = (env[HOSTED_KEY_ENV] ?? "").trim();
  return key ? { mode: "hosted", key } : { mode: "missing-key" };
}

/** The key a hosted session runs under, or undefined outside hosted mode. */
export function hostedSessionKey(env: NodeJS.ProcessEnv): string | undefined {
  const hosted = readHostedMode(env);
  return hosted.mode === "hosted" ? hosted.key : undefined;
}

/** The warning for a flag set without a key. */
export const MISSING_HOSTED_KEY_WARNING =
  `ws-pi: ${HOSTED_FLAG_ENV} is set but ${HOSTED_KEY_ENV} is missing or empty: running as an ordinary session (own key, mailbox wait, dashboard feed).`;
