import { test } from "node:test";
import assert from "node:assert/strict";
import * as fs from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { SessionEntry } from "@earendil-works/pi-coding-agent";
import { cleanOrphanSummarySidecars, createSummarySidecar, seedNativeSummarySidecar, type SidecarIO, DISPLAY_SUMMARY_SIDECAR_SUFFIX } from "../src/display-summary-sidecar.ts";

const summary = { subtitle: "src/a.ts", toolIntention: "inspect", toolResult: "found" };

test("validated late-tool candidates replay once on ownership confirmation, without rereading the sidecar", async (t) => {
  const f = await fixture(t);
  await fs.writeFile(f.sidecarFile, row("a") + "\n" + row("b") + "\n" + row("missing") + "\n");
  const names = new Set<string>();
  const replay: string[] = [];
  let reads = 0;
  const cache = openCache(f, { toolNames: names, restore: (batch) => { replay.push(...batch.keys()); }, io: {
    ...fs, open: async (...args: Parameters<typeof fs.open>) => {
      if (args[0] === f.sidecarFile) reads += 1;
      return fs.open(...args);
    },
  } });
  await cache.ready;
  assert.deepEqual(replay, []);
  assert.equal(cache.snapshot().size, 0);
  names.add("read");
  cache.sync();
  await cache.drain();
  assert.deepEqual(replay, ["a", "b"]);
  assert.deepEqual(cache.snapshot().get("a"), summary);
  await cache.drain();
  assert.deepEqual(replay, ["a", "b"]);
  assert.equal(reads, 1, "eligibility changes use originating validated candidates, not disk replay");
});
const toolNames = new Set(["read"]);
const entry = (id: string): SessionEntry => ({ type: "message", id: `entry-${id}`, parentId: null, timestamp: "now", message: {
  role: "toolResult", toolCallId: id, toolName: "read", content: [], isError: false, timestamp: 0,
} });

async function fixture(t: { after(fn: () => Promise<void>): void }, saved = true) {
  const directory = await fs.mkdtemp(join(tmpdir(), "ws-display-sidecar-"));
  t.after(() => fs.rm(directory, { recursive: true, force: true }));
  const sessionFile = join(directory, "lead.jsonl");
  const entries = [entry("a"), entry("b")];
  const save = () => fs.writeFile(sessionFile, [JSON.stringify({ type: "session", id: "lead" }), ...entries.map((e) => JSON.stringify(e))].join("\n") + "\n");
  if (saved) await save();
  return { sessionFile, sidecarFile: sessionFile + DISPLAY_SUMMARY_SIDECAR_SUFFIX, entries, save };
}

test("accepted batches append incrementally and reopen restores renderer IDs", async (t) => {
  const f = await fixture(t);
  const deps = { conversation: { sessionId: "lead", sessionFile: f.sessionFile, entries: () => f.entries }, toolNames, restore: () => {} };
  const cache = createSummarySidecar(deps);
  await cache.ready;
  cache.accept(new Map([["a", summary]]));
  await cache.drain();
  const first = await fs.readFile(f.sidecarFile, "utf8");
  cache.accept(new Map([["b", { ...summary, toolResult: "second" }]]));
  await cache.drain();
  const second = await fs.readFile(f.sidecarFile, "utf8");
  assert.ok(second.startsWith(first));
  assert.equal(second.trim().split("\n").length, 2);
  assert.equal(JSON.parse(first).sessionId, "lead");
  assert.equal(JSON.parse(first).id, "a", "not entry-a");
  let restored = new Map();
  await createSummarySidecar({ ...deps, restore: (batch) => { restored = new Map(batch); } }).ready;
  assert.deepEqual(restored.get("a"), summary);
  assert.equal(restored.get("b").toolResult, "second");
});

test("first save backfills accepted memory; observed deletion never recreates the sidecar", async (t) => {
  const f = await fixture(t, false);
  const cache = createSummarySidecar({ conversation: { sessionId: "lead", sessionFile: f.sessionFile, entries: () => f.entries }, toolNames, restore: () => {} });
  await cache.ready;
  cache.accept(new Map([["a", summary]]));
  await cache.drain();
  await assert.rejects(fs.lstat(f.sidecarFile), { code: "ENOENT" });
  assert.deepEqual(cache.snapshot().get("a"), summary);
  await f.save();
  await cache.drain();
  assert.deepEqual(JSON.parse(await fs.readFile(f.sidecarFile, "utf8")).summary, summary);
  await fs.unlink(f.sessionFile);
  await fs.unlink(f.sidecarFile);
  cache.accept(new Map([["b", summary]]));
  await cache.drain();
  await f.save();
  await cache.drain();
  await assert.rejects(fs.lstat(f.sidecarFile), { code: "ENOENT" });
  assert.deepEqual(cache.snapshot().get("b"), summary);
});

