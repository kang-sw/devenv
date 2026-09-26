/** Read-only, durable child-session telemetry.  This deliberately does not
 * use SessionManager: opening a production history can migrate/write it. */
import { createHash } from "node:crypto";
import { closeSync, fstatSync, openSync, readFileSync, readSync } from "node:fs";
import { isDeepStrictEqual } from "node:util";

export interface TelemetryOrigin { sessionId: string; sessionPath: string; prefixEntryId?: string; emptyPrefix?: true }
/**
 * Bounded cumulative cost estimate: the footer's unit and the unit a hop
 * reports upward as its descendant usage. `unknownContributors` makes an
 * incomplete sum render as `+ ?`; `descendants` counts contributing agents.
 */
export interface CumulativeCost {
  knownUsd: number;
  knownContributors: number;
  unknownContributors: number;
  descendants: number;
}
/** The footer's rendering of a cumulative cost: `—` when nothing is known, `~$N.NN` with ` + ?` when incomplete. */
export function formatCumulativeCost(cost: CumulativeCost): string {
  if (cost.knownContributors === 0 && cost.unknownContributors > 0) return "—";
  const known = `~$${cost.knownUsd.toFixed(2)}`;
  return cost.unknownContributors > 0 ? `${known} + ?` : known;
}
export interface AgentTelemetry {
  version: 1;
  origin: TelemetryOrigin;
  model?: string;
  effort?: string;
  /** Current context-window occupancy. Pi's live ContextUsage.tokens overrides the latest-call usage fallback. */
  contextTokens?: number;
  /** Complete child-attributable cumulative estimate. Existing widget semantics read only this field. */
  estimatedUsd?: number;
  /** Known subtotal when one or more attributable usage entries have unknown cost. */
  partialEstimatedUsd?: number;
  /**
   * The child's last reported usage of everything below it, excluding its own
   * usage (which the fields above reduce from its session). Legacy location:
   * records written before `OwnershipMetadata.descendantUsage` carry it here.
   * It is still parsed and read as a fallback, and no longer written.
   */
  descendantUsage?: CumulativeCost;
}
type Entry = { id: string; type: string; message?: { role?: string; usage?: unknown }; usage?: unknown };
/** One read of a child session: its entries, a `transient` absence of evidence, or `undefined` for a readable contradiction. */
export type SessionEntriesRead = { headerId: string; parentSession?: string; entries: Entry[] } | { transient: true } | undefined;

const nonnegative = (v: unknown): number | undefined => typeof v === "number" && Number.isFinite(v) && v >= 0 ? v : undefined;
const nonnegativeInteger = (v: unknown): number | undefined => { const n = nonnegative(v); return n !== undefined && Number.isSafeInteger(n) ? n : undefined; };
export function parseCumulativeCost(value: unknown): CumulativeCost | undefined {
  const cost = value as Partial<CumulativeCost> | null;
  if (!cost || typeof cost !== "object") return undefined;
  const knownUsd = nonnegative(cost.knownUsd);
  const knownContributors = nonnegativeInteger(cost.knownContributors);
  const unknownContributors = nonnegativeInteger(cost.unknownContributors);
  const descendants = nonnegativeInteger(cost.descendants);
  return knownUsd === undefined || knownContributors === undefined || unknownContributors === undefined || descendants === undefined
    ? undefined : { knownUsd, knownContributors, unknownContributors, descendants };
}
/**
 * Monotonic merge of one agent's cumulative cost: a regressing or unknown
 * observation keeps the previous known floor and marks the sum incomplete.
 * Shared by the owner estimate and the eviction record, whose rewrite merges
 * with the existing record rather than letting the last write win.
 */
