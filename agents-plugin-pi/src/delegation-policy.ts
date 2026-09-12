/** Adapter-owned delegation authority. Tool curation is not an OS sandbox. */
import { createHash } from "node:crypto";
import { readFileSync, realpathSync } from "node:fs";
import { join } from "node:path";
import { parse as parseYaml } from "yaml";

export const DEFAULT_MAX_AGENT_DEPTH = 2;
export const DELEGATION_ENV = "WS_PI_DELEGATION_POLICY";
export const SUBTREE_ENV = "WS_PI_SUBTREE_CHANNEL";
export const CHILD_MANAGEMENT_TOOLS = ["ws-agent-spawn", "ws-agent-send", "ws-agent-list", "ws-agent-stop", "ws-agent-transcript", "explore"] as const;
export type SessionAuthority = "leaf" | "delegate" | "lead";
export interface DelegationPolicy {
  version: 1;
  depth: number;
  maxDepth: number;
  tools: string[];
  authority: SessionAuthority;
  sessionKey?: string;
  parentSessionKey?: string;
}
export interface PlaybookProfile {
  class: "worker" | "reviewer" | "delegate" | "explore";
  authority: SessionAuthority;
  readOnly: boolean;
  requiresChildren: boolean;
}
export interface RenderProvenance extends PlaybookProfile {
  path: string;
  digest: string;
  sessionKey?: string;
}
const AUTHORITY = { leaf: 0, delegate: 1, lead: 2 } as const;
export const READ_TOOLS = ["read", "grep", "find", "ls"];
// Positive inventory: newly shipped mutators must never become read authority by default.
const READ_WS = new Set([
  "runtime_read", "runtime_debug_events", "session_children", "agenda_list", "todo_list", "todo_read",
  "api_list", "config_list", "config_resolve_agent", "git_status", "git_diff", "git_log", "git_merge_base",
  "project_tree", "infra_read", "convention_read", "note_query", "tickets_query", "tickets_template",
  "tickets_checklist", "tickets_verify", "playbook_read", "playbook_render",
]);
export function readOnlyWsTools(tools: readonly string[]): string[] {
  return tools.filter(name => name.startsWith("ws__") && READ_WS.has(name.slice(4)));
}
export function parseDelegationPolicy(value: unknown): DelegationPolicy {
  const p = value as DelegationPolicy;
  if (!p || p.version !== 1 || !Number.isSafeInteger(p.depth) || !Number.isSafeInteger(p.maxDepth) ||
      p.depth < 0 || p.maxDepth < p.depth || !Array.isArray(p.tools) || p.tools.some(t => typeof t !== "string" || !t) ||
      !Object.hasOwn(AUTHORITY, p.authority) || (p.sessionKey !== undefined && typeof p.sessionKey !== "string") ||
      (p.parentSessionKey !== undefined && typeof p.parentSessionKey !== "string")) throw new Error("ws-pi-agent: malformed delegation policy");
  return { ...p, tools: [...new Set(p.tools)] };
}
export function readDelegationPolicy(env: NodeJS.ProcessEnv = process.env): DelegationPolicy | undefined {
  const raw = env[DELEGATION_ENV];
  return raw ? parseDelegationPolicy(JSON.parse(raw)) : undefined;
}
export function terminalTools(tools: readonly string[], depth: number, maxDepth: number): string[] {
  return [...new Set(tools)].filter(tool => depth < maxDepth || !CHILD_MANAGEMENT_TOOLS.includes(tool as never));
}
export function childPolicy(parent: DelegationPolicy, tools: readonly string[], authority: SessionAuthority, requiresChildren = false, sessionKey?: string): DelegationPolicy {
  const depth = parent.depth + 1;
  if (depth > parent.maxDepth) throw new Error(`ws-pi-agent: maximum delegation depth ${parent.maxDepth} reached`);
  if (requiresChildren && depth === parent.maxDepth) throw new Error("ws-pi-agent: playbook requires children but delegation budget is exhausted");
  const effective = terminalTools(tools, depth, parent.maxDepth);
  if (parent.depth > 0) {
    const excess = effective.filter(tool => !parent.tools.includes(tool));
    if (excess.length || AUTHORITY[authority] > AUTHORITY[parent.authority]) {
      throw new Error(`ws-pi-agent: child capability exceeds parent ceiling (${excess.join(", ") || `${parent.authority} -> ${authority}`})`);
    }
  }
  return { version: 1, depth, maxDepth: parent.maxDepth, tools: effective, authority, ...(sessionKey ? { sessionKey } : {}), ...(parent.sessionKey ? { parentSessionKey: parent.sessionKey } : {}) };
}
export function assertPolicyTool(policy: DelegationPolicy | undefined, name: string): void {
  if (policy && !policy.tools.includes(name)) throw new Error(`ws-pi-agent: ${name} exceeds this agent's capability ceiling`);
}
export function assertSessionAuthority(policy: DelegationPolicy | undefined, args: Record<string, unknown>, knownKeys: ReadonlySet<string>): void {
  if (!policy) return;
  if (policy.authority !== "lead" && typeof args.session_key === "string" && !knownKeys.has(args.session_key)) throw new Error("ws-pi-agent: session key is outside this agent's authority");
  if (args.capability !== undefined && (!Object.hasOwn(AUTHORITY, String(args.capability)) || AUTHORITY[args.capability as SessionAuthority] > AUTHORITY[policy.authority])) throw new Error("ws-pi-agent: requested session capability exceeds parent ceiling");
}
export function promptDigest(path: string): string { return createHash("sha256").update(readFileSync(path)).digest("hex"); }

