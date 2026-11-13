/**
 * The Pi mailbox waiter on its whole process path: the production
 * `createSubprocessWait` (stream and one-shot) spawns the real Python
 * launcher, and the launcher carries every `mailbox wait` to a fake daemon
 * over the authenticated CLI transport. The plugin's runtime binary is a
 * sentinel that journals any execution, its staged daemon copy included, and
 * the source-loaded plugin's `go` does the same, so the assertions cover every
 * process a wait started, not only the outer one: Python for each spawn, no
 * ws executable, no daemon start, no per-arm rebuild. The obsolete
 * `WS_MCP_LAUNCHER_DIRECT` is set throughout and must change nothing.
 *
 * Both plugin shapes a Pi session runs from are covered: a release-backed
 * plugin with a checksum-identified runtime, and a source-loaded plugin (a
 * valid local-devenv marker) whose runtime the bridge's bootstrap installed
 * through the launcher before its first wait.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { execFile, execFileSync, spawn, type SpawnOptions } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, unlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { buildLocalDevenvBootstrap } from "../src/local-devenv.ts";
import { createSubprocessWait, sessionMailboxWaitOptions, type MailboxWaitOutcome } from "../src/mailbox-waiter.ts";
import { startFakeDaemon, type FakeDaemon } from "./fixtures/fake-daemon.ts";

const PACKAGE_ROOT = dirname(dirname(fileURLToPath(import.meta.url)));
const RUNTIME_TEXT = readFileSync(join(PACKAGE_ROOT, "runtime.json"), "utf8");
const PLUGIN_VERSION = (JSON.parse(RUNTIME_TEXT) as { plugin_version: string }).plugin_version;
const EXE = process.platform === "win32" ? ".exe" : "";
const SESSION_KEY = "wait-key";

interface Plugin {
  root: string;
  pluginDir: string;
  launcherPath: string;
  projectDir: string;
  cache: string;
  /** Journal of every execution of the ws runtime binary or a copy of it. */
  executions: string;
  /** Journal of every `go` run (source-loaded only). */
  goRuns: string;
}

const sha256 = (bytes: Buffer | string): string => createHash("sha256").update(bytes).digest("hex");

function sentinel(journal: string): string {
  return `#!/usr/bin/env python3\nimport json, sys\nwith open(${JSON.stringify(journal)}, "a") as f: f.write(json.dumps(sys.argv) + "\\n")\nraise SystemExit(1)\n`;
}

function preparePlugin(root: string, pluginDir: string): Plugin {
  mkdirSync(join(pluginDir, "bin"), { recursive: true });
  mkdirSync(join(pluginDir, "rsrc"), { recursive: true });
  // The launcher's rsrc sentinel; without it each run waits for the tree.
  writeFileSync(join(pluginDir, "rsrc", "manifest.json"), "{}");
  const launcherPath = join(pluginDir, "bin", "ws-mcp-launcher.py");
  writeFileSync(launcherPath, readFileSync(join(PACKAGE_ROOT, "bin", "ws-mcp-launcher.py")));
  writeFileSync(join(pluginDir, "runtime.json"), RUNTIME_TEXT);
  const projectDir = join(root, "project");
  mkdirSync(projectDir, { recursive: true });
  return { root, pluginDir, launcherPath, projectDir, cache: join(root, "cache"), executions: join(root, "ws-binary-executions"), goRuns: join(root, "go-runs") };
}

function releasePlugin(root: string): Plugin {
  const plugin = preparePlugin(root, join(root, "plugin"));
  // A downloaded release runtime: sentinel bytes identified by checksum-verified metadata.
  const runtimeDir = join(root, "runtime");
  mkdirSync(runtimeDir, { recursive: true });
  const binary = join(runtimeDir, `ws-mcp-${PLUGIN_VERSION}-${sha256(RUNTIME_TEXT).slice(0, 12)}${EXE}`);
  writeFileSync(binary, sentinel(plugin.executions), { mode: 0o755 });
  writeFileSync(`${binary}.metadata.json`, JSON.stringify({ schema_version: 1, binary_sha256: sha256(readFileSync(binary)), version: PLUGIN_VERSION, release_tag: `v${PLUGIN_VERSION}` }));
  return plugin;
}