export function mergeCumulativeCost(previous: CumulativeCost | undefined, observed: CumulativeCost): CumulativeCost {
  if (!previous) return { ...observed };
  const regressed = observed.knownUsd < previous.knownUsd;
  if (!regressed && observed.knownContributors > 0) return { ...observed };
  return {
    knownUsd: previous.knownUsd,
    knownContributors: previous.knownContributors,
    unknownContributors: 1,
    descendants: 1,
  };
}
function usageOf(value: unknown): { contextTokens?: number; cost?: number } | undefined {
  if (!value || typeof value !== "object") return undefined;
  const u = value as { input?: unknown; output?: unknown; cacheRead?: unknown; cacheWrite?: unknown; totalTokens?: unknown; cost?: { total?: unknown } };
  const explicitTotal = nonnegative(u.totalTokens);
  const parts = [u.input, u.output, u.cacheRead, u.cacheWrite].map(nonnegative).filter((part): part is number => part !== undefined);
  const summed = parts.length > 0 ? parts.reduce((total, part) => total + part, 0) : undefined;
  const contextTokens = explicitTotal ?? (summed !== undefined && Number.isFinite(summed) ? summed : undefined);
  const cost = nonnegative(u.cost?.total);
  return contextTokens === undefined && cost === undefined ? undefined : { contextTokens, cost };
}
export function parseTelemetry(value: unknown): AgentTelemetry | undefined {
  const t = value as Partial<AgentTelemetry> | null;
  if (!t || t.version !== 1 || !t.origin || typeof t.origin.sessionId !== "string" || !t.origin.sessionId || typeof t.origin.sessionPath !== "string" || !t.origin.sessionPath) return undefined;
  const o = t.origin;
  if (o.prefixEntryId !== undefined && (typeof o.prefixEntryId !== "string" || !o.prefixEntryId)) return undefined;
  if (!o.emptyPrefix && !o.prefixEntryId) return undefined;
  const out: AgentTelemetry = { version: 1, origin: { sessionId: o.sessionId, sessionPath: o.sessionPath, ...(o.prefixEntryId ? { prefixEntryId: o.prefixEntryId } : { emptyPrefix: true }) } };
  if (typeof t.model === "string" && t.model) out.model = t.model;
  if (typeof t.effort === "string" && t.effort) out.effort = t.effort;
  for (const k of ["contextTokens", "estimatedUsd", "partialEstimatedUsd"] as const) { const n = nonnegative(t[k]); if (n !== undefined) out[k] = n; }
  if (out.estimatedUsd !== undefined && out.partialEstimatedUsd !== undefined) delete out.partialEstimatedUsd;
  const descendantUsage = parseCumulativeCost(t.descendantUsage);
  if (descendantUsage) out.descendantUsage = descendantUsage;
  return out;
}
export function readSessionEntries(path: string): SessionEntriesRead {
  // Missing or unreachable files do not contradict an already validated origin.
  let raw: string; try { raw = readFileSync(path, "utf8"); } catch { return { transient: true }; }
  const lines = raw.split("\n"); if (lines.at(-1) === "") lines.pop();
  const parsed: unknown[] = [];
  for (let i = 0; i < lines.length; i++) { try { parsed.push(JSON.parse(lines[i])); } catch { return i === lines.length - 1 ? { transient: true } : undefined; } }
  const h = parsed.shift() as { type?: unknown; version?: unknown; id?: unknown; parentSession?: unknown } | undefined;
  if (!h || h.type !== "session" || h.version !== 3 || typeof h.id !== "string" || !h.id) return undefined;
  const entries: Entry[] = [];
  const byId = new Map<string, Entry>();
  for (const raw of parsed) {
    if (!raw || typeof raw !== "object" || typeof (raw as Entry).id !== "string" || !(raw as Entry).id) return undefined;
    const e = raw as Entry, prior = byId.get(e.id);
    if (prior) { if (!isDeepStrictEqual(prior, e)) return undefined; continue; }
    byId.set(e.id, e); entries.push(e);
  }
  return { headerId: h.id, ...(typeof h.parentSession === "string" && h.parentSession ? { parentSession: h.parentSession } : {}), entries };
}
/**
 * The fields of an entry that `reduceTelemetry` and `refreshAgentTelemetry`
 * read, and nothing else: a retained entry must not hold a child's tool
 * payloads. Absent, `null`, and present stay distinct for `message`,
 * `message.usage`, and `usage`; a non-object `message` projects to `{}`, which
 * reads the same (no `role`, no `usage`). A new reader of entries widens this.
 */
