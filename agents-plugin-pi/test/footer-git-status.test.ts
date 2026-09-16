import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { createFooterGitCache, gitStatusSpans, parseGitStatus, queryFooterGit, runFooterGit, GIT_IDLE_MS, GIT_INPUT_DELAY_MS, GIT_TIMEOUT_MS } from "../src/footer-git-status.ts";

const clean = () => ({ ahead: 0, behind: 0, added: 0, deleted: 0, changed: 0, untracked: 0 });
const flush = async () => { for (let i = 0; i < 5; i++) await Promise.resolve(); };

test("NUL records count unstaged paths, not staged-only changes or renamed/newline path fragments", () => {
  const status = parseGitStatus("# branch.ab +1 -2\0" +
    "1 M. N... 100644 100644 100644 a b staged\0" +
    "1 .M N... 100644 100644 100644 a b unstaged\0" +
    "2 R. N... 100644 100644 100644 a b R100 renamed\0? fake-old-name\0" +
    "? real\nnew\0", "12\t3\tunstaged\0-\t-\tbinary\0");
  assert.deepEqual(status, { ahead: 1, behind: 2, added: 12, deleted: 3, changed: 1, untracked: 1 });
  assert.deepEqual(gitStatusSpans(status), [
    { text: "↑1", color: "success" }, { text: "↓2", color: "warning" },
    { text: "+12", color: "success" }, { text: "-3", color: "error" },
    { text: "~1", color: "warning" }, { text: "?1", color: "accent" },
  ]);
  assert.deepEqual(gitStatusSpans(clean()), []);
  assert.deepEqual(gitStatusSpans(undefined), []);
  for (const operation of ["merging", "rebasing", "cherry-picking", "reverting", "conflicting"] as const) {
    assert.deepEqual(gitStatusSpans({ ...clean(), operation }), [{ text: operation, color: operation === "conflicting" ? "error" : "warning" }]);
  }
});

