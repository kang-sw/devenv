/**
 * Live gate for src/claude-code-provider.ts
 * (260908-feat-ws-pi-claude-code-lead-provider, Phase 1). Skipped unless
 * `WS_PI_LIVE_CLAUDE=1`: it drives a real `pi -p` (isolated: no other
 * extensions, skills, context files, prompt templates or built-in tools) with
 * this extension on `claude-code/sonnet` against the owner's subscription
 * login, so it costs subscription usage and several minutes of wall clock.
 *
 * Checks, in one Pi session per run:
 *   1. the spike's two-tool prompt exits 0 with both tool strings in the
 *      answer, three assistant calls (toolUse, toolUse, stop) and ONE claude
 *      process (one `claude process ready` log line);
 *   2. the first tool parks its MCP handler for at least three minutes
 *      (`WS_PI_LIVE_WAIT_MS`, default 185000) before Pi returns the result,
 *      and the handler still resolves (no MCP tool-call timeout);
 *   3. a second, identical run reports `cacheRead > 0` on its first
 *      assistant call (Anthropic-side prompt cache across claude processes).
 *
 * Run: WS_PI_LIVE_CLAUDE=1 node --test test/claude-code-provider.live.integration.test.ts
 * (wrap in a hard timeout; `WS_PI_LIVE_WAIT_MS=0` for a quick smoke run).
 */

import { test } from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const live = process.env.WS_PI_LIVE_CLAUDE === "1";
const waitMs = Number(process.env.WS_PI_LIVE_WAIT_MS ?? 185_000);
const providerPath = join(dirname(fileURLToPath(import.meta.url)), "..", "src", "claude-code-provider.ts");
const PROMPT = "Do exactly this, in order, without commentary: 1) call extra_a. 2) call extra_b with word=\"kiwi\". 3) reply with the two strings the tools returned, on one line.";

function extensionSource(logPath: string): string {
  return `import { appendFileSync } from "node:fs";
import { registerClaudeCodeProvider } from ${JSON.stringify(providerPath)};
const LOG = ${JSON.stringify(logPath)};
const log = (line) => appendFileSync(LOG, new Date().toISOString() + " " + line + "\\n");
const WAIT_MS = Number(process.env.WS_PI_LIVE_WAIT_MS ?? "0");
export default function (pi) {
  registerClaudeCodeProvider(pi, { log });
  pi.registerTool({
    name: "extra_a", label: "extra_a", description: "Live-gate tool A. Returns a fixed token.",
    parameters: { type: "object", properties: {} },
    execute: async () => {
      log("extra_a: parking Pi side for " + WAIT_MS + "ms");
      await new Promise((r) => setTimeout(r, WAIT_MS));
      log("extra_a: returning");
      return { content: [{ type: "text", text: "extra_a says: alpha-7731" }] };
    },
  });
  pi.registerTool({
    name: "extra_b", label: "extra_b", description: "Live-gate tool B. Echoes its argument.",
    parameters: { type: "object", properties: { word: { type: "string", description: "A word to echo." } }, required: ["word"] },
    execute: async (_id, params) => ({ content: [{ type: "text", text: "extra_b echoes: " + params.word }] }),
  });
  pi.on("session_start", async (_event, ctx) => {
    pi.setActiveTools(["extra_a", "extra_b"]);
    const model = ctx.modelRegistry.getAll().find((m) => m.provider === "claude-code" && m.id === "sonnet");
    log("hasConfiguredAuth(claude-code/sonnet)=" + (model ? ctx.modelRegistry.hasConfiguredAuth(model) : "model-missing"));
  });
}
`;
}

interface AssistantRecord { stopReason: string; usage: { input: number; output: number; cacheRead: number; cacheWrite: number }; content: unknown[] }

function readAssistantRecords(sessionDir: string): AssistantRecord[] {
  const files = readdirSync(sessionDir).filter(f => f.endsWith(".jsonl")).map(f => join(sessionDir, f));
  assert.equal(files.length, 1, "exactly one session file per run");
  const records: AssistantRecord[] = [];
  for (const line of readFileSync(files[0], "utf8").split("\n")) {
    if (!line) continue;
    let entry: { message?: AssistantRecord & { role?: string } };
    try { entry = JSON.parse(line); } catch { continue; }
    if (entry.message?.role === "assistant") records.push(entry.message);
  }
  return records;
}