export function projectEntry(e: Entry): Entry {
  const out: Entry = { id: e.id, type: e.type };
  if (Object.hasOwn(e, "message")) {
    const m: unknown = e.message;
    out.message = m === null ? m as never : typeof m !== "object" ? {} : {
      ...(Object.hasOwn(m, "role") ? { role: (m as { role?: string }).role } : {}),
      ...(Object.hasOwn(m, "usage") ? { usage: (m as { usage?: unknown }).usage } : {}),
    };
  }
  if (Object.hasOwn(e, "usage")) out.usage = e.usage;
  return out;
}

/** One refresh's byte-range view of a session file. `read` may return fewer bytes when the file shrank meanwhile. */
export interface SessionFileHandle { size: number; read(start: number, length: number): Buffer; close(): void }
/** Byte-range access to session files; injectable so a test can observe which ranges a refresh reads. */
export interface SessionFileIo { open(path: string): SessionFileHandle | undefined }
export const nodeSessionFileIo: SessionFileIo = {
  open(path) {
    let fd: number; try { fd = openSync(path, "r"); } catch { return undefined; }
    let size: number; try { size = fstatSync(fd).size; } catch { closeSync(fd); return undefined; }
    return {
      size,
      read(start, length) {
        const buf = Buffer.alloc(length); let n = 0;
        while (n < length) { const r = readSync(fd, buf, n, length - n, start + n); if (r === 0) break; n += r; }
        return n < length ? buf.subarray(0, n) : buf;
      },
      close() { closeSync(fd); },
    };
  },
};

const NEWLINE = 0x0a;
const lineHash = (bytes: Uint8Array, withNewline = false): string => { const h = createHash("sha256").update(bytes); if (withNewline) h.update("\n"); return h.digest("base64"); };
const parseLine = (bytes: Buffer): { value: unknown } | undefined => { try { return { value: JSON.parse(bytes.toString("utf8")) }; } catch { return undefined; } };
const validEntry = (v: unknown): v is Entry => !!v && typeof v === "object" && typeof (v as Entry).id === "string" && !!(v as Entry).id;
/** The first-occurrence lookup of the conflicting-duplicate fallback could not locate the entry: the file changed under the read. */
class ReplacedUnderRead extends Error {}

/**
 * Result-equivalent to `readSessionEntries`, but a refresh reads only the
 * bytes appended since the previous one. It relies on Pi's session writer
 * being append-only in steady state and resets to byte 0 on every non-append
 * shape that writer produces: a path change, a missing or unreadable file, a
 * file whose size no longer exceeds the consumed offset, a header line that differs
 * from the consumed one (a migration rewrite keeps the id and changes the
 * version), or a consumed boundary that no longer ends in a newline.
 *
 * The last line of the file is never consumed: it is parsed on every read, so
 * a torn tail reads `transient` and an unterminated tail that parses is an
 * entry, exactly as the full read classifies them. Consumed lines keep only
 * `projectEntry` projections plus one raw-line hash per id; a same-id line
 * with a different hash triggers a one-off full-file lookup that applies the
 * full read's `isDeepStrictEqual` rule, and the (id, hash) verdict is cached.
 * An invalid verdict from a consumed line is cached until a reset.
 */
export class IncrementalSessionReader {
  private path: string | undefined;
  /** Start of the file's last line: every byte before it is consumed. */
  private offset = 0;
  /** Length (with its newline) and hash of the consumed raw header line. */
  private header: { length: number; hash: string } | undefined;
  private headerId: string | undefined;
  private parentSession: string | undefined;
  private entries: Entry[] = [];
  /** Raw-line hash of each id's first occurrence. */
  private hashes = new Map<string, string>();
  /** `${id}\n${hash}` of a later same-id line → whether it deep-equals the first occurrence. */
  private variants = new Map<string, boolean>();
  /** Sticky verdict over the consumed prefix: a line failed to parse, or a readable contradiction. */
  private verdict: "unparseable" | "invalid" | undefined;

