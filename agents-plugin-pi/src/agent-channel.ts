/**
 * Parent-child control channel (260924-feat-pi-agent-channel-transport).
 *
 * Every direct RPC child gets one dedicated, adapter-owned channel. The parent
 * binds an endpoint before spawning (pipe first, loopback TCP as fallback) and
 * hands the child an endpoint descriptor, a per-launch credential, and the
 * launch generation through `RpcClientOptions.env`. The child reads and deletes
 * those values as the first action of its extension factory, connects, and
 * authenticates with a versioned hello; the parent accepts one live connection
 * per launch and frees the slot when that connection ends, so the child can
 * reconnect with the same in-memory bootstrap for as long as it lives.
 *
 * Layers, bottom up:
 * - `ChannelConnection`: the backend contract. Ordered, at-most-once message
 *   delivery within one connection, plus an end event every backend must
 *   eventually report. Framing (NDJSON here) is the backend's own business.
 *   Disconnect of the peer *process* is judged by RPC lifecycle, never here.
 * - `bindChannelEndpoint` / `connectChannelEndpoint`: the pipe and TCP backends
 *   over `node:net`. `"file"` is reserved in `ChannelBackendKind` but has no
 *   backend; its lease design lives in the research ledger.
 * - `ParentChannel` / `ChildChannel`: the protocol layer. Authenticated hello,
 *   connection policy, generation stamping on every message, readiness delivery,
 *   and the child's capped-backoff reconnect loop.
 */
import { randomBytes, timingSafeEqual } from "node:crypto";
import { existsSync, lstatSync, mkdirSync, readdirSync, unlinkSync } from "node:fs";
import net from "node:net";
import { tmpdir, userInfo } from "node:os";
import { join } from "node:path";

export const CHANNEL_PROTOCOL_VERSION = 1;

/** Bootstrap env names. The child deletes all three before anything else runs. */
export const CHANNEL_ENDPOINT_ENV = "WS_PI_CHANNEL_ENDPOINT";
export const CHANNEL_CREDENTIAL_ENV = "WS_PI_CHANNEL_CREDENTIAL";
export const CHANNEL_GENERATION_ENV = "WS_PI_CHANNEL_GENERATION";
export const CHANNEL_BOOTSTRAP_ENVS = [CHANNEL_ENDPOINT_ENV, CHANNEL_CREDENTIAL_ENV, CHANNEL_GENERATION_ENV] as const;

/** `"file"` is reserved for a future lease-based backend; only pipe and tcp bind today. */
export type ChannelBackendKind = "pipe" | "tcp" | "file";
export type ChannelEndpoint =
  | { kind: "pipe"; path: string }
  | { kind: "tcp"; host: string; port: number };

/**
 * Backend contract. `send` is ordered and at-most-once within this connection;
 * `onEnd` fires exactly once, eventually, when the connection is over. Nothing
 * here promises immediate close detection.
 */
export interface ChannelConnection {
  readonly kind: ChannelBackendKind;
  readonly ended: boolean;
  send(msg: unknown): void;
  onMessage(cb: (msg: unknown) => void): () => void;
  onEnd(cb: () => void): () => void;
  close(): void;
}

const MAX_FRAME_CHARS = 8 * 1024 * 1024;
/** Parent-side bound on an accepted socket that has not yet sent its hello. */
const HELLO_LINE_TIMEOUT_MS = 5_000;
const DEFAULT_CHILD_HELLO_TIMEOUT_MS = 10_000;
export const DEFAULT_RECONNECT_BACKOFF_CAP_MS = 1_000;
const RECONNECT_BACKOFF_BASE_MS = 50;

// ---------------------------------------------------------------------------
// Backend: NDJSON over net.Socket (Unix socket, Windows named pipe, loopback TCP)
// ---------------------------------------------------------------------------

