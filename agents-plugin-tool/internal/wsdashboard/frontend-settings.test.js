const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

function read(name) {
  return fs.readFileSync(path.join(__dirname, "assets", name), "utf8");
}

const origin = "http://localhost:19544";
const context = vm.createContext({window: {}, URLSearchParams});
vm.runInContext(read("shell.js"), context);
const shell = context.window.WsShell;

test("settings links and frames keep the exact inspection identifiers", () => {
  const origins = [
    {scope: "global", target: "", context: {worktree: "", session: ""}},
    {scope: "project", target: "project-a", context: {worktree: "", session: ""}},
    {scope: "repo", target: "tree-a", context: {worktree: "tree-a", session: ""}},
    {scope: "session", target: "session-a", context: {worktree: "tree-a", session: "session-a"}}
  ];
  for (const selection of origins) {
    const href = shell.settingsScopeHref("builtin", selection, () => "");
    const frame = shell.framed(new URL(href, origin), origin);
    const src = new URL(shell.src(frame), origin);
    assert.equal(src.pathname, "/settings");
    assert.equal(src.searchParams.get("scope"), "builtin");
    assert.equal(src.searchParams.get("target"), null);
    assert.equal(src.searchParams.get("worktree"), selection.context.worktree || null);
    assert.equal(src.searchParams.get("session"), selection.context.session || null);
    assert.equal(src.searchParams.get("frame"), "1");
    assert.equal(shell.pagePath(src) + src.hash, href);
    assert.equal(shell.framed(src, origin).path, frame.path);
    assert.equal(frame.rid, "");
  }
});

test("scope navigation requires an exact target and never chooses a sibling", () => {
  const projectFor = tree => tree === "tree-a" ? "project-a" : "";
  let selection = {scope: "builtin", target: "", context: {worktree: "tree-a", session: "session-a"}};
  assert.equal(shell.settingsScopeHref("project", selection, projectFor),
    "/settings?scope=project&target=project-a&worktree=tree-a&session=session-a");
  assert.equal(shell.settingsScopeHref("repo", selection, projectFor),
    "/settings?scope=repo&target=tree-a&worktree=tree-a&session=session-a");
  assert.equal(shell.settingsScopeHref("session", selection, projectFor),
    "/settings?scope=session&target=session-a&worktree=tree-a&session=session-a");
  selection.context = {worktree: "stale", session: ""};
  assert.equal(shell.settingsScopeHref("project", selection, projectFor), "");
  assert.equal(shell.settingsScopeHref("session", selection, projectFor), "");
  assert.equal(shell.settingsScopeHref("global", selection, projectFor), "/settings?scope=global&worktree=stale");
  selection.context = {worktree: "", session: ""};
  assert.equal(shell.settingsScopeHref("repo", selection, projectFor), "");
});

test("settings distinguishes an older daemon from a missing target and never posts on load", async () => {
  const calls = [];
  const replies = [
    {status: 404, ok: false, json: async () => { throw new Error("text response"); }},
    {status: 404, ok: false, json: async () => ({error: {code: "target_not_found"}})},
    {status: 400, ok: false, json: async () => ({error: {code: "invalid_context"}})}
  ];
  const source = read("dashboard-settings.js");
  const method = source.match(/\n  async function request\(path, body\) \{[\s\S]*?\n  \}/)[0];
  const load = vm.runInNewContext(method + "\nrequest", {
    fetch: async (url, options) => {
      calls.push([url, options.method]);
      return replies.shift();
    }
  });
  for (const code of ["upgrade_required", "target_not_found", "invalid_context"]) {
    await assert.rejects(load("/api/config/snapshot"), {code});
  }
  assert.deepEqual(calls, Array(3).fill(["/api/config/snapshot", undefined]));
});

test("rail gears preserve native activation and separate it from row actions", () => {
  const markup = read("dashboard.tmpl");
  assert.match(markup, /<button type="button" class="r-config-gear"[^>]*data-scope-entry=/);
  assert.match(markup, /<a class="settings-page-link" href="\/settings">/);
  const rail = read("dashboard-rail.js");
  assert.match(rail, /gear && root.hasAttribute\("data-can-control"\)\) \{\s+e.preventDefault\(\);\s+e.stopPropagation\(\);/);
  const css = read("dashboard-rail.css");
  assert.match(css, /\.r-config-gear \{[^}]*opacity: \.32;/);
  assert.match(css, /\.r-config-gear:focus-visible,\n[^{}]+\.r-config-gear\[aria-current="page"\] \{ opacity: 1; \}/);
});

test("project rail target survives standalone scope traversal without becoming inspection context", () => {
  const context = {worktree: "", session: ""};
  const project = {scope: "project", target: "project-a", context};
  const unbound = {scope: "builtin", target: "", context};
  const builtin = shell.settingsScopeHref("builtin", project, () => "");
  assert.equal(builtin, "/settings?scope=builtin#project=project-a");
  const remembered = new URLSearchParams(new URL(builtin, origin).hash.slice(1)).get("project");
  assert.equal(shell.settingsScopeHref("global", unbound, () => "", remembered), "/settings?scope=global#project=project-a");
  assert.equal(shell.settingsScopeHref("project", unbound, () => "", remembered), "/settings?scope=project&target=project-a");
});
