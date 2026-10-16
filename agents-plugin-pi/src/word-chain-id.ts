/**
 * Readable word-chain ids: three random words from the EFF large wordlist
 * joined by `-` (e.g. `amber-tide-fox`), the same shape every other ws
 * readable stem has. A TypeScript port of the ws tooling's Go generator, with
 * a byte-identical copy of its wordlist beside this file
 * (`eff_large_wordlist.txt`, shipped through the package's `src/` entry).
 *
 * The id format is opaque to callers; uniqueness is the caller's job.
 */

import { randomInt } from "node:crypto";
import { readFileSync } from "node:fs";

let wordPool: readonly string[] | undefined;

/** The parsed wordlist, read once on first use. */
export function wordChainPool(): readonly string[] {
  wordPool ??= readFileSync(new URL("./eff_large_wordlist.txt", import.meta.url), "utf8")
    .split("\n")
    .map((line) => line.trim())
    .filter((word) => word.length > 0);
  return wordPool;
}

/** Mints one three-word id; `pick(n)` returns an index in `[0, n)` and defaults to a crypto-random one. */
export function generateWordChainId(pick: (n: number) => number = randomInt): string {
  const pool = wordChainPool();
  return [0, 1, 2].map(() => pool[pick(pool.length)]).join("-");
}
