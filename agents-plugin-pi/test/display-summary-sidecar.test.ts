import { test } from "node:test";
import assert from "node:assert/strict";
import * as fs from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { SessionEntry } from "@earendil-works/pi-coding-agent";
import { createSummarySidecar, DISPLAY_SUMMARY_SIDECAR_SUFFIX } from "../src/display-summary-sidecar.ts";

const summary = { toolIntention: "inspect", toolResult: "found" };
const toolNames = new Set(["read"]);
const entry = (id: string): SessionEntry => ({ type: "message", id: `entry-${id}`, parentId: null, timestamp: "now", message: {
  role: "toolResult", toolCallId: id, toolName: "read", content: [], isError: false, timestamp: 0,
} });

async function fixture(t: { after(fn: () => Promise<void>): void }, saved = true) {
  const directory = await fs.mkdtemp(join(tmpdir(), "ws-display-sidecar-"));
  t.after(() => fs.rm(directory, { recursive: true, force: true }));
  const sessionFile = join(directory, "lead.jsonl");
  const entries = [entry("a"), entry("b")];
  const save = () => fs.writeFile(sessionFile, [JSON.stringify({ type: "session", id: "lead" }), ...entries.map((e) => JSON.stringify(e))].join("\n") + "\n");
  if (saved) await save();
  return { sessionFile, sidecarFile: sessionFile + DISPLAY_SUMMARY_SIDECAR_SUFFIX, entries, save };
}

test("accepted batches append incrementally and reopen restores renderer IDs", async (t) => {
  const f = await fixture(t);
  const deps = { conversation: { sessionId: "lead", sessionFile: f.sessionFile, entries: () => f.entries }, toolNames, restore: () => {} };
  const cache = createSummarySidecar(deps);
  await cache.ready;
  cache.accept(new Map([["a", summary]]));
  await cache.drain();
  const first = await fs.readFile(f.sidecarFile, "utf8");
  cache.accept(new Map([["b", { ...summary, toolResult: "second" }]]));
  await cache.drain();
  const second = await fs.readFile(f.sidecarFile, "utf8");
  assert.ok(second.startsWith(first));
  assert.equal(second.trim().split("\n").length, 2);
  assert.equal(JSON.parse(first).sessionId, "lead");
  assert.equal(JSON.parse(first).id, "a", "not entry-a");
  let restored = new Map();
  await createSummarySidecar({ ...deps, restore: (batch) => { restored = new Map(batch); } }).ready;
  assert.deepEqual(restored.get("a"), summary);
  assert.equal(restored.get("b").toolResult, "second");
});

test("first save backfills accepted memory; observed deletion never recreates the sidecar", async (t) => {
  const f = await fixture(t, false);
  const cache = createSummarySidecar({ conversation: { sessionId: "lead", sessionFile: f.sessionFile, entries: () => f.entries }, toolNames, restore: () => {} });
  await cache.ready;
  cache.accept(new Map([["a", summary]]));
  await cache.drain();
  await assert.rejects(fs.lstat(f.sidecarFile), { code: "ENOENT" });
  assert.deepEqual(cache.snapshot().get("a"), summary);
  await f.save();
  await cache.drain();
  assert.deepEqual(JSON.parse(await fs.readFile(f.sidecarFile, "utf8")).summary, summary);
  await fs.unlink(f.sessionFile);
  await fs.unlink(f.sidecarFile);
  cache.accept(new Map([["b", summary]]));
  await cache.drain();
  await f.save();
  await cache.drain();
  await assert.rejects(fs.lstat(f.sidecarFile), { code: "ENOENT" });
  assert.deepEqual(cache.snapshot().get("b"), summary);
});
