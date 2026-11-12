/**
 * Key continuity from a hosted session to a terminal resume: a session the
 * dashboard hosts re-logs in with the daemon-minted key and records it as the
 * session's lead key entry, so a later `pi --session <file>` restores that
 * key and re-logs in with it instead of minting (the terminal half is pinned
 * in lead-key-resume.test.ts, "a session the dashboard hosted").
 *
 * Its own file: this process's hosted mode is resolved once and kept, so the
 * hosted variables must be set before the first bootstrap in the process.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { restoreLeadKey } from "../src/bridge.ts";
import { HOSTED_FLAG_ENV, HOSTED_KEY_ENV, HOSTED_ROW_ENV } from "../src/hosted-session.ts";
import { boot, leadKeyEntry } from "./fixtures/lead-key-bridge.ts";

test("a hosted bootstrap re-logs in with the daemon key, mints nothing, and records it as the session's lead key", async () => {
  const row = join(mkdtempSync(join(tmpdir(), "ws-pi-hosted-row-")), "registration.json");
  writeFileSync(row, JSON.stringify({ schema_version: 1, hosted: true, host_pid: process.pid }));
  const saved = { flag: process.env[HOSTED_FLAG_ENV], key: process.env[HOSTED_KEY_ENV], row: process.env[HOSTED_ROW_ENV] };
  Object.assign(process.env, { [HOSTED_FLAG_ENV]: "1", [HOSTED_KEY_ENV]: "hosted-key", [HOSTED_ROW_ENV]: row });
  const sessionFile = "/sessions/hosted/lead.jsonl";
  try {
    const h = await boot({ accept: ["hosted-key"] }, { entries: [], sessionId: "sess-1", sessionFile });
    try {
      assert.equal(h.handle.defaultSessionKeyRef.current, "hosted-key");
      assert.equal(h.mints(), 0);
      assert.deepEqual(h.calls().filter((c) => c.name === "ferrule").map((c) => c.args), [
        { root: h.cwd, format: "json", relogin_session_key: "hosted-key", harness_session: sessionFile, harness_cwd: h.cwd },
      ]);
      assert.deepEqual(h.appended, [leadKeyEntry("sess-1", "hosted-key")]);
      assert.equal(restoreLeadKey(h.appended, "sess-1"), "hosted-key", "a terminal resume of this session file restores the hosted key");
    } finally { h.handle.shutdown(); h.restore(); }
  } finally {
    for (const [name, value] of [[HOSTED_FLAG_ENV, saved.flag], [HOSTED_KEY_ENV, saved.key], [HOSTED_ROW_ENV, saved.row]] as const) {
      if (value === undefined) delete process.env[name]; else process.env[name] = value;
    }
  }
});
