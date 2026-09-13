import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { describe, test } from "node:test";
import { allocateAgentHome, createAgentStorageContext, createOwnershipDiagnosticReporter, observeSessionWrite, pruneStaleAgentHomes, readOwnership, setOwnershipDiagnosticReporter, touchOwnership, writeOwnership } from "../src/agent-storage.ts";
import { createThreadRegistryHandle, ensureRespondent, handleForkRaisedQuestion } from "../src/ask.ts";
import { openViewer } from "../src/audit.ts";
import { applyRpcEvent, getAgentTranscriptPath, REPORT_TO_LEAD_TOOL_NAME, syncOwnershipProtection, type RpcAgentRecord, type RpcAgentRegistry } from "../src/spawner.ts";

// A separate process pauses INSIDE the production writer/deleter's claim.
// A fixture-only release file is a barrier, not a timing-dependent overlap guess.
async function holdClaim(home: string, kind: "writer" | "deleter") {
  const releasePath = join(dirname(dirname(dirname(home))), `release-${kind}`);
  const child = spawn(process.execPath, ["--input-type=module", "--eval", `
    import { existsSync, writeSync } from "node:fs";
    import { readOwnership, writeOwnership, removeOwnedAgentHome } from ${JSON.stringify(new URL("../src/agent-storage.ts", import.meta.url).href)};
    const metadata = readOwnership(${JSON.stringify(home)});
    function barrier() {
      writeSync(1, "claimed\\n");
      const deadline = Date.now() + 10_000;
      const sleeper = new Int32Array(new SharedArrayBuffer(4));
      while (!existsSync(${JSON.stringify(releasePath)})) {
        if (Date.now() > deadline) throw new Error("fixture release timed out");
        Atomics.wait(sleeper, 0, 0, 10);
      }
    }
    if (${JSON.stringify(kind)} === "writer") {
      writeOwnership({ ...metadata, get updatedAt() { barrier(); return metadata.updatedAt; } });
    } else {
      const result = removeOwnedAgentHome(metadata, undefined, () => { barrier(); return true; });
      if (result.status !== "deleted") throw new Error(JSON.stringify(result));
    }
  `], { stdio: ["ignore", "pipe", "pipe"] });
  let stderr = "";
  child.stderr.on("data", chunk => { stderr += chunk; });
  const exited = once(child, "exit");
  try {
    const signal = await Promise.race([
      once(child.stdout, "data").then(([chunk]) => String(chunk)),
      exited.then(([code]) => { throw new Error(`claim holder exited ${code}: ${stderr}`); }),
    ]);
    assert.equal(signal, "claimed\n");
  } catch (error) { child.kill(); await exited; throw error; }
  return async () => {
    writeFileSync(releasePath, "release");
    const [code] = await exited;
    assert.equal(code, 0, stderr);
  };
}

function fixture() {
  const root = mkdtempSync(join(tmpdir(), "ws-pi-contention-"));
  const ownership = allocateAgentHome(createAgentStorageContext("other-lead", root), "fork-child", "fork");
  writeFileSync(ownership.sessionPath!, '{"type":"message","message":{"role":"user","content":"history"}}\n');
  observeSessionWrite(ownership.home, ownership.sessionPath!);
  writeOwnership({ ...readOwnership(ownership.home)!, lastActivityAt: 1, liveness: { lifecycle: "stopped", running: false } });
  const record: RpcAgentRecord = {
    agentId: ownership.agentId, ownership, sessionPath: ownership.sessionPath!, spawnRole: "fork",
    toolGroup: "full-worker", wsToolNames: [], systemPromptPath: "/unused-prompt.md",
    running: false, streaming: false, reportLog: [],
  };
  const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
  const handle = createThreadRegistryHandle();
  handle.pathRef.current = join(root, "threads.json");
  return { root, ownership, record, registry, handle };
}

