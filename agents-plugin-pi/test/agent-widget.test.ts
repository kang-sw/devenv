/**
 * Unit tests for agent-widget.ts's pure row/render seam — `buildAgentRows`,
 * `buildWidgetLines` — driven with duck-typed fake `RpcAgentRecord`/
 * `ThreadRecord` values, no live `pi` session or RPC client. Controller tests
 * retain the IO seam: they inspect the real factory-rendered output,
 * retired-footer clearing, and timer lifecycle without a live session.
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { promises as fs } from "node:fs";
import { RpcClient } from "@earendil-works/pi-coding-agent";
import { buildAgentRows, buildAgentTree, buildWidgetLines, createAgentWidgetController, shouldArmAgentWidget, AGENT_STATE_BULLET, AGENT_STATUS_KEY, AGENT_WIDGET_KEY, AGENT_WIDGET_ROW_CAP, AGENT_WIDGET_ATTENTION_TICK_MS, AGENT_WIDGET_TICK_MS, type AgentRowState } from "../src/agent-widget.ts";
import type { RpcAgentRecord, RpcAgentRegistry } from "../src/spawner.ts";
import type { ThreadRecord } from "../src/ask.ts";
import { visibleWidth } from "../src/text-width.ts";
import { agentDisplayDetail, type AgentDisplayDetail } from "../src/agent-display.ts";
import {
  AGENT_WIDGET_COMMAND,
  AGENT_WIDGET_EXPANDED_TICK_MS,
  DEFAULT_AGENT_WIDGET_BATCH_ROWS,
  ESTIMATED_DOCK_ROWS,
  MAX_AGENT_WIDGET_BATCH_ROWS,
  buildAgentRowEntries,
  buildExpandedWidgetLines,
  chooseBatchAllotment,
  collectBatchViews,
  computeExpandedBudget,
  formatFinishedBatchFallback,
  formatRunningBatch,
  measureDockRows,
  planExpandedRows,
  registerAgentWidgetCommand,
  resolveAgentWidgetRows,
  type AgentRowEntry,
  type AgentWidgetController,
} from "../src/agent-widget.ts";
import { batchView, closeOpenBatch, createBatchBuffer, observeBatchEvent, type AgentBatchBuffer } from "../src/agent-batches.ts";

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
    // lastActivityAt (spawner.ts) reads only the agent-output high-water mark.
    // Default fixtures to output just observed so unrelated row tests retain a
    // zero activity duration; tests concerned with clock semantics override it.
    lastOutputAt: NOW,
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

describe("buildAgentTree", () => {
  test("unions local records with propagated descendants as a parent-grouped three-level tree", () => {
    const root = record({
      agentId: "root-root-0000-0000-000000000000",
      spawnRole: "execute-worker",
      client: {} as never,
      running: true,
      subtreeDescendants: [
        { id: "child-child-0000-000000000000", parentId: null, depth: 0, role: "explore", live: true },
        { id: "grand-grand-0000-000000000000", parentId: "child-child-0000-000000000000", depth: 1, role: "worker", live: false },
      ],
    });
    const sibling = record({ agentId: "sibling-0000-0000-000000000000", spawnRole: "fork" });

    assert.deepEqual(buildAgentTree(registryOf(root, sibling)), [
      { id: root.agentId, parentId: null, depth: 0, role: "execute", state: "running", live: true, openable: true },
      { id: "child-child-0000-000000000000", parentId: root.agentId, depth: 1, role: "explore", state: "running", live: true, openable: false },
      { id: "grand-grand-0000-000000000000", parentId: "child-child-0000-000000000000", depth: 2, role: "worker", state: undefined, live: false, openable: false },
      { id: sibling.agentId, parentId: null, depth: 0, role: "fork", state: undefined, live: false, openable: true },
    ]);
  });
});

describe("detailed propagated rows", () => {
  const detail = (overrides: Partial<AgentDisplayDetail> = {}): AgentDisplayDetail => ({
    name: "nested", state: "running", runStartedAt: NOW - 20_000,
    lastOutputAt: NOW - 3_000, model: "own-model", effort: "high",
    contextTokens: 12_000, estimatedUsd: 0.123, outputTps: 42, ...overrides,
  });
  const remote = (display: unknown, live = true): RpcAgentRecord => record({
    subtreeDescendants: [{ id: "remote00", parentId: null, depth: 0, role: "worker", live, display: display as AgentDisplayDetail }],
  });

  test("grandchild and deeper detail matches local own-record presentation, not ancestor totals", () => {
    const own = record({ alias: "nested", running: true, runStartedAt: NOW - 20_000, lastOutputAt: NOW - 3_000,
      telemetry: { model: "own-model", effort: "high", contextTokens: 12_000, estimatedUsd: 0.123 } as never,
      outputRate: { rate: () => 42 } as never });
    const display = agentDisplayDetail(own)!;
    const parent = remote(display);
    parent.running = true;
    parent.runStartedAt = NOW - 100_000;
    parent.telemetry = { estimatedUsd: 99 } as never;
    parent.subtreeDescendants!.push({ id: "deeper00", parentId: "remote00", depth: 1, role: "worker", live: true, display });
    const rows = buildAgentRows(registryOf(parent), [], NOW);
    const local = buildAgentRows(registryOf(own), [], NOW)[0];
    assert.deepEqual(rows.slice(1), [{ ...local, depth: 1 }, { ...local, depth: 2 }]);
    const localLine = buildWidgetLines([local], 0, 200)![0];
    assert.deepEqual(buildWidgetLines(rows, 0, 200)!.slice(1), [`│ ${localLine}`, `│ │ ${localLine}`]);
    assert.ok(buildAgentTree(registryOf(parent)).slice(1).every(node => !node.openable));
  });

  test("owner clocks advance locally and settlement freezes run duration only", () => {
    const parent = remote(detail());
    const first = buildAgentRows(registryOf(parent), [], NOW)[0];
    const later = buildAgentRows(registryOf(parent), [], NOW + 10_000)[0];
    assert.equal(later.elapsedMs, first.elapsedMs + 10_000);
    assert.equal(later.lastActivityMs, first.lastActivityMs + 10_000);
    parent.subtreeDescendants![0].display!.settledAt = NOW - 5_000;
    assert.equal(buildAgentRows(registryOf(parent), [], NOW + 10_000)[0].elapsedMs, 15_000);
    assert.equal(buildAgentRows(registryOf(parent), [], NOW + 10_000)[0].lastActivityMs, 13_000);
  });

  test("forwarded states keep ranks and never acquire local command affordances", () => {
    const states: AgentRowState[] = ["running", "pending-delivery", "waiting-on-children", "awaiting-approval", "idle-awaiting-owner", "awaiting-owner"];
    const parents = states.map((state, index) => ({ ...remote(detail({ state, name: state })), agentId: `parent-${index}` }));
    parents.forEach((parent, index) => { parent.subtreeDescendants![0].id = `remote-${index}`; });
    const ranked = buildAgentRows(registryOf(...parents), [], NOW);
    assert.deepEqual(ranked.map(row => row.state), ["idle-awaiting-owner", "awaiting-owner", "awaiting-approval", "waiting-on-children", "pending-delivery", "running"]);
    assert.ok(ranked.every(row => row.answerHint === undefined && row.inspectionHint === undefined));
    const lines = buildWidgetLines(ranked, 0, 200, true)!.join("\n");
    assert.match(lines, /awaiting approval/);
    assert.match(lines, /OWNER ACTION/);
    assert.doesNotMatch(lines, /\/answer|\/audit|ws-approve/);
  });

  test("settled idle detail and disconnected cached running detail are omitted", () => {
    assert.deepEqual(buildAgentRows(registryOf(remote(detail({ state: null }))), [], NOW), []);
    const parent = remote(detail(), false);
    assert.deepEqual(buildAgentRows(registryOf(parent), [], NOW), []);
    assert.equal(buildAgentTree(registryOf(parent))[1].state, undefined);
    assert.equal(buildAgentTree(registryOf(parent))[1].openable, false);
  });

  test("boundary sanitizes detail, omits unknown telemetry, and falls back for malformed base blocks", () => {
    for (const malformed of [undefined, {}, { ...detail(), state: "bogus" }, { ...detail(), runStartedAt: NaN }, { ...detail(), lastOutputAt: -1 }, { ...detail(), name: "\x1b\n" }]) {
      const parent = remote(malformed);
      assert.equal(buildAgentTree(registryOf(parent))[1].display, undefined);
      assert.deepEqual(buildAgentRows(registryOf(parent), [], NOW)[0], {
        name: "remote00", role: "worker", state: "running", elapsedMs: 0, lastActivityMs: 0, depth: 1, livenessOnly: true,
      });
    }
    const parent = remote({ ...detail(), name: "\x1bnested\n", model: "\x1bmodel", effort: undefined,
      contextTokens: NaN, estimatedUsd: -1, outputTps: Infinity });
    const row = buildAgentRows(registryOf(parent), [], NOW)[0];
    assert.equal(row.name, "nested");
    assert.equal(row.model, "model");
    for (const key of ["effort", "contextTokens", "estimatedUsd", "outputTps"] as const) assert.equal(row[key], undefined);
    assert.match(buildWidgetLines([row], 0, 200)![0], /model \(—\) · \? · \$—/);
    assert.equal(buildAgentTree(registryOf(remote(detail({ name: "x".repeat(400) }))))[1].display!.name.length, 256);
  });

  test("nested width degradation drops TPS first and retains bounded gutters at every width", () => {
    const row = buildAgentRows(registryOf(remote(detail())), [], NOW)[0];
    const full = buildWidgetLines([row], 0, 200)![0];
    assert.match(full, /42t\/s/);
    const withoutTps = full.replace(", 42t/s", "");
    assert.equal(buildWidgetLines([row], 0, visibleWidth(withoutTps))![0], withoutTps);
    for (let width = 0; width <= visibleWidth(full); width++) {
      const line = buildWidgetLines([row], 0, width)![0];
      assert.ok(visibleWidth(line) <= width);
      assert.doesNotMatch(line, /\/answer|\/audit/);
    }
  });
});

describe("buildAgentRows", () => {
  test("a plain live (client-holding) non-threadBound record is a running row with no answer hint", () => {
    const r = record({ client: {} as never, running: true, runStartedAt: NOW - 5_000 });
    const rows = buildAgentRows(registryOf(r), [], NOW);
    assert.deepEqual(rows, [{ name: "11111111", role: "worker", state: "running", elapsedMs: 5_000, lastActivityMs: 0 }]);
  });

  test("name precedence: alias > title > shortened uuid", () => {
    const byAlias = record({ client: {} as never, running: true, alias: "scout", title: "irrelevant title" });
    const byTitle = record({ client: {} as never, running: true, title: "the title" });
    const byUuid = record({ client: {} as never, running: true });
    assert.equal(buildAgentRows(registryOf(byAlias), [], NOW)[0].name, "scout");
    assert.equal(buildAgentRows(registryOf(byTitle), [], NOW)[0].name, "the title");
    assert.equal(buildAgentRows(registryOf(byUuid), [], NOW)[0].name, "11111111");
  });

  test("roleFromSpawnRole: worker -> worker, execute-worker -> execute, fork -> fork, explore -> explore, unset -> worker", () => {
    const worker = record({ client: {} as never, running: true, spawnRole: "worker" });
    const exec = record({ client: {} as never, running: true, spawnRole: "execute-worker" });
    const fork = record({ client: {} as never, running: true, spawnRole: "fork" });
    const explore = record({ client: {} as never, running: true, spawnRole: "explore" });
    const unset = record({ client: {} as never, running: true });
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

  test("last-writer owner records stay visible: running while active, idle-awaiting-owner after settle, both with /audit", () => {
    const idle = record({ agentId: "owner-idle", alias: "reviewer", lastWriter: "owner", running: false });
    const active = record({ agentId: "owner-running", title: "builder", lastWriter: "owner", running: true, client: {} as never });
    const rows = buildAgentRows(registryOf(idle, active), [], NOW);
    const idleRow = rows.find((row) => row.name === "reviewer")!;
    const activeRow = rows.find((row) => row.name === "builder")!;
    assert.equal(idleRow.state, "idle-awaiting-owner");
    assert.equal(idleRow.inspectionHint, "/audit reviewer");
    assert.equal(activeRow.state, "running");
    assert.equal(activeRow.inspectionHint, "/audit owner-running");
  });

  test("a pendingApproval record is included and ranked awaiting-approval even without a live client", () => {
    const r = record({ pendingApproval: { cmdId: "c1", command: "rm -rf /" } });
    const rows = buildAgentRows(registryOf(r), [], NOW);
    assert.equal(rows.length, 1);
    assert.equal(rows[0].state, "awaiting-approval");
  });

  test("review round 1 Important: waiting-on-children and pending-delivery are classified as their own distinct, otherwise-untested states", () => {
    const waiting = record({ agentId: "50000000-0000-0000-0000-000000000000", waitingOnChildren: true });
    const delivering = record({ agentId: "60000000-0000-0000-0000-000000000000", terminalDelivery: { state: "held" } });
    const rows = buildAgentRows(registryOf(waiting, delivering), [], NOW);
    assert.equal(rows.find((r) => r.name === "50000000")!.state, "waiting-on-children");
    assert.equal(rows.find((r) => r.name === "60000000")!.state, "pending-delivery");
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

  test("review relay #1 Critical: a pending ws-queue-question thread with NO respondent yet (empty registry) still renders one row, named by the thread's own title, carrying the /answer hint", () => {
    const t = thread({ threadId: "q9", title: "why is the build red", status: "pending", origin: "lead-ask", respondentAgentId: undefined, touchedAt: new Date(NOW - 5_000).toISOString() });
    const rows = buildAgentRows(registryOf(), [t], NOW);
    assert.deepEqual(rows, [{ name: "why is the build red", role: "thread", state: "awaiting-owner", elapsedMs: 5_000, lastActivityMs: 5_000, answerHint: "/answer q9" }]);
  });

  test("review relay #1 Critical: the lead-restart scenario — a thread with a respondentAgentId but no matching registry record (reviveOrphans never re-sets threadBound) — still renders one row", () => {
    const t = thread({ threadId: "q10", title: "post-restart question", status: "open", origin: "fork-raised", respondentAgentId: "ffffffff-0000-0000-0000-000000000000", touchedAt: new Date(NOW - 9_000).toISOString() });
    // The orphan record IS in the registry (reviveOrphans always re-registers it), just not threadBound.
    const orphan = record({ agentId: "ffffffff-0000-0000-0000-000000000000" });
    const rows = buildAgentRows(registryOf(orphan), [t], NOW);
    assert.deepEqual(rows, [{ name: "post-restart question", role: "thread", state: "awaiting-owner", elapsedMs: 9_000, lastActivityMs: 9_000, answerHint: "/answer q10" }]);
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
    const running = record({ agentId: "cccccccc-0000-0000-0000-000000000000", client: {} as never, running: true, runStartedAt: NOW - 3_000 });
    const neverPrompted = record({ agentId: "dddddddd-0000-0000-0000-000000000000", client: {} as never, running: true });
    const threads: ThreadRecord[] = [thread({ respondentAgentId: bound.agentId, origin: "lead-ask", touchedAt: new Date(NOW - 7_000).toISOString() })];
    const rows = buildAgentRows(registryOf(bound, running, neverPrompted), threads, NOW);
    assert.equal(rows.find((r) => r.state === "awaiting-owner")!.elapsedMs, 7_000);
    assert.equal(rows.find((r) => r.name === "cccccccc")!.elapsedMs, 3_000);
    assert.equal(rows.find((r) => r.name === "dddddddd")!.elapsedMs, 0);
  });

  test("elapsed freezes at settle for lingering non-thread rows while the bound-thread clock stays untouched", () => {
    const frozen = record({
      agentId: "eeeeeeee-0000-0000-0000-000000000000",
      client: {} as never,
      waitingOnChildren: true,
      runStartedAt: NOW - 20_000,
      settledAt: NOW - 5_000,
    });
    const bound = record({ threadBound: true, runStartedAt: NOW - 20_000, settledAt: NOW - 5_000 });
    const threads: ThreadRecord[] = [thread({ respondentAgentId: bound.agentId, origin: "lead-ask", touchedAt: new Date(NOW - 3_000).toISOString() })];
    const rows = buildAgentRows(registryOf(frozen, bound), threads, NOW);
    assert.equal(rows.find((row) => row.name === "eeeeeeee")!.elapsedMs, 15_000);
    assert.equal(rows.find((row) => row.state === "awaiting-owner")!.elapsedMs, 3_000);
  });

  test("elapsed never goes negative even when the source clock is in the future", () => {
    const bound = record({ threadBound: true });
    const running = record({ agentId: "eeeeeeee-0000-0000-0000-000000000000", client: {} as never, running: true, runStartedAt: NOW + 10_000 });
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
    const runningNew = record({ agentId: "30000000-0000-0000-0000-000000000000", client: {} as never, running: true, runStartedAt: NOW - 1_000 });
    const runningOld = record({ agentId: "40000000-0000-0000-0000-000000000000", client: {} as never, running: true, runStartedAt: NOW - 9_000 });
    const threads: ThreadRecord[] = [thread({ respondentAgentId: bound.agentId, origin: "lead-ask", touchedAt: new Date(NOW - 2_000).toISOString() })];
    const rows = buildAgentRows(registryOf(runningNew, approvalOld, runningOld, bound), threads, NOW);
    assert.deepEqual(
      rows.map((r) => r.name),
      ["10000000", "20000000", "40000000", "30000000"],
    );
  });

  test("keeps live propagated descendants grouped under their local parent and filters dormant propagated rows", () => {
    const parent = record({
      agentId: "parent00-0000-0000-0000-000000000000",
      client: {} as never,
      running: true,
      runStartedAt: NOW - 4_000,
      subtreeDescendants: [
        { id: "child000-0000-0000-0000-000000000000", parentId: null, depth: 0, role: "explore", live: true },
        { id: "grand000-0000-0000-0000-000000000000", parentId: "child000-0000-0000-0000-000000000000", depth: 1, role: "worker", live: true },
        { id: "dormant0-0000-0000-0000-000000000000", parentId: null, depth: 0, role: "fork", live: false },
      ],
    });
    const rows = buildAgentRows(registryOf(parent), [], NOW);
    assert.deepEqual(rows.map(({ name, role, state, depth, livenessOnly }) => ({ name, role, state, depth, livenessOnly })), [
      { name: "parent00", role: "worker", state: "running", depth: undefined, livenessOnly: undefined },
      { name: "child000", role: "explore", state: "running", depth: 1, livenessOnly: true },
      { name: "grand000", role: "worker", state: "running", depth: 2, livenessOnly: true },
    ]);
  });

  test("preserves synthetic thread union and settled-local filtering while building tree-backed rows", () => {
    const settled = record({
      agentId: "settled0-0000-0000-0000-000000000000",
      subtreeDescendants: [{ id: "dormant0-0000-0000-0000-000000000000", parentId: null, depth: 0, role: "worker", live: false }],
    });
    const pending = thread({ threadId: "q-tree", status: "pending", respondentAgentId: undefined, touchedAt: new Date(NOW - 2_000).toISOString() });
    assert.deepEqual(buildAgentRows(registryOf(settled), [pending], NOW), [
      { name: "a question", role: "thread", state: "awaiting-owner", elapsedMs: 2_000, lastActivityMs: 2_000, answerHint: "/answer q-tree" },
    ]);
  });
});

describe("buildWidgetLines", () => {
  test("renders one gutter segment per propagated depth and no invented deep-row telemetry", () => {
    const parent = record({
      agentId: "parent00-0000-0000-0000-000000000000",
      client: {} as never,
      running: true,
      subtreeDescendants: [
        { id: "child000-0000-0000-0000-000000000000", parentId: null, depth: 0, role: "explore", live: true },
        { id: "grand000-0000-0000-0000-000000000000", parentId: "child000-0000-0000-0000-000000000000", depth: 1, role: "worker", live: true },
      ],
    });
    const lines = buildWidgetLines(buildAgentRows(registryOf(parent), [], NOW), 0, 120)!;
    assert.match(lines[0], /^· parent00 · running/);
    assert.equal(lines[1], "│ · child000 · running");
    assert.equal(lines[2], "│ │ · grand000 · running");
    assert.ok(lines.every((line) => !line.includes("dormant")));
    assert.ok(lines.slice(1).every((line) => !line.includes("0s") && !line.includes("$—")), "propagated rows expose liveness only, not fabricated clocks or telemetry");
    for (const width of [0, 1, 2, 3, 4, 8, 20]) {
      const narrow = buildWidgetLines(buildAgentRows(registryOf(parent), [], NOW), 0, width)!;
      assert.ok(narrow.every((line) => visibleWidth(line) <= width), `nested gutter remains bounded at width=${width}`);
    }
  });

  test("owner-question titles never reach the gutter; one valid qN command leads the row", () => {
    const title = "very long\u001b[31m owner\nquestion\u0000 title that must never render";
    const rows = buildAgentRows(registryOf(), [thread({ threadId: "q42", title })], NOW);
    for (const width of [0, 1, 8, 10, 11, 20, 30, 40, 80, 120]) {
      const line = buildWidgetLines(rows, 1, width, true)![0];
      assert.ok(!line.includes("very long") && !line.includes("ownerquestion") && !line.includes("\u001b[31m") && !line.includes("\u0000") && !line.includes("\n"), `title and controls are absent at ${width}`);
      const plainLine = line.replace(/\u001b\[[0-9;]*m/g, "");
      const answerFragments = plainLine.match(/\/answer(?:\s+\S*)?/g) ?? [];
      assert.ok(answerFragments.every((fragment) => fragment === "/answer q42"), `no partial or invented answer command at ${width}`);
      if (width >= 40) {
        assert.match(line, /^● \u001b\[1m⚠ OWNER ACTION · \/answer q42/, `the state bullet leads, then the owner cue, at ${width}`);
        assert.equal(line.match(/\/answer q42/g)?.length, 1, `only one qN command appears at ${width}`);
      }
      assert.ok(visibleWidth(line) <= width, `display fits at ${width}`);
    }
  });

  test("an unusable owner-question title is irrelevant to the qN cue, while approval has no fabricated answer target", () => {
    const question = buildAgentRows(registryOf(), [thread({ threadId: "q43", title: "\n\t" })], NOW);
    const questionLine = buildWidgetLines(question, 1, 80, true)![0];
    assert.match(questionLine, /^● \u001b\[1m⚠ OWNER ACTION · \/answer q43/);
    assert.equal(questionLine.match(/\/answer q43/g)?.length, 1);
    const approval = buildWidgetLines([{ name: "exec", role: "execute", state: "awaiting-approval", elapsedMs: 0, lastActivityMs: 0 }], 0, 80, true)![0];
    assert.match(approval, /awaiting approval/);
    assert.ok(!approval.includes("/answer"));
  });

  test("owner actions have a labeled semantic cue while approval identity, telemetry, and qN remain ordinary", () => {
    const question = {
      name: "worker",
      role: "fork" as const,
      state: "awaiting-owner" as const,
      elapsedMs: 3_000,
      lastActivityMs: 3_000,
      answerHint: "/answer q7",
      model: "test-model",
      effort: "high",
      contextTokens: 42,
      estimatedUsd: .1,
    };
    const approval = { name: "awaiting approval audit", role: "execute" as const, state: "awaiting-approval" as const, elapsedMs: 3_000, lastActivityMs: 3_000, model: "test-model", effort: "high", contextTokens: 42, estimatedUsd: .1 };
    const semanticCalls: Array<[string, string]> = [];
    const theme = { fg(color: "error" | "warning" | "accent" | "dim" | "syntaxNumber", text: string) { semanticCalls.push([color, text]); return text; } };
    const animated = buildWidgetLines([question, approval], 1, 180, true, theme)!;
    assert.equal(animated[0], "● [1m⚠ OWNER ACTION · /answer q7[22m · awaiting owner · 3s (3s) · test-model (high) · 0.0k · $0.1");
    assert.equal(animated[1], "▲ awaiting approval audit · awaiting approval · 3s (3s) · test-model (high) · 0.0k · $0.1");
    assert.ok(semanticCalls.some(([color, text]) => color === "error" && text === "● "), "the owner-held bullet participates in the current attention-cycle color, same as the cue text");
    assert.ok(semanticCalls.some(([color, text]) => color === "error" && text === "⚠ OWNER ACTION · /answer q7"), "the owner cue text uses the identical color");
    assert.ok(semanticCalls.some(([color, text]) => color === "error" && text === "▲ "), "the approval bullet is colored too, but statically (STATE_BULLET_COLOR), never bold, never joining the attention cycle");
    assert.ok(!animated[0].slice(animated[0].indexOf(" · awaiting owner")).includes("[1m"), "state, elapsed, telemetry, and separators stay plain");
    assert.ok(!animated[1].includes("[1m"), "approval remains entirely ordinary even beside an animated question");
  });

  test("review round 1 Important: every AgentRowState's bullet glyph matches AGENT_STATE_BULLET on the rendered row, including waiting-on-children and pending-delivery which no other test exercises", () => {
    const rowFor = (state: AgentRowState) => ({ name: "r", role: "worker" as const, state, elapsedMs: 0, lastActivityMs: 0 });
    for (const state of Object.keys(AGENT_STATE_BULLET) as AgentRowState[]) {
      const line = buildWidgetLines([rowFor(state)], 0, 80)![0];
      assert.ok(line.startsWith(`${AGENT_STATE_BULLET[state]} `), `state=${state}: the row leads with its own state bullet ("${AGENT_STATE_BULLET[state]}"), not another state's glyph`);
    }
  });

  test("an owner-held row without qN receives an honest labeled inspection presentation", () => {
    const row = { name: "parked reviewer", role: "fork" as const, state: "idle-awaiting-owner" as const, elapsedMs: 3_000, lastActivityMs: 3_000, inspectionHint: "/audit reviewer" };
    const line = buildWidgetLines([row], 0, 120, true)![0];
    assert.equal(line, "● [1m⚠ OWNER ACTION · parked reviewer[22m · idle awaiting owner · 3s (3s) · — (—) · ? · $— — /audit reviewer");
    assert.ok(!line.includes("/answer"), "presentation never fabricates an answer target for an owner-held idle row");
  });

  test("narrow-width formatting preserves owner cues without reconstructing hints, while approval-only rows stay plain", () => {
    const question = { name: "worker", role: "thread" as const, state: "awaiting-owner" as const, elapsedMs: 0, lastActivityMs: 0, answerHint: "/answer q8" };
    const ownerHeld = { name: "reviewer", role: "fork" as const, state: "idle-awaiting-owner" as const, elapsedMs: 0, lastActivityMs: 0, inspectionHint: "/audit a-very-long-inspection-target" };
    const approval = { name: "exec", role: "execute" as const, state: "awaiting-approval" as const, elapsedMs: 0, lastActivityMs: 0 };
    for (const emphasize of [false, true]) {
      for (const row of [question, ownerHeld]) {
        for (const width of [0, 1, 8]) {
          const line = buildWidgetLines([row], 0, width, emphasize)![0];
          assert.ok(visibleWidth(line) <= width, `width=${width}, emphasize=${emphasize}: output stays bounded`);
          assert.ok(!line.includes("/answer q8") && !line.includes("/audit a-very-long-inspection-target"), `width=${width}, emphasize=${emphasize}: absent long hint is not reconstructed`);
        }
      }
      for (const width of [0, 1, 8, 40]) {
        const lines = buildWidgetLines([approval], 0, width, emphasize)!;
        assert.ok(lines.every((line) => visibleWidth(line) <= width), `approval width=${width}: output stays bounded`);
        assert.ok(lines.every((line) => !line.includes("[1m")), `approval width=${width}: no owner-attention styling`);
      }
    }
  });

  test("telemetry retains prefixless context values between model/effort and cost at wide widths without displacing a 40-column answer cue", () => {
    const telemetry = { name: "模型-worker", role: "worker" as const, state: "running" as const, elapsedMs: 0, lastActivityMs: 0, model: "provider/模型", effort: "high", contextTokens: 0, estimatedUsd: 0 };
    const wideLine = buildWidgetLines([telemetry], 0, 120)![0];
    assert.match(wideLine, /0s \(0s\).*provider\/模型 \(high\).*0\.0k.*\$0/);
    assert.ok(!wideLine.includes("ctx "), "the live row removes only the ctx prefix");
    const missing = { ...telemetry, state: "awaiting-owner" as const, model: undefined, effort: undefined, contextTokens: undefined, estimatedUsd: undefined, answerHint: "/answer q1" };
    const narrowLine = buildWidgetLines([missing], 0, 40)![0];
    assert.match(narrowLine, /^● ⚠ OWNER ACTION · \/answer q1/);
    assert.ok(visibleWidth(narrowLine) <= 40);
    const compact = { ...telemetry, name: "a", model: "p", effort: "l", contextTokens: 132_400, estimatedUsd: .123456789 };
    const compactLine = buildWidgetLines([compact], 0, 80)![0];
    assert.match(compactLine, /0s \(0s\).*p \(l\).*132\.4k.*\$0.123/);
    assert.ok(!compactLine.includes("ctx "), "a populated contextTokens keeps its value but not its prefix on the row");

    for (const [width, includesTelemetry] of [[49, true], [48, false]] as const) {
      const boundaryPlain = buildWidgetLines([compact], 0, width)![0];
      assert.equal(boundaryPlain.includes("132.4k"), includesTelemetry, `plain telemetry is all-or-nothing at width=${width}`);
      assert.ok(!boundaryPlain.includes("ctx "), `plain boundary output has no ctx prefix at width=${width}`);
      assert.ok(visibleWidth(boundaryPlain) <= width, `plain boundary output stays bounded at width=${width}`);

      const boundarySpans: Array<[string, string]> = [];
      const boundaryThemed = buildWidgetLines([compact], 0, width, false, {
        fg(color, text) {
          boundarySpans.push([color, text]);
          return `\u001b[38;5;1m${text}\u001b[39m`;
        },
      })![0];
      assert.equal(boundarySpans.some(([color, text]) => color === "syntaxNumber" && text === "132.4k"), includesTelemetry, `themed telemetry is all-or-nothing at width=${width}`);
      assert.ok(!boundaryThemed.includes("ctx "), `themed boundary output has no ctx prefix at width=${width}`);
      assert.ok(visibleWidth(boundaryThemed) <= width, `themed boundary output stays bounded at width=${width}`);
    }

    const themedSpans: Array<[string, string]> = [];
    const themed = buildWidgetLines([compact], 0, 80, false, {
      fg(color, text) {
        themedSpans.push([color, text]);
        return `[38;5;1m${text}[39m`;
      },
    })![0];
    assert.ok(themedSpans.some(([color, text]) => color === "accent" && text === "p"), "model uses the theme accent");
    assert.ok(themedSpans.some(([color, text]) => color === "syntaxNumber" && text === "132.4k"), "the retained token value uses the numeric theme span");
    assert.ok(themedSpans.some(([color, text]) => color === "syntaxNumber" && text === "0s"), "activity remains in its numeric theme span");
    assert.ok(themedSpans.some(([color, text]) => color === "warning" && text === "$0.123"), "estimated cost uses the warning/gold theme color without a label");
    assert.ok(!themedSpans.some(([, text]) => text.includes("ctx ")), "no theme span carries the ctx prefix on a row");
    assert.ok(visibleWidth(themed) <= 80, "ANSI theme styling does not change width accounting");
  });
  test("active duration follows total duration with unchanged numeric styling", () => {
    const row = { name: "gutter-probe", role: "worker" as const, state: "running" as const,
      elapsedMs: 60_000, lastActivityMs: 25_000, model: "gpt-5.6-luna", effort: "high",
      contextTokens: 17_000, estimatedUsd: .33 };
    const expected = "· gutter-probe · running · 1m (25s) · gpt-5.6-luna (high) · 17.0k · $0.33";
    assert.equal(buildWidgetLines([row], 0, 180)![0], expected);
    const spans: Array<[string, string]> = [];
    const theme = { fg(color: string, text: string) {
      spans.push([color, text]);
      return `\u001b[36m${text}\u001b[39m`;
    } };
    const themed = buildWidgetLines([row], 0, 180, false, theme)![0];
    assert.equal(themed.replace(/\u001b\[[0-9;]*m/g, ""), expected);
    assert.ok(spans.some(([color, text]) => color === "syntaxNumber" && text === "25s"));
    assert.ok(!themed.includes("active "));
    for (let width = 0; width <= expected.length; width++) {
      const plain = buildWidgetLines([row], 0, width)![0];
      const styled = buildWidgetLines([row], 0, width, false, theme)![0];
      assert.equal(styled.replace(/\u001b\[[0-9;]*m/g, ""), plain);
      assert.ok(visibleWidth(styled) <= width, `width=${width}`);
    }
  });

  function runningRow(elapsedMs: number, name = "w") {
    return { name, role: "worker" as const, state: "running" as const, elapsedMs, lastActivityMs: elapsedMs, answerHint: undefined };
  }
  function awaitingRow(name: string) {
    return { name, role: "thread" as const, state: "awaiting-owner" as const, elapsedMs: 1_000, lastActivityMs: 1_000, answerHint: `/answer ${name}` };
  }

  test("hide-on-empty: undefined for zero rows regardless of width", () => {
    assert.equal(buildWidgetLines([], 0, 80), undefined);
    assert.equal(buildWidgetLines([], 0, 40), undefined);
  });

  test("under the cap: every row renders, no heading line, no tail line", () => {
    const rows = [runningRow(5_000, "a"), runningRow(10_000, "b")];
    const lines = buildWidgetLines(rows, 0, 80);
    assert.equal(lines?.length, 2);
    assert.ok(lines![0].includes("a "));
    assert.ok(lines![1].includes("b "));
  });

  test("exact cap boundary: 5 total rows (0 hidden) renders all 5 with no tail", () => {
    const rows = [awaitingRow("t1"), awaitingRow("t2"), runningRow(4, "r1"), runningRow(3, "r2"), runningRow(2, "r3")];
    const lines = buildWidgetLines(rows, 0, 80)!;
    assert.equal(lines.length, AGENT_WIDGET_ROW_CAP, "all rows render with no heading line and no tail");
    assert.ok(!lines.some((l) => l.includes("more")));
  });

  test("over the cap: running rows are trimmed to fit AGENT_WIDGET_ROW_CAP total, with a +N more tail — the KEPT running rows are the front of the (already elapsed-descending, per buildAgentRows) input", () => {
    // Fixture order mirrors buildAgentRows's real contract: running rows arrive already
    // sorted elapsed-DESCENDING, so slicing the front keeps the longest-running ones.
    const rows = [awaitingRow("t1"), awaitingRow("t2"), awaitingRow("t3"), runningRow(4, "r4"), runningRow(3, "r3"), runningRow(2, "r2"), runningRow(1, "r1")];
    const lines = buildWidgetLines(rows, 0, 80)!;
    assert.equal(lines.length, AGENT_WIDGET_ROW_CAP + 1, "no heading line; 5 shown rows plus one +N more tail line");
    assert.ok(lines.slice(0, 3).every((l, i) => l.includes(`t${i + 1}`)), "all 3 awaiting rows are kept verbatim");
    assert.ok(lines[3].includes("r4") && lines[4].includes("r3"), "the two KEPT running rows are the longest-elapsed (r4, r3), not merely the first two in array order");
    assert.equal(lines[lines.length - 1], "+2 more", "4 running rows minus the 2 slots left after 3 awaiting rows = 2 hidden (the shortest-elapsed r2/r1)");
  });

  test("over-cap trimming retains parent-child adjacency instead of regrouping all awaiting rows", () => {
    const parent = { name: "parent", role: "worker" as const, state: "waiting-on-children" as const, elapsedMs: 5_000, lastActivityMs: 5_000 };
    const child = { ...runningRow(0, "child"), depth: 1, livenessOnly: true as const };
    const delivery = { name: "delivery", role: "worker" as const, state: "pending-delivery" as const, elapsedMs: 4_000, lastActivityMs: 4_000 };
    const lines = buildWidgetLines([parent, child, delivery, runningRow(3, "r3"), runningRow(2, "r2"), runningRow(1, "r1")], 0, 80)!;
    assert.match(lines[0], /parent/);
    assert.match(lines[1], /^│ · child · running$/, "the visible child stays immediately below its parent");
    assert.match(lines[2], /delivery/);
    assert.equal(lines.at(-1), "+1 more");
  });

  test("awaiting rows alone exceeding the cap are NEVER folded — no tail line is added in that case", () => {
    const rows = [awaitingRow("t1"), awaitingRow("t2"), awaitingRow("t3"), awaitingRow("t4"), awaitingRow("t5"), awaitingRow("t6")];
    const lines = buildWidgetLines(rows, 0, 80)!;
    assert.equal(lines.length, 6, "all 6 awaiting rows are shown even though this exceeds AGENT_WIDGET_ROW_CAP, with no heading line");
    assert.ok(!lines.some((l) => l.includes("more")), "no synthetic tail — only running rows are ever trimmed");
  });

  test("a thread row's rendered line carries its sole /answer hint in the leading owner-action cue", () => {
    const lines = buildWidgetLines([awaitingRow("q7")], 0, 80)!;
    assert.match(lines[0], /^● ⚠ OWNER ACTION · \/answer q7/);
    assert.equal(lines[0].match(/\/answer q7/g)?.length, 1);
  });

  test("telemetry fields remain independent, Unicode-safe, and subordinate to the 40-column answer cue", () => {
    const complete = { name: "模型-worker", role: "thread" as const, state: "awaiting-owner" as const, elapsedMs: 0, lastActivityMs: 0, answerHint: "/answer q1", model: "provider/模型", effort: "high", contextTokens: 0, estimatedUsd: 0 };
    const completeWide = { ...complete, name: "模", role: "worker" as const, state: "running" as const, answerHint: undefined };
    const unknown = { ...completeWide, name: "missing", model: undefined, effort: undefined, contextTokens: undefined, estimatedUsd: undefined };
    const narrow = buildWidgetLines([complete], 1, 40)![0];
    assert.match(narrow, /^● ⚠ OWNER ACTION · \/answer q1/); assert.ok(visibleWidth(narrow) <= 40);
    for (const width of [80, 120]) {
      const lines = buildWidgetLines([completeWide, unknown], 0, width)!;
      assert.match(lines[0], /0s \(0s\).*provider\/模型 \(high\).*0\.0k.*\$0/, `complete reported zero is not rendered as unknown at ${width}`);
      assert.ok(!lines[0].includes("ctx "), `the live-row ctx prefix never reappears at ${width}`);
      assert.match(lines[1], /0s \(0s\).*— \(—\).*\?.*\$—/, `unknown fields remain independently unknown at ${width}`);
      assert.ok(lines.every(line => visibleWidth(line) <= width), `Unicode display width is bounded at ${width}`);
    }
  });

  test("rendering telemetry performs neither disk reads nor RPC", (t) => {
    t.mock.method(fs, "readFile", async () => assert.fail("widget rendering must not read disk"));
    t.mock.method(RpcClient.prototype, "getState", async () => assert.fail("widget rendering must not query RPC"));
    const row = { name: "worker", role: "worker" as const, state: "running" as const, elapsedMs: 0, lastActivityMs: 0, model: "p/m", effort: "low", contextTokens: 1, estimatedUsd: .01 };
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

describe("createAgentWidgetController", () => {
  test("owner-answer waits cycle error, warning, accent at 330ms; disabled state is static error and approval remains ordinary", (t) => {
    const waiting = record({ agentId: "aaaaaaaa-0000-0000-0000-000000000000", threadBound: true });
    const approval = record({ agentId: "bbbbbbbb-0000-0000-0000-000000000000", pendingApproval: { cmdId: "a", command: "x" } });
    const threads = new Map([["q1", thread({ respondentAgentId: waiting.agentId, title: "owner needs this" })]]);
    const callbacks = new Map<number, () => void>();
    const schedules: Array<{ id: number; ms: number }> = [];
    const cleared: number[] = [];
    let next = 1;
    let widget: ((tui: unknown, theme: unknown) => { render(width: number): string[] }) | undefined;
    let enabled = true;
    const semanticCalls: Array<[string, string, string]> = [];
    const themed = (name: string) => ({ fg(color: string, text: string) { semanticCalls.push([name, color, text]); return text; } });
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
    const renderPhase = (themeName: string, color: string, answerId = "q1") => {
      semanticCalls.length = 0;
      const output = widget!({}, themed(themeName)).render(120).join("\n");
      assert.match(output, /^● /, "260914: the owner-held row leads with its static filled-dot bullet, not a count heading");
      assert.match(output, new RegExp(`\\u001b\\[1m⚠ OWNER ACTION · /answer ${answerId}`), "cue is bold and explicitly labeled in every phase");
      assert.doesNotMatch(output, /\[1mawaiting approval/, "approval never joins the owner-action cycle");
      assert.ok(semanticCalls.some(([theme, actual, text]) => theme === themeName && actual === color && text === "● "), `${color} styles the owner-held bullet through the current theme, so the bullet column participates in the flash`);
      assert.ok(semanticCalls.some(([theme, actual, text]) => theme === themeName && actual === color && text.startsWith("⚠ OWNER ACTION")), `${color} styles only the owner cue through the current theme`);
    };
    renderPhase("first-theme", "error");
    const attentionId = schedules.find(({ ms }) => ms === 330)!.id;
    callbacks.get(attentionId)!();
    renderPhase("mutated-theme", "warning");
    callbacks.get(attentionId)!();
    renderPhase("mutated-theme", "accent");
    threads.clear(); waiting.threadBound = false;
    controller.refresh();
    assert.ok(cleared.includes(attentionId), "resolving the last question clears the timer even while approval remains");
    const approvalOnly = widget!({}, {}).render(120).join("\n");
    assert.match(approvalOnly, /awaiting approval/, "approval-only state remains visible");
    assert.ok(!approvalOnly.includes("[1m"), "approval-only state is not attention-styled");
    enabled = false;
    threads.set("q2", thread({ threadId: "q2", title: "static wait" }));
    controller.refresh();
    assert.equal(schedules.filter(({ ms }) => ms === 330).length, 1, "disabled config never re-arms animation");
    renderPhase("disabled-theme", "error", "q2");
    controller.stop();
  });

  test("an asynchronously read animation setting applies when it lands", async (t) => {
    const waiting = record({ agentId: "aaaaaaaa-0000-0000-0000-000000000000", threadBound: true });
    const threads = new Map([["q1", thread({ respondentAgentId: waiting.agentId, title: "owner needs this" })]]);
    const live = new Map<number, number>();
    let next = 1;
    t.mock.method(global, "setInterval", ((_callback: () => void, ms: number) => { const id = next++; live.set(id, ms); return { id, unref() {} } as never; }) as typeof setInterval);
    t.mock.method(global, "clearInterval", ((timer: { id: number }) => { live.delete(timer.id); }) as typeof clearInterval);
    const controller = createAgentWidgetController({ ui: { setWidget() {}, setStatus() {} } }, registryOf(waiting), threads, {
      ownerLead: true,
      animationEnabled: async () => false,
    });
    controller.refresh();
    assert.ok([...live.values()].includes(AGENT_WIDGET_ATTENTION_TICK_MS), "paints at once with the enabled default while the read is pending");
    await new Promise((resolve) => setImmediate(resolve));
    assert.ok(![...live.values()].includes(AGENT_WIDGET_ATTENTION_TICK_MS), "the landed false stops the animation");
    controller.stop();
  });

  test("an approval-only lead widget remains visible without creating an attention timer", (t) => {
    const approval = record({ pendingApproval: { cmdId: "a", command: "x" } });
    const intervals: number[] = [];
    let widget: ((tui: unknown, theme: unknown) => { render(width: number): string[] }) | undefined;
    t.mock.method(global, "setInterval", ((callback: () => void, ms: number) => {
      intervals.push(ms); return { unref() {}, callback } as never;
    }) as typeof setInterval);
    const controller = createAgentWidgetController({ ui: {
      setWidget(_key, content) { widget = typeof content === "function" ? content : undefined; },
      setStatus() {},
    } }, registryOf(approval), new Map(), { ownerLead: true });
    controller.refresh();
    assert.deepEqual(intervals, [AGENT_WIDGET_TICK_MS], "approval keeps elapsed repainting only");
    const lines = widget!({}, {}).render(120);
    assert.match(lines[0]!, /awaiting approval/);
    assert.ok(lines.every((line) => !line.includes("[1m")), "approval-only rows remain ordinary");
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
  test("renders every row without a heading line, preserves the body cap, clears only its retired footer key, and disarms on empty", (t) => {
    const records = Array.from({ length: 7 }, (_, i) => record({
      agentId: `${String(i + 1).padStart(8, "0")}-0000-0000-0000-000000000000`,
      client: {} as never, running: true,
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
      assert.ok(lines[0].startsWith("· "), `width=${width}: 260914 dropped the count heading — the first line is the first running row, led by its bullet`);
      assert.ok(lines.every((line) => line.length <= width), `width=${width}: every rendered line stays bounded`);
      assert.equal(lines.length, AGENT_WIDGET_ROW_CAP + 1, "no heading line; the body cap plus one +N more tail");
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
    assert.match(lines[0], /^● ⚠ OWNER ACTION · \/answer q1/);
    assert.equal(lines[0].match(/\/answer q1/g)?.length, 1);
    controller.stop();
  });

  test("renders a mixed matched pending thread as one deduplicated row ahead of the capped running body", () => {
    const respondent = record({
      agentId: "aaaaaaaa-0000-0000-0000-000000000000",
      alias: "waiting respondent",
      threadBound: true,
    });
    const running = Array.from({ length: 6 }, (_, i) => record({
      agentId: `${String(i + 1).padStart(8, "0")}-0000-0000-0000-000000000000`,
      client: {} as never, running: true,
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
    assert.match(lines[0], /^● ⚠ OWNER ACTION · \/answer q1/, "the protected waiting row leads with its sole valid command ahead of capped running rows, with no heading line and no double-counting of the matched thread");
    assert.equal(lines[0].match(/\/answer q1/g)?.length, 1);
    assert.equal(lines.length, AGENT_WIDGET_ROW_CAP + 1, "the matched thread row plus four running rows, and the capped-running summary tail");
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

describe("output TPS duration field", () => {
  const compact = { name: "a", role: "worker" as const, state: "running" as const, elapsedMs: 0, lastActivityMs: 0, model: "p", effort: "l", contextTokens: 132_400, estimatedUsd: .123456789 };

  test("sits beside activity inside duration parentheses as an integer with no tilde", () => {
    const line = buildWidgetLines([{ ...compact, elapsedMs: 20_000, outputTps: 60.6 }], 0, 120)![0];
    assert.equal(line, "· a · running · 20s (0s, 61t/s) · p (l) · 132.4k · $0.123");
    assert.equal(line.match(/61t\/s/g)?.length, 1);
    assert.ok(!line.includes("~"));
  });

  test("all internal roles render identically across states and local, detailed, and liveness-only rows", () => {
    for (const state of Object.keys(AGENT_STATE_BULLET) as AgentRowState[]) {
      for (const depth of [undefined, 1, 2]) {
        for (const livenessOnly of [undefined, true] as const) {
          const row = { ...compact, state, depth, livenessOnly, outputTps: 42 };
          const expected = buildWidgetLines([row], 0, 200)![0];
          for (const role of ["worker", "execute", "fork", "explore", "thread"] as const) {
            assert.equal(buildWidgetLines([{ ...row, role }], 0, 200)![0], expected);
          }
          assert.doesNotMatch(expected, / · (worker|execute|fork|explore|thread) · /);
        }
      }
    }
  });

  test("TPS styling and narrow degradation preserve the plain row and protected cues", () => {
    const spans: Array<[string, string]> = [];
    const theme = { fg(color: string, text: string) {
      spans.push([color, text]);
      return `\u001b[36m${text}\u001b[39m`;
    } };
    for (const row of [
      { ...compact, outputTps: 42, depth: 2 },
      { ...compact, outputTps: 42, inspectionHint: "/audit x" },
      { ...compact, outputTps: 42, state: "awaiting-owner" as const, answerHint: "/answer q7" },
    ]) {
      const full = buildWidgetLines([row], 0, 200)![0];
      for (let width = 0; width <= visibleWidth(full); width++) {
        const plain = buildWidgetLines([row], 0, width)![0];
        const styled = buildWidgetLines([row], 0, width, false, theme)![0];
        assert.equal(styled.replace(/\u001b\[[0-9;]*m/g, ""), plain);
        assert.ok(visibleWidth(styled) <= width);
        assert.doesNotMatch(plain, /\(\d+s, \)/);
      }
    }
    assert.ok(spans.some(([color, text]) => color === "syntaxNumber" && text === "42t/s"));
  });

  test("is absent before the first eligible message", () => {
    const line = buildWidgetLines([compact], 0, 120)![0];
    assert.ok(!line.includes("t/s"));
    assert.match(line, /0s \(0s\)/);
    assert.match(line, / · p \(l\) · 132\.4k · \$0\.123$/);
  });

  test("is dropped first when narrow; the rest of the group then follows the all-or-nothing rule", () => {
    const row = { ...compact, outputTps: 42 };
    // 49 columns fit the group without TPS; ", 42t/s" adds 7.
    for (const [width, tps, group] of [[56, true, true], [55, false, true], [49, false, true], [48, false, false]] as const) {
      for (const theme of [undefined, { fg: (_color: string, text: string) => `\u001b[38;5;1m${text}\u001b[39m` }]) {
        const line = buildWidgetLines([row], 0, width, false, theme)![0];
        const plainLine = line.replace(/\u001b\[[0-9;]*m/g, "");
        assert.equal(plainLine.includes("42t/s"), tps, `TPS at width=${width}`);
        assert.equal(plainLine.includes("p (l)") && plainLine.includes("132.4k") && plainLine.includes("$0.123"), group, `group at width=${width}`);
        assert.ok(visibleWidth(line) <= width, `bounded at width=${width}`);
      }
    }
  });

  test("with a protected inspection hint, TPS is still dropped first within the width left after the hint", () => {
    const row = { ...compact, outputTps: 42, inspectionHint: "/audit x" };
    // The hint " — /audit x" reserves 11 columns, shifting the unhinted boundaries.
    for (const [width, tps, group] of [[67, true, true], [66, false, true], [60, false, true], [59, false, false]] as const) {
      for (const theme of [undefined, { fg: (_color: string, text: string) => `\u001b[38;5;1m${text}\u001b[39m` }]) {
        const line = buildWidgetLines([row], 0, width, false, theme)![0];
        const plainLine = line.replace(/\u001b\[[0-9;]*m/g, "");
        assert.ok(plainLine.endsWith(" — /audit x"), `hint kept at width=${width}`);
        assert.equal(plainLine.includes("42t/s"), tps, `TPS at width=${width}`);
        assert.equal(plainLine.includes("p (l)") && plainLine.includes("132.4k") && plainLine.includes("$0.123"), group, `group at width=${width}`);
        assert.ok(visibleWidth(line) <= width, `bounded at width=${width}`);
      }
    }
  });

  test("RPC-backed rows read the record's rate; liveness-only and synthetic thread rows carry none", () => {
    const measured = record({ running: true, outputRate: { observe() {}, rate: () => 42 } });
    const unmeasured = record({ agentId: "22222222-2222-3333-4444-555555555555", running: true, outputRate: { observe() {}, rate: () => undefined } });
    const rows = buildAgentRows(registryOf(measured, unmeasured), [thread()], NOW);
    assert.equal(rows.find((row) => row.name === measured.agentId.slice(0, 8))?.outputTps, 42);
    assert.ok(!("outputTps" in rows.find((row) => row.name === unmeasured.agentId.slice(0, 8))!));
    assert.ok(!("outputTps" in rows.find((row) => row.role === "thread")!));
  });
});

// ---------------------------------------------------------------------------
// 261028 Phase 1: the expanded mode.
// ---------------------------------------------------------------------------

/** Appends a batch built through the real event path: one assistant message announcing `tools`, each call started then (unless `running`) ended. */
function addBatch(buffer: AgentBatchBuffer, tools: string[], startedAt: number, opts: { running?: boolean; endedAt?: number; args?: unknown } = {}): void {
  const calls = tools.map((name, i) => ({ type: "toolCall", id: `${startedAt}-${i}`, name, arguments: opts.args ?? { path: `${name}.ts` } }));
  observeBatchEvent(buffer, { type: "message_end", message: { role: "assistant", content: calls } }, startedAt);
  calls.forEach((call, i) => observeBatchEvent(buffer, { type: "tool_execution_start", toolCallId: call.id, toolName: call.name }, startedAt + 1 + i));
  if (opts.running) return;
  const endedAt = opts.endedAt ?? startedAt + 100;
  calls.forEach((call) => observeBatchEvent(buffer, { type: "tool_execution_end", toolCallId: call.id, result: { content: [{ type: "text", text: "ok" }] } }, endedAt));
}