function wrapSocket(kind: ChannelBackendKind, socket: net.Socket, initialBuffer = ""): ChannelConnection {
  const listeners = new Set<(msg: unknown) => void>();
  const endListeners = new Set<() => void>();
  const backlog: unknown[] = [];
  let buffer = initialBuffer;
  let ended = false;
  const deliver = (msg: unknown) => {
    if (listeners.size === 0) backlog.push(msg);
    else for (const listener of [...listeners]) listener(msg);
  };
  const drain = () => {
    let index: number;
    while ((index = buffer.indexOf("\n")) >= 0) {
      const line = buffer.slice(0, index);
      buffer = buffer.slice(index + 1);
      if (!line) continue;
      let parsed: unknown;
      try { parsed = JSON.parse(line); }
      catch { socket.destroy(new Error("ws-pi-channel: malformed frame")); return; }
      deliver(parsed);
    }
    if (buffer.length > MAX_FRAME_CHARS) socket.destroy(new Error("ws-pi-channel: frame too large"));
  };
  socket.setEncoding("utf8");
  socket.on("data", (chunk: string) => { buffer += chunk; drain(); });
  socket.on("close", () => {
    if (ended) return;
    ended = true;
    for (const listener of [...endListeners]) listener();
  });
  socket.on("error", () => { /* every failure surfaces through close */ });
  if (buffer.length) queueMicrotask(drain);
  return {
    kind,
    get ended() { return ended; },
    send(msg) {
      if (ended || socket.destroyed) throw new Error("ws-pi-channel: send on a closed connection");
      socket.write(JSON.stringify(msg) + "\n");
    },
    onMessage(cb) {
      listeners.add(cb);
      while (backlog.length) cb(backlog.shift());
      return () => { listeners.delete(cb); };
    },
    onEnd(cb) {
      if (ended) { queueMicrotask(cb); return () => {}; }
      endListeners.add(cb);
      return () => { endListeners.delete(cb); };
    },
    close() {
      // Graceful first so a welcome or reject line already written reaches the
      // peer; the destroy timer bounds a peer that never answers the FIN.
      socket.end();
      const force = setTimeout(() => socket.destroy(), 50);
      force.unref();
    },
  };
}

export interface ChannelBindFailure { kind: ChannelBackendKind; error: string }

export class ChannelBindError extends Error {
  readonly failures: ChannelBindFailure[];
  constructor(failures: ChannelBindFailure[]) {
    super(`ws-pi-channel: every backend failed to bind: ${failures.map(f => `${f.kind}: ${f.error}`).join("; ")}`);
    this.failures = failures;
  }
}

export interface ChannelBindOptions {
  /** Test seam: bind exactly this backend instead of the pipe -> tcp order. */
  force?: "pipe" | "tcp";
  /** Test seam: the pipe path to bind instead of a fresh one under the socket directory. */
  pipePath?: string;
  /** Test seam: the loopback host to bind instead of 127.0.0.1. */
  tcpHost?: string;
  /** Test seam: the Unix socket directory instead of the per-user one under `os.tmpdir()`. */
  socketDir?: string;
  /** Test seam: the parent-side bound on an accepted socket that has not sent its hello (default 5 s). */
  helloLineTimeoutMs?: number;
}

export interface BoundChannelEndpoint {
  server: net.Server;
  endpoint: ChannelEndpoint;
  /** Backends tried and failed before this one bound. */
  fallbackFailures: ChannelBindFailure[];
  close(): void;
}

/** Short per-user directory: macOS tmpdir is already per-user; Linux /tmp is shared, so the name carries the uid. */
export function defaultChannelSocketDir(): string {
  const owner = process.getuid?.() ?? userInfo().username;
  return join(tmpdir(), `ws-pi-${owner}`);
}

/**
 * `mkdirSync` is a no-op on an existing directory, and under a shared `/tmp`
 * the per-user name is predictable: another OS user who created it first
 * would own it, could swap our socket file for theirs, and would receive the
 * child's hello with the credential. So a directory that is not ours, or is
 * open to others, fails the pipe bind (the TCP fallback names the reason).
 */
function assertPrivateSocketDir(dir: string): void {
  const stat = lstatSync(dir);
  if (!stat.isDirectory()) throw new Error(`socket directory ${dir} is not a directory`);
  const uid = process.getuid?.();
  if (uid !== undefined && stat.uid !== uid) throw new Error(`socket directory ${dir} is owned by uid ${stat.uid}, not ${uid}`);
  if ((stat.mode & 0o077) !== 0) throw new Error(`socket directory ${dir} is accessible to other users (mode ${(stat.mode & 0o777).toString(8)})`);
}

