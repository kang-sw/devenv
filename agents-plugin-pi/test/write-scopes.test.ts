import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, test } from "node:test";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import {
  authorizeWritePath,
  canDelegateWriteCapability,
  matchesWriteScopeInclude,
  normalizeWriteScopes,
  parseEffectiveWriteCapability,
  registerScopedWriteTools,
  validateWriteScopePattern,
  type EffectiveWriteCapability,
} from "../src/write-scopes.ts";

const roots: string[] = [];
function tempRoot(): string {
  const root = mkdtempSync(join(tmpdir(), "ws-pi-write-scope-"));
  roots.push(root);
  return root;
}
afterEach(() => {
  for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true });
});

describe("write-scope glob contract", () => {
  const fixture = JSON.parse(readFileSync(new URL("./fixtures/write-scope-globs.json", import.meta.url), "utf8")) as {
    cases: Array<{ pattern: string; candidate: string; matches: boolean }>;
  };

  test("committed fixture matches Node path.posix.matchesGlob semantics", () => {
    for (const entry of fixture.cases) {
      validateWriteScopePattern(entry.pattern);
      assert.equal(matchesWriteScopeInclude(entry.candidate, [entry.pattern]), entry.matches, JSON.stringify(entry));
    }
  });

  test("rejects syntax outside the deliberately narrow positive subset", () => {
    for (const pattern of [
      "", "/absolute", "trailing/", "double//segment", ".", "..", "a/../b", "a/./b",
      "!secret.md", "[!a].md", "[^a].md", "{a,b}.md", "@(a|b).md", "+(a|b).md",
      "?(a).md", "*(a).md", "!(a).md", "a/**b/c", "a/b**/c", "[z-a].md", "unterminated[", "dangling\\", "nul\0byte",
    ]) {
      assert.throws(() => validateWriteScopePattern(pattern), /write scope include pattern/);
    }
  });

  test("persisted capabilities reject contradictory or extra authority fields", () => {
    for (const malformed of [
      null,
      { mode: "unrestricted", scopes: [] },
      { mode: "none", scopes: [{ path: "/tmp", kind: "tree" }] },
      { mode: "scoped", scopes: [{ path: "/tmp", kind: "file", include: ["*.md"] }] },
    ]) assert.throws(() => parseEffectiveWriteCapability(malformed), /invalid write scope/);
  });
});

describe("scope normalization and authorization", () => {
  test("binds existing and future exact files to their canonical parent", () => {
    const root = tempRoot();
    const existing = join(root, "existing.md");
    const future = join(root, "future.md");
    writeFileSync(existing, "old");
    const capability = normalizeWriteScopes([
      { path: existing, kind: "file" },
      { path: future, kind: "file" },
    ]);
    assert.equal(capability.mode, "scoped");
    assert.doesNotThrow(() => authorizeWritePath(capability, existing));
    assert.doesNotThrow(() => authorizeWritePath(capability, future));
    assert.throws(() => authorizeWritePath(capability, join(root, "other.md")), /outside delegated write scopes/);
  });

  test("tree grants authorize descendants, recursive globs, and future matching files dynamically", () => {
    const root = tempRoot();
    mkdirSync(join(root, "reports"));
    const all = normalizeWriteScopes([{ path: root, kind: "tree" }]);
    assert.doesNotThrow(() => authorizeWritePath(all, join(root, "new", "deep.txt")));

    const markdown = normalizeWriteScopes([{ path: root, kind: "tree", include: ["**/*.md"] }]);
    assert.doesNotThrow(() => authorizeWritePath(markdown, join(root, "future", "report.md")));
    assert.throws(() => authorizeWritePath(markdown, join(root, "future", "report.txt")), /outside delegated write scopes/);

    rmSync(root, { recursive: true });
    assert.throws(() => authorizeWritePath(all, join(root, "recreated", "file.txt")), /outside delegated write scopes/);
  });

  test("rejects relative grants, absent file parents, and absent tree roots", () => {
    const root = tempRoot();
    assert.throws(() => normalizeWriteScopes([{ path: "relative.md", kind: "file" }]), /absolute/);
    assert.throws(() => normalizeWriteScopes([{ path: join(root, "missing", "file.md"), kind: "file" }]), /parent must exist/);
    assert.throws(() => normalizeWriteScopes([{ path: join(root, "missing"), kind: "tree" }]), /tree root must exist/);
  });

  test("rejects lexical traversal and canonical symlink escapes without exposing inventory", () => {
    const root = tempRoot();
    const outside = tempRoot();
    mkdirSync(join(root, "safe"));
    symlinkSync(outside, join(root, "safe", "escape"));
    const capability = normalizeWriteScopes([{ path: join(root, "safe"), kind: "tree" }]);
    for (const candidate of [join(root, "safe", "..", "outside.md"), join(root, "safe", "escape", "owned.md")]) {
      assert.throws(
        () => authorizeWritePath(capability, candidate),
        (error: unknown) => error instanceof Error && error.message === "ws-pi-agent: write path is outside delegated write scopes",
      );
    }
  });
});