test("real Git snapshots distinguish staged-only, unstaged line totals, untracked paths, operation states and conflicts", async () => {
  const cwd = mkdtempSync(join(tmpdir(), "footer-git-"));
  const git = (...args: string[]) => execFileSync("git", args, { cwd, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim();
  const signal = new AbortController().signal;
  try {
    assert.equal(await queryFooterGit(cwd, signal), undefined);
    git("init", "-q"); git("config", "user.name", "Test"); git("config", "user.email", "test@example.com");
    writeFileSync(join(cwd, "file"), "one\ntwo\n"); git("add", "."); git("commit", "-qm", "base");
    assert.deepEqual(await queryFooterGit(cwd, signal), clean());
    writeFileSync(join(cwd, "staged"), "staged\n"); git("add", "staged");
    assert.deepEqual(await queryFooterGit(cwd, signal), clean());
    writeFileSync(join(cwd, "file"), "replacement\n"); writeFileSync(join(cwd, "untracked"), "x");
    assert.deepEqual(await queryFooterGit(cwd, signal), { ...clean(), added: 1, deleted: 2, changed: 1, untracked: 1 });
    const gitDir = git("rev-parse", "--absolute-git-dir");
    for (const [marker, operation] of [["MERGE_HEAD", "merging"], ["CHERRY_PICK_HEAD", "cherry-picking"], ["REVERT_HEAD", "reverting"], ["rebase-merge", "rebasing"], ["rebase-apply", "rebasing"]] as const) {
      writeFileSync(join(gitDir, marker), git("rev-parse", "HEAD"));
      assert.equal((await queryFooterGit(cwd, signal))?.operation, operation);
      rmSync(join(gitDir, marker));
    }
    const oid = git("rev-parse", "HEAD:file");
    execFileSync("git", ["update-index", "--index-info"], { cwd, input: `0 ${"0".repeat(40)}\tfile\n100644 ${oid} 1\tfile\n100644 ${oid} 2\tfile\n100644 ${oid} 3\tfile\n` });
    writeFileSync(join(gitDir, "MERGE_HEAD"), git("rev-parse", "HEAD"));
    assert.equal((await queryFooterGit(cwd, signal))?.operation, "conflicting");
  } finally { rmSync(cwd, { recursive: true, force: true }); }
});

test("query bounds the whole refresh and distinguishes nonrepository from transient errors", async () => {
  const signal = new AbortController().signal;
  await assert.rejects(queryFooterGit("/unused", signal, async () => { throw new Error("transient"); }), /transient/);
  assert.equal(await queryFooterGit("/unused", signal, async () => { throw { stderr: "fatal: not a git repository" }; }), undefined);
  const started = Date.now();
  // Keep the event loop alive: AbortSignal.timeout deliberately uses an unref timer.
  const keepAlive = setInterval(() => {}, 1000);
  try {
    await assert.rejects(queryFooterGit("/unused", signal, (_cwd, _args, bounded) => new Promise((_resolve, reject) => {
      bounded.addEventListener("abort", () => reject(bounded.reason), { once: true });
    })), /timeout/i);
    assert.ok(Date.now() - started < GIT_TIMEOUT_MS + 1500);
  } finally { clearInterval(keepAlive); }
});

test("real Git subprocess is killed on cancellation and timeout", async () => {
  const abort = new AbortController();
  const cancelled = runFooterGit(process.cwd(), ["hash-object", "--stdin"], abort.signal);
  abort.abort();
  await assert.rejects(cancelled, /abort/i);
  await assert.rejects(runFooterGit(process.cwd(), ["hash-object", "--stdin"], new AbortController().signal),
    (error: any) => error.killed === true && error.signal === "SIGKILL");
});

test("cache requests each turn, coalesces per cwd, rerenders changes only, retains stale errors and clears nonrepositories", async () => {
  let cwd = "/one", changed = 0;
  const calls: Array<{ cwd: string; signal: AbortSignal; resolve: (status: any) => void; reject: (error: Error) => void }> = [];
  const cache = createFooterGitCache({ cwd: () => cwd, isIdle: () => true, changed: () => changed++,
    query: (cwd, signal) => new Promise((resolve, reject) => calls.push({ cwd, signal, resolve, reject })) });
  cache.turnEnd(); cache.turnEnd(); assert.equal(calls.length, 1);
  cwd = "/two"; cache.turnEnd(); assert.equal(calls.length, 2);
  calls[0].resolve({ ...clean(), ahead: 1 }); calls[1].resolve(clean()); await flush();
  assert.equal(changed, 1); assert.deepEqual(cache.spans("/one"), [{ text: "↑1", color: "success" }]);
  cwd = "/one"; cache.turnEnd(); calls[2].reject(new Error("transient")); await flush();
  assert.equal(cache.spans(cwd).length, 1); assert.equal(changed, 1);
  cache.turnEnd(); calls[3].resolve({ ...clean(), ahead: 1 }); await flush(); assert.equal(changed, 1);
  cache.turnEnd(); calls[4].resolve(undefined); await flush(); assert.equal(changed, 2); assert.deepEqual(cache.spans(cwd), []);
  cache.turnEnd(); cache.stop(); assert.equal(calls[5].signal.aborted, true);
  calls[5].resolve({ ...clean(), ahead: 9 }); await flush(); assert.equal(changed, 2);
  cache.turnEnd(); assert.equal(calls.length, 6);
});

test("five-minute timer, resettable post-input deferral, still-idle check and stop cleanup", async t => {
  t.mock.timers.enable({ apis: ["Date", "setTimeout"], now: 0 });
  let idle = true;
  const signals: AbortSignal[] = [];
  const cache = createFooterGitCache({ cwd: () => "/one", isIdle: () => idle, changed() {},
    query: async (_cwd, signal) => { signals.push(signal); return clean(); } });
  t.mock.timers.tick(GIT_IDLE_MS - 1); assert.equal(signals.length, 0);
  cache.input();
  t.mock.timers.tick(GIT_INPUT_DELAY_MS - 1); assert.equal(signals.length, 0);
  t.mock.timers.tick(1); assert.equal(signals.length, 1);
  cache.input(); assert.equal(signals[0].aborted, true); await flush();
  t.mock.timers.tick(20_000); cache.input();
  t.mock.timers.tick(GIT_INPUT_DELAY_MS - 1); assert.equal(signals.length, 1);
  t.mock.timers.tick(1); assert.equal(signals.length, 2); await flush();
  t.mock.timers.tick(GIT_IDLE_MS); assert.equal(signals.length, 3);
  cache.input(); await flush(); idle = false;
  t.mock.timers.tick(GIT_INPUT_DELAY_MS); assert.equal(signals.length, 3);
  cache.turnEnd(); assert.equal(signals.length, 4); await flush();
  cache.stop(); idle = true; t.mock.timers.tick(GIT_IDLE_MS * 2); assert.equal(signals.length, 4);
});