// The cwd is shared by both runs: Claude Code folds it into the prompt
// prefix, so a per-run cwd would defeat the cross-process cache check.
async function runPi(root: string, label: string, wait: number): Promise<{ code: number | null; stdout: string; stderr: string; log: string; records: AssistantRecord[]; ms: number }> {
  const cwd = join(root, "cwd");
  const sessionDir = join(root, `${label}-sessions`);
  const logPath = join(root, `${label}-provider.log`);
  mkdirSync(cwd, { recursive: true }); mkdirSync(sessionDir);
  writeFileSync(logPath, "");
  const extPath = join(root, `${label}-ext.ts`);
  writeFileSync(extPath, extensionSource(logPath));
  const env = { ...process.env, WS_PI_LIVE_WAIT_MS: String(wait) };
  for (const key of ["ANTHROPIC_API_KEY", "WS_MCP_BOOTSTRAP_BINARY", "WS_MCP_BOOTSTRAP_URL", "WS_PI_SPAWN_ROLE", "WS_PI_EXPLORE_MODE"]) delete env[key];
  const started = Date.now();
  const child = spawn("pi", ["-p", "--no-extensions", "--no-skills", "--no-context-files", "--no-prompt-templates", "--no-builtin-tools", "-e", extPath, "--session-dir", sessionDir, "--model", "claude-code/sonnet", PROMPT], { cwd, env, stdio: ["ignore", "pipe", "pipe"] });
  let stdout = ""; let stderr = "";
  child.stdout.on("data", d => { stdout += d; });
  child.stderr.on("data", d => { stderr += d; });
  const killer = setTimeout(() => child.kill("SIGKILL"), wait + 240_000);
  const code = await new Promise<number | null>(resolve => child.on("exit", resolve));
  clearTimeout(killer);
  return { code, stdout, stderr, log: readFileSync(logPath, "utf8"), records: readAssistantRecords(sessionDir), ms: Date.now() - started };
}

test("live: claude-code/sonnet two-tool round trip, parked handler survives a three-minute wait, second run hits the prompt cache", { skip: !live && "set WS_PI_LIVE_CLAUDE=1 to run the live gate" }, async () => {
  const root = mkdtempSync(join(tmpdir(), "ws-pi-claude-code-live-"));
  try {
    const first = await runPi(root, "first", waitMs);
    console.log(`first run: exit ${first.code} in ${first.ms}ms\nstdout: ${first.stdout.trim()}\nlog:\n${first.log}`);
    assert.equal(first.code, 0, `pi -p exit code (stderr: ${first.stderr.slice(-2000)})`);
    assert.match(first.stdout, /alpha-7731/);
    assert.match(first.stdout, /kiwi/);
    assert.equal(first.records.length, 3, `three assistant calls: ${JSON.stringify(first.records.map(r => r.stopReason))}`);
    assert.deepEqual(first.records.map(r => r.stopReason), ["toolUse", "toolUse", "stop"]);
    assert.equal((first.log.match(/claude process ready/g) ?? []).length, 1, "one claude process");
    assert.equal((first.log.match(/resync:/g) ?? []).length, 0, "no resync");
    assert.ok(first.ms >= waitMs, "the parked handler waited the full interval");
    for (const [i, r] of first.records.entries()) console.log(`first call ${i + 1}: stop=${r.stopReason} usage=${JSON.stringify(r.usage)}`);

    const second = await runPi(root, "second", 0);
    console.log(`second run: exit ${second.code} in ${second.ms}ms\nstdout: ${second.stdout.trim()}\nlog:\n${second.log}`);
    assert.equal(second.code, 0, `pi -p exit code (stderr: ${second.stderr.slice(-2000)})`);
    assert.match(second.stdout, /alpha-7731/);
    assert.deepEqual(second.records.map(r => r.stopReason), ["toolUse", "toolUse", "stop"]);
    for (const [i, r] of second.records.entries()) console.log(`second call ${i + 1}: stop=${r.stopReason} usage=${JSON.stringify(r.usage)}`);
    assert.ok(second.records[0].usage.cacheRead > 0, `cacheRead > 0 on the identical second run: ${JSON.stringify(second.records[0].usage)}`);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