function deferred<T = void>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}
const row = (id: unknown, value: unknown = summary, sessionId = "lead", version = 1) => JSON.stringify({ version, sessionId, id, summary: value });
function openCache(f: Awaited<ReturnType<typeof fixture>>, extra: Partial<Parameters<typeof createSummarySidecar>[0]> = {}) {
  return createSummarySidecar({ conversation: { sessionId: "lead", sessionFile: f.sessionFile, entries: () => f.entries }, toolNames, restore: () => {}, ...extra });
}
async function records(path: string) {
  return (await fs.readFile(path, "utf8")).trim().split("\n").map((line) => JSON.parse(line));
}

test("last valid eligible value wins; invalid records never replace it", async (t) => {
  const f = await fixture(t);
  const latest = { ...summary, toolResult: "latest", optionalContext: "context" };
  await fs.writeFile(f.sidecarFile, [row("a"), row("b"), row("a", latest),
    row("a", { toolIntention: "", toolResult: "bad" }), row("a", { ...summary, optionalContext: 4 }),
    row("a", summary, "other"), row("a", summary, "lead", 2), row("entry-a"), row("missing"), row(" "), row(12), row("a", []), row("a", { ...summary, toolResult: null }), "garbage"].join("\n") + "\n");
  let replay = new Map();
  const cache = openCache(f, { restore: (batch) => { replay = new Map(batch); } });
  await cache.ready;
  assert.deepEqual([...replay], [["a", latest], ["b", summary]]);
  assert.deepEqual(cache.snapshot(), replay);
  cache.accept(new Map([["missing", summary], ["b", latest]]));
  await cache.drain();
  assert.ok((await fs.readFile(f.sidecarFile, "utf8")).endsWith(row("b", latest) + "\n"));
});

for (const foreign of [row("a", summary, "foreign"), row("a", summary, "lead", 2)]) {
  test(`wholly foreign file is immutable: ${foreign}`, async (t) => {
    const f = await fixture(t);
    await fs.writeFile(f.sidecarFile, foreign);
    const cache = openCache(f);
    await cache.ready;
    assert.equal(cache.snapshot().size, 0);
    cache.accept(new Map([["a", summary]]));
    await cache.drain();
    assert.deepEqual(cache.snapshot().get("a"), summary);
    assert.equal(await fs.readFile(f.sidecarFile, "utf8"), foreign);
  });
}

for (const tail of ["{broken", row("b"), row("a", { toolIntention: 7 })]) {
  test(`recognized file preserves bytes and repairs unterminated boundary: ${tail}`, async (t) => {
    const f = await fixture(t);
    const before = row("a") + "\n" + tail;
    await fs.writeFile(f.sidecarFile, before);
    const cache = openCache(f);
    await cache.ready;
    assert.deepEqual(cache.snapshot().get("a"), summary);
    assert.equal(cache.snapshot().has("b"), tail === row("b"));
    const next = { ...summary, toolResult: "new" };
    cache.accept(new Map([["b", next]]));
    await cache.drain();
    assert.equal(await fs.readFile(f.sidecarFile, "utf8"), before + "\n" + row("b", next) + "\n");
    const reopened = openCache(f);
    await reopened.ready;
    assert.deepEqual(reopened.snapshot().get("b"), next);
  });
}