  private readonly io: SessionFileIo;

  constructor(io: SessionFileIo = nodeSessionFileIo) { this.io = io; }

  read(path: string): SessionEntriesRead {
    if (path !== this.path) this.reset(path);
    const file = this.io.open(path);
    if (!file) { this.reset(path); return { transient: true }; }
    try { return this.readOpen(file); }
    catch { this.reset(path); return { transient: true }; }
    finally { file.close(); }
  }

  private reset(path: string): void {
    this.path = path; this.offset = 0; this.header = undefined; this.verdict = undefined;
    this.headerId = undefined; this.parentSession = undefined;
    this.entries = []; this.hashes = new Map(); this.variants = new Map();
  }

  private invalidate(verdict: "unparseable" | "invalid"): void {
    if (this.verdict !== "unparseable") this.verdict = verdict;
    this.headerId = undefined; this.parentSession = undefined;
    this.entries = []; this.hashes = new Map(); this.variants = new Map();
  }

  private boundaryIntact(file: SessionFileHandle): boolean {
    if (file.size <= this.offset || !this.header) return false;
    const header = file.read(0, this.header.length);
    if (header.length !== this.header.length || lineHash(header) !== this.header.hash) return false;
    if (this.offset === this.header.length) return true;
    const boundary = file.read(this.offset - 1, 1);
    return boundary.length === 1 && boundary[0] === NEWLINE;
  }

  private readOpen(file: SessionFileHandle): SessionEntriesRead {
    if (this.offset > 0 && !this.boundaryIntact(file)) this.reset(this.path!);
    // A consumed line that fails to parse is never the last line again.
    if (this.verdict === "unparseable") return undefined;
    const tail = file.read(this.offset, Math.max(0, file.size - this.offset));
    let start = 0;
    for (let nl = tail.indexOf(NEWLINE); nl >= 0 && nl < tail.length - 1; nl = tail.indexOf(NEWLINE, start)) {
      this.consume(tail.subarray(start, nl), file);
      start = nl + 1;
    }
    const lastEnd = tail.at(-1) === NEWLINE ? tail.length - 1 : tail.length;
    if (start === tail.length) return undefined; // An empty file has no header.
    return this.evaluateLast(tail.subarray(start, lastEnd), file);
  }

  private consume(bytes: Buffer, file: SessionFileHandle): void {
    const isHeader = this.offset === 0;
    if (isHeader) this.header = { length: bytes.length + 1, hash: lineHash(bytes, true) };
    this.offset += bytes.length + 1;
    if (this.verdict === "unparseable") return;
    const parsed = parseLine(bytes);
    if (!parsed) { this.invalidate("unparseable"); return; }
    if (this.verdict === "invalid") return;
    if (isHeader) {
      const header = this.headerOf(parsed.value);
      if (!header) { this.invalidate("invalid"); return; }
      this.headerId = header.id; this.parentSession = header.parentSession;
      return;
    }
    const admitted = this.admit(parsed.value, bytes, file);
    if (admitted === undefined) { this.invalidate("invalid"); return; }
    if (admitted) { this.hashes.set(admitted.entry.id, admitted.hash); this.entries.push(projectEntry(admitted.entry)); }
  }

  /** The last line decides `transient`, then the consumed prefix's verdict, then its own validity; nothing of it is retained. */
  private evaluateLast(bytes: Buffer, file: SessionFileHandle): SessionEntriesRead {
    if (this.verdict === "unparseable") return undefined;
    const parsed = parseLine(bytes);
    if (!parsed) return { transient: true };
    if (this.verdict) return undefined;
    if (this.offset === 0) {
      const header = this.headerOf(parsed.value);
      return header ? { headerId: header.id, ...(header.parentSession ? { parentSession: header.parentSession } : {}), entries: [] } : undefined;
    }
    const admitted = this.admit(parsed.value, bytes, file);
    if (admitted === undefined) return undefined;
    return {
      headerId: this.headerId!,
      ...(this.parentSession ? { parentSession: this.parentSession } : {}),
      entries: admitted ? [...this.entries, projectEntry(admitted.entry)] : [...this.entries],
    };
  }

