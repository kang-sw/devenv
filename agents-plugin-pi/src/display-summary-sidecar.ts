/** Append-only, cosmetic display state. Never changes Pi's conversation file. */
import { constants } from "node:fs";
import * as fs from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import type { SessionEntry } from "@earendil-works/pi-coding-agent";
import { summaryItemsForMessage, type DisplaySummary } from "./display-summary.ts";

export const DISPLAY_SUMMARY_SIDECAR_SUFFIX = ".ws-display-summaries.log";
export type SidecarIO = Pick<typeof fs, "open" | "lstat" | "readdir" | "unlink">;

export interface SummaryConversation {
  sessionId: string;
  sessionFile?: string;
  /** All retained history, not the active/compaction-aware branch. */
  entries(): readonly SessionEntry[];
}

interface SidecarRecord {
  version: 1;
  sessionId: string;
  id: string;
  summary: DisplaySummary;
}

function object(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
}

function text(value: unknown): value is string {
  return typeof value === "string" && value.trim().length > 0;
}

function record(value: unknown): SidecarRecord | undefined {
  const raw = object(value);
  const summary = object(raw?.summary);
  if (raw?.version !== 1 || !text(raw.sessionId) || !text(raw.id) || !summary ||
    !text(summary.toolIntention) || !text(summary.toolResult) ||
    (summary.optionalContext !== undefined && typeof summary.optionalContext !== "string")) return undefined;
  return { version: 1, sessionId: raw.sessionId, id: raw.id, summary: {
    toolIntention: summary.toolIntention, toolResult: summary.toolResult,
    ...(summary.optionalContext === undefined ? {} : { optionalContext: summary.optionalContext as string }),
  } };
}

function parseLines(data: string): unknown[] {
  return data.split("\n").filter((line) => line.trim()).map((line) => {
    try { return JSON.parse(line); } catch { return undefined; }
  });
}

/** IDs are renderer IDs, not entry IDs (except standalone custom entries). */
export function retainedSummaryIds(entries: readonly SessionEntry[], toolNames: ReadonlySet<string>): Set<string> {
  const ids = new Set<string>();
  for (const entry of entries) {
    if (entry.type === "message") {
      const message = entry.message;
      if (message.role === "toolResult" && toolNames.has(message.toolName) && text(message.toolCallId)) ids.add(message.toolCallId);
      if (message.role === "custom") for (const item of summaryItemsForMessage(message)) ids.add(item.id);
    } else if (entry.type === "custom_message") {
      for (const item of summaryItemsForMessage(entry)) ids.add(item.id);
    } else if (entry.type === "custom" && entry.customType !== "ws-lead-compaction-history") {
      ids.add(entry.id);
    }
  }
  return ids;
}

async function regular(path: string, io: SidecarIO): Promise<boolean> {
  try { return (await io.lstat(path)).isFile(); } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return false;
    throw error;
  }
}

/** Do not follow a final-component symlink, including one swapped after lstat. */
async function readRegular(path: string, io: SidecarIO): Promise<string> {
  if (!await regular(path, io)) throw new Error("Not a regular file");
  const handle = await io.open(path, constants.O_RDONLY | constants.O_NOFOLLOW);
  try {
    if (!(await handle.stat()).isFile()) throw new Error("Not a regular file");
    return await handle.readFile("utf8");
  } finally { await handle.close(); }
}

async function conversationFile(path: string, io: SidecarIO): Promise<{ sessionId: string; parentSession?: string; entries: SessionEntry[] }> {
  const values = parseLines(await readRegular(path, io));
  const header = object(values[0]);
  if (header?.type !== "session" || !text(header.id)) throw new Error("Invalid Pi session header");
  return {
    sessionId: header.id,
    ...(text(header.parentSession) ? { parentSession: resolve(header.parentSession) } : {}),
    entries: values.slice(1).filter((value): value is SessionEntry => Boolean(object(value) && text(object(value)?.id) && text(object(value)?.type))),
  };
}

interface LoadedSidecar {
  exists: boolean;
  usable: boolean;
  data: string;
  summaries: Map<string, DisplaySummary>;
}

async function loadSidecar(path: string, sessionId: string, ids: ReadonlySet<string>, io: SidecarIO): Promise<LoadedSidecar> {
  let data: string;
  try {
    const stat = await io.lstat(path);
    if (!stat.isFile()) return { exists: true, usable: false, data: "", summaries: new Map() };
    data = await readRegular(path, io);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return { exists: false, usable: true, data: "", summaries: new Map() };
    throw error;
  }
  const lines = parseLines(data);
  // A supported ownership marker recognizes the file even if its value is bad.
  // A wholly unknown file is never repaired, replaced, or mixed with our data.
  const usable = !data.trim() || lines.some((value) => {
    const raw = object(value);
    return raw?.version === 1 && raw.sessionId === sessionId;
  });
  const summaries = new Map<string, DisplaySummary>();
  if (usable) for (const value of lines) {
    const row = record(value);
    if (row?.sessionId === sessionId && ids.has(row.id)) summaries.set(row.id, row.summary);
  }
  return { exists: true, usable, data, summaries };
}