test("a record written before subtitles existed loads without one; an interim `title` loads as the subtitle; a blank one is dropped, not the record", async (t) => {
  const f = await fixture(t);
  const legacy = { toolIntention: "inspect", toolResult: "legacy" };
  const blanks = await fixture(t);
  await fs.writeFile(blanks.sidecarFile, [row("a", { ...legacy, subtitle: " " }), row("b", { ...legacy, title: " " })].join("\n") + "\n");
  let blankReplay = new Map();
  await openCache(blanks, { restore: (batch) => { blankReplay = new Map(batch); } }).ready;
  assert.deepEqual([...blankReplay], [["a", legacy], ["b", legacy]], "a blank subtitle or title drops the field, not the record");

  await fs.writeFile(f.sidecarFile, [row("a", legacy), row("b", { ...legacy, title: "src/b.ts" })].join("\n") + "\n");
  let replay = new Map();
  const cache = openCache(f, { restore: (batch) => { replay = new Map(batch); } });
  await cache.ready;
  assert.deepEqual([...replay], [["a", legacy], ["b", { subtitle: "src/b.ts", ...legacy }]]);
  cache.accept(new Map([["a", summary]]));
  await cache.drain();
  const reopened = openCache(f);
  await reopened.ready;
  assert.deepEqual(reopened.snapshot().get("a"), summary, "a value with a subtitle round-trips");
});

test("recognized ownership with an invalid value still permits append", async (t) => {
  const f = await fixture(t);
  const before = row("a", { toolResult: "missing intention" }) + "\n";
  await fs.writeFile(f.sidecarFile, before);
  const cache = openCache(f);
  await cache.ready;
  assert.equal(cache.snapshot().size, 0);
  cache.accept(new Map([["a", summary]]));
  await cache.drain();
  assert.equal(await fs.readFile(f.sidecarFile, "utf8"), before + row("a") + "\n");
});

for (const failure of ["read", "write"] as const) {
  test(`${failure} failure keeps live values and raw fallback without rejecting drain`, async (t) => {
    const f = await fixture(t);
    if (failure === "read") await fs.writeFile(f.sidecarFile, row("b") + "\n");
    const io: SidecarIO = { ...fs, open: async (...args: Parameters<typeof fs.open>) => {
      if (args[0] === f.sidecarFile) throw new Error(`injected ${failure} failure`);
      return fs.open(...args);
    } };
    const cache = openCache(f, { io });
    await cache.ready;
    cache.accept(new Map([["a", summary]]));
    await cache.drain();
    assert.deepEqual(cache.snapshot().get("a"), summary);
    assert.equal(cache.snapshot().get("b"), undefined);
    if (failure === "read") assert.equal(await fs.readFile(f.sidecarFile, "utf8"), row("b") + "\n");
    else await assert.rejects(fs.lstat(f.sidecarFile), { code: "ENOENT" });
  });
}

test("batches serialize while append is held; drain waits; live acceptance beats delayed replay", async (t) => {
  const f = await fixture(t);
  await fs.writeFile(f.sidecarFile, row("a") + "\n");
  const entered = deferred();
  const release = deferred();
  let held = false;
  const io: SidecarIO = { ...fs, open: async (...args: Parameters<typeof fs.open>) => {
    if (args[0] === f.sidecarFile && !held) { held = true; entered.resolve(); await release.promise; }
    return fs.open(...args);
  } };
  let replay = new Map();
  const cache = openCache(f, { io, restore: (batch) => { replay = new Map(batch); } });
  await entered.promise;
  const newer = { ...summary, toolResult: "live" };
  cache.accept(new Map([["a", newer]]));
  cache.accept(new Map([["b", summary]]));
  let drained = false;
  const drain = cache.drain().then(() => { drained = true; });
  assert.equal(drained, false);
  release.resolve();
  await drain;
  assert.equal(replay.has("a"), false);
  assert.deepEqual(cache.snapshot().get("a"), newer);
  assert.deepEqual((await records(f.sidecarFile)).map((r) => [r.id, r.summary.toolResult]), [["a", "found"], ["a", "live"], ["b", "found"]]);
});

test("newer live acceptance beats a validated candidate when ownership arrives later", async (t) => {
  const f = await fixture(t);
  await fs.writeFile(f.sidecarFile, row("a") + "\n" + row("b") + "\n");
  const names = new Set<string>();
  const restored: string[] = [];
  const cache = openCache(f, { toolNames: names, restore: (batch) => { restored.push(...batch.keys()); } });
  await cache.ready;
  const live = { ...summary, toolResult: "newer live" };
  cache.accept(new Map([["a", live]]));
  names.add("read");
  cache.sync();
  await cache.drain();
  assert.deepEqual(restored, ["b"]);
  assert.deepEqual(cache.snapshot().get("a"), live);
  assert.deepEqual((await records(f.sidecarFile)).at(-1).summary, live);
});

