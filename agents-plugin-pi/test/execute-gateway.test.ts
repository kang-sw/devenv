/**
 * Unit tests for execute-gateway.ts's pure-logic seams (260904 Phase 1,
 * end-to-end approval gateway): `buildExecuteWorkerPrompt`,
 * `resolveExecuteModelAlias`, `validatePendingApproval` (the ticket's own
 * `cmd_id` race-binding requirement), `computeLeadActiveTools` (the §8 lead
 * `--tools` reshaping + auto-include-footgun fix), `buildApprovalPromptText`
 * (the §7 payload formatter), `approvalDecisionPath` (the parent/child
 * decision-file path both sides must agree on), `resolveApprovalContextCwd`
 * and `validateApprovalDecisionInput` (review fix, relay #1, CORRECTNESS
 * findings #1/#2), `sliceLines` (review fix, relay #1, TEST finding #4), and
 * `waitForDecisionFile` (review fix, relay #1, TEST finding #5 — needs only a
 * real filesystem + timers, not a subprocess/model, so it does not belong in
 * the live-gate-only bucket below).
 *
 * NOT covered here — genuinely live-gate only, per the plan's Verification
 * Plan split and mirroring test/spawner.test.ts's own documented pure/IO
 * split: `scrapeWorkingContext` (real `git` subprocess calls), and the
 * `ws-worker-exec`/`ws-execute`/`ws-approve`/ugly-read tool `execute()`
 * bodies (all need a live `pi --mode rpc` session or a real `RpcClient`) —
 * their pure inner logic (`sliceLines`, `resolveApprovalContextCwd`,
 * `validateApprovalDecisionInput`) is extracted and covered directly instead.
 * Exercised only by the plan's documented manual verification gate (no
 * provider credentials in this sandbox — deferred, not faked).
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
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  buildExecuteWorkerPrompt,
  resolveExecuteModelAlias,
  validatePendingApproval,
  computeLeadActiveTools,
  buildApprovalPromptText,
  approvalDecisionPath,
  resolveApprovalContextCwd,
  validateApprovalDecisionInput,
  sliceLines,
  capOutput,
  waitForDecisionFile,
  createApprovalRelay,
  EXECUTE_TOOL_NAME,
  APPROVE_TOOL_NAME,
  UGLY_READ_TOOL_NAME,
  ONE_LINER_EXEC_TOOL_NAME,
  ONE_LINER_TIMEOUT_MS,
  ONE_LINER_OUTPUT_CAP_BYTES,
  registerExecuteGateway,
  type PendingApproval,
  type WorkingContext,
} from "../src/execute-gateway.ts";
import { GATED_EXEC_TOOL_NAME, TOOL_GROUPS, type RpcAgentRecord, type RpcAgentRegistry } from "../src/spawner.ts";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

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
    const pending: PendingApproval = { cmdId: "call-1", command: "echo hi" };
    assert.deepEqual(validatePendingApproval(pending, "call-1"), { ok: true });
  });

  test("cmd_id mismatch (stale or wrong agent's id) -> rejected with a reason naming both ids", () => {
    const pending: PendingApproval = { cmdId: "call-1", command: "echo hi" };
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

describe("approvalDecisionPath", () => {
  test("joins sessionDir, the fixed \"approvals\" segment, and <cmdId>.decision.json", () => {
    assert.equal(approvalDecisionPath("/tmp/ws-pi-agent-x", "call-1"), "/tmp/ws-pi-agent-x/approvals/call-1.decision.json");
  });

  test("different cmdIds produce different, non-colliding paths under the same sessionDir", () => {
    const a = approvalDecisionPath("/tmp/ws-pi-agent-x", "call-1");
    const b = approvalDecisionPath("/tmp/ws-pi-agent-x", "call-2");
    assert.notEqual(a, b);
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

describe("sliceLines (review fix, relay #1, TEST finding #4)", () => {
  const raw = ["line1", "line2", "line3", "line4", "line5"].join("\n");

  test("no offset/limit returns the whole file unchanged", () => {
    assert.equal(sliceLines(raw), raw);
  });

  test("offset (1-indexed) starts from that line, to EOF when limit is omitted", () => {
    assert.equal(sliceLines(raw, 3), "line3\nline4\nline5");
  });

  test("limit caps the number of lines returned from the start (or from offset)", () => {
    assert.equal(sliceLines(raw, undefined, 2), "line1\nline2");
    assert.equal(sliceLines(raw, 2, 2), "line2\nline3");
  });

  test("offset beyond EOF returns an empty string", () => {
    assert.equal(sliceLines(raw, 100), "");
  });

  test("limit 0 returns an empty string", () => {
    assert.equal(sliceLines(raw, 1, 0), "");
  });

  test("limit extending past EOF is clamped to the last available line, never throws", () => {
    assert.equal(sliceLines(raw, 4, 100), "line4\nline5");
  });

  test("offset 0 or negative is treated the same as offset 1 (start of file)", () => {
    assert.equal(sliceLines(raw, 0), raw);
    assert.equal(sliceLines(raw, -5), raw);
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
    // Each "🙂" is 4 UTF-8 bytes; cutting at 10 bytes lands mid-emoji (byte 8
    // is the second byte of the third emoji), mirroring
    // spawner.test.ts's truncatePromptForStorage multibyte test.
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

describe("waitForDecisionFile (review fix, relay #1, TEST finding #5)", () => {
  async function withTempDir<T>(fn: (dir: string) => Promise<T>): Promise<T> {
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-agent-decision-test-"));
    try {
      return await fn(dir);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  }

  test("resolves with the parsed decision as soon as the file appears (short poll interval, real timers)", async () => {
    await withTempDir(async (dir) => {
      const path = join(dir, "call-1.decision.json");
      const decision = { decision: "approve" as const };
      setTimeout(() => writeFileSync(path, JSON.stringify(decision)), 20);
      const result = await waitForDecisionFile(path, undefined, 5);
      assert.deepEqual(result, decision);
    });
  });

  test("a pre-aborted signal resolves immediately with \"aborted\", never touching the filesystem poll", async () => {
    await withTempDir(async (dir) => {
      const path = join(dir, "never-written.decision.json");
      const controller = new AbortController();
      controller.abort();
      const result = await waitForDecisionFile(path, controller.signal, 5);
      assert.equal(result, "aborted");
    });
  });

  test("aborting mid-poll resolves with \"aborted\" and stops polling (no late resolution once the file later appears)", async () => {
    await withTempDir(async (dir) => {
      const path = join(dir, "call-2.decision.json");
      const controller = new AbortController();
      setTimeout(() => controller.abort(), 15);
      const resultPromise = waitForDecisionFile(path, controller.signal, 5);
      const result = await resultPromise;
      assert.equal(result, "aborted");
      // Writing the file after abort must not cause a second resolution (the
      // promise already settled) — this only verifies no throw/hang occurs.
      writeFileSync(path, JSON.stringify({ decision: "approve" }));
    });
  });

  test("a partially-written (malformed JSON) file is tolerated — polling continues until a valid decision file appears", async () => {
    await withTempDir(async (dir) => {
      const path = join(dir, "call-3.decision.json");
      writeFileSync(path, "{not valid json");
      setTimeout(() => writeFileSync(path, JSON.stringify({ decision: "deny", reason: "no" })), 20);
      const result = await waitForDecisionFile(path, undefined, 5);
      assert.deepEqual(result, { decision: "deny", reason: "no" });
    });
  });
});

describe("createApprovalRelay (260905: unconditional ws-agent-approval push)", () => {
  function freshRecord(pending: PendingApproval): RpcAgentRecord {
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
    const api = {
      sendMessage: (message: unknown, options?: unknown) => {
        sent.push({ message: message as never, options: options as never });
      },
      sendUserMessage: () => {
        throw new Error("the approval relay must push a custom message, never a bare user message");
      },
    } as unknown as ExtensionAPI;
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

  test("a timed-out command (killed:true) resolves normally with the partial output plus a timeout line — never a thrown error", async () => {
    const tool = registerAndCapture(async () => ({ stdout: "partial", stderr: "", code: 143, killed: true }));
    const result = await tool.execute("call-2", { command: "sleep 60", why: "check a slow command" });
    const text = result.content[0].text;
    assert.ok(text.includes("partial"), "partial output must still be returned");
    assert.ok(text.includes("timed out"), "a timeout line must be appended when the timeout fired");
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
