/**
 * Lead session key persistence across resume, and adoption of the key a lead
 * revives with (261007, bridge.ts "Lead key persistence").
 *
 * The production `startBridge` runs against the fake ws-mcp launcher of
 * fixtures/lead-key-bridge.ts, which journals every tools/call so a test can
 * assert which keys were re-logged in, minted, or forwarded. The trigger for reuse is the
 * `ws-pi-lead-key` session entry, not `SessionStartEvent.reason`: a reopened
 * file (`pi -c`, `-r`, `--session`) starts with `reason: "startup"`,
 * `/reload` re-runs the bootstrap in the same session, and an in-process
 * switch fires `resume` - `startBridge` sees none of them, only the entries
 * and the session id, so one bootstrap covers all three.
 */
import { test, describe } from "node:test";
import assert from "node:assert/strict";
import {
  HOSTED_SESSION_HELD_MARKER,
  LEAD_KEY_ENTRY,
  buildDefaultKeyChangedLine,
  harnessSessionArgs,
  reloginLeadKey,
  restoreLeadKey,
  revivedKeyToAdopt,
} from "../src/bridge.ts";
import { ADAPTER_MESSAGE_LABEL } from "../src/adapter-label.ts";
import { buildLeadCompactionSummary, renderLeadProse } from "../src/lead-compaction.ts";
import type { McpToolCallResult } from "../src/mcp-stdio-client.ts";
import { boot, heldRefusal, leadKeyEntry, type Call, type Harness } from "./fixtures/lead-key-bridge.ts";

const SENTINEL = "obsidian-latch";

const ctx = { sessionManager: { buildContextEntries: () => [] } };
const texts = (content: any[]): string[] => content.filter((item) => item.type === "text").map((item) => item.text as string);

describe("restoreLeadKey", () => {
  test("returns the newest entry recorded under the session id and ignores other sessions and malformed entries", () => {
    const entries = [
      leadKeyEntry("s1", "older"),
      { type: "custom", customType: LEAD_KEY_ENTRY, data: { sessionId: "s1" } },
      leadKeyEntry("s2", "foreign"),
      { type: "custom", customType: "ws-pi-fork-keys", data: { sessionId: "s1", current: "fork" } },
      leadKeyEntry("s1", "newest"),
      null,
    ];
    assert.equal(restoreLeadKey(entries, "s1"), "newest");
    assert.equal(restoreLeadKey(entries, "s2"), "foreign");
    assert.equal(restoreLeadKey(entries, "s3"), undefined);
    assert.equal(restoreLeadKey(entries, undefined), undefined);
    assert.equal(restoreLeadKey([], "s1"), undefined);
  });

  test("a newest entry with a blank key yields nothing rather than an older key", () => {
    assert.equal(restoreLeadKey([leadKeyEntry("s1", "older"), leadKeyEntry("s1", "  ")], "s1"), undefined);
  });
});

describe("revivedKeyToAdopt", () => {
  test("an explicit key other than the default is adopted; the sentinel, the default, blanks, and non-strings are not", () => {
    assert.equal(revivedKeyToAdopt("revived", "own", SENTINEL), "revived");
    assert.equal(revivedKeyToAdopt(" revived ", "own", SENTINEL), "revived");
    assert.equal(revivedKeyToAdopt("revived", undefined, SENTINEL), "revived", "a degraded bootstrap adopts the revived key");
    assert.equal(revivedKeyToAdopt("own", "own", SENTINEL), undefined);
    assert.equal(revivedKeyToAdopt(SENTINEL, "own", SENTINEL), undefined);
    assert.equal(revivedKeyToAdopt(SENTINEL, undefined, SENTINEL), undefined);
    assert.equal(revivedKeyToAdopt("", "own", SENTINEL), undefined);
    assert.equal(revivedKeyToAdopt(undefined, "own", SENTINEL), undefined);
    assert.equal(revivedKeyToAdopt(42, "own", SENTINEL), undefined);
  });
});

describe("buildDefaultKeyChangedLine", () => {
  test("opens with the shared adapter label line and names both keys", () => {
    const line = buildDefaultKeyChangedLine("old-key", "new-key");
    assert.equal(line, `${ADAPTER_MESSAGE_LABEL}\nDefault session key changed: old-key -> new-key. Calls that omit session_key and compaction summaries now use new-key.`);
    assert.match(buildDefaultKeyChangedLine(undefined, "new-key"), /changed: \(unset\) -> new-key\./);
  });
});

