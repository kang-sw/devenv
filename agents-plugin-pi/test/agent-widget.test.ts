/**
 * Unit tests for agent-widget.ts's pure row/render seam — `buildAgentRows`,
 * `buildWidgetLines`, `buildHeadingLine` — driven with duck-typed fake
 * `RpcAgentRecord`/`ThreadRecord` values, no live `pi` session or RPC client.
 * Controller tests retain the IO seam: they inspect the real factory-rendered
 * output, retired-footer clearing, and timer lifecycle without a live session.
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { promises as fs } from "node:fs";
import { RpcClient } from "@earendil-works/pi-coding-agent";
import { buildAgentRows, buildWidgetLines, buildHeadingLine, createAgentWidgetController, shouldArmAgentWidget, AGENT_STATUS_KEY, AGENT_WIDGET_KEY, AGENT_WIDGET_ROW_CAP, AGENT_WIDGET_ATTENTION_TICK_MS, AGENT_WIDGET_TICK_MS } from "../src/agent-widget.ts";
import type { RpcAgentRecord, RpcAgentRegistry } from "../src/spawner.ts";
import type { ThreadRecord } from "../src/ask.ts";
import { visibleWidth } from "../src/text-width.ts";

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
    assert.deepEqual(rows, [{ name: "11111111", role: "worker", state: "running", elapsedMs: 5_000 }]);
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
    assert.deepEqual(rows, [{ name: "why is the build red", role: "thread", state: "awaiting-owner", elapsedMs: 5_000, answerHint: "/answer q9", answerDisplay: "why is the build red" }]);
  });

  test("review relay #1 Critical: the lead-restart scenario — a thread with a respondentAgentId but no matching registry record (reviveOrphans never re-sets threadBound) — still renders one row", () => {
    const t = thread({ threadId: "q10", title: "post-restart question", status: "open", origin: "fork-raised", respondentAgentId: "ffffffff-0000-0000-0000-000000000000", touchedAt: new Date(NOW - 9_000).toISOString() });
    // The orphan record IS in the registry (reviveOrphans always re-registers it), just not threadBound.
    const orphan = record({ agentId: "ffffffff-0000-0000-0000-000000000000" });
    const rows = buildAgentRows(registryOf(orphan), [t], NOW);
    assert.deepEqual(rows, [{ name: "post-restart question", role: "thread", state: "awaiting-owner", elapsedMs: 9_000, answerHint: "/answer q10", answerDisplay: "post-restart question" }]);
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
  test("owner-question display titles are sanitized and width-bound while qN remains the only command hint", () => {
    const title = "very long\u001b[31m owner\nquestion title that must truncate";
    const rows = buildAgentRows(registryOf(), [thread({ threadId: "q42", title })], NOW);
    for (const width of [40, 80, 120]) {
      const line = buildWidgetLines(rows, 1, width, true)![1];
      assert.ok(!line.includes("\u001b[31m"), `injected SGR is absent at ${width}`);
      assert.match(line, /\/answer q42$/, `qN survives at ${width}`);
      assert.ok(visibleWidth(line) <= width, `display fits at ${width}`);
    }
  });

  test("an unusable owner-question title falls back to its id, while approval has no fabricated answer target", () => {
    const question = buildAgentRows(registryOf(), [thread({ threadId: "q43", title: "\n\t" })], NOW);
    assert.match(buildWidgetLines(question, 1, 80, true)![1], /\/answer q43.*— \/answer q43$/);
    const approval = buildWidgetLines([{ name: "exec", role: "execute", state: "awaiting-approval", elapsedMs: 0 }], 0, 80, true)![1];
    assert.match(approval, /awaiting approval/);
    assert.ok(!approval.includes("/answer"));
  });

  test("attention styles only structured cue fields, even when titles and names contain separators or state words", () => {
    const question = {
      name: "worker",
      role: "fork" as const,
      state: "awaiting-owner" as const,
      elapsedMs: 3_000,
      answerHint: "/answer q7",
      answerDisplay: "Choose — database",
      model: "test-model",
      effort: "high",
      latestInput: 42,
      estimatedUsd: .1,
    };
    const approval = { name: "awaiting approval audit", role: "execute" as const, state: "awaiting-approval" as const, elapsedMs: 3_000, model: "test-model", effort: "high", latestInput: 42, estimatedUsd: .1 };
    const animated = buildWidgetLines([question, approval], 1, 180, true)!;
    assert.equal(animated[1], "\u001b[1m/answer Choose — database\u001b[22m · fork · awaiting owner · 3s · test-model (high) · in 0.0k · est $0.1 — /answer q7");
    assert.equal(animated[2], "awaiting approval audit · execute · \u001b[1mawaiting approval\u001b[22m · 3s · test-model (high) · in 0.0k · est $0.1");
    assert.ok(!animated[1].slice(animated[1].indexOf(" · fork")).includes("\u001b[1m"), "role, elapsed, telemetry, separators, and qN stay plain");
    assert.ok(!animated[2].startsWith("\u001b[1m"), "a state-like name cannot redirect approval styling");
    const staticDisabled = buildWidgetLines([question, approval], 1, 180, true)!;
    assert.deepEqual(staticDisabled, animated, "static disabled emphasis uses the same cue-only ANSI boundaries");
  });

  test("a supplied future idle-awaiting-owner row emphasizes its state and preserves only its supplied inspection hint", () => {
    const row = { name: "parked reviewer", role: "fork" as const, state: "idle-awaiting-owner" as const, elapsedMs: 3_000, inspectionHint: "/audit reviewer" };
    const line = buildWidgetLines([row], 0, 120, true)![1];
    assert.equal(line, "parked reviewer · fork · \u001b[1midle awaiting owner\u001b[22m · 3s · — (—) · in — · est $— — /audit reviewer");
    assert.ok(!line.includes("/answer"), "presentation never fabricates an answer target for an owner-held idle row");
  });

  test("terminal-width formatting never reconstructs absent protected hints during attention styling", () => {
    const question = { name: "worker", role: "thread" as const, state: "awaiting-owner" as const, elapsedMs: 0, answerHint: "/answer q8", answerDisplay: "a long owner question" };
    const ownerHeld = { name: "reviewer", role: "fork" as const, state: "idle-awaiting-owner" as const, elapsedMs: 0, inspectionHint: "/audit a-very-long-inspection-target" };
    for (const emphasize of [false, true]) {
      for (const row of [question, ownerHeld]) {
        for (const width of [0, 1, 8]) {
          const line = buildWidgetLines([row], 0, width, emphasize)![1];
          assert.ok(visibleWidth(line) <= width, `width=${width}, emphasize=${emphasize}: output stays bounded`);
          assert.ok(!line.includes("/answer q8") && !line.includes("/audit a-very-long-inspection-target"), `width=${width}, emphasize=${emphasize}: absent long hint is not reconstructed`);
        }
      }
    }
  });
  test("telemetry exposes compact token and cost fields at wide widths and never displaces a 40-column answer cue", () => {
    const telemetry = { name: "模型-worker", role: "worker" as const, state: "running" as const, elapsedMs: 0, model: "provider/模型", effort: "high", latestInput: 0, estimatedUsd: 0 };
    assert.match(buildWidgetLines([telemetry], 0, 120)![1], /provider\/模型 \(high\).*in 0.0k.*est \$0/);
    const missing = { ...telemetry, model: undefined, effort: undefined, latestInput: undefined, estimatedUsd: undefined, answerHint: "/answer q1" };
    assert.match(buildWidgetLines([missing], 0, 40)![1], /\/answer q1$/);
    assert.ok(visibleWidth(buildWidgetLines([missing], 0, 40)![1]) <= 40);
    const compact = { ...telemetry, name: "a", model: "p", effort: "l", latestInput: 132_400, estimatedUsd: .123456789 };
    assert.match(buildWidgetLines([compact], 0, 80)![1], /p \(l\).*in 132.4k.*est \$0.123/);
    const noMegabyteUnit = { ...compact, latestInput: 1_354_100, estimatedUsd: 12.34567 };
    assert.match(buildWidgetLines([noMegabyteUnit], 0, 120)![1], /in 1354.1k.*est \$12.346/);

    const themedSpans: Array<[string, string]> = [];
    const themed = buildWidgetLines([compact], 0, 80, false, {
      fg(color, text) {
        themedSpans.push([color, text]);
        return `\u001b[38;5;1m${text}\u001b[39m`;
      },
    })![1];
    assert.ok(themedSpans.some(([color, text]) => color === "accent" && text === "p"), "model uses the theme accent");
    assert.ok(themedSpans.some(([color, text]) => color === "syntaxNumber" && text === "in 132.4k"), "input telemetry uses the numeric theme color");
    assert.ok(themedSpans.some(([color, text]) => color === "warning" && text === "est $0.123"), "estimated cost uses the warning/gold theme color");
    assert.ok(visibleWidth(themed) <= 80, "ANSI theme styling does not change width accounting");
  });
  function runningRow(elapsedMs: number, name = "w") {
    return { name, role: "worker" as const, state: "running" as const, elapsedMs, answerHint: undefined };
  }
  function awaitingRow(name: string) {
    return { name, role: "thread" as const, state: "awaiting-owner" as const, elapsedMs: 1_000, answerHint: `/answer ${name}` };
  }

  test("hide-on-empty: undefined for zero rows regardless of width", () => {
    assert.equal(buildWidgetLines([], 0, 80), undefined);
    assert.equal(buildWidgetLines([], 0, 40), undefined);
  });

  test("under the cap: every row renders, no tail line", () => {
    const rows = [runningRow(5_000, "a"), runningRow(10_000, "b")];
    const lines = buildWidgetLines(rows, 0, 80);
    assert.equal(lines?.length, 3);
    assert.equal(lines![0], "ws: 2 agents");
    assert.ok(lines![1].includes("a "));
    assert.ok(lines![2].includes("b "));
  });

  test("exact cap boundary: 5 total rows (0 hidden) renders all 5 with no tail", () => {
    const rows = [awaitingRow("t1"), awaitingRow("t2"), runningRow(4, "r1"), runningRow(3, "r2"), runningRow(2, "r3")];
    const lines = buildWidgetLines(rows, 0, 80)!;
    assert.equal(lines.length, AGENT_WIDGET_ROW_CAP + 1, "heading does not consume a row-cap slot");
    assert.ok(!lines.some((l) => l.includes("more")));
  });

  test("over the cap: running rows are trimmed to fit AGENT_WIDGET_ROW_CAP total, with a +N more tail — the KEPT running rows are the front of the (already elapsed-descending, per buildAgentRows) input", () => {
    // Fixture order mirrors buildAgentRows's real contract: running rows arrive already
    // sorted elapsed-DESCENDING, so slicing the front keeps the longest-running ones.
    const rows = [awaitingRow("t1"), awaitingRow("t2"), awaitingRow("t3"), runningRow(4, "r4"), runningRow(3, "r3"), runningRow(2, "r2"), runningRow(1, "r1")];
    const lines = buildWidgetLines(rows, 0, 80)!;
    assert.equal(lines.length, AGENT_WIDGET_ROW_CAP + 2, "heading, 5 shown rows, plus one +N more tail line");
    assert.ok(lines.slice(1, 4).every((l, i) => l.includes(`t${i + 1}`)), "all 3 awaiting rows are kept verbatim");
    assert.ok(lines[4].includes("r4") && lines[5].includes("r3"), "the two KEPT running rows are the longest-elapsed (r4, r3), not merely the first two in array order");
    assert.equal(lines[lines.length - 1], "+2 more", "4 running rows minus the 2 slots left after 3 awaiting rows = 2 hidden (the shortest-elapsed r2/r1)");
  });

  test("awaiting rows alone exceeding the cap are NEVER folded — no tail line is added in that case", () => {
    const rows = [awaitingRow("t1"), awaitingRow("t2"), awaitingRow("t3"), awaitingRow("t4"), awaitingRow("t5"), awaitingRow("t6")];
    const lines = buildWidgetLines(rows, 0, 80)!;
    assert.equal(lines.length, 7, "heading plus all 6 awaiting rows are shown even though this exceeds AGENT_WIDGET_ROW_CAP");
    assert.ok(!lines.some((l) => l.includes("more")), "no synthetic tail — only running rows are ever trimmed");
  });

  test("a thread row's rendered line carries the /answer hint after an em-dash separator", () => {
    const lines = buildWidgetLines([awaitingRow("q7")], 0, 80)!;
    assert.match(lines[1], /— \/answer q7$/);
  });

  test("telemetry fields remain independent, Unicode-safe, and subordinate to the 40-column answer cue", () => {
    const complete = { name: "模型-worker", role: "thread" as const, state: "awaiting-owner" as const, elapsedMs: 0, answerHint: "/answer q1", model: "provider/模型", effort: "high", latestInput: 0, estimatedUsd: 0 };
    const completeWide = { ...complete, name: "模", role: "worker" as const, state: "running" as const, answerHint: undefined };
    const unknown = { ...completeWide, name: "missing", model: undefined, effort: undefined, latestInput: undefined, estimatedUsd: undefined };
    const narrow = buildWidgetLines([complete], 1, 40)![1];
    assert.match(narrow, /\/answer q1$/); assert.ok(visibleWidth(narrow) <= 40);
    for (const width of [80, 120]) {
      const lines = buildWidgetLines([completeWide, unknown], 0, width)!;
      assert.match(lines[1], /provider\/模型 \(high\).*in 0.0k.*est \$0/, `complete reported zero is not rendered as unknown at ${width}`);
      assert.match(lines[2], /— \(—\).*in —.*est \$—/, `unknown fields remain independently unknown at ${width}`);
      assert.ok(lines.every(line => visibleWidth(line) <= width), `Unicode display width is bounded at ${width}`);
    }
  });

  test("rendering telemetry performs neither disk reads nor RPC", (t) => {
    t.mock.method(fs, "readFile", async () => assert.fail("widget rendering must not read disk"));
    t.mock.method(RpcClient.prototype, "getState", async () => assert.fail("widget rendering must not query RPC"));
    const row = { name: "worker", role: "worker" as const, state: "running" as const, elapsedMs: 0, model: "p/m", effort: "low", latestInput: 1, estimatedUsd: .01 };
    for (const width of [40, 80, 120]) assert.doesNotThrow(() => buildWidgetLines([row], 0, width));
  });

  test("every line is bounded to the given width at 40, 80, and 120 columns", () => {
    const longName = "a-very-long-agent-name-that-should-get-truncated-eventually";
    const rows = [runningRow(500_000, longName)];
    for (const width of [40, 80, 120]) {
      const lines = buildWidgetLines(rows, 0, width)!;
      assert.ok(lines.every((line) => line.length <= width), `width=${width}: every line must not exceed the bound`);
    }
  });
});

describe("buildHeadingLine", () => {
  test("undefined when there are no rows and no pending questions — hides the panel", () => {
    assert.equal(buildHeadingLine([], 0), undefined);
  });

  test("N agents, no question part, when pendingCount is 0", () => {
    const rows = [
      { name: "a", role: "worker" as const, state: "running" as const, elapsedMs: 0, answerHint: undefined },
      { name: "b", role: "worker" as const, state: "running" as const, elapsedMs: 0, answerHint: undefined },
    ];
    assert.equal(buildHeadingLine(rows, 0), "ws: 2 agents");
  });

  test("singular \"1 question\" vs plural \"N questions\"", () => {
    const rows = [{ name: "a", role: "worker" as const, state: "running" as const, elapsedMs: 0, answerHint: undefined }];
    assert.equal(buildHeadingLine(rows, 1), "ws: 1 agents · 1 question");
    assert.equal(buildHeadingLine(rows, 2), "ws: 1 agents · 2 questions");
  });

  test("still renders when rows is empty but pendingCount is positive", () => {
    assert.equal(buildHeadingLine([], 1), "ws: 0 agents · 1 question");
  });
});

describe("createAgentWidgetController", () => {
  test("owner waits toggle bold/plain exactly every 330ms, share one timer, and disarm on resolution or config disable", (t) => {
    const waiting = record({ agentId: "aaaaaaaa-0000-0000-0000-000000000000", threadBound: true });
    const approval = record({ agentId: "bbbbbbbb-0000-0000-0000-000000000000", pendingApproval: { cmdId: "a", command: "x" } });
    const threads = new Map([["q1", thread({ respondentAgentId: waiting.agentId, title: "owner needs this" })]]);
    const callbacks = new Map<number, () => void>();
    const schedules: Array<{ id: number; ms: number }> = [];
    const cleared: number[] = [];
    let next = 1;
    let widget: ((tui: unknown, theme: unknown) => { render(width: number): string[] }) | undefined;
    let enabled = true;
    t.mock.method(global, "setInterval", ((callback: () => void, ms: number) => {
      const id = next++; callbacks.set(id, callback); schedules.push({ id, ms }); return { id, unref() {} } as never;
    }) as typeof setInterval);
    t.mock.method(global, "clearInterval", ((timer: { id: number }) => { cleared.push(timer.id); callbacks.delete(timer.id); }) as typeof clearInterval);
    const controller = createAgentWidgetController({ ui: { setWidget(_key, content) { widget = typeof content === "function" ? content : undefined; }, setStatus() {} } }, registryOf(waiting, approval), threads, {
      ownerLead: true,
      animationEnabled: () => enabled,
    });
    controller.refresh();
    assert.equal(AGENT_WIDGET_ATTENTION_TICK_MS, 330, "the ticket cadence is literal, not merely self-referential");
    assert.deepEqual(schedules.map(({ ms }) => ms).sort((a, b) => a - b), [330, AGENT_WIDGET_TICK_MS]);
    const initial = widget!({}, {}).render(120).join("\n");
    assert.match(initial, /\u001b\[1mws: 2 agents/);
    assert.match(initial, /\u001b\[1m\/answer owner needs this/);
    assert.match(initial, /\u001b\[1mawaiting approval/);
    const attentionId = schedules.find(({ ms }) => ms === 330)!.id;
    const attention = [attentionId, callbacks.get(attentionId)!] as const;
    attention[1]();
    const plain = widget!({}, {}).render(120).join("\n");
    assert.ok(!plain.includes("\u001b[1m"), "the plain phase has no emphasis");
    assert.match(plain, /\/answer owner needs this.*\/answer q1/, "the question cue remains visible in the plain phase");
    assert.match(plain, /awaiting approval/, "the approval state remains visible in the plain phase");
    threads.clear(); waiting.threadBound = false; approval.pendingApproval = undefined;
    controller.refresh();
    assert.ok(cleared.includes(attentionId), "final enabled resolution clears the actual attention timer");
    enabled = false;
    threads.set("q2", thread({ threadId: "q2", title: "static wait" }));
    controller.refresh();
    assert.equal(schedules.filter(({ ms }) => ms === 330).length, 1, "disabled config never re-arms animation");
    assert.match(widget!({}, {}).render(120).join("\n"), /\u001b\[1m\/answer static wait/, "disabled animation uses static bold emphasis");
    controller.stop();
  });

  test("fork and headless-equivalent controllers never own an attention timer, and replacement stop clears it once", (t) => {
    const threads = new Map([["q1", thread({ title: "wait" })]]);
    const intervals: number[] = [];
    const cleared: unknown[] = [];
    t.mock.method(global, "setInterval", ((callback: () => void, ms: number) => { intervals.push(ms); return { unref() {}, callback } as never; }) as typeof setInterval);
    t.mock.method(global, "clearInterval", ((timer: unknown) => { cleared.push(timer); }) as typeof clearInterval);
    const ui = { setWidget() {}, setStatus() {} };
    const fork = createAgentWidgetController({ ui }, registryOf(), threads, { ownerLead: false });
    fork.refresh();
    assert.ok(!intervals.includes(AGENT_WIDGET_ATTENTION_TICK_MS));
    const lead = createAgentWidgetController({ ui }, registryOf(), threads, { ownerLead: true });
    lead.refresh();
    assert.equal(intervals.filter((ms) => ms === AGENT_WIDGET_ATTENTION_TICK_MS).length, 1);
    lead.stop(); lead.stop();
    assert.equal(cleared.length, 2, "stop clears its elapsed and actual attention interval once each");
    fork.stop();
  });
  test("renders the uncapped heading at real widths, preserves the body cap, clears only its retired footer key, and disarms on empty", (t) => {
    const records = Array.from({ length: 7 }, (_, i) => record({
      agentId: `${String(i + 1).padStart(8, "0")}-0000-0000-0000-000000000000`,
      client: {} as never,
      runStartedAt: NOW - i,
    }));
    const registry = registryOf(...records);
    const threads = new Map<string, ThreadRecord>();
    const statuses = new Map<string, string | undefined>([["goal-loop", "Goal loop: settling"]]);
    let widget: ((tui: unknown, theme: unknown) => { render(width: number): string[] }) | undefined;
    const timerCallbacks: (() => void)[] = [];
    const cleared: unknown[] = [];
    t.mock.method(global, "setInterval", ((callback: () => void) => {
      timerCallbacks.push(callback);
      return { unref() {} } as never;
    }) as typeof setInterval);
    t.mock.method(global, "clearInterval", ((timer: unknown) => { cleared.push(timer); }) as typeof clearInterval);
    const controller = createAgentWidgetController({ ui: {
      setWidget(key, content) {
        assert.equal(key, AGENT_WIDGET_KEY);
        widget = typeof content === "function" ? content : undefined;
      },
      setStatus(key, text) { statuses.set(key, text); },
    } }, registry, threads);

    controller.refresh();
    assert.equal(statuses.get(AGENT_STATUS_KEY), undefined, "the obsolete agent footer is cleared on every refresh");
    assert.equal(statuses.get("goal-loop"), "Goal loop: settling", "unrelated footer ownership is untouched");
    assert.equal(timerCallbacks.length, 1, "visible panel arms one elapsed refresh timer");
    for (const width of [40, 80, 120]) {
      const lines = widget!({}, {}).render(width);
      assert.equal(lines[0], "ws: 7 agents", `width=${width}: the uncapped count stays in the first line`);
      assert.ok(lines.every((line) => line.length <= width), `width=${width}: every rendered line stays bounded`);
      assert.equal(lines.length, AGENT_WIDGET_ROW_CAP + 2, "the heading does not consume a body row-cap slot");
      assert.equal(lines.at(-1), "+2 more");
    }

    registry.clear();
    controller.refresh();
    assert.equal(widget, undefined, "empty rows and no pending questions hide the panel");
    assert.equal(cleared.length, 1, "becoming empty clears the timer");
    controller.stop();
    assert.equal(statuses.get(AGENT_STATUS_KEY), undefined, "shutdown keeps the retired agent footer cleared");
  });

  test("keeps a pending-question-only panel visible without an RPC agent", () => {
    const threads = new Map([["q1", thread({ status: "pending", title: "need owner" })]]);
    let widget: ((tui: unknown, theme: unknown) => { render(width: number): string[] }) | undefined;
    const controller = createAgentWidgetController({ ui: {
      setWidget(_key, content) { widget = typeof content === "function" ? content : undefined; },
      setStatus() {},
    } }, registryOf(), threads);
    controller.refresh();
    const lines = widget!({}, {}).render(80);
    assert.equal(lines[0], "ws: 1 agents · 1 question");
    assert.ok(lines[1].includes("need owner"));
    controller.stop();
  });

  test("renders a mixed matched pending thread with its deduplicated uncapped count before the capped body", () => {
    const respondent = record({
      agentId: "aaaaaaaa-0000-0000-0000-000000000000",
      alias: "waiting respondent",
      threadBound: true,
    });
    const running = Array.from({ length: 6 }, (_, i) => record({
      agentId: `${String(i + 1).padStart(8, "0")}-0000-0000-0000-000000000000`,
      client: {} as never,
      runStartedAt: NOW - i,
    }));
    const threads = new Map([["q1", thread({
      threadId: "q1",
      status: "pending",
      respondentAgentId: respondent.agentId,
      touchedAt: new Date(NOW - 1_000).toISOString(),
    })]]);
    let widget: ((tui: unknown, theme: unknown) => { render(width: number): string[] }) | undefined;
    const controller = createAgentWidgetController({ ui: {
      setWidget(_key, content) { widget = typeof content === "function" ? content : undefined; },
      setStatus() {},
    } }, registryOf(respondent, ...running), threads);

    controller.refresh();
    const lines = widget!({}, {}).render(80);
    assert.equal(lines[0], "ws: 7 agents · 1 question", "the matched thread is counted once, while its pending suffix remains visible");
    assert.ok(lines[1].includes("/answer a question") && lines[1].includes("/answer q1"), "the protected waiting row uses its display title and remains ahead of capped running rows");
    assert.equal(lines.length, AGENT_WIDGET_ROW_CAP + 2, "heading plus five body rows and the capped-running summary");
    assert.equal(lines.at(-1), "+2 more");
    controller.stop();
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
