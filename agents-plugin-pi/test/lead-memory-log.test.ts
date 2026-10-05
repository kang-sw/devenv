import { test, after } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync, readFileSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { registerLeadMemoryLog } from "../src/lead-memory-log.ts";
import { WS_PI_SPAWN_ROLE_ENV } from "../src/process-role.ts";

const dir = mkdtempSync(join(tmpdir(), "ws-memlog-test-"));
after(() => rmSync(dir, { recursive: true, force: true }));

function fixture(file: string) {
  const handlers = new Map<string, () => Promise<void>>();
  registerLeadMemoryLog({ on: (e: string, h: () => Promise<void>) => handlers.set(e, h) } as unknown as ExtensionAPI, file);
  return handlers;
}

test("lead logs one sample immediately and stops on shutdown; repeat start does not stack", async () => {
  const file = join(dir, "lead.jsonl");
  const h = fixture(file);
  await h.get("session_start")!();
  await h.get("session_start")!();
  const lines = readFileSync(file, "utf8").trim().split("\n");
  assert.equal(lines.length, 1);
  const row = JSON.parse(lines[0]);
  assert.deepEqual(Object.keys(row), ["t", "pid", "rss", "heapUsed", "heapTotal", "external", "arrayBuffers"]);
  await h.get("session_shutdown")!();
});

test("child roles do not log", async () => {
  const file = join(dir, "child.jsonl");
  const prev = process.env[WS_PI_SPAWN_ROLE_ENV];
  process.env[WS_PI_SPAWN_ROLE_ENV] = "worker";
  try {
    await fixture(file).get("session_start")!();
  } finally {
    if (prev === undefined) delete process.env[WS_PI_SPAWN_ROLE_ENV]; else process.env[WS_PI_SPAWN_ROLE_ENV] = prev;
  }
  assert.equal(existsSync(file), false);
});