describe("reloginLeadKey", () => {
  const ok = (text: string): McpToolCallResult => ({ isError: false, content: [{ type: "text", text }] });
  test("ok only when ws-mcp returns the same key; a refusal, a thrown call, or another key is not ok", async () => {
    const seen: Array<[string, Record<string, unknown>]> = [];
    const same = await reloginLeadKey(async (name, args) => { seen.push([name, args]); return ok(JSON.stringify({ session_key: "k" })); }, "/root", "k");
    assert.equal(same.ok, true);
    assert.deepEqual(seen, [["ferrule", { root: "/root", format: "json", relogin_session_key: "k" }]]);
    const refused = await reloginLeadKey(async () => ({ isError: true, content: [{ type: "text", text: "nope" }] }), "/root", "k");
    assert.deepEqual(refused, { ok: false, reason: "nope", held: false });
    const threw = await reloginLeadKey(async () => { throw new Error("transport down"); }, "/root", "k");
    assert.deepEqual(threw, { ok: false, reason: "transport down", held: false });
    const other = await reloginLeadKey(async () => ok(JSON.stringify({ session_key: "z" })), "/root", "k");
    assert.equal(other.ok, false);
    assert.match((other as { reason: string }).reason, /returned z instead of k/);
    assert.equal((other as { held: boolean }).held, false);
  });

  test("a refusal carrying the hosted-session marker is held, from an error result or a thrown call", async () => {
    const text = heldRefusal("k");
    assert.ok(text.includes(HOSTED_SESSION_HELD_MARKER));
    assert.deepEqual(await reloginLeadKey(async () => ({ isError: true, content: [{ type: "text", text }] }), "/root", "k"), { ok: false, reason: text, held: true });
    assert.deepEqual(await reloginLeadKey(async () => { throw new Error(text); }, "/root", "k"), { ok: false, reason: text, held: true });
  });

  test("the harness report rides along with the re-login arguments", async () => {
    const seen: Array<Record<string, unknown>> = [];
    await reloginLeadKey(async (_name, args) => { seen.push(args); return ok(JSON.stringify({ session_key: "k" })); }, "/root", "k", { harness_session: "/s/a.jsonl", harness_cwd: "/root/sub" });
    assert.deepEqual(seen, [{ root: "/root", format: "json", relogin_session_key: "k", harness_session: "/s/a.jsonl", harness_cwd: "/root/sub" }]);
  });
});

describe("harnessSessionArgs", () => {
  test("both fields with a session file and an absolute directory, neither otherwise", () => {
    assert.deepEqual(harnessSessionArgs("/s/a.jsonl", "/work/app"), { harness_session: "/s/a.jsonl", harness_cwd: "/work/app" });
    assert.deepEqual(harnessSessionArgs(undefined, "/work/app"), {});
    assert.deepEqual(harnessSessionArgs("", "/work/app"), {});
    assert.deepEqual(harnessSessionArgs("/s/a.jsonl", "work/app"), {});
  });
});