describe("monotonic delegation", () => {
  const none: EffectiveWriteCapability = { mode: "none" };
  const unrestricted: EffectiveWriteCapability = { mode: "unrestricted" };

  test("unrestricted may narrow, read-only may not grant, and unrestricted children cannot descend from scoped parents", () => {
    const root = tempRoot();
    const scoped = normalizeWriteScopes([{ path: root, kind: "tree" }]);
    assert.equal(canDelegateWriteCapability(unrestricted, scoped), true);
    assert.equal(canDelegateWriteCapability(none, scoped), false);
    assert.equal(canDelegateWriteCapability(scoped, unrestricted), false);
  });

  test("scoped parents may grant exact or conservatively provable narrower scopes and refuse ambiguous glob subsets", () => {
    const root = tempRoot();
    mkdirSync(join(root, "reports"));
    const parentAll = normalizeWriteScopes([{ path: root, kind: "tree" }]);
    const childFile = normalizeWriteScopes([{ path: join(root, "reports", "one.md"), kind: "file" }]);
    const childTree = normalizeWriteScopes([{ path: join(root, "reports"), kind: "tree", include: ["**/*.md"] }]);
    assert.equal(canDelegateWriteCapability(parentAll, childFile), true);
    assert.equal(canDelegateWriteCapability(parentAll, childTree), true);

    const parentGlob = normalizeWriteScopes([{ path: root, kind: "tree", include: ["reports/**/*.md"] }]);
    const exactPattern = normalizeWriteScopes([{ path: join(root, "reports"), kind: "tree", include: ["**/*.md"] }]);
    const ambiguousPattern = normalizeWriteScopes([{ path: join(root, "reports"), kind: "tree", include: ["2026/**/*.md"] }]);
    assert.equal(canDelegateWriteCapability(parentGlob, exactPattern), true);
    assert.equal(canDelegateWriteCapability(parentGlob, ambiguousPattern), false);
  });
});

describe("native scoped edit/write wrappers", () => {
  test("exposes only edit/write wrappers and preserves native create/replace/edit behavior", async () => {
    const root = tempRoot();
    const target = join(root, "result.md");
    const capability = normalizeWriteScopes([{ path: target, kind: "file" }]);
    const tools = new Map<string, any>();
    const pi = { registerTool: (tool: any) => tools.set(tool.name, tool) } as ExtensionAPI;
    registerScopedWriteTools(pi, capability);
    assert.deepEqual([...tools.keys()], ["edit", "write"]);
    assert.equal(tools.has("bash"), false);
    assert.equal(tools.has("delete"), false);
    assert.equal(tools.has("rename"), false);

    const ctx = { cwd: root };
    const writeResult = await tools.get("write").execute("w", { path: target, content: "before\n" }, undefined, undefined, ctx);
    assert.match(writeResult.content[0].text, /Successfully wrote/);
    const editResult = await tools.get("edit").execute("e", { path: target, edits: [{ oldText: "before", newText: "after" }] }, undefined, undefined, ctx);
    assert.match(editResult.content[0].text, /Successfully replaced/);
    assert.equal(readFileSync(target, "utf8"), "after\n");
    await assert.rejects(
      tools.get("write").execute("bad", { path: join(root, "other.md"), content: "no" }, undefined, undefined, ctx),
      /outside delegated write scopes/,
    );
  });
});
