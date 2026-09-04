/**
 * Process-role marker: env vars a spawned child process carries so the
 * running extension instance can tell "am I the host lead, a fork, or a
 * spawned worker/explore leaf" without any transport back to the parent.
 *
 * Subsumes the older `WS_PI_AGENT_CHILD_ENV` marker (spawner.ts,
 * pre-260904): rather than a single boolean "is a child" flag, every spawned
 * child now carries a role value (`"worker"` for the RPC-backed
 * `ws-agent-spawn` path, `"explore"` for the one-shot recon leaf); the host
 * lead process carries no marker at all (`readSpawnRole` returns `undefined`
 * there). `"fork"` is reserved by this phase for a not-yet-implemented
 * side-thread fork spawn (`260904-feat-ws-pi-side-thread-fork-question-surface`,
 * out of scope here) — only the role value and its parent-key env var are
 * reserved; nothing in this tree sets `"fork"` yet.
 *
 * Standalone module (not folded into spawner.ts): bridge.ts also needs
 * `isLeadOrFork`/`readSpawnRole` (for the workflow_manual->workflow_state
 * mapping's role gate, §3, and for reading `WS_PI_PARENT_SESSION_KEY_ENV`),
 * and spawner.ts already carries a type-only `import type { BridgeHandle }
 * from "./bridge.ts"` — a value-level import the other way (bridge.ts ->
 * spawner.ts) would be an inverted, if not technically cyclic, dependency.
 * Putting these tiny, dependency-free primitives in their own module lets
 * both bridge.ts and spawner.ts import them without either depending on the
 * other.
 */

export type SpawnRole = "worker" | "explore" | "fork";

/** Env var carrying the spawned child's role. Absent on the host lead process. */
export const WS_PI_SPAWN_ROLE_ENV = "WS_PI_SPAWN_ROLE";

/**
 * Env var carrying the lead's own session key, delivered to a `fork` child
 * only (reserved by this phase — no spawn path sets it yet; see this file's
 * top-of-file doc comment).
 */
export const WS_PI_PARENT_SESSION_KEY_ENV = "WS_PI_PARENT_SESSION_KEY";

const VALID_ROLES: ReadonlySet<string> = new Set<SpawnRole>(["worker", "explore", "fork"]);

/**
 * Reads `env[WS_PI_SPAWN_ROLE_ENV]` and validates it against the three
 * literal `SpawnRole` values; any other value (unset, empty, unrecognized)
 * returns `undefined` — treated the same as "host lead, no role marker" by
 * every consumer of this function.
 */
export function readSpawnRole(env: NodeJS.ProcessEnv): SpawnRole | undefined {
  const value = env[WS_PI_SPAWN_ROLE_ENV];
  return typeof value === "string" && VALID_ROLES.has(value) ? (value as SpawnRole) : undefined;
}

/**
 * `true` when `role` is the host lead (`undefined`, no marker) or a `fork`
 * child — the two roles that should see the ws system-prompt block (§4) and
 * the workflow_manual->workflow_state mapping (§3). `false` for `worker`/
 * `explore` — spawned children that never see either.
 */
export function isLeadOrFork(role: SpawnRole | undefined): boolean {
  return role === undefined || role === "fork";
}
