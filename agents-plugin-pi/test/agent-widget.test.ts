/**
 * Unit tests for agent-widget.ts's pure row/render seam — `buildAgentRows`,
 * `buildWidgetLines`, `buildStatusSegment` — driven with duck-typed fake
 * `RpcAgentRecord`/`ThreadRecord` values, no live `pi` session or RPC client.
 * `createAgentWidgetController` (the `ctx.ui.setWidget`/`setStatus` IO glue
 * and the 10-second elapsed timer) is left untested here, same live-gate
 * split `ask.ts`/`spawner.ts` already use between pure helpers and their
 * `registerX`/`createX` IO functions — see this ticket's plan and
 * agent-widget.ts's own header comment.
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { buildAgentRows, buildWidgetLines, buildStatusSegment, shouldArmAgentWidget, AGENT_WIDGET_ROW_CAP } from "../src/agent-widget.ts";
import type { RpcAgentRecord, RpcAgentRegistry } from "../src/spawner.ts";
import type { ThreadRecord } from "../src/ask.ts";

const NOW = Date.parse("2026-09-05T10:05:00.000Z");

function record(overrides: Partial<RpcAgentRecord> = {}): RpcAgentRecord {
  return {
    agentId: "11111111-2222-3333-4444-555555555555",
    sessionPath: "/tmp/ws-pi-agent-x/session.jsonl",
    systemPromptPath: "/tmp/ws-pi-agent-x/prompt.md",
    wsToolNames: [],
    toolGroup: "full-worker",
    streaming: false,
    running: false,
    reportLog: [],
    ...overrides,
  };
}

function thread(overrides: Partial<ThreadRecord> = {}): ThreadRecord {
  return {
    threadId: "q1",
    title: "a question",
    status: "open",
    origin: "lead-ask",
    createdAt: "2026-09-05T10:00:00.000Z",
    touchedAt: "2026-09-05T10:00:00.000Z",
    ...overrides,
  };
}

function registryOf(...records: RpcAgentRecord[]): RpcAgentRegistry {
  const map: RpcAgentRegistry = new Map();
  for (const r of records) map.set(r.agentId, r);
  return map;
}

describe("buildAgentRows", () => {
  test("a plain live (client-holding) non-threadBound record is a running row with no answer hint", () => {
    const r = record({ client: {} as never, runStartedAt: NOW - 5_000 });
    const rows = buildAgentRows(registryOf(r), [], NOW);
    assert.deepEqual(rows, [{ name: "11111111", role: "worker", state: "running", elapsedMs: 5_000, answerHint: undefined }]);
  });

  test("name precedence: alias > title > shortened uuid", () => {
    const byAlias = record({ client: {} as never, alias: "scout", title: "irrelevant title" });
    const byTitle = record({ client: {} as never, title: "the title" });
    const byUuid = record({ client: {} as never });
    assert.equal(buildAgentRows(registryOf(byAlias), [], NOW)[0].name, "scout");
    assert.equal(buildAgentRows(registryOf(byTitle), [], NOW)[0].name, "the title");
    assert.equal(buildAgentRows(registryOf(byUuid), [], NOW)[0].name, "11111111");
  });

  test("roleFromSpawnRole: worker -> worker, execute-worker -> execute, fork -> fork, explore -> explore, unset -> worker", () => {
    const worker = record({ client: {} as never, spawnRole: "worker" });
    const exec = record({ client: {} as never, spawnRole: "execute-worker" });
    const fork = record({ client: {} as never, spawnRole: "fork" });
    const explore = record({ client: {} as never, spawnRole: "explore" });
    const unset = record({ client: {} as never });
    assert.equal(buildAgentRows(registryOf(worker), [], NOW)[0].role, "worker");
    assert.equal(buildAgentRows(registryOf(exec), [], NOW)[0].role, "execute");
    assert.equal(buildAgentRows(registryOf(fork), [], NOW)[0].role, "fork");
    assert.equal(buildAgentRows(registryOf(explore), [], NOW)[0].role, "explore");
    assert.equal(buildAgentRows(registryOf(unset), [], NOW)[0].role, "worker");
  });

  test("a plain idle record (no client, not threadBound, no pendingApproval) is excluded entirely", () => {
    const r = record();
    assert.deepEqual(buildAgentRows(registryOf(r), [], NOW), []);
  });

  test("a pendingApproval record is included and ranked awaiting-approval even without a live client", () => {
    const r = record({ pendingApproval: { cmdId: "c1", command: "rm -rf /" } });
    const rows = buildAgentRows(registryOf(r), [], NOW);
    assert.equal(rows.length, 1);
    assert.equal(rows[0].state, "awaiting-approval");
  });

  test("a threadBound record renders even while dormant (no live client) — the owner's action cue must not disappear", () => {
    const r = record({ threadBound: true });
    const rows = buildAgentRows(registryOf(r), [thread({ respondentAgentId: r.agentId, origin: "lead-ask" })], NOW);
    assert.equal(rows.length, 1);
    assert.equal(rows[0].state, "awaiting-owner");
  });

  test("state precedence: threadBound beats pendingApproval beats running", () => {
    const both = record({ threadBound: true, pendingApproval: { cmdId: "c1", command: "x" } });
    const rows = buildAgentRows(registryOf(both), [thread({ respondentAgentId: both.agentId, origin: "lead-ask" })], NOW);
    assert.equal(rows[0].state, "awaiting-owner", "threadBound must win over a simultaneously-set pendingApproval");
  });

  test("role \"thread\" applies ONLY to a threadBound record bound to a lead-ask thread — a fork-raised (Entry A) threadBound record keeps its own spawnRole, but review relay #1 #2: BOTH origins carry the /answer hint and the thread clock, since those follow the awaiting-owner STATE, not the role override", () => {
    const leadAsk = record({ agentId: "aaaaaaaa-0000-0000-0000-000000000000", threadBound: true, spawnRole: "fork" });
    const forkRaised = record({ agentId: "bbbbbbbb-0000-0000-0000-000000000000", threadBound: true, spawnRole: "fork" });
    const threads: ThreadRecord[] = [
      thread({ threadId: "q1", respondentAgentId: leadAsk.agentId, origin: "lead-ask", touchedAt: new Date(NOW - 4_000).toISOString() }),
      thread({ threadId: "q2", respondentAgentId: forkRaised.agentId, origin: "fork-raised", touchedAt: new Date(NOW - 6_000).toISOString() }),
    ];
    const rows = buildAgentRows(registryOf(leadAsk, forkRaised), threads, NOW);
    const leadAskRow = rows.find((row) => row.name === "aaaaaaaa")!;
    const forkRaisedRow = rows.find((row) => row.name === "bbbbbbbb")!;
    assert.equal(leadAskRow.role, "thread");
    assert.equal(leadAskRow.answerHint, "/answer q1");
    assert.equal(leadAskRow.elapsedMs, 4_000);
    assert.equal(forkRaisedRow.role, "fork", "a fork-raised threadBound record is not overridden to \"thread\"");
    assert.equal(forkRaisedRow.answerHint, "/answer q2", "the hint follows the awaiting-owner state, not the role override — the owner owes an answer here too");
    assert.equal(forkRaisedRow.elapsedMs, 6_000, "the fork-raised row's clock is the thread's touchedAt, not runStartedAt");
  });

  test("review relay #1 Critical: a pending ws-ask thread with NO respondent yet (empty registry) still renders one row, named by the thread's own title, carrying the /answer hint", () => {
    const t = thread({ threadId: "q9", title: "why is the build red", status: "pending", origin: "lead-ask", respondentAgentId: undefined, touchedAt: new Date(NOW - 5_000).toISOString() });
    const rows = buildAgentRows(registryOf(), [t], NOW);
    assert.deepEqual(rows, [{ name: "why is the build red", role: "thread", state: "awaiting-owner", elapsedMs: 5_000, answerHint: "/answer q9" }]);
  });

  test("review relay #1 Critical: the lead-restart scenario — a thread with a respondentAgentId but no matching registry record (reviveOrphans never re-sets threadBound) — still renders one row", () => {
    const t = thread({ threadId: "q10", title: "post-restart question", status: "open", origin: "fork-raised", respondentAgentId: "ffffffff-0000-0000-0000-000000000000", touchedAt: new Date(NOW - 9_000).toISOString() });
    // The orphan record IS in the registry (reviveOrphans always re-registers it), just not threadBound.
    const orphan = record({ agentId: "ffffffff-0000-0000-0000-000000000000" });
    const rows = buildAgentRows(registryOf(orphan), [t], NOW);
    assert.deepEqual(rows, [{ name: "post-restart question", role: "thread", state: "awaiting-owner", elapsedMs: 9_000, answerHint: "/answer q10" }]);
  });

  test("review relay #1 Critical: dedupe — once a live threadBound record covers the thread, no second synthetic row is added for the same thread", () => {
    const respondent = record({ agentId: "12121212-0000-0000-0000-000000000000", threadBound: true });
    const t = thread({ threadId: "q11", respondentAgentId: respondent.agentId, origin: "lead-ask" });
    const rows = buildAgentRows(registryOf(respondent), [t], NOW);
    assert.equal(rows.length, 1, "exactly one row for the one live thread, not two");
    assert.equal(rows[0].name, respondent.agentId.slice(0, 8), "the covered row is named from the RECORD, not synthesized from the thread's title");
  });

  test("a dormant/closed thread yields no row at all, whether or not a respondent record exists", () => {
    const closed = thread({ threadId: "q12", status: "closed", respondentAgentId: undefined });
    const dormant = thread({ threadId: "q13", status: "dormant", respondentAgentId: undefined });
    assert.deepEqual(buildAgentRows(registryOf(), [closed, dormant], NOW), []);
  });

  test("elapsed: a thread row uses now - Date.parse(touchedAt); a non-thread row uses now - runStartedAt, defaulting to 0 when never prompted", () => {
    const bound = record({ threadBound: true });
    const running = record({ agentId: "cccccccc-0000-0000-0000-000000000000", client: {} as never, runStartedAt: NOW - 3_000 });
    const neverPrompted = record({ agentId: "dddddddd-0000-0000-0000-000000000000", client: {} as never });
    const threads: ThreadRecord[] = [thread({ respondentAgentId: bound.agentId, origin: "lead-ask", touchedAt: new Date(NOW - 7_000).toISOString() })];
    const rows = buildAgentRows(registryOf(bound, running, neverPrompted), threads, NOW);
    assert.equal(rows.find((r) => r.state === "awaiting-owner")!.elapsedMs, 7_000);
    assert.equal(rows.find((r) => r.name === "cccccccc")!.elapsedMs, 3_000);
    assert.equal(rows.find((r) => r.name === "dddddddd")!.elapsedMs, 0);
  });

  test("elapsed never goes negative even when the source clock is in the future", () => {
    const bound = record({ threadBound: true });
    const running = record({ agentId: "eeeeeeee-0000-0000-0000-000000000000", client: {} as never, runStartedAt: NOW + 10_000 });
    const threads: ThreadRecord[] = [thread({ respondentAgentId: bound.agentId, origin: "lead-ask", touchedAt: new Date(NOW + 10_000).toISOString() })];
    const rows = buildAgentRows(registryOf(bound, running), threads, NOW);
    for (const row of rows) assert.equal(row.elapsedMs, 0);
  });

  test("a malformed touchedAt (unparsable date) collapses elapsedMs to 0 rather than propagating NaN", () => {
    const t = thread({ threadId: "q14", touchedAt: "not-a-date", respondentAgentId: undefined });
    const rows = buildAgentRows(registryOf(), [t], NOW);
    assert.equal(rows[0].elapsedMs, 0);
  });

  test("sort: state rank first (awaiting-owner, awaiting-approval, running), then elapsed descending within a state", () => {
    const bound = record({ agentId: "10000000-0000-0000-0000-000000000000", threadBound: true });
    const approvalOld = record({ agentId: "20000000-0000-0000-0000-000000000000", pendingApproval: { cmdId: "c", command: "x" } });
    const runningNew = record({ agentId: "30000000-0000-0000-0000-000000000000", client: {} as never, runStartedAt: NOW - 1_000 });
    const runningOld = record({ agentId: "40000000-0000-0000-0000-000000000000", client: {} as never, runStartedAt: NOW - 9_000 });
    const threads: ThreadRecord[] = [thread({ respondentAgentId: bound.agentId, origin: "lead-ask", touchedAt: new Date(NOW - 2_000).toISOString() })];
    const rows = buildAgentRows(registryOf(runningNew, approvalOld, runningOld, bound), threads, NOW);
    assert.deepEqual(
      rows.map((r) => r.name),
      ["10000000", "20000000", "40000000", "30000000"],
    );
  });
});

describe("buildWidgetLines", () => {
  function runningRow(elapsedMs: number, name = "w") {
    return { name, role: "worker" as const, state: "running" as const, elapsedMs, answerHint: undefined };
  }
  function awaitingRow(name: string) {
    return { name, role: "thread" as const, state: "awaiting-owner" as const, elapsedMs: 1_000, answerHint: `/answer ${name}` };
  }

  test("hide-on-empty: undefined for zero rows regardless of width", () => {
    assert.equal(buildWidgetLines([], 80), undefined);
    assert.equal(buildWidgetLines([], 40), undefined);
  });

  test("under the cap: every row renders, no tail line", () => {
    const rows = [runningRow(5_000, "a"), runningRow(10_000, "b")];
    const lines = buildWidgetLines(rows, 80);
    assert.equal(lines?.length, 2);
    assert.ok(lines![0].includes("a "));
    assert.ok(lines![1].includes("b "));
  });

  test("exact cap boundary: 5 total rows (0 hidden) renders all 5 with no tail", () => {
    const rows = [awaitingRow("t1"), awaitingRow("t2"), runningRow(4, "r1"), runningRow(3, "r2"), runningRow(2, "r3")];
    const lines = buildWidgetLines(rows, 80)!;
    assert.equal(lines.length, AGENT_WIDGET_ROW_CAP, "exactly at the cap: nothing hidden, no +N more tail");
    assert.ok(!lines.some((l) => l.includes("more")));
  });

  test("over the cap: running rows are trimmed to fit AGENT_WIDGET_ROW_CAP total, with a +N more tail — the KEPT running rows are the front of the (already elapsed-descending, per buildAgentRows) input", () => {
    // Fixture order mirrors buildAgentRows's real contract: running rows arrive already
    // sorted elapsed-DESCENDING, so slicing the front keeps the longest-running ones.
    const rows = [awaitingRow("t1"), awaitingRow("t2"), awaitingRow("t3"), runningRow(4, "r4"), runningRow(3, "r3"), runningRow(2, "r2"), runningRow(1, "r1")];
    const lines = buildWidgetLines(rows, 80)!;
    assert.equal(lines.length, AGENT_WIDGET_ROW_CAP + 1, "5 shown rows plus one +N more tail line");
    assert.ok(lines.slice(0, 3).every((l, i) => l.includes(`t${i + 1}`)), "all 3 awaiting rows are kept verbatim");
    assert.ok(lines[3].includes("r4") && lines[4].includes("r3"), "the two KEPT running rows are the longest-elapsed (r4, r3), not merely the first two in array order");
    assert.equal(lines[lines.length - 1], "+2 more", "4 running rows minus the 2 slots left after 3 awaiting rows = 2 hidden (the shortest-elapsed r2/r1)");
  });

  test("awaiting rows alone exceeding the cap are NEVER folded — no tail line is added in that case", () => {
    const rows = [awaitingRow("t1"), awaitingRow("t2"), awaitingRow("t3"), awaitingRow("t4"), awaitingRow("t5"), awaitingRow("t6")];
    const lines = buildWidgetLines(rows, 80)!;
    assert.equal(lines.length, 6, "all 6 awaiting rows are shown even though this exceeds AGENT_WIDGET_ROW_CAP");
    assert.ok(!lines.some((l) => l.includes("more")), "no synthetic tail — only running rows are ever trimmed");
  });

  test("a thread row's rendered line carries the /answer hint after an em-dash separator", () => {
    const lines = buildWidgetLines([awaitingRow("q7")], 80)!;
    assert.match(lines[0], /— \/answer q7$/);
  });

  test("every line is bounded to the given width at 40, 80, and 120 columns", () => {
    const longName = "a-very-long-agent-name-that-should-get-truncated-eventually";
    const rows = [runningRow(500_000, longName)];
    for (const width of [40, 80, 120]) {
      const lines = buildWidgetLines(rows, width)!;
      assert.ok(lines[0].length <= width, `width=${width}: line must not exceed the bound (got ${lines[0].length})`);
    }
  });
});

describe("buildStatusSegment", () => {
  test("undefined when there are no rows and no pending questions — clears the segment", () => {
    assert.equal(buildStatusSegment([], 0), undefined);
  });

  test("N agents, no question part, when pendingCount is 0", () => {
    const rows = [
      { name: "a", role: "worker" as const, state: "running" as const, elapsedMs: 0, answerHint: undefined },
      { name: "b", role: "worker" as const, state: "running" as const, elapsedMs: 0, answerHint: undefined },
    ];
    assert.equal(buildStatusSegment(rows, 0), "ws: 2 agents");
  });

  test("singular \"1 question\" vs plural \"N questions\"", () => {
    const rows = [{ name: "a", role: "worker" as const, state: "running" as const, elapsedMs: 0, answerHint: undefined }];
    assert.equal(buildStatusSegment(rows, 1), "ws: 1 agents · 1 question");
    assert.equal(buildStatusSegment(rows, 2), "ws: 1 agents · 2 questions");
  });

  test("still renders when rows is empty but pendingCount is positive", () => {
    assert.equal(buildStatusSegment([], 1), "ws: 0 agents · 1 question");
  });
});

describe("shouldArmAgentWidget (review relay #1 Important #5: the wiring gate index.ts uses, now directly testable)", () => {
  test("the host lead (role undefined) in a TUI session arms the widget", () => {
    assert.equal(shouldArmAgentWidget(undefined, "tui"), true);
  });

  test("a fork (role \"fork\") in a TUI session also arms the widget", () => {
    assert.equal(shouldArmAgentWidget("fork", "tui"), true);
  });

  test("a worker or explore child NEVER arms the widget, even if somehow run in tui mode", () => {
    assert.equal(shouldArmAgentWidget("worker", "tui"), false);
    assert.equal(shouldArmAgentWidget("explore", "tui"), false);
  });

  test("a lead/fork session that is NOT tui mode (rpc/headless) does not arm the widget", () => {
    assert.equal(shouldArmAgentWidget(undefined, "rpc"), false);
    assert.equal(shouldArmAgentWidget(undefined, undefined), false);
    assert.equal(shouldArmAgentWidget("fork", "rpc"), false);
  });
});
