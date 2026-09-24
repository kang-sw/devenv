/**
 * In-process stand-in for the child half of the control channel, for tests
 * that patch `RpcClient.prototype.start` instead of spawning a real Pi child.
 * The patched `start()` calls `connectFakeChild(this.options.env,
 * this.options.args)`: it performs the authenticated hello the parent's
 * `spawnAgent`/`sendToAgent` wait for, publishes the role's stage-2
 * readiness with the same payload shape the real extension sends, and sends
 * the quiescent subtree snapshot the real extension's `session_start` does.
 *
 * `node --test` also runs this file (default glob); it defines no tests.
 */
import { ChildChannel, readAndDeleteChannelBootstrap, type ChildChannelOptions } from "../../src/agent-channel.ts";
import { FORK_READINESS_KIND } from "../../src/fork-context.ts";
import { WEB_READINESS_KIND } from "../../src/web-readiness.ts";
import { WS_PI_SPAWN_ROLE_ENV } from "../../src/process-role.ts";
import { SubtreeUpstream } from "../../src/subtree-lifecycle.ts";

export interface FakeChildOptions extends ChildChannelOptions {
  /** Fork readiness overrides merged over the defaults; `null` publishes none. */
  fork?: Record<string, unknown> | null;
  /** Web readiness overrides merged over the defaults; `null` publishes none. */
  web?: Record<string, unknown> | null;
  /** `null` sends no subtree snapshot, leaving the parent's view waiting. */
  subtree?: null;
}

const open = new Set<ChildChannel>();

export function sessionDirFromArgs(args: readonly string[] | undefined): string | undefined {
  const index = args?.indexOf("--session-dir") ?? -1;
  return index >= 0 ? args![index + 1] : undefined;
}

/** Default fork readiness: the session the harnesses' `getState()` reports, under the launch's session dir. */
export function defaultForkReadiness(sessionDir: string | undefined): Record<string, unknown> {
  return { ownSessionKey: "fork-child-key", sessionPath: `${sessionDir}/session.jsonl`, sessionId: "fork-child-session-id", activeTools: [], registeredTools: [] };
}

export const DEFAULT_WEB_READINESS = { tools: ["web_search", "ws_web_fetch"] };

/**
 * Connects to the bootstrap in `env` (a copy is consumed so the caller's
 * options object keeps the keys for assertions). Returns `undefined` when the
 * env carries no bootstrap, so harnesses shared with channel-less launches
 * still work.
 */
export async function connectFakeChild(env: Record<string, string | undefined> | undefined, args?: readonly string[], opts: FakeChildOptions = {}): Promise<ChildChannel | undefined> {
  const bootstrap = readAndDeleteChannelBootstrap({ ...(env ?? {}) });
  if (!bootstrap) return undefined;
  const { fork, web, subtree, ...channelOptions } = opts;
  const channel = await ChildChannel.connect(bootstrap, { reconnect: false, ...channelOptions });
  open.add(channel);
  const role = env?.[WS_PI_SPAWN_ROLE_ENV];
  if (role === "fork" && fork !== null) channel.publishReadiness(FORK_READINESS_KIND, { ...defaultForkReadiness(sessionDirFromArgs(args)), ...fork });
  if (role === "explore" && web !== null) channel.publishReadiness(WEB_READINESS_KIND, { ...DEFAULT_WEB_READINESS, ...web });
  if (subtree !== null) new SubtreeUpstream(channel).publish({ outstanding: 0, active: 0, deliveries: 0, delegated: false, descendants: [] });
  return channel;
}

/** Closes every fake child opened since the last call; harness `restore()` paths call this. */
export function closeFakeChildren(): void {
  for (const channel of open) channel.close();
  open.clear();
}
