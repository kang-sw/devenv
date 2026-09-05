/**
 * Unit tests for fork.ts's pure-logic seams (260904 Phase 1, side-thread
 * fork ticket): `computeForkToolSurface`/`addForkToolIfLead` (§3's dynamic
 * tool-surface formula, including the role-differentiation fix — a fork
 * must never regain `ws-fork`), `shouldNudge`/`classifyForkTurnOutcome`/
 * `isIdleWithoutFinal` (§4's anti-bleed disambiguation), `extractReportField`/
 * `validateFinalReportShape`/`checkExpectsCommitCompletion` (§4's required
 * report shape + `expects_commit` non-completion rule), `tailLines`, and
 * `getForkSourceSessionFile`.
 *
 * `wireAntiBleedLoop`'s own event handling is additionally driven here
 * `wireAntiBleedLoop`'s own event handling is additionally driven here
 * against a duck-typed fake client/record for the seams 260904 Phase 2 added
 * (the thread-bound suppression, and the question-report hook through its
 * real `applyRpcEvent` call site) and for 260905's push model (every advisory
 * is a `ws-agent-advisory` push, the nudge routes through `promptAgent`, and
 * the idle-without-final judgment reads `reportLog` filtered by the last lead
 * prompt).
 * Review relay #1 (C1/I1/I4) adds: `buildForkSpawnCtx` — the `ws-fork` spawn
 * ctx, extracted so its (required, silently droppable) `pi` field is asserted
 * rather than assumed — with a push-on-final check through the real
 * `attachEventListener`; `armForkRoleWiring`, the shared question-routing +
 * anti-bleed arm used by both a fresh spawn and the shutdown sidecar's orphan
 * revival; and `deliverAs`/status-line assertions on the advisory pushes (the
 * harness now captures `sendMessage`'s options argument, which it previously
 * dropped).
 *
 * NOT covered here — genuinely live-gate only, mirroring
 * test/execute-gateway.test.ts's own pure/IO split: `registerFork`'s tool
 * `execute()` body (needs a live `pi --mode rpc` session or a real
 * `RpcClient`). Exercised only by the plan's documented manual verification
 * gate (no provider credentials in this sandbox — deferred, not faked).
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import {
  FORK_TOOL_NAME,
  FORK_EXCLUDED_TOOL_NAMES,
  computeForkToolSurface,
  addForkToolIfLead,
  MAX_FORK_NUDGES,
  shouldNudge,
  classifyForkTurnOutcome,
  isIdleWithoutFinal,
  REQUIRED_FINAL_REPORT_FIELDS,
  extractReportField,
  validateFinalReportShape,
  checkExpectsCommitCompletion,
  tailLines,
  getForkSourceSessionFile,
  buildForkDirectiveText,
  buildForkInitialMessage,
  wireAntiBleedLoop,
  armForkRoleWiring,
  buildForkSpawnCtx,
} from "../src/fork.ts";
import { applyRpcEvent, attachEventListener, REPORT_TO_LEAD_TOOL_NAME, type RpcAgentRecord, type RpcAgentRegistry } from "../src/spawner.ts";
import type { BridgeHandle } from "../src/bridge.ts";
import type { ExtensionAPI, RpcClient } from "@earendil-works/pi-coding-agent";

describe("FORK_TOOL_NAME / FORK_EXCLUDED_TOOL_NAMES", () => {
  test("FORK_TOOL_NAME is the literal ws-fork", () => {
    assert.equal(FORK_TOOL_NAME, "ws-fork");
  });

  test("excludes ws-fork itself plus Phase 2's ws-ask/ws-resolve (a fork's only question path is ws-report-to-lead)", () => {
    assert.deepEqual([...FORK_EXCLUDED_TOOL_NAMES].sort(), [FORK_TOOL_NAME, "ws-ask", "ws-resolve"].sort());
  });
});

describe("computeForkToolSurface", () => {
  test("removes ws-fork from the lead's active tools and adds ws-report-to-lead", () => {
    const result = computeForkToolSurface(["bash", "edit", FORK_TOOL_NAME, "ws-agent-spawn"]);
    assert.ok(!result.includes(FORK_TOOL_NAME), "a fork's own surface must never include ws-fork (no recursive forking)");
    assert.ok(result.includes(REPORT_TO_LEAD_TOOL_NAME));
    assert.ok(result.includes("bash"));
    assert.ok(result.includes("edit"));
    assert.ok(result.includes("ws-agent-spawn"));
  });

  test("never duplicates ws-report-to-lead if the lead's active tools already carry it", () => {
    const result = computeForkToolSurface(["bash", REPORT_TO_LEAD_TOOL_NAME]);
    assert.equal(result.filter((name) => name === REPORT_TO_LEAD_TOOL_NAME).length, 1);
  });

  test("an empty lead tool list still ends up with exactly ws-report-to-lead", () => {
    assert.deepEqual(computeForkToolSurface([]), [REPORT_TO_LEAD_TOOL_NAME]);
  });

  test("also removes the Phase 2 owner-question primitives ws-ask/ws-resolve", () => {
    const result = computeForkToolSurface(["bash", "ws-ask", "ws-resolve", FORK_TOOL_NAME]);
    assert.ok(!result.includes("ws-ask"), 'a fork\'s only question path is ws-report-to-lead(kind:"question")');
    assert.ok(!result.includes("ws-resolve"));
    assert.deepEqual([...result].sort(), ["bash", REPORT_TO_LEAD_TOOL_NAME].sort());
  });

  test("a lead surface with no ws-fork present is unaffected besides the ws-report-to-lead addition", () => {
    const result = computeForkToolSurface(["bash", "edit", "write"]);
    assert.deepEqual([...result].sort(), ["bash", "edit", REPORT_TO_LEAD_TOOL_NAME, "write"].sort());
  });
});

describe("addForkToolIfLead (risk-signal fix: role-differentiated, never folded into computeLeadActiveTools)", () => {
  test("role undefined (true top lead) gains ws-fork when absent", () => {
    const result = addForkToolIfLead(["bash", "edit"], undefined);
    assert.ok(result.includes(FORK_TOOL_NAME));
  });

  test("role undefined does not duplicate ws-fork if already present", () => {
    const result = addForkToolIfLead(["bash", FORK_TOOL_NAME], undefined);
    assert.equal(result.filter((name) => name === FORK_TOOL_NAME).length, 1);
  });

  test('role "fork" NEVER regains ws-fork, even if somehow present in the input (defense in depth)', () => {
    const withoutIt = addForkToolIfLead(["bash", "edit"], "fork");
    assert.ok(!withoutIt.includes(FORK_TOOL_NAME));
    const alreadyPresent = addForkToolIfLead(["bash", FORK_TOOL_NAME], "fork");
    assert.deepEqual(alreadyPresent, ["bash", FORK_TOOL_NAME], "an existing entry is left as-is, but never newly added");
  });

  test('role "worker" and "explore" never gain ws-fork', () => {
    assert.ok(!addForkToolIfLead(["bash"], "worker").includes(FORK_TOOL_NAME));
    assert.ok(!addForkToolIfLead(["bash"], "explore").includes(FORK_TOOL_NAME));
  });

  test("an empty active-tools list for the true lead ends up with exactly ws-fork", () => {
    assert.deepEqual(addForkToolIfLead([], undefined), [FORK_TOOL_NAME]);
  });
});

describe("shouldNudge", () => {
  test("allows nudging while nudgeCount is below MAX_FORK_NUDGES", () => {
    for (let i = 0; i < MAX_FORK_NUDGES; i++) {
      assert.equal(shouldNudge(i), true, `expected shouldNudge(${i}) to be true`);
    }
  });

  test("refuses once nudgeCount has reached MAX_FORK_NUDGES (the fail-loud transition)", () => {
    assert.equal(shouldNudge(MAX_FORK_NUDGES), false);
    assert.equal(shouldNudge(MAX_FORK_NUDGES + 1), false);
  });
});

describe("classifyForkTurnOutcome (§4 disambiguation table)", () => {
  test('reportKind "question" always classifies as "question", tool call or not', () => {
    assert.equal(classifyForkTurnOutcome({ hadToolCall: true, reportKind: "question" }), "question");
    assert.equal(classifyForkTurnOutcome({ hadToolCall: false, reportKind: "question" }), "question");
  });

  test('reportKind "final" always classifies as "final", tool call or not', () => {
    assert.equal(classifyForkTurnOutcome({ hadToolCall: true, reportKind: "final" }), "final");
    assert.equal(classifyForkTurnOutcome({ hadToolCall: false, reportKind: "final" }), "final");
  });

  test('no report but a tool call happened -> "acknowledge-and-return" (not itself a bleed signal)', () => {
    assert.equal(classifyForkTurnOutcome({ hadToolCall: true }), "acknowledge-and-return");
  });

  test('neither a report nor a tool call -> "no-signal" (the actual bleed condition)', () => {
    assert.equal(classifyForkTurnOutcome({ hadToolCall: false }), "no-signal");
  });
});

describe("isIdleWithoutFinal", () => {
  test("true when no kind in the list is \"final\"", () => {
    assert.equal(isIdleWithoutFinal([undefined, "question", undefined]), true);
  });

  test("false as soon as any kind is \"final\"", () => {
    assert.equal(isIdleWithoutFinal(["question", "final"]), false);
  });

  test("an empty list is vacuously true (no final report was ever seen)", () => {
    assert.equal(isIdleWithoutFinal([]), true);
  });
});

describe("extractReportField", () => {
  const message = ["Outcome: did the thing", "Files changed: a.ts, b.ts", "Commit: none", "  Blockers:   none  "].join("\n");

  test("extracts the trimmed value after a matching '<field>:' line", () => {
    assert.equal(extractReportField(message, "Outcome"), "did the thing");
    assert.equal(extractReportField(message, "Files changed"), "a.ts, b.ts");
  });

  test("tolerates leading whitespace on the line itself", () => {
    assert.equal(extractReportField(message, "Blockers"), "none");
  });

  test("returns undefined when the field's line is absent", () => {
    assert.equal(extractReportField(message, "Verification"), undefined);
  });

  test("does not match a field name that is a substring of another field's line", () => {
    // "Commit" must not accidentally match inside some other longer field.
    assert.equal(extractReportField("Recommit: no", "Commit"), undefined);
  });
});

describe("validateFinalReportShape (§4 required field-prefix check)", () => {
  const wellFormed = [
    "Outcome: did the thing",
    "Files changed: a.ts",
    "Verification: ran npm test, all green",
    "Blockers: none",
    "Commit: abc123",
    "Decisions: chose X over Y",
  ].join("\n");

  test("a message carrying every required field passes", () => {
    assert.deepEqual(validateFinalReportShape(wellFormed), { ok: true });
  });

  test("a message missing one or more required fields lists exactly the missing ones", () => {
    const partial = ["Outcome: did the thing", "Commit: none"].join("\n");
    const result = validateFinalReportShape(partial);
    assert.equal(result.ok, false);
    if (!result.ok) {
      assert.deepEqual(result.missing.sort(), ["Blockers", "Decisions", "Files changed", "Verification"].sort());
    }
  });

  test("an empty message is missing every required field", () => {
    const result = validateFinalReportShape("");
    assert.equal(result.ok, false);
    if (!result.ok) {
      assert.deepEqual(result.missing.sort(), [...REQUIRED_FINAL_REPORT_FIELDS].sort());
    }
  });

  test("Commit: none is valid SHAPE-wise (the expects_commit non-completion rule is a separate check)", () => {
    assert.deepEqual(validateFinalReportShape(wellFormed.replace("Commit: abc123", "Commit: none")), { ok: true });
  });
});

describe("checkExpectsCommitCompletion (§4 expects_commit non-completion rule)", () => {
  test("expects_commit:false never flags anything, regardless of commitLine", () => {
    assert.deepEqual(checkExpectsCommitCompletion(false, undefined), { ok: true });
    assert.deepEqual(checkExpectsCommitCompletion(false, "none"), { ok: true });
    assert.deepEqual(checkExpectsCommitCompletion(false, "abc123"), { ok: true });
  });

  test("expects_commit:true with a missing Commit line is flagged non-completion", () => {
    const result = checkExpectsCommitCompletion(true, undefined);
    assert.equal(result.ok, false);
  });

  test('expects_commit:true with the literal "none" (case/whitespace tolerant) is flagged non-completion', () => {
    for (const commitLine of ["none", "None", "NONE", "  none  "]) {
      const result = checkExpectsCommitCompletion(true, commitLine);
      assert.equal(result.ok, false, `expected commitLine=${JSON.stringify(commitLine)} to be flagged`);
    }
  });

  test("expects_commit:true with a real commit value is accepted", () => {
    assert.deepEqual(checkExpectsCommitCompletion(true, "abc123"), { ok: true });
  });
});

describe("tailLines", () => {
  const text = ["l1", "l2", "l3", "l4", "l5"].join("\n");

  test("returns the last n lines", () => {
    assert.equal(tailLines(text, 2), "l4\nl5");
  });

  test("n beyond the total line count returns the whole text unchanged", () => {
    assert.equal(tailLines(text, 100), text);
  });

  test("n <= 0 returns an empty string", () => {
    assert.equal(tailLines(text, 0), "");
    assert.equal(tailLines(text, -3), "");
  });
});

describe("getForkSourceSessionFile", () => {
  test("extracts the session file path from a well-formed toolCtx.sessionManager.getSessionFile()", () => {
    const toolCtx = { sessionManager: { getSessionFile: () => "/tmp/lead-session.jsonl" } };
    assert.equal(getForkSourceSessionFile(toolCtx), "/tmp/lead-session.jsonl");
  });

  test("returns undefined when toolCtx, sessionManager, or getSessionFile is missing", () => {
    assert.equal(getForkSourceSessionFile(undefined), undefined);
    assert.equal(getForkSourceSessionFile({}), undefined);
    assert.equal(getForkSourceSessionFile({ sessionManager: {} }), undefined);
  });

  test("returns undefined when getSessionFile returns an empty string or a non-string", () => {
    assert.equal(getForkSourceSessionFile({ sessionManager: { getSessionFile: () => "" } }), undefined);
    assert.equal(getForkSourceSessionFile({ sessionManager: { getSessionFile: () => undefined } }), undefined);
  });
});

describe("buildForkDirectiveText", () => {
  test("names the report tool and both kind values, with no identity framing or ALL-CAPS override language", () => {
    const text = buildForkDirectiveText();
    assert.ok(text.includes(REPORT_TO_LEAD_TOOL_NAME));
    assert.ok(text.includes('kind:"question"'));
    assert.ok(text.includes('kind:"final"'));
    for (const field of REQUIRED_FINAL_REPORT_FIELDS) {
      assert.ok(text.includes(`${field}:`), `expected the directive to name required field "${field}"`);
    }
  });

  test("carries no identity-framing persona opener", () => {
    const text = buildForkDirectiveText();
    assert.ok(!/\byou\s+are\s+a\b/i.test(text), `directive text must not open with "you are a ..." identity framing: ${text}`);
  });

  test("carries no ALL-CAPS override-style words", () => {
    const text = buildForkDirectiveText();
    const allCapsWords = text.match(/\b[A-Z]{4,}\b/g) ?? [];
    assert.deepEqual(allCapsWords, [], `directive text must not carry ALL-CAPS override word(s): ${JSON.stringify(allCapsWords)}`);
  });
});

describe("buildForkInitialMessage (260905 structural anti-bleed frame)", () => {
  const task = "Run `od -An -N8 -tx1 /dev/urandom` and report the hex.";

  test("fences the task, demotes inherited context, and keeps the task inline", () => {
    const msg = buildForkInitialMessage(task);
    assert.ok(msg.includes(task), "the lead's task text must survive inside the frame");
    assert.ok(/reference\/background only/i.test(msg), "must demote inherited context to reference");
    assert.ok(msg.includes("--- Message from the lead ---"), "must fence the lead's message");
    assert.ok(msg.includes("--- end of message ---"), "must close the fence");
    assert.ok(msg.includes(REPORT_TO_LEAD_TOOL_NAME), "must keep the report contract pointer");
  });

  test("stays calm — no ALL-CAPS override words (chosen over the aggressive header)", () => {
    const allCapsWords = buildForkInitialMessage(task).match(/\b[A-Z]{4,}\b/g) ?? [];
    assert.deepEqual(allCapsWords, [], `framed message must stay calm: ${JSON.stringify(allCapsWords)}`);
  });
});

/**
 * 260904 Phase 2 (review relay #1 C1/I6), rewritten for 260905's push model:
 * the loop's thread-bound suppression, its advisory PUSHES (formerly
 * `pi.sendUserMessage` steers), the nudge routed through `promptAgent`, and
 * the question-report hook's new suppression contract — driven against a
 * duck-typed fake client/record (no subprocess, no real `RpcClient`). The
 * event stream is replayed by hand through the listener `wireAntiBleedLoop`
 * registers; the hook is exercised through the real `applyRpcEvent` (its
 * actual call site) rather than through the loop.
 */