/** Only direct children, consistent supported ownership, and absent conversations. */
export async function cleanOrphanSummarySidecars(sessionFile: string, io: SidecarIO = fs): Promise<void> {
  const directory = dirname(sessionFile);
  try {
    for (const name of await io.readdir(directory)) {
      if (!name.endsWith(DISPLAY_SUMMARY_SIDECAR_SUFFIX)) continue;
      const sidecar = join(directory, name);
      const conversation = sidecar.slice(0, -DISPLAY_SUMMARY_SIDECAR_SUFFIX.length);
      try {
        // lstat presence includes symlinks and directories: neither is an orphan.
        try { await io.lstat(conversation); continue; } catch (error) {
          if ((error as NodeJS.ErrnoException).code !== "ENOENT") continue;
        }
        const values = parseLines(await readRegular(sidecar, io));
        const rows = values.map(record);
        if (!rows.length || rows.some((row) => !row) || rows.some((row) => row!.sessionId !== rows[0]!.sessionId)) continue;
        // Recheck absence and the sidecar's type immediately before mutation.
        try { await io.lstat(conversation); continue; } catch (error) {
          if ((error as NodeJS.ErrnoException).code !== "ENOENT") continue;
        }
        if (await regular(sidecar, io)) await io.unlink(sidecar);
      } catch { /* Unrecognized/unreadable caches stay untouched. */ }
    }
  } catch { /* Directory access is cosmetic too. */ }
}

export interface SummarySidecar {
  /** Startup replay, deliberately separate from unfinished model requests. */
  readonly ready: Promise<void>;
  accept(batch: ReadonlyMap<string, DisplaySummary>): void;
  /** Observe first save and backfill early accepted batches. */
  sync(): void;
  /** Accepted/restored values, including work not yet appended. */
  snapshot(): ReadonlyMap<string, DisplaySummary>;
  drain(): Promise<void>;
}

export interface SummarySidecarDeps {
  conversation: SummaryConversation;
  toolNames: ReadonlySet<string>;
  restore(batch: ReadonlyMap<string, DisplaySummary>): void;
  /** Only for native fork cutover; never a writable parent fallback. */
  nativeSource?: { sessionFile: string; summaries: ReadonlyMap<string, DisplaySummary> };
  io?: SidecarIO;
  /** A replacement on the same file must read after outgoing accepted writes. */
  startAfter?: Promise<void>;
}

/** Pi 1.0.4 exposes the already-created native child in session_shutdown. */
export async function seedNativeSummarySidecar(
  target: string,
  source: { sessionFile: string; summaries: ReadonlyMap<string, DisplaySummary> },
  toolNames: ReadonlySet<string>,
  io: SidecarIO = fs,
): Promise<void> {
  try {
    const child = await conversationFile(resolve(target), io);
    if (child.parentSession !== resolve(source.sessionFile) || resolve(target) === resolve(source.sessionFile)) return;
    const cache = createSummarySidecar({
      conversation: { sessionId: child.sessionId, sessionFile: target, entries: () => child.entries },
      toolNames, nativeSource: source, restore: () => {}, io,
    });
    await cache.drain();
  } catch { /* An unsaved/unavailable native child stays memory-only. */ }
}