async function sourcePlugin(root: string): Promise<Plugin> {
  const sourceRoot = join(root, "source");
  mkdirSync(sourceRoot, { recursive: true });
  execFileSync("git", ["init", "-q"], { cwd: sourceRoot });
  execFileSync("git", ["-c", "user.email=t@example.com", "-c", "user.name=T", "commit", "--allow-empty", "-q", "-m", "init"], { cwd: sourceRoot });
  // Inside its own source checkout, so the launcher honors the marker.
  const plugin = preparePlugin(root, join(sourceRoot, "plugin"));
  const toolDir = join(root, "tool");
  mkdirSync(join(toolDir, "cmd", "ws-mcp"), { recursive: true });
  writeFileSync(join(toolDir, "cmd", "ws-mcp", "main.go"), "package main\n");
  const go = join(root, "go");
  writeFileSync(go, `#!/bin/sh\necho "$@" >> ${JSON.stringify(plugin.goRuns)}\nexit 1\n`, { mode: 0o755 });
  writeFileSync(join(plugin.pluginDir, ".local-devenv-runtime"), JSON.stringify({ schema_version: 1, source_root: sourceRoot, tool_dir: toolDir, go }));

  // What the bridge does at session start: build (the seam writes the
  // sentinel), let the launcher install it, then release the bootstrap.
  const bootstrap = await buildLocalDevenvBootstrap(plugin.pluginDir, PLUGIN_VERSION, {
    runBuild: (argv) => writeFileSync(argv[argv.indexOf("-o") + 1]!, sentinel(plugin.executions), { mode: 0o755 }),
  });
  assert.ok(bootstrap, "the marker is valid");
  // Asynchronous: the fake daemon answers /health from this event loop.
  const { stdout } = await promisify(execFile)("python3", [plugin.launcherPath, "version"], { cwd: plugin.pluginDir, env: { ...process.env, ...bootstrap.env }, encoding: "utf8" });
  assert.equal(stdout.trim(), PLUGIN_VERSION);
  for (const path of [bootstrap.context.builtPath, `${bootstrap.context.builtPath}.metadata.json`]) unlinkSync(path);
  return plugin;
}

const arrival = (kinds: string[]): string => `${JSON.stringify({ timed_out: false, unread: 0, kinds })}\n`;

