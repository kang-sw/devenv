import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

test("Pi Lead Guide maps assigned review to supported wrappers without allocating review", () => {
  const guide = readFileSync(new URL("../pi-lead-guide.md", import.meta.url), "utf8");
  const subtree = guide.split("## Worker subtrees")[1].split("## Subagents park themselves")[0];
  const body = subtree.replace(/\s+/g, " ");
  for (const text of [
    "Nested playbooks must come from that worker's own `ws__playbook_render`",
    "When an assignment requires independent review, use `ws__playbook_render` with `reviewer` for a single full-scope reviewer",
    "`code-review-correctness`, `code-review-fit`, and `code-review-test` for the selected partitions",
    "Do not render `code-reviewer` as a delegated role: it is a shared include, so delegated provenance admission rejects it",
    "This mapping adds no independent review to assignments that do not require it",
    "reviewer findings-file grants follow the existing worker lifecycle guidance",
  ]) {
    assert.ok(body.includes(text), `Worker subtree guidance missing: ${text}`);
  }
});
