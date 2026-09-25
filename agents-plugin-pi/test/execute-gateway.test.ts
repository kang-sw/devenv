/**
 * Unit tests for execute-gateway.ts's pure-logic seams (260904 Phase 1,
 * end-to-end approval gateway): `buildExecuteWorkerPrompt`,
 * `resolveExecuteModelAlias`, `validatePendingApproval` (the ticket's own
 * `cmd_id` race-binding requirement), `computeLeadActiveTools` (the §8 lead
 * `--tools` reshaping + auto-include-footgun fix), `buildApprovalPromptText`
 * (the §7 payload formatter), `resolveApprovalContextCwd`
 * and `validateApprovalDecisionInput` (review fix, relay #1, CORRECTNESS
 * findings #1/#2).
 *
 * 260924 (channel approval decisions): the decision file and its poll are
 * gone. The registered `ws-approve`/`ws-worker-exec` pair is covered below
 * over a real in-process `ParentChannel`/`ChildChannel` pair and the child's
 * `ChildApprovalGate`, with `spawner.ts`'s `attachApprovalChannel` on the
 * parent side — the same objects a live launch wires, minus the process.
 *
 * NOT covered here — genuinely live-gate only, per the plan's Verification
 * Plan split and mirroring test/spawner.test.ts's own documented pure/IO
 * split: `scrapeWorkingContext` (real `git` subprocess calls), and the
 * `ws-execute` tool `execute()` body (which needs a live `RpcClient`). Its
 * pure inner logic (`resolveApprovalContextCwd`,
 * `validateApprovalDecisionInput`) is extracted and covered directly instead.
 * The ugly-read tool's `execute()` delegates to Pi's native read and is
 * covered below against real tmpdir files (260925). The registered
 * `ws-worker-exec`/`ws-approve` channel decision relay is covered below
 * with a minimal fake ExtensionAPI; live provider transport remains the
 * documented manual gate.
 *
 * 260905 (push model): `createApprovalRelay` IS covered below (`describe
 * ("createApprovalRelay")`) — it takes `pi: ExtensionAPI` as a plain
 * parameter, so a minimal fake `{sendMessage}` object is sufficient to assert
 * the `ws-agent-approval` push, with a non-git tmpdir as `sessionCtx.cwd` so
 * `scrapeWorkingContext`'s real `git` calls degrade to `undefined` fields
 * (execute-gateway.ts's own documented non-git-cwd behavior) rather than
 * needing a live session. The former `info.waiterWoken` skip branch is gone
 * with `ws-agent-wait`: the push is now unconditional and is the lead's ONLY
 * notification path, so the tests assert exactly that. A LIVE `pi --mode rpc`
 * approve/deny round-trip remains the manual gate named in the plan's
 * Verification Plan.
 *
 * 260906 Phase 2 addendum: `ws-execute`'s `execute()` body threads through
 * `spawnAgent`, whose only non-injectable dependency is `RpcClient`'s real
 * subprocess transport — the same seam `test/spawner.test.ts`'s
 * `installRpcHarness` monkey-patches. Reusing that technique (own local
 * copy, below) makes `ws-execute`'s `onModelResolved` forwarding
 * unit-testable without a live session, narrowing the "live-gate only" note
 * above to just the gated-exec/approve tool bodies.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { afterEach, beforeEach, test, describe } from "node:test";
import assert from "node:assert/strict";
import { existsSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  buildExecuteWorkerPrompt,
  resolveExecuteModelAlias,
  validatePendingApproval,
  computeLeadActiveTools,
  buildApprovalPromptText,
  resolveApprovalContextCwd,
  validateApprovalDecisionInput,
  capOutput,
  mergeExecOutput,
  createApprovalRelay,
  EXECUTE_TOOL_NAME,
  APPROVE_TOOL_NAME,
  UGLY_READ_TOOL_NAME,
  ONE_LINER_EXEC_TOOL_NAME,
  ONE_LINER_TIMEOUT_MS,
  ONE_LINER_OUTPUT_CAP_BYTES,
  registerExecuteGateway as registerExecuteGatewayBase,
  type WorkingContext,
} from "../src/execute-gateway.ts";
import { attachApprovalChannel, leadIdleRef, registerPushFlush, GATED_EXEC_TOOL_NAME, TOOL_GROUPS, resolveTools, type PendingApprovalState, type RpcAgentRecord, type RpcAgentRegistry } from "../src/spawner.ts";
import { ChildChannel, ParentChannel, readAndDeleteChannelBootstrap } from "../src/agent-channel.ts";
import { APPROVAL_DECISION_MESSAGE, ChildApprovalGate, approvalDecisionMessage, type ApprovalChildLink } from "../src/approval-protocol.ts";
import { PUSH_BATCH_CUSTOM_TYPE } from "../src/push-protocol.ts";
import { closeFakeChildren, connectFakeChild } from "./fixtures/channel-child.ts";
import { RpcClient } from "@earendil-works/pi-coding-agent";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

const TEST_EXTENSION_ENTRY = "/tmp/loaded ws adapter/index copy.ts";
function registerExecuteGateway(pi: any, bridge: any, registry: any, sessionCtx: any, ...rest: any[]) {
  return registerExecuteGatewayBase(pi, bridge, registry, { ...sessionCtx, executeWorkerPromptPath: join(process.cwd(), "execute-worker-guide.md"), extensionPath: sessionCtx.extensionPath ?? TEST_EXTENSION_ENTRY }, ...rest);
}

describe("buildExecuteWorkerPrompt", () => {
  test("no command: returns the lead's prompt unchanged", () => {
    assert.equal(buildExecuteWorkerPrompt({ prompt: "investigate the failing test" }), "investigate the failing test");
  });

  test("command given: prefixes a verbatim-command-already-run block ahead of the prompt", () => {
    const text = buildExecuteWorkerPrompt({ command: "npm test", output: "5 passing", prompt: "fix the remaining failure" });
    assert.ok(text.includes("Verbatim command already run"));
    assert.ok(text.includes("npm test"));
    assert.ok(text.includes("5 passing"));
    assert.ok(text.endsWith("fix the remaining failure"));
    assert.ok(text.indexOf("npm test") < text.indexOf("fix the remaining failure"), "the command block must precede the prompt");
  });

  test("command given but output omitted: still includes the command block with an empty output section, never throws", () => {
    const text = buildExecuteWorkerPrompt({ command: "echo hi", prompt: "continue" });
    assert.ok(text.includes("echo hi"));
    assert.ok(text.endsWith("continue"));
  });

  test("empty-string command is treated as a given command (only undefined omits the block) — distinguishes 'no command' from 'command with empty output'", () => {
    const text = buildExecuteWorkerPrompt({ command: "", output: "", prompt: "go" });
    assert.ok(text.includes("Verbatim command already run"));
  });
});

describe("resolveExecuteModelAlias", () => {
  test("complex:true resolves to undefined (inherits the lead's own model)", () => {
    assert.equal(resolveExecuteModelAlias(true), undefined);
  });

  test("complex:false or omitted resolves to the existing \"small\" alias", () => {
    assert.equal(resolveExecuteModelAlias(false), "small");
    assert.equal(resolveExecuteModelAlias(undefined), "small");
  });
});

describe("validatePendingApproval (cmd_id race-binding)", () => {
  test("no pending approval at all -> rejected", () => {
    const result = validatePendingApproval(undefined, "call-1");
    assert.deepEqual(result, { ok: false, reason: "no pending approval for this agent" });
  });

  test("cmd_id matches the pending one exactly -> accepted", () => {
    const pending: PendingApprovalState = { cmdId: "call-1", command: "echo hi" };
    assert.deepEqual(validatePendingApproval(pending, "call-1"), { ok: true });
  });

  test("cmd_id mismatch (stale or wrong agent's id) -> rejected with a reason naming both ids", () => {
    const pending: PendingApprovalState = { cmdId: "call-1", command: "echo hi" };
    const result = validatePendingApproval(pending, "call-STALE");
    assert.equal(result.ok, false);
    if (!result.ok) {
      assert.ok(result.reason.includes("call-1"));
      assert.ok(result.reason.includes("call-STALE"));
    }
  });

  test("re-using an already-resolved cmd_id (pending cleared to undefined by a prior ws-approve) is rejected the same as never having had one", () => {
    assert.deepEqual(validatePendingApproval(undefined, "call-1"), { ok: false, reason: "no pending approval for this agent" });
  });
});

describe("computeLeadActiveTools", () => {
  test("removes bash, read, and the gated-exec tool; adds ws-execute/ws-approve/the ugly-read tool", () => {
    const result = computeLeadActiveTools(["bash", "read", "edit", "write", GATED_EXEC_TOOL_NAME, "ws-agent-spawn"]);
    assert.ok(!result.includes("bash"));
    assert.ok(!result.includes("read"));
    assert.ok(!result.includes(GATED_EXEC_TOOL_NAME), "the gated-exec tool must never be active on the lead's own session (auto-include footgun fix)");
    assert.ok(result.includes("edit"), "unrelated existing tools must survive untouched");
    assert.ok(result.includes("write"));
    assert.ok(result.includes("ws-agent-spawn"));
    assert.ok(result.includes(EXECUTE_TOOL_NAME));
    assert.ok(result.includes(APPROVE_TOOL_NAME));
    assert.ok(result.includes(UGLY_READ_TOOL_NAME));
    assert.ok(result.includes(ONE_LINER_EXEC_TOOL_NAME));
  });

  test("is idempotent — running it twice in a row never re-adds a removed tool or duplicates an added one", () => {
    const once = computeLeadActiveTools(["bash", "read", GATED_EXEC_TOOL_NAME, "ws-agent-spawn", "explore"]);
    const twice = computeLeadActiveTools(once);
    assert.deepEqual([...twice].sort(), [...once].sort());
    assert.equal(new Set(twice).size, twice.length, "no duplicate entries after a 2nd pass");
  });

  test("never duplicates ws-execute/ws-approve/the ugly-read tool/the one-liner exec hatch if the current list already carries them (e.g. after an earlier setActiveTools call)", () => {
    const result = computeLeadActiveTools(["ws-agent-spawn", EXECUTE_TOOL_NAME, APPROVE_TOOL_NAME, UGLY_READ_TOOL_NAME, ONE_LINER_EXEC_TOOL_NAME]);
    assert.equal(result.filter((name) => name === EXECUTE_TOOL_NAME).length, 1);
    assert.equal(result.filter((name) => name === APPROVE_TOOL_NAME).length, 1);
    assert.equal(result.filter((name) => name === UGLY_READ_TOOL_NAME).length, 1);
    assert.equal(result.filter((name) => name === ONE_LINER_EXEC_TOOL_NAME).length, 1);
  });

  test("an empty current list still ends up with exactly the 4 added tools", () => {
    assert.deepEqual(
      [...computeLeadActiveTools([])].sort(),
      [APPROVE_TOOL_NAME, EXECUTE_TOOL_NAME, UGLY_READ_TOOL_NAME, ONE_LINER_EXEC_TOOL_NAME].sort(),
    );
  });

  test("the one-liner exec hatch never appears in any spawned child's tool group (TOOL_GROUPS is the actual enforcement mechanism, not computeLeadActiveTools)", () => {
    for (const [groupName, tools] of Object.entries(TOOL_GROUPS)) {
      assert.ok(!tools.includes(ONE_LINER_EXEC_TOOL_NAME), `TOOL_GROUPS["${groupName}"] must not include ${ONE_LINER_EXEC_TOOL_NAME}`);
    }
  });
});

describe("buildApprovalPromptText", () => {
  const fullContext: WorkingContext = {
    cwd: "/repo/worktree",
    worktree_root: "/repo/worktree",
    branch: "feature/x",
    ahead_behind: "2/0",
    dirty: true,
  };

  test("includes agent_id, cmd_id, command, rationale, and every context field when all are present", () => {
    const text = buildApprovalPromptText({
      agent_id: "agent-1",
      cmd_id: "call-1",
      command: "rm -rf build",
      rationale: "clean stale output",
      context: fullContext,
    });
    for (const needle of ["agent-1", "call-1", "rm -rf build", "clean stale output", "/repo/worktree", "feature/x", "2/0", "dirty: true"]) {
      assert.ok(text.includes(needle), `expected prompt text to include "${needle}":\n${text}`);
    }
  });

  test("instructs the lead to call ws-approve", () => {
    const text = buildApprovalPromptText({ agent_id: "a", cmd_id: "c", command: "echo hi", context: { cwd: "/repo" } });
    assert.ok(text.includes(APPROVE_TOOL_NAME));
  });

  test("omits rationale and every undefined context field instead of printing a misleading blank", () => {
    const text = buildApprovalPromptText({ agent_id: "a", cmd_id: "c", command: "echo hi", context: { cwd: "/repo" } });
    assert.ok(!text.includes("rationale:"));
    assert.ok(!text.includes("worktree_root:"));
    assert.ok(!text.includes("branch:"));
    assert.ok(!text.includes("ahead_behind:"));
    assert.ok(!text.includes("dirty:"));
    assert.ok(text.includes("cwd: /repo"));
  });
});

describe("resolveApprovalContextCwd (review fix, relay #1, CORRECTNESS finding #1)", () => {
  test("a worker-supplied cwd override on pendingApproval takes precedence over the session's base cwd", () => {
    assert.equal(resolveApprovalContextCwd({ cwd: "/repo/subdir" }, "/repo"), "/repo/subdir");
  });

  test("falls back to the session's base cwd when pendingApproval carries no override", () => {
    assert.equal(resolveApprovalContextCwd({ cwd: undefined }, "/repo"), "/repo");
    assert.equal(resolveApprovalContextCwd({}, "/repo"), "/repo");
  });
});

describe("validateApprovalDecisionInput (review fix, relay #1, CORRECTNESS finding #2)", () => {
  test("decision:approve requires neither reason nor command", () => {
    assert.deepEqual(validateApprovalDecisionInput("approve", undefined, undefined), { ok: true });
  });

  test("decision:run-instead with a non-empty command is accepted", () => {
    assert.deepEqual(validateApprovalDecisionInput("run-instead", undefined, "echo substituted"), { ok: true });
  });

  test("decision:run-instead with a missing, empty, or whitespace-only command is rejected", () => {
    for (const command of [undefined, "", "   "]) {
      const result = validateApprovalDecisionInput("run-instead", undefined, command);
      assert.equal(result.ok, false, `expected run-instead with command=${JSON.stringify(command)} to be rejected`);
    }
  });

  test("decision:deny with a non-empty reason is accepted", () => {
    assert.deepEqual(validateApprovalDecisionInput("deny", "not safe", undefined), { ok: true });
  });

  test("decision:deny with a missing, empty, or whitespace-only reason is rejected", () => {
    for (const reason of [undefined, "", "   "]) {
      const result = validateApprovalDecisionInput("deny", reason, undefined);
      assert.equal(result.ok, false, `expected deny with reason=${JSON.stringify(reason)} to be rejected`);
    }
  });

  test("decision:approve ignores an omitted reason/command even though deny/run-instead would reject them", () => {
    assert.deepEqual(validateApprovalDecisionInput("approve", "", ""), { ok: true });
  });
});

describe("do-i-really-have-to-read-this-myself (260925: execute() delegates to Pi's native read)", () => {
  type ReadResult = { content: Array<{ type: string; text?: string; data?: string; mimeType?: string }> };
  type CapturedTool = { execute: (toolCallId: string, params: unknown, signal?: AbortSignal, onUpdate?: unknown, ctx?: unknown) => Promise<ReadResult> };
  // 1x1 opaque PNG: small enough that the host's auto-resize is a pass-through.
  const PNG_1X1 = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");
  let cwd: string;

  function registerAndCapture(): CapturedTool {
    const registered = new Map<string, CapturedTool>();
    const pi = { registerTool: (def: { name: string } & CapturedTool) => registered.set(def.name, def) } as unknown as ExtensionAPI;
    registerExecuteGateway(pi, {} as never, new Map(), { cwd, executeWorkerPromptPath: "/tmp/fake-execute-worker-guide.md" });
    const tool = registered.get(UGLY_READ_TOOL_NAME);
    assert.ok(tool, `${UGLY_READ_TOOL_NAME} must be registered by registerExecuteGateway`);
    return tool!;
  }
  const textOf = (result: ReadResult) => result.content.filter((block) => block.type === "text").map((block) => block.text).join("\n");

  beforeEach(() => { cwd = mkdtempSync(join(tmpdir(), "ws-pi-ugly-read-")); });
  afterEach(() => { rmSync(cwd, { recursive: true, force: true }); });

  test("an image file comes back as an image content block, resolved relative to the session cwd", async () => {
    writeFileSync(join(cwd, "pixel.png"), PNG_1X1);
    const result = await registerAndCapture().execute("call-1", { path: "pixel.png" }, undefined, undefined, {});
    const image = result.content.find((block) => block.type === "image");
    assert.ok(image, "an image content block is returned, not mojibake text");
    assert.equal(image!.mimeType, "image/png");
    assert.ok(image!.data && image!.data.length > 0, "the image carries base64 data");
  });

  test("the tool-call ctx reaches the native read: a non-vision model gets the host's omission note", async () => {
    writeFileSync(join(cwd, "pixel.png"), PNG_1X1);
    const result = await registerAndCapture().execute("call-2", { path: "pixel.png" }, undefined, undefined, { model: { input: ["text"] } });
    assert.match(textOf(result), /does not support images/);
  });

  test("a text file honors 1-indexed offset/limit and says how to continue", async () => {
    writeFileSync(join(cwd, "five.txt"), ["line1", "line2", "line3", "line4", "line5"].join("\n"));
    const tool = registerAndCapture();
    const sliced = textOf(await tool.execute("call-3", { path: join(cwd, "five.txt"), offset: 2, limit: 2 }, undefined, undefined, {}));
    assert.ok(sliced.startsWith("line2\nline3"), sliced);
    assert.doesNotMatch(sliced, /line1|line4/);
    assert.match(sliced, /Use offset=4 to continue/);
    assert.equal(textOf(await tool.execute("call-4", { path: "five.txt" }, undefined, undefined, {})), "line1\nline2\nline3\nline4\nline5");
  });

  test("a text file over 2000 lines is truncated with the host's offset-continuation hint", async () => {
    writeFileSync(join(cwd, "long.txt"), Array.from({ length: 2500 }, (_, index) => `row-${index + 1}`).join("\n"));
    const text = textOf(await registerAndCapture().execute("call-5", { path: "long.txt" }, undefined, undefined, {}));
    assert.match(text, /row-2000\n/);
    assert.doesNotMatch(text, /row-2001\b/);
    assert.match(text, /\[Showing lines 1-2000 of 2500\. Use offset=2001 to continue\.\]/);
  });
});

describe("capOutput (byte-cap-to-last-complete-line for the one-liner exec hatch)", () => {
  test("exactly at the byte cap: returned unchanged, no hint", () => {
    const raw = "x".repeat(10);
    assert.equal(capOutput(raw, 10), raw);
  });

  test("one byte over the cap: truncated and a drop-hint is appended", () => {
    const raw = "line1\nline2\nline3";
    const capped = capOutput(raw, Buffer.byteLength(raw, "utf8") - 1);
    assert.notEqual(capped, raw);
    assert.ok(capped.includes("truncated"), "a drop-hint must be appended when truncation happens");
    assert.ok(capped.includes("ws-execute"), "the drop-hint must point at ws-execute for bulk output");
  });

  test("a multibyte character straddling the cut is never corrupted — the partial trailing codepoint is dropped cleanly", () => {
    // Each "🙂" is 4 UTF-8 bytes (emoji1 = bytes 0-3, emoji2 = 4-7, emoji3 =
    // 8-11); cutting at 10 bytes lands mid-emoji, keeping only the third
    // emoji's first two bytes (byte 8 is that emoji's first byte, not its
    // second), mirroring spawner.test.ts's truncatePromptForStorage
    // multibyte test.
    const raw = "🙂🙂🙂🙂🙂";
    const capped = capOutput(raw, 10);
    assert.ok(capped.startsWith("🙂🙂"));
    const beforeHint = capped.split("\n")[0];
    assert.ok(!beforeHint.includes("�"), "no replacement character from a split codepoint");
  });

  test("a single line longer than the cap with no newline inside it: the byte-trimmed head is kept, not emptied, and still gets a hint", () => {
    const raw = "a".repeat(50);
    const capped = capOutput(raw, 10);
    assert.ok(capped.startsWith("a".repeat(10)), "the head must be kept at a character boundary, not dropped to nothing");
    assert.ok(capped.length > 10, "the result must carry more than just the bare head (the drop-hint)");
    assert.ok(capped.includes("truncated"));
  });

  test("a decoded head with a trailing partial line is trimmed back to the last complete line", () => {
    const raw = "complete line one\ncomplete line two\npartial-tail-that-gets-cut";
    // Cap lands inside "partial-tail-that-gets-cut", after the second newline.
    const cutPoint = raw.indexOf("partial-tail") + 5;
    const capped = capOutput(raw, cutPoint);
    assert.ok(capped.startsWith("complete line one\ncomplete line two"));
    assert.ok(!capped.includes("partial-tail-that-gets-cut"), "the trailing partial line must be dropped, not kept half-cut");
  });

  test("module constants: 30s timeout, 4KB cap", () => {
    assert.equal(ONE_LINER_TIMEOUT_MS, 30_000);
    assert.equal(ONE_LINER_OUTPUT_CAP_BYTES, 4096);
  });
});

describe("mergeExecOutput (review relay #1, Minor a)", () => {
  test("stdout already ends with a newline: concatenated with no extra separator", () => {
    assert.equal(mergeExecOutput("out\n", "err"), "out\nerr");
  });

  test("stdout has no trailing newline: a newline is inserted so stderr is never glued onto stdout's last line", () => {
    assert.equal(mergeExecOutput("out", "err"), "out\nerr");
  });

  test("stdout-only or stderr-only output is returned unchanged, never gains a spurious newline", () => {
    assert.equal(mergeExecOutput("out", ""), "out");
    assert.equal(mergeExecOutput("", "err"), "err");
    assert.equal(mergeExecOutput("", ""), "");
  });
});

describe("createApprovalRelay (260905: unconditional ws-agent-approval push)", () => {
  function freshRecord(pending: PendingApprovalState): RpcAgentRecord {
    return {
      agentId: "rpc-agent-1",
      sessionPath: "/tmp/ws-pi-agent-x/session.jsonl",
      systemPromptPath: "/tmp/ws-pi-agent-x/prompt.md",
      wsToolNames: [],
      toolGroup: "execute-worker",
      spawnRole: "execute-worker",
      streaming: false,
      running: true,
      // A live client is what puts the record in the fan-in denominator M
      // (review relay #1, I3: M is "not dormant/stopped/exited", not
      // "running"). Never a real `RpcClient` — nothing here calls into it.
      client: {} as RpcAgentRecord["client"],
      reportLog: [],
      pendingApproval: pending,
    };
  }

  function fakePi(): {
    api: ExtensionAPI;
    sent: Array<{ message: { customType?: string; content?: string; details?: Record<string, unknown> }; options?: { deliverAs?: string; triggerTurn?: boolean } }>;
  } {
    const sent: Array<{ message: { customType?: string; content?: string; details?: Record<string, unknown> }; options?: { deliverAs?: string; triggerTurn?: boolean } }> = [];
    // Phase 2 requires a live accessor and confirmed streaming start before
    // custom delivery. Keep every approval payload/fan-in assertion below.
    let idle = true;
    leadIdleRef.current = () => idle;
    const handlers = new Map<string, () => void>();
    const api = {
      on: (event: string, handler: () => void) => handlers.set(event, handler),
      sendMessage: (message: unknown, options?: unknown) => {
        assert.equal(idle, false, "approval custom message cannot start an idle run");
        const batch = message as { customType?: string; details?: { items?: unknown[] } };
        if (batch.customType === PUSH_BATCH_CUSTOM_TYPE && Array.isArray(batch.details?.items)) {
          for (const item of batch.details.items) sent.push({ message: item as never, options: options as never });
        } else {
          sent.push({ message: message as never, options: options as never });
        }
      },
      sendUserMessage: (content: unknown, options: unknown) => {
        assert.match(String(content), /^\d+ ws messages waiting;[^\n]+$/);
        assert.deepEqual(options, { deliverAs: "followUp" });
        idle = false;
        handlers.get("agent_start")?.();
        idle = true;
      },
    } as unknown as ExtensionAPI;
    registerPushFlush(api, { delayMs: () => 10 });
    return { api, sent };
  }

  function withTempCwd<T>(fn: (cwd: string) => T): T {
    // Non-git tmpdir: scrapeWorkingContext's real `git` subprocess calls
    // degrade to undefined fields rather than throwing (execute-gateway.ts's
    // own documented behavior) — no live pi/RpcClient session is needed to
    // exercise the relay.
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-agent-approval-relay-test-"));
    try {
      return fn(dir);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  }

  test("pushes exactly one ws-agent-approval message carrying the cmd_id and the §7 request text, delivered as steer", () => {
    withTempCwd((cwd) => {
      const pi = fakePi();
      const record = freshRecord({ cmdId: "call-2", command: "rm -rf build", rationale: "clean stale output" });
      const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
      const relay = createApprovalRelay(pi.api, { cwd }, { current: registry });

      relay(record);

      assert.equal(pi.sent.length, 1, "this push is the lead's only notification path — there is no wait return to duplicate");
      const [{ message, options }] = pi.sent;
      assert.equal(message.customType, "ws-agent-approval");
      assert.deepEqual(options, { deliverAs: "steer", triggerTurn: true }, "an approval request must interrupt, not queue — the child cannot progress until it is answered");
      assert.equal(message.details?.cmd_id, "call-2");
      assert.equal(message.details?.agent_id, "rpc-agent-1");
      assert.ok(String(message.details?.request).includes("rm -rf build"), "the request text must carry the pending command");
      assert.ok(String(message.details?.request).includes("call-2"), "the request text must carry the pending cmd_id");
    });
  });

  test("the push carries the fan-in status line, and an approval-blocked child is still counted as running", () => {
    withTempCwd((cwd) => {
      const pi = fakePi();
      const record = freshRecord({ cmdId: "call-1", command: "echo hi" });
      const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
      createApprovalRelay(pi.api, { cwd }, { current: registry })(record);

      assert.equal(pi.sent[0].message.details?.status, "1 delegated agent still running");
    });
  });

  test("a re-issued request (the earlier decision was discarded on a connection drop) says so, so the lead decides again knowingly", () => {
    withTempCwd((cwd) => {
      const pi = fakePi();
      const record = freshRecord({ cmdId: "call-5", command: "echo hi", reissued: true });
      createApprovalRelay(pi.api, { cwd }, { current: new Map([[record.agentId, record]]) })(record);
      assert.equal(pi.sent.length, 1);
      assert.match(String(pi.sent[0].message.details?.request), /earlier decision for this cmd_id was not delivered .* discarded; decide again/);
      const plain = fakePi();
      createApprovalRelay(plain.api, { cwd }, { current: new Map() })(freshRecord({ cmdId: "call-6", command: "echo hi" }));
      assert.doesNotMatch(String(plain.sent[0].message.details?.request), /decide again/);
    });
  });

  test("no pendingApproval on the record: nothing is pushed", () => {
    withTempCwd((cwd) => {
      const pi = fakePi();
      const record = freshRecord({ cmdId: "call-3", command: "echo hi" });
      record.pendingApproval = undefined;

      createApprovalRelay(pi.api, { cwd }, { current: new Map() })(record);

      assert.deepEqual(pi.sent, [], "nothing is pending to relay");
    });
  });

  test("an unfilled registry ref (the relay is built BEFORE registerAgentTools) degrades to a status-less push rather than throwing", () => {
    withTempCwd((cwd) => {
      const pi = fakePi();
      const record = freshRecord({ cmdId: "call-4", command: "echo hi" });

      assert.doesNotThrow(() => createApprovalRelay(pi.api, { cwd }, { current: undefined })(record));
      assert.equal(pi.sent[0].message.details?.status, undefined, "Edition: no readable fan-in means no status line");
      assert.equal(pi.sent[0].message.details?.cmd_id, "call-4", "the approval itself still relays");
    });
  });
});

describe("execute-worker registration boundary", () => {
  test("the execute-worker's allowlist reaches the registered ws-worker-exec implementation while excluded tools stay unavailable", () => {
    const registered = new Map<string, { name: string }>();
    const pi = { registerTool: (tool: { name: string }) => registered.set(tool.name, tool) } as unknown as ExtensionAPI;
    registerExecuteGateway(pi, {} as never, new Map(), { cwd: "/tmp", executeWorkerPromptPath: "/tmp/guide.md" });
    const active = new Set(resolveTools("execute-worker").split(","));
    assert.ok(registered.has(GATED_EXEC_TOOL_NAME), "registerExecuteGateway installs the actual gated tool implementation");
    assert.ok(active.has(GATED_EXEC_TOOL_NAME), "the execute-worker can reach that registered implementation");
    assert.ok(active.has("ws-report-to-lead"), "the execute-worker retains its report channel");
    for (const unavailable of ["bash", "edit", "write", "ws-agent-spawn"]) assert.ok(!active.has(unavailable), `execute-worker must not receive ${unavailable}`);
  });
});

describe("registered ws-approve/ws-worker-exec decision relay (260924: over the control channel)", () => {
  type Tool = { execute: (...args: any[]) => Promise<any> };

  /**
   * The parent's registry record and channel, the child's gate and channel,
   * and both registered tools — wired the way a live launch wires them
   * (`attachApprovalChannel` on the parent, `approvalGate.attach` on the
   * child). `link` wraps the child's channel as the gate sees it, to inject
   * the acknowledgment failures a real connection drop produces.
   */
  async function harness(opts: { reconnect?: boolean; link?: (child: ChildChannel) => ApprovalChildLink } = {}) {
    const home = mkdtempSync(join(tmpdir(), "ws-pi-agent-registered-decision-test-"));
    const parent = await ParentChannel.bind(1, { socketDir: join(home, "ch") });
    const approvalGate = new ChildApprovalGate();
    /** `onResume` runs once, right after the child computes its next hello's resume section (before the hello is sent). */
    const hooks: { onResume?: () => void } = {};
    const resume = () => {
      const section = approvalGate.resume();
      const hook = hooks.onResume;
      hooks.onResume = undefined;
      hook?.();
      return section;
    };
    const child = await ChildChannel.connect(readAndDeleteChannelBootstrap({ ...parent.bootstrapEnv() })!, { reconnect: opts.reconnect ?? false, backoffCapMs: 50, resume });
    approvalGate.attach(child);
    await parent.hello();
    const decisionsDelivered: string[] = [];
    child.onMessage((msg) => { if (msg.t === APPROVAL_DECISION_MESSAGE) decisionsDelivered.push(String(msg.cmd_id)); });
    const executions: string[] = [];
    const registered = new Map<string, Tool>();
    const record = { agentId: "execute-worker-1", sessionPath: join(home, "session.jsonl"), channel: parent, running: true, client: {}, pendingApproval: undefined as PendingApprovalState | undefined } as unknown as RpcAgentRecord;
    const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
    const pi = {
      registerTool: (tool: { name: string } & Tool) => registered.set(tool.name, tool),
      exec: async (_shell: string, args: string[]) => {
        executions.push(args[1]!);
        return { stdout: `ran ${args[1]}`, stderr: "", code: 0, killed: false };
      },
    } as unknown as ExtensionAPI;
    registerExecuteGateway(pi, {} as never, registry, { cwd: home, executeWorkerPromptPath: "/tmp/fake-execute-worker-guide.md", channel: opts.link?.(child) ?? child, approvalGate });
    const asked: RpcAgentRecord[] = [];
    const detach = attachApprovalChannel(record, parent, () => (r) => asked.push(r));
    const approve = registered.get(APPROVE_TOOL_NAME)!;
    const exec = registered.get(GATED_EXEC_TOOL_NAME)!;
    assert.ok(approve && exec, "registerExecuteGateway installs both ends of the relay");
    const until = (done: () => boolean, what: string) => new Promise<void>((resolve, reject) => {
      const deadline = Date.now() + 5_000;
      const tick = () => done() ? resolve() : Date.now() > deadline ? reject(new Error(`timed out waiting for ${what}`)) : setTimeout(tick, 5);
      tick();
    });
    const released = () => until(() => record.pendingApproval === undefined, "the request to be released");
    /** Drops the live connection from the parent side and, with reconnect on, waits for the child's reconnect hello. */
    const drop = async () => {
      const reconnected = opts.reconnect ? new Promise<void>((resolve) => { const off = parent.onConnection((_c, hello) => { if (hello.reconnect) { off(); resolve(); } }); }) : undefined;
      const dropped = new Promise<void>((resolve) => { const off = parent.onDisconnect(() => { off(); resolve(); }); });
      parent.live!.close();
      await dropped;
      await reconnected;
    };
    const close = () => { detach(); child.close(); parent.close(); rmSync(home, { recursive: true, force: true }); };
    return { home, parent, child, approvalGate, hooks, executions, decisionsDelivered, asked, record, approve, exec, released, until, drop, close };
  }

  test("approve, deny, and run-instead reach the gated tool over the channel, the acknowledgment releases the request, and no decision file or directory is ever created", async () => {
    const h = await harness();
    try {
      const cases = [
        { cmdId: "call<>:\"/\\|?*%\u0000\u001f\u007f\u009f", decision: "approve" as const, executed: "echo original", text: /exit code: 0/ },
        { cmdId: "call%7C-literal", decision: "deny" as const, reason: "not approved", executed: undefined, text: /Lead denied this command: not approved/ },
        { cmdId: "call|run-instead", decision: "run-instead" as const, command: "echo substituted", executed: "echo substituted", text: /Lead substituted a different command/ },
      ];
      for (const item of cases) {
        h.record.pendingApproval = { cmdId: item.cmdId, command: "echo original" };
        const executionsBefore = h.executions.length;
        const controller = new AbortController();
        const abortTimer = setTimeout(() => controller.abort(), 5_000);
        const pending = h.exec.execute(item.cmdId, { command: "echo original", rationale: "exercise the registered reader" }, controller.signal);
        assert.equal(h.approvalGate.pending, item.cmdId, "the gated tool waits on its own toolCallId");
        const reply = await h.approve.execute("lead-tool-call", { agent_id: h.record.agentId, cmd_id: item.cmdId, decision: item.decision, reason: item.reason, command: item.command });
        assert.deepEqual(JSON.parse(reply.content[0].text), { ok: true });
        assert.equal(h.record.pendingApproval?.decision, "sent", "ws-approve marks the decision in flight, not consumed");
        const result = await pending;
        clearTimeout(abortTimer);
        assert.match(result.content[0]!.text, item.text);
        await h.released();
        assert.equal(h.approvalGate.pending, undefined);
        if (item.executed === undefined) assert.equal(h.executions.length, executionsBefore, "deny never runs the proposed command");
        else assert.equal(h.executions.at(-1), item.executed, "approve runs the proposal and run-instead runs the replacement");
      }
      assert.equal(existsSync(join(h.home, "approvals")), false, "no decision directory");
    } finally { h.close(); }
  });

  test("a second ws-approve while the first decision is in flight is rejected; a decision for a different cmd_id never satisfies the wait", async () => {
    const h = await harness();
    try {
      h.record.pendingApproval = { cmdId: "call-1", command: "echo original" };
      const controller = new AbortController();
      const pending = h.exec.execute("call-1", { command: "echo original", rationale: "r" }, controller.signal);
      await assert.rejects(h.approve.execute("x", { agent_id: h.record.agentId, cmd_id: "call-2", decision: "approve" }), /cmd_id mismatch/);
      assert.equal(h.record.pendingApproval?.decision, undefined, "a rejected call sends nothing");
      // A forged decision for another cmd_id over the real connection is ignored by the wait.
      h.parent.send(approvalDecisionMessage("call-2", { decision: "approve" }));
      await h.approve.execute("x", { agent_id: h.record.agentId, cmd_id: "call-1", decision: "deny", reason: "no" });
      await assert.rejects(h.approve.execute("x", { agent_id: h.record.agentId, cmd_id: "call-1", decision: "approve" }), /already sent and awaiting worker consumption/);
      assert.match((await pending).content[0].text, /Lead denied this command: no/);
      await h.released();
      assert.deepEqual(h.executions, [], "nothing ran: the forged approve was not for this cmd_id and the real decision was a deny");
    } finally { h.close(); }
  });

  test("ws-approve with no live connection discards immediately with a not-delivered error and sends nothing; the reconnect hello re-issues the request, and only the decision sent over the new connection is consumed", async () => {
    const h = await harness({ reconnect: true });
    try {
      h.record.pendingApproval = { cmdId: "call-1", command: "echo original" };
      const controller = new AbortController();
      const pending = h.exec.execute("call-1", { command: "echo original", rationale: "r" }, controller.signal);
      const dropped = new Promise<void>((resolve) => { const off = h.parent.onDisconnect(() => { off(); resolve(); }); });
      const reconnected = new Promise<void>((resolve) => { const off = h.parent.onConnection((_c, hello) => { if (hello.reconnect) { off(); resolve(); } }); });
      h.parent.live!.close();
      await dropped;
      await assert.rejects(h.approve.execute("x", { agent_id: h.record.agentId, cmd_id: "call-1", decision: "approve" }), /not delivered: the worker has no live connection.*re-issued to you when the worker reconnects/);
      assert.deepEqual(h.record.pendingApproval, { cmdId: "call-1", command: "echo original", decision: "discarded" }, "the attempt is recorded as discarded so the reconnect hello reconciles it");
      await assert.rejects(h.approve.execute("x", { agent_id: h.record.agentId, cmd_id: "call-1", decision: "approve" }), /discarded when the worker's connection dropped/, "no retry by hand while the reconnect is pending");
      assert.equal(h.approvalGate.pending, "call-1", "the child still waits");

      await reconnected;
      await h.until(() => h.asked.length === 1, "the re-issued request");
      assert.deepEqual(h.record.pendingApproval, { cmdId: "call-1", command: "echo original", reissued: true, issue: 1 }, "the reconnect hello reported the cmd_id: the lead is asked afresh");
      assert.deepEqual(h.decisionsDelivered, [], "the parent never sent anything on its own");
      await h.approve.execute("x", { agent_id: h.record.agentId, cmd_id: "call-1", decision: "approve" });
      assert.match((await pending).content[0].text, /exit code: 0/);
      await h.released();
      assert.deepEqual(h.decisionsDelivered, ["call-1"], "exactly one decision frame, the one the lead sent over the new connection");
      assert.deepEqual(h.executions, ["echo original"], "exactly one execution");
    } finally { h.close(); }
  });

  test("a decision whose acknowledgment cannot be sent is not consumed: the parent discards it on the drop, never re-sends, re-asks on the reconnect hello, and the command runs exactly once on the fresh decision", async () => {
    let failNextSend = false;
    const h = await harness({ reconnect: true, link: (child) => ({ onMessage: (cb) => child.onMessage(cb), send: (msg) => { if (failNextSend) { failNextSend = false; throw new Error("ws-pi-channel: not connected to the parent"); } child.send(msg); } }) });
    try {
      h.record.pendingApproval = { cmdId: "call-1", command: "echo original" };
      const controller = new AbortController();
      const pending = h.exec.execute("call-1", { command: "echo original", rationale: "r" }, controller.signal);
      failNextSend = true; // the connection ends between the decision's arrival and its acknowledgment
      await h.approve.execute("x", { agent_id: h.record.agentId, cmd_id: "call-1", decision: "approve" });
      await h.until(() => h.decisionsDelivered.length === 1, "the decision to reach the child");
      assert.equal(h.record.pendingApproval?.decision, "sent");
      assert.equal(h.approvalGate.pending, "call-1", "unacknowledged means not consumed: the child keeps waiting");
      assert.deepEqual(h.executions, [], "the command did not start");

      await h.drop();
      await h.until(() => h.asked.length === 1, "the re-issued request");
      assert.deepEqual(h.record.pendingApproval, { cmdId: "call-1", command: "echo original", reissued: true, issue: 1 });
      assert.deepEqual(h.decisionsDelivered, ["call-1"], "the discarded decision was not re-sent on the reconnect");
      await h.approve.execute("x", { agent_id: h.record.agentId, cmd_id: "call-1", decision: "run-instead", command: "echo fresh" });
      assert.match((await pending).content[0].text, /Lead substituted a different command/);
      await h.released();
      assert.deepEqual(h.executions, ["echo fresh"], "exactly one execution, from the decision sent over the new connection");
      assert.deepEqual(h.decisionsDelivered, ["call-1", "call-1"], "two frames in total, both sent by ws-approve");
    } finally { h.close(); }
  });

  test("an acknowledgment lost with the connection: the child consumed and ran the command once; the reconnect hello lists it as consumed, so the parent releases the request instead of re-asking or re-sending", async () => {
    let swallowNextSend = false;
    const h = await harness({ reconnect: true, link: (child) => ({ onMessage: (cb) => child.onMessage(cb), send: (msg) => { if (swallowNextSend) { swallowNextSend = false; return; } child.send(msg); } }) });
    try {
      h.record.pendingApproval = { cmdId: "call-1", command: "echo original" };
      const controller = new AbortController();
      const pending = h.exec.execute("call-1", { command: "echo original", rationale: "r" }, controller.signal);
      swallowNextSend = true; // written into a socket that is already dead
      await h.approve.execute("x", { agent_id: h.record.agentId, cmd_id: "call-1", decision: "approve" });
      assert.match((await pending).content[0].text, /exit code: 0/);
      assert.deepEqual(h.executions, ["echo original"]);
      assert.equal(h.record.pendingApproval?.decision, "sent", "the parent never saw the acknowledgment");

      await h.drop();
      await h.released();
      assert.deepEqual(h.asked, [], "nothing to re-ask: the child no longer waits");
      assert.deepEqual(h.decisionsDelivered, ["call-1"], "nothing re-sent");
      assert.deepEqual(h.executions, ["echo original"], "the command ran exactly once across the disconnect");
      await assert.rejects(h.approve.execute("x", { agent_id: h.record.agentId, cmd_id: "call-1", decision: "approve" }), /no pending approval/);
    } finally { h.close(); }
  });

  test("a decision that reaches the child before the gated tool's execute() starts is kept for it and consumed once the wait begins", async () => {
    const h = await harness();
    try {
      h.record.pendingApproval = { cmdId: "call-1", command: "echo original" };
      await h.approve.execute("x", { agent_id: h.record.agentId, cmd_id: "call-1", decision: "approve" });
      await h.until(() => h.decisionsDelivered.length === 1, "the decision to reach the child");
      assert.equal(h.approvalGate.pending, undefined, "no wait exists yet");
      const result = await h.exec.execute("call-1", { command: "echo original", rationale: "r" }, new AbortController().signal);
      assert.match(result.content[0].text, /exit code: 0/);
      await h.released();
      assert.deepEqual(h.executions, ["echo original"]);
    } finally { h.close(); }
  });

  test("a decision kept early, then a disconnect before the wait opens: the parent re-asks, the old decision is never consumed, and the fresh one over the new connection is", async () => {
    const h = await harness({ reconnect: true });
    try {
      h.record.pendingApproval = { cmdId: "call-1", command: "echo original" };
      await h.approve.execute("x", { agent_id: h.record.agentId, cmd_id: "call-1", decision: "approve" });
      await h.until(() => h.decisionsDelivered.length === 1, "the decision to reach the child");
      assert.equal(h.approvalGate.pending, undefined, "kept early: no wait yet");

      await h.drop();
      await h.until(() => h.asked.length === 1, "the re-issued request");
      assert.deepEqual(h.record.pendingApproval, { cmdId: "call-1", command: "echo original", reissued: true, issue: 1 }, "no consumption evidence: not released");
      // The wait opens on a fully re-established connection, where an acknowledgment could be sent.
      await h.until(() => h.child.connected, "the child's welcome");
      const pending = h.exec.execute("call-1", { command: "echo original", rationale: "r" }, new AbortController().signal);
      assert.equal(await Promise.race([pending, new Promise((resolve) => setTimeout(() => resolve("waiting"), 50))]), "waiting", "the old connection's decision was discarded with it");
      assert.deepEqual(h.executions, []);
      await h.approve.execute("x", { agent_id: h.record.agentId, cmd_id: "call-1", decision: "run-instead", command: "echo fresh" });
      assert.match((await pending).content[0].text, /Lead substituted a different command/);
      await h.released();
      assert.deepEqual(h.executions, ["echo fresh"], "exactly one execution, on the decision sent over the new connection");
    } finally { h.close(); }
  });

  test("a wait that opens with a kept decision between the reconnect hello and its welcome does not hang: the request is re-issued", async () => {
    const h = await harness({ reconnect: true });
    try {
      h.record.pendingApproval = { cmdId: "call-1", command: "echo original" };
      await h.approve.execute("x", { agent_id: h.record.agentId, cmd_id: "call-1", decision: "approve" });
      await h.until(() => h.decisionsDelivered.length === 1, "the decision to reach the child");
      let pending: Promise<any> | undefined;
      // The hello is built without the cmd_id; the wait opens right after, before the welcome.
      h.hooks.onResume = () => queueMicrotask(() => { pending = h.exec.execute("call-1", { command: "echo original", rationale: "r" }, new AbortController().signal); });
      await h.drop();
      assert.ok(pending, "the wait opened inside the reconnect handshake");
      await h.until(() => h.asked.length === 1, "the re-issued request");
      assert.equal(h.record.pendingApproval?.decision, undefined, "re-issued, not released");
      assert.equal(h.approvalGate.pending, "call-1");
      await h.approve.execute("x", { agent_id: h.record.agentId, cmd_id: "call-1", decision: "approve" });
      assert.match((await pending!).content[0].text, /exit code: 0/);
      await h.released();
      assert.deepEqual(h.executions, ["echo original"]);
    } finally { h.close(); }
  });

  test("a decision lost in transit, then a reconnect before the wait opens: the request is re-issued, not released", async () => {
    const h = await harness({ reconnect: true });
    try {
      // As ws-approve records it; the frame itself never reaches the child.
      h.record.pendingApproval = { cmdId: "call-1", command: "echo original", decision: "sent" };
      await h.drop();
      await h.until(() => h.asked.length === 1, "the re-issued request");
      assert.deepEqual(h.record.pendingApproval, { cmdId: "call-1", command: "echo original", reissued: true, issue: 1 });
      const pending = h.exec.execute("call-1", { command: "echo original", rationale: "r" }, new AbortController().signal);
      await h.approve.execute("x", { agent_id: h.record.agentId, cmd_id: "call-1", decision: "approve" });
      assert.match((await pending).content[0].text, /exit code: 0/);
      await h.released();
      assert.deepEqual(h.executions, ["echo original"]);
      assert.deepEqual(h.decisionsDelivered, ["call-1"], "only the decision sent over the new connection");
    } finally { h.close(); }
  });

  test("ws-worker-exec in a process with no parent channel fails loud instead of waiting forever", async () => {
    const registered = new Map<string, Tool>();
    const pi = { registerTool: (tool: { name: string } & Tool) => registered.set(tool.name, tool) } as unknown as ExtensionAPI;
    registerExecuteGateway(pi, {} as never, new Map(), { cwd: "/tmp", executeWorkerPromptPath: "/tmp/guide.md" });
    await assert.rejects(registered.get(GATED_EXEC_TOOL_NAME)!.execute("call-1", { command: "echo", rationale: "r" }), /no parent control channel/);
  });
});

