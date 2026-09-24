/**
 * Nested-hop probe for `test/agent-channel-live.test.ts`: run through a real
 * Pi child's `bash` (so it inherits that child's scrubbed environment), it
 * spawns a real Explore grandchild through the production `spawnAgent` path
 * and prints one JSON line describing the grandchild's channel. Guarded by an
 * env marker because `node --test` also loads every file under `test/`.
 */
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

if (process.env.WS_PI_NESTED_PROBE === "1") {
  const packageRoot = dirname(dirname(dirname(fileURLToPath(import.meta.url))));
  const cli = join(dirname(fileURLToPath(import.meta.resolve("@earendil-works/pi-coding-agent"))), "cli.js");
  process.argv[1] = cli;
  const [{ spawnAgent, stopAgent }, { createAgentStorageContext }, { RpcClient }] = await Promise.all([
    import("../../src/spawner.ts"),
    import("../../src/agent-storage.ts"),
    import("@earendil-works/pi-coding-agent"),
  ]);
  const root = process.argv[2];
  const inheritedBootstrap = Object.keys(process.env).filter(key => key.startsWith("WS_PI_CHANNEL_"));
  RpcClient.prototype.prompt = async () => {};
  const registry: any = new Map();
  const context: any = {
    cwd: packageRoot, storage: createAgentStorageContext("nested-lead", root), wsToolNames: [], inheritModel: "openrouter/openai/gpt-4o",
    extensionPath: join(packageRoot, "src", "index.ts"), toolGroup: "read-only-explore", spawnRole: "explore", exploreMode: "code-search",
  };
  try {
    const result = await spawnAgent(registry, context, { systemPromptPath: join(packageRoot, "explore-guide.md"), prompt: "nested probe" });
    const record = registry.get(result.agent_id);
    const readiness = await record.channel.readiness("web");
    const grandchildEnv = await record.client.bash("env");
    const report = {
      inheritedBootstrap,
      generation: record.channel.generation,
      endpointKind: record.channel.endpoint.kind,
      accepted: record.channel.accepted,
      readiness,
      grandchildSeesBootstrap: /WS_PI_CHANNEL_/.test(String(grandchildEnv?.output ?? "")),
      grandchildRole: /WS_PI_SPAWN_ROLE=explore/.test(String(grandchildEnv?.output ?? "")),
    };
    await stopAgent(registry, result.agent_id, undefined, { silent: true });
    process.stdout.write(`NESTED ${JSON.stringify(report)}\n`);
  } finally {
    for (const record of registry.values()) { record.ownershipObserverStop?.(); await record.client?.stop(); }
  }
}