describe("lead key persistence at bootstrap", () => {
  test("reuses the key recorded under the same session id with no mint, and records nothing new (startup, reload, and resume alike)", async () => {
    const h = await boot({ accept: ["original-key"] }, { entries: [leadKeyEntry("sess-1", "original-key")], sessionId: "sess-1" });
    try {
      assert.equal(h.handle.defaultSessionKeyRef.current, "original-key");
      assert.deepEqual(h.relogins(), ["original-key"]);
      assert.equal(h.mints(), 0);
      assert.deepEqual(h.appended, [], "the newest entry already holds this key");
      const manual = h.calls().find((c) => c.name === "workflow_manual");
      assert.equal(manual?.args.session_key, "original-key", "the session-start snapshot fetch runs under the reused key");
      assert.deepEqual(h.notices.filter((n) => n.level === "warning"), []);
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("the newest entry for the session wins over an older one", async () => {
    const h = await boot({ accept: ["older-key", "newer-key"] }, { entries: [leadKeyEntry("sess-1", "older-key"), leadKeyEntry("sess-1", "newer-key")], sessionId: "sess-1" });
    try {
      assert.equal(h.handle.defaultSessionKeyRef.current, "newer-key");
      assert.deepEqual(h.relogins(), ["newer-key"]);
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("mints and records when no entry matches: a new session, and a fork whose file carries the lead's entry under the lead's session id", async () => {
    const fresh = await boot({ accept: ["lead-key"] }, { entries: [], sessionId: "new-sess" });
    try {
      assert.equal(fresh.handle.defaultSessionKeyRef.current, "minted-1");
      assert.deepEqual(fresh.relogins(), []);
      assert.equal(fresh.mints(), 1);
      assert.deepEqual(fresh.appended, [{ type: "custom", customType: LEAD_KEY_ENTRY, data: { sessionId: "new-sess", key: "minted-1" } }]);
    } finally { fresh.handle.shutdown(); fresh.restore(); }

    const fork = await boot({ accept: ["lead-key"] }, { entries: [leadKeyEntry("lead-sess", "lead-key")], sessionId: "fork-sess" });
    try {
      assert.equal(fork.handle.defaultSessionKeyRef.current, "minted-1");
      assert.deepEqual(fork.relogins(), [], "the lead's key is accepted by ws-mcp but never tried: the session id does not match");
      assert.deepEqual(fork.appended, [{ type: "custom", customType: LEAD_KEY_ENTRY, data: { sessionId: "fork-sess", key: "minted-1" } }]);
    } finally { fork.handle.shutdown(); fork.restore(); }
  });

  test("a refused recorded key falls back to a fresh mint with a UI warning only, and the fresh key is recorded", async () => {
    const h = await boot({ accept: [] }, { entries: [leadKeyEntry("sess-1", "stale-key")], sessionId: "sess-1" });
    try {
      assert.equal(h.handle.defaultSessionKeyRef.current, "minted-1");
      assert.deepEqual(h.relogins(), ["stale-key"]);
      assert.equal(h.mints(), 1);
      const warnings = h.notices.filter((n) => n.level === "warning");
      assert.equal(warnings.length, 1);
      assert.match(warnings[0]!.message, /recorded ws key stale-key was refused \(.*not a known session key.*\); minting a fresh key/);
      assert.deepEqual(h.appended, [{ type: "custom", customType: LEAD_KEY_ENTRY, data: { sessionId: "sess-1", key: "minted-1" } }]);
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("without a session id nothing is reused or recorded", async () => {
    const h = await boot({ accept: ["original-key"] }, { entries: [leadKeyEntry("sess-1", "original-key")], sessionId: undefined });
    try {
      assert.equal(h.handle.defaultSessionKeyRef.current, "minted-1");
      assert.deepEqual(h.relogins(), []);
      assert.deepEqual(h.appended, []);
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("a child process keeps its policy key and never consults the entries", async () => {
    const policy = { version: 1, depth: 1, maxDepth: 2, tools: ["ws__workflow_manual"], authority: "lead", sessionKey: "child-key", parentSessionKey: "lead-key" };
    const h = await boot({ accept: ["original-key"] }, { entries: [leadKeyEntry("sess-1", "original-key")], sessionId: "sess-1", policy });
    try {
      assert.equal(h.handle.defaultSessionKeyRef.current, "child-key");
      assert.deepEqual(h.calls().filter((c) => c.name === "ferrule"), [], "a policy key needs no ferrule at all");
      assert.deepEqual(h.appended, []);
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("a child process without a policy key mints under its policy and never re-logs in", async () => {
    const policy = { version: 1, depth: 1, maxDepth: 2, tools: ["ws__workflow_manual"], authority: "delegate", parentSessionKey: "lead-key" };
    const h = await boot({ accept: ["original-key"] }, { entries: [leadKeyEntry("sess-1", "original-key")], sessionId: "sess-1", policy });
    try {
      assert.equal(h.handle.defaultSessionKeyRef.current, "minted-1");
      assert.deepEqual(h.relogins(), []);
      const mint = h.calls().find((c) => c.name === "ferrule");
      assert.equal(mint?.args.capability, "delegate");
      assert.equal(mint?.args.parent_session_key, "lead-key");
      assert.deepEqual(h.appended, []);
    } finally { h.handle.shutdown(); h.restore(); }
  });
});

const SESSION_FILE = "/sessions/2026/lead-session-1.jsonl";
const ferruleArgs = (calls: Call[]) => calls.filter((c) => c.name === "ferrule").map((c) => c.args);

describe("harness session report at login", () => {
  test("the fresh mint, the recorded-key re-login, and the revive re-login each carry the session file and the harness directory", async () => {
    const fresh = await boot({}, { entries: [], sessionId: "sess-1", sessionFile: SESSION_FILE });
    try {
      assert.notEqual(fresh.cwd, SESSION_FILE);
      assert.deepEqual(ferruleArgs(fresh.calls()), [{ root: fresh.cwd, format: "json", harness_session: SESSION_FILE, harness_cwd: fresh.cwd }]);
    } finally { fresh.handle.shutdown(); fresh.restore(); }

    const resumed = await boot({ accept: ["original-key", "revived-key"] }, { entries: [leadKeyEntry("sess-1", "original-key")], sessionId: "sess-1", sessionFile: SESSION_FILE });
    try {
      await resumed.tools.get("ws__workflow_manual").execute("one", { session_key: "revived-key" }, undefined, undefined, ctx);
      assert.equal(resumed.handle.defaultSessionKeyRef.current, "revived-key");
      assert.deepEqual(ferruleArgs(resumed.calls()), [
        { root: resumed.cwd, format: "json", relogin_session_key: "original-key", harness_session: SESSION_FILE, harness_cwd: resumed.cwd },
        { root: resumed.cwd, format: "json", relogin_session_key: "revived-key", harness_session: SESSION_FILE, harness_cwd: resumed.cwd },
      ]);
    } finally { resumed.handle.shutdown(); resumed.restore(); }
  });

  test("without a session file no login carries either field", async () => {
    const h = await boot({ accept: ["original-key"] }, { entries: [leadKeyEntry("sess-1", "stale-key")], sessionId: "sess-1" });
    try {
      assert.deepEqual(ferruleArgs(h.calls()), [
        { root: h.cwd, format: "json", relogin_session_key: "stale-key" },
        { root: h.cwd, format: "json" },
      ]);
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("a child process minting under its policy sends exactly its policy arguments, never the harness session", async () => {
    const policy = { version: 1, depth: 1, maxDepth: 2, tools: ["ws__workflow_manual"], authority: "delegate", parentSessionKey: "lead-key" };
    const h = await boot({}, { entries: [], sessionId: "sess-1", sessionFile: SESSION_FILE, policy });
    try {
      assert.deepEqual(ferruleArgs(h.calls()), [{ root: h.cwd, format: "json", capability: "delegate", parent_session_key: "lead-key" }]);
    } finally { h.handle.shutdown(); h.restore(); }
  });
});

describe("a session the dashboard hosted", () => {
  test("resumed in a terminal, it re-logs in with the hosted key its entry recorded and mints nothing", async () => {
    // The entry a hosted bootstrap appends (lead-key-hosted.test.ts); no hosted variables are set here.
    const h = await boot({ accept: ["hosted-key"] }, { entries: [leadKeyEntry("sess-1", "hosted-key")], sessionId: "sess-1", sessionFile: SESSION_FILE });
    try {
      assert.equal(h.handle.defaultSessionKeyRef.current, "hosted-key");
      assert.deepEqual(h.relogins(), ["hosted-key"]);
      assert.equal(h.mints(), 0);
      assert.deepEqual(h.appended, []);
    } finally { h.handle.shutdown(); h.restore(); }
  });
});

describe("a recorded key held by a hosted session", () => {
  test("mints nothing, appends nothing, leaves the session keyless, and warns once with ws-mcp's refusal", async () => {
    const h = await boot({ held: ["hosted-key"] }, { entries: [leadKeyEntry("sess-1", "hosted-key")], sessionId: "sess-1", sessionFile: SESSION_FILE });
    try {
      assert.equal(ferruleArgs(h.calls()).length, 1, "exactly the one re-login");
      assert.deepEqual(h.relogins(), ["hosted-key"]);
      assert.equal(h.mints(), 0);
      assert.deepEqual(h.appended, [], "the recorded entry stays the session's newest");
      assert.equal(h.handle.defaultSessionKeyRef.current, undefined);
      assert.ok(!h.calls().some((c) => c.name === "workflow_manual"), "a keyless bootstrap fetches no snapshot");
      const warnings = h.notices.filter((n) => n.level === "warning");
      assert.equal(warnings.length, 1, warnings.map((w) => w.message).join(" | "));
      assert.ok(warnings[0]!.message.includes(heldRefusal("hosted-key")), warnings[0]!.message);
      assert.match(warnings[0]!.message, /No key was minted\.$/);
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("a revive with a held key adopts nothing and warns with ws-mcp's refusal", async () => {
    const h = await boot({ held: ["hosted-key"] }, { entries: [], sessionId: "sess-1" });
    try {
      const revived = await h.tools.get("ws__workflow_manual").execute("one", { session_key: "hosted-key" }, undefined, undefined, ctx);
      assert.ok(!texts(revived.content).some((t) => t.startsWith(ADAPTER_MESSAGE_LABEL)));
      assert.equal(h.handle.defaultSessionKeyRef.current, "minted-1");
      assert.equal(h.appended.length, 1, "only the bootstrap mint was recorded");
      const warnings = h.notices.filter((n) => n.level === "warning");
      assert.equal(warnings.length, 1);
      assert.ok(warnings[0]!.message.includes(heldRefusal("hosted-key")), warnings[0]!.message);
    } finally { h.handle.shutdown(); h.restore(); }
  });
});

describe("adopting the key a lead revives with", () => {
  test("a lead workflow_manual with another accepted key re-logs in, adopts it, records it, appends the pinned line once, and the next compaction summary names it", async () => {
    const h = await boot({ accept: ["revived-key"] }, { entries: [], sessionId: "sess-1" });
    try {
      assert.equal(h.handle.defaultSessionKeyRef.current, "minted-1");
      const manual = h.tools.get("ws__workflow_manual");

      const revived = await manual.execute("one", { session_key: "revived-key" }, undefined, undefined, ctx);
      const lines = texts(revived.content);
      assert.equal(lines.at(-1), buildDefaultKeyChangedLine("minted-1", "revived-key"), "the change line is the last text item");
      assert.match(lines[0]!, /## Session Key\nrevived-key/, "the manual result itself is the revived key's");
      assert.equal(h.handle.defaultSessionKeyRef.current, "revived-key");
      assert.deepEqual(h.relogins(), ["revived-key"]);
      assert.deepEqual(h.appended, [
        { type: "custom", customType: LEAD_KEY_ENTRY, data: { sessionId: "sess-1", key: "minted-1" } },
        { type: "custom", customType: LEAD_KEY_ENTRY, data: { sessionId: "sess-1", key: "revived-key" } },
      ]);

      // Reviving with the key that is already the default: no re-login, no line.
      const again = await manual.execute("two", { session_key: "revived-key" }, undefined, undefined, ctx);
      assert.ok(!texts(again.content).some((t) => t.startsWith(ADAPTER_MESSAGE_LABEL)), "no change line when the key is unchanged");
      assert.deepEqual(h.relogins(), ["revived-key"]);
      assert.equal(h.appended.length, 2);

      // An omitted key now fills the adopted key.
      await manual.execute("three", {}, undefined, undefined, ctx);
      assert.equal(h.calls().filter((c) => c.name === "workflow_manual").at(-1)?.args.session_key, "revived-key");

      const summary = buildLeadCompactionSummary({ sessionKey: h.handle.defaultSessionKeyRef.current, branchEntries: [], registry: undefined, prose: renderLeadProse({ current_work: "x" }), dialogBudgetBytes: 4096, sessionFile: "/sessions/lead.jsonl" });
      assert.match(summary, /ws session key: `revived-key` \(preserve verbatim\)/);
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("the verbatim workflow_manual path (no static snapshot) adopts the same way", async () => {
    const h = await boot({ accept: ["revived-key"], noSnapshot: true }, { entries: [], sessionId: "sess-1" });
    try {
      assert.equal(h.handle.staticBodySnapshotRef.current, undefined);
      const manual = h.tools.get("ws__workflow_manual");
      const revived = await manual.execute("one", { session_key: "revived-key" }, undefined, undefined, ctx);
      assert.equal(texts(revived.content).at(-1), buildDefaultKeyChangedLine("minted-1", "revived-key"));
      assert.equal(h.handle.defaultSessionKeyRef.current, "revived-key");
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("no adoption on a refused re-login, a failed workflow_manual, the bootstrap sentinel, or an explicit key to another tool", async () => {
    const h = await boot({ accept: [], failManual: ["dead-key"] }, { entries: [], sessionId: "sess-1" });
    try {
      const manual = h.tools.get("ws__workflow_manual");
      const noLine = (content: any[]) => assert.ok(!texts(content).some((t) => t.startsWith(ADAPTER_MESSAGE_LABEL)));

      // Refused re-login (ws-mcp knows no such lead key): the manual result
      // stands, nothing is adopted.
      const refused = await manual.execute("one", { session_key: "foreign-key" }, undefined, undefined, ctx);
      noLine(refused.content);
      assert.deepEqual(h.relogins(), ["foreign-key"]);
      assert.equal(h.handle.defaultSessionKeyRef.current, "minted-1");

      // Failed workflow_manual: the call throws before any re-login.
      await assert.rejects(manual.execute("two", { session_key: "dead-key" }, undefined, undefined, ctx), /unknown_session/);
      assert.deepEqual(h.relogins(), ["foreign-key"]);
      assert.equal(h.handle.defaultSessionKeyRef.current, "minted-1");

      // The bootstrap sentinel is a fresh-bootstrap request, not a revive.
      const sentinel = await manual.execute("three", { session_key: SENTINEL }, undefined, undefined, ctx);
      noLine(sentinel.content);
      assert.deepEqual(h.relogins(), ["foreign-key"]);

      // An explicit key to any other tool (a child's key, say) is forwarded, never adopted.
      await h.tools.get("ws__playbook_read").execute("four", { name: "lead-workflow-manual", session_key: "child-key" }, undefined, undefined, ctx);
      assert.deepEqual(h.relogins(), ["foreign-key"]);
      assert.equal(h.handle.defaultSessionKeyRef.current, "minted-1");
      assert.equal(h.appended.length, 1, "only the bootstrap mint was recorded");
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("an adoption notifies key observers once with the new and previous key; a refused re-login or an unchanged key notifies nothing", async () => {
    const h = await boot({ accept: ["revived-key"] }, { entries: [], sessionId: "sess-1" });
    try {
      const adopted: Array<[string, string | undefined]> = [];
      const unsubscribe = h.handle.onDefaultKeyAdopted((current, previous) => {
        assert.equal(h.handle.defaultSessionKeyRef.current, current, "the default key is already switched when observers run");
        adopted.push([current, previous]);
      });
      h.handle.onDefaultKeyAdopted(() => { throw new Error("a failing observer"); });
      const manual = h.tools.get("ws__workflow_manual");

      await manual.execute("refused", { session_key: "foreign-key" }, undefined, undefined, ctx);
      assert.deepEqual(adopted, [], "a refused re-login adopts nothing");

      const revived = await manual.execute("one", { session_key: "revived-key" }, undefined, undefined, ctx);
      assert.equal(texts(revived.content).at(-1), buildDefaultKeyChangedLine("minted-1", "revived-key"), "a throwing observer cannot fail the adopting call");
      assert.deepEqual(adopted, [["revived-key", "minted-1"]]);

      await manual.execute("two", { session_key: "revived-key" }, undefined, undefined, ctx);
      assert.deepEqual(adopted, [["revived-key", "minted-1"]], "an unchanged key notifies nothing");

      unsubscribe();
      h.handle.defaultSessionKeyRef.current = "minted-1";
      await manual.execute("three", { session_key: "revived-key" }, undefined, undefined, ctx);
      assert.equal(adopted.length, 1, "an unsubscribed observer is not notified");
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("a legacy session with no key entry mints on resume, then converges on the first revive with its original key", async () => {
    // Legacy file: compaction summaries and fork entries, but no lead key entry.
    const legacyEntries = [{ type: "custom", customType: "ws-pi-fork-keys", data: { sessionId: "other", current: "fork-key", previous: [] } }];
    const first = await boot({ accept: ["original-key"] }, { entries: legacyEntries, sessionId: "legacy-sess" });
    let appended: Harness["appended"];
    try {
      assert.equal(first.handle.defaultSessionKeyRef.current, "minted-1");
      assert.deepEqual(first.relogins(), []);
      const revived = await first.tools.get("ws__workflow_manual").execute("one", { session_key: "original-key" }, undefined, undefined, ctx);
      assert.equal(texts(revived.content).at(-1), buildDefaultKeyChangedLine("minted-1", "original-key"));
      assert.equal(first.handle.defaultSessionKeyRef.current, "original-key");
      appended = first.appended;
    } finally { first.handle.shutdown(); first.restore(); }

    // The next restart of the same session file reuses the adopted key.
    const second = await boot({ accept: ["original-key"] }, { entries: [...legacyEntries, ...appended], sessionId: "legacy-sess" });
    try {
      assert.equal(second.handle.defaultSessionKeyRef.current, "original-key");
      assert.deepEqual(second.relogins(), ["original-key"]);
      assert.equal(second.mints(), 0);
      assert.deepEqual(second.appended, []);
    } finally { second.handle.shutdown(); second.restore(); }
  });
});
