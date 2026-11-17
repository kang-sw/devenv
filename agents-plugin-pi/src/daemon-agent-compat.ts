/** Compatibility for same-version daemons whose compiled prose still names native tools. */
import { readFileSync, writeFileSync } from "node:fs";
import type { McpContentItem, McpToolCallResult } from "./mcp-stdio-client.ts";

export const DAEMON_AGENT_CAPABILITIES = { experimental: { wsPiDaemonAgents: true } };

/** Only an uncurated owner lead can advertise daemon coordination before discovery. */
export function daemonAgentInitializeCapabilities(role: string | undefined, hasDelegationPolicy: boolean): Record<string, unknown> {
  return role === undefined && !hasDelegationPolicy ? DAEMON_AGENT_CAPABILITIES : {};
}

// This also travels inside rendered files: daemon children need not load this extension.
export const PI_AGENT_COMPAT_GUIDE = `## Pi delegation compatibility

For Pi delegation, this section supersedes legacy native-agent instructions in the text above or in older playbooks. Use only tools actually listed in this session; if agent.* is unavailable, report delegation unavailable rather than inventing a native tool or bypassing an explicit disable.

- Legacy ws-agent-spawn means ws/agent.spawn; read a rendered prompt file and pass its contents as prompt. Choose a capability tier (small, medium, large, xlarge; ws/agent.tiers lists what is spawnable now), not a concrete model, and an authority (read-only, workdir-write, or full) no higher than your own. Legacy model_name, model_effort, alias and write_scopes are not agent.spawn arguments; do not port them.
- Legacy ws-agent-send means ws/mailbox.send to the child's returned address (id:<id>), and ws-agent-list means ws/agent.list. Use the returned address, not a native alias or agent_id argument. There is no stop or transcript: a child sleeps after its idle timer and wakes on its next mail, and ws/agent.interrupt ends a running turn.
- The old explore tool/researcher means ws/agent.spawn at tier small with authority read-only and a local evidence question. Authority caps a child's ws tools; it promises no web access, write scopes, or OS sandbox.
- ws-fork is retired, with no context-cloning replacement. Give an ordinary child explicit task inputs instead.
- Daemon children return their ordinary final answer to their direct parent at turn completion. End your turn while they work. Do not poll. Native ws-agent-report/settled/orphan lifecycle instructions do not describe daemon children; ws-report-to-lead remains internal to retained execution workers, not a daemon progress API.

The ws/ prefix denotes the installed ws MCP namespace (for example ws__agent_spawn on the Pi bridge); use the registered tool names and schemas in this session.`;

const LEGACY_GUIDANCE = /\bws-agent-(?:spawn|send|list|stop|transcript|report|settled|orphaned)\b|\bws-fork\b|\bws-report-to-lead\b|the `explore` researcher/;

/** Append rather than blindly rename: legacy argument and lifecycle contracts differ. */
export function adaptLegacyPiGuidance(text: string): string {
  return LEGACY_GUIDANCE.test(text) && !text.includes(PI_AGENT_COMPAT_GUIDE)
    ? `${text.trimEnd()}\n\n${PI_AGENT_COMPAT_GUIDE}\n`
    : text;
}

export function adaptPiGuidanceContent(content: McpContentItem[]): McpContentItem[] {
  return content.map(item => item.type === "text" && typeof item.text === "string"
    ? { ...item, text: adaptLegacyPiGuidance(item.text) } : item);
}

/** Adapt only a successful render's prompt artifact, before any provenance digest is captured. */
export function adaptPiPromptArtifact(path: string): void {
  const original = readFileSync(path, "utf8");
  const adapted = adaptLegacyPiGuidance(original);
  if (adapted !== original) writeFileSync(path, adapted, "utf8");
}

type CallTool = (name: string, args: Record<string, unknown>) => Promise<McpToolCallResult>;
const FALLBACK_KEY = "agent.profile.search-answer-pi";
const READ_TOOLS = new Set(["read", "grep", "find", "ls"]);
const PROFILE_FIELDS = new Set(["description", "backend", "model", "effort", "tools", "ws_tools", "mcp_servers", "plugins", "skills", "instructions", "idle_timeout_seconds", "list_in_index", "lead", "permission_mode"]);
const emptyArray = (value: unknown) => value === undefined || value === null || (Array.isArray(value) && value.length === 0);

/** Whole-profile overrides are authoritative; a builtin's name alone proves nothing. */
export function isReadOnlyFallbackProfile(value: unknown): boolean {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const p = value as Record<string, unknown>;
  return Object.keys(p).every(key => PROFILE_FIELDS.has(key)) && p.backend === "pi" &&
    Array.isArray(p.tools) && p.tools.length > 0 && p.tools.every(tool => typeof tool === "string" && READ_TOOLS.has(tool)) &&
    emptyArray(p.ws_tools) && emptyArray(p.plugins) &&
    (p.mcp_servers === undefined || p.mcp_servers === null || (typeof p.mcp_servers === "object" && !Array.isArray(p.mcp_servers) && Object.keys(p.mcp_servers).length === 0)) &&
    (p.skills === undefined || p.skills === false) && (p.lead === undefined || p.lead === false) &&
    (p.permission_mode === undefined || p.permission_mode === "");
}

/** Unknown-profile rejection happens before creation; no other failure is safe to retry. */
export async function callPiWithExploreFallback(callTool: CallTool, rawName: string, args: Record<string, unknown>): Promise<McpToolCallResult> {
  const result = await callTool(rawName, args);
  const error = result.content.find(item => item.type === "text")?.text;
  if (rawName !== "pi.spawn" || args.profile !== "explore" || !result.isError ||
      typeof error !== "string" || !/^pi\.spawn: unknown agent profile "explore"(?:: no agent profile resolves for | for )[^\r\n]+\s*$/.test(error)) return result;

  let proven = false;
  try {
    const config = await callTool("config.get", { key: FALLBACK_KEY, format: "json", ...(args.session_key !== undefined ? { session_key: args.session_key } : {}) });
    if (!config.isError) {
      const text = config.content.find(item => item.type === "text")?.text;
      const parsed = JSON.parse(text ?? "null");
      proven = parsed?.key === FALLBACK_KEY && isReadOnlyFallbackProfile(parsed.value);
    }
  } catch { /* Cannot prove read-only: never broaden or retry an uncertain spawn. */ }
  if (!proven) return { isError: true, content: [{ type: "text", text: "pi.spawn: this daemon lacks profile explore; refusing fallback because the effective search-answer-pi profile could not be verified as Pi read/grep/find/ls only, without ws tools, MCP servers, plugins, skills or lead mode." }] };
  return callTool(rawName, { ...args, profile: "search-answer-pi" });
}
