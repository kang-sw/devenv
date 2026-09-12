import assert from "node:assert/strict";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, realpathSync, renameSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { describe, test } from "node:test";
import { allocateAgentHome, createAgentStorageContext, inspectOwnedHomeRemoval, isOwnedSessionPath, observeSessionWrite, pruneStaleAgentHomes, readOwnership, removeOwnedAgentHome, touchOwnership, writeOwnership, updateOwnership } from "../src/agent-storage.ts";
import { writePrivateJson } from "../src/fork-context.ts";
import { exploreLeaf, prepareForkLaunch, validateForkReadiness } from "../src/spawner.ts";

describe("agent storage", () => {
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

  test("keeps terminal no-session leaves out of ordinary session files and rejects traversal", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const leaf = allocateAgentHome(createAgentStorageContext("lead-1", root), "agent-2", "explore", undefined, true);
      assert.equal(leaf.sessionPath, undefined);
      assert.throws(() => createAgentStorageContext("../escape", root), /unsafe Pi session id/);
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("rejects a pre-existing owned session symlink and metadata whose encoded home differs from its locator", () => {
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

  test("validates the complete session candidate and regular-file type at every ownership boundary", () => {
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
    const launch = prepareForkLaunch(undefined);
    try {
      const ownership = allocateAgentHome(createAgentStorageContext("lead-1", root), "agent-7", "fork");
      const record = { ownership, sessionPath: ownership.sessionPath } as never;
      const before = readOwnership(ownership.home);
      const candidate = `${ownership.home}/..`;
      writePrivateJson(launch.readinessPath, { nonce: launch.nonce, ownSessionKey: "child-key", sessionId: "child-id", sessionPath: candidate, activeTools: [], registeredTools: [] });
      assert.throws(() => validateForkReadiness(launch, record, { sessionId: "child-id", sessionFile: candidate }), /escaped owned home/);
      assert.equal(record.sessionPath, ownership.sessionPath);
      assert.deepEqual(readOwnership(ownership.home), before);
    } finally {
      rmSync(dirname(launch.contextPath), { recursive: true, force: true });
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
      assert.equal(diagnostics.mock.callCount(), 1);
      assert.match(String(diagnostics.mock.calls[0].arguments[0]), /ENOENT/);
      assert.equal(missing.liveness.lifecycle, "unknown");
      assert.equal(missing.liveness.running, true, "missing history is not proof of process death");
      assert.equal(missing.lastActivityAt, observed.lastActivityAt);
      assert.deepEqual(missing.sessionSignature, observed.sessionSignature);
      observeSessionWrite(owned.home, join(owned.home, "invalid-parent", "session.jsonl"));
      assert.equal(diagnostics.mock.callCount(), 2, "other observation IO failures remain diagnostic");
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("a real stat failure before the first write is still diagnostic", (t) => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    const diagnostics = t.mock.method(console, "error", () => {});
    try {
      const owned = allocateAgentHome(createAgentStorageContext("lead-1", root), "agent-8", "worker");
      const before = readOwnership(owned.home)!;
      observeSessionWrite(owned.home, join(owned.home, "ownership.json", "invalid"));
      assert.equal(diagnostics.mock.callCount(), 1);
      assert.match(String(diagnostics.mock.calls[0].arguments[0]), /ENOTDIR/);
      const after = readOwnership(owned.home)!;
      assert.equal(after.liveness.lifecycle, "unknown");
      assert.equal(after.lastActivityAt, before.lastActivityAt);
      assert.equal(after.sessionSignature, undefined);
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

  test("retains unknown and protected children, mismatched descriptors, and homes containing symlinks", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const context = createAgentStorageContext("lead-1", root);
      const unknown = allocateAgentHome(context, "agent-unknown", "worker");
      assert.equal(inspectOwnedHomeRemoval(unknown).status, "retained");

      const protections = [
        { threadBound: true }, { ownerHeld: true }, { pendingQuestion: true },
        { waitingOnChildren: true }, { expectedReport: true }, { pendingApprovalCommandId: "call-1" },
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

  test("refuses removal when the home, lead subtree, or namespace is redirected through a symlink", () => {
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

  test("a replacement symlink racing deletion is never traversed outside the owned home", () => {
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

  test("surfaces deletion failure without throwing and retains ownership for a safe retry", (t) => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    const diagnostics = t.mock.method(console, "error", () => {});
    try {
      const owned = allocateAgentHome(createAgentStorageContext("lead-1", root), "agent-failure", "worker");
      updateOwnership(owned.home, { liveness: { lifecycle: "stopped", running: false } });
      const result = removeOwnedAgentHome(owned, (home) => {
        rmSync(join(home, "ownership.json"));
        throw new Error("permission denied after partial removal");
      });
      assert.equal(result.status, "failed");
      assert.equal(diagnostics.mock.callCount(), 1);
      assert.ok(readOwnership(owned.home), "metadata remains available for retry");
    } finally { rmSync(root, { recursive: true, force: true }); }
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

  test("the deletion claim blocks a racing activity write and stale eligibility is rechecked under that claim", () => {
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
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("a claim abandoned by a dead process is recovered without weakening malformed/live claim retention", () => {
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
    } finally { rmSync(root, { recursive: true, force: true }); }
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

  test("a terminal explore spawn error still removes its no-session scratch home", async () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    const bin = join(root, "empty-bin");
    const oldPath = process.env.PATH;
    const oldArgv = process.argv[1];
    try {
      mkdirSync(bin);
      process.argv[1] = join(root, "no-such-current-script");
      process.env.PATH = bin;
      const registry = new Map();
      const result = await exploreLeaf(
        { callTool: async () => ({ isError: false, content: [{ type: "text", text: "/tmp/offline-explore.md\n" }] }) } as never,
        registry,
        { sessionKey: "key", cwd: root, storage: createAgentStorageContext("lead-error", root) },
        { query: "offline query" },
      );
      assert.equal(result.state, "done");
      assert.equal(registry.size, 0);
      assert.deepEqual(readdirSync(join(realpathSync(root), "ws-agents")), [], "failed launch leaves no scratch child or lead directory");
    } finally {
      process.argv[1] = oldArgv;
      if (oldPath === undefined) delete process.env.PATH; else process.env.PATH = oldPath;
      rmSync(root, { recursive: true, force: true });
    }
  });

  test("dispatches a terminal explore leaf through the real child-process seam into its owned scratch home", async () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    const bin = join(root, "bin");
    const capture = join(root, "leaf-capture.json");
    const fakePi = join(bin, "pi");
    const oldPath = process.env.PATH;
    const oldCapture = process.env.WS_PI_TEST_CAPTURE;
    const oldArgv = process.argv[1];
    try {
      // The production fallback is a bare `pi` only when there is no current
      // script to re-invoke. A disposable executable lets this test drive the
      // real spawn/stdout/close path without a provider or model request.
      mkdirSync(bin);
      writeFileSync(fakePi, "#!/bin/sh\nmkdir -p \"$WS_PI_APPROVAL_DIR\"\nprintf '%s' '{\"decision\":\"approve\"}' > \"$WS_PI_APPROVAL_DIR/call-1.decision.json\"\nprintf '%s\\n%s\\n' \"$WS_PI_APPROVAL_DIR\" \"$*\" > \"$WS_PI_TEST_CAPTURE\"\nprintf '%s\\n' '{\"type\":\"message_end\",\"message\":{\"role\":\"assistant\",\"stopReason\":\"stop\",\"content\":[{\"type\":\"text\",\"text\":\"offline evidence\"}]}}'\n");
      chmodSync(fakePi, 0o755);
      process.argv[1] = join(root, "no-such-current-script");
      process.env.PATH = `${bin}:${oldPath ?? ""}`;
      process.env.WS_PI_TEST_CAPTURE = capture;
      const registry = new Map();
      const result = await exploreLeaf(
        { callTool: async () => ({ isError: false, content: [{ type: "text", text: "/tmp/offline-explore.md\n" }] }) } as never,
        registry,
        { sessionKey: "key", cwd: root, storage: createAgentStorageContext("lead-1", root) },
        { query: "offline query" },
      );
      const [approvalDir, argv] = readFileSync(capture, "utf8").trimEnd().split("\n");
      const home = join(approvalDir!, "..");
      assert.equal(result.output, "offline evidence");
      assert.equal(registry.size, 0, "terminal leaves self-reap after harvest");
      assert.ok(approvalDir!.startsWith(join(realpathSync(root), "ws-agents", "lead-1")));
      assert.equal(readOwnership(home), undefined, "terminal leaf scratch is removed after process close and harvest");
      assert.match(argv!, /--no-session/);
      assert.ok(!argv!.includes("ws-pi-agent-"));
    } finally {
      process.argv[1] = oldArgv;
      if (oldPath === undefined) delete process.env.PATH; else process.env.PATH = oldPath;
      if (oldCapture === undefined) delete process.env.WS_PI_TEST_CAPTURE; else process.env.WS_PI_TEST_CAPTURE = oldCapture;
      rmSync(root, { recursive: true, force: true });
    }
  });
});