describe("wireAntiBleedLoop / applyRpcEvent question surface seams (Phase 2, 260905 push model)", () => {
  function harness() {
    // Review relay #1 (I4): the OPTIONS object is captured too. Capturing only
    // the message left every "(followUp)" assertion in this file unverified —
    // a regression back to `deliverAs: "steer"` would have stayed green.
    const pushes: Array<{ customType?: string; details?: Record<string, unknown>; deliverAs?: string; triggerTurn?: boolean }> = [];
    const prompts: string[] = [];
    let listener: ((evt: unknown) => void) | undefined;
    const record = {
      agentId: "a1",
      sessionPath: "/nonexistent/session.jsonl",
      systemPromptPath: "/nonexistent/prompt.md",
      wsToolNames: [],
      toolGroup: "full-worker",
      spawnRole: "fork",
      streaming: false,
      running: false,
      reportLog: [],
      client: {
        onEvent(l: (evt: unknown) => void) {
          listener = l;
          return () => {};
        },
        prompt(message: string) {
          prompts.push(message);
          return Promise.resolve();
        },
      },
    } as unknown as RpcAgentRecord;
    const pi = {
      sendMessage(message: { customType?: string; details?: Record<string, unknown> }, options?: { deliverAs?: string; triggerTurn?: boolean }) {
        pushes.push({ ...message, deliverAs: options?.deliverAs, triggerTurn: options?.triggerTurn });
      },
      sendUserMessage() {
        throw new Error("wireAntiBleedLoop must push custom messages, never a bare user message");
      },
    } as unknown as ExtensionAPI;
    // Two extra live siblings so the status line this loop's registry argument
    // produces is distinguishable from the `0 of 0` an empty/wrong registry
    // would render (review relay #1, I4).
    const registry: RpcAgentRegistry = new Map([
      ["a1", record],
      ["sibling-1", { ...record, agentId: "sibling-1", running: true, reportLog: [] } as RpcAgentRecord],
      ["sibling-2", { ...record, agentId: "sibling-2", running: true, reportLog: [] } as RpcAgentRecord],
    ]);
    return { pi, registry, record, pushes, prompts, emit: (evt: unknown) => listener?.(evt) };
  }

  /** The advisory names pushed for each `ws-agent-advisory` message, in order. */
  function advisories(pushes: Array<{ customType?: string; details?: Record<string, unknown> }>): string[] {
    return pushes.filter((p) => p.customType === "ws-agent-advisory").map((p) => String(p.details?.advisory));
  }

  const questionTurn = [
    { type: "agent_start" },
    { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { kind: "question", message: "Which of the two anchors should I use?" } },
    { type: "agent_settled" },
  ];

  const questionReportEvent = {
    type: "tool_execution_start",
    toolName: REPORT_TO_LEAD_TOOL_NAME,
    args: { kind: "question", message: "Which of the two anchors should I use?" },
  };

  test("a question turn is a valid stop — no nudge, no advisory", () => {
    const h = harness();
    wireAntiBleedLoop(h.pi, h.registry, "a1", h.record, false);
    for (const evt of questionTurn) h.emit(evt);
    assert.deepEqual(advisories(h.pushes), [], "a question turn is a valid stop");
    assert.deepEqual(h.prompts, [], "a question turn is never nudged");
  });

  test("C1 (widened 260905): a bound owner discussion thread suppresses the no-signal nudge", () => {
    const h = harness();
    h.record.threadBound = true;
    wireAntiBleedLoop(h.pi, h.registry, "a1", h.record, false);
    for (let turn = 0; turn < 5; turn += 1) {
      h.emit({ type: "agent_start" });
      h.emit({ type: "agent_settled" });
    }
    assert.deepEqual(h.prompts, [], "a thread-bound fork must never be re-prompted mid-discussion");
    assert.deepEqual(advisories(h.pushes), [], "and must never be reported to the lead as stalled");
  });

  test("C1: the same record still nudges once the thread closes", () => {
    const h = harness();
    h.record.threadBound = true;
    wireAntiBleedLoop(h.pi, h.registry, "a1", h.record, false);
    h.emit({ type: "agent_start" });
    h.emit({ type: "agent_settled" });
    assert.deepEqual(h.prompts, []);
    h.record.threadBound = false;
    h.emit({ type: "agent_start" });
    h.emit({ type: "agent_settled" });
    assert.equal(h.prompts.length, 1, "suppression is scoped to the bound window, not permanent");
  });

  test("260905: the nudge goes through promptAgent — running latches without moving the lead-prompt watermark", () => {
    const h = harness();
    h.record.lastLeadPromptAt = 1_000;
    wireAntiBleedLoop(h.pi, h.registry, "a1", h.record, false);
    h.emit({ type: "agent_start" });
    h.emit({ type: "agent_settled" });
    assert.equal(h.prompts.length, 1);
    assert.equal(h.record.running, true, "the nudged fork is outstanding again");
    assert.equal(h.record.lastLeadPromptAt, 1_000, "an internal re-prompt is not a new task boundary");
  });

  test('260905: a kind:"final" already filed for THIS task stops the idle-without-final flag (reportLog, not the deleted pendingReports)', () => {
    const h = harness();
    h.record.lastLeadPromptAt = 1_000;
    h.record.reportLog.push({ kind: "final", at: 2_000 });
    wireAntiBleedLoop(h.pi, h.registry, "a1", h.record, false);
    h.emit({ type: "agent_start" });
    h.emit({ type: "agent_settled" });
    assert.deepEqual(h.prompts, [], "the fork already completed — a text-only turn after that is not a bleed");
    assert.deepEqual(advisories(h.pushes), []);
  });

  test('260905: a kind:"final" from BEFORE the last lead prompt does NOT count — the stale completion is filtered out', () => {
    const h = harness();
    h.record.lastLeadPromptAt = 5_000;
    h.record.reportLog.push({ kind: "final", at: 1_000 });
    wireAntiBleedLoop(h.pi, h.registry, "a1", h.record, false);
    h.emit({ type: "agent_start" });
    h.emit({ type: "agent_settled" });
    assert.equal(h.prompts.length, 1, "the new task has produced no completion signal yet");
  });

  test("260905: acknowledge-and-return pushes a ws-agent-advisory (followUp), never a bare steer", () => {
    const h = harness();
    wireAntiBleedLoop(h.pi, h.registry, "a1", h.record, false);
    h.emit({ type: "agent_start" });
    h.emit({ type: "tool_execution_start", toolName: "bash", args: {} });
    h.emit({ type: "agent_settled" });
    assert.deepEqual(advisories(h.pushes), ["acknowledge-and-return"]);
    assert.deepEqual(h.prompts, [], "a turn that did real work is not itself a bleed signal");
    // I4: the deliverAs half of this test's own name, previously unasserted.
    assert.equal(h.pushes[0].deliverAs, "followUp", "an advisory is never a steer under the push model");
    assert.equal(h.pushes[0].triggerTurn, true, "an idle lead must act on it rather than leaving it queued");
  });

  test("I4: every advisory family is delivered followUp, never steer", () => {
    const h = harness();
    wireAntiBleedLoop(h.pi, h.registry, "a1", h.record, true);
    // final-report-shape
    h.emit({ type: "agent_start" });
    h.emit({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { kind: "final", message: "all done" } });
    // expects-commit
    h.emit({
      type: "tool_execution_start",
      toolName: REPORT_TO_LEAD_TOOL_NAME,
      args: { kind: "final", message: REQUIRED_FINAL_REPORT_FIELDS.map((f) => (f === "Commit" ? "Commit: none" : `${f}: something`)).join("\n") },
    });
    // stalled (nudge budget exhausted)
    h.record.reportLog.length = 0;
    for (let turn = 0; turn < MAX_FORK_NUDGES + 1; turn += 1) {
      h.emit({ type: "agent_start" });
      h.emit({ type: "agent_settled" });
    }
    assert.deepEqual(advisories(h.pushes), ["final-report-shape", "expects-commit", "stalled"]);
    assert.deepEqual(
      h.pushes.map((p) => p.deliverAs),
      ["followUp", "followUp", "followUp"],
    );
  });

  test("I4: an advisory's status line is computed from the registry actually threaded in, not an empty one", () => {
    const h = harness();
    wireAntiBleedLoop(h.pi, h.registry, "a1", h.record, false);
    h.emit({ type: "agent_start" });
    h.emit({ type: "tool_execution_start", toolName: "bash", args: {} });
    h.emit({ type: "agent_settled" });
    assert.deepEqual(advisories(h.pushes), ["acknowledge-and-return"]);
    assert.equal(
      h.pushes[0].details?.status,
      "2 of 3 delegated agents still running",
      "the two live siblings are what distinguish the real shared registry from an empty stand-in",
    );
    assert.equal(h.pushes[0].details?.agent_id, "a1");
  });

  test("260905: exhausting the nudge budget pushes a `stalled` advisory carrying a transcript tail", () => {
    const h = harness();
    wireAntiBleedLoop(h.pi, h.registry, "a1", h.record, false);
    for (let turn = 0; turn < MAX_FORK_NUDGES + 1; turn += 1) {
      h.emit({ type: "agent_start" });
      h.emit({ type: "agent_settled" });
    }
    assert.equal(h.prompts.length, MAX_FORK_NUDGES);
    assert.deepEqual(advisories(h.pushes), ["stalled"]);
    const stalled = h.pushes.find((p) => p.details?.advisory === "stalled");
    assert.ok(typeof stalled?.details?.transcript_tail === "string", "an unreadable transcript degrades to a placeholder, never to a missing field");
  });

  test('260905: a malformed kind:"final" pushes a final-report-shape advisory', () => {
    const h = harness();
    wireAntiBleedLoop(h.pi, h.registry, "a1", h.record, false);
    h.emit({ type: "agent_start" });
    h.emit({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { kind: "final", message: "all done" } });
    assert.deepEqual(advisories(h.pushes), ["final-report-shape"]);
  });

  test('260905: expects_commit with Commit: none pushes an expects-commit advisory', () => {
    const h = harness();
    wireAntiBleedLoop(h.pi, h.registry, "a1", h.record, true);
    h.emit({ type: "agent_start" });
    h.emit({
      type: "tool_execution_start",
      toolName: REPORT_TO_LEAD_TOOL_NAME,
      args: { kind: "final", message: REQUIRED_FINAL_REPORT_FIELDS.map((f) => (f === "Commit" ? "Commit: none" : `${f}: something`)).join("\n") },
    });
    assert.deepEqual(advisories(h.pushes), ["expects-commit"]);
  });

  test("260905: a well-formed final with a real Commit pushes no advisory at all", () => {
    const h = harness();
    wireAntiBleedLoop(h.pi, h.registry, "a1", h.record, true);
    h.emit({ type: "agent_start" });
    h.emit({
      type: "tool_execution_start",
      toolName: REPORT_TO_LEAD_TOOL_NAME,
      args: { kind: "final", message: REQUIRED_FINAL_REPORT_FIELDS.map((f) => (f === "Commit" ? "Commit: abc1234" : `${f}: something`)).join("\n") },
    });
    assert.deepEqual(advisories(h.pushes), []);
  });

  test("I6 (260905): a hook return SUPPRESSES the question push — the owner surface consumed it", () => {
    const h = harness();
    const seen: Array<{ agentId: string; message: string }> = [];
    h.record.onQuestionReport = (rec, message) => {
      seen.push({ agentId: rec.agentId, message });
      return "[ws] thread T1 — the owner answers this.";
    };
    const outcome = applyRpcEvent(h.record, questionReportEvent);
    assert.deepEqual(seen, [{ agentId: "a1", message: "Which of the two anchors should I use?" }]);
    assert.deepEqual(outcome, {}, "§1 keeps the lead out of a fork-raised question entirely");
    assert.equal(h.record.reportLog.length, 1, "the report is still logged for the anti-bleed loop");
  });

  test("I6: returning undefined (headless) keeps the ws-agent-question push", () => {
    const h = harness();
    h.record.onQuestionReport = () => undefined;
    assert.deepEqual(applyRpcEvent(h.record, questionReportEvent), {
      push: { family: "ws-agent-question", payload: { question: "Which of the two anchors should I use?" }, deliverAs: "steer" },
    });
  });

  test("I6: a throwing hook degrades to the headless baseline rather than dropping the question", () => {
    const h = harness();
    h.record.onQuestionReport = () => {
      throw new Error("boom");
    };
    assert.deepEqual(applyRpcEvent(h.record, questionReportEvent), {
      push: { family: "ws-agent-question", payload: { question: "Which of the two anchors should I use?" }, deliverAs: "steer" },
    });
  });

  test("I6: a final-kind report never reaches the question hook", () => {
    const h = harness();
    let calls = 0;
    h.record.onQuestionReport = () => {
      calls += 1;
      return "replaced";
    };
    const outcome = applyRpcEvent(h.record, {
      type: "tool_execution_start",
      toolName: REPORT_TO_LEAD_TOOL_NAME,
      args: { kind: "final", message: "Outcome: x" },
    });
    assert.equal(calls, 0);
    assert.deepEqual(outcome, { push: { family: "ws-agent-report", payload: { kind: "final", report: "Outcome: x" }, deliverAs: "followUp" } });
  });

  test("I6: with no hook set the question is pushed to the lead", () => {
    const h = harness();
    assert.deepEqual(applyRpcEvent(h.record, questionReportEvent), {
      push: { family: "ws-agent-question", payload: { question: "Which of the two anchors should I use?" }, deliverAs: "steer" },
    });
  });
});