const unixSocketsToUnlinkOnExit = new Set<string>();
let exitUnlinkHookInstalled = false;
/** Pi's RPC shutdown ends in `process.exit`, which skips `server.close()`; unlink synchronously on exit instead. */
function trackUnixSocketForExit(path: string): void {
  unixSocketsToUnlinkOnExit.add(path);
  if (exitUnlinkHookInstalled) return;
  exitUnlinkHookInstalled = true;
  process.once("exit", () => {
    for (const socketPath of unixSocketsToUnlinkOnExit) {
      try { unlinkSync(socketPath); } catch { /* already gone */ }
    }
  });
}

function probeUnixSocket(path: string): Promise<"live" | "stale" | "unknown"> {
  return new Promise(resolve => {
    const socket = net.connect({ path });
    socket.unref();
    const timer = setTimeout(() => { socket.destroy(); resolve("unknown"); }, 250);
    timer.unref();
    socket.once("connect", () => { clearTimeout(timer); socket.destroy(); resolve("live"); });
    socket.once("error", (error: NodeJS.ErrnoException) => {
      clearTimeout(timer);
      resolve(error.code === "ECONNREFUSED" ? "stale" : "unknown");
    });
  });
}

/** A socket file whose connect is refused has no listener behind it (a SIGKILLed parent left it); unlink it. */
export async function sweepStaleChannelSockets(dir: string): Promise<string[]> {
  let entries: string[];
  try { entries = readdirSync(dir); } catch { return []; }
  const removed: string[] = [];
  await Promise.all(entries.filter(name => name.endsWith(".sock")).map(async name => {
    const path = join(dir, name);
    if ((await probeUnixSocket(path)) !== "stale") return;
    try { unlinkSync(path); removed.push(path); } catch { /* raced with its owner */ }
  }));
  return removed;
}

function listen(server: net.Server, target: { path: string } | { host: string; port: number }): Promise<void> {
  return new Promise((resolve, reject) => {
    const onError = (error: Error) => { server.off("listening", onListening); reject(error); };
    const onListening = () => { server.off("error", onError); resolve(); };
    server.once("error", onError);
    server.once("listening", onListening);
    server.listen(target);
  });
}

function describeError(error: unknown): string {
  const e = error as NodeJS.ErrnoException | undefined;
  return e?.code ? `${e.code}: ${e.message}` : String(e?.message ?? error);
}

async function bindPipe(opts: ChannelBindOptions): Promise<BoundChannelEndpoint> {
  const server = net.createServer();
  server.unref();
  if (process.platform === "win32") {
    const path = opts.pipePath ?? `\\\\.\\pipe\\ws-pi-${process.pid}-${randomBytes(6).toString("hex")}`;
    try { await listen(server, { path }); } catch (error) { server.close(); throw error; }
    return { server, endpoint: { kind: "pipe", path }, fallbackFailures: [], close() { server.close(); } };
  }
  let path = opts.pipePath;
  if (!path) {
    const dir = opts.socketDir ?? defaultChannelSocketDir();
    mkdirSync(dir, { recursive: true, mode: 0o700 });
    assertPrivateSocketDir(dir);
    await sweepStaleChannelSockets(dir);
    path = join(dir, `${randomBytes(6).toString("hex")}.sock`);
  }
  // An over-long path fails here with EINVAL; the caller falls back to TCP.
  try { await listen(server, { path }); } catch (error) { server.close(); throw error; }
  trackUnixSocketForExit(path);
  return {
    server,
    endpoint: { kind: "pipe", path },
    fallbackFailures: [],
    close() {
      server.close();
      unixSocketsToUnlinkOnExit.delete(path);
      // Node unlinks a socket it created on close; keep the belt-and-braces unlink for a close that raced.
      if (existsSync(path)) { try { unlinkSync(path); } catch { /* already gone */ } }
    },
  };
}

async function bindTcp(opts: ChannelBindOptions): Promise<BoundChannelEndpoint> {
  const server = net.createServer();
  server.unref();
  const host = opts.tcpHost ?? "127.0.0.1";
  try { await listen(server, { host, port: 0 }); } catch (error) { server.close(); throw error; }
  const address = server.address() as net.AddressInfo;
  return { server, endpoint: { kind: "tcp", host, port: address.port }, fallbackFailures: [], close() { server.close(); } };
}