async function exerciseWaits(plugin: Plugin, daemon: FakeDaemon): Promise<void> {
  const spawned: { command: string; args: string[] }[] = [];
  const spy = ((command: string, args: readonly string[], options: SpawnOptions) => {
    spawned.push({ command, args: [...args] });
    return spawn(command, args, options);
  }) as typeof spawn;
  const diagnostics: string[] = [];
  const options = sessionMailboxWaitOptions({ launcherPath: plugin.launcherPath, pluginDir: plugin.pluginDir, sessionKey: SESSION_KEY, cwd: plugin.projectDir, onStderr: (line) => diagnostics.push(line) });
  const argv = ["mailbox", "wait", "--session-key", SESSION_KEY, "--timeout", "10m", "--format", "json", "--root", plugin.projectDir];

  // Stream: one child serves several wakes across re-arms; a board-mode
  // change replaces it; abort stops it and closes its daemon connection.
  const streamWait = createSubprocessWait(options, spy);
  const live = new AbortController();
  const board = { boardInbox: true, boardInterrupt: true };
  const first = streamWait(live.signal, board);
  const stream = await daemon.nextCli();
  assert.deepEqual(stream.hello.argv, [...argv, "--board-inbox", "--board-interrupt", "--stream"]);
  stream.stdout(arrival(["board"]));
  assert.equal(await first, "board");
  const second = streamWait(live.signal, board);
  stream.stdout(arrival(["interrupt"]));
  assert.deepEqual(await second, { interrupt: true });
  const third = streamWait(live.signal, board);
  stream.stdout(arrival(["mail"]));
  assert.equal(await third, "mail");
  assert.equal(spawned.length, 1, "re-arms reuse the one stream child");
  const mailOnly = streamWait(live.signal);
  await stream.closed;
  const restarted = await daemon.nextCli();
  assert.deepEqual(restarted.hello.argv, [...argv, "--stream"]);
  live.abort();
  assert.equal(await mailOnly, "stopped");
  await restarted.closed;
  await streamWait.dispose?.();

  // One-shot (a runtime that rejected --stream): each wait is one launcher
  // run whose exit code and stdout kinds come back from the daemon's frames.
  const oneShot = createSubprocessWait({ ...options, streamSupport: { unsupported: true } }, spy);
  const cases: { arm?: { boardInbox?: boolean; boardSteer?: boolean; boardInterrupt?: boolean }; flags: string[]; exit: number; data: string; outcome: MailboxWaitOutcome }[] = [
    { flags: [], exit: 0, data: "", outcome: "mail" },
    { arm: { boardInbox: true }, flags: ["--board-inbox"], exit: 0, data: arrival(["board"]), outcome: "board" },
    { arm: { boardSteer: true }, flags: ["--board-steer"], exit: 0, data: arrival(["board"]), outcome: "board" },
    { arm: { boardInterrupt: true }, flags: ["--board-interrupt"], exit: 0, data: arrival(["interrupt"]), outcome: { interrupt: true } },
    { flags: [], exit: 3, data: "", outcome: "timeout" },
  ];
  for (const { arm, flags, exit, data, outcome } of cases) {
    const pending = oneShot(new AbortController().signal, arm);
    const connection = await daemon.nextCli();
    assert.deepEqual(connection.hello.argv, [...argv, ...flags]);
    connection.final(exit, data);
    assert.deepEqual(await pending, outcome);
  }
  const held = new AbortController();
  const aborted = oneShot(held.signal);
  const holding = await daemon.nextCli();
  held.abort();
  assert.equal(await aborted, "stopped");
  await holding.closed;
  await oneShot.dispose?.();

  // Every process the waits started: Python for each, and nothing below it
  // but the daemon connection.
  assert.equal(spawned.length, 2 + cases.length + 1);
  for (const { command, args } of spawned) {
    assert.equal(command, "python3");
    assert.equal(args[0], plugin.launcherPath);
    assert.deepEqual(args.slice(1, 3), ["mailbox", "wait"]);
  }
  assert.equal(daemon.hellos.length, spawned.length, "each launcher run reached the daemon exactly once");
  assert.ok(daemon.hellos.every((hello) => hello.mode === "cli"), "no MCP or probe session was opened");
  const executions = existsSync(plugin.executions) ? readFileSync(plugin.executions, "utf8") : "";
  assert.equal(executions, "", "no ws executable ran, the daemon starter included");
  assert.equal(existsSync(join(plugin.cache, "dashboard", "bin")), false, "nothing was staged for a daemon start");
  assert.equal(existsSync(plugin.goRuns), false, "no wait rebuilt the runtime");
  assert.deepEqual(diagnostics.filter((line) => line.includes("not reachable")), []);
}

async function withPlugin(prefix: string, make: (root: string) => Plugin | Promise<Plugin>): Promise<void> {
  const root = mkdtempSync(join(tmpdir(), prefix));
  const prior = { ...process.env };
  let daemon: FakeDaemon | undefined;
  try {
    for (const key of ["WS_MCP_BOOTSTRAP_BINARY", "WS_MCP_BOOTSTRAP_URL", "WS_MCP_LAUNCHER_DEBUG", "WS_RSRC_ROOT", "WS_SKILLS_ROOT"]) delete process.env[key];
    Object.assign(process.env, {
      WS_CACHE_HOME: join(root, "cache"),
      WS_CONFIG_HOME: join(root, "config"),
      WS_MCP_RUNTIME_DIR: join(root, "runtime"),
      WS_MCP_PROJECT_ROOT: join(root, "project"),
      WS_MCP_LAUNCHER_DIRECT: "1",
    });
    daemon = await startFakeDaemon(join(root, "cache"), { version: PLUGIN_VERSION });
    await exerciseWaits(await make(root), daemon);
  } finally {
    await daemon?.close();
    for (const key of Object.keys(process.env)) if (!(key in prior)) delete process.env[key];
    Object.assign(process.env, prior);
    rmSync(root, { recursive: true, force: true, maxRetries: 20, retryDelay: 100 });
  }
}

test("Pi waiter always uses the Python launcher and the daemon: release-backed plugin", async () => {
  await withPlugin("ws-pi-wait-release-", releasePlugin);
});

test("Pi waiter always uses the Python launcher and the daemon: source-loaded plugin", async () => {
  await withPlugin("ws-pi-wait-source-", sourcePlugin);
});