/**
 * Review relay #1, C1: `RpcSpawnCtx.pi` became a REQUIRED field with the push
 * model and `registerFork`'s own ctx literal was the one call site that never
 * got it — silently, since this package has no `tsc` step. The result was that
 * every `ws-fork` child ran with `ctx.pi === undefined`, so `pushToLead`'s
 * `if (!pi) return` guard dropped its `kind:"final"` report (the lead's only
 * completion signal now that `ws-agent-wait` is gone), its settles, and its
 * headless questions. These tests pin both halves: the ctx carries `pi`, and a
 * record wired from that ctx actually pushes on a final.
 */
describe("buildForkSpawnCtx (the ws-fork push channel)", () => {
  const bridge = { wsToolNames: ["ws__ferrule"], defaultSessionKeyRef: { current: "amber-otter-canyon" } } as unknown as BridgeHandle;
  const pi = { sendMessage() {} } as unknown as ExtensionAPI;

  test("carries the spawning session's own pi — without it a fork has no report channel at all", () => {
    const ctx = buildForkSpawnCtx(pi, bridge, { cwd: "/repo", modelCatalogPath: "/repo/model-catalog.json" }, {
      forkFrom: "/tmp/lead-session.jsonl",
      explicitTools: "read,grep",
    });
    assert.equal(ctx.pi, pi, "a ws-fork spawn must push into the session that spawned it");
  });

  test("carries the fork spawn shape: --fork source, explicit tools, parent session key, and spawnRole fork", () => {
    const ctx = buildForkSpawnCtx(pi, bridge, { cwd: "/repo", modelCatalogPath: "/repo/model-catalog.json" }, {
      forkFrom: "/tmp/lead-session.jsonl",
      explicitTools: "read,grep",
      inheritModel: "openrouter/some-model",
    });
    assert.equal(ctx.forkFrom, "/tmp/lead-session.jsonl");
    assert.equal(ctx.explicitTools, "read,grep");
    assert.equal(ctx.parentSessionKey, "amber-otter-canyon");
    assert.equal(ctx.spawnRole, "fork");
    assert.equal(ctx.inheritModel, "openrouter/some-model");
    assert.equal(ctx.cwd, "/repo");
    assert.deepEqual([...ctx.wsToolNames], ["ws__ferrule"]);
  });

  test("C1: a record wired through that ctx's pi pushes ws-agent-report on the fork's own final", () => {
    const sent: Array<{ customType?: string; details?: Record<string, unknown> }> = [];
    const pushPi = { sendMessage: (m: { customType?: string; details?: Record<string, unknown> }) => void sent.push(m) } as unknown as ExtensionAPI;
    const ctx = buildForkSpawnCtx(pushPi, bridge, { cwd: "/repo", modelCatalogPath: "/c.json" }, {
      forkFrom: "/tmp/lead-session.jsonl",
      explicitTools: "read",
    });

    let listener: ((evt: unknown) => void) | undefined;
    const client = {
      onEvent(l: (evt: unknown) => void) {
        listener = l;
        return () => {};
      },
      getState: async () => ({}),
    } as unknown as RpcClient;
    const record = {
      agentId: "fork-1",
      sessionPath: "/tmp/f.jsonl",
      systemPromptPath: "/tmp/p.md",
      wsToolNames: [],
      toolGroup: "full-worker",
      spawnRole: "fork",
      streaming: false,
      running: true,
      reportLog: [],
      client,
    } as unknown as RpcAgentRecord;
    const registry: RpcAgentRegistry = new Map([["fork-1", record]]);

    // Exactly what spawnAgent does with the ctx it is handed.
    attachEventListener(ctx.pi, registry, record, client);
    listener?.({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { kind: "final", message: "Outcome: shipped" } });

    assert.deepEqual(
      sent.map((m) => m.customType),
      ["ws-agent-report"],
      "a fork's final is the lead's completion signal — dropping it strands the whole ws-fork surface",
    );
    assert.equal(sent[0].details?.report, "Outcome: shipped");
    assert.equal(sent[0].details?.agent_id, "fork-1");
  });
});