let liveSeq = 0;
/** A locally-owned live (running) agent with a fresh batch buffer. */
function liveRecord(alias: string, overrides: Partial<RpcAgentRecord> = {}): RpcAgentRecord {
  liveSeq += 1;
  return record({
    agentId: `${String(liveSeq).padStart(8, "0")}-aaaa-bbbb-cccc-000000000000`,
    alias,
    client: {} as never,
    running: true,
    runStartedAt: NOW - 10_000 - (1_000 - liveSeq),
    toolBatches: createBatchBuffer(),
    ...overrides,
  });
}

const fakeTheme = { fg: (color: string, text: string) => `<${color}>${text}</${color}>` };
const SPINNER = "[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏]";

/** Rows for `registry` through the same path the controller uses. */
function expandedLines(registry: RpcAgentRegistry, opts: { width?: number; allotment?: number; budget?: number; theme?: typeof fakeTheme; threads?: ThreadRecord[] } = {}): string[] {
  const entries = buildAgentRowEntries(registry, opts.threads ?? [], NOW);
  return buildExpandedWidgetLines(entries, 0, {
    width: opts.width ?? 120, now: NOW, allotment: opts.allotment ?? 3, budget: opts.budget ?? 100,
    batches: collectBatchViews(registry), theme: opts.theme,
  }) ?? [];
}

