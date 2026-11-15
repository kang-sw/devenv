import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

/** Only prepared workflow payloads have this marker. Plain Pi installs keep
 * their existing configuration; a prepared copy follows its workflow owner,
 * including descendants that load the same captured bundle entry. */
export function applyWorkflowOwner(root: string, env: NodeJS.ProcessEnv = process.env): void {
  const marker = join(root, "workflow-owner.json");
  if (!existsSync(marker)) return;
  const owner = JSON.parse(readFileSync(marker, "utf8")).plugin;
  if (owner !== "ws" && owner !== "wsflow") throw new Error("Invalid native workflow package owner");
  env.WS_MCP_NAMESPACE = owner;
  env.WS_MCP_SETUP_TOOL = `${owner}.setup`;
  env.WS_MCP_NO_AGENT = owner === "wsflow" ? "1" : "";
  env.WS_RSRC_ROOT = join(root, "rsrc");
  env.WS_SKILLS_ROOT = join(root, "skills");
}
