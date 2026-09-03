/**
 * Minimal, self-owned JSON-RPC-over-stdio MCP client for talking to the
 * ws-mcp launcher.
 *
 * Deliberately does not depend on @modelcontextprotocol/sdk (plan
 * Constraints: "Prefer the minimal self-owned stdio client ... to keep the
 * dependency surface empty"). The live-probe evidence gathered for this
 * plan showed the wire format is simple enough to justify a hand-rolled
 * client:
 *   - Newline-delimited JSON-RPC, one object per line, no Content-Length
 *     header framing (unlike the framed stdio transport some MCP SDKs use).
 *   - `tools/call` results use a plain MCP envelope
 *     `{content: [...], isError?: boolean}` with no protocol-level error for
 *     ordinary tool failures.
 *   - Response order is NOT guaranteed to match request order (confirmed by
 *     a live probe where a later-sent request's response arrived first) —
 *     every outgoing call must be correlated by its own `id`.
 *
 * The newline-framing/decoding (`JsonRpcLineBuffer`) and the request/response
 * id correlation (`PendingRequestRegistry`) are split into their own
 * dependency-free classes so both are unit-testable by feeding synthetic
 * `Buffer`s / messages directly, with no subprocess involved.
 */

import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { StringDecoder } from "node:string_decoder";

export interface McpToolInfo {
  name: string;
  description?: string;
  inputSchema: Record<string, unknown>;
}

export interface McpContentItem {
  type: string;
  text?: string;
  [key: string]: unknown;
}

export interface McpToolCallResult {
  content: McpContentItem[];
  isError?: boolean;
}

export interface McpInitializeResult {
  protocolVersion: string;
  capabilities: Record<string, unknown>;
  serverInfo: { name: string; version: string };
}

interface PendingCall {
  resolve: (value: unknown) => void;
  reject: (reason: unknown) => void;
}

export interface JsonRpcResponse {
  jsonrpc: "2.0";
  id?: number;
  result?: unknown;
  error?: { code: number; message: string; data?: unknown };
}

/**
 * Buffers raw stdout bytes into newline-delimited JSON-RPC messages.
 *
 * Decodes through `node:string_decoder`'s `StringDecoder` rather than
 * `Buffer#toString()` per chunk: a multibyte UTF-8 codepoint (em-dash,
 * arrows, box-drawing — all routine in ws playbook/manual prose) can land
 * split across two `'data'` events once a response exceeds the pipe's
 * buffer size, and decoding each chunk independently corrupts the split
 * codepoint into U+FFFD replacement characters. `StringDecoder` holds back
 * an incomplete trailing sequence until the next chunk completes it, so a
 * split codepoint decodes correctly regardless of where the chunk boundary
 * falls.
 *
 * Pure/synchronous, no `node:child_process` dependency — testable by
 * feeding synthetic `Buffer`s directly.
 */
export class JsonRpcLineBuffer {
  private readonly decoder = new StringDecoder("utf8");
  private readonly onMessage: (msg: unknown) => void;
  private readonly onParseError?: (line: string) => void;
  private carry = "";

  // NOTE: deliberately not using TypeScript constructor parameter properties
  // (`constructor(private readonly x: T)`) here — Node's native `.ts`
  // type-stripping (used to run this file with zero build step, see
  // package.json's "test" script and Pi's own extension loading) only
  // erases syntax, it does not transform it, and parameter properties are
  // not erasable (they inject an implicit `this.x = x` assignment). Node
  // throws `ERR_UNSUPPORTED_TYPESCRIPT_SYNTAX` on them; confirmed live via a
  // direct `node` run of this file. Explicit field declarations (above) +
  // explicit assignment (below) are the erasable-safe equivalent.
  constructor(onMessage: (msg: unknown) => void, onParseError?: (line: string) => void) {
    this.onMessage = onMessage;
    this.onParseError = onParseError;
  }

  feed(chunk: Buffer): void {
    this.carry += this.decoder.write(chunk);
    const lines = this.carry.split("\n");
    this.carry = lines.pop() ?? "";
    for (const line of lines) {
      if (!line.trim()) continue;
      let msg: unknown;
      try {
        msg = JSON.parse(line);
      } catch {
        this.onParseError?.(line);
        continue;
      }
      this.onMessage(msg);
    }
  }
}

/**
 * Correlates JSON-RPC responses to their originating request by numeric id.
 *
 * Pure bookkeeping — no IO — so out-of-order response delivery (confirmed
 * live: ws-mcp does not guarantee a response arrives in the order its
 * request was sent, so a simple FIFO queue would settle the wrong promise)
 * is directly testable without a subprocess.
 */
export class PendingRequestRegistry {
  private nextId = 1;
  private readonly pending = new Map<number, PendingCall>();

  get size(): number {
    return this.pending.size;
  }

  /** Allocates the next request id and registers its resolver pair. */
  register(resolve: (value: unknown) => void, reject: (reason: unknown) => void): number {
    const id = this.nextId++;
    this.pending.set(id, { resolve, reject });
    return id;
  }

  /** Cancels a registered id without settling it (e.g. a failed stdin write). */
  cancel(id: number): void {
    this.pending.delete(id);
  }