/** A hand-built entry for the pure planner tests; `depth` puts it in a tree. */
function planEntry(name: string, state: AgentRowState, depth = 0, batchSource = true): AgentRowEntry {
  return { agentId: name, batchSource, row: { name, role: "worker", state, elapsedMs: 1_000, lastActivityMs: 1_000, depth } };
}

describe("expanded mode: batch row formatting", () => {
  test("a finished batch without a summary reads `called <names> · total N tool calls · <rel> ago`, singular for one call", () => {
    const buffer = createBatchBuffer();
    addBatch(buffer, ["read", "edit", "read"], NOW - 70_000, { endedAt: NOW - 65_000 });
    addBatch(buffer, ["bash"], NOW - 20_000, { endedAt: NOW - 5_000 });
    const [three, one] = buffer.batches.map(batchView);
    assert.equal(formatFinishedBatchFallback(three, NOW), "called read, edit · total 3 tool calls · 1m ago");
    assert.equal(formatFinishedBatchFallback(one, NOW), "called bash · total 1 tool call · 5s ago");
  });

  test("finished rows are dimmed through the theme and the running row is not", () => {
    const rec = liveRecord("alpha");
    addBatch(rec.toolBatches!, ["read"], NOW - 60_000);
    addBatch(rec.toolBatches!, ["grep"], NOW - 12_000, { running: true });
    const lines = expandedLines(registryOf(rec), { theme: fakeTheme });
    const finished = lines.find((line) => line.includes("called read"))!;
    assert.ok(finished.includes("<dim>called read · total 1 tool call · "), finished);
    const running = lines.find((line) => line.includes("grep"))!;
    assert.ok(!running.includes("<dim>"), running);
    assert.ok(!running.includes("called"));
  });

  test("the running row is `<spinner> <tool> (<args>) · <elapsed> · <done>/<n> done`, with `+<k> running` only when k > 0", () => {
    const buffer = createBatchBuffer();
    // Three parallel calls; the first (oldest) started 12s ago and is reported.
    observeBatchEvent(buffer, { type: "message_end", message: { role: "assistant", content: ["a", "b", "c"].map((id, i) => ({ type: "toolCall", id, name: ["read", "grep", "bash"][i], arguments: { path: `${id}.ts` } })) } }, NOW - 13_000);
    observeBatchEvent(buffer, { type: "tool_execution_start", toolCallId: "a", toolName: "read" }, NOW - 12_000);
    observeBatchEvent(buffer, { type: "tool_execution_start", toolCallId: "b", toolName: "grep" }, NOW - 11_000);
    observeBatchEvent(buffer, { type: "tool_execution_start", toolCallId: "c", toolName: "bash" }, NOW - 10_000);
    assert.match(formatRunningBatch(batchView(buffer.batches[0]), NOW, 120), new RegExp(`^${SPINNER} read \\(path: a\\.ts\\) · 12s · 0/3 done · \\+2 running$`));
    observeBatchEvent(buffer, { type: "tool_execution_end", toolCallId: "a" }, NOW - 8_000);
    assert.match(formatRunningBatch(batchView(buffer.batches[0]), NOW, 120), new RegExp(`^${SPINNER} grep \\(path: b\\.ts\\) · 11s · 1/3 done · \\+1 running$`));
    observeBatchEvent(buffer, { type: "tool_execution_end", toolCallId: "c" }, NOW - 7_000);
    const last = formatRunningBatch(batchView(buffer.batches[0]), NOW, 120);
    assert.match(last, new RegExp(`^${SPINNER} grep \\(path: b\\.ts\\) · 11s · 2/3 done$`));
    assert.ok(!last.includes("running"));
  });

  test("the spinner advances once per second and no tool-verb prefix column leads either row kind", () => {
    const rec = liveRecord("alpha");
    addBatch(rec.toolBatches!, ["read"], NOW - 60_000);
    addBatch(rec.toolBatches!, ["edit"], NOW - 5_000, { running: true });
    const view = batchView(rec.toolBatches!.batches[1]);
    assert.notEqual(formatRunningBatch(view, NOW, 80)[0], formatRunningBatch(view, NOW + 1_000, 80)[0]);
    const lines = expandedLines(registryOf(rec));
    for (const line of lines.slice(1)) {
      assert.doesNotMatch(line, /^\s*(read|ran|edit|bash)\s*\|/);
      assert.doesNotMatch(line, /\|/);
    }
    assert.match(lines[1], /^ {2}called read/, "a finished row is the 2-column indent plus its text, with no verb column");
    assert.match(lines[2], new RegExp(`^ {2}${SPINNER} edit \\(`), "the running row leads with the spinner, then the tool name");
  });

  test("only the args preview is truncated, so every line fits at 40, 80 and 120 columns", () => {
    const rec = liveRecord("alpha");
    addBatch(rec.toolBatches!, ["bash", "read", "grep"], NOW - 20_000, { running: true, args: { command: "x".repeat(200) } });
    // The first call alone finishes: 1/3 done, the two others still run.
    observeBatchEvent(rec.toolBatches!, { type: "tool_execution_end", toolCallId: `${NOW - 20_000}-0` }, NOW - 15_000);
    const other = liveRecord("beta");
    addBatch(other.toolBatches!, ["write"], NOW - 90_000);
    for (const width of [40, 80, 120]) {
      const lines = expandedLines(registryOf(rec, other), { width });
      assert.ok(lines.every((line) => visibleWidth(line) <= width), `width=${width}: ${lines.map((l) => visibleWidth(l)).join(",")}`);
      const running = lines.find((line) => new RegExp(`^ {2}${SPINNER} `).test(line))!;
      assert.ok(running, `width=${width}: a running row renders`);
      if (width >= 80) {
        assert.ok(running.includes("…) · "), `width=${width}: the args preview is truncated inside its parentheses`);
        assert.match(running, /· 19s · 1\/3 done · \+1 running$/, `width=${width}: the fixed fields survive intact`);
        assert.ok(!running.includes("x".repeat(100)));
      }
    }
  });
});

