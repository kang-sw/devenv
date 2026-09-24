/** Shared replace-by-rename step for the adapter's temp-then-rename durable writes. */
import { renameSync } from "node:fs";

/** Focused deterministic test seam; production callers pass nothing. */
export interface RenameRetryHooks {
  platform?: NodeJS.Platform;
  rename?: typeof renameSync;
  sleep?: (milliseconds: number) => void;
}

const RENAME_ATTEMPTS = 5;
const renameSleeper = new Int32Array(new SharedArrayBuffer(4));

function sleepSync(milliseconds: number): void {
  Atomics.wait(renameSleeper, 0, 0, milliseconds);
}

function isRetryableWindowsRenameError(error: unknown): boolean {
  const code = (error as NodeJS.ErrnoException | undefined)?.code;
  return code === "EPERM" || code === "EBUSY";
}

/**
 * Renames `from` over `to`, retrying the Windows sharing-violation class
 * (EPERM/EBUSY from a concurrent reader) with bounded synchronous backoff.
 * Other platforms and other errors fail on the first attempt. Exhaustion
 * throws `exhausted(attempts, cause)` when given, otherwise the last error.
 */
export function renameWithWindowsRetry(
  from: string,
  to: string,
  hooks: RenameRetryHooks = {},
  exhausted?: (attempts: number, cause: NodeJS.ErrnoException) => Error,
): void {
  const rename = hooks.rename ?? renameSync;
  const platform = hooks.platform ?? process.platform;
  const sleep = hooks.sleep ?? sleepSync;
  for (let attempt = 0; ; attempt += 1) {
    try {
      rename(from, to);
      return;
    } catch (error) {
      if (platform !== "win32" || !isRetryableWindowsRenameError(error)) throw error;
      if (attempt === RENAME_ATTEMPTS - 1) throw exhausted ? exhausted(attempt + 1, error as NodeJS.ErrnoException) : error;
      sleep(10 * 2 ** attempt);
    }
  }
}