/** Ordered bind: pipe, then loopback TCP. Fails closed with every backend's failure named. */
export async function bindChannelEndpoint(opts: ChannelBindOptions = {}): Promise<BoundChannelEndpoint> {
  const order: Array<"pipe" | "tcp"> = opts.force ? [opts.force] : ["pipe", "tcp"];
  const failures: ChannelBindFailure[] = [];
  for (const kind of order) {
    try {
      const bound = await (kind === "pipe" ? bindPipe(opts) : bindTcp(opts));
      bound.fallbackFailures = failures;
      return bound;
    } catch (error) {
      failures.push({ kind, error: describeError(error) });
    }
  }
  throw new ChannelBindError(failures);
}

/**
 * The returned socket is ref'd: while a child waits for its hello answer inside
 * the extension factory nothing else keeps its event loop alive yet (Pi
 * attaches the RPC stdin reader only after `session_start`), and an unref'd
 * socket there lets Node exit 0 silently mid-handshake. `ChildChannel`
 * unrefs it once the hello settles.
 */
export function connectChannelEndpoint(endpoint: ChannelEndpoint): Promise<net.Socket> {
  return new Promise((resolve, reject) => {
    const socket = endpoint.kind === "pipe" ? net.connect({ path: endpoint.path }) : net.connect({ host: endpoint.host, port: endpoint.port });
    socket.once("connect", () => { socket.off("error", reject); resolve(socket); });
    socket.once("error", reject);
  });
}

// ---------------------------------------------------------------------------
// Protocol: bootstrap, hello, connection policy, readiness
// ---------------------------------------------------------------------------

export interface ChannelBootstrap {
  endpoint: ChannelEndpoint;
  credential: string;
  generation: number;
}

function parseEndpoint(raw: string): ChannelEndpoint {
  let value: Partial<{ kind: string; path: string; host: string; port: number }>;
  try { value = JSON.parse(raw); } catch { throw new Error("ws-pi-channel: malformed endpoint descriptor"); }
  if (value?.kind === "pipe" && typeof value.path === "string" && value.path) return { kind: "pipe", path: value.path };
  if (value?.kind === "tcp" && typeof value.host === "string" && value.host && Number.isInteger(value.port) && (value.port as number) > 0) return { kind: "tcp", host: value.host, port: value.port as number };
  throw new Error("ws-pi-channel: malformed endpoint descriptor");
}

/**
 * Read-then-delete. Deletion happens whether or not the values parse, so a
 * child that fails to start still leaks nothing into whatever it spawns.
 * Absent values mean this process was not launched by the adapter's spawner.
 */
export function readAndDeleteChannelBootstrap(env: NodeJS.ProcessEnv = process.env): ChannelBootstrap | undefined {
  const [endpoint, credential, generation] = CHANNEL_BOOTSTRAP_ENVS.map(key => env[key]);
  for (const key of CHANNEL_BOOTSTRAP_ENVS) delete env[key];
  if (!endpoint && !credential && !generation) return undefined;
  if (!endpoint || !credential || !generation || !/^\d+$/.test(generation)) throw new Error("ws-pi-channel: incomplete channel bootstrap");
  return { endpoint: parseEndpoint(endpoint), credential, generation: Number(generation) };
}

export type ChannelRejectReason = "busy" | "auth" | "generation" | "version" | "malformed" | "timeout";

export interface ChannelHello {
  pid?: number;
  reconnect: boolean;
  /** Extensible resume section: per-feature state the child carries across a reconnect. */
  resume: Record<string, unknown>;
}

export class ChannelRejected extends Error {
  readonly reason: string;
  constructor(reason: string) {
    super(`ws-pi-channel: rejected (${reason})`);
    this.reason = reason;
  }
}

interface ProtocolMessage { t: string; gen: number; [key: string]: unknown }

function equalSecret(a: string, b: string): boolean {
  const left = Buffer.from(a);
  const right = Buffer.from(b);
  return left.length === right.length && timingSafeEqual(left, right);
}

