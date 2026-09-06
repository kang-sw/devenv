/**
 * Unit tests for agent-sidecar.ts (260905, push model): the shutdown orphan
 * sidecar that keeps a crashed lead's still-live children from vanishing
 * silently once `ws-agent-wait` no longer exists to go looking for them.
 *
 * Covers the pure seams directly (`captureOrphans`'s selection rule,
 * `serializeOrphans`/`parseOrphans`'s round-trip and every tolerated
 * corruption, `rehydrateOrphanRecord`'s dormant resting state,
 * `buildOrphanSummary`) plus the two thin filesystem functions against a real
 * tmpdir — `readAndClearSidecar`'s delete-on-read contract is the whole point
 * of the file and cannot be asserted without touching disk.
 *
 * Review relay #1 (I1) adds `reviveOrphans`, the role-keyed re-arm that turns
 * a persisted `spawnRole` back into behavior: the full capture -> serialize ->
 * parse -> revive round trip, ending in the assertion the ticket's own verify
 * list names — a revived fork's `kind:"question"` routes to the owner surface
 * instead of being pushed at the lead.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { existsSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  SIDECAR_VERSION,
  MID_TURN_ORPHAN_CAVEAT,
  buildOrphanPush,
  buildOrphanSummary,
  captureOrphans,
  partitionOrphansByState,
  parseOrphans,
  readAndClearSidecar,
  rehydrateOrphanRecord,
  serializeOrphans,
  sidecarPath,
  reviveOrphans,
  writeSidecar,
  type PersistedOrphan,
} from "../src/agent-sidecar.ts";
import { armForkRoleWiring } from "../src/fork.ts";
import { applyRpcEvent, listAgents, REPORT_TO_LEAD_TOOL_NAME, type RpcAgentRecord, type RpcAgentRegistry } from "../src/spawner.ts";
import type { ExtensionAPI, RpcClient } from "@earendil-works/pi-coding-agent";

function record(overrides: Partial<RpcAgentRecord> = {}): RpcAgentRecord {
  return {
    agentId: "a1",
    sessionPath: "/tmp/ws-pi-agent-x/session.jsonl",
    systemPromptPath: "/tmp/ws-pi-agent-x/prompt.md",
    wsToolNames: ["ws__todo_list"],
    toolGroup: "full-worker",
    streaming: false,
    running: false,
    reportLog: [],
    ...overrides,
  };
}

function withTempDir<T>(fn: (dir: string) => T): T {
  const dir = mkdtempSync(join(tmpdir(), "ws-pi-agent-sidecar-test-"));
  try {
    return fn(dir);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

describe("sidecarPath", () => {
  test("is a sibling of the lead's own session file, matching ask.ts's thread-registry convention", () => {
    assert.equal(sidecarPath("/home/u/.pi/sessions/s1.jsonl"), "/home/u/.pi/sessions/s1.jsonl.ws-agents.json");
  });
});

describe("captureOrphans", () => {
  test("260905 (alias/park/cap): captures both LIVE and dormant/parked records — automatic parking means a settled child is routinely dormant well before shutdown", () => {
    const registry: RpcAgentRegistry = new Map([
      ["live", record({ agentId: "live", client: {} as RpcClient })],
      ["dormant", record({ agentId: "dormant" })],
    ]);
    assert.deepEqual(
      captureOrphans(registry)
        .map((o) => o.agentId)
        .sort(),
      ["dormant", "live"],
      "a dormant record is still resumable — capturing it too keeps the roll-call complete",
    );
  });

  test("skips a threadBound record — the owner surface's own registry file already persists it", () => {
    const registry: RpcAgentRegistry = new Map([
      ["worker", record({ agentId: "worker", client: {} as RpcClient })],
      ["discussing", record({ agentId: "discussing", client: {} as RpcClient, threadBound: true })],
    ]);
    assert.deepEqual(
      captureOrphans(registry).map((o) => o.agentId),
      ["worker"],
    );
  });

  test("260906: skips a oneShot record (a lead explore) — it has no dormant-resumable resting state to revive; a non-oneShot record is unaffected", () => {
    const registry: RpcAgentRegistry = new Map([
      ["worker", record({ agentId: "worker", client: {} as RpcClient })],
      ["explore", record({ agentId: "explore", client: {} as RpcClient, oneShot: true, spawnRole: "explore" })],
    ]);
    assert.deepEqual(
      captureOrphans(registry).map((o) => o.agentId),
      ["worker"],
    );
  });

  test("records the state at shutdown and the last-report time (relay #2: the roll-call needs both)", () => {
    const registry: RpcAgentRegistry = new Map([
      ["busy", record({ agentId: "busy", client: {} as RpcClient, running: true, reportLog: [{ at: 1_000 }, { kind: "final", at: 2_000 }] })],
      ["quiet", record({ agentId: "quiet", client: {} as RpcClient, running: false })],
    ]);
    const [busy, quiet] = captureOrphans(registry);
    assert.equal(busy.state, "running", "a prompt was still outstanding when the session went away");
    assert.equal(busy.lastReportAt, new Date(2_000).toISOString(), "the NEWEST report, not the first");
    assert.equal(quiet.state, "idle");
    assert.equal(quiet.lastReportAt, undefined, "a child that never reported carries no time at all");
  });

  test("carries exactly the resume fields plus spawnRole/alias/title/prompt, and nothing runtime-only", () => {
    const registry: RpcAgentRegistry = new Map([
      [
        "a1",
        record({
          client: {} as RpcClient,
          alias: "scout",
          title: "Reviews the auth module",
          prompt: "Please review src/auth.ts for bugs",
          modelBase: "prov/model",
          modelEffort: "high",
          explicitTools: "bash,ws-report-to-lead",
          spawnRole: "fork",
          running: true,
          streaming: true,
          reportLog: [{ kind: "final", at: 1 }],
        }),
      ],
    ]);
    assert.deepEqual(captureOrphans(registry), [
      {
        agentId: "a1",
        alias: "scout",
        title: "Reviews the auth module",
        prompt: "Please review src/auth.ts for bugs",
        sessionPath: "/tmp/ws-pi-agent-x/session.jsonl",
        systemPromptPath: "/tmp/ws-pi-agent-x/prompt.md",
        modelBase: "prov/model",
        modelEffort: "high",
        wsToolNames: ["ws__todo_list"],
        toolGroup: "full-worker",
        explicitTools: "bash,ws-report-to-lead",
        spawnRole: "fork",
        state: "running",
        lastReportAt: new Date(1).toISOString(),
      },
    ]);
  });

  test("copies the tool-name list rather than aliasing the live record's array", () => {
    const live = record({ client: {} as RpcClient });
    const [orphan] = captureOrphans(new Map([["a1", live]]));
    assert.notEqual(orphan.wsToolNames, live.wsToolNames);
    assert.deepEqual(orphan.wsToolNames, [...live.wsToolNames]);
  });

  test("review relay #1 (Important): a revived orphan that never reports again still has its lastReportAt survive a SECOND capture, via lastReportAtOverride", () => {
    const lastReportAt = new Date(1_700_000_060_000).toISOString();
    const revived = rehydrateOrphanRecord({
      agentId: "a1",
      sessionPath: "/tmp/s1.jsonl",
      systemPromptPath: "/tmp/p1.md",
      wsToolNames: [],
      toolGroup: "full-worker",
      lastReportAt,
    });
    // reportLog is empty on the revived record (no synthetic entry) — before
    // the fix, captureOrphans's lastReportAt(record) helper read only
    // reportLog and would drop the value here.
    assert.deepEqual(revived.reportLog, []);
    const [recaptured] = captureOrphans(new Map([["a1", revived]]));
    assert.equal(recaptured.lastReportAt, lastReportAt, "a revive-then-shutdown-again cycle must not silently drop the last-report time");
  });
});

describe("serializeOrphans / parseOrphans", () => {
  const orphans: PersistedOrphan[] = [
    {
      agentId: "a1",
      alias: "scout",
      title: "Reviews the auth module",
      prompt: "Please review src/auth.ts for bugs",
      sessionPath: "/tmp/s1.jsonl",
      systemPromptPath: "/tmp/p1.md",
      modelBase: "prov/model",
      modelEffort: "high",
      wsToolNames: ["ws__todo_list"],
      toolGroup: "full-worker",
      explicitTools: "bash",
      spawnRole: "fork",
      state: "running",
      lastReportAt: "2026-09-05T10:00:00.000Z",
    },
  ];

  test("round-trips a full orphan set unchanged, state, last-report time and alias/title/prompt included", () => {
    assert.deepEqual(parseOrphans(serializeOrphans(orphans)), orphans);
  });

  test("260905 (alias/park/cap): an old-shape orphan with no alias/title/prompt still round-trips (they parse as undefined, not invented)", () => {
    const oldShape: PersistedOrphan[] = [
      {
        agentId: "a2",
        sessionPath: "/tmp/s2.jsonl",
        systemPromptPath: "/tmp/p2.md",
        wsToolNames: [],
        toolGroup: "full-worker",
        state: "idle",
      },
    ];
    const [parsed] = parseOrphans(serializeOrphans(oldShape));
    assert.equal(parsed.alias, undefined);
    assert.equal(parsed.title, undefined);
    assert.equal(parsed.prompt, undefined);
  });

  test("relay #2: an idle orphan with no reports round-trips with lastReportAt absent, not invented", () => {
    const idle: PersistedOrphan[] = [
      { agentId: "a2", sessionPath: "/tmp/s.jsonl", systemPromptPath: "/tmp/p.md", wsToolNames: [], toolGroup: "full-worker", state: "idle" },
    ];
    const [parsed] = parseOrphans(serializeOrphans(idle));
    assert.equal(parsed.state, "idle");
    assert.equal(parsed.lastReportAt, undefined);
  });

  test("relay #2: a sidecar written before these fields existed still parses, reading as idle", () => {
    const raw = JSON.stringify({
      version: SIDECAR_VERSION,
      orphans: [{ agentId: "old", sessionPath: "/tmp/s.jsonl", systemPromptPath: "/tmp/p.md", wsToolNames: [], toolGroup: "full-worker" }],
    });
    const [parsed] = parseOrphans(raw);
    assert.equal(parsed.agentId, "old", "an older sidecar is still revivable — no crash, no dropped entry");
    assert.equal(parsed.state, "idle", "the conservative read: claim nothing about work that may not have been outstanding");
    assert.equal(parsed.lastReportAt, undefined);
  });

  test("relay #2: a corrupt state value degrades to idle rather than poisoning the entry", () => {
    const raw = JSON.stringify({
      version: SIDECAR_VERSION,
      orphans: [
        { agentId: "a", sessionPath: "/tmp/s.jsonl", systemPromptPath: "/tmp/p.md", wsToolNames: [], toolGroup: "full-worker", state: 42, lastReportAt: 7 },
      ],
    });
    const [parsed] = parseOrphans(raw);
    assert.equal(parsed.state, "idle");
    assert.equal(parsed.lastReportAt, undefined);
  });

  test("stamps the version and a writtenAt, and ends with a newline so the file is readable by hand", () => {
    const raw = serializeOrphans(orphans, "2026-09-05T00:00:00.000Z");
    const parsed = JSON.parse(raw) as { version: number; writtenAt: string };
    assert.equal(parsed.version, SIDECAR_VERSION);
    assert.equal(parsed.writtenAt, "2026-09-05T00:00:00.000Z");
    assert.ok(raw.endsWith("\n"));
  });

  test("malformed JSON degrades to no orphans rather than throwing — session_start must always come up", () => {
    assert.deepEqual(parseOrphans("{not json"), []);
    assert.deepEqual(parseOrphans(""), []);
    assert.deepEqual(parseOrphans("null"), []);
    assert.deepEqual(parseOrphans("[]"), []);
  });

  test("a version mismatch or a non-array orphans field yields no orphans", () => {
    assert.deepEqual(parseOrphans(JSON.stringify({ version: SIDECAR_VERSION + 1, orphans })), []);
    assert.deepEqual(parseOrphans(JSON.stringify({ version: SIDECAR_VERSION, orphans: "nope" })), []);
  });

  test("entries missing a load-bearing field are dropped individually; the rest survive", () => {
    const raw = JSON.stringify({
      version: SIDECAR_VERSION,
      orphans: [
        { sessionPath: "/tmp/s.jsonl", systemPromptPath: "/tmp/p.md" }, // no agentId
        { agentId: "b", systemPromptPath: "/tmp/p.md" }, // no sessionPath
        null,
        orphans[0],
      ],
    });
    assert.deepEqual(
      parseOrphans(raw).map((o) => o.agentId),
      ["a1"],
    );
  });

  test("a missing wsToolNames/toolGroup falls back to an empty list and full-worker rather than producing an unusable record", () => {
    const raw = JSON.stringify({
      version: SIDECAR_VERSION,
      orphans: [{ agentId: "a", sessionPath: "/tmp/s.jsonl", systemPromptPath: "/tmp/p.md" }],
    });
    const [parsed] = parseOrphans(raw);
    assert.deepEqual(parsed.wsToolNames, []);
    assert.equal(parsed.toolGroup, "full-worker");
  });

  test("review relay #1 (Minor a): a lastReportAt that is a string but does not parse as a date is rejected, not passed through to poison later Date.parse arithmetic", () => {
    const raw = JSON.stringify({
      version: SIDECAR_VERSION,
      orphans: [
        { agentId: "a", sessionPath: "/tmp/s.jsonl", systemPromptPath: "/tmp/p.md", wsToolNames: [], toolGroup: "full-worker", lastReportAt: "not-a-date" },
      ],
    });
    const [parsed] = parseOrphans(raw);
    assert.equal(parsed.lastReportAt, undefined);
  });
});

describe("rehydrateOrphanRecord", () => {
  test("rebuilds a DORMANT record: no client, not running, not streaming, empty report log", () => {
    const revived = rehydrateOrphanRecord({
      agentId: "a1",
      sessionPath: "/tmp/s1.jsonl",
      systemPromptPath: "/tmp/p1.md",
      wsToolNames: ["ws__todo_list"],
      toolGroup: "full-worker",
      spawnRole: "worker",
    });
    assert.equal(revived.client, undefined, "client === undefined is what routes ws-agent-send into sendToAgent's relaunch branch");
    assert.equal(revived.running, false, "a revived orphan must not count toward the fan-in until the lead actually prompts it");
    assert.equal(revived.streaming, false);
    assert.deepEqual(revived.reportLog, []);
    assert.equal(revived.spawnRole, "worker");
    assert.equal(revived.sessionPath, "/tmp/s1.jsonl");
  });

  test("copies the tool-name list instead of aliasing the parsed array", () => {
    const orphan: PersistedOrphan = {
      agentId: "a1",
      sessionPath: "/tmp/s.jsonl",
      systemPromptPath: "/tmp/p.md",
      wsToolNames: ["x"],
      toolGroup: "full-worker",
    };
    const revived = rehydrateOrphanRecord(orphan);
    assert.notEqual(revived.wsToolNames, orphan.wsToolNames);
    assert.deepEqual(revived.wsToolNames, orphan.wsToolNames);
  });

  test("260905 (list-model/last-report-fidelity): a rehydrated orphan lists the sidecar's last_report_at via lastReportAtOverride", () => {
    const lastReportAt = new Date(1_700_000_060_000).toISOString();
    const revived = rehydrateOrphanRecord({
      agentId: "a1",
      sessionPath: "/tmp/s1.jsonl",
      systemPromptPath: "/tmp/p1.md",
      wsToolNames: [],
      toolGroup: "full-worker",
      lastReportAt,
    });
    const registry: RpcAgentRegistry = new Map([["a1", revived]]);
    const [entry] = listAgents(registry);
    assert.equal(entry.last_report_at, lastReportAt);
  });

  test("260905 (list-model/last-report-fidelity): a rehydrated orphan that reports afterwards lists the new time, not the stale override", () => {
    const staleOverride = new Date(1_700_000_060_000).toISOString();
    const revived = rehydrateOrphanRecord({
      agentId: "a1",
      sessionPath: "/tmp/s1.jsonl",
      systemPromptPath: "/tmp/p1.md",
      wsToolNames: [],
      toolGroup: "full-worker",
      lastReportAt: staleOverride,
    });
    const newReportAt = 1_700_000_120_000;
    revived.reportLog.push({ at: newReportAt });
    const registry: RpcAgentRegistry = new Map([["a1", revived]]);
    const [entry] = listAgents(registry);
    assert.equal(entry.last_report_at, new Date(newReportAt).toISOString());
  });

  test("review relay #1 (Minor c): any real reportLog entry wins over the override even when the override is numerically NEWER", () => {
    const laterOverride = new Date(1_700_000_120_000).toISOString();
    const revived = rehydrateOrphanRecord({
      agentId: "a1",
      sessionPath: "/tmp/s1.jsonl",
      systemPromptPath: "/tmp/p1.md",
      wsToolNames: [],
      toolGroup: "full-worker",
      lastReportAt: laterOverride,
    });
    const earlierReportAt = 1_700_000_060_000;
    revived.reportLog.push({ at: earlierReportAt });
    const registry: RpcAgentRegistry = new Map([["a1", revived]]);
    const [entry] = listAgents(registry);
    assert.equal(
      entry.last_report_at,
      new Date(earlierReportAt).toISOString(),
      "the contract is 'any real report wins', not 'the larger timestamp wins' — the override must never resurface once reportLog is non-empty",
    );
  });
});

describe("buildOrphanSummary", () => {
  function orphan(overrides: Partial<PersistedOrphan> = {}): PersistedOrphan {
    return { agentId: "a1", sessionPath: "s", systemPromptPath: "p", wsToolNames: [], toolGroup: "full-worker", ...overrides };
  }

  test("names every RUNNING orphan with its role, last-report time and the re-issue caveat, one per line", () => {
    assert.equal(
      buildOrphanSummary([
        orphan({ agentId: "a1", spawnRole: "fork", state: "running", lastReportAt: "2026-09-05T10:00:00.000Z" }),
        orphan({ agentId: "a2", state: "running" }),
      ]),
      [
        `a1 (fork, running, last report 2026-09-05T10:00:00.000Z) — ${MID_TURN_ORPHAN_CAVEAT}`,
        `a2 (worker, running, no reports) — ${MID_TURN_ORPHAN_CAVEAT}`,
      ].join("\n"),
    );
  });

  test("Edition: idle entries collapse into one closing line — they lost nothing and need no instruction re-issued", () => {
    const summary = buildOrphanSummary([
      orphan({ agentId: "a1", state: "running" }),
      orphan({ agentId: "a2", state: "idle" }),
      orphan({ agentId: "a3" }),
    ]);
    assert.equal(summary.split("\n").length, 2);
    assert.match(summary, /^a1 \(worker, running, no reports\) — /);
    assert.equal(summary.split("\n")[1], "2 idle agents re-registered dormant: a2, a3", "an orphan with no state field reads as idle");
  });

  test("Edition: an all-running set has no idle line at all", () => {
    const summary = buildOrphanSummary([orphan({ agentId: "a1", state: "running" })]);
    assert.equal(summary.split("\n").length, 1);
    assert.ok(!summary.includes("re-registered dormant"));
  });

  test("Edition: the idle line agrees in number", () => {
    const summary = buildOrphanSummary([orphan({ agentId: "a1", state: "running" }), orphan({ agentId: "a2", state: "idle" })]);
    assert.match(summary, /1 idle agent re-registered dormant: a2$/);
  });
});

/**
 * Edition (live-run fix): a `/reload` after three workers had all finished
 * announced all three, and the lead had nothing to do with any of them. The
 * push decision is pure and lives here rather than at the `session_start` call
 * site precisely so it has coverage — the glue around it is live-gate only.
 */
