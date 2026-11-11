/**
 * Hosted-session mode: a session the dashboard daemon spawned and feeds. The
 * daemon writes every input (dashboard messages, mailbox mail) as the
 * session's user turns and reads its output itself, so in this mode the
 * lead runs no mailbox waiter and sends no dashboard feed, and the bridge
 * re-logs in with the daemon-minted key instead of minting one.
 *
 * The daemon sets three variables on the harness it spawns: the flag, the
 * key, and the path of the session's registration file, whose `host_pid` is
 * that harness process. Every process below the harness inherits them (a
 * second harness the session starts from its shell, say), so the mode holds
 * only in the process the registration names: see resolveHostedMode. The
 * same variables gate the Claude mod and ws-mcp. The flag without a key, or
 * with a registration that never names a process, gates nothing and only
 * warns.
 */
import { readFile } from "node:fs/promises";

/** Set to a truthy value (`1`, `true`, `yes`, `on`) by the daemon on a session it hosts. */
export const HOSTED_FLAG_ENV = "WS_HOSTED_SESSION";

/** The lead session key the daemon minted for the session it hosts. */
export const HOSTED_KEY_ENV = "WS_HOSTED_SESSION_KEY";

/** The session's registration file, which names the hosted harness process as `host_pid`. */
export const HOSTED_ROW_ENV = "WS_HOSTED_SESSION_ROW";

/**
 * `off`: an ordinary session (no flag, no registration path, or a
 * registration that names another process). `hosted`: this process is the
 * one the daemon hosts. `missing-key`: the flag without a key, and
 * `unconfirmed`: a registration that never named a harness process; both
 * gate nothing and only warn.
 */
export type HostedMode = { mode: "off" } | { mode: "hosted"; key: string } | { mode: "missing-key" } | { mode: "unconfirmed"; reason: string };

/** The truthy spellings ws-mcp's own env flags accept. */
export function flagOn(value: string | undefined): boolean {
  return ["1", "true", "yes", "on"].includes((value ?? "").trim().toLowerCase());
}

/** The mode the flag and the key select, before the registration is read. */
export function readHostedMode(env: NodeJS.ProcessEnv): HostedMode {
  if (!flagOn(env[HOSTED_FLAG_ENV])) return { mode: "off" };
  const key = (env[HOSTED_KEY_ENV] ?? "").trim();
  return key ? { mode: "hosted", key } : { mode: "missing-key" };
}

/**
 * What a registration file's text says of the process `pid`: `mine` when its
 * `host_pid` is that process, `other` when it is another one, and `pending`
 * when it names none yet (zero or absent, or the text is missing or does not
 * parse): the daemon writes the PID right after it starts the harness.
 */
export function rowVerdict(text: string | undefined, pid: number): "mine" | "other" | "pending" {
  let hostPid: unknown;
  try {
    hostPid = (JSON.parse(text ?? "") as { host_pid?: unknown } | null)?.host_pid;
  } catch {
    return "pending";
  }
  if (typeof hostPid !== "number" || !Number.isInteger(hostPid) || hostPid <= 0) return "pending";
  return hostPid === pid ? "mine" : "other";
}

/** How often, and for how long, a registration that names no harness yet is read again. */
export const ROW_POLL_MS = 100;
export const ROW_WAIT_MS = 3_000;

/** What resolveHostedMode reads the world through. */
export type HostedProbe = {
  /** This harness process: the extension runs inside it. */
  pid: number;
  /** The text of the file at `path`; a rejection reads as not there yet. */
  readRow: (path: string) => Promise<string>;
  sleep: (ms: number) => Promise<void>;
};

const processProbe = (): HostedProbe => ({
  pid: process.pid,
  readRow: (path) => readFile(path, "utf8"),
  sleep: (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
});

/**
 * The mode of this process:
 *
 * - flag off: `off`; flag without a key: `missing-key`.
 * - no registration path: `off`.
 * - the registration names this process: `hosted`.
 * - it names another process: `off`, silently (the variables were inherited).
 * - it names none: read again every ROW_POLL_MS for ROW_WAIT_MS, then
 *   `unconfirmed`.
 */
export async function resolveHostedMode(env: NodeJS.ProcessEnv, probe: HostedProbe = processProbe()): Promise<HostedMode> {
  const selected = readHostedMode(env);
  if (selected.mode !== "hosted") return selected;
  const row = (env[HOSTED_ROW_ENV] ?? "").trim();
  if (!row) return { mode: "off" };
  for (let waited = 0; ; waited += ROW_POLL_MS) {
    const verdict = rowVerdict(await probe.readRow(row).catch(() => undefined), probe.pid);
    if (verdict === "mine") return selected;
    if (verdict === "other") return { mode: "off" };
    if (waited >= ROW_WAIT_MS) return { mode: "unconfirmed", reason: `${row} named no harness process within ${ROW_WAIT_MS / 1000} s` };
    await probe.sleep(ROW_POLL_MS);
  }
}

let resolved: Promise<HostedMode> | undefined;

/**
 * This process's mode, resolved at the first call and kept: a process that
 * resolved as an ordinary session never becomes a hosted one later.
 */
export function hostedMode(): Promise<HostedMode> {
  return (resolved ??= resolveHostedMode(process.env));
}

/** The key a hosted session runs under, or undefined outside hosted mode. */
export async function hostedSessionKey(): Promise<string | undefined> {
  const hosted = await hostedMode();
  return hosted.mode === "hosted" ? hosted.key : undefined;
}

/** The warning for a flag set without a key. */
export const MISSING_HOSTED_KEY_WARNING =
  `ws-pi: ${HOSTED_FLAG_ENV} is set but ${HOSTED_KEY_ENV} is missing or empty: running as an ordinary session (own key, mailbox wait, dashboard feed).`;

/** The warning for a flag, key, and registration path this process could not be matched against. */
export function unconfirmedHostedWarning(reason: string): string {
  return `ws-pi: ${HOSTED_FLAG_ENV} is set but ${reason}: running as an ordinary session (own key, mailbox wait, dashboard feed).`;
}