export function createSummarySidecar(deps: SummarySidecarDeps): SummarySidecar {
  const io = deps.io ?? fs;
  const { conversation } = deps;
  const path = conversation.sessionFile ? resolve(conversation.sessionFile) : undefined;
  const sidecar = path ? path + DISPLAY_SUMMARY_SIDECAR_SUFFIX : undefined;
  const values = new Map<string, DisplaySummary>();
  const live = new Set<string>();
  let pending: Map<string, DisplaySummary>[] = [];
  let initialized = false;
  let observedSaved = false;
  let disabled = false;
  let queue = deps.startAfter ?? Promise.resolve();

  async function available(): Promise<boolean> {
    if (!path || disabled) return false;
    const stat = await io.lstat(path).catch((error: NodeJS.ErrnoException) => {
      if (error.code === "ENOENT") return undefined;
      throw error;
    });
    if (!stat) {
      if (observedSaved) disabled = true;
      return false;
    }
    if (!stat.isFile()) { disabled = true; return false; }
    observedSaved = true;
    const header = await conversationFile(path, io);
    if (header.sessionId !== conversation.sessionId) { disabled = true; return false; }
    return true;
  }

  async function inherit(ids: ReadonlySet<string>): Promise<Map<string, DisplaySummary>> {
    const child = await conversationFile(path!, io);
    const sourcePath = child.parentSession;
    if (!sourcePath || sourcePath === path) return new Map();
    try {
      const source = await conversationFile(sourcePath, io);
      const sourceEntries = new Map(source.entries.map((entry) => [entry.id, entry]));
      // parentSession is the provenance; copied entry identity is the retained-row
      // filter. Equal session IDs/names by themselves prove nothing.
      const copied = child.entries.filter((entry) => {
        const original = sourceEntries.get(entry.id);
        return original?.type === entry.type;
      });
      const copiedIds = retainedSummaryIds(copied, deps.toolNames);
      const sourceIds = retainedSummaryIds(source.entries, deps.toolNames);
      const sourceSidecar = sourcePath + DISPLAY_SUMMARY_SIDECAR_SUFFIX;
      try { if (!(await io.lstat(sourceSidecar)).isFile()) return new Map(); } catch (error) {
        if ((error as NodeJS.ErrnoException).code !== "ENOENT") return new Map();
      }
      const cached = await loadSidecar(sourceSidecar, source.sessionId, sourceIds, io);
      const inherited = cached.summaries;
      if (deps.nativeSource && resolve(deps.nativeSource.sessionFile) === sourcePath) {
        for (const [id, summary] of deps.nativeSource.summaries) if (sourceIds.has(id)) inherited.set(id, summary);
      }
      return new Map([...inherited].filter(([id]) => ids.has(id) && copiedIds.has(id)));
    } catch { return new Map(); }
  }

  async function append(batch: ReadonlyMap<string, DisplaySummary>): Promise<void> {
    if (!sidecar || !batch.size || !await available()) return;
    const ids = retainedSummaryIds(conversation.entries(), deps.toolNames);
    const eligible = [...batch].filter(([id]) => ids.has(id));
    if (!eligible.length) return;
    // Re-read on each append: also recognizes deletion/replacement/foreign data.
    const loaded = await loadSidecar(sidecar, conversation.sessionId, ids, io);
    if (!loaded.usable) { disabled = true; return; }
    const boundary = loaded.data && !loaded.data.endsWith("\n") ? "\n" : "";
    const data = boundary + eligible.map(([id, summary]) => JSON.stringify({ version: 1, sessionId: conversation.sessionId, id, summary })).join("\n") + "\n";
    if (!await available()) return;
    const handle = await io.open(sidecar, constants.O_WRONLY | constants.O_APPEND | constants.O_CREAT | constants.O_NOFOLLOW, 0o600);
    try {
      if (!(await handle.stat()).isFile()) throw new Error("Not a regular file");
      await handle.writeFile(data, "utf8");
    } finally { await handle.close(); }
  }

  async function sync(): Promise<void> {
    if (!await available()) return;
    const ids = retainedSummaryIds(conversation.entries(), deps.toolNames);
    if (!initialized) {
      const loaded = await loadSidecar(sidecar!, conversation.sessionId, ids, io);
      if (!loaded.usable) { disabled = true; return; }
      const restored = loaded.exists ? loaded.summaries : await inherit(ids);
      const replay = new Map<string, DisplaySummary>();
      for (const [id, summary] of restored) if (!live.has(id)) {
        values.set(id, summary);
        replay.set(id, summary);
      }
      initialized = true;
      deps.restore(replay);
      if (!loaded.exists && restored.size) {
        // Any newer acceptance is appended afterward, never overwritten by replay.
        await append(restored);
      }
    }
    // One append per accepted batch; an unsaved conversation retains the batches
    // until its first real file exists. Failed I/O never clears live summaries.
    while (pending.length && !disabled) {
      const batch = pending[0]!;
      await append(batch);
      if (disabled) return;
      pending.shift();
    }
  }

  function schedule(): Promise<void> {
    queue = queue.then(sync).catch(() => { /* No cache I/O escapes the cosmetic path. */ });
    return queue;
  }
  const ready = schedule();
  return {
    ready,
    accept(batch) {
      const copy = new Map(batch);
      for (const [id, summary] of copy) { values.set(id, summary); live.add(id); }
      pending.push(copy);
      void schedule();
    },
    sync() { void schedule(); },
    snapshot: () => new Map(values),
    drain: () => schedule(),
  };
}
