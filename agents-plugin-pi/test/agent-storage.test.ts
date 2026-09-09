import assert from "node:assert/strict";
import { chmodSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, test } from "node:test";
import { allocateAgentHome, createAgentStorageContext, observeSessionWrite, readOwnership, touchOwnership } from "../src/agent-storage.ts";
import { exploreLeaf } from "../src/spawner.ts";

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
      writeFileSync(fakePi, "#!/bin/sh\nprintf '%s\\n%s\\n' \"$WS_PI_APPROVAL_DIR\" \"$*\" > \"$WS_PI_TEST_CAPTURE\"\nprintf '%s\\n' '{\"type\":\"message_end\",\"message\":{\"role\":\"assistant\",\"stopReason\":\"stop\",\"content\":[{\"type\":\"text\",\"text\":\"offline evidence\"}]}}'\n");
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
      assert.equal(readOwnership(home)?.sessionPath, undefined, "terminal leaf owns scratch only");
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