type Waiter<T> = { resolve: (value: T) => void; reject: (error: Error) => void };

/**
 * The parent side of one direct child's channel for one launch. Owns the
 * listener for the child's whole lifetime; `close()` when the child exits or
 * is stopped. Connection policy: credential and generation checked in the
 * hello; one live connection; a second is rejected `busy`, never replaces;
 * a reconnect after the live one ended is accepted with the same credential
 * and generation.
 */
export class ParentChannel {
  readonly credential = randomBytes(32).toString("base64url");
  readonly generation: number;
  readonly endpoint: ChannelEndpoint;
  readonly fallbackFailures: ChannelBindFailure[];
  /** Diagnostics: rejected hellos, in order. */
  readonly rejects: ChannelRejectReason[] = [];
  accepted = 0;
  private liveConnection: ChannelConnection | undefined;
  private closedFlag = false;
  private readonly bound: BoundChannelEndpoint;
  private readonly helloLineTimeoutMs: number;
  /** Accepted sockets that have not sent their hello yet; `close()` destroys them so none is welcomed later. */
  private readonly preHello = new Set<net.Socket>();
  private helloWaiters: Waiter<ChannelHello>[] = [];
  private firstHello: ChannelHello | undefined;
  private readonly readinessByKind = new Map<string, unknown>();
  private readinessWaiters = new Map<string, Waiter<unknown>[]>();
  private readonly connectionListeners = new Set<(conn: ChannelConnection, hello: ChannelHello) => void>();
  private readonly disconnectListeners = new Set<() => void>();
  private readonly messageListeners = new Set<(msg: Record<string, unknown>) => void>();

  static async bind(generation: number, opts: ChannelBindOptions = {}): Promise<ParentChannel> {
    return new ParentChannel(await bindChannelEndpoint(opts), generation, opts.helloLineTimeoutMs ?? HELLO_LINE_TIMEOUT_MS);
  }

  private constructor(bound: BoundChannelEndpoint, generation: number, helloLineTimeoutMs: number) {
    this.bound = bound;
    this.endpoint = bound.endpoint;
    this.fallbackFailures = bound.fallbackFailures;
    this.generation = generation;
    this.helloLineTimeoutMs = helloLineTimeoutMs;
    bound.server.on("connection", socket => this.handleSocket(socket));
  }

  get live(): ChannelConnection | undefined { return this.liveConnection; }
  get closed(): boolean { return this.closedFlag; }

  /** The three values the child reads and deletes. */
  bootstrapEnv(): Record<string, string> {
    return {
      [CHANNEL_ENDPOINT_ENV]: JSON.stringify(this.endpoint),
      [CHANNEL_CREDENTIAL_ENV]: this.credential,
      [CHANNEL_GENERATION_ENV]: String(this.generation),
    };
  }

  /** Resolves with the first accepted hello of this launch; rejects when the channel closes first. */
  hello(): Promise<ChannelHello> {
    if (this.firstHello) return Promise.resolve(this.firstHello);
    if (this.closedFlag) return Promise.reject(new Error("ws-pi-channel: channel closed before the child's hello"));
    return new Promise((resolve, reject) => this.helloWaiters.push({ resolve, reject }));
  }

  /** The latest readiness payload of `kind`, or a wait for its first arrival; rejects when the channel closes first. */
  readiness(kind: string): Promise<unknown> {
    if (this.readinessByKind.has(kind)) return Promise.resolve(this.readinessByKind.get(kind));
    if (this.closedFlag) return Promise.reject(new Error(`ws-pi-channel: channel closed before ${kind} readiness`));
    return new Promise((resolve, reject) => {
      const waiters = this.readinessWaiters.get(kind) ?? [];
      waiters.push({ resolve, reject });
      this.readinessWaiters.set(kind, waiters);
    });
  }

  /** Every accepted connection, including reconnects, with its hello. */
  onConnection(cb: (conn: ChannelConnection, hello: ChannelHello) => void): () => void {
    this.connectionListeners.add(cb);
    return () => { this.connectionListeners.delete(cb); };
  }

