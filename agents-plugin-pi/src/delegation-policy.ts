/** Adapter-owned delegation authority. Tool curation is not an OS sandbox. */
import { createHash } from "node:crypto";
import { readFileSync, realpathSync } from "node:fs";
import { join } from "node:path";
import { parse as parseYaml } from "yaml";
import { canDelegateWriteCapability, parseEffectiveWriteCapability, type EffectiveWriteCapability } from "./write-scopes.ts";

export const DEFAULT_MAX_AGENT_DEPTH = 2;
export const DELEGATION_ENV = "WS_PI_DELEGATION_POLICY";
export const CHILD_MANAGEMENT_TOOLS = ["ws-agent-spawn", "ws-agent-send", "ws-agent-list", "ws-agent-stop", "ws-agent-transcript", "explore"] as const;
export type SessionAuthority = "leaf" | "delegate" | "lead";
export const NETWORK_TOOLS = ["web_search", "ws_web_fetch"] as const;
export interface NetworkAuthority { search: boolean; fetch: boolean }
const NO_NETWORK: NetworkAuthority = { search: false, fetch: false };
export interface DelegationPolicy {
  version: 1;
  depth: number;
  maxDepth: number;
  tools: string[];
  authority: SessionAuthority;
  /** Separate from active tools: workers can delegate network reads without exposing a web tool themselves. */
  network?: NetworkAuthority;
  /** Explicit filesystem write authority; tool-name presence is not the authority model. */
  write?: EffectiveWriteCapability;
  sessionKey?: string;
  parentSessionKey?: string;
}
export interface PlaybookProfile {
  class: "worker" | "reviewer" | "delegate" | "explore";
  authority: SessionAuthority;
  readOnly: boolean;
  requiresChildren: boolean;
  /** Trusted playbook identity requires the code-review findings artifact contract. */
  reviewArtifact: boolean;
}
export interface RenderProvenance extends PlaybookProfile {
  path: string;
  digest: string;
  /** Admission-only immutable byte snapshot; never restored from persisted metadata. */
  readonly promptBase64: string;
  sessionKey?: string;
}
type StoredRenderProvenance = Omit<RenderProvenance, "promptBase64">;
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
function legacyWriteCapability(policy: Pick<DelegationPolicy, "depth" | "tools">): EffectiveWriteCapability {
  if (policy.depth === 0 || (policy.tools.includes("edit") && policy.tools.includes("write"))) return { mode: "unrestricted" };
  return { mode: "none" };
}

export function effectiveWriteCapability(policy: Pick<DelegationPolicy, "depth" | "tools" | "write">): EffectiveWriteCapability {
  return policy.write ? parseEffectiveWriteCapability(policy.write) : legacyWriteCapability(policy);
}

export function parseDelegationPolicy(value: unknown): DelegationPolicy {
  const p = value as DelegationPolicy;
  if (!p || p.version !== 1 || !Number.isSafeInteger(p.depth) || !Number.isSafeInteger(p.maxDepth) ||
      p.depth < 0 || p.maxDepth < p.depth || !Array.isArray(p.tools) || p.tools.some(t => typeof t !== "string" || !t) ||
      !Object.hasOwn(AUTHORITY, p.authority) || (p.sessionKey !== undefined && typeof p.sessionKey !== "string") ||
      (p.parentSessionKey !== undefined && typeof p.parentSessionKey !== "string") ||
      (p.network !== undefined && (!p.network || typeof p.network.search !== "boolean" || typeof p.network.fetch !== "boolean"))) throw new Error("ws-pi-agent: malformed delegation policy");
  let write: EffectiveWriteCapability;
  try { write = p.write === undefined ? legacyWriteCapability(p) : parseEffectiveWriteCapability(p.write); }
  catch { throw new Error("ws-pi-agent: malformed delegation policy"); }
  return { ...p, tools: [...new Set(p.tools)], write, ...(p.network ? { network: { ...p.network } } : {}) };
}
export function readDelegationPolicy(env: NodeJS.ProcessEnv = process.env): DelegationPolicy | undefined {
  const raw = env[DELEGATION_ENV];
  return raw ? parseDelegationPolicy(JSON.parse(raw)) : undefined;
}
export function terminalTools(tools: readonly string[], depth: number, maxDepth: number): string[] {
  return [...new Set(tools)].filter(tool => depth < maxDepth || !CHILD_MANAGEMENT_TOOLS.includes(tool as never));
}
export function childPolicy(parent: DelegationPolicy, tools: readonly string[], authority: SessionAuthority, requiresChildren = false, sessionKey?: string, network?: NetworkAuthority, write?: EffectiveWriteCapability): DelegationPolicy {
  const depth = parent.depth + 1;
  if (depth > parent.maxDepth) throw new Error(`ws-pi-agent: maximum delegation depth ${parent.maxDepth} reached`);
  if (requiresChildren && depth === parent.maxDepth) throw new Error("ws-pi-agent: playbook requires children but delegation budget is exhausted");
  const effective = terminalTools(tools, depth, parent.maxDepth);
  const requested = network ?? { search: effective.includes("web_search"), fetch: effective.includes("ws_web_fetch") };
  const ceiling = parent.network ?? NO_NETWORK;
  if (parent.depth > 0 && ((requested.search && !ceiling.search) || (requested.fetch && !ceiling.fetch))) {
    throw new Error("ws-pi-agent: child network capability exceeds parent ceiling");
  }
  if ((effective.includes("web_search") && !requested.search) || (effective.includes("ws_web_fetch") && !requested.fetch)) {
    throw new Error("ws-pi-agent: network tool lacks explicit authority");
  }
  const requestedWrite = write ?? legacyWriteCapability({ depth, tools: effective });
  if (!canDelegateWriteCapability(effectiveWriteCapability(parent), requestedWrite)) {
    throw new Error("ws-pi-agent: child write capability exceeds parent ceiling");
  }
  if (parent.depth > 0) {
    const scopedWrapperTools = requestedWrite.mode === "scoped" ? new Set(["edit", "write"]) : undefined;
    const excess = effective.filter(tool => !NETWORK_TOOLS.includes(tool as never) && !scopedWrapperTools?.has(tool) && !parent.tools.includes(tool));
    if (excess.length || AUTHORITY[authority] > AUTHORITY[parent.authority]) {
      throw new Error(`ws-pi-agent: child capability exceeds parent ceiling (${excess.join(", ") || `${parent.authority} -> ${authority}`})`);
    }
  }
  return { version: 1, depth, maxDepth: parent.maxDepth, tools: effective, authority, write: requestedWrite, ...(network || requested.search || requested.fetch ? { network: requested } : {}), ...(sessionKey ? { sessionKey } : {}), ...(!sessionKey && parent.sessionKey ? { parentSessionKey: parent.sessionKey } : {}) };
}
export function assertPolicyTool(policy: DelegationPolicy | undefined, name: string): void {
  if (policy && (!policy.tools.includes(name) || (name === "web_search" && !policy.network?.search) || (name === "ws_web_fetch" && !policy.network?.fetch))) throw new Error(`ws-pi-agent: ${name} exceeds this agent's capability ceiling`);
}
export function assertSessionAuthority(policy: DelegationPolicy | undefined, args: Record<string, unknown>, knownKeys: ReadonlySet<string>): void {
  if (!policy) return;
  if (typeof args.session_key === "string" && !knownKeys.has(args.session_key)) throw new Error("ws-pi-agent: session key is outside this agent's authority");
  if (args.capability !== undefined && (!Object.hasOwn(AUTHORITY, String(args.capability)) || AUTHORITY[args.capability as SessionAuthority] > AUTHORITY[policy.authority])) throw new Error("ws-pi-agent: requested session capability exceeds parent ceiling");
}
export function promptDigest(path: string): string { return createHash("sha256").update(readFileSync(path)).digest("hex"); }