describe("expanded mode: row entries", () => {
  test("buildAgentRowEntries returns the buildAgentRows rows; every agent row is a batch source, a synthetic thread row is not", () => {
    const parent = liveRecord("parent", {
      subtreeDescendants: [
        { id: "child000-0000-0000-0000-000000000000", parentId: null, depth: 0, role: "explore", live: true },
        { id: "grand000-0000-0000-0000-000000000000", parentId: "child000-0000-0000-0000-000000000000", depth: 1, role: "worker", live: true },
      ],
    });
    const sibling = liveRecord("sibling");
    const threads = [thread({ threadId: "q9", title: "owner question", status: "pending" })];
    const registry = registryOf(parent, sibling);
    const entries = buildAgentRowEntries(registry, threads, NOW);
    assert.deepEqual(entries.map((entry) => entry.row), buildAgentRows(registry, threads, NOW));
    const byName = new Map(entries.map((entry) => [entry.row.name, entry]));
    assert.equal(byName.get("parent")!.batchSource, true);
    assert.equal(byName.get("parent")!.agentId, parent.agentId);
    assert.equal(byName.get("sibling")!.batchSource, true);
    const descendants = entries.filter((entry) => entry.agentId?.startsWith("child000") || entry.agentId?.startsWith("grand000"));
    assert.equal(descendants.length, 2);
    assert.ok(descendants.every((entry) => entry.batchSource === true), "Phase 2: propagated descendants own an allotment fed by the relayed agent-view model");
    const synthetic = entries.find((entry) => entry.row.role === "thread")!;
    assert.equal(synthetic.batchSource, false);
    assert.equal(synthetic.agentId, undefined);
  });
});

