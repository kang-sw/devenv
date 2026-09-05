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
  buildOrphanSummary,
  captureOrphans,
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
import { applyRpcEvent, REPORT_TO_LEAD_TOOL_NAME, type RpcAgentRecord, type RpcAgentRegistry } from "../src/spawner.ts";
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
  test("captures only records with a LIVE client", () => {
    const registry: RpcAgentRegistry = new Map([
      ["live", record({ agentId: "live", client: {} as RpcClient })],
      ["dormant", record({ agentId: "dormant" })],
    ]);
    assert.deepEqual(
      captureOrphans(registry).map((o) => o.agentId),
      ["live"],
      "a dormant record was already stopped deliberately — it is not an orphan",
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

  test("carries exactly the resume fields plus spawnRole, and nothing runtime-only", () => {
    const registry: RpcAgentRegistry = new Map([
      [
        "a1",
        record({
          client: {} as RpcClient,
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
        sessionPath: "/tmp/ws-pi-agent-x/session.jsonl",
        systemPromptPath: "/tmp/ws-pi-agent-x/prompt.md",
        modelBase: "prov/model",
        modelEffort: "high",
        wsToolNames: ["ws__todo_list"],
        toolGroup: "full-worker",
        explicitTools: "bash,ws-report-to-lead",
        spawnRole: "fork",
      },
    ]);
  });

  test("copies the tool-name list rather than aliasing the live record's array", () => {
    const live = record({ client: {} as RpcClient });
    const [orphan] = captureOrphans(new Map([["a1", live]]));
    assert.notEqual(orphan.wsToolNames, live.wsToolNames);
    assert.deepEqual(orphan.wsToolNames, [...live.wsToolNames]);
  });
});

describe("serializeOrphans / parseOrphans", () => {
  const orphans: PersistedOrphan[] = [
    {
      agentId: "a1",
      sessionPath: "/tmp/s1.jsonl",
      systemPromptPath: "/tmp/p1.md",
      modelBase: "prov/model",
      modelEffort: "high",
      wsToolNames: ["ws__todo_list"],
      toolGroup: "full-worker",
      explicitTools: "bash",
      spawnRole: "fork",
    },
  ];

  test("round-trips a full orphan set unchanged", () => {
    assert.deepEqual(parseOrphans(serializeOrphans(orphans)), orphans);
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
});

describe("buildOrphanSummary", () => {
  test("names every orphan and its role in one line", () => {
    assert.equal(
      buildOrphanSummary([
        { agentId: "a1", sessionPath: "s", systemPromptPath: "p", wsToolNames: [], toolGroup: "full-worker", spawnRole: "fork" },
        { agentId: "a2", sessionPath: "s", systemPromptPath: "p", wsToolNames: [], toolGroup: "full-worker" },
      ]),
      "a1 (fork), a2 (worker)",
    );
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
    assert.deepEqual(outcome, {}, "§1: routed to the owner surface, never pushed at the lead as ws-agent-question");

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
