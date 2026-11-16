import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  type GoalLoopConfig,
} from "../../src/goal-loop.ts";
import { staticConfigReader, type GoalLoopConfigReader } from "../../src/adapter-config.ts";
import { renderLeadProse } from "../../src/lead-compaction.ts";

/** First line of every adapter-injected user message (261007). */
export const LABEL = "[system message from ws-pi-plugin]";

/** `text` as the adapter sends it: under the label line. */
export const labeled = (text: string): string => `${LABEL}\n${text}`;

/** The lever's rendered prose for a single `current_work` field — the carry the old lever passed raw. */
export const prose = (text: string): string => renderLeadProse({ current_work: text });

/** A `session_before_compact` event with Pi's preparation fields the lead handler reads. */
export function compactionEvent(reason: "manual" | "threshold" | "overflow", branchEntries: unknown[] = [], extra: Record<string, unknown> = {}): never {
  return {
    type: "session_before_compact",
    reason,
    branchEntries,
    willRetry: false,
    signal: new AbortController().signal,
    preparation: {
      firstKeptEntryId: "kept-entry",
      tokensBefore: 1234,
      messagesToSummarize: [],
      turnPrefixMessages: [],
      isSplitTurn: false,
      fileOps: { read: new Set(["/tmp/read.ts"]), written: new Set(), edited: new Set(["/tmp/edit.ts"]) },
      settings: { enabled: true, reserveTokens: 16384, keepRecentTokens: 20000 },
    },
    ...extra,
  } as never;
}

export const tmpDir = mkdtempSync(join(tmpdir(), "ws-goal-loop-test-"));

export function writeConfig(name: string, contents: string): string {
  const path = join(tmpDir, name);
  writeFileSync(path, contents, "utf8");
  return path;
}

/**
 * A synchronous reader over a JSON file, re-read at each use: the test
 * stand-in for ws-mcp's store, so rewriting the file mid-test is a "tuned
 * value" the next read sees. A missing or malformed file reads as nothing
 * tuned, as an unreachable ws-mcp does.
 */
export function fileReader(path: string): GoalLoopConfigReader {
  return (keys) => {
    let config: GoalLoopConfig = {};
    try {
      config = JSON.parse(readFileSync(path, "utf8")) as GoalLoopConfig;
    } catch {
      // nothing tuned
    }
    return staticConfigReader(config)(keys);
  };
}