describe("do-i-really-have-to-run-this-myself (the one-liner exec hatch's execute() body)", () => {
  type FakeExecResult = { stdout: string; stderr: string; code: number; killed: boolean };
  type CapturedTool = {
    execute: (toolCallId: string, params: unknown, signal?: AbortSignal) => Promise<{ content: Array<{ type: string; text: string }> }>;
  };

  // Unlike ws-worker-exec/ws-execute/ws-approve/the ugly-read tool (all
  // RpcClient/registry/filesystem-dependent, hence live-gate only per this
  // file's header comment), this tool's execute() only touches `pi.exec`, so
  // a plain stub (same fakePi() convention as the createApprovalRelay block
  // above, stubbing pi.exec instead of pi.sendMessage) is enough to unit-test
  // it directly.
  function registerAndCapture(execFn: (command: string, args: string[], options?: { cwd?: string; timeout?: number; signal?: AbortSignal }) => Promise<FakeExecResult>): CapturedTool {
    const registered = new Map<string, CapturedTool>();
    const pi = {
      registerTool: (def: { name: string } & CapturedTool) => {
        registered.set(def.name, def);
      },
      exec: execFn,
    } as unknown as ExtensionAPI;
    const bridge = {} as unknown as Parameters<typeof registerExecuteGateway>[1];
    const registry: RpcAgentRegistry = new Map();
    registerExecuteGateway(pi, bridge, registry, { cwd: "/tmp/ws-pi-agent-one-liner-test", executeWorkerPromptPath: "/tmp/fake-execute-worker-guide.md" });
    const tool = registered.get(ONE_LINER_EXEC_TOOL_NAME);
    assert.ok(tool, `${ONE_LINER_EXEC_TOOL_NAME} must be registered by registerExecuteGateway`);
    return tool!;
  }

  test("why is echoed first, followed by exit code and the (uncapped) output", async () => {
    const tool = registerAndCapture(async () => ({ stdout: "hello\n", stderr: "", code: 0, killed: false }));
    const result = await tool.execute("call-1", { command: "echo hello", why: "quick sanity check before continuing" });
    const text = result.content[0].text;
    assert.ok(text.startsWith("why: quick sanity check before continuing"), "why must be echoed first in the result");
    assert.ok(text.includes("exit code: 0"));
    assert.ok(text.includes("hello"));
  });

  test("a timed-out command (killed:true, signal not aborted) resolves normally with the partial output plus a timeout line — never a thrown error", async () => {
    // Review relay #1, Important: a real SIGTERM'd child exits by signal, so
    // the installed package's exec.js coerces the null exit code to 0
    // (`code ?? 0`) — not the plan's original 143 assumption. This stub
    // reflects the actual shipped shape.
    const tool = registerAndCapture(async () => ({ stdout: "partial", stderr: "", code: 0, killed: true }));
    const result = await tool.execute("call-2", { command: "sleep 60", why: "check a slow command" });
    const text = result.content[0].text;
    assert.ok(text.includes("partial"), "partial output must still be returned");
    assert.ok(text.includes("timed out"), "a timeout line must be appended when the timeout fired");
    assert.ok(!text.split("\n").includes("exit code: 0"), "a killed run's exit-code line must never read as a bare, unannotated clean exit");
  });

  test("an interrupted command (killed:true, signal.aborted:true) reports an interrupt, never a fabricated timeout claim", async () => {
    const tool = registerAndCapture(async () => ({ stdout: "started", stderr: "", code: 0, killed: true }));
    const controller = new AbortController();
    controller.abort();
    const result = await tool.execute("call-2b", { command: "sleep 60", why: "check an interrupted command" }, controller.signal);
    const text = result.content[0].text;
    assert.ok(text.includes("interrupted"), "an aborted signal must be reported as an interrupt, not a timeout");
    assert.ok(!text.includes("timed out"), "an interrupt must never be misreported as the 30s timeout firing");
    assert.ok(!text.split("\n").includes("exit code: 0"), "a killed run's exit-code line must never read as a bare, unannotated clean exit");
  });

  test("no timeout line when the command finished on its own (killed:false)", async () => {
    const tool = registerAndCapture(async () => ({ stdout: "done", stderr: "", code: 0, killed: false }));
    const result = await tool.execute("call-3", { command: "true", why: "confirm no spurious timeout line" });
    assert.ok(!result.content[0].text.includes("timed out"));
  });

  test("stdout and stderr are merged (same concatenation order as ws-execute's own pre-run) and run through the fixed byte cap", async () => {
    let sawCap: number | undefined;
    const oversized = "y".repeat(ONE_LINER_OUTPUT_CAP_BYTES + 500);
    const tool = registerAndCapture(async () => ({ stdout: oversized, stderr: "-stderr-tail", code: 0, killed: false }));
    const result = await tool.execute("call-4", { command: "big-output", why: "trigger the cap" });
    const text = result.content[0].text;
    sawCap = Buffer.byteLength(text, "utf8");
    assert.ok(sawCap < Buffer.byteLength(oversized, "utf8") + Buffer.byteLength("-stderr-tail", "utf8"), "the merged output must actually be capped, not passed through raw");
    assert.ok(text.includes("truncated"), "the capped output must carry its own drop-hint");
  });

  test("pi.exec is called with the fixed 30s timeout and the session's own cwd — no cwd/env override param exists", async () => {
    let capturedOptions: { cwd?: string; timeout?: number } | undefined;
    const tool = registerAndCapture(async (_command, _args, options) => {
      capturedOptions = options;
      return { stdout: "", stderr: "", code: 0, killed: false };
    });
    await tool.execute("call-5", { command: "pwd", why: "confirm cwd/timeout wiring" });
    assert.equal(capturedOptions?.cwd, "/tmp/ws-pi-agent-one-liner-test");
    assert.equal(capturedOptions?.timeout, ONE_LINER_TIMEOUT_MS);
  });
});