  private headerOf(value: unknown): { id: string; parentSession?: string } | undefined {
    const h = value as { type?: unknown; version?: unknown; id?: unknown; parentSession?: unknown } | null;
    if (!h || h.type !== "session" || h.version !== 3 || typeof h.id !== "string" || !h.id) return undefined;
    return { id: h.id, ...(typeof h.parentSession === "string" && h.parentSession ? { parentSession: h.parentSession } : {}) };
  }

  /** A new entry, `false` for a duplicate equal to its first occurrence, `undefined` for a contradiction. */
  private admit(value: unknown, bytes: Buffer, file: SessionFileHandle): { entry: Entry; hash: string } | false | undefined {
    if (!validEntry(value)) return undefined;
    const hash = lineHash(bytes), prior = this.hashes.get(value.id);
    if (prior === undefined) return { entry: value, hash };
    if (prior === hash) return false;
    const key = `${value.id}\n${hash}`;
    let equal = this.variants.get(key);
    if (equal === undefined) { equal = this.equalsFirstOccurrence(value, file); this.variants.set(key, equal); }
    return equal ? false : undefined;
  }

  /** The conflicting-duplicate fallback: one full read that applies the full reader's rule to the id's first occurrence. */
  private equalsFirstOccurrence(e: Entry, file: SessionFileHandle): boolean {
    const lines = file.read(0, file.size).toString("utf8").split("\n");
    for (let i = 1; i < lines.length; i++) {
      let v: unknown; try { v = JSON.parse(lines[i]); } catch { continue; }
      if (validEntry(v) && v.id === e.id) return isDeepStrictEqual(v, e);
    }
    throw new ReplacedUnderRead();
  }
}

/** Recomputes, never adds. Unavailable or invalid input returns undefined; callers classify the read before choosing fallback. */
export function reduceTelemetry(origin: TelemetryOrigin, read: SessionEntriesRead): Pick<AgentTelemetry, "contextTokens" | "estimatedUsd" | "partialEstimatedUsd"> | undefined {
  if (!read || "transient" in read || read.headerId !== origin.sessionId) return undefined;
  let start = 0;
  if (origin.prefixEntryId) { const at = read.entries.findIndex(e => e.id === origin.prefixEntryId); if (at < 0) return undefined; start = at + 1; }
  else if (!origin.emptyPrefix) return undefined;
  let total = 0, observedCost = false, invalidCost = false, contextTokens: number | undefined;
  for (const e of read.entries.slice(start)) {
    const assistant = e.type === "message" && e.message?.role === "assistant";
    const summary = e.type === "compaction" || e.type === "branch_summary";
    const rawUsage = e.message?.usage ?? e.usage;
    const usage = usageOf(rawUsage);
    if (!usage) { if (assistant || (rawUsage !== undefined && (summary || e.type === "message"))) invalidCost = true; if (assistant || summary) contextTokens = undefined; continue; }
    if (assistant) contextTokens = usage.contextTokens; // a later summary makes pre-compaction occupancy unknown.
    if (summary) contextTokens = undefined;
    if (usage.cost === undefined) invalidCost = true; else { observedCost = true; total += usage.cost; }
  }
  return {
    ...(contextTokens !== undefined ? { contextTokens } : {}),
    ...(!invalidCost && observedCost ? { estimatedUsd: total } : invalidCost && observedCost ? { partialEstimatedUsd: total } : {}),
  };
}
export function refreshTelemetry(snapshot: AgentTelemetry): AgentTelemetry | undefined {
  const reduced = reduceTelemetry(snapshot.origin, readSessionEntries(snapshot.origin.sessionPath));
  if (reduced === undefined) return undefined;
  const next = { ...snapshot };
  // Context is a last-valid snapshot: a compaction or provider gap makes the
  // current value unknown, but must not replace a valid same-session value
  // with a false near-zero fallback. Cost remains a full recomputation.
  delete next.estimatedUsd; delete next.partialEstimatedUsd;
  return { ...next, ...reduced };
}
