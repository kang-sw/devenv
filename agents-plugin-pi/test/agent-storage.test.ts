import assert from "node:assert/strict";
import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, realpathSync, renameSync, rmSync, statSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { describe, test } from "node:test";
import { allocateAgentHome, createAgentStorageContext, inspectOwnedHomeRemoval, isOwnedSessionPath, observeSessionWrite, ownershipPath, pruneStaleAgentHomes, readOwnerArtifacts, readOwnership, removeOwnedAgentHome, reportOwnershipDiagnostic, touchOwnership, writeOwnerArtifact, writeOwnership, updateOwnership } from "../src/agent-storage.ts";
import { ownerNotifyRef, validateForkReadiness } from "../src/spawner.ts";
import { symlinkSkip } from "./fixtures/symlink-probe.ts";

describe("agent storage", () => {
  test("ownership diagnostics are bounded per reporter session and never expose raw errors", () => {
    const notices: string[] = [];
    try {
      ownerNotifyRef.current = message => notices.push(message);
      reportOwnershipDiagnostic("observer-session-write", new Error("/private/observer-path"));
      reportOwnershipDiagnostic("metadata-update", new Error("/private/first-path"));
      reportOwnershipDiagnostic("metadata-update", new Error("/private/second-path"));
      assert.deepEqual(notices, ["ws: could not persist owned-agent metadata; the affected operation was rejected."]);
      assert.doesNotMatch(notices[0]!, /private|first|second|Error/);

      ownerNotifyRef.current = undefined;
      ownerNotifyRef.current = message => notices.push(message);
      reportOwnershipDiagnostic("metadata-update", new Error("new adapter session"));
      assert.equal(notices.length, 2, "a replacement owner notification session gets a fresh bounded ledger");
    } finally { ownerNotifyRef.current = undefined; }
  });

  test("ownership storage contains no direct terminal-stream diagnostics", () => {
    const source = readFileSync(new URL("../src/agent-storage.ts", import.meta.url), "utf8");
    assert.doesNotMatch(source, /console\.|process\.(?:stdout|stderr)/);
  });

  test("allocates a persistent child beneath the configured Pi root and persists ownership", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const context = createAgentStorageContext("lead_1", root);
      const owned = allocateAgentHome(context, "agent-1", "worker");
      assert.equal(owned.home, join(context.root, "ws-agents", "lead_1", "agent-1"));
      assert.equal(owned.sessionPath, join(owned.home, "session.jsonl"));
      const before = readOwnership(owned.home)!;
      touchOwnership(owned.home);
      assert.ok(readOwnership(owned.home)!.lastActivityAt >= before.lastActivityAt);
      writeFileSync(owned.sessionPath!, "session write\n");
      observeSessionWrite(owned.home, owned.sessionPath!);
      assert.equal(readOwnership(owned.home)!.sessionSignature?.size, 14);
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("accepts former public Explore modes in persisted ownership without rewriting their labels", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const context = createAgentStorageContext("lead", root);
      const modes = ["code-search", "history-search", "docs-search", "web-search", "diagnosis", "comparison", "synthesis"] as const;
      for (const [index, mode] of modes.entries()) {
        const owned = allocateAgentHome(context, `research-${index}`, "explore", "search");
        const metadata = readOwnership(owned.home)!;
        writeFileSync(ownershipPath(owned.home), `${JSON.stringify({ ...metadata, exploreMode: mode })}\n`);
        assert.equal(readOwnership(owned.home)?.exploreMode, mode);
        assert.equal(touchOwnership(owned.home), true);
        assert.equal(JSON.parse(readFileSync(ownershipPath(owned.home), "utf8")).exploreMode, mode);
      }
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("normalizes persistence-only Explore aliases at ownership read and never serializes them again", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const context = createAgentStorageContext("lead", root);
      for (const [index, [alias, expected]] of [["simple", "code-search"], ["deep", "synthesis"]].entries()) {
        const owned = allocateAgentHome(context, `research-alias-${index}`, "explore", "search");
        const metadata = readOwnership(owned.home)!;
        writeFileSync(ownershipPath(owned.home), `${JSON.stringify({ ...metadata, exploreMode: alias })}\n`);
        assert.equal(readOwnership(owned.home)?.exploreMode, expected);
        assert.equal(touchOwnership(owned.home), true);
        assert.equal(JSON.parse(readFileSync(ownershipPath(owned.home), "utf8")).exploreMode, expected);
      }
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("owner artifacts use the storage boundary and refuse a symlinked bucket", { skip: symlinkSkip() }, () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    const outside = mkdtempSync(join(tmpdir(), "ws-pi-storage-outside-"));
    try {
      const context = createAgentStorageContext("lead", root);
      assert.equal(writeOwnerArtifact(context, ".cost-rollup", "agent.json", "safe"), true, "the storage layer creates a fresh owner namespace safely");
      assert.deepEqual(readOwnerArtifacts(context, ".cost-rollup"), [{ name: "agent.json", content: "safe" }]);
      const bucket = join(context.root, "ws-agents", "lead", ".cost-rollup");
      rmSync(bucket, { recursive: true, force: true });
      symlinkSync(outside, bucket);
      assert.equal(writeOwnerArtifact(context, ".cost-rollup", "escape.json", "no"), false);
      assert.equal(existsSync(join(outside, "escape.json")), false);
      assert.deepEqual(readOwnerArtifacts(context, ".cost-rollup"), []);
    } finally { rmSync(root, { recursive: true, force: true }); rmSync(outside, { recursive: true, force: true }); }
  });

  test("rejects traversal in the owner identity", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      assert.throws(() => createAgentStorageContext("../escape", root), /unsafe Pi session id/);
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("rejects a pre-existing owned session symlink and metadata whose encoded home differs from its locator", { skip: symlinkSkip() }, () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const context = createAgentStorageContext("lead-1", root);
      const home = join(context.root, "ws-agents", "lead-1", "agent-3");
      mkdirSync(home, { recursive: true });
      const outside = join(root, "outside.jsonl");
      writeFileSync(outside, "outside\n");
      symlinkSync(outside, join(home, "session.jsonl"));
      assert.throws(() => allocateAgentHome(context, "agent-3", "worker"), /symlink|owned path|escapes owned/i);

      const owned = allocateAgentHome(context, "agent-4", "worker");
      const metadata = readOwnership(owned.home)!;
      writeFileSync(join(owned.home, "ownership.json"), JSON.stringify({ ...metadata, home: join(root, "elsewhere") }));
      assert.equal(readOwnership(owned.home), undefined, "metadata can never redirect a locator's future writes");
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("validates the complete session candidate and regular-file type at every ownership boundary", { skip: symlinkSkip() }, () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const owned = allocateAgentHome(createAgentStorageContext("lead-1", root), "agent-5", "worker");
      const metadata = readOwnership(owned.home)!;
      const nested = join(owned.home, "nested");
      mkdirSync(nested);
      const sibling = `${owned.home}-sibling`;
      mkdirSync(sibling);
      writeFileSync(join(sibling, "session.jsonl"), "outside");
      symlinkSync(sibling, join(nested, "outside"));
      symlinkSync(nested, join(owned.home, "alias"));
      writeFileSync(owned.sessionPath!, "session");
      symlinkSync(owned.sessionPath!, join(nested, "linked.jsonl"));
      const invalid = [owned.home, `${owned.home}/.`, `${owned.home}/..`, sibling,
        join(sibling, "session.jsonl"), nested, join(nested, "linked.jsonl"),
        join(nested, "outside", "session.jsonl"), join(nested, "outside", "missing.jsonl"),
        join(owned.home, "alias", "missing.jsonl"), join(nested, "absent", "session.jsonl")];
      for (const candidate of invalid) {
        assert.equal(isOwnedSessionPath(owned.home, candidate), false, candidate);
        assert.throws(() => writeOwnership({ ...metadata, sessionPath: candidate }), undefined, candidate);
        writeFileSync(join(owned.home, "ownership.json"), JSON.stringify({ ...metadata, sessionPath: candidate }));
        assert.equal(readOwnership(owned.home), undefined, candidate);
      }
      for (const candidate of [owned.sessionPath!, join(nested, "new.jsonl")]) {
        assert.equal(isOwnedSessionPath(owned.home, candidate), true, candidate);
        writeOwnership({ ...metadata, sessionPath: candidate });
        assert.equal(readOwnership(owned.home)?.sessionPath, candidate);
      }
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("fork readiness rejects a terminal traversal before changing memory or disk", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const ownership = allocateAgentHome(createAgentStorageContext("lead-1", root), "agent-7", "fork");
      const record = { ownership, sessionPath: ownership.sessionPath } as never;
      const before = readOwnership(ownership.home);
      const candidate = `${ownership.home}/..`;
      const ready = { ownSessionKey: "child-key", sessionId: "child-id", sessionPath: candidate, activeTools: [], registeredTools: [] };
      assert.throws(() => validateForkReadiness(ready, record, { sessionId: "child-id", sessionFile: candidate }), /escaped owned home/);
      assert.equal(record.sessionPath, ownership.sessionPath);
      assert.deepEqual(readOwnership(ownership.home), before);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  test("missing first write is pending; disappearing observed history remains diagnostic and conservative", (t) => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    const diagnostics = t.mock.method(console, "error", () => {});
    try {
      const owned = allocateAgentHome(createAgentStorageContext("lead-1", root), "agent-6", "worker");
      const before = readOwnership(owned.home)!;
      observeSessionWrite(owned.home, owned.sessionPath!);
      assert.deepEqual(readOwnership(owned.home), before);
      assert.equal(diagnostics.mock.callCount(), 0);
      writeFileSync(owned.sessionPath!, "first write");
      observeSessionWrite(owned.home, owned.sessionPath!);
      updateOwnership(owned.home, { liveness: { lifecycle: "live", running: true } });
      const observed = readOwnership(owned.home)!;
      assert.equal(observed.sessionSignature?.size, 11);
      rmSync(owned.sessionPath!);
      observeSessionWrite(owned.home, owned.sessionPath!);
      const missing = readOwnership(owned.home)!;
      assert.equal(diagnostics.mock.callCount(), 0, "observer failures never write through console");
      assert.equal(missing.liveness.lifecycle, "unknown");
      assert.equal(missing.liveness.running, true, "missing history is not proof of process death");
      assert.equal(missing.lastActivityAt, observed.lastActivityAt);
      assert.deepEqual(missing.sessionSignature, observed.sessionSignature);
      observeSessionWrite(owned.home, join(owned.home, "invalid-parent", "session.jsonl"));
      assert.equal(diagnostics.mock.callCount(), 0, "ordinary observer IO failures stay outside the terminal stream");
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("a real stat failure before the first write is still diagnostic", (t) => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    const diagnostics = t.mock.method(console, "error", () => {});
    try {
      const owned = allocateAgentHome(createAgentStorageContext("lead-1", root), "agent-8", "worker");
      const before = readOwnership(owned.home)!;
      observeSessionWrite(owned.home, join(owned.home, "ownership.json", "invalid"));
      assert.equal(diagnostics.mock.callCount(), 0, "observer IO failures never write through console");
      const after = readOwnership(owned.home)!;
      assert.equal(after.liveness.lifecycle, "unknown");
      assert.equal(after.lastActivityAt, before.lastActivityAt);
      assert.equal(after.sessionSignature, undefined);
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("observer samples with nothing new to persist write nothing and take no lock", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const owned = allocateAgentHome(createAgentStorageContext("lead-1", root), "agent-quiet", "worker");
      // A dead-pid claim is reclaimed by any lock acquisition, so its survival
      // proves that no sample even attempted to take the lock.
      const lock = join(dirname(owned.home), `.${owned.agentId}.ownership-lock`);
      const plantStaleClaim = () => { mkdirSync(lock); writeFileSync(join(lock, "owner.json"), JSON.stringify({ pid: 2_147_483_647 })); };
      const file = () => ({ content: readFileSync(ownershipPath(owned.home), "utf8"), ino: statSync(ownershipPath(owned.home)).ino });

      plantStaleClaim();
      const beforeFirstWrite = file();
      observeSessionWrite(owned.home, owned.sessionPath!);
      assert.deepEqual(file(), beforeFirstWrite, "a pending first write is not a change");
      assert.equal(existsSync(join(lock, "owner.json")), true);
      rmSync(lock, { recursive: true });

      writeFileSync(owned.sessionPath!, "session write\n");
      observeSessionWrite(owned.home, owned.sessionPath!);
      const observed = file();
      assert.equal(readOwnership(owned.home)!.sessionSignature?.size, 14);

      plantStaleClaim();
      observeSessionWrite(owned.home, owned.sessionPath!);
      observeSessionWrite(owned.home, owned.sessionPath!);
      assert.deepEqual(file(), observed, "an unchanged signature is not rewritten, not even updatedAt");
      assert.equal(existsSync(join(lock, "owner.json")), true, "an unchanged sample never takes the lock");

      writeFileSync(owned.sessionPath!, "session write\nmore\n");
      observeSessionWrite(owned.home, owned.sessionPath!);
      assert.equal(existsSync(lock), false, "a changed sample takes (and reclaims) the lock");
      assert.equal(readOwnership(owned.home)!.sessionSignature?.size, 19, "a changed signature is still written");
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("an observer write that hit a busy lock is written at the next sample with no further session change", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const owned = allocateAgentHome(createAgentStorageContext("lead-1", root), "agent-busy", "worker");
      const lock = join(dirname(owned.home), `.${owned.agentId}.ownership-lock`);
      mkdirSync(lock);
      writeFileSync(join(lock, "owner.json"), JSON.stringify({ pid: process.pid }));
      writeFileSync(owned.sessionPath!, "first write");
      observeSessionWrite(owned.home, owned.sessionPath!);
      assert.equal(readOwnership(owned.home)!.sessionSignature, undefined, "the busy sample persisted nothing");
      rmSync(lock, { recursive: true });
      observeSessionWrite(owned.home, owned.sessionPath!);
      assert.equal(readOwnership(owned.home)!.sessionSignature?.size, 11);
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("an already-unknown observation failure is not rewritten on every sample", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const owned = allocateAgentHome(createAgentStorageContext("lead-1", root), "agent-unknown", "worker");
      const broken = join(owned.home, "ownership.json", "invalid");
      observeSessionWrite(owned.home, broken);
      assert.equal(readOwnership(owned.home)!.liveness.lifecycle, "unknown");
      const content = readFileSync(ownershipPath(owned.home), "utf8"), ino = statSync(ownershipPath(owned.home)).ino;
      observeSessionWrite(owned.home, broken);
      assert.equal(readFileSync(ownershipPath(owned.home), "utf8"), content);
      assert.equal(statSync(ownershipPath(owned.home)).ino, ino, "no replacement rename happened");
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("ownership rename retries Windows EPERM/EBUSY and fails as before once the budget is exhausted", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const owned = allocateAgentHome(createAgentStorageContext("lead-1", root), "agent-rename", "worker");
      const temps = () => readdirSync(owned.home).filter(name => name.endsWith(".tmp"));
      for (const code of ["EPERM", "EBUSY"]) {
        const metadata = { ...readOwnership(owned.home)!, lastActivityAt: code === "EPERM" ? 11 : 12 };
        const delays: number[] = [];
        let attempts = 0;
        writeOwnership(metadata, {
          platform: "win32",
          sleep: milliseconds => delays.push(milliseconds),
          rename: (source, destination) => {
            if (++attempts === 1) throw Object.assign(new Error(code), { code });
            renameSync(source, destination);
          },
        });
        assert.equal(attempts, 2);
        assert.deepEqual(delays, [10]);
        assert.equal(readOwnership(owned.home)!.lastActivityAt, metadata.lastActivityAt);
        assert.deepEqual(temps(), []);
      }

      const before = readFileSync(ownershipPath(owned.home), "utf8");
      const failures = Array.from({ length: 5 }, () => Object.assign(new Error("EPERM"), { code: "EPERM" }));
      const delays: number[] = [];
      let attempts = 0;
      assert.throws(() => writeOwnership({ ...readOwnership(owned.home)!, lastActivityAt: 99 }, {
        platform: "win32",
        sleep: milliseconds => delays.push(milliseconds),
        rename: () => { throw failures[attempts++]; },
      }), error => error === failures[4], "exhaustion rethrows the rename error itself");
      assert.equal(attempts, 5);
      assert.deepEqual(delays, [10, 20, 40, 80]);
      assert.equal(readFileSync(ownershipPath(owned.home), "utf8"), before);
      assert.deepEqual(temps(), []);
      assert.equal(existsSync(join(dirname(owned.home), `.${owned.agentId}.ownership-lock`)), false, "the lock is released on failure");
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("owner artifact rename retries Windows EPERM/EBUSY and reports failure once the budget is exhausted", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const context = createAgentStorageContext("lead-1", root);
      const bucket = join(context.root, "ws-agents", "lead-1", ".evicted");
      for (const code of ["EPERM", "EBUSY"]) {
        const delays: number[] = [];
        let attempts = 0;
        assert.equal(writeOwnerArtifact(context, ".evicted", "agent.json", code, {
          platform: "win32",
          sleep: milliseconds => delays.push(milliseconds),
          rename: (source, destination) => {
            if (++attempts === 1) throw Object.assign(new Error(code), { code });
            renameSync(source, destination);
          },
        }), true);
        assert.equal(attempts, 2);
        assert.deepEqual(delays, [10]);
        assert.deepEqual(readdirSync(bucket), ["agent.json"]);
        assert.equal(readFileSync(join(bucket, "agent.json"), "utf8"), code);
      }

      let attempts = 0;
      assert.equal(writeOwnerArtifact(context, ".evicted", "agent.json", "lost", {
        platform: "win32",
        sleep: () => {},
        rename: () => { attempts++; throw Object.assign(new Error("EBUSY"), { code: "EBUSY" }); },
      }), false);
      assert.equal(attempts, 5);
      assert.deepEqual(readdirSync(bucket), ["agent.json"], "no temporary file is left behind");
      assert.equal(readFileSync(join(bucket, "agent.json"), "utf8"), "EBUSY", "the prior artifact is kept");
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("new records carry no liveness pid while legacy records with one load unchanged", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const owned = allocateAgentHome(createAgentStorageContext("lead-1", root), "agent-pid", "worker");
      assert.equal(readOwnership(owned.home)!.liveness.pid, undefined);
      assert.equal("pid" in JSON.parse(readFileSync(ownershipPath(owned.home), "utf8")).liveness, false);

      const legacy = { ...readOwnership(owned.home)!, liveness: { ...readOwnership(owned.home)!.liveness, pid: 4242 } };
      writeFileSync(ownershipPath(owned.home), `${JSON.stringify(legacy, null, 2)}\n`);
      assert.deepEqual(readOwnership(owned.home), legacy);
      assert.equal(touchOwnership(owned.home), true);
      assert.equal(readOwnership(owned.home)!.liveness.pid, 4242, "an update keeps the legacy field");
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("removes only an exactly-owned, confirmed-stopped home and drops its empty lead directory", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const context = createAgentStorageContext("lead-1", root);
      const owned = allocateAgentHome(context, "agent-delete", "worker");
      writeFileSync(owned.sessionPath!, "history");
      const unrelated = join(context.root, "unrelated.txt");
      writeFileSync(unrelated, "keep");
      updateOwnership(owned.home, { liveness: { lifecycle: "stopped", running: false, observedAt: Date.now() } });
      assert.equal(inspectOwnedHomeRemoval(owned).status, "eligible");
      assert.deepEqual(removeOwnedAgentHome(owned), { status: "deleted" });
      assert.equal(readOwnership(owned.home), undefined);
      assert.equal(readFileSync(unrelated, "utf8"), "keep");
      assert.equal(existsSync(join(context.root, "ws-agents", "lead-1")), false, "empty lead subtree is gone");
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("retains unknown and protected children, mismatched descriptors, and homes containing symlinks", { skip: symlinkSkip() }, () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const context = createAgentStorageContext("lead-1", root);
      const unknown = allocateAgentHome(context, "agent-unknown", "worker");
      assert.equal(inspectOwnedHomeRemoval(unknown).status, "retained");

      const protections = [
        { threadBound: true }, { ownerHeld: true }, { pendingQuestion: true },
        { waitingOnChildren: true }, { pendingDelivery: true }, { pendingApprovalCommandId: "call-1" },
      ];
      for (const [index, protection] of protections.entries()) {
        const protectedChild = allocateAgentHome(context, `agent-protected-${index}`, "execute-worker");
        updateOwnership(protectedChild.home, { liveness: { lifecycle: "stopped", running: false, ...protection } });
        assert.match((inspectOwnedHomeRemoval(protectedChild) as { reason: string }).reason, /protected/);
        assert.equal(existsSync(protectedChild.home), true);
      }

      const stoppedFork = allocateAgentHome(context, "agent-fork", "fork");
      writeFileSync(stoppedFork.sessionPath!, "fork history");
      updateOwnership(stoppedFork.home, { liveness: { lifecycle: "stopped", running: false } });
      assert.deepEqual(removeOwnedAgentHome(stoppedFork), { status: "deleted" });

      const mismatched = allocateAgentHome(context, "agent-mismatch", "worker");
      updateOwnership(mismatched.home, { liveness: { lifecycle: "stopped", running: false } });
      assert.equal(inspectOwnedHomeRemoval({ ...mismatched, role: "fork" }).status, "retained");

      const symlinked = allocateAgentHome(context, "agent-symlink", "worker");
      updateOwnership(symlinked.home, { liveness: { lifecycle: "stopped", running: false } });
      const outside = join(context.root, "outside.txt");
      writeFileSync(outside, "outside");
      symlinkSync(outside, join(symlinked.home, "link"));
      assert.match((inspectOwnedHomeRemoval(symlinked) as { reason: string }).reason, /symlink/);
      assert.equal(readFileSync(outside, "utf8"), "outside");
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("refuses removal when the home, lead subtree, or namespace is redirected through a symlink", { skip: symlinkSkip() }, () => {
    for (const level of ["home", "owner", "namespace"] as const) {
      const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
      try {
        const context = createAgentStorageContext("lead-1", root);
        const owned = allocateAgentHome(context, "agent-link", "worker");
        updateOwnership(owned.home, { liveness: { lifecycle: "stopped", running: false } });
        const namespace = join(context.root, "ws-agents");
        const ownerRoot = join(namespace, "lead-1");
        const target = level === "home" ? owned.home : level === "owner" ? ownerRoot : namespace;
        const external = join(context.root, `external-${level}`);
        renameSync(target, external);
        symlinkSync(external, target);
        assert.equal(inspectOwnedHomeRemoval(owned).status, "retained", level);
        assert.equal(existsSync(join(external, ...(level === "namespace" ? ["lead-1", "agent-link", "ownership.json"] : level === "owner" ? ["agent-link", "ownership.json"] : ["ownership.json"]))), true, level);
      } finally { rmSync(root, { recursive: true, force: true }); }
    }
  });

  test("a replacement symlink racing deletion is never traversed outside the owned home", { skip: symlinkSkip() }, () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const context = createAgentStorageContext("lead-1", root);
      const owned = allocateAgentHome(context, "agent-race", "worker");
      writeFileSync(owned.sessionPath!, "owned history");
      updateOwnership(owned.home, { liveness: { lifecycle: "stopped", running: false } });
      const external = join(root, "external-race-target");
      mkdirSync(external);
      writeFileSync(join(external, "must-survive"), "outside");

      const result = removeOwnedAgentHome(owned, staged => {
        assert.notEqual(staged, owned.home, "the checked home is atomically detached before recursive removal");
        symlinkSync(external, owned.home, "dir");
        rmSync(staged, { recursive: true, force: false });
      });
      assert.deepEqual(result, { status: "deleted" });
      assert.equal(readFileSync(join(external, "must-survive"), "utf8"), "outside");
      assert.equal(realpathSync(owned.home), realpathSync(external));
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("surfaces deletion failure once without terminal output and retains ownership for a safe retry", (t) => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    const diagnostics = t.mock.method(console, "error", () => {});
    const notices: string[] = [];
    ownerNotifyRef.current = message => notices.push(message);
    try {
      const owned = allocateAgentHome(createAgentStorageContext("lead-1", root), "agent-failure", "worker");
      updateOwnership(owned.home, { liveness: { lifecycle: "stopped", running: false } });
      const result = removeOwnedAgentHome(owned, (home) => {
        rmSync(join(home, "ownership.json"));
        throw new Error("permission denied after partial removal");
      });
      assert.equal(result.status, "failed");
      assert.equal(diagnostics.mock.callCount(), 0);
      assert.deepEqual(notices, ["ws: could not remove an owned-agent home; it was retained for safety."]);
      assert.doesNotMatch(notices[0]!, /permission|agent-failure|ws-pi-storage/);
      assert.ok(readOwnership(owned.home), "metadata remains available for retry");
    } finally { ownerNotifyRef.current = undefined; rmSync(root, { recursive: true, force: true }); }
  });

  test("prunes stopped children at the TTL boundary across lead subtrees while retaining recent, protected, live, and unknown homes", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    const day = 24 * 60 * 60 * 1000;
    const now = Date.parse("2026-10-10T00:00:00.000Z");
    try {
      const old = allocateAgentHome(createAgentStorageContext("lead-old", root), "agent-old", "worker");
      const boundary = allocateAgentHome(createAgentStorageContext("lead-boundary", root), "agent-boundary", "fork");
      const recent = allocateAgentHome(createAgentStorageContext("lead-mixed", root), "agent-recent", "worker");
      const protectedChild = allocateAgentHome(createAgentStorageContext("lead-mixed", root), "agent-protected", "worker");
      const live = allocateAgentHome(createAgentStorageContext("lead-live", root), "agent-live", "worker");
      const unknown = allocateAgentHome(createAgentStorageContext("lead-unknown", root), "agent-unknown", "worker");
      const legacyHome = join(realpathSync(root), "ws-agents", "legacy-lead", "legacy-child");
      mkdirSync(legacyHome, { recursive: true });
      for (const [owned, lastActivityAt] of [[old, now - 31 * day], [boundary, now - 30 * day], [recent, now - 29 * day]] as const) {
        const metadata = readOwnership(owned.home)!;
        writeOwnership({ ...metadata, lastActivityAt, liveness: { ...metadata.liveness, lifecycle: "stopped", running: false } });
      }
      for (const [owned, liveness] of [
        [protectedChild, { lifecycle: "stopped", running: false, ownerHeld: true }],
        [live, { lifecycle: "live", running: true }],
      ] as const) {
        const metadata = readOwnership(owned.home)!;
        writeOwnership({ ...metadata, lastActivityAt: now - 40 * day, liveness: { ...metadata.liveness, ...liveness } });
      }
      const unknownMetadata = readOwnership(unknown.home)!;
      writeOwnership({ ...unknownMetadata, lastActivityAt: now - 40 * day, liveness: { ...unknownMetadata.liveness, lifecycle: "unknown" } });

      const result = pruneStaleAgentHomes(realpathSync(root), 30, { now: () => now });
      assert.deepEqual(new Set(result.deletedHomes), new Set([old.home, boundary.home]));
      assert.equal(existsSync(old.home), false);
      assert.equal(existsSync(boundary.home), false, "age equal to the TTL is stale");
      for (const home of [recent.home, protectedChild.home, live.home, unknown.home, legacyHome]) assert.equal(existsSync(home), true, home);
      assert.equal(existsSync(dirname(recent.home)), true, "a mixed-age lead subtree survives with its retained children");
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("a newly observed session write refreshes activity before the stale decision", (t) => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    const now = Date.parse("2026-10-10T00:00:00.000Z");
    t.mock.method(Date, "now", () => now);
    try {
      const owned = allocateAgentHome(createAgentStorageContext("lead-write", root), "agent-write", "worker");
      const metadata = readOwnership(owned.home)!;
      writeOwnership({ ...metadata, lastActivityAt: now - 31 * 86_400_000, liveness: { ...metadata.liveness, lifecycle: "stopped", running: false } });
      writeFileSync(owned.sessionPath!, "new history");
      const result = pruneStaleAgentHomes(realpathSync(root), 30, { now: () => now });
      assert.deepEqual(result.deletedHomes, []);
      assert.equal(existsSync(owned.home), true);
      assert.equal(readOwnership(owned.home)!.lastActivityAt, now);
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("the deletion claim blocks a racing activity write and stale eligibility is rechecked under that claim", (t) => {
    const diagnostics = t.mock.method(console, "error", () => {});
    const notices: string[] = [];
    ownerNotifyRef.current = message => notices.push(message);
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const owned = allocateAgentHome(createAgentStorageContext("lead-race", root), "agent-race-lock", "worker");
      updateOwnership(owned.home, { liveness: { lifecycle: "stopped", running: false } });
      const cutoff = Date.now() - 1;
      touchOwnership(owned.home);
      const refreshed = removeOwnedAgentHome(owned, undefined, metadata => metadata.lastActivityAt <= cutoff);
      assert.equal(refreshed.status, "retained", "a touch that wins the claim invalidates stale eligibility");

      const metadata = readOwnership(owned.home)!;
      writeOwnership({ ...metadata, lastActivityAt: 1 });
      const removed = removeOwnedAgentHome(owned, staged => {
        assert.equal(touchOwnership(owned.home), false, "a touch cannot cross the deletion claim or recreate the detached home");
        rmSync(staged, { recursive: true, force: false });
      }, current => current.lastActivityAt <= cutoff);
      assert.deepEqual(removed, { status: "deleted" });
      assert.equal(existsSync(owned.home), false);
      assert.equal(diagnostics.mock.callCount(), 0);
      assert.deepEqual(notices, ["ws: could not persist owned-agent metadata; the affected operation was rejected."]);
    } finally { ownerNotifyRef.current = undefined; rmSync(root, { recursive: true, force: true }); }
  });

  test("a claim abandoned by a dead process is recovered without weakening malformed/live claim retention", (t) => {
    const diagnostics = t.mock.method(console, "error", () => {});
    const notices: string[] = [];
    ownerNotifyRef.current = message => notices.push(message);
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const owned = allocateAgentHome(createAgentStorageContext("lead-abandoned", root), "agent-abandoned", "worker");
      const lock = join(dirname(owned.home), `.${owned.agentId}.ownership-lock`);
      mkdirSync(lock);
      writeFileSync(join(lock, "owner.json"), JSON.stringify({ pid: 2_147_483_647 }));
      assert.equal(touchOwnership(owned.home), true);
      assert.equal(existsSync(lock), false);

      mkdirSync(lock);
      writeFileSync(join(lock, "owner.json"), JSON.stringify({ pid: process.pid }));
      assert.equal(touchOwnership(owned.home), false, "a live holder is never reclaimed");
      rmSync(lock, { recursive: true, force: true });
      mkdirSync(lock);
      writeFileSync(join(lock, "owner.json"), "{}");
      assert.equal(touchOwnership(owned.home), false, "malformed holder facts fail closed");
      assert.equal(diagnostics.mock.callCount(), 0, "live and unreadable claims never write through console");
      assert.equal(notices.length, 1, "one session coalesces repeated metadata-update failures");
    } finally { ownerNotifyRef.current = undefined; rmSync(root, { recursive: true, force: true }); }
  });

  test("disabled pruning does not scan, and one deletion failure does not stop later homes", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    const now = Date.parse("2026-10-10T00:00:00.000Z");
    try {
      const first = allocateAgentHome(createAgentStorageContext("lead-a", root), "agent-a", "worker");
      const second = allocateAgentHome(createAgentStorageContext("lead-b", root), "agent-b", "worker");
      for (const owned of [first, second]) {
        const metadata = readOwnership(owned.home)!;
        writeOwnership({ ...metadata, lastActivityAt: 1, liveness: { ...metadata.liveness, lifecycle: "stopped", running: false } });
      }
      assert.equal(pruneStaleAgentHomes(realpathSync(root), false, { now: () => now }).scanned, 0);
      const result = pruneStaleAgentHomes(realpathSync(root), 30, {
        now: () => now,
        removeOwned: owned => owned.home === first.home ? { status: "failed", error: "permission denied" } : removeOwnedAgentHome(owned),
      });
      assert.equal(result.failed, 1);
      assert.deepEqual(result.deletedHomes, [second.home]);
      assert.equal(existsSync(first.home), true);
      assert.equal(existsSync(second.home), false);
    } finally { rmSync(root, { recursive: true, force: true }); }
  });


});
