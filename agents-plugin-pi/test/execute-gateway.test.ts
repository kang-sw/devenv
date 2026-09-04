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
 * bodies plus `createApprovalRelay`'s `pi.sendUserMessage` call (all need a
 * live `pi --mode rpc` session or a real `RpcClient`) — their pure inner
 * logic (`sliceLines`, `resolveApprovalContextCwd`,
 * `validateApprovalDecisionInput`) is extracted and covered directly instead.
 * Exercised only by the plan's documented manual verification gate (no
 * provider credentials in this sandbox — deferred, not faked).
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
  waitForDecisionFile,
  EXECUTE_TOOL_NAME,
  APPROVE_TOOL_NAME,
  UGLY_READ_TOOL_NAME,
  type PendingApproval,
  type WorkingContext,
} from "../src/execute-gateway.ts";
import { GATED_EXEC_TOOL_NAME } from "../src/spawner.ts";

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
  test("complex:true resolves to the \"complex\" alias", () => {
    assert.equal(resolveExecuteModelAlias(true), "complex");
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
  });

  test("is idempotent — running it twice in a row never re-adds a removed tool or duplicates an added one", () => {
    const once = computeLeadActiveTools(["bash", "read", GATED_EXEC_TOOL_NAME, "ws-agent-spawn", "explore"]);
    const twice = computeLeadActiveTools(once);
    assert.deepEqual([...twice].sort(), [...once].sort());
    assert.equal(new Set(twice).size, twice.length, "no duplicate entries after a 2nd pass");
  });

  test("never duplicates ws-execute/ws-approve/the ugly-read tool if the current list already carries them (e.g. after an earlier setActiveTools call)", () => {
    const result = computeLeadActiveTools(["ws-agent-spawn", EXECUTE_TOOL_NAME, APPROVE_TOOL_NAME, UGLY_READ_TOOL_NAME]);
    assert.equal(result.filter((name) => name === EXECUTE_TOOL_NAME).length, 1);
    assert.equal(result.filter((name) => name === APPROVE_TOOL_NAME).length, 1);
    assert.equal(result.filter((name) => name === UGLY_READ_TOOL_NAME).length, 1);
  });

  test("an empty current list still ends up with exactly the 3 added tools", () => {
    assert.deepEqual([...computeLeadActiveTools([])].sort(), [APPROVE_TOOL_NAME, EXECUTE_TOOL_NAME, UGLY_READ_TOOL_NAME].sort());
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