test("live acceptance inside native replay supersedes inherited writes", async (t) => {
  const f = await fixture(t);
  await fs.writeFile(f.sidecarFile, row("a") + "\n");
  const c = await nativeChild(f);
  const names = new Set<string>();
  const live = { ...summary, toolResult: "accepted during restore" };
  let restores = 0;
  const child = c.cache({ toolNames: names, restore: (batch) => {
    restores++;
    assert.deepEqual([...batch], [["a", summary]]);
    child.accept(new Map([["a", live]]));
  } });
  await child.ready;
  assert.equal(restores, 0);
  names.add("read");
  child.sync();
  await child.drain();
  assert.equal(restores, 1);
  assert.deepEqual(child.snapshot().get("a"), live);
  assert.deepEqual((await records(c.sidecar)).map((record) => record.summary), [live], "no stale inheritance appended after live acceptance");
  assert.equal(await fs.readFile(f.sidecarFile, "utf8"), row("a") + "\n");
});

for (const data of ["", row("child-only", { ...summary, toolResult: "child own" }, "child") + "\n"]) {
  test(`late ownership retries only authoritative child's own candidates: ${JSON.stringify(data)}`, async (t) => {
    const f = await fixture(t);
    await fs.writeFile(f.sidecarFile, row("a") + "\n");
    const c = await nativeChild(f);
    await fs.writeFile(c.sidecar, data);
    const names = new Set<string>();
    const replay: string[] = [];
    let parentReads = 0;
    const child = c.cache({ toolNames: names, restore: (batch) => { replay.push(...batch.keys()); }, io: {
      ...fs, open: async (...args: Parameters<typeof fs.open>) => {
        if (args[0] === f.sidecarFile) parentReads++;
        return fs.open(...args);
      },
    } });
    await child.ready;
    assert.deepEqual([...child.snapshot()], []);
    names.add("read");
    child.sync();
    await child.drain();
    child.sync();
    await child.drain();
    assert.deepEqual(replay, data ? ["child-only"] : []);
    assert.equal(child.snapshot().has("a"), false, "parent never fills the child's cache holes");
    assert.equal(child.snapshot().get("child-only")?.toolResult, data ? "child own" : undefined);
    assert.equal(parentReads, 0);
    assert.equal(await fs.readFile(c.sidecar, "utf8"), data);
  });
}

test("no-file conversations remain memory-only", async () => {
  const cache = createSummarySidecar({ conversation: { sessionId: "unsaved", entries: () => [entry("a")] }, toolNames,
    restore: () => assert.fail("no disk replay"), io: { ...fs, lstat: async () => { assert.fail("no filesystem work"); } } });
  await cache.ready;
  cache.accept(new Map([["a", summary]]));
  cache.sync();
  await cache.drain();
  assert.deepEqual(cache.snapshot().get("a"), summary);
});

test("cleanup is direct-directory, supported consistent ownership only", async (t) => {
  const f = await fixture(t);
  const suffix = DISPLAY_SUMMARY_SIDECAR_SUFFIX;
  const files = new Map([
    ["orphan.jsonl", row("a") + "\n"], ["foreign.jsonl", row("a", summary, "lead", 2)],
    ["mixed.jsonl", row("a") + "\n" + row("b", summary, "other")], ["empty.jsonl", ""],
    ["malformed.jsonl", row("a") + "\n{broken"], ["lead.jsonl", row("a")],
    ["linked-conversation.jsonl", row("a")],
  ]);
  for (const [name, data] of files) await fs.writeFile(join(f.sessionFile, "..", name + suffix), data);
  await fs.symlink(f.sessionFile, join(f.sessionFile, "..", "linked-conversation.jsonl"));
  await fs.symlink(f.sidecarFile, join(f.sessionFile, "..", "linked-sidecar.jsonl" + suffix));
  const nested = join(f.sessionFile, "..", "nested");
  await fs.mkdir(nested);
  await fs.writeFile(join(nested, "orphan.jsonl" + suffix), row("a"));
  await cleanOrphanSummarySidecars(f.sessionFile);
  await assert.rejects(fs.lstat(join(f.sessionFile, "..", "orphan.jsonl" + suffix)), { code: "ENOENT" });
  for (const [name, data] of files) if (name !== "orphan.jsonl") assert.equal(await fs.readFile(join(f.sessionFile, "..", name + suffix), "utf8"), data);
  assert.ok((await fs.lstat(join(f.sessionFile, "..", "linked-sidecar.jsonl" + suffix))).isSymbolicLink());
  assert.equal(await fs.readFile(join(nested, "orphan.jsonl" + suffix), "utf8"), row("a"));
});