/** Classification uses the installed, manifest-verified playbook, never its rendered filename or prompt claims. */
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
  const reviewArtifact = Array.isArray(meta.includes) && meta.includes.includes("code-reviewer");
  if (name === "explore") return { class: "explore", authority: "leaf", readOnly: true, requiresChildren: false, reviewArtifact };
  if (meta.role === "reviewer") return { class: "reviewer", authority: "delegate", readOnly: true, requiresChildren: false, reviewArtifact };
  if (meta.role === "worker") return { class: "worker", authority: "lead", readOnly: false, requiresChildren: true, reviewArtifact };
  if (meta.role === "implementer" || meta.role === "delegate") return { class: "delegate", authority: "delegate", readOnly: false, requiresChildren: false, reviewArtifact };
  if (meta.role === "leaf") return { class: "delegate", authority: "leaf", readOnly: true, requiresChildren: false, reviewArtifact };
  throw new Error("ws-pi-agent: unsupported delegated playbook class");
}

/** Only successful bridge renders add entries; restore from adapter custom entries, not prompt claims. */
export class RenderRegistry {
  private entries = new Map<string, StoredRenderProvenance>();
  record(path: string, profile: PlaybookProfile): StoredRenderProvenance {
    const canonical = realpathSync(path);
    const bytes = readFileSync(canonical);
    const sessionKey = bytes.toString("utf8").match(/^\*\*Your ws session_key: `([^`]+)`\*\*/m)?.[1];
    const entry = { ...profile, path: canonical, digest: createHash("sha256").update(bytes).digest("hex"), ...(sessionKey ? { sessionKey } : {}) };
    this.entries.set(canonical, entry);
    return entry;
  }
  get(path: string): RenderProvenance | undefined {
    let canonical: string;
    try { canonical = realpathSync(path); } catch { return undefined; }
    const entry = this.entries.get(canonical);
    if (!entry) return undefined;
    // Hash and launch must consume one read, even if the path changes while model resolution awaits.
    const bytes = readFileSync(canonical);
    if (createHash("sha256").update(bytes).digest("hex") !== entry.digest) throw new Error("ws-pi-agent: rendered prompt changed since authorization");
    return Object.freeze({ ...entry, promptBase64: bytes.toString("base64") });
  }
  restore(values: readonly unknown[]): void {
    for (const value of values) {
      const e = value as RenderProvenance;
      if (!e || typeof e.path !== "string" || typeof e.digest !== "string" || !["worker", "reviewer", "delegate", "explore"].includes(e.class) || !Object.hasOwn(AUTHORITY, e.authority) || typeof e.readOnly !== "boolean" || typeof e.requiresChildren !== "boolean" || typeof e.reviewArtifact !== "boolean") continue;
      try {
        const canonical = realpathSync(e.path);
        const { promptBase64: _snapshot, ...metadata } = e;
        if (promptDigest(canonical) === e.digest) this.entries.set(canonical, { ...metadata, path: canonical });
      } catch { /* stale/missing provenance is not authority */ }
    }
  }
  values(): StoredRenderProvenance[] { return [...this.entries.values()]; }
}