/**
 * 260906 Phase 2 (dispatch-row rendering): `ws-execute`'s `onModelResolved`
 * forwarding, both for a named `complex:false` tier hit and a `complex:true`
 * inherit — mirrors `test/spawner.test.ts`'s "spawnAgent: onModelResolved"
 * describe block, driven at the `ws-execute` tool level instead.
 */
describe("ws-execute: onModelResolved forwarding (260906 Phase 2)", () => {
  interface CapturedTool {
    execute: (
      id: string,
      params: unknown,
      signal?: AbortSignal,
      update?: (partial: { content: unknown[]; details?: unknown }) => void,
      ctx?: unknown,
    ) => Promise<{ content: Array<{ type: string; text: string }>; details?: unknown }>;
  }

  function installRpcHarness() {
    const original = Object.fromEntries(["start", "stop", "abort", "onEvent", "prompt", "getState", "setThinkingLevel"].map(name => [name, RpcClient.prototype[name as keyof RpcClient]]));
    Object.assign(RpcClient.prototype, {
      start: async function(this: { options?: { env?: Record<string, string>; args?: string[] } }) { await connectFakeChild(this.options?.env, this.options?.args); }, stop: async () => {}, abort: async () => {},
      onEvent: () => () => {}, prompt: async () => {}, setThinkingLevel: async () => {},
      getState: async () => ({ model: { provider: "pi", id: "small" }, thinkingLevel: "medium", sessionFile: "/tmp/ws-pi-agent-test/session.jsonl" }),
    });
    return { restore: () => { closeFakeChildren(); Object.assign(RpcClient.prototype, original); } };
  }

  function harness(callTool: (name: string, args?: unknown) => Promise<{ content: Array<{ type: string; text: string }> }>) {
    const tools = new Map<string, CapturedTool & { name: string }>();
    const pi = {
      registerTool: (def: { name: string } & CapturedTool) => tools.set(def.name, def),
      sendMessage() {}, sendUserMessage() {},
      exec: async () => ({ stdout: "", stderr: "", code: 0, killed: false }),
    } as unknown as ExtensionAPI;
    const bridge = { client: { callTool }, wsToolNames: [], defaultSessionKeyRef: { current: "lead-key" } } as unknown as Parameters<typeof registerExecuteGateway>[1];
    const registry: RpcAgentRegistry = new Map();
    registerExecuteGateway(pi, bridge, registry, { cwd: "/tmp", executeWorkerPromptPath: "/tmp/fake-execute-worker-guide.md" });
    const ctx = { sessionManager: { getSessionId: () => "test-lead" }, agentStorageRoot: storageRoot(), model: { provider: "lead", id: "large" }, thinkingLevel: "high", modelRegistry: { getAll: () => [{ provider: "openai-codex", id: "gpt-5.6-high" }, { provider: "pi", id: "small" }], hasConfiguredAuth: () => true } };
    return { tool: tools.get(EXECUTE_TOOL_NAME)!, registry, ctx };
  }

  test("complex:false resolves the 'small' alias and forwards onModelResolved as onUpdate details and the final return", async () => {
    const rpc = installRpcHarness();
    try {
      const { tool, registry, ctx } = harness(async (name) => { assert.equal(name, "config.resolve_agent"); return { content: [{ type: "text", text: JSON.stringify({ resolved_from: "pi", model: "gpt-5.6-high", backend: "codex" }) }] }; });
      const updates: Array<{ content: unknown[]; details?: unknown }> = [];
      const raw = await tool.execute("call", { prompt: "investigate", complex: false }, undefined, (partial) => updates.push(partial), ctx);
      const parsed = JSON.parse(raw.content[0]!.text);
      assert.ok(parsed.agent_id);
      const expected = { tier: "small", model: "openai-codex/gpt-5.6-high", effort: undefined, inherited: false };
      assert.equal(updates.length, 1, "onModelResolved fires exactly once");
      assert.deepEqual((updates[0]!.details as { resolved: unknown }).resolved, expected);
      assert.deepEqual((raw.details as { resolved?: unknown } | undefined)?.resolved, expected, "the final return repeats the same shape");
      const record = registry.get(parsed.agent_id)!;
      assert.equal(record.ownership?.home, join(realpathSync(ctx.agentStorageRoot), "ws-agents", "test-lead", parsed.agent_id));
      assert.equal(record.ownership?.sessionPath, record.sessionPath);
      assert.equal(record.modelTier, "small");
      assert.equal(record.modelSource, "tier");
    } finally { rpc.restore(); }
  });

  test("complex:true inherits the lead's own model, never calls config.resolve_agent, and still publishes an inherited resolved line", async () => {
    const rpc = installRpcHarness();
    try {
      const { tool, registry, ctx } = harness(async () => { assert.fail("config.resolve_agent must not be called for complex:true"); });
      const updates: Array<{ content: unknown[]; details?: unknown }> = [];
      const raw = await tool.execute("call", { prompt: "investigate", complex: true }, undefined, (partial) => updates.push(partial), ctx);
      const parsed = JSON.parse(raw.content[0]!.text);
      assert.ok(parsed.agent_id);
      assert.equal(updates.length, 1);
      const resolved = (updates[0]!.details as { resolved: { tier: string; model?: string; effort?: string; inherited: boolean } }).resolved;
      assert.equal(resolved.tier, "inherit");
      assert.equal(resolved.inherited, true);
      assert.equal(resolved.model, "lead/large", "inherits the ctx.model snapshot");
      assert.deepEqual((raw.details as { resolved?: unknown } | undefined)?.resolved, resolved);
      const record = registry.get(parsed.agent_id)!;
      assert.equal(record.modelTier, undefined);
      assert.equal(record.modelSource, "inherit");
    } finally { rpc.restore(); }
  });

  test("an omitted complex forwards the same 'small' alias behavior as complex:false", async () => {
    const rpc = installRpcHarness();
    try {
      const { tool, ctx } = harness(async () => ({ content: [{ type: "text", text: JSON.stringify({ resolved_from: "pi", model: "pi/small" }) }] }));
      const updates: unknown[] = [];
      await tool.execute("call", { prompt: "investigate" }, undefined, (partial) => updates.push(partial), ctx);
      assert.equal(updates.length, 1);
    } finally { rpc.restore(); }
  });
});
const storageRoots = new Set<string>();
afterEach(() => {
  for (const root of storageRoots) rmSync(root, { recursive: true, force: true });
  storageRoots.clear();
});
function storageRoot(): string {
  const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
  storageRoots.add(root);
  return root;
}
