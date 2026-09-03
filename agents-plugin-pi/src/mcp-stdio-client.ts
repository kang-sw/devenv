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
 */

import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";

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

interface JsonRpcResponse {
  jsonrpc: "2.0";
  id?: number;
  result?: unknown;
  error?: { code: number; message: string; data?: unknown };
}

export class McpStdioClient {
  private readonly proc: ChildProcessWithoutNullStreams;
  private nextId = 1;
  private readonly pending = new Map<number, PendingCall>();
  private stdoutBuffer = "";
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

    this.proc.stdout.on("data", (chunk: Buffer) => this.onStdout(chunk));

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

    this.proc.on("exit", (code, signal) => {
      this.closed = true;
      const err = new Error(`ws-mcp process exited unexpectedly (code=${code}, signal=${signal})`);
      for (const { reject } of this.pending.values()) reject(err);
      this.pending.clear();
    });
  }

  private onStdout(chunk: Buffer): void {
    this.stdoutBuffer += chunk.toString();
    const lines = this.stdoutBuffer.split("\n");
    this.stdoutBuffer = lines.pop() ?? "";
    for (const line of lines) {
      if (!line.trim()) continue;
      let msg: JsonRpcResponse;
      try {
        msg = JSON.parse(line) as JsonRpcResponse;
      } catch {
        console.error(`[ws-mcp] failed to parse JSON-RPC line from stdout: ${line}`);
        continue;
      }
      this.handleMessage(msg);
    }
  }

  private handleMessage(msg: JsonRpcResponse): void {
    if (typeof msg.id === "number" && this.pending.has(msg.id)) {
      const pending = this.pending.get(msg.id)!;
      this.pending.delete(msg.id);
      if (msg.error) {
        pending.reject(new Error(`ws-mcp JSON-RPC error: ${msg.error.message ?? JSON.stringify(msg.error)}`));
      } else {
        pending.resolve(msg.result);
      }
      return;
    }
    console.error(`[ws-mcp] unmatched or unexpected message on stdout: ${JSON.stringify(msg)}`);
  }

  private request<T>(method: string, params?: unknown): Promise<T> {
    if (this.closed) {
      return Promise.reject(new Error(`ws-pi-bridge: client is closed; cannot send "${method}"`));
    }
    const id = this.nextId++;
    const payload = { jsonrpc: "2.0" as const, id, method, params };
    return new Promise<T>((resolve, reject) => {
      this.pending.set(id, { resolve: resolve as (value: unknown) => void, reject });
      this.proc.stdin.write(`${JSON.stringify(payload)}\n`, (err) => {
        if (err) {
          this.pending.delete(id);
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
