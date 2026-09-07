/**
 * Unit tests for local-devenv.ts: `readLocalDevenvMarker`'s no-marker/valid/
 * each-invalid-field cases, `buildLocalDevenvBootstrap`'s build-and-inject
 * happy path (injected `runBuild`, no real `go` binary needed) and its
 * fail-loud propagation of an injected build failure, and
 * `bridge.ts`'s `wrapLaunchErrorWithLocalDevenvContext`.
 *
 * Marker validation fixtures use real `mkdtemp`'d directories/files
 * (matching `test/execute-gateway.test.ts`'s pattern) rather than mocking
 * `node:fs`. The happy-path test uses a real temporary git repository for
 * `source_root` (real `git rev-parse --short HEAD`, per the plan's explicit
 * "only the actual go build invocation is injected" boundary) so no `go`
 * binary is ever invoked for real.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { readLocalDevenvMarker, buildLocalDevenvBootstrap, type LocalDevenvBuildDeps } from "../src/local-devenv.ts";
import { wrapLaunchErrorWithLocalDevenvContext } from "../src/bridge.ts";
import { buildStdioSpawnOptions, spawnWsMcpClient } from "../src/mcp-stdio-client.ts";
import { isLeadOrFork, readSpawnRole, WS_PI_SPAWN_ROLE_ENV } from "../src/process-role.ts";

function tempDir(prefix: string): string {
  return mkdtempSync(join(tmpdir(), prefix));
}

/** A real executable file — the marker's `go` field just needs to pass the executable-file check; it is never actually invoked. */
function makeExecutableStub(dir: string, name = "go-stub"): string {
  const path = join(dir, name);
  writeFileSync(path, "#!/bin/sh\nexit 0\n", { mode: 0o755 });
  return path;
}

function makeToolDir(dir: string): string {
  const toolDir = join(dir, "tool");
  mkdirSync(join(toolDir, "cmd", "ws-mcp"), { recursive: true });
  return toolDir;
}

/** A real git repo with one commit, so `git rev-parse --short HEAD` succeeds for real. */
function makeSourceRoot(dir: string): string {
  const sourceRoot = join(dir, "source");
  mkdirSync(sourceRoot, { recursive: true });
  execFileSync("git", ["init", "-q"], { cwd: sourceRoot });
  execFileSync("git", ["config", "user.email", "test@example.com"], { cwd: sourceRoot });
  execFileSync("git", ["config", "user.name", "Test"], { cwd: sourceRoot });
  execFileSync("git", ["commit", "--allow-empty", "-q", "-m", "init"], { cwd: sourceRoot });
  return sourceRoot;
}

function writeMarker(pluginDir: string, payload: unknown): void {
  writeFileSync(join(pluginDir, ".local-devenv-runtime"), typeof payload === "string" ? payload : JSON.stringify(payload));
}