/**
 * Review relay #1, I1: role wiring must be re-armable on a record the shutdown
 * sidecar revived as DORMANT, not only on a freshly-spawned live one.
 */
describe("armForkRoleWiring (fresh spawn and sidecar revival)", () => {
  const pi = { sendMessage() {} } as unknown as ExtensionAPI;

  function dormantForkRecord(): RpcAgentRecord {
    return {
      agentId: "fork-1",
      sessionPath: "/tmp/f.jsonl",
      systemPromptPath: "/tmp/p.md",
      wsToolNames: [],
      toolGroup: "full-worker",
      spawnRole: "fork",
      streaming: false,
      running: false,
      reportLog: [],
    } as unknown as RpcAgentRecord;
  }

  test("arms question routing on a dormant record — a revived fork's question still reaches the owner surface", () => {
    const record = dormantForkRecord();
    const asked: Array<{ agentId: string; message: string }> = [];
    armForkRoleWiring(pi, new Map([["fork-1", record]]), record, (agentId, message) => {
      asked.push({ agentId, message });
      return "[ws] thread q1 — the owner answers this.";
    });

    const outcome = applyRpcEvent(record, {
      type: "tool_execution_start",
      toolName: REPORT_TO_LEAD_TOOL_NAME,
      args: { kind: "question", message: "which anchor?" },
    });
    assert.deepEqual(asked, [{ agentId: "fork-1", message: "which anchor?" }]);
    assert.deepEqual(outcome, {}, "§1: routed to the owner, never pushed at the lead as ws-agent-question");
  });

  test("defers the anti-bleed loop to onResume when the record is dormant, and arms it immediately when it is live", () => {
    const dormant = dormantForkRecord();
    armForkRoleWiring(pi, new Map([["fork-1", dormant]]), dormant);
    assert.equal(typeof dormant.onResume, "function", "wireAntiBleedLoop needs a client the dormant record has not got yet");

    let subscriptions = 0;
    const client = {
      onEvent() {
        subscriptions += 1;
        return () => {};
      },
    } as unknown as RpcClient;
    dormant.client = client;
    dormant.onResume?.(dormant);
    assert.equal(subscriptions, 1, "the resume is what restores the loop");

    const live = dormantForkRecord();
    live.client = client;
    armForkRoleWiring(pi, new Map([["fork-1", live]]), live);
    assert.equal(subscriptions, 2, "an already-live record is wired straight away");
  });
});