/** Classification uses the installed, manifest-verified playbook, never its rendered filename. */
export function playbookProfile(pluginDir: string, name: unknown): PlaybookProfile {
  if (typeof name !== "string" || !/^[a-z][a-z0-9-]*$/.test(name)) throw new Error("ws-pi-agent: unrecognized delegated playbook");
  const relative = `${name}/${name}.md`;
  const manifest = JSON.parse(readFileSync(join(pluginDir, "rsrc", "manifest.json"), "utf8"));
  const source = join(pluginDir, "rsrc", relative);
  if (!manifest.files?.[relative] || promptDigest(source) !== manifest.files[relative]) throw new Error("ws-pi-agent: delegated playbook lacks trusted shipped provenance");
  const body = readFileSync(source, "utf8");
  const header = body.match(/^---\r?\n([\s\S]*?)\r?\n---/);
  const meta = header ? parseYaml(header[1]) : undefined;
  if (meta?.kind !== "render") throw new Error("ws-pi-agent: lead-control playbooks cannot be delegated");
  if (name === "explore") return { class: "explore", authority: "leaf", readOnly: true, requiresChildren: false };
  if (meta.role === "reviewer") return { class: "reviewer", authority: "delegate", readOnly: true, requiresChildren: false };
  if (meta.role === "worker") return { class: "worker", authority: "lead", readOnly: false, requiresChildren: true };
  if (meta.role === "implementer" || meta.role === "delegate") return { class: "delegate", authority: "delegate", readOnly: false, requiresChildren: false };
  if (meta.role === "leaf") return { class: "delegate", authority: "leaf", readOnly: true, requiresChildren: false };
  throw new Error("ws-pi-agent: unsupported delegated playbook class");
}

/** Only successful bridge renders add entries; restore from adapter custom entries, not prompt claims. */
export class RenderRegistry {
  private entries = new Map<string, RenderProvenance>();
  record(path: string, profile: PlaybookProfile): RenderProvenance {
    const canonical = realpathSync(path);
    const body = readFileSync(canonical, "utf8");
    const sessionKey = body.match(/^\*\*Your ws session_key: `([^`]+)`\*\*/m)?.[1];
    const entry = { ...profile, path: canonical, digest: promptDigest(canonical), ...(sessionKey ? { sessionKey } : {}) };
    this.entries.set(canonical, entry);
    return entry;
  }
  get(path: string): RenderProvenance | undefined {
    let canonical: string;
    try { canonical = realpathSync(path); } catch { return undefined; }
    const entry = this.entries.get(canonical);
    if (entry && promptDigest(canonical) !== entry.digest) throw new Error("ws-pi-agent: rendered prompt changed since authorization");
    return entry;
  }
  restore(values: readonly unknown[]): void {
    for (const value of values) {
      const e = value as RenderProvenance;
      if (e && typeof e.path === "string" && typeof e.digest === "string" && ["worker", "reviewer", "delegate", "explore"].includes(e.class) && Object.hasOwn(AUTHORITY, e.authority) && typeof e.readOnly === "boolean" && typeof e.requiresChildren === "boolean") this.entries.set(e.path, e);
    }
  }
}
