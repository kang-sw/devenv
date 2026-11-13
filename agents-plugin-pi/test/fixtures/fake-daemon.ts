/**
 * A loopback stand-in for the dashboard daemon's side of the launcher
 * transport: `GET /health`, then the `/mcp` HTTP Upgrade, the hello line and
 * its accept line, then CLI frames (`internal/wsdashboard/proxy.go` is the
 * real side). It writes the endpoint file the Python launcher discovers under
 * `<cache root>/dashboard/endpoint.json`, so a launcher run with
 * `WS_CACHE_HOME` pointed at that root talks to it and to nothing else.
 *
 * Tests drive each CLI connection by hand: live `stdout` frames for a
 * streaming wait, one `final` frame with an exit code for a one-shot command,
 * and `closed` for the launcher going away (an abort).
 *
 * `node --test` also runs this file (default glob); it defines no tests.
 */
import { createServer, type Socket } from "node:net";
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

export interface FakeDaemonHello {
  protocol: number;
  mode: string;
  argv: string[];
  cwd: string;
  env: Record<string, string>;
  owner_pid: number;
  version: string;
}

export interface FakeCliConnection {
  readonly hello: FakeDaemonHello;
  /** One live stdout frame, as a streaming command sends each arrival. */
  stdout(data: string): void;
  /** The final frame: buffered stdout and the exit code the launcher exits with. */
  final(exitCode: number, data?: string): void;
  /** Settles when the launcher side of the connection closes. */
  readonly closed: Promise<void>;
}

export interface FakeDaemon {
  readonly port: number;
  /** Every accepted hello, in arrival order (CLI and MCP). */
  readonly hellos: FakeDaemonHello[];
  /** `GET /health` requests answered. */
  healthCount(): number;
  /** The next CLI connection, in arrival order; rejects after `timeoutMs`. */
  nextCli(timeoutMs?: number): Promise<FakeCliConnection>;
  close(): Promise<void>;
}

const TOKEN = "f".repeat(64);
const PID = 4242;

export async function startFakeDaemon(cacheRoot: string, options: { version: string; buildTime?: number }): Promise<FakeDaemon> {
  const hellos: FakeDaemonHello[] = [];
  const queued: FakeCliConnection[] = [];
  const waiting: ((connection: FakeCliConnection) => void)[] = [];
  const sockets = new Set<Socket>();
  let health = 0;
  // A build newer than any runtime the test installs: the launcher never sees a reason to replace it.
  const buildTime = options.buildTime ?? 4_000_000_000_000;

  const offer = (connection: FakeCliConnection): void => {
    const next = waiting.shift();
    if (next) next(connection);
    else queued.push(connection);
  };

  const server = createServer((socket) => {
    sockets.add(socket);
    const closed = new Promise<void>((resolve) => socket.once("close", () => { sockets.delete(socket); resolve(); }));
    socket.on("error", () => { /* The launcher may vanish mid-frame (an abort). */ });
    let buffer = Buffer.alloc(0);
    let upgraded = false;
    const onData = (chunk: Buffer): void => {
      buffer = Buffer.concat([buffer, chunk]);
      if (!upgraded) {
        const end = buffer.indexOf("\r\n\r\n");
        if (end < 0) return;
        const head = buffer.subarray(0, end).toString("latin1").split("\r\n");
        buffer = buffer.subarray(end + 4);
        const path = head[0]?.split(" ")[1] ?? "";
        const headers = new Map(head.slice(1).map((line) => {
          const colon = line.indexOf(":");
          return [line.slice(0, colon).trim().toLowerCase(), line.slice(colon + 1).trim()] as const;
        }));
        if (path === "/health") {
          health += 1;
          const body = JSON.stringify({ service: "ws-dashboard", version: options.version, pid: PID, port: address(), started_at: "x", lan: false, build_time: buildTime });
          socket.end(`HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: ${Buffer.byteLength(body)}\r\nConnection: close\r\n\r\n${body}`);
          return;
        }
        if (path !== "/mcp" || headers.get("authorization") !== `Bearer ${TOKEN}` || headers.get("upgrade") !== "ws-mcp") {
          socket.end("HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n");
          return;
        }
        upgraded = true;
        socket.write("HTTP/1.1 101 Switching Protocols\r\nUpgrade: ws-mcp\r\nConnection: Upgrade\r\n\r\n");
      }
      const newline = buffer.indexOf("\n");
      if (newline < 0) return;
      socket.off("data", onData);
      const hello = JSON.parse(buffer.subarray(0, newline).toString("utf8")) as FakeDaemonHello;
      hellos.push(hello);
      socket.write(`${JSON.stringify({ ok: true, version: options.version, pid: PID })}\n`);
      if (hello.mode !== "cli") {
        socket.end();
        return;
      }
      const send = (frame: Record<string, unknown>): void => { if (!socket.destroyed) socket.write(`${JSON.stringify(frame)}\n`); };
      offer({
        hello,
        stdout: (data) => send({ stream: "stdout", data }),
        final: (exitCode, data = "") => { send({ final: true, exit_code: exitCode, data }); socket.end(); },
        closed,
      });
    };
    socket.on("data", onData);
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = (): number => (server.address() as { port: number }).port;

  const daemonDir = join(cacheRoot, "dashboard");
  mkdirSync(daemonDir, { recursive: true });
  writeFileSync(join(daemonDir, "endpoint.json"), JSON.stringify({ port: address(), pid: PID, version: options.version, token: TOKEN, started_at: "2026-11-14T00:00:00Z" }));

  return {
    port: address(),
    hellos,
    healthCount: () => health,
    nextCli: (timeoutMs = 15_000) => {
      const ready = queued.shift();
      if (ready) return Promise.resolve(ready);
      return new Promise((resolve, reject) => {
        const timer = setTimeout(() => {
          waiting.splice(waiting.indexOf(accept), 1);
          reject(new Error(`no CLI connection reached the fake daemon within ${timeoutMs}ms`));
        }, timeoutMs);
        const accept = (connection: FakeCliConnection): void => { clearTimeout(timer); resolve(connection); };
        waiting.push(accept);
      });
    },
    close: async () => {
      for (const socket of sockets) socket.destroy();
      await new Promise<void>((resolve) => server.close(() => resolve()));
    },
  };
}