async function nativeChild(f: Awaited<ReturnType<typeof fixture>>, id = "child", parent: string | undefined = f.sessionFile) {
  const path = join(f.sessionFile, "..", "child.jsonl");
  const entries = [entry("a"), entry("child-only")];
  await fs.writeFile(path, [JSON.stringify({ type: "session", id, ...(parent ? { parentSession: parent } : {}) }), ...entries.map((e) => JSON.stringify(e))].join("\n") + "\n");
  return { path, entries, sidecar: path + DISPLAY_SUMMARY_SIDECAR_SUFFIX, cache: (extra: Partial<Parameters<typeof createSummarySidecar>[0]> = {}) => createSummarySidecar({ conversation: { sessionId: id, sessionFile: path, entries: () => entries }, toolNames, restore: () => {}, ...extra }) };
}

test("native initial open inherits copied-only rows, then writes independently even with equal IDs", async (t) => {
  const f = await fixture(t);
  await fs.writeFile(f.sidecarFile, row("a") + "\n" + row("b") + "\n" + row("child-only") + "\n");
  const c = await nativeChild(f, "lead");
  const child = c.cache();
  await child.ready;
  assert.deepEqual([...child.snapshot()], [["a", summary]]);
  const parentBefore = await fs.readFile(f.sidecarFile, "utf8");
  child.accept(new Map([["a", { ...summary, toolResult: "child" }]]));
  await child.drain();
  assert.equal(await fs.readFile(f.sidecarFile, "utf8"), parentBefore);
  const childBefore = await fs.readFile(c.sidecar, "utf8");
  const parent = openCache(f);
  await parent.ready;
  parent.accept(new Map([["a", { ...summary, toolResult: "parent" }]]));
  await parent.drain();
  assert.equal(await fs.readFile(c.sidecar, "utf8"), childBefore);
  assert.ok((await records(c.sidecar)).every((r) => r.sessionId === "lead"));
});

for (const data of ["", row("child-only", summary, "child") + "\n{broken"]) {
  test(`existing usable native child is authoritative: ${JSON.stringify(data)}`, async (t) => {
    const f = await fixture(t);
    await fs.writeFile(f.sidecarFile, row("a") + "\n");
    const c = await nativeChild(f);
    await fs.writeFile(c.sidecar, data);
    const child = c.cache();
    await child.ready;
    assert.equal(child.snapshot().has("a"), false);
    assert.equal(child.snapshot().has("child-only"), Boolean(data));
    assert.equal(await fs.readFile(c.sidecar, "utf8"), data);
  });
}

for (const provenance of ["missing", "unavailable"] as const) {
  test(`native inheritance requires provenance and available source: ${provenance}`, async (t) => {
    const f = await fixture(t);
    await fs.writeFile(f.sidecarFile, row("a") + "\n");
    const c = await nativeChild(f, "lead", provenance === "missing" ? undefined : f.sessionFile);
    // nativeChild's default parameter supplies provenance; explicitly remove it.
    if (provenance === "missing") await fs.writeFile(c.path, [JSON.stringify({ type: "session", id: "lead" }), ...c.entries.map((e) => JSON.stringify(e))].join("\n") + "\n");
    else await fs.unlink(f.sessionFile);
    const child = c.cache({ nativeSource: { sessionFile: f.sessionFile, summaries: new Map([["a", summary]]) } });
    await child.ready;
    assert.equal(child.snapshot().size, 0);
    await assert.rejects(fs.lstat(c.sidecar), { code: "ENOENT" });
  });
}

test("native cutover seeds accepted-but-unwritten values without needing a source sidecar", async (t) => {
  const f = await fixture(t);
  const c = await nativeChild(f);
  await seedNativeSummarySidecar(c.path, { sessionFile: f.sessionFile, summaries: new Map([["a", summary], ["b", summary]]) }, toolNames);
  assert.deepEqual((await records(c.sidecar)).map((r) => [r.sessionId, r.id]), [["child", "a"]]);
});

