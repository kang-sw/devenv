import { normalizeServerRouteLocation } from "./routeBasis.js";

function assertEqual(actual: string | null, expected: string | null, label: string) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${expected}, got ${actual}`);
  }
}

function route(pathname: string, search = "", hash = "") {
  return { pathname, search, hash };
}

assertEqual(
  normalizeServerRouteLocation(route("/", "?tab=resources", "#top"), "server-local"),
  "/servers/server-local?tab=resources#top",
  "root normalizes to explicit server route and preserves query/hash",
);
assertEqual(
  normalizeServerRouteLocation(route("/servers"), "server-local"),
  "/servers/server-local",
  "exact servers basis normalizes to selected server",
);
assertEqual(
  normalizeServerRouteLocation(route("/servers/other"), "server-local"),
  "/servers/server-local",
  "wrong server basis normalizes to selected server from resources",
);
assertEqual(
  normalizeServerRouteLocation(route("/servers/server-local/workspaces/workspace-devenv"), "server-local"),
  null,
  "already server-scoped route is preserved",
);
assertEqual(
  normalizeServerRouteLocation(route("/api/dashboard/resources"), "server-local"),
  null,
  "non-browser app route is ignored",
);
