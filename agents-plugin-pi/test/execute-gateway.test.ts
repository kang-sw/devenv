/**
 * Unit tests for execute-gateway.ts's pure-logic seams (260904 Phase 1,
 * end-to-end approval gateway): `buildExecuteWorkerPrompt`,
 * `resolveExecuteModelAlias`, `validatePendingApproval` (the ticket's own
 * `cmd_id` race-binding requirement), `computeLeadActiveTools` (the §8 lead
 * `--tools` reshaping + auto-include-footgun fix), `buildApprovalPromptText`
 * (the §7 payload formatter), and `approvalDecisionPath` (the parent/child
 * decision-file path both sides must agree on).
 *
 * NOT covered here — genuinely live-gate only, per the plan's Verification
 * Plan split and mirroring test/spawner.test.ts's own documented pure/IO
 * split: `scrapeWorkingContext` (real `git` subprocess calls),
 * `waitForDecisionFile` (real filesystem polling + timers), and the
 * `ws-worker-exec`/`ws-execute`/`ws-approve`/ugly-read tool `execute()`
 * bodies plus `createApprovalRelay`'s `pi.sendUserMessage` call (all need a
 * live `pi --mode rpc` session or a real `RpcClient`/filesystem). Exercised
 * only by the plan's documented manual verification gate (no provider
 * credentials in this sandbox — deferred, not faked).
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import {
  buildExecuteWorkerPrompt,
  resolveExecuteModelAlias,
  validatePendingApproval,
  computeLeadActiveTools,
  buildApprovalPromptText,
  approvalDecisionPath,
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