test("native cutover keeps accepted live values when an existing regular parent sidecar cannot be read", async (t) => {
  const f = await fixture(t);
  const c = await nativeChild(f);
  const before = row("a") + "\n";
  await fs.writeFile(f.sidecarFile, before);
  const live = { ...summary, toolResult: "accepted but append failed" };
  const io: SidecarIO = { ...fs, open: async (...args: Parameters<typeof fs.open>) => {
    if (args[0] === f.sidecarFile) throw new Error("existing parent cache read failed");
    return fs.open(...args);
  } };
  await seedNativeSummarySidecar(c.path, { sessionFile: f.sessionFile, summaries: new Map([["a", live], ["b", live]]) }, toolNames, io);
  assert.deepEqual((await records(c.sidecar)).map((r) => [r.sessionId, r.id, r.summary]), [["child", "a", live]]);
  assert.equal(await fs.readFile(f.sidecarFile, "utf8"), before);
});

test("parent-sidecar read failure caused by symlink replacement still excludes native inheritance", async (t) => {
  const f = await fixture(t);
  const c = await nativeChild(f);
  await fs.writeFile(f.sidecarFile, row("a") + "\n");
  const target = f.sidecarFile + ".target";
  let replaced = false;
  const io: SidecarIO = { ...fs, open: async (...args: Parameters<typeof fs.open>) => {
    if (args[0] === f.sidecarFile && !replaced) {
      replaced = true;
      await fs.rename(f.sidecarFile, target);
      await fs.symlink(target, f.sidecarFile);
      throw new Error("sidecar replaced during read");
    }
    return fs.open(...args);
  } };
  await seedNativeSummarySidecar(c.path, { sessionFile: f.sessionFile, summaries: new Map([["a", summary]]) }, toolNames, io);
  assert.equal(replaced, true);
  await assert.rejects(fs.lstat(c.sidecar), { code: "ENOENT" });
  assert.equal(await fs.readFile(target, "utf8"), row("a") + "\n");
});

for (const linked of ["conversation", "sidecar", "source", "source-sidecar", "child", "child-sidecar"] as const) {
  test(`symlink ${linked} is excluded from mutation and inheritance`, async (t) => {
    const f = await fixture(t);
    await fs.writeFile(f.sidecarFile, row("a") + "\n");
    const c = await nativeChild(f);
    if (linked === "child-sidecar") await fs.writeFile(c.sidecar, row("a", summary, "child") + "\n");
    const path = linked === "conversation" || linked === "source" ? f.sessionFile : linked === "sidecar" || linked === "source-sidecar" ? f.sidecarFile : linked === "child-sidecar" ? c.sidecar : c.path;
    const target = path + ".target";
    await fs.rename(path, target);
    await fs.symlink(target, path);
    const before = await fs.readFile(target, "utf8");
    if (linked === "conversation" || linked === "sidecar") {
      const cache = openCache(f);
      await cache.ready;
      cache.accept(new Map([["b", summary]]));
      await cache.drain();
      assert.deepEqual(cache.snapshot().get("b"), summary);
    } else {
      await seedNativeSummarySidecar(c.path, { sessionFile: f.sessionFile, summaries: new Map([["a", summary]]) }, toolNames);
      if (linked !== "child-sidecar") await assert.rejects(fs.lstat(c.sidecar), { code: "ENOENT" });
    }
    assert.equal(await fs.readFile(target, "utf8"), before);
    assert.ok((await fs.lstat(path)).isSymbolicLink());
  });
}

test("all retained history is eligible, including abandoned and precompact rows; Previous conversation is not", async (t) => {
  const f = await fixture(t);
  f.entries.push({ type: "compaction", id: "compact", parentId: "entry-b", timestamp: "now", summary: "earlier", firstKeptEntryId: "entry-b", tokensBefore: 12 } as SessionEntry);
  f.entries.push({ ...entry("abandoned"), parentId: "entry-a" });
  f.entries.push({ type: "custom", id: "history", parentId: null, timestamp: "now", customType: "ws-lead-compaction-history", data: {} });
  await f.save();
  await fs.writeFile(f.sidecarFile, ["a", "b", "abandoned", "history", "compact"].map((id) => row(id)).join("\n") + "\n");
  const cache = openCache(f);
  await cache.ready;
  assert.deepEqual([...cache.snapshot().keys()], ["a", "b", "abandoned"]);
  cache.accept(new Map([["history", summary], ["abandoned", { ...summary, toolResult: "retained" }]]));
  await cache.drain();
  assert.deepEqual((await records(f.sidecarFile)).slice(5).map((r) => r.id), ["abandoned"]);
});