  /** The live connection ended; the slot is free for a reconnect. */
  onDisconnect(cb: () => void): () => void {
    this.disconnectListeners.add(cb);
    return () => { this.disconnectListeners.delete(cb); };
  }

  /** Feature messages from the child for this launch's generation (readiness is consumed internally). */
  onMessage(cb: (msg: Record<string, unknown>) => void): () => void {
    this.messageListeners.add(cb);
    return () => { this.messageListeners.delete(cb); };
  }

  /** Sends to the live connection, stamped with this launch's generation. Throws while no connection is live. */
  send(msg: Record<string, unknown>): void {
    if (!this.liveConnection) throw new Error("ws-pi-channel: no live child connection");
    this.liveConnection.send({ ...msg, gen: this.generation });
  }

  close(): void {
    if (this.closedFlag) return;
    this.closedFlag = true;
    this.liveConnection?.close();
    this.liveConnection = undefined;
    for (const socket of this.preHello) socket.destroy();
    this.preHello.clear();
    this.bound.close();
    const closed = new Error("ws-pi-channel: channel closed");
    for (const waiter of this.helloWaiters.splice(0)) waiter.reject(closed);
    for (const waiters of this.readinessWaiters.values()) for (const waiter of waiters) waiter.reject(closed);
    this.readinessWaiters.clear();
  }

  private acceptReadiness(kind: string, payload: unknown): void {
    this.readinessByKind.set(kind, payload);
    const waiters = this.readinessWaiters.get(kind);
    this.readinessWaiters.delete(kind);
    if (waiters) for (const waiter of waiters) waiter.resolve(payload);
  }

  private handleSocket(socket: net.Socket): void {
    if (this.closedFlag) { socket.destroy(); return; }
    socket.unref();
    socket.setEncoding("utf8");
    this.preHello.add(socket);
    let buffer = "";
    let done = false;
    const settle = () => {
      done = true;
      clearTimeout(timer);
      this.preHello.delete(socket);
    };
    const reject = (reason: ChannelRejectReason) => {
      if (done) return;
      settle();
      this.rejects.push(reason);
      try { socket.end(JSON.stringify({ t: "reject", reason }) + "\n"); } catch { /* peer gone */ }
      const destroy = setTimeout(() => socket.destroy(), 50);
      destroy.unref();
    };
    const timer = setTimeout(() => reject("timeout"), this.helloLineTimeoutMs);
    timer.unref();
    // A pre-hello socket that goes away (a stale-socket probe from another
    // parent, a peer that gave up) is not a rejected hello: no diagnostic.
    socket.once("close", () => { if (!done) settle(); });
    const onData = (chunk: string) => {
      buffer += chunk;
      const index = buffer.indexOf("\n");
      if (index < 0) {
        if (buffer.length > 1 << 20) reject("malformed");
        return;
      }
      // Accepted before `close()`, hello after it: a closed channel welcomes nobody.
      if (this.closedFlag) { settle(); socket.destroy(); return; }
      clearTimeout(timer);
      socket.off("data", onData);
      const rest = buffer.slice(index + 1);
      let hello: Record<string, unknown>;
      try { hello = JSON.parse(buffer.slice(0, index)); } catch { return reject("malformed"); }
      if (!hello || typeof hello !== "object" || hello.t !== "hello") return reject("malformed");
      if (hello.v !== CHANNEL_PROTOCOL_VERSION) return reject("version");
      if (typeof hello.cred !== "string" || !equalSecret(hello.cred, this.credential)) return reject("auth");
      if (hello.gen !== this.generation) return reject("generation");
      if (this.liveConnection) return reject("busy");
      settle();
      socket.removeAllListeners("data");
      const accepted: ChannelHello = {
        pid: typeof hello.pid === "number" ? hello.pid : undefined,
        reconnect: hello.reconnect === true,
        resume: hello.resume && typeof hello.resume === "object" && !Array.isArray(hello.resume) ? hello.resume as Record<string, unknown> : {},
      };
      const conn = wrapSocket(this.endpoint.kind, socket, rest);
      this.liveConnection = conn;
      this.accepted += 1;
      conn.onEnd(() => {
        if (this.liveConnection !== conn) return;
        this.liveConnection = undefined;
        for (const listener of [...this.disconnectListeners]) listener();
      });
      conn.onMessage(raw => {
        const msg = raw as Partial<ProtocolMessage> | null;
        if (!msg || typeof msg !== "object" || msg.gen !== this.generation || typeof msg.t !== "string") return;
        if (msg.t === "ready") {
          if (typeof msg.kind === "string") this.acceptReadiness(msg.kind, msg.payload);
          return;
        }
        for (const listener of [...this.messageListeners]) listener(msg as Record<string, unknown>);
      });
      conn.send({ t: "welcome", v: CHANNEL_PROTOCOL_VERSION, gen: this.generation });
      // A reconnecting child restores the readiness it already proved through the resume section.
      const readiness = accepted.resume.readiness;
      if (accepted.reconnect && readiness && typeof readiness === "object" && !Array.isArray(readiness)) {
        for (const [kind, payload] of Object.entries(readiness as Record<string, unknown>)) this.acceptReadiness(kind, payload);
      }
      if (!this.firstHello) {
        this.firstHello = accepted;
        for (const waiter of this.helloWaiters.splice(0)) waiter.resolve(accepted);
      }
      for (const listener of [...this.connectionListeners]) listener(conn, accepted);
    };
    socket.on("data", onData);
    socket.on("error", () => { /* surfaced through close */ });
  }
}

