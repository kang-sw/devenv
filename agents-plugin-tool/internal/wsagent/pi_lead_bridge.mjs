// servers, sessionKey, launchWarning and workflowManual are JSON constants
// prepended by Go.
// Keep this watchdog independent of the bundle and all npm dependencies.
import { readFileSync, realpathSync } from "node:fs";
import { dirname, join } from "node:path";

// Kept in sync with the package peer contract by pi-lead-health.test.mjs.
export const supportedPiRange = ">=0.84.4 <1.0.0";
export function piVersionWarning(cli = process.argv[1]) {
  let version;
  try {
    for (let dir = dirname(realpathSync(cli)); ; dir = dirname(dir)) {
      let pkg;
      try { pkg = JSON.parse(readFileSync(join(dir, "package.json"), "utf8")); } catch { /* Not a package root. */ }
      if (pkg?.name === "@earendil-works/pi-coding-agent" || pkg?.name === "@mariozechner/pi-coding-agent") {
        version = pkg.version;
        break;
      }
      if (dirname(dir) === dir) break;
    }
  } catch { /* Standalone hosts may not publish npm metadata. */ }
  const match = typeof version === "string" && /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:\+[0-9A-Za-z.-]+)?$/.exec(version);
  if (match && +match[1] === 0 && (+match[2] > 84 || (+match[2] === 84 && +match[3] >= 4))) return "";
  return `[ws Pi] UNSUPPORTED HOST: Pi ${version ?? "version unknown (no host package metadata)"}; supported @earendil-works/pi-coding-agent ${supportedPiRange} (development baseline 0.84.4). Continuing so mail remains reachable, but extension compatibility is unverified. Install a supported Pi version and restart.`;
}

export default function (pi) {
  const fallback = Object.keys(servers ?? {}).length > 0;
  for (const [name, config] of Object.entries(servers ?? {})) pi.registerMcpServer(name, config);
  const versionWarning = piVersionWarning();
  let initialized = false;
  // The native extension emits only at the end of its awaited session_start.
  // The event is process-local: no manifest, protocol or persisted field.
  pi.events.on("ws:lead-initialized", () => { initialized = true; });
  pi.on("session_shutdown", () => { initialized = false; });
  let lastWarning = "";
  function health(ctx, force = false) {
    const tools = new Set(pi.getAllTools().map((tool) => tool.name));
    const missing = fallback ? [] : ["ws-execute", "ws__playbook_read"].filter((name) => !tools.has(name));
    const loadWarning = launchWarning || (!initialized || missing.length ? `[ws Pi] EXTENSION NOT LOADED OR INCOMPLETE: ${missing.length ? "missing native lead tools " + missing.join(", ") : "native lead initialization did not complete; registered tools may be unusable"}. The ws extension failed to initialize; do not assume its tools exist. Dashboard input/mail delivery remains reachable. Repair/reinstall the extension and restart this session.` : "");
    const warning = [loadWarning, versionWarning].filter(Boolean).join("\n\n");
    if (warning && (force || warning !== lastWarning)) ctx.ui.notify(warning, "error");
    lastWarning = warning;
    return warning;
  }
  // Loaded after the real extension, so its awaited session_start has finished.
  pi.on("session_start", (_event, ctx) => health(ctx));
  // Pi writes extension notifications before its public agent_start RPC line.
  // The mapper reapplies this diagnostic after that line clears transient errors.
  pi.on("agent_start", (_event, ctx) => health(ctx, true));
  pi.on("before_agent_start", (event, ctx) => {
    const warning = health(ctx);
    const identity = fallback ? "Your ws session key is " + JSON.stringify(sessionKey) +
      ". Pass it as session_key in ws tool calls. This hosted session already has a key; do not mint a replacement." : "";
    // The fallback stands in for the extension, which appends the manual itself.
    const manual = fallback ? workflowManual : "";
    const extra = [warning, identity, manual].filter(Boolean).join("\n\n");
    return extra ? { systemPrompt: event.systemPrompt + "\n\n" + extra } : undefined;
  });
}
