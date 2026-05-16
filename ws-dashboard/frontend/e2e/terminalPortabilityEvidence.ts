import type { DashboardBindMode } from "./daemonHarness.js";
import type { TerminalShellProfile } from "../src/terminalCommandPlan.js";

export type TerminalPortabilityEvidence = {
  os: string;
  platform: NodeJS.Platform;
  shellProfile: TerminalShellProfile;
  daemon: {
    mode: "spawned" | "external";
    command?: string[];
    baseUrl: string;
    pairingUrlSource: "scraped" | "provided";
    host?: string;
    port?: number;
    bindMode?: DashboardBindMode;
    staticDir?: string;
  };
  forwarding?: {
    used: boolean;
    kind?: "ssh-local-forward";
    localEndpoint?: string;
    remoteEndpoint?: string;
  };
  readiness: {
    signal: "pairing-url" | "http";
    result: "pass" | "fail";
    detail: string;
  };
  browserGate: {
    result: "pass" | "fail" | "skipped";
    commandProfile: TerminalShellProfile;
    limitations: string[];
  };
};

// CONTRACT: The browser gate must emit ignored machine evidence in this shape
// and the implementation wrap-up must summarize portable evidence in a tracked
// dogfood artifact without committing private host details.