export interface ChildChannelOptions {
  /** Per-feature resume state included in every reconnect hello. */
  resume?: () => Record<string, unknown>;
  /** Default true: reconnect after every connection end until `close()`. Tests' in-process stand-ins pass false. */
  reconnect?: boolean;
  backoffCapMs?: number;
  helloTimeoutMs?: number;
}

/**
 * The child side. `connect` performs the first authenticated hello and throws
 * on rejection or timeout so the extension factory fails closed. Afterwards the
 * channel reconnects on its own after any connection end, with a capped
 * backoff and no attempt limit, using the in-memory bootstrap.
 */
export class ChildChannel {
  readonly generation: number;
  connections = 0;
  private readonly bootstrap: ChannelBootstrap;
  private readonly options: ChildChannelOptions;
  private connection: ChannelConnection | undefined;
  private closedFlag = false;
  private readonly readinessByKind = new Map<string, unknown>();
  private readonly resumeProviders = new Map<string, () => unknown>();
  private readonly messageListeners = new Set<(msg: Record<string, unknown>) => void>();
  private readonly reconnectListeners = new Set<() => void>();
  private readonly disconnectListeners = new Set<() => void>();

  static async connect(bootstrap: ChannelBootstrap, options: ChildChannelOptions = {}): Promise<ChildChannel> {
    const channel = new ChildChannel(bootstrap, options);
    await channel.establish(false);
    return channel;
  }

  private constructor(bootstrap: ChannelBootstrap, options: ChildChannelOptions) {
    this.bootstrap = bootstrap;
    this.options = options;
    this.generation = bootstrap.generation;
  }

  get connected(): boolean { return this.connection !== undefined; }
  get closed(): boolean { return this.closedFlag; }

  onMessage(cb: (msg: Record<string, unknown>) => void): () => void {
    this.messageListeners.add(cb);
    return () => { this.messageListeners.delete(cb); };
  }

  onReconnect(cb: () => void): () => void {
    this.reconnectListeners.add(cb);
    return () => { this.reconnectListeners.delete(cb); };
  }

  onDisconnect(cb: () => void): () => void {
    this.disconnectListeners.add(cb);
    return () => { this.disconnectListeners.delete(cb); };
  }

  /** Sends now, stamped with the launch generation. Throws while disconnected; callers decide whether to wait for `onReconnect`. */
  send(msg: Record<string, unknown>): void {
    if (!this.connection) throw new Error("ws-pi-channel: not connected to the parent");
    this.connection.send({ ...msg, gen: this.generation });
  }

  /**
   * Stage-2 readiness: sent as its own message on the authenticated connection.
   * Sticky: a reconnect hello carries every published kind in `resume.readiness`,
   * so a drop between the hello and the parent's validation loses nothing.
   */
  publishReadiness(kind: string, payload: unknown): void {
    this.readinessByKind.set(kind, payload);
    // Best effort: a socket already destroyed but not yet reported as ended
    // throws from `send`; the stored payload rides the next reconnect hello.
    try { if (this.connection) this.send({ t: "ready", kind, payload }); } catch { /* carried by the reconnect */ }
  }