describe("durable ownership contention at accepted operation boundaries", () => {
  test("observer contention stays silent while authoritative failures coalesce outside terminal output", { timeout: 15_000 }, async t => {
    const { root, ownership } = fixture();
    const diagnostics = t.mock.method(console, "error", () => {});
    const notices: string[] = [];
    setOwnershipDiagnosticReporter(createOwnershipDiagnosticReporter(message => notices.push(message)));
    let release: (() => Promise<void>) | undefined;
    try {
      const before = readOwnership(ownership.home)!;
      writeFileSync(ownership.sessionPath!, "new history after the first observation\n");
      release = await holdClaim(ownership.home, "writer");

      observeSessionWrite(ownership.home, ownership.sessionPath!);
      assert.equal(diagnostics.mock.callCount(), 0, "a best-effort observer does not write an expected live-holder collision");
      assert.deepEqual(notices, []);
      assert.deepEqual(readOwnership(ownership.home), before, "the skipped sample does not accept an ownership update");

      assert.equal(touchOwnership(ownership.home), false, "authoritative writes still fail closed under the same live claim");
      assert.equal(diagnostics.mock.callCount(), 0, "authoritative contention stays outside console");
      assert.deepEqual(notices, ["ws: could not persist owned-agent metadata; the affected operation was rejected."]);

      await release(); release = undefined;
      observeSessionWrite(ownership.home, ownership.sessionPath!);
      const observed = readOwnership(ownership.home)!;
      assert.equal(observed.sessionSignature?.size, 40, "the next observer sample records the session change");
      assert.ok(observed.lastActivityAt > before.lastActivityAt);

      const lock = join(dirname(ownership.home), `.${ownership.agentId}.ownership-lock`);
      mkdirSync(lock);
      writeFileSync(join(lock, "owner.json"), "{}");
      const beforeMalformed = readOwnership(ownership.home)!;
      observeSessionWrite(ownership.home, ownership.sessionPath!);
      assert.equal(diagnostics.mock.callCount(), 0, "malformed observer lock facts stay outside console");
      assert.equal(notices.length, 1, "observer-only failures never add a normal notification");
      assert.deepEqual(readOwnership(ownership.home), beforeMalformed);
    } finally {
      if (release) await release();
      setOwnershipDiagnosticReporter();
      rmSync(root, { recursive: true, force: true });
    }
  });

  for (const kind of ["writer", "deleter"] as const) {
    test(`${kind} overlap refuses owner binds and references rather than accepting unpersisted protection/activity`, { timeout: 15_000 }, async t => {
      const { root, ownership, record, registry, handle } = fixture();
      const diagnostics = t.mock.method(console, "error", () => {});
      const diagnosticNotices: string[] = [];
      setOwnershipDiagnosticReporter(createOwnershipDiagnosticReporter(message => diagnosticNotices.push(message)));
      let release: (() => Promise<void>) | undefined;
      try {
        release = await holdClaim(ownership.home, kind);
        assert.equal(syncOwnershipProtection({ ...record, lastWriter: "owner" }), false);
        assert.throws(() => handleForkRaisedQuestion(handle, registry, record.agentId, "Owner decision?"), /cannot bind owner thread.*busy/);
        assert.equal(record.threadBound, undefined, "local bind must not precede durable protection");
        assert.equal(handle.threads.size, 0, "no accepted pending thread on a failed bind");
        assert.equal(existsSync(handle.pathRef.current!), false, "no persisted phantom thread");

        const thread = { threadId: "q1", title: "reopen", question: "reopen?", status: "dormant", origin: "fork-raised", respondentAgentId: record.agentId, createdAt: "now", touchedAt: "now" } as const;
        await assert.rejects(ensureRespondent({} as never, {} as never, {} as never, registry, handle, { ...thread }, { cwd: root, extensionPath: "/unused" }), /cannot bind owner thread/);
        assert.equal(record.onFinalReport, undefined, "a failed reopen does not install an owner handler");

        const notices: string[] = [];
        await openViewer({ mode: "tui", ui: {
          notify: (message: string) => notices.push(message),
          custom: () => assert.fail("audit cannot open after an uncommitted reference"),
        } } as never, registry, record.agentId);
        assert.match(notices[0]!, /history unavailable.*busy.*retry/);
        assert.throws(() => getAgentTranscriptPath(registry, record.agentId), /history unavailable.*retry/);
        assert.equal(readOwnership(ownership.home)!.lastActivityAt, 1);
        assert.equal(readOwnership(ownership.home)!.liveness.threadBound, undefined);
        assert.equal(diagnostics.mock.callCount(), 0, "failed metadata writes never use console");
        assert.equal(diagnosticNotices.length, 1, "repeated authoritative metadata failures coalesce per session");

        await release(); release = undefined;
        if (kind === "writer") {
          const accepted = handleForkRaisedQuestion(handle, registry, record.agentId, "Retry owner decision?");
          assert.equal(accepted.status, "pending");
          assert.equal(record.threadBound, true);
          assert.equal(readOwnership(ownership.home)!.liveness.threadBound, true);
          assert.deepEqual(pruneStaleAgentHomes(root, 30).deletedHomes, []);
          assert.equal(existsSync(ownership.home), true, "another lead must retain the accepted owner thread");
        } else {
          assert.equal(existsSync(ownership.home), false, "the deleter won before any operation was accepted");
          assert.throws(() => handleForkRaisedQuestion(handle, registry, record.agentId, "Gone?"), /cannot bind owner thread/);
          assert.equal(handle.threads.size, 0);
        }
      } finally {
        if (release) await release();
        setOwnershipDiagnosticReporter();
        rmSync(root, { recursive: true, force: true });
      }
    });
  }

  test("a failed owner bind relays the question to the lead instead of dropping or claiming an owner thread", { timeout: 15_000 }, async t => {
    const { root, ownership, record, registry, handle } = fixture();
    t.mock.method(console, "error", () => {});
    let release: (() => Promise<void>) | undefined;
    try {
      release = await holdClaim(ownership.home, "writer");
      record.onQuestionReport = (rec, message) => {
        handleForkRaisedQuestion(handle, registry, rec.agentId, message);
        return "registered owner thread";
      };
      const outcome = applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { kind: "question", message: "Owner decision?" } } as never);
      assert.equal(outcome.push?.family, "ws-agent-question");
      assert.equal(outcome.push?.payload.question, "Owner decision?");
      assert.equal(record.threadBound, undefined);
      assert.equal(handle.threads.size, 0);
    } finally {
      if (release) await release();
      rmSync(root, { recursive: true, force: true });
    }
  });

  for (const reference of ["audit", "transcript"] as const) {
    test(`an accepted ${reference} reference refreshes durable activity and defeats a stale scan`, async () => {
      const { root, ownership, record, registry } = fixture();
      try {
        if (reference === "audit") {
          let opened = false;
          await openViewer({ mode: "tui", ui: { custom: async () => { opened = true; } } } as never, registry, record.agentId);
          assert.equal(opened, true);
        } else {
          assert.equal(getAgentTranscriptPath(registry, record.agentId).transcript_path, ownership.sessionPath);
        }
        assert.ok(readOwnership(ownership.home)!.lastActivityAt > 1);
        assert.deepEqual(pruneStaleAgentHomes(root, 30).deletedHomes, []);
        assert.equal(existsSync(ownership.home), true);
      } finally { rmSync(root, { recursive: true, force: true }); }
    });
  }

  test("missing or unreadable metadata refuses a new bind and coalesces its owner signal", t => {
    const { root, ownership, record, registry, handle } = fixture();
    const diagnostics = t.mock.method(console, "error", () => {});
    const notices: string[] = [];
    setOwnershipDiagnosticReporter(createOwnershipDiagnosticReporter(message => notices.push(message)));
    try {
      const metadataPath = join(ownership.home, "ownership.json");
      writeFileSync(metadataPath, "not-json");
      assert.equal(touchOwnership(ownership.home), false, "unreadable metadata rejects the write");
      rmSync(metadataPath);
      assert.equal(touchOwnership(ownership.home), false, "missing metadata rejects the write");
      assert.throws(() => handleForkRaisedQuestion(handle, registry, record.agentId, "Unknown?"), /cannot bind owner thread/);
      assert.equal(record.threadBound, undefined);
      assert.equal(handle.threads.size, 0);
      assert.deepEqual(pruneStaleAgentHomes(root, 30).deletedHomes, []);
      assert.equal(diagnostics.mock.callCount(), 0);
      assert.deepEqual(notices, ["ws: could not persist owned-agent metadata; the affected operation was rejected."]);
    } finally { setOwnershipDiagnosticReporter(); rmSync(root, { recursive: true, force: true }); }
  });
});
