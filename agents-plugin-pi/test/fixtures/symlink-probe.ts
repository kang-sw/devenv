/**
 * Whether this process may create symbolic links. Windows refuses
 * `symlinkSync` with EPERM without Developer Mode or the symlink privilege, so
 * a test that needs a real link skips there instead of failing. Any other
 * probe failure is rethrown: only a missing privilege is a reason to skip.
 *
 * Use as a node:test option: `test(name, { skip: symlinkSkip() }, fn)`.
 *
 * `node --test` also runs this file (default glob); it defines no tests.
 */
import { mkdirSync, mkdtempSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

let probed: boolean | undefined;

/** Probes once per process: one directory link, created without a type as most callers do. */
export function symlinksAvailable(): boolean {
  if (probed !== undefined) return probed;
  const root = mkdtempSync(join(tmpdir(), "ws-pi-symlink-probe-"));
  try {
    mkdirSync(join(root, "target"));
    symlinkSync(join(root, "target"), join(root, "link"));
    probed = true;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "EPERM") throw error;
    probed = false;
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
  return probed;
}

/** The node:test `skip` option: `false` when links work, otherwise the reason. */
export function symlinkSkip(): string | false {
  return symlinksAvailable() ? false : "symlink creation is not permitted here (EPERM; on Windows, enable Developer Mode)";
}
