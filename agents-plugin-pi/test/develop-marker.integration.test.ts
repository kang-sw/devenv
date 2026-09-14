/**
 * Opt-in live proof for the ignored local-devenv marker. Run with:
 *
 *   WS_PI_VERIFY_DEVELOP_MARKER=1 node --test test/develop-marker.integration.test.ts
 *
 * It intentionally does real source compilation and a launcher subprocess,
 * so normal `npm test` skips it. The test verifies that the marker names the
 * develop worktree, then replays the capture path that produced the bridge
 * fixtures: build -> initialize -> tools/list -> ferrule -> playbook.read ->
 * workflow_manual. This keeps the committed fixtures tied to a reproducible
 * live develop-root probe rather than a synthetic update.
 */

import { test } from "node:test";
import assert from "node:assert/strict";
import { execFile, execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { buildLocalDevenvBootstrap, readLocalDevenvMarker } from "../src/local-devenv.ts";
import { spawnWsMcpClient } from "../src/mcp-stdio-client.ts";
import { cutStaticBody } from "../src/bridge.ts";

const TEST_DIR = dirname(fileURLToPath(import.meta.url));
const PLUGIN_DIR = join(TEST_DIR, "..");
const RUNTIME = JSON.parse(readFileSync(join(PLUGIN_DIR, "runtime.json"), "utf8")) as {
  plugin_version: string;
  tools: Record<string, string>;
};
const STATIC_FIXTURE = readFileSync(join(TEST_DIR, "fixtures", "workflow-manual-static-body.txt"), "utf8");
const VERIFY_DEVELOP_MARKER = process.env.WS_PI_VERIFY_DEVELOP_MARKER === "1";

function runBuild(argv: string[], options: { cwd: string }): Promise<void> {
  return new Promise((resolve, reject) => {
    execFile(argv[0]!, argv.slice(1), { cwd: options.cwd }, (error) => {
      if (error) reject(error);
      else resolve();
    });
  });
}

function textResult(result: { content: Array<{ type: string; text?: string }>; isError?: boolean }): string {
  assert.equal(result.isError, undefined, `live ws-mcp tool failed: ${JSON.stringify(result)}`);
  const text = result.content.find((item) => item.type === "text")?.text;
  assert.ok(text, `live ws-mcp result must contain text: ${JSON.stringify(result)}`);
  return text;
}

test("opt-in develop marker builds and recaptures the bundled bridge contract", { skip: !VERIFY_DEVELOP_MARKER }, async () => {
  const marker = readLocalDevenvMarker(PLUGIN_DIR);
  assert.ok(marker, "WS_PI_VERIFY_DEVELOP_MARKER=1 requires agents-plugin-pi/.local-devenv-runtime");
  assert.equal(marker.tool_dir, join(marker.source_root, "agents-plugin-tool"));
  assert.equal(execFileSync("git", ["branch", "--show-current"], { cwd: marker.source_root, encoding: "utf8" }).trim(), "develop");

  const bootstrap = await buildLocalDevenvBootstrap(PLUGIN_DIR, RUNTIME.plugin_version, { runBuild });
  assert.ok(bootstrap, "the opted-in marker must produce a develop-root build");
  assert.equal(bootstrap.context.sourceRoot, marker.source_root);
  assert.equal(
    bootstrap.context.sourceCommit,
    execFileSync("git", ["rev-parse", "--short", "HEAD"], { cwd: marker.source_root, encoding: "utf8" }).trim(),
  );

  const client = spawnWsMcpClient(join(PLUGIN_DIR, "bin", "ws-mcp-launcher.py"), PLUGIN_DIR, undefined, bootstrap.env);
  try {
    const initialized = await client.initialize({ name: "ws-pi-develop-marker-verify", version: "0.1.0" });
    assert.equal(initialized.serverInfo.version, RUNTIME.plugin_version);
    assert.deepEqual((await client.listTools()).map((tool) => tool.name).sort(), Object.keys(RUNTIME.tools).sort());

    const ferrule = textResult(await client.callTool("ferrule", { root: join(PLUGIN_DIR, "..") }));
    const sessionKey = /^session_key: (.+)$/m.exec(ferrule)?.[1];
    assert.ok(sessionKey, `ferrule must return a session key: ${ferrule}`);
    assert.equal(textResult(await client.callTool("playbook.read", { name: "lead-workflow-manual", session_key: sessionKey })), STATIC_FIXTURE);
    assert.equal(cutStaticBody(textResult(await client.callTool("workflow_manual", { session_key: sessionKey })), STATIC_FIXTURE).found, true);
  } finally {
    client.close();
  }
});