describe("readLocalDevenvMarker", () => {
  test("no marker file -> undefined", () => {
    const dir = tempDir("ws-pi-local-devenv-none-");
    try {
      assert.equal(readLocalDevenvMarker(dir), undefined);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  test("invalid JSON throws naming the marker path", () => {
    const dir = tempDir("ws-pi-local-devenv-badjson-");
    try {
      writeMarker(dir, "{not json");
      assert.throws(() => readLocalDevenvMarker(dir), /not valid JSON/);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  test("non-object payload throws", () => {
    const dir = tempDir("ws-pi-local-devenv-nonobj-");
    try {
      writeMarker(dir, [1, 2, 3]);
      assert.throws(() => readLocalDevenvMarker(dir), /must be a JSON object/);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  test("missing schema_version throws naming schema_version", () => {
    const dir = tempDir("ws-pi-local-devenv-noschema-");
    try {
      writeMarker(dir, { source_root: "/a", tool_dir: "/b", go: "/c" });
      assert.throws(() => readLocalDevenvMarker(dir), /schema_version/);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  test("wrong schema_version throws naming schema_version", () => {
    const dir = tempDir("ws-pi-local-devenv-wrongschema-");
    try {
      writeMarker(dir, { schema_version: 2, source_root: "/a", tool_dir: "/b", go: "/c" });
      assert.throws(() => readLocalDevenvMarker(dir), /schema_version/);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  for (const key of ["source_root", "tool_dir", "go"] as const) {
    test(`missing "${key}" throws naming that field`, () => {
      const dir = tempDir(`ws-pi-local-devenv-missing-${key}-`);
      try {
        const payload: Record<string, unknown> = { schema_version: 1, source_root: "/a", tool_dir: "/b", go: "/c" };
        delete payload[key];
        writeMarker(dir, payload);
        assert.throws(() => readLocalDevenvMarker(dir), new RegExp(`field "${key}"`));
      } finally {
        rmSync(dir, { recursive: true, force: true });
      }
    });

    test(`relative "${key}" throws naming that field`, () => {
      const dir = tempDir(`ws-pi-local-devenv-relative-${key}-`);
      try {
        const payload: Record<string, unknown> = { schema_version: 1, source_root: "/a", tool_dir: "/b", go: "/c" };
        payload[key] = "relative/path";
        writeMarker(dir, payload);
        assert.throws(() => readLocalDevenvMarker(dir), new RegExp(`field "${key}"`));
      } finally {
        rmSync(dir, { recursive: true, force: true });
      }
    });
  }

  test("tool_dir without cmd/ws-mcp throws", () => {
    const dir = tempDir("ws-pi-local-devenv-notooldir-");
    try {
      const toolDir = join(dir, "tool-no-cmd");
      mkdirSync(toolDir, { recursive: true });
      writeMarker(dir, { schema_version: 1, source_root: dir, tool_dir: toolDir, go: makeExecutableStub(dir) });
      assert.throws(() => readLocalDevenvMarker(dir), /tool_dir/);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  test("non-executable go throws", () => {
    const dir = tempDir("ws-pi-local-devenv-noexec-");
    try {
      const toolDir = makeToolDir(dir);
      const goPath = join(dir, "go-not-executable");
      writeFileSync(goPath, "#!/bin/sh\n", { mode: 0o644 });
      writeMarker(dir, { schema_version: 1, source_root: dir, tool_dir: toolDir, go: goPath });
      assert.throws(() => readLocalDevenvMarker(dir), /field "go"/);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  test("go path that is a directory (not a file) throws", () => {
    const dir = tempDir("ws-pi-local-devenv-godir-");
    try {
      const toolDir = makeToolDir(dir);
      const goDir = join(dir, "go-is-a-dir");
      mkdirSync(goDir, { recursive: true });
      writeMarker(dir, { schema_version: 1, source_root: dir, tool_dir: toolDir, go: goDir });
      assert.throws(() => readLocalDevenvMarker(dir), /field "go"/);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  test("valid marker parses cleanly", () => {
    const dir = tempDir("ws-pi-local-devenv-valid-marker-");
    try {
      const toolDir = makeToolDir(dir);
      const goPath = makeExecutableStub(dir);
      writeMarker(dir, { schema_version: 1, source_root: dir, tool_dir: toolDir, go: goPath });
      assert.deepEqual(readLocalDevenvMarker(dir), { source_root: dir, tool_dir: toolDir, go: goPath });
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});

describe("buildLocalDevenvBootstrap", () => {
  test("no marker -> undefined, runBuild never invoked", async () => {
    const dir = tempDir("ws-pi-local-devenv-bootstrap-none-");
    try {
      let called = false;
      const deps: LocalDevenvBuildDeps = { runBuild: () => { called = true; } };
      const result = await buildLocalDevenvBootstrap(dir, "0.45.2", deps);
      assert.equal(result, undefined);
      assert.equal(called, false);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  test("valid marker: composes ldflags, builds into a pid-scoped temp path, renames on success", async () => {
    const dir = tempDir("ws-pi-local-devenv-bootstrap-ok-");
    try {
      const sourceRoot = makeSourceRoot(dir);
      const toolDir = makeToolDir(dir);
      const goPath = makeExecutableStub(dir);
      writeMarker(dir, { schema_version: 1, source_root: sourceRoot, tool_dir: toolDir, go: goPath });

      const shortCommit = execFileSync("git", ["rev-parse", "--short", "HEAD"], { cwd: sourceRoot, encoding: "utf8" }).trim();

      const notifications: string[] = [];
      let capturedArgv: string[] | undefined;
      let capturedCwd: string | undefined;
      let callCount = 0;
      const deps: LocalDevenvBuildDeps = {
        runBuild: (argv, opts) => {
          callCount += 1;
          capturedArgv = argv;
          capturedCwd = opts.cwd;
          // Simulate a successful `go build -o <tmpPath> ...`: write a file
          // at the declared -o path.
          const outIndex = argv.indexOf("-o");
          writeFileSync(argv[outIndex + 1], "fake-binary");
        },
        notify: (m) => notifications.push(m),
        now: (() => {
          let t = 1000;
          return () => (t += 5);
        })(),
      };

      const result = await buildLocalDevenvBootstrap(dir, "0.45.2", deps);
      assert.ok(result, "expected a build result");
      assert.equal(callCount, 1);
      assert.equal(capturedCwd, toolDir);
      assert.deepEqual(capturedArgv?.slice(0, 2), [goPath, "build"]);
      const ldflagsIndex = capturedArgv!.indexOf("-ldflags");
      assert.equal(capturedArgv![ldflagsIndex + 1], `-X main.version=0.45.2 -X main.sourceCommit=${shortCommit}`);
      assert.equal(capturedArgv![capturedArgv!.length - 1], "./cmd/ws-mcp");

      const finalPath = join(dir, ".runtime", "local-devenv", "ws-mcp");
      assert.equal(result!.env.WS_MCP_BOOTSTRAP_BINARY, finalPath);
      assert.equal(result!.context.sourceRoot, sourceRoot);
      assert.equal(result!.context.sourceCommit, shortCommit);
      assert.equal(result!.context.builtPath, finalPath);
      assert.ok(existsSync(finalPath), "expected the renamed final binary to exist");
      assert.equal(readFileSync(finalPath, "utf8"), "fake-binary");

      // The declared -o path matched ws-mcp.<pid>.tmp under .runtime/local-devenv/.
      const tmpPathUsed = capturedArgv![capturedArgv!.indexOf("-o") + 1];
      assert.match(tmpPathUsed, new RegExp(`\\.runtime[/\\\\]local-devenv[/\\\\]ws-mcp\\.${process.pid}\\.tmp$`));

      assert.ok(notifications.some((m) => m.includes(`building ws-mcp from`) && m.includes(sourceRoot) && m.includes(shortCommit)));
      assert.ok(notifications.some((m) => /finished in \d+ms/.test(m)));
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  test("runBuild throwing propagates unchanged and cleans up the partial temp build artifact (fail loud, no cache fallback)", async () => {
    const dir = tempDir("ws-pi-local-devenv-bootstrap-fail-");
    try {
      const sourceRoot = makeSourceRoot(dir);
      const toolDir = makeToolDir(dir);
      const goPath = makeExecutableStub(dir);
      writeMarker(dir, { schema_version: 1, source_root: sourceRoot, tool_dir: toolDir, go: goPath });

      const buildError = new Error("go build: simulated failure");
      let tmpPathUsed: string | undefined;
      const deps: LocalDevenvBuildDeps = {
        runBuild: (argv) => {
          // Simulate a partial/interrupted build: `go build -o <tmp>` writes
          // a (possibly incomplete) file at the declared temp path before
          // the failure is reported — the review fix's whole point is that
          // this partial artifact must actually exist for the cleanup
          // branch to have something to remove; a stub that throws with no
          // file ever written leaves that branch unexercised.
          tmpPathUsed = argv[argv.indexOf("-o") + 1];
          writeFileSync(tmpPathUsed, "partial-binary");
          throw buildError;
        },
      };

      await assert.rejects(() => buildLocalDevenvBootstrap(dir, "0.45.2", deps), (err: unknown) => {
        assert.equal(err, buildError);
        return true;
      });

      assert.ok(tmpPathUsed, "expected runBuild to have been invoked with an -o temp path");
      assert.equal(existsSync(tmpPathUsed!), false, "expected the partial temp build artifact to be cleaned up on failure");
      // No final (renamed) binary should have been produced either.
      assert.equal(existsSync(join(dir, ".runtime", "local-devenv", "ws-mcp")), false);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});

describe("wrapLaunchErrorWithLocalDevenvContext", () => {
  test("no context: passes the original Error through unchanged", () => {
    const err = new Error("launcher failed");
    const wrapped = wrapLaunchErrorWithLocalDevenvContext(err, undefined);
    assert.equal(wrapped, err);
  });

  test("no context, non-Error thrown value: coerces to Error", () => {
    const wrapped = wrapLaunchErrorWithLocalDevenvContext("plain string failure", undefined);
    assert.ok(wrapped instanceof Error);
    assert.match(wrapped.message, /plain string failure/);
  });

  test("with context: message carries source root, commit, and built path ahead of the original message", () => {
    const err = new Error("incompatible ws-mcp runtime after repair");
    const wrapped = wrapLaunchErrorWithLocalDevenvContext(err, {
      sourceRoot: "/Users/dev/devenv",
      sourceCommit: "abc1234",
      builtPath: "/Users/dev/devenv/agents-plugin-pi/.runtime/local-devenv/ws-mcp",
    });
    assert.ok(wrapped instanceof Error);
    assert.match(wrapped.message, /\/Users\/dev\/devenv/);
    assert.match(wrapped.message, /abc1234/);
    assert.match(wrapped.message, /\/Users\/dev\/devenv\/agents-plugin-pi\/\.runtime\/local-devenv\/ws-mcp/);
    assert.match(wrapped.message, /incompatible ws-mcp runtime after repair/);
  });
});

describe("buildStdioSpawnOptions / spawnWsMcpClient env forwarding", () => {
  test("no env -> spawn options carry no env key at all (byte-for-byte unchanged default behavior)", () => {
    const opts = buildStdioSpawnOptions("/some/cwd");
    assert.deepEqual(opts, { cwd: "/some/cwd", stdio: ["pipe", "pipe", "pipe"] });
    assert.equal("env" in opts, false);
  });

  test("env provided -> merged on top of a copy of process.env", () => {
    const opts = buildStdioSpawnOptions("/some/cwd", { WS_MCP_BOOTSTRAP_BINARY: "/tmp/ws-mcp" });
    assert.equal(opts.cwd, "/some/cwd");
    assert.deepEqual(opts.stdio, ["pipe", "pipe", "pipe"]);
    assert.ok(opts.env);
    assert.equal(opts.env!.WS_MCP_BOOTSTRAP_BINARY, "/tmp/ws-mcp");
    // process.env is copied in, not replaced.
    if (process.env.PATH) {
      assert.equal(opts.env!.PATH, process.env.PATH);
    }
  });

  test("spawnWsMcpClient's 4th parameter forwards through to a real child's env (readback via onStderr)", async () => {
    // spawnWsMcpClient hardcodes "python3 <path> serve --stdio" as the
    // launch shape, so this real-subprocess check uses a tiny python3 script
    // in place of the actual launcher — it never speaks the JSON-RPC
    // protocol, it just echoes the injected env var to stderr and exits,
    // which is enough to prove the env fragment reached the real child
    // process (mirroring this file's IO-free-first-then-real-subprocess-if-
    // needed style; the "python3" launch command itself is exercised live by
    // the ticket's owner verification step, not here).
    const scriptDir = tempDir("ws-pi-local-devenv-spawn-env-");
    try {
      const script = join(scriptDir, "print_env.py");
      writeFileSync(script, "import os, sys\nsys.stderr.write(os.environ.get('WS_MCP_BOOTSTRAP_BINARY', '<unset>'))\nsys.stderr.flush()\n");
      const collected: string[] = [];
      const client = spawnWsMcpClient(script, scriptDir, (line) => collected.push(line), {
        WS_MCP_BOOTSTRAP_BINARY: "/tmp/injected-ws-mcp",
      });
      const deadline = Date.now() + 3000;
      while (collected.join("").length === 0 && Date.now() < deadline) {
        await new Promise((resolve) => setTimeout(resolve, 20));
      }
      client.close();
      assert.equal(collected.join(""), "/tmp/injected-ws-mcp");
    } finally {
      rmSync(scriptDir, { recursive: true, force: true });
    }
  });
});

describe("local-devenv gate: worker/explore never trigger buildLocalDevenvBootstrap", () => {
  // Mirrors startBridge's exact gate expression: isLeadOrFork(readSpawnRole(process.env)).
  test("worker role -> gate is false", () => {
    assert.equal(isLeadOrFork(readSpawnRole({ [WS_PI_SPAWN_ROLE_ENV]: "worker" })), false);
  });

  test("explore role -> gate is false", () => {
    assert.equal(isLeadOrFork(readSpawnRole({ [WS_PI_SPAWN_ROLE_ENV]: "explore" })), false);
  });

  test("host lead (no role marker) -> gate is true", () => {
    assert.equal(isLeadOrFork(readSpawnRole({})), true);
  });

  test("fork role -> gate is true", () => {
    assert.equal(isLeadOrFork(readSpawnRole({ [WS_PI_SPAWN_ROLE_ENV]: "fork" })), true);
  });
});