describe("expanded mode: fixed allotment", () => {
  test("each batch-source agent gets exactly `allotment` rows after its header, blank-padded; a descendant renders its relayed batches", () => {
    const childId = "child000-0000-0000-0000-000000000000";
    const busy = liveRecord("busy", {
      runStartedAt: NOW - 50_000,
      subtreeDescendants: [{ id: childId, parentId: null, depth: 0, role: "worker", live: true }],
      descendantViews: new Map([[childId, [{ seq: 4, toolNames: ["grep"], callCount: 2, startedAt: NOW - 9_000, endedAt: NOW - 8_000 }]]]),
    });
    addBatch(busy.toolBatches!, ["read"], NOW - 40_000);
    const fresh = liveRecord("fresh", { runStartedAt: NOW - 5_000 });
    const lines = expandedLines(registryOf(busy, fresh), { allotment: 3 });
    assert.equal(lines.length, 1 + 3 + 1 + 3 + 1 + 3, "busy header + 3, its descendant header + 3, fresh header + 3");
    assert.match(lines[0], /^· busy/);
    assert.match(lines[1], /^ {2}called read/);
    assert.deepEqual(lines.slice(2, 4), ["", ""], "blank padding up to the allotment at depth 0");
    assert.match(lines[4], /^│ · child000/);
    assert.match(lines[5], /^│ {3}called grep · total 2 tool calls · 8s ago/, "relayed rows sit under the descendant's gutter");
    assert.deepEqual(lines.slice(6, 8), ["│ ", "│ "], "a descendant's padding keeps its tree lane");
    assert.match(lines[8], /^· fresh/);
    assert.deepEqual(lines.slice(9), ["", "", ""], "an agent with no batches is all padding");
  });

  test("the latest batches are shown oldest-first with the running one last", () => {
    const rec = liveRecord("long");
    for (let i = 1; i <= 4; i++) addBatch(rec.toolBatches!, [`t${i}`], NOW - (5 - i) * 10_000);
    addBatch(rec.toolBatches!, ["t5"], NOW - 2_000, { running: true });
    const lines = expandedLines(registryOf(rec), { allotment: 3 });
    assert.equal(lines.length, 4);
    assert.match(lines[1], /called t3/);
    assert.match(lines[2], /called t4/);
    assert.match(lines[3], new RegExp(`^ {2}${SPINNER} t5 \\(`));
    assert.equal(expandedLines(registryOf(rec), { allotment: 1 }).length, 2);
    assert.match(expandedLines(registryOf(rec), { allotment: 1 })[1], new RegExp(`${SPINNER} t5`), "at allotment 1 the running batch alone remains");
  });

  test("the height is the same however many batches the agents hold", () => {
    const rec = liveRecord("steady");
    const heights: number[] = [];
    heights.push(expandedLines(registryOf(rec)).length);
    addBatch(rec.toolBatches!, ["read"], NOW - 30_000);
    heights.push(expandedLines(registryOf(rec)).length);
    addBatch(rec.toolBatches!, ["edit"], NOW - 20_000, { running: true });
    heights.push(expandedLines(registryOf(rec)).length);
    for (let i = 0; i < 6; i++) addBatch(rec.toolBatches!, ["bash"], NOW - 10_000 + i);
    heights.push(expandedLines(registryOf(rec)).length);
    assert.deepEqual(heights, [4, 4, 4, 4]);
  });
});

