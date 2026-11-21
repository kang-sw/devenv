import { createHash } from "node:crypto";
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { createInterface } from "node:readline";

const words = fs.readFileSync(new URL("../agents-plugin-tool/internal/wskey/eff_large_wordlist.txt", import.meta.url), "utf8")
  .split("\n").map((word) => word.trim()).filter(Boolean);

// Same four EFF words as wsmailbox.ReplyID.
export function fixtureReplyID(key) {
  let out = "";
  for (let i = 0; i < 4; i++) {
    const digest = createHash("sha256").update(`${key}#${i}`).digest();
    out += words[Number(digest.readBigUInt64BE(0) % BigInt(words.length))];
  }
  return out;
}

function gitPath() {
  const exe = process.platform === "win32" ? "git.exe" : "git";
  for (const dir of (process.env.PATH || "").split(path.delimiter)) {
    if (!path.isAbsolute(dir)) continue;
    try {
      const candidate = path.join(dir, exe);
      fs.accessSync(candidate, fs.constants.X_OK);
      if (fs.statSync(candidate).isFile()) return dir;
    } catch { /* try the next PATH entry */ }
  }
  throw new Error(`no ${exe} on an absolute PATH entry for the capture fixture`);
}

// The daemon and fixture commands use an allowlist. The build and Playwright
// resolve their toolchain and browser caches in the runner's environment.
export function scratchEnv(root) {
  const env = {
    HOME: path.join(root, "home"),
    TMPDIR: path.join(root, "tmp"),
    XDG_CACHE_HOME: path.join(root, "xdg", "cache"),
    XDG_CONFIG_HOME: path.join(root, "xdg", "config"),
    XDG_DATA_HOME: path.join(root, "xdg", "data"),
    XDG_STATE_HOME: path.join(root, "xdg", "state"),
    XDG_RUNTIME_DIR: path.join(root, "xdg", "run"),
    WS_CACHE_HOME: path.join(root, "cache"),
    WS_CONFIG_HOME: path.join(root, "config"),
  };
  if (process.platform === "win32") {
    env.USERPROFILE = env.HOME;
    env.APPDATA = path.join(root, "AppData", "Roaming");
    env.LOCALAPPDATA = path.join(root, "AppData", "Local");
  }
  for (const dir of Object.values(env)) fs.mkdirSync(dir, { recursive: true, mode: 0o700 });
  return {
    ...env,
    PATH: gitPath(),
    TMP: env.TMPDIR,
    TEMP: env.TMPDIR,
    XDG_CONFIG_DIRS: env.XDG_CONFIG_HOME,
    XDG_DATA_DIRS: env.XDG_DATA_HOME,
    GIT_CONFIG_NOSYSTEM: "1",
    WS_MCP_DASHBOARD_SCRATCH_CAPTURE: "1",
    ...(process.platform === "win32" ? { SystemRoot: process.env.SystemRoot, PATHEXT: ".EXE" } : {}),
  };
}

export async function scratchCookie({ port, token }) {
  const response = await fetch(`http://127.0.0.1:${port}/auth/cookie`, {
    method: "POST", headers: { Authorization: `Bearer ${token}` },
  });
  if (!response.ok) throw new Error(`scratch daemon cookie: HTTP ${response.status}`);
  const { name, value } = await response.json();
  return { name, value, httpOnly: true, sameSite: "Strict" };
}

// Seed fixture boards through their owner, the scratch daemon. Use the same
// hosted CLI connection as the launcher without starting a second ws-mcp.
export function fixtureCLI({ port, token }, cwd, env) {
  return (tool, args) => new Promise((resolve, reject) => {
    const req = http.get(`http://127.0.0.1:${port}/mcp`, {
      headers: { Authorization: `Bearer ${token}`, Connection: "Upgrade", Upgrade: "ws-mcp" },
    });
    req.setTimeout(15000, () => req.destroy(new Error(`fixture ${tool}: handshake timed out`)));
    req.on("error", reject);
    req.on("response", (res) => {
      res.resume();
      reject(new Error(`fixture ${tool}: dashboard MCP returned HTTP ${res.statusCode}`));
    });
    req.on("upgrade", async (_res, socket, head) => {
      try {
        if (head.length) socket.unshift(head);
        socket.setTimeout(15000, () => socket.destroy(new Error(`fixture ${tool}: response timed out`)));
        const hello = { protocol: 1, mode: "cli", cwd, env, owner_pid: process.pid, argv: ["call", tool, JSON.stringify(args)] };
        socket.write(JSON.stringify(hello) + "\n");
        let accepted = false, stderr = "";
        for await (const line of createInterface({ input: socket })) {
          const frame = JSON.parse(line);
          if (!accepted) {
            if (!frame.ok) throw new Error(`fixture ${tool}: ${frame.reason}`);
            accepted = true;
          } else if (frame.final) {
            if (frame.exit_code !== 0) throw new Error(`fixture ${tool}: ${stderr || frame.data}`);
            resolve(frame.data);
            return;
          } else if (frame.stream === "stderr") {
            stderr += frame.data;
          }
        }
        throw new Error(`fixture ${tool}: dashboard MCP closed before completing the call`);
      } catch (err) {
        reject(err);
      } finally {
        socket.destroy();
      }
    });
  });
}
