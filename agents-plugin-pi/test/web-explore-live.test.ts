import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, realpathSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { createAgentStorageContext } from "../src/agent-storage.ts";

// Opt-in external acceptance: real RPC child, real model and real bundled search.
// The two parent contexts use production spawn admission/launch/relay code, not a mocked RpcClient.
test("real direct-lead and nested-worker Explore children search through the same isolated facade", {
  skip: !process.env.WS_PI_WEB_LIVE_CLI || !process.env.WS_PI_WEB_LIVE_MODEL,
  timeout: 180_000,
}, async () => {
  const oldArgv = process.argv[1];
  process.argv[1] = process.env.WS_PI_WEB_LIVE_CLI!;
  const { spawnAgent, resolveTools } = await import("../src/spawner.ts");
  process.argv[1] = oldArgv;
  const root = realpathSync(mkdtempSync(join(tmpdir(), "ws-live-explore-")));
  const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
  try {
    for (const depth of [0, 1]) {
      const registry: any = new Map();
      const pushes: any[] = [];
      let resolveFinal: (message: any) => void = () => {};
      const final = new Promise<any>(resolve => { resolveFinal = resolve; });
      const pi: any = { sendMessage(message: any) {
        pushes.push(message);
        if (message.customType === "ws-agent-report" && message.details?.kind === "final") resolveFinal(message);
      } };
      let timer: ReturnType<typeof setTimeout> | undefined;
      let searchSucceeded = false;
      let finalReport = "";
      try {
        const result = await spawnAgent(registry, {
          pi, cwd: packageRoot, storage: createAgentStorageContext(`live-parent-${depth}`, root),
          wsToolNames: [], extensionPath: join(packageRoot, "src", "index.ts"),
          client: { callTool: async () => { throw Error("unexpected tier resolution"); } } as never,
          inheritModel: process.env.WS_PI_WEB_LIVE_MODEL, toolGroup: "read-only-explore", spawnRole: "explore", exploreMode: "web-search",
          parentPolicy: { version: 1, depth, maxDepth: 2, authority: "lead", tools: resolveTools("full-worker").split(","), network: { search: true, fetch: true } },
        }, {
          systemPromptPath: join(packageRoot, "explore-guide.md"),
          prompt: "Call web_search exactly once with query 'OpenAI official API documentation'. Do not use any other evidence tools. If it succeeds with source URLs, call ws-report-to-lead with kind final and message WEB_SEARCH_LIVE_OK plus one returned URL. If it fails, report WEB_SEARCH_LIVE_FAILED plus the redacted diagnostic instead. Then stop.",
        });
        registry.get(result.agent_id).client.onEvent((event: any) => {
          if (["tool_execution_start", "tool_execution_end", "agent_settled", "message_end"].includes(event.type)) pushes.push({ customType: `${event.type}:${event.toolName ?? event.message?.stopReason ?? ""}` });
          if (event.type === "tool_execution_end" && event.toolName === "web_search" && !event.isError && event.result?.details?.results?.length) searchSucceeded = true;
          if (event.type === "tool_execution_start" && event.toolName === "ws-report-to-lead" && event.args?.kind === "final") finalReport = event.args.message;
          if (event.type === "tool_execution_end" && event.toolName === "ws-report-to-lead" && !event.isError && finalReport) resolveFinal({ content: finalReport });
        });
        const message = await Promise.race([final, new Promise((_, reject) => { timer = setTimeout(() => reject(Error(`live child did not report final; received ${pushes.map(p => p.customType).join(",")}`)), 70_000); })]);
        assert.equal(searchSucceeded, true, "real web_search tool execution returned source metadata");
        assert.match(message.content, /WEB_SEARCH_LIVE_OK/);
        assert.doesNotMatch(message.content, /WEB_SEARCH_LIVE_FAILED/);
        assert.equal(registry.get(result.agent_id).delegation.depth, depth + 1);
        assert.deepEqual(registry.get(result.agent_id).delegation.network, { search: true, fetch: true });
        console.log(JSON.stringify({ realRpcExploreDepth: depth + 1, searchSucceeded: true }));
      } finally {
        if (timer) clearTimeout(timer);
        for (const record of registry.values()) {
          record.ownershipObserverStop?.();
          await record.client?.stop();
        }
      }
    }
  } finally { rmSync(root, { recursive: true, force: true }); }
});