describe("expanded mode: sizing", () => {
  test("chooseBatchAllotment shrinks uniformly from the requested rows down to 1", () => {
    const three = [planEntry("a", "running"), planEntry("b", "running"), planEntry("c", "running")];
    // headers + sources * n must fit the budget.
    assert.equal(chooseBatchAllotment(three, 5, 18), 5);
    assert.equal(chooseBatchAllotment(three, 5, 17), 4);
    assert.equal(chooseBatchAllotment(three, 5, 15), 4);
    assert.equal(chooseBatchAllotment(three, 5, 14), 3);
    assert.equal(chooseBatchAllotment(three, 5, 12), 3);
    assert.equal(chooseBatchAllotment(three, 5, 11), 2);
    assert.equal(chooseBatchAllotment(three, 5, 9), 2);
    assert.equal(chooseBatchAllotment(three, 5, 8), 1);
    assert.equal(chooseBatchAllotment(three, 5, 0), 1, "never below one row");
    assert.equal(chooseBatchAllotment(three, 2, 100), 2, "never above the requested rows");
    const withDescendant = [...three, planEntry("d", "running", 1, false)];
    assert.equal(chooseBatchAllotment(withDescendant, 5, 4 + 3 * 5), 5, "a non-source header costs one row but no allotment");
    assert.equal(chooseBatchAllotment(withDescendant, 5, 4 + 3 * 5 - 1), 4);
  });

  test("computeExpandedBudget is terminal rows minus the reserved dock minus 5, at least 1", () => {
    assert.equal(computeExpandedBudget(40, 10), 25);
    assert.equal(computeExpandedBudget(40, 0), 35);
    assert.equal(computeExpandedBudget(40.9, 10.9), 25, "fractions floor");
    assert.equal(computeExpandedBudget(15, 10), 1);
    assert.equal(computeExpandedBudget(10, 30), 1);
    assert.equal(computeExpandedBudget(30, -5), 25, "a negative reservation counts as none");
  });

  test("resolveAgentWidgetRows clamps to 1..5 and defaults to 3", () => {
    assert.equal(DEFAULT_AGENT_WIDGET_BATCH_ROWS, 3);
    assert.equal(MAX_AGENT_WIDGET_BATCH_ROWS, 5);
    for (const [input, expected] of [[1, 1], [2, 2], [3, 3], [5, 5], [6, 5], [99, 5], [0, 3], [-2, 3], [2.5, 3], [Number.NaN, 3], ["4", 3], [undefined, 3], [null, 3]] as const) {
      assert.equal(resolveAgentWidgetRows(input), expected, String(input));
    }
  });
});