describe("buildOrphanPush (announce only what was cut off)", () => {
  function orphan(overrides: Partial<PersistedOrphan> = {}): PersistedOrphan {
    return { agentId: "a1", sessionPath: "s", systemPromptPath: "p", wsToolNames: [], toolGroup: "full-worker", ...overrides };
  }

  test("an all-idle set is worth NO message — every entry is still re-registered, and ws-agent-list is the trace", () => {
    assert.equal(buildOrphanPush([orphan({ agentId: "a1", state: "idle" }), orphan({ agentId: "a2" })]), undefined);
  });

  test("an empty set pushes nothing either", () => {
    assert.equal(buildOrphanPush([]), undefined);
  });

  test("a mixed set announces the running entries, counts only those, and names the idle ones once", () => {
    const push = buildOrphanPush([
      orphan({ agentId: "a1", state: "running" }),
      orphan({ agentId: "a2", state: "idle" }),
      orphan({ agentId: "a3", state: "idle" }),
    ])!;
    assert.equal(push.count, 1, "count is what the lead must act on, not how many were re-registered");
    assert.deepEqual(push.idle_agent_ids, ["a2", "a3"]);
    const agents = String(push.agents).split("\n");
    assert.equal(agents.length, 2);
    assert.ok(agents[0].startsWith("a1 ("));
    assert.ok(agents[0].includes(MID_TURN_ORPHAN_CAVEAT));
    assert.equal(agents[1], "2 idle agents re-registered dormant: a2, a3");
  });

  test("an all-running set carries no idle_agent_ids field to render", () => {
    const push = buildOrphanPush([orphan({ agentId: "a1", state: "running" }), orphan({ agentId: "a2", state: "running" })])!;
    assert.equal(push.count, 2);
    assert.equal("idle_agent_ids" in push, false);
    assert.equal(String(push.agents).split("\n").length, 2);
  });
});