  /**
   * Registers one feature's resume state under `key` for every later hello.
   * A provider returning `undefined` contributes nothing to that hello.
   */
  provideResume(key: string, provider: () => unknown): () => void {
    this.resumeProviders.set(key, provider);
    return () => { if (this.resumeProviders.get(key) === provider) this.resumeProviders.delete(key); };
  }

  close(): void {
    this.closedFlag = true;
    this.connection?.close();
    this.connection = undefined;
  }

  private async establish(reconnect: boolean): Promise<void> {
    const socket = await connectChannelEndpoint(this.bootstrap.endpoint);
    const conn = wrapSocket(this.bootstrap.endpoint.kind, socket);
    const timeoutMs = this.options.helloTimeoutMs ?? DEFAULT_CHILD_HELLO_TIMEOUT_MS;
    // Ref'd only for the handshake (see `connectChannelEndpoint`); an
    // established channel never keeps a process alive on its own.
    try { await this.awaitWelcome(conn, reconnect, timeoutMs); }
    finally { socket.unref(); }
    this.connection = conn;
    this.connections += 1;
    conn.onMessage(raw => {
      const msg = raw as Partial<ProtocolMessage> | null;
      if (!msg || typeof msg !== "object" || msg.gen !== this.generation || typeof msg.t !== "string") return;
      for (const listener of [...this.messageListeners]) listener(msg as Record<string, unknown>);
    });
    conn.onEnd(() => {
      if (this.connection !== conn) return;
      this.connection = undefined;
      for (const listener of [...this.disconnectListeners]) listener();
      if (this.options.reconnect !== false && !this.closedFlag) void this.reconnectLoop();
    });
    if (reconnect) for (const listener of [...this.reconnectListeners]) listener();
  }

  private awaitWelcome(conn: ChannelConnection, reconnect: boolean, timeoutMs: number): Promise<void> {
    return new Promise<void>((resolve, reject) => {
      let settled = false;
      const timer = setTimeout(() => { if (settled) return; settled = true; conn.close(); reject(new ChannelRejected("timeout")); }, timeoutMs);
      timer.unref();
      const offMessage = conn.onMessage(raw => {
        if (settled) return;
        settled = true;
        clearTimeout(timer);
        offMessage();
        const msg = raw as Partial<ProtocolMessage> | null;
        if (msg?.t === "welcome" && msg.gen === this.generation) resolve();
        else { conn.close(); reject(new ChannelRejected(String(msg?.reason ?? "unknown"))); }
      });
      conn.onEnd(() => {
        if (settled) return;
        settled = true;
        clearTimeout(timer);
        reject(new ChannelRejected("closed-before-welcome"));
      });
      const provided: Record<string, unknown> = {};
      for (const [key, provider] of this.resumeProviders) {
        let value: unknown;
        try { value = provider(); } catch { continue; /* one feature's failure must not block the hello */ }
        if (value !== undefined) provided[key] = value;
      }
      const resume = { ...(this.options.resume?.() ?? {}), ...provided, ...(reconnect && this.readinessByKind.size ? { readiness: Object.fromEntries(this.readinessByKind) } : {}) };
      conn.send({ t: "hello", v: CHANNEL_PROTOCOL_VERSION, cred: this.bootstrap.credential, gen: this.generation, pid: process.pid, reconnect, resume });
    });
  }

  private async reconnectLoop(): Promise<void> {
    const cap = this.options.backoffCapMs ?? DEFAULT_RECONNECT_BACKOFF_CAP_MS;
    for (let attempt = 0; !this.closedFlag; attempt += 1) {
      const delay = Math.min(cap, RECONNECT_BACKOFF_BASE_MS * 2 ** attempt);
      await new Promise<void>(resolve => { const timer = setTimeout(resolve, delay); timer.unref(); });
      if (this.closedFlag) return;
      try { await this.establish(true); return; }
      catch { /* the parent may be closing, busy, or briefly unreachable; keep trying until this process exits */ }
    }
  }
}
