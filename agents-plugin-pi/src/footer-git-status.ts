/** Asynchronous, bounded Git snapshots; rendering only consumes cached spans. */
import { execFile } from "node:child_process";
import { stat } from "node:fs/promises";
import { join, resolve } from "node:path";

export type GitColor = "success" | "warning" | "error" | "accent";
export interface GitSpan { text: string; color: GitColor }
export interface GitStatus {
  ahead: number; behind: number; added: number; deleted: number; changed: number; untracked: number;
  operation?: "conflicting" | "merging" | "rebasing" | "cherry-picking" | "reverting";
}
export const GIT_TIMEOUT_MS = 2000;
export const GIT_IDLE_MS = 300_000;
export const GIT_INPUT_DELAY_MS = 30_000;

export function gitStatusSpans(status: GitStatus | undefined): GitSpan[] {
  if (!status) return [];
  const spans: GitSpan[] = [];
  if (status.operation) spans.push({ text: status.operation, color: status.operation === "conflicting" ? "error" : "warning" });
  for (const [key, prefix, color] of [
    ["ahead", "↑", "success"], ["behind", "↓", "warning"], ["added", "+", "success"],
    ["deleted", "-", "error"], ["changed", "~", "warning"], ["untracked", "?", "accent"],
  ] as const) if (status[key]) spans.push({ text: `${prefix}${status[key]}`, color });
  return spans;
}

export function parseGitStatus(porcelain: string, numstat: string): GitStatus {
  const status: GitStatus = { ahead: 0, behind: 0, added: 0, deleted: 0, changed: 0, untracked: 0 };
  const records = porcelain.split("\0");
  for (let i = 0; i < records.length; i++) {
    const entry = records[i];
    const ab = /^# branch\.ab \+(\d+) -(\d+)$/.exec(entry);
    if (ab) { status.ahead = Number(ab[1]); status.behind = Number(ab[2]); }
    else if (entry.startsWith("? ")) status.untracked++;
    else if (entry.startsWith("u ")) { status.operation = "conflicting"; status.changed++; }
    else if (/^[12] /.test(entry)) {
      if (entry[3] !== ".") status.changed++;
      if (entry[0] === "2") i++; // Original rename path is a separate NUL record.
    }
  }
  const lines = numstat.split("\0");
  for (let i = 0; i < lines.length; i++) {
    const match = /^(\d+|-)\t(\d+|-)\t(.*)$/s.exec(lines[i]);
    if (!match) continue;
    if (match[1] !== "-") status.added += Number(match[1]);
    if (match[2] !== "-") status.deleted += Number(match[2]);
    if (match[3] === "") i += 2;
  }
  return status;
}

type GitRunner = (cwd: string, args: string[], signal: AbortSignal) => Promise<string>;
export const runFooterGit: GitRunner = (cwd, args, signal) => new Promise((resolveResult, reject) => {
  execFile("git", args, { cwd, signal, timeout: GIT_TIMEOUT_MS, killSignal: "SIGKILL", maxBuffer: 4 * 1024 * 1024,
    encoding: "utf8", env: { ...process.env, LC_ALL: "C", GIT_OPTIONAL_LOCKS: "0" } }, (error, stdout, stderr) => {
    if (error) reject(Object.assign(error, { stderr })); else resolveResult(stdout);
  });
});

export async function queryFooterGit(cwd: string, signal: AbortSignal, run: GitRunner = runFooterGit): Promise<GitStatus | undefined> {
  const bounded = AbortSignal.any([signal, AbortSignal.timeout(GIT_TIMEOUT_MS)]);
  let gitDir: string;
  try { gitDir = (await run(cwd, ["rev-parse", "--absolute-git-dir"], bounded)).trim(); }
  catch (error) {
    if (!bounded.aborted && /not a git repository/.test(String((error as { stderr?: string }).stderr))) return undefined;
    throw error;
  }
  const porcelain = await run(cwd, ["status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all"], bounded);
  const numstat = await run(cwd, ["diff", "--numstat", "-z", "--no-ext-diff", "--no-textconv"], bounded);
  const status = parseGitStatus(porcelain, numstat);
  if (!status.operation) {
    for (const [marker, operation] of [["rebase-merge", "rebasing"], ["rebase-apply", "rebasing"], ["MERGE_HEAD", "merging"], ["CHERRY_PICK_HEAD", "cherry-picking"], ["REVERT_HEAD", "reverting"]] as const) {
      try { await stat(join(gitDir, marker)); status.operation = operation; break; }
      catch (error) { if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error; }
    }
  }
  bounded.throwIfAborted();
  return status;
}

export interface GitCacheOptions {
  cwd(): string;
  isIdle(): boolean;
  changed(): void;
  query?: typeof queryFooterGit;
}

/** One timer and at most one snapshot query per cwd. Disposed generations cannot publish. */
export function createFooterGitCache(options: GitCacheOptions) {
  const cache = new Map<string, GitSpan[]>();
  const flights = new Map<string, { abort: AbortController; idle: boolean; retry: boolean }>();
  const query = options.query ?? queryFooterGit;
  let stopped = false, deferred = false, due = Date.now() + GIT_IDLE_MS;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const key = () => resolve(options.cwd());
  const arm = () => {
    if (timer) clearTimeout(timer);
    if (!stopped) { timer = setTimeout(tick, Math.max(0, due - Date.now())); timer.unref?.(); }
  };
  const refresh = (idle: boolean, cwd = key()) => {
    if (stopped) return;
    const existing = flights.get(cwd);
    if (existing) {
      if (!idle) {
        existing.idle = false;
        existing.retry = existing.abort.signal.aborted;
      }
      return;
    }
    const flight = { abort: new AbortController(), idle, retry: false };
    flights.set(cwd, flight);
    void query(cwd, flight.abort.signal).then(status => {
      if (stopped || flight.abort.signal.aborted) return;
      const spans = gitStatusSpans(status);
      if (JSON.stringify(cache.get(cwd) ?? []) === JSON.stringify(spans)) return;
      cache.delete(cwd); cache.set(cwd, spans);
      if (cache.size > 8) cache.delete(cache.keys().next().value!);
      options.changed();
    }).catch(() => { /* Transient failure retains the last successful snapshot. */ }).finally(() => {
      if (flights.get(cwd) === flight) flights.delete(cwd);
      if (flight.retry) refresh(false, cwd);
    });
  };
  function tick() {
    timer = undefined;
    deferred = false;
    due = Date.now() + GIT_IDLE_MS;
    if (options.isIdle()) refresh(true);
    arm();
  }
  arm();
  return {
    spans(cwd: string): readonly GitSpan[] { return cache.get(resolve(cwd)) ?? []; },
    turnEnd() { refresh(false); },
    input() {
      if (stopped) return;
      const flight = flights.get(key());
      if (!deferred && Date.now() + GIT_INPUT_DELAY_MS < due && !flight?.idle) return;
      if (flight?.idle) flight.abort.abort();
      deferred = true; due = Date.now() + GIT_INPUT_DELAY_MS; arm();
    },
    stop() { stopped = true; if (timer) clearTimeout(timer); timer = undefined; for (const flight of flights.values()) flight.abort.abort(); cache.clear(); },
  };
}