describe("partitionOrphansByState", () => {
  function orphan(overrides: Partial<PersistedOrphan> = {}): PersistedOrphan {
    return { agentId: "a1", sessionPath: "s", systemPromptPath: "p", wsToolNames: [], toolGroup: "full-worker", ...overrides };
  }

  test("a missing state field counts as idle, the conservative default", () => {
    const { running, idle } = partitionOrphansByState([
      orphan({ agentId: "a1", state: "running" }),
      orphan({ agentId: "a2", state: "idle" }),
      orphan({ agentId: "a3" }),
    ]);
    assert.deepEqual(running.map((o) => o.agentId), ["a1"]);
    assert.deepEqual(idle.map((o) => o.agentId), ["a2", "a3"]);
  });
});

describe("writeSidecar / readAndClearSidecar (filesystem)", () => {
  test("a written sidecar round-trips and is DELETED by the read — one revival per crash, never a growing backlog", () => {
    withTempDir((dir) => {
      const sessionFile = join(dir, "session.jsonl");
      const orphans = captureOrphans(new Map([["a1", record({ client: {} as RpcClient, spawnRole: "worker" })]]));

      writeSidecar(sessionFile, orphans);
      assert.equal(existsSync(sidecarPath(sessionFile)), true);

      assert.deepEqual(readAndClearSidecar(sessionFile), orphans);
      assert.equal(existsSync(sidecarPath(sessionFile)), false, "leaving the file behind would re-announce the same stale agents every start");
      assert.deepEqual(readAndClearSidecar(sessionFile), [], "a second read finds nothing");
    });
  });

  test("an empty orphan set writes no file at all", () => {
    withTempDir((dir) => {
      const sessionFile = join(dir, "session.jsonl");
      writeSidecar(sessionFile, []);
      assert.equal(existsSync(sidecarPath(sessionFile)), false);
    });
  });

  test("a missing sidecar reads as no orphans", () => {
    withTempDir((dir) => {
      assert.deepEqual(readAndClearSidecar(join(dir, "never-written.jsonl")), []);
    });
  });

  test("a corrupt sidecar yields no orphans AND is still deleted, so it cannot wedge every future start", () => {
    withTempDir((dir) => {
      const sessionFile = join(dir, "session.jsonl");
      writeFileSync(sidecarPath(sessionFile), "{ this is not json");
      assert.deepEqual(readAndClearSidecar(sessionFile), []);
      assert.equal(existsSync(sidecarPath(sessionFile)), false);
    });
  });

  test("writeSidecar to an unwritable path is swallowed — a failed snapshot must not break session shutdown", () => {
    assert.doesNotThrow(() =>
      writeSidecar("/nonexistent-dir-ws-pi/session.jsonl", [
        { agentId: "a1", sessionPath: "s", systemPromptPath: "p", wsToolNames: [], toolGroup: "full-worker" },
      ]),
    );
  });
});

