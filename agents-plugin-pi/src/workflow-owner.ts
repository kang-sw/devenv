import { existsSync, readFileSync } from "node:fs";
import { isAbsolute, join } from "node:path";
import { resolveHostedMode, type HostedProbe } from "./hosted-session.ts";

let capturedWorkflowRoot: string | undefined;

/** Launch-scoped selection, available after consumeWorkflowOwner completes. */
export function hostedWorkflowRoot(): string | undefined {
  return capturedWorkflowRoot;
}

/** Consume launch-only selection before any child can inherit it. An inherited
 * hosted flag is insufficient: the registration must name this Pi process. */
export async function consumeWorkflowOwner(root: string, env: NodeJS.ProcessEnv = process.env, probe?: HostedProbe): Promise<void> {
  const explicit = env.WS_PI_WORKFLOW_ROOT;
  delete env.WS_PI_WORKFLOW_ROOT;
  capturedWorkflowRoot = undefined;
  if (explicit && (await resolveHostedMode(env, probe)).mode === "hosted") {
    const selectedEnv = { ...env, WS_PI_WORKFLOW_ROOT: explicit };
    capturedWorkflowRoot = runtimeWorkflowRoot(selectedEnv);
    applyWorkflowOwner(root, selectedEnv);
    delete selectedEnv.WS_PI_WORKFLOW_ROOT;
    Object.assign(env, selectedEnv);
  } else {
    applyWorkflowOwner(root, env);
  }
}

/** The daemon supplies this launch-only override after resolving a validated
 * workflow package. Validate again before using paths from the environment. */
export function runtimeWorkflowRoot(env: NodeJS.ProcessEnv = process.env): string | undefined {
  const root = env.WS_PI_WORKFLOW_ROOT;
  if (!root) return undefined;
  if (!isAbsolute(root)) throw new Error("Hosted workflow root must be absolute");
  const owner = JSON.parse(readFileSync(join(root, ".claude-plugin", "plugin.json"), "utf8")).name;
  if (owner !== "ws" && owner !== "wsflow") throw new Error("Invalid hosted workflow package owner");
  return root;
}

/** Keep Pi's adapter knobs while taking workflow and dashboard defaults from
 * the explicitly selected package. Ordinary/prepared Pi keeps its own defaults. */
export function workflowConfigManifests(root: string, env?: NodeJS.ProcessEnv): string[] {
  const ownerRoot = env ? runtimeWorkflowRoot(env) : capturedWorkflowRoot;
  return ownerRoot
    ? [join(root, "config-manifest.json"), join(ownerRoot, "config-manifest.json"), join(ownerRoot, "dashboard-config-manifest.json")]
    : [join(root, "config-manifest.json"), join(root, "dashboard-config-manifest.json")];
}

/** Only prepared workflow payloads have this marker. Plain Pi installs keep
 * their existing configuration; a prepared copy follows its workflow owner,
 * including descendants that load the same captured bundle entry. */
export function applyWorkflowOwner(root: string, env: NodeJS.ProcessEnv = process.env): void {
  const explicitRoot = runtimeWorkflowRoot(env);
  if (explicitRoot) {
    const owner = JSON.parse(readFileSync(join(explicitRoot, ".claude-plugin", "plugin.json"), "utf8")).name;
    env.WS_MCP_NAMESPACE = owner;
    env.WS_MCP_SETUP_TOOL = owner === "wsflow" ? "setup" : "ws.setup";
    env.WS_MCP_NO_AGENT = owner === "wsflow" ? "1" : "";
    env.WS_MCP_CLI_NAME = owner === "wsflow" ? "wsflow-cli" : "ws-cli";
    env.WS_RSRC_ROOT = join(explicitRoot, "rsrc");
    env.WS_SKILLS_ROOT = join(explicitRoot, "skills");
    return;
  }
  const marker = join(root, "workflow-owner.json");
  if (!existsSync(marker)) return;
  const owner = JSON.parse(readFileSync(marker, "utf8")).plugin;
  if (owner !== "ws" && owner !== "wsflow") throw new Error("Invalid native workflow package owner");
  env.WS_MCP_NAMESPACE = owner;
  env.WS_MCP_SETUP_TOOL = owner === "wsflow" ? "setup" : "ws.setup";
  env.WS_MCP_NO_AGENT = owner === "wsflow" ? "1" : "";
  env.WS_RSRC_ROOT = join(root, "rsrc");
  env.WS_SKILLS_ROOT = join(root, "skills");
}