describe("expanded mode: planExpandedRows fold", () => {
  const running = (n: number) => Array.from({ length: n }, (_, i) => planEntry(`r${i}`, "running"));
  const names = (plan: ReturnType<typeof planExpandedRows>) => plan.items.map((item) => item.entry.row.name);

  test("everything shown when it fits the budget", () => {
    const plan = planExpandedRows(running(3), 2, 9);
    assert.equal(plan.hidden, 0);
    assert.equal(plan.padding, 0);
    assert.deepEqual(plan.items.map((item) => item.batchRows), [2, 2, 2]);
  });

  test("over budget at allotment 1, running agents fold into a `+N more` tail and the output pads to the budget", () => {
    const entries = running(6);
    const plan = planExpandedRows(entries, 1, 7);
    assert.deepEqual(names(plan), ["r0", "r1", "r2"]);
    assert.equal(plan.hidden, 3);
    assert.equal(plan.padding, 0);
    const padded = planExpandedRows(entries, 1, 8);
    assert.equal(padded.hidden, 3);
    assert.equal(padded.padding, 1, "the unused row is padding, so the height stays at the budget");

    for (const budget of [7, 8, 9]) {
      const lines = buildExpandedWidgetLines(entries, 0, { width: 80, now: NOW, allotment: 1, budget, batches: new Map() })!;
      assert.equal(lines.length, budget, `budget=${budget}`);
      assert.ok(lines.some((line) => /^\+\d+ more$/.test(line)), `budget=${budget}: the tail is present`);
    }
    const folded = buildExpandedWidgetLines(entries, 0, { width: 80, now: NOW, allotment: 1, budget: 7, batches: new Map() })!;
    assert.equal(folded.at(-1), "+3 more");
  });

  test("an awaiting agent never folds and keeps its ancestors' headers; only running agents collapse", () => {
    const entries = [
      planEntry("r0", "running"),
      planEntry("r1", "running"),
      planEntry("parent", "running"),
      planEntry("waiting", "awaiting-approval", 1, false),
      planEntry("r2", "running"),
    ];
    const plan = planExpandedRows(entries, 1, 4);
    assert.deepEqual(names(plan), ["parent", "waiting"], "the awaiting row brings its running ancestor along");
    assert.equal(plan.hidden, 3);
    assert.deepEqual(plan.items.map((item) => item.batchRows), [1, 0]);
    const lines = buildExpandedWidgetLines(entries, 0, { width: 80, now: NOW, allotment: 1, budget: 4, batches: new Map() })!;
    assert.equal(lines.length, 4);
    assert.match(lines[0], /parent/);
    assert.match(lines[2], /^│ .*waiting/);
    assert.equal(lines[3], "+3 more");
  });

  test("every ancestor up the chain is kept, and an awaiting-owner row at the top level needs none", () => {
    const entries = [
      planEntry("filler", "running"),
      planEntry("root", "running"),
      planEntry("mid", "running", 1, false),
      planEntry("leaf", "awaiting-owner", 2, false),
      planEntry("ask", "awaiting-owner"),
    ];
    const plan = planExpandedRows(entries, 1, 3);
    assert.deepEqual(names(plan), ["root", "mid", "leaf", "ask"]);
    assert.equal(plan.hidden, 1);
    assert.deepEqual(plan.items.map((item) => item.batchRows), [0, 0, 0, 0], "no budget is left for batch rows");
  });

  test("when the kept headers alone exceed the budget, the output exceeds the budget instead of hiding them", () => {
    const entries = [
      planEntry("w0", "awaiting-owner"),
      planEntry("w1", "awaiting-approval"),
      planEntry("w2", "idle-awaiting-owner"),
      planEntry("w3", "awaiting-approval"),
      planEntry("run", "running"),
    ];
    const plan = planExpandedRows(entries, 1, 2);
    assert.deepEqual(names(plan), ["w0", "w1", "w2", "w3"]);
    assert.equal(plan.hidden, 1);
    assert.equal(plan.padding, 0);
    const lines = buildExpandedWidgetLines(entries, 0, { width: 80, now: NOW, allotment: 1, budget: 2, batches: new Map() })!;
    assert.equal(lines.length, 5, "four awaiting headers plus the tail, over the budget of 2");
    assert.equal(lines.at(-1), "+1 more");
    for (const name of ["w0", "w1", "w2", "w3"]) assert.ok(lines.some((line) => line.includes(name)), `${name} stays visible`);
  });
});

describe("expanded mode: measureDockRows", () => {
  const block = (height: number, seen?: number[]) => ({ render: (width: number) => { seen?.push(width); return Array.from({ length: height }, () => "x"); } });

  test("sums the render heights of tui.children after the first and skips the widget inside its container", () => {
    const self = block(7);
    const widths: number[] = [];
    const tui = { children: [block(100), { children: [block(2, widths), self, block(1)], render: () => ["never-used"] }, block(3)] };
    assert.equal(measureDockRows(tui, self, 90), 2 + 1 + 3);
    assert.deepEqual(widths, [90], "children render at the widget's width");
  });

  test("skips the widget when it is itself a top-level child", () => {
    const self = block(4);
    assert.equal(measureDockRows({ children: [block(50), block(2), self, block(1)] }, self, 80), 3);
  });

  test("undefined when the widget is not found or the host shape is not as expected", () => {
    const self = block(4);
    assert.equal(measureDockRows({ children: [block(50), block(2)] }, self, 80), undefined, "widget not found");
    assert.equal(measureDockRows({}, self, 80), undefined, "children absent");
    assert.equal(measureDockRows(undefined, self, 80), undefined);
    assert.equal(measureDockRows({ children: [block(50)] }, self, 80), undefined, "only the transcript");
    assert.equal(measureDockRows({ children: "nope" }, self, 80), undefined);
    const throwing = { children: [block(1), { render() { throw new Error("boom"); } }, self] };
    assert.equal(measureDockRows(throwing, self, 80), undefined, "a throwing sibling falls back rather than crashing the paint");
  });
});