/**
 * Review relay #1, I1: the sidecar's whole point is that a revived child keeps
 * behaving like what it was. `spawnRole` was persisted and parsed but nothing
 * READ it on revival, so a revived fork came back as a plain record — no
 * question routing (a §1 violation the moment it asks something), no
 * anti-bleed loop. These tests exercise the full round trip: capture ->
 * serialize -> parse -> revive with the real role wiring -> the revived fork's
 * question routes to the owner surface instead of being pushed at the lead.
 */
describe("reviveOrphans (role wiring re-armed on revival)", () => {
  const pi = { sendMessage() {} } as unknown as ExtensionAPI;

  test("a full sidecar round trip revives a fork whose question still routes to the owner surface", () => {
    const source: RpcAgentRegistry = new Map([
      ["fork-1", record({ agentId: "fork-1", spawnRole: "fork", client: {} as RpcClient })],
      ["worker-1", record({ agentId: "worker-1", spawnRole: "worker", client: {} as RpcClient })],
    ]);
    const parsed = parseOrphans(serializeOrphans(captureOrphans(source)));

    const revivedRegistry: RpcAgentRegistry = new Map();
    const asked: Array<{ agentId: string; message: string }> = [];
    reviveOrphans(revivedRegistry, parsed, {
      fork: (rec) =>
        armForkRoleWiring(pi, revivedRegistry, rec, (agentId, message) => {
          asked.push({ agentId, message });
          return "[ws] thread q1 — the owner answers this.";
        }),
    });

    assert.deepEqual([...revivedRegistry.keys()].sort(), ["fork-1", "worker-1"]);
    const fork = revivedRegistry.get("fork-1")!;
    assert.equal(fork.client, undefined, "revived dormant — ws-agent-send relaunches it from its own session file");

    const outcome = applyRpcEvent(fork, {
      type: "tool_execution_start",
      toolName: REPORT_TO_LEAD_TOOL_NAME,
      args: { kind: "question", message: "which anchor?" },
    });
    assert.deepEqual(asked, [{ agentId: "fork-1", message: "which anchor?" }]);
    assert.deepEqual(
      outcome,
      { push: { family: "ws-agent-advisory", payload: { advisory: "fork-question-thread", detail: "[ws] thread q1 — the owner answers this." }, deliverAs: "followUp" } },
      "§1: routed to the owner surface, and the lead sees the registration notice, not a ws-agent-question",
    );

    const worker = revivedRegistry.get("worker-1")!;
    assert.equal(worker.onQuestionReport, undefined, "a plain worker has no role wiring to re-arm");
  });

  test("a revived fork's anti-bleed loop is restored on its first resume", () => {
    const parsed = parseOrphans(serializeOrphans([{ ...(captureOrphans(new Map([["f", record({ agentId: "f", spawnRole: "fork", client: {} as RpcClient })]]))[0]) }]));
    const registry: RpcAgentRegistry = new Map();
    reviveOrphans(registry, parsed, { fork: (rec) => armForkRoleWiring(pi, registry, rec) });

    const fork = registry.get("f")!;
    let subscriptions = 0;
    fork.client = {
      onEvent() {
        subscriptions += 1;
        return () => {};
      },
    } as unknown as RpcClient;
    fork.onResume?.(fork);
    assert.equal(subscriptions, 1, "sendToAgent's dormant-resume branch fires this once the client exists");
  });

  test("an execute-worker's approval relay is re-armed on the record itself, not left to the resume call site", () => {
    const parsed = parseOrphans(
      serializeOrphans(captureOrphans(new Map([["ex", record({ agentId: "ex", spawnRole: "execute-worker", client: {} as RpcClient })]]))),
    );
    const registry: RpcAgentRegistry = new Map();
    const relayed: string[] = [];
    reviveOrphans(registry, parsed, {
      executeWorker: (rec) => {
        rec.onApprovalPending = (r) => relayed.push(r.agentId);
      },
      fork: () => assert.fail("an execute-worker must not get the fork wiring"),
    });
    registry.get("ex")!.onApprovalPending?.(registry.get("ex")!);
    assert.deepEqual(relayed, ["ex"]);
  });

  test("an id already live on the registry is left untouched — a live child beats a stale sidecar entry", () => {
    const live = record({ agentId: "a1", client: {} as RpcClient, spawnRole: "fork" });
    const registry: RpcAgentRegistry = new Map([["a1", live]]);
    const revived = reviveOrphans(registry, [{ agentId: "a1", sessionPath: "/x", systemPromptPath: "/y", wsToolNames: [], toolGroup: "full-worker" }], {
      fork: () => assert.fail("nothing should be re-armed for a record that was never replaced"),
    });
    assert.deepEqual(revived, []);
    assert.equal(registry.get("a1"), live);
  });

  test("a throwing wiring callback still leaves the record registered and does not stop the rest", () => {
    const registry: RpcAgentRegistry = new Map();
    const orphans: PersistedOrphan[] = [
      { agentId: "f1", sessionPath: "/x", systemPromptPath: "/y", wsToolNames: [], toolGroup: "full-worker", spawnRole: "fork" },
      { agentId: "w1", sessionPath: "/x", systemPromptPath: "/y", wsToolNames: [], toolGroup: "full-worker", spawnRole: "worker" },
    ];
    const revived = reviveOrphans(registry, orphans, {
      fork: () => {
        throw new Error("wiring blew up");
      },
    });
    assert.equal(revived.length, 2);
    assert.deepEqual([...registry.keys()].sort(), ["f1", "w1"]);
  });
});
