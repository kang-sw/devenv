import { test, after } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync, readFileSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { LEAD_MEMORY_LOG_ENV, LEAD_MEMORY_LOG_FILE, registerLeadMemoryLog } from "../src/lead-memory-log.ts";
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

async function startWithDefaultFile(agentDir: string, flag: string | undefined): Promise<void> {
  const prevDir = process.env.PI_CODING_AGENT_DIR;
  const prevFlag = process.env[LEAD_MEMORY_LOG_ENV];
  process.env.PI_CODING_AGENT_DIR = agentDir;
  if (flag === undefined) delete process.env[LEAD_MEMORY_LOG_ENV]; else process.env[LEAD_MEMORY_LOG_ENV] = flag;
  try {
    const handlers = new Map<string, () => Promise<void>>();
    registerLeadMemoryLog({ on: (e: string, h: () => Promise<void>) => handlers.set(e, h) } as unknown as ExtensionAPI);
    await handlers.get("session_start")!();
    await handlers.get("session_shutdown")!();
  } finally {
    if (prevDir === undefined) delete process.env.PI_CODING_AGENT_DIR; else process.env.PI_CODING_AGENT_DIR = prevDir;
    if (prevFlag === undefined) delete process.env[LEAD_MEMORY_LOG_ENV]; else process.env[LEAD_MEMORY_LOG_ENV] = prevFlag;
  }
}

test("the default agent-dir log is off unless WS_PI_LEAD_MEMORY_LOG=1", async () => {
  const off = join(dir, "agent-off");
  await startWithDefaultFile(off, undefined);
  assert.equal(existsSync(join(off, LEAD_MEMORY_LOG_FILE)), false);
  const on = join(dir, "agent-on");
  await startWithDefaultFile(on, "1");
  assert.equal(readFileSync(join(on, LEAD_MEMORY_LOG_FILE), "utf8").trim().split("\n").length, 1);
});