  /** Settles the pending call matching `msg.id`, if any. Returns whether one matched. */
  settle(msg: JsonRpcResponse): boolean {
    if (typeof msg.id !== "number" || !this.pending.has(msg.id)) {
      return false;
    }
    const pending = this.pending.get(msg.id)!;
    this.pending.delete(msg.id);
    if (msg.error) {
      pending.reject(new Error(`ws-mcp JSON-RPC error: ${msg.error.message ?? JSON.stringify(msg.error)}`));
    } else {
      pending.resolve(msg.result);
    }
    return true;
  }

  /** Rejects every still-pending call (subprocess died / failed to start). */
  rejectAll(err: unknown): void {
    for (const { reject } of this.pending.values()) reject(err);
    this.pending.clear();
  }
}

export class McpStdioClient {
  private readonly proc: ChildProcessWithoutNullStreams;
  private readonly registry = new PendingRequestRegistry();
  private readonly lineBuffer: JsonRpcLineBuffer;
  private closed = false;

  constructor(
    command: string,
    args: string[],
    options: { cwd: string; onStderr?: (line: string) => void },
  ) {
    this.proc = spawn(command, args, {
      cwd: options.cwd,
      stdio: ["pipe", "pipe", "pipe"],
    });

    this.lineBuffer = new JsonRpcLineBuffer(
      (msg) => this.handleMessage(msg as JsonRpcResponse),
      (line) => console.error(`[ws-mcp] failed to parse JSON-RPC line from stdout: ${line}`),
    );
    this.proc.stdout.on("data", (chunk: Buffer) => this.lineBuffer.feed(chunk));

    // The launcher writes all diagnostics to stderr only (never stdout,
    // which is reserved for JSON-RPC) — see ws-mcp-launcher.py note()/fail().
    // Pipe it straight to a diagnostic sink; never parse it as protocol data.
    this.proc.stderr.on("data", (chunk: Buffer) => {
      const text = chunk.toString();
      if (options.onStderr) {
        options.onStderr(text);
      } else {
        console.error(`[ws-mcp] ${text.trimEnd()}`);
      }
    });

    // Spawn-level failure (missing python3, a bad launcher path, a
    // permission error, ...). Node emits this as an async 'error' event
    // with no corresponding 'exit' — without a listener it becomes an
    // unhandled exception that neither rejects the in-flight initialize()
    // call nor runs startBridge's cleanup, leaving session_start hung
    // forever instead of failing loudly. Reject every pending call instead.
    this.proc.on("error", (err) => {
      this.closed = true;
      this.registry.rejectAll(new Error(`ws-mcp process failed to start: ${err.message}`));
    });

    // Writing to stdin after the child has already died (e.g. a request
    // sent in the same tick as an unrelated crash) surfaces as EPIPE on the
    // stream itself, not just as the write() callback's err argument. The
    // 'error'/'exit' handlers already reject pending calls; this only stops
    // that EPIPE from becoming an unhandled stream exception.
    this.proc.stdin.on("error", (err) => {
      console.error(`[ws-mcp] stdin write error: ${err.message}`);
    });

    this.proc.on("exit", (code, signal) => {
      this.closed = true;
      this.registry.rejectAll(new Error(`ws-mcp process exited unexpectedly (code=${code}, signal=${signal})`));
    });
  }

  private handleMessage(msg: JsonRpcResponse): void {
    if (this.registry.settle(msg)) return;
    console.error(`[ws-mcp] unmatched or unexpected message on stdout: ${JSON.stringify(msg)}`);
  }

  private request<T>(method: string, params?: unknown): Promise<T> {
    if (this.closed) {
      return Promise.reject(new Error(`ws-pi-bridge: client is closed; cannot send "${method}"`));
    }
    return new Promise<T>((resolve, reject) => {
      const id = this.registry.register(resolve as (value: unknown) => void, reject);
      const payload = { jsonrpc: "2.0" as const, id, method, params };
      this.proc.stdin.write(`${JSON.stringify(payload)}\n`, (err) => {
        if (err) {
          this.registry.cancel(id);
          reject(err);
        }
      });
    });
  }

  async initialize(clientInfo: { name: string; version: string }): Promise<McpInitializeResult> {
    return this.request<McpInitializeResult>("initialize", {
      protocolVersion: "2025-03-26",
      capabilities: {},
      clientInfo,
    });
  }

  async listTools(): Promise<McpToolInfo[]> {
    const result = await this.request<{ tools: McpToolInfo[] }>("tools/list", {});
    return result.tools;
  }

  async callTool(name: string, args: Record<string, unknown>): Promise<McpToolCallResult> {
    return this.request<McpToolCallResult>("tools/call", { name, arguments: args });
  }

  /** Idempotent — safe to call more than once (e.g. from a guarded shutdown hook). */
  close(): void {
    if (this.closed) return;
    this.closed = true;
    try {
      this.proc.stdin.end();
    } catch {
      // best-effort
    }
    try {
      this.proc.kill();
    } catch {
      // best-effort
    }
  }
}

export function spawnWsMcpClient(
  launcherPath: string,
  pluginDir: string,
  onStderr?: (line: string) => void,
): McpStdioClient {
  // Mirrors agents-plugin/.mcp.json's launch shape: python3 <launcher>
  // serve --stdio, run with cwd set to the launcher's own plugin directory.
  return new McpStdioClient("python3", [launcherPath, "serve", "--stdio"], {
    cwd: pluginDir,
    onStderr,
  });
}
