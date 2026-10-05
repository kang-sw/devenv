/**
 * Compensates for host Pi's `RpcClient`, which appends every child stderr
 * chunk to a private `stderr` string for the client's whole lifetime (and
 * re-echoes it to its own stderr, so grandchildren's output also accumulates
 * in every ancestor's client). Only the exit/timeout Error messages read it,
 * and they only need the tail. This attaches a second `data` listener — run
 * after the host's own, since it is registered after `start()` — that trims
 * the string back to its last `RPC_STDERR_TAIL_BYTES` characters.
 *
 * Reaches into private fields through a narrow cast; any shape difference
 * (host version drift) makes this a silent no-op.
 */

export const RPC_STDERR_TAIL_BYTES = 64 * 1024;

interface StderrCapShape {
  process?: { stderr?: { on?: (event: string, listener: () => void) => unknown } | null };
  stderr?: unknown;
}

/** Call right after `RpcClient.start()` resolves. Returns whether a listener was attached. */
export function capRpcClientStderr(client: unknown, limit = RPC_STDERR_TAIL_BYTES): boolean {
  const shape = client as StderrCapShape | null | undefined;
  const stream = shape?.process?.stderr;
  if (!shape || typeof stream?.on !== "function" || typeof shape.stderr !== "string") return false;
  stream.on("data", () => {
    const current = shape.stderr;
    if (typeof current !== "string" || current.length <= limit) return;
    // slice() of a long concatenated (rope) string can pin the whole parent; a flat copy releases it.
    shape.stderr = Buffer.from(current.slice(-limit)).toString();
  });
  return true;
}