describe("expanded mode: controller", () => {
  type Factory = (tui: unknown, theme: unknown) => { render(width: number): string[] };

  /** Mocks the clock and the interval API; returns the live interval cadences. */
  function installClock(t: { mock: { method: (...args: any[]) => unknown } }) {
    const live = new Map<number, number>();
    let next = 1;
    t.mock.method(Date, "now", () => NOW);
    t.mock.method(global, "setInterval", ((_callback: () => void, ms: number) => { const id = next++; live.set(id, ms); return { id, unref() {} } as never; }) as typeof setInterval);
    t.mock.method(global, "clearInterval", ((timer: { id: number }) => { live.delete(timer.id); }) as typeof clearInterval);
    return { live: () => [...live.values()].sort((a, b) => a - b) };
  }

  function harness(registry: RpcAgentRegistry, options: Parameters<typeof createAgentWidgetController>[3] = {}) {
    const state: { factory?: Factory; paints: number } = { paints: 0 };
    const controller = createAgentWidgetController({ ui: {
      setWidget(_key, content) { state.paints += 1; state.factory = typeof content === "function" ? content as Factory : undefined; },
      setStatus() {},
    } }, registry, new Map(), options);
    const render = (rows = 40, width = 120, children?: unknown[]) => state.factory!({ terminal: { rows }, children }, {}).render(width);
    return { controller, state, render };
  }

  test("toggleExpanded and isExpanded switch modes and repaint", (t) => {
    installClock(t);
    const rec = liveRecord("alpha");
    addBatch(rec.toolBatches!, ["read"], NOW - 30_000);
    const { controller, state, render } = harness(registryOf(rec));
    controller.refresh();
    assert.equal(controller.isExpanded(), false, "every launch starts in the normal mode");
    assert.equal(render().length, 1, "normal mode: the header alone");
    const before = state.paints;
    assert.equal(controller.toggleExpanded(), true);
    assert.equal(controller.isExpanded(), true);
    assert.ok(state.paints > before, "toggling repaints");
    const lines = render();
    assert.equal(lines.length, 1 + DEFAULT_AGENT_WIDGET_BATCH_ROWS);
    assert.match(lines[1], /called read/);
    assert.equal(controller.toggleExpanded(), false);
    assert.equal(render().length, 1, "back to the normal mode");
    controller.stop();
  });

  test("a new controller starts normal regardless of an earlier one's state", (t) => {
    installClock(t);
    const rec = liveRecord("alpha");
    const first = harness(registryOf(rec));
    first.controller.toggleExpanded();
    first.controller.stop();
    const second = harness(registryOf(rec));
    assert.equal(second.controller.isExpanded(), false);
    second.controller.refresh();
    assert.equal(second.render().length, 1);
    second.controller.stop();
  });

  test("the expanded height holds while batches come and go, and moves on enter/leave and on terminal resize", (t) => {
    installClock(t);
    const a = liveRecord("alpha");
    const b = liveRecord("beta");
    const registry = registryOf(a);
    const { controller, render } = harness(registry);
    controller.toggleExpanded();
    assert.equal(render().length, 4);

    addBatch(a.toolBatches!, ["read"], NOW - 50_000);
    controller.refresh();
    assert.equal(render().length, 4, "a first batch");
    addBatch(a.toolBatches!, ["edit"], NOW - 20_000, { running: true });
    controller.refresh();
    assert.equal(render().length, 4, "a running batch");
    closeOpenBatch(a.toolBatches!, NOW - 1_000);
    for (let i = 0; i < 6; i++) addBatch(a.toolBatches!, ["bash"], NOW - 10_000 + i);
    controller.refresh();
    assert.equal(render().length, 4, "finished and many batches");

    registry.set(b.agentId, b);
    controller.refresh();
    assert.equal(render().length, 8, "an agent entering adds its header and allotment");
    registry.delete(b.agentId);
    controller.refresh();
    assert.equal(render().length, 4, "an agent leaving removes them");

    // No dock children: the estimated reservation applies, so a short terminal shrinks the allotment.
    assert.equal(render(18).length, 1 + 2, "rows 18 -> budget 18 - 10 - 5 = 3 -> allotment 2, recomputed on resize");
    assert.equal(render(40).length, 4, "and back when the terminal grows");
    controller.stop();
  });

  test("the dock is measured from tui.children when the host exposes it", (t) => {
    installClock(t);
    const recs = ["a", "b", "c"].map((alias) => liveRecord(alias));
    const { controller, state } = harness(registryOf(...recs), { rowsPerAgent: () => 5 });
    controller.toggleExpanded();
    const block = (height: number) => ({ render: () => Array.from({ length: height }, () => "x") });
    const tui: { terminal: { rows: number }; children: unknown[] } = { terminal: { rows: 30 }, children: [] };
    const component = state.factory!(tui, {});
    tui.children.push(block(60), { children: [component, block(5)], render: () => [] }, block(2));
    // Measured reservation 5 + 2 = 7: budget 30 - 7 - 5 = 18 = 3 headers + 3 * 5.
    assert.equal(component.render(100).length, 18);
    assert.equal(ESTIMATED_DOCK_ROWS, 10);
    controller.stop();

    // Without children the 10-row estimate gives budget 15 -> allotment 4 -> 15 rows.
    const bare = harness(registryOf(...recs), { rowsPerAgent: () => 5 });
    bare.controller.toggleExpanded();
    assert.equal(bare.state.factory!({ terminal: { rows: 30 } }, {}).render(100).length, 15);
    bare.controller.stop();
  });

  test("the repaint timer is 1s only while expanded with a running batch, else the 10s tick", (t) => {
    const clock = installClock(t);
    const rec = liveRecord("alpha");
    addBatch(rec.toolBatches!, ["read"], NOW - 30_000);
    const { controller } = harness(registryOf(rec));
    assert.equal(AGENT_WIDGET_EXPANDED_TICK_MS, 1_000);
    controller.refresh();
    assert.deepEqual(clock.live(), [AGENT_WIDGET_TICK_MS], "normal mode");
    controller.toggleExpanded();
    assert.deepEqual(clock.live(), [AGENT_WIDGET_TICK_MS], "expanded but nothing running");

    addBatch(rec.toolBatches!, ["edit"], NOW - 3_000, { running: true });
    controller.refresh();
    assert.deepEqual(clock.live(), [AGENT_WIDGET_EXPANDED_TICK_MS], "expanded with a running batch swaps to the fast tick");

    closeOpenBatch(rec.toolBatches!, NOW);
    controller.refresh();
    assert.deepEqual(clock.live(), [AGENT_WIDGET_TICK_MS], "the batch finished");

    addBatch(rec.toolBatches!, ["grep"], NOW - 1_000, { running: true });
    controller.refresh();
    assert.deepEqual(clock.live(), [AGENT_WIDGET_EXPANDED_TICK_MS]);
    controller.toggleExpanded();
    assert.deepEqual(clock.live(), [AGENT_WIDGET_TICK_MS], "leaving the expanded mode drops back");
    controller.stop();
    assert.deepEqual(clock.live(), []);
  });

  test("rowsPerAgent changes the allotment, read synchronously or as a Promise, and is clamped", async (t) => {
    installClock(t);
    const rec = liveRecord("alpha");
    const sync = harness(registryOf(rec), { rowsPerAgent: () => 2 });
    sync.controller.toggleExpanded();
    assert.equal(sync.render().length, 3);
    sync.controller.stop();

    const clamped = harness(registryOf(rec), { rowsPerAgent: () => 9 });
    clamped.controller.toggleExpanded();
    assert.equal(clamped.render().length, 1 + MAX_AGENT_WIDGET_BATCH_ROWS);
    clamped.controller.stop();

    const invalid = harness(registryOf(rec), { rowsPerAgent: () => 0 });
    invalid.controller.toggleExpanded();
    assert.equal(invalid.render().length, 1 + DEFAULT_AGENT_WIDGET_BATCH_ROWS);
    invalid.controller.stop();

    const lazy = harness(registryOf(rec), { rowsPerAgent: async () => 5 });
    lazy.controller.toggleExpanded();
    assert.equal(lazy.render().length, 1 + DEFAULT_AGENT_WIDGET_BATCH_ROWS, "the default applies while the read is pending");
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(lazy.render().length, 1 + 5, "the landed value repaints");
    lazy.controller.stop();
  });
});

describe("registerAgentWidgetCommand", () => {
  function fakePi() {
    const commands = new Map<string, { description: string; handler: (args: string, ctx: { ui?: { notify?(message: string, type?: string): void } }) => unknown }>();
    return { commands, pi: { registerCommand: (name: string, options: never) => { commands.set(name, options); } } };
  }

  test("registers `agents`; the handler toggles the live handle", () => {
    const { commands, pi } = fakePi();
    let toggles = 0;
    const handle = { toggleExpanded: () => { toggles += 1; return true; }, isExpanded: () => true, refresh() {}, stop() {} } as AgentWidgetController;
    registerAgentWidgetCommand(pi, () => handle);
    assert.equal(AGENT_WIDGET_COMMAND, "agents");
    assert.deepEqual([...commands.keys()], ["agents"]);
    assert.ok(commands.get("agents")!.description.length > 0);
    const notices: unknown[] = [];
    commands.get("agents")!.handler("", { ui: { notify: (...args) => { notices.push(args); } } });
    assert.equal(toggles, 1);
    assert.deepEqual(notices, []);
  });

  test("the handle is read at each invocation; with none it notifies a warning and toggles nothing", () => {
    const { commands, pi } = fakePi();
    let handle: AgentWidgetController | undefined;
    let toggles = 0;
    registerAgentWidgetCommand(pi, () => handle);
    const notices: Array<[string, string | undefined]> = [];
    const ctx = { ui: { notify: (message: string, type?: string) => { notices.push([message, type]); } } };
    commands.get("agents")!.handler("", ctx);
    assert.equal(notices.length, 1);
    assert.equal(notices[0][1], "warning");
    assert.match(notices[0][0], /TUI/);
    assert.doesNotThrow(() => commands.get("agents")!.handler("", {}), "no ui at all is tolerated");

    handle = { toggleExpanded: () => { toggles += 1; return false; }, isExpanded: () => false, refresh() {}, stop() {} } as AgentWidgetController;
    commands.get("agents")!.handler("", ctx);
    assert.equal(toggles, 1, "a handle that appears later is used");
    assert.equal(notices.length, 1);
  });
});
