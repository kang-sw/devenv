import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

test("Pi Lead Guide loads", () => {
  const guide = readFileSync(new URL("../pi-lead-guide.md", import.meta.url), "utf8");
  assert.ok(guide.trim());
});
