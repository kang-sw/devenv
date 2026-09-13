/**
 * `260908-feat-ws-pi-subagent-audit-window-and-owner-steering`: `/audit`,
 * session-backed conversation binding, and the shared owner-steering shell.
 * Phase 1 supplied view mode; Phase 2 adds record-attributed owner sends,
 * last-writer ownership, and the hold/finish/interrupt modal reused by the
 * fork-raised `/answer` path.
 *
 * Registration gate is STRICTER than `agent-widget.ts`'s
 * `shouldArmAgentWidget` (`isLeadOrFork`): the ticket's own wording is
 * "nothing is registered in child processes or headless leads", and a
 * `fork` process is itself a child process. `shouldRegisterAudit` therefore
 * requires `role === undefined` (a true lead — `process-role.ts`'s own doc:
 * "the host lead process carries no marker at all") AND `mode === "tui"`.
 * `pi.registerCommand`/`pi.registerShortcut` are never called when the gate
 * is false — `registerAuditCommands` returns before either call, not merely
 * guarding their handlers internally.
 *
 * Placement: `ask.ts` imports the record binding and steering shell from this
 * module; `agent-widget.ts` therefore keeps its `ask.ts` dependency type-only
 * to avoid a runtime cycle.
 */

import { readFileSync } from "node:fs";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { getMarkdownTheme, getSelectListTheme } from "@earendil-works/pi-coding-agent";
import { isOwnerHeld, lastActivityAt, resolveAgentId, sendToAgent, type RpcAgentRecord, type RpcAgentRegistry } from "./spawner.ts";
import { touchOwnership } from "./agent-storage.ts";
import { AGENT_STATE_LABEL, AGENT_STATE_RANK, classifyRegistryRowState, formatCompactDuration, formatContextTokens, rowName, type AgentRowState } from "./agent-widget.ts";
import {
  ConversationViewComponent,
  conversationOverlayHeight,
  isEscapeKey,
  toolResultContentText,
  wrapInBorder,
  type ConversationChannel,
  type ConversationItem,
  type ConversationViewTui,
} from "./conversation-view.ts";
import { loadHostPiTui, SelectList, truncateToWidth, visibleWidth, type Component, type MarkdownTheme, type SelectItem, type SelectListTheme } from "./pi-tui.ts";
import type { SpawnRole } from "./process-role.ts";

// ---------------------------------------------------------------------------
// Session-file parsing. Pure, best-effort, unit-tested directly against a
// fixture file (`test/audit.test.ts`) with no live `pi` session.
// ---------------------------------------------------------------------------

/** Minimal shape of one session-file line this parser cares about — see `node_modules/@earendil-works/pi-coding-agent/docs/session-format.md`. Every other `type`/`role` is skipped (Phase 1's explicit mapping contract; see the plan's Escalations). */
interface SessionMessageEntry {
  type?: string;
  timestamp?: string;
  message?: {
    role?: string;
    content?: unknown;
    toolCallId?: string;
    toolName?: string;
    isError?: boolean;
  };
}

/** A single `assistant` content block: `{type:"text",text}` | `{type:"toolCall",id,name,arguments}` | `{type:"thinking",...}` (dropped). */
interface AssistantContentBlock {
  type?: string;
  text?: string;
  id?: string;
  name?: string;
  arguments?: unknown;
}

/** `UserMessage.content`'s `string | (TextContent|ImageContent)[]` — joins every `"text"` block, dropping images (best-effort; Phase 1 has no image rendering). */
function joinTextContent(content: unknown): string {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  return content
    .filter((block): block is { type?: string; text?: string } => typeof block === "object" && block !== null)
    .filter((block) => block.type === "text" && typeof block.text === "string")
    .map((block) => block.text as string)
    .join("");
}

/**
 * Reads and parses `path` (the child's own Pi session file) into
 * `ConversationItem`s. Best-effort throughout, per the codebase's
 * established convention (`fork.ts`'s `tailLines(readFileSync(...))`,
 * `spawner.ts:357`'s per-line `JSON.parse`): a missing/unreadable file
 * yields `[]`; a malformed line is skipped, not fatal to the rest of the
 * parse.
 *
 * Mapping: `role:"user"` becomes `"user"` only when it consumes the next
 * matching record-owned `ownerSends` entry; other user messages are
 * `"lead-message"`. `role:"assistant"` becomes one
 * `"assistant"` item per `text` content block (thinking blocks dropped)
 * plus one `"tool-call"` item per `toolCall` block, in original order;
 * `role:"toolResult"` -> one `"tool-result"` item, its `content` converted
 * through `conversation-view.ts`'s own `toolResultContentText` (identical
 * `{content?: unknown}` shape to the live `tool_execution_end` event this
 * helper already converts). Every other entry `type` (`model_change`,
 * `compaction`, `branch_summary`, session header, ...) and every other
 * `AgentMessage` role (`bashExecution`, `custom`, `branchSummary`,
 * `compactionSummary`) is skipped — not part of the ticket's Phase 1
 * mapping contract.
 */
export interface SessionHistoryRead {
  status: "available" | "unavailable";
  items: ConversationItem[];
}

export function readSessionHistory(path: string, ownerSends: readonly { text: string; at: number }[] = []): SessionHistoryRead {
  let raw: string;
  try {
    raw = readFileSync(path, "utf8");
  } catch {
    return { status: "unavailable", items: [] };
  }

  const items: ConversationItem[] = [];
  let nextOwnerSend = 0;
  for (const line of raw.split("\n")) {
    if (!line.trim()) continue;
    let entry: SessionMessageEntry;
    try {
      entry = JSON.parse(line) as SessionMessageEntry;
    } catch {
      continue;
    }
    const message = entry?.message;
    if (entry?.type !== "message" || !message || typeof message !== "object") continue;

    if (message.role === "user") {
      const text = joinTextContent(message.content);
      const ownerSend = ownerSends[nextOwnerSend];
      const entryAt = typeof entry.timestamp === "string" ? Date.parse(entry.timestamp) : Number.NaN;
      const afterOwnerDispatch = !Number.isFinite(entryAt) || ownerSend === undefined || entryAt >= ownerSend.at;
      if (ownerSend && ownerSend.text === text && afterOwnerDispatch) {
        items.push({ kind: "user", text });
        nextOwnerSend++;
      } else {
        items.push({ kind: "lead-message", text });
      }
      continue;
    }

    if (message.role === "assistant") {
      const blocks = Array.isArray(message.content) ? (message.content as AssistantContentBlock[]) : [];
      for (const block of blocks) {
        if (!block || typeof block !== "object") continue;
        if (block.type === "text" && typeof block.text === "string") {
          items.push({ kind: "assistant", text: block.text });
        } else if (block.type === "toolCall" && typeof block.id === "string" && typeof block.name === "string") {
          items.push({ kind: "tool-call", id: block.id, name: block.name, args: block.arguments });
        }
        // "thinking" blocks (and any other block kind) are dropped — Phase 1's mapping contract.
      }
      continue;
    }

    if (message.role === "toolResult") {
      if (typeof message.toolCallId !== "string" || typeof message.toolName !== "string") continue;
      items.push({
        kind: "tool-result",
        id: message.toolCallId,
        name: message.toolName,
        content: toolResultContentText(message),
        isError: message.isError,
      });
      continue;
    }

    // Every other role (`bashExecution`, `custom`, `branchSummary`,
    // `compactionSummary`) is skipped — see this function's doc comment.
  }
  return { status: "available", items };
}

/** Compatibility parser for callers that need only available transcript items. */
export function parseSessionFile(path: string): ConversationItem[] {
  return readSessionHistory(path).items;
}

// ---------------------------------------------------------------------------
// Shared record-backed ConversationChannel. `/audit` and fork-raised
// `/answer` supply their owner-send closures to this same event/liveness seam.
// ---------------------------------------------------------------------------

/**
 * Builds a `ConversationChannel` over `rpcRegistry`'s live record for
 * `agentId`. `sync()` attaches to `record.client.onEvent` only when a
 * client exists — a dormant record (`client === undefined`) is never
 * attached, which is what makes "opening a dormant child resumes nothing"
 * true for free. `send` is present only when the caller supplies the shared
 * owner delivery closure.
 */
export function createAuditChannel(
  rpcRegistry: RpcAgentRegistry,
  agentId: string,
  send?: (text: string) => Promise<void>,
): ConversationChannel {
  const listeners = new Set<(evt: unknown) => void>();
  let attached: unknown;
  let detach: (() => void) | undefined;

  function sync(): void {
    const record = rpcRegistry.get(agentId);
    const client = record?.client;
    if (!client || client === attached) return;
    detach?.();
    attached = client;
    detach = client.onEvent((evt) => {
      for (const listener of listeners) listener(evt);
    });
  }

  return {
    onEvent(listener) {
      listeners.add(listener);
      sync();
      return () => {
        listeners.delete(listener);
        if (listeners.size === 0) {
          detach?.();
          detach = undefined;
          attached = undefined;
        }
      };
    },
    liveness() {
      const record = rpcRegistry.get(agentId);
      return record?.running ? "running" : isOwnerHeld(record) ? "idle-awaiting-owner" : "settled";
    },
    ...(send ? { send: async (text: string) => { await send(text); sync(); } } : {}),
  };
}

// ---------------------------------------------------------------------------
// The picker's row builder — the registry's three live tiers
// (`agent-widget.ts`'s own naming/ordering rules, reused verbatim) plus a
// fourth, picker-only dormant tier by last activity.
// ---------------------------------------------------------------------------

/**
 * One row per registry child, running AND dormant. Tiers 1-3 reuse
 * `agent-widget.ts`'s own `classifyRegistryRowState` and its three-state
 * ordering (awaiting-owner, awaiting-approval, running by elapsed
 * descending) verbatim. Tier 4 is picker-only: every `undefined` (dormant)
 * record, ordered by `spawner.ts`'s `lastActivityAt`, most-recent first —
 * the widget itself never rows a dormant child at all. Unlike
 * `agent-widget.ts`'s `buildAgentRows`, this reads the registry alone (no
 * `ThreadRecord` union) — the ticket scopes the picker to "the registry's
 * children," not `ask.ts`'s pending-thread rows.
 */
interface AuditPickerRow {
  agentId: string;
  identity: string;
  status: string;
  model: string;
  contextTokens: string;
  activity: string;
  state?: AgentRowState;
  elapsedMs: number;
  lastActivity?: number;
}

/** The widget's alias > title > short-id identity rule. */
function auditIdentity(record: RpcAgentRecord): string {
  return rowName(record);
}

function elapsedSince(now: number, timestamp: number): number {
  return Number.isFinite(timestamp) ? Math.max(0, now - timestamp) : 0;
}

/** Display validity is stricter than the sort key: spawner keeps zero/NaN
 * fallbacks for ordering, but neither describes a meaningful last activity. */
function formatDormantActivity(now: number, activityAt: number): string {
  if (!Number.isFinite(activityAt) || activityAt <= 0) return "last active —";
  return `last active ${formatCompactDuration(elapsedSince(now, activityAt))} ago`;
}

/**
 * Fits the three non-optional fields without ever leaving a partial telemetry
 * value behind. At very narrow widths the identity is the expendable part of
 * this protected group; only after that does normal whole-row truncation run.
 */
function fitProtectedAuditFields(identity: string, status: string, activity: string, width: number): string {
  if (width <= 0) return "";
  const separator = " · ";
  const suffix = `${status}${separator}${activity}`;
  const remainingIdentity = width - visibleWidth(separator) - visibleWidth(suffix);
  if (remainingIdentity >= 1) return `${truncateToWidth(identity, remainingIdentity)}${separator}${suffix}`;
  return truncateToWidth(`${identity}${separator}${suffix}`, width);
}

/** Formats one picker row, dropping whole optional telemetry fields in order. */
export function formatAuditPickerLabel(row: Omit<AuditPickerRow, "agentId" | "state" | "elapsedMs" | "lastActivity">, width = Number.POSITIVE_INFINITY): string {
  const separator = " · ";
  const complete = [row.identity, row.status, row.model, row.contextTokens, row.activity].join(separator);
  if (visibleWidth(complete) <= width) return complete;
  const withoutTokens = [row.identity, row.status, row.model, row.activity].join(separator);
  if (visibleWidth(withoutTokens) <= width) return withoutTokens;
  const protectedFields = [row.identity, row.status, row.activity].join(separator);
  if (visibleWidth(protectedFields) <= width) return protectedFields;
  return fitProtectedAuditFields(row.identity, row.status, row.activity, width);
}

export function buildAuditPickerItems(registry: RpcAgentRegistry, now: number, labelWidth = Number.POSITIVE_INFINITY): SelectItem[] {
  const live: AuditPickerRow[] = [];
  const dormant: AuditPickerRow[] = [];

  for (const record of registry.values()) {
    const state = classifyRegistryRowState(record);
    const identity = auditIdentity(record);
    const model = record.telemetry?.model ?? record.observedModel ?? "—";
    const contextTokens = formatContextTokens(record.telemetry?.contextTokens ?? record.observedContextTokens);
    if (state === undefined) {
      const activityAt = lastActivityAt(record);
      dormant.push({
        agentId: record.agentId,
        identity,
        status: "dormant",
        model,
        contextTokens,
        activity: formatDormantActivity(now, activityAt),
        elapsedMs: elapsedSince(now, activityAt),
        lastActivity: activityAt,
      });
      continue;
    }
    const elapsedMs = elapsedSince(now, record.runStartedAt ?? now);
    live.push({
      agentId: record.agentId,
      identity,
      status: AGENT_STATE_LABEL[state],
      model,
      contextTokens,
      activity: state === "idle-awaiting-owner"
        ? formatDormantActivity(now, lastActivityAt(record))
        : `running for ${formatCompactDuration(elapsedMs)}`,
      state,
      elapsedMs,
    });
  }

  live.sort((a, b) => {
    const rankDiff = AGENT_STATE_RANK[a.state!] - AGENT_STATE_RANK[b.state!];
    return rankDiff !== 0 ? rankDiff : b.elapsedMs - a.elapsedMs;
  });
  dormant.sort((a, b) => b.lastActivity! - a.lastActivity!);

  return [...live, ...dormant].map(({ agentId, identity, status, model, contextTokens, activity }) => ({
    value: agentId,
    label: formatAuditPickerLabel({ identity, status, model, contextTokens, activity }, labelWidth),
  }));
}

// ---------------------------------------------------------------------------
// IO glue: registration gate, the picker modal, and the viewer overlay. Not
// unit tested at the `registerAuditCommands` wiring level beyond the gate
// itself and the fake-`pi`-harness command/shortcut dispatch — the same
// live-glue split `ask.ts`'s own header comment describes.
// ---------------------------------------------------------------------------

/**
 * Duck-typed `ctx` surface for `ctx.ui.notify`/`ctx.mode`, mirroring
 * `ask.ts`'s own `AskUiCtx` convention.
 */
export interface AuditUiCtx {
  mode?: string;
  ui?: {
    notify?(message: string, type?: "info" | "warning" | "error"): void;
  };
}

/**
 * Duck-typed slice of `ctx.ui.custom`'s real signature, mirroring `ask.ts`'s
 * own (non-exported) `AskCustomUiCtx` — narrowed to what this module's
 * factories need: a `tui` structurally compatible with `ConversationViewTui`
 * (also `SelectList`'s `Component` surface), an optional `theme`, and a
 * factory that may resolve asynchronously (building the live component
 * awaits `loadHostPiTui()`).
 */
interface AuditCustomUiCtx {
  ui: {
    custom<T>(
      factory: (
        tui: ConversationViewTui,
        theme: { bg?(color: string, text: string): string; fg?(color: string, text: string): string } | undefined,
        keybindings: unknown,
        done: (result: T) => void,
      ) => Component | Promise<Component>,
      options?: { overlay?: boolean; overlayOptions?: unknown },
    ): Promise<T>;
  };
}

function notify(ctx: AuditUiCtx | undefined, message: string, type?: "info" | "warning" | "error"): void {
  ctx?.ui?.notify?.(message, type);
}

/**
 * 260908: the audit surface's own arming gate — STRICTER than
 * `agent-widget.ts`'s `shouldArmAgentWidget` (`isLeadOrFork`). The ticket's
 * literal wording is "nothing is registered in child processes or headless
 * leads": a `fork` process is itself a child process, so only a true lead
 * (`role === undefined` — `process-role.ts`'s own doc: "the host lead
 * process carries no marker at all") in a TUI session ever registers
 * `/audit`.
 */
export function shouldRegisterAudit(role: SpawnRole | undefined, mode: string | undefined): boolean {
  return role === undefined && mode === "tui";
}

const IDENTITY_SELECT_LIST_THEME: SelectListTheme = {
  selectedPrefix: (t) => t,
  selectedText: (t) => t,
  description: (t) => t,
  scrollInfo: (t) => t,
  noMatch: (t) => t,
};

/** The ask.ts/`openThread` overlay geometry, reused verbatim (the plan's literal instruction). */
const AUDIT_OVERLAY_OPTIONS = { overlay: true, overlayOptions: { width: "80%", maxHeight: "80%", anchor: "center" } } as const;

/**
 * Adds the conversation view's width-safe box chrome around a `SelectList`
 * without changing the list's selection/cancel behavior. The wrapper forwards
 * all input directly to the list and only requests a repaint after it acts.
 */
function wrapAuditPicker(
  itemsForLabelWidth: (width: number) => SelectItem[],
  tui: ConversationViewTui,
  header: string,
  theme: SelectListTheme,
  done: (result: string | undefined) => void,
): Component {
  let labelWidth = -1;
  let selectedValue: string | undefined;
  let list: SelectList | undefined;

  function ensureList(nextLabelWidth: number): SelectList {
    if (list && nextLabelWidth === labelWidth) return list;
    const items = itemsForLabelWidth(nextLabelWidth);
    const next = new SelectList(items, Math.min(10, items.length), theme);
    const selectedIndex = selectedValue === undefined ? 0 : items.findIndex((item) => item.value === selectedValue);
    next.setSelectedIndex(selectedIndex >= 0 ? selectedIndex : 0);
    next.onSelectionChange = (item) => { selectedValue = item.value; };
    next.onSelect = (item) => done(item.value);
    next.onCancel = () => done(undefined);
    list = next;
    labelWidth = nextLabelWidth;
    return next;
  }

  return {
    render(width: number): string[] {
      const w = Math.max(1, width);
      const innerWidth = Math.max(1, w - 4);
      // SelectList reserves two columns for its cursor plus two safety
      // columns before truncating a value. Give the formatter that exact
      // budget so model/token values are omitted whole rather than clipped.
      const currentList = ensureList(Math.max(0, innerWidth - 4));
      const innerLines = [header, "", ...currentList.render(innerWidth)].map((line) => truncateToWidth(line, innerWidth));
      return wrapInBorder(innerLines, w, innerWidth);
    },
    invalidate(): void {
      list?.invalidate();
    },
    handleInput(data: string): void {
      const currentList = ensureList(labelWidth < 0 ? 0 : labelWidth);
      currentList.handleInput(data);
      selectedValue = currentList.getSelectedItem()?.value ?? selectedValue;
      tui.requestRender();
    },
  };
}

/**
 * Opens the picker: one row per registry child (running and dormant, via
 * `buildAuditPickerItems`), arrow keys move, Enter selects, Esc cancels —
 * `SelectList`'s own `handleInput` already implements all three. Resolves
 * the selected `agentId`, or `undefined` on cancel / when there is nothing
 * to audit.
 */
export async function openPicker(ctx: AuditUiCtx & { ui?: { custom?: unknown } }, rpcRegistry: RpcAgentRegistry): Promise<string | undefined> {
  const items = buildAuditPickerItems(rpcRegistry, Date.now());
  if (items.length === 0) {
    notify(ctx, "ws: no live or recent subagents to audit.", "info");
    return undefined;
  }
  let theme: SelectListTheme | undefined;
  try {
    const candidate = getSelectListTheme();
    // The host's selector theme is lazily backed by its global TUI theme.
    // Touch it here so headless/test callers fall back before render rather
    // than leaving a deferred "Theme not initialized" failure in the picker.
    candidate.selectedText("");
    theme = candidate;
  } catch {
    // best effort — mirrors `ask.ts`'s own `getMarkdownTheme()` precedent.
  }
  return (ctx as unknown as AuditCustomUiCtx).ui.custom<string | undefined>(
    (tui, hostTheme, _keybindings, done) =>
      wrapAuditPicker(
        (labelWidth) => buildAuditPickerItems(rpcRegistry, Date.now(), labelWidth),
        tui,
        hostTheme?.fg?.("accent", "ws audit: select subagent") ?? "ws audit: select subagent",
        theme ?? IDENTITY_SELECT_LIST_THEME,
        done,
      ),
    AUDIT_OVERLAY_OPTIONS,
  );
}

/** One owner conversation overlay across `/audit` and `/answer`. */
export interface ActiveOwnerOverlay {
  token: number;
  close: () => void;
  threadId?: string;
  closeWithSummary?: (summary: string, alreadyRendered?: boolean) => void;
}

let activeOwnerOverlay: ActiveOwnerOverlay | undefined;
let ownerOverlayToken = 0;

export function reserveOwnerOverlay(): number {
  activeOwnerOverlay?.close();
  activeOwnerOverlay = undefined;
  return ++ownerOverlayToken;
}

export function activateOwnerOverlay(overlay: ActiveOwnerOverlay): void {
  if (overlay.token === ownerOverlayToken) activeOwnerOverlay = overlay;
}

export function clearOwnerOverlay(token: number): void {
  if (activeOwnerOverlay?.token === token) activeOwnerOverlay = undefined;
}

export function currentOwnerOverlay(): ActiveOwnerOverlay | undefined {
  return activeOwnerOverlay;
}

export const OWNER_FINISH_MESSAGE = "The owner has finished steering. Continue with the lead and report your result through the normal channel.";

type SteeringAction = "hold" | "finish" | "interrupt";
const STEERING_ACTIONS: readonly SteeringAction[] = ["hold", "finish", "interrupt"];
interface OwnerSteeringOptions {
  done(): void;
  finish(): Promise<void> | void;
  interrupt(): Promise<void> | void;
  interruptEnabled(): boolean;
  notify?(message: string, type?: "info" | "warning" | "error"): void;
  theme?: { fg?(color: string, text: string): string };
  matchesKey?: (data: string, keyId: string) => boolean;
}

/** Shared interactive Esc modal used by `/audit` and fork-raised `/answer`. */
export class OwnerSteeringComponent implements Component {
  private modal = false;
  private selected = 0;
  private busy = false;
  private readonly tui: ConversationViewTui;
  private readonly view: ConversationViewComponent;
  private readonly options: OwnerSteeringOptions;

  constructor(
    tui: ConversationViewTui,
    view: ConversationViewComponent,
    options: OwnerSteeringOptions,
  ) {
    this.tui = tui;
    this.view = view;
    this.options = options;
  }

  invalidate(): void { this.view.invalidate(); }
  getMode(): "view" | "interactive" { return this.view.getMode(); }

  render(width: number): string[] {
    if (!this.modal) return this.view.render(width);
    const w = Math.max(1, width);
    const innerWidth = Math.max(1, w - 4);
    const labels = STEERING_ACTIONS.map((action, index) => {
      const label = `[${action}]`;
      if (action === "interrupt" && !this.options.interruptEnabled()) return this.options.theme?.fg?.("dim", label) ?? label;
      return index === this.selected ? (this.options.theme?.fg?.("accent", label) ?? label) : label;
    }).join(" ");
    const state = this.busy ? "working…" : "Esc: cancel · Ctrl+C ignored";
    return wrapInBorder([
      "Leave owner steering",
      "",
      truncateToWidth(labels, innerWidth),
      "",
      truncateToWidth(state, innerWidth),
    ], w, innerWidth);
  }

  handleInput(data: string): void {
    if (!this.modal) {
      if (isEscapeKey(data)) {
        if (this.view.getMode() === "interactive") {
          this.modal = true;
          this.selected = 0;
          this.tui.requestRender();
        } else {
          this.options.done();
        }
        return;
      }
      this.view.handleInput(data);
      return;
    }
    if (this.busy || data === "\x03") return;
    if (isEscapeKey(data)) {
      this.modal = false;
      this.tui.requestRender();
      return;
    }
    if (this.options.matchesKey?.(data, "left") || data === "\x1b[D") {
      this.selected = (this.selected + STEERING_ACTIONS.length - 1) % STEERING_ACTIONS.length;
      this.tui.requestRender();
      return;
    }
    if (this.options.matchesKey?.(data, "right") || data === "\x1b[C") {
      this.selected = (this.selected + 1) % STEERING_ACTIONS.length;
      this.tui.requestRender();
      return;
    }
    if (data !== "\r" && data !== "\n") return;
    const action = STEERING_ACTIONS[this.selected]!;
    if (action === "hold") {
      this.options.done();
      return;
    }
    if (action === "interrupt" && !this.options.interruptEnabled()) return;
    this.busy = true;
    Promise.resolve(action === "finish" ? this.options.finish() : this.options.interrupt()).then(() => {
      this.busy = false;
      if (action === "finish") this.options.done();
      else this.modal = false;
      this.tui.requestRender();
    }).catch((error) => {
      this.busy = false;
      this.options.notify?.(`ws: ${action} failed: ${error instanceof Error ? error.message : String(error)}`, "error");
      this.tui.requestRender();
    });
  }

  /** Typed `/done` is the same operation as choosing finish. */
  finish(): void {
    if (this.busy) return;
    this.busy = true;
    Promise.resolve(this.options.finish()).then(() => this.options.done()).catch((error) => {
      this.busy = false;
      this.options.notify?.(`ws: finish failed: ${error instanceof Error ? error.message : String(error)}`, "error");
      this.tui.requestRender();
    });
  }
}

/**
 * Opens the record-backed viewer for `agentId`. View-mode Esc closes; Enter
 * raises the same component to interactive owner steering, where Esc opens
 * the shared action modal. Opening alone never resumes a dormant child.
 */
export async function openViewer(
  ctx: AuditUiCtx & { ui?: { custom?: unknown } },
  rpcRegistry: RpcAgentRegistry,
  agentId: string,
  sendCtx?: { pi: ExtensionAPI; cwd: string; extensionPath: string },
): Promise<void> {
  if (ctx.mode !== "tui") {
    // Defensive; unreachable given `shouldRegisterAudit`'s registration
    // gate, but matches `ask.ts`'s `openThread` per-call defensive style.
    notify(ctx, "ws: the audit window needs interactive mode.", "warning");
    return;
  }
  const record = rpcRegistry.get(agentId);
  if (!record) {
    notify(ctx, `ws: no agent "${agentId}" to audit.`, "warning");
    return;
  }
  if (record.ownership && !touchOwnership(record.ownership.home)) {
    notify(ctx, "ws: history unavailable — owned session home is busy, gone, or unreadable; retry /audit.", "warning");
    return;
  }

  const token = reserveOwnerOverlay();

  const channel = createAuditChannel(rpcRegistry, agentId, sendCtx ? async (text) => {
    await sendToAgent(rpcRegistry, { ...sendCtx, writer: "owner" }, agentId, text, record.streaming === true);
  } : undefined);
  const history = readSessionHistory(record.sessionPath, record.ownerSends);
  const initialItems = history.status === "available"
    ? history.items
    : [{ kind: "note" as const, text: "History unavailable: the child session file is gone or unreadable." }];
  const headerHint = `ws audit: ${rowName(record)} · Esc: actions · Enter: interact`;

  let markdownTheme: MarkdownTheme | undefined;
  try {
    markdownTheme = getMarkdownTheme();
  } catch {
    // best effort — see `ask.ts`'s `openThread` for the identical precedent.
  }

  try {
    await (ctx as unknown as AuditCustomUiCtx).ui.custom<undefined>(async (tui, theme, keybindings, done) => {
      const hostPiTui = await loadHostPiTui();
      let component!: OwnerSteeringComponent;
      const view = new ConversationViewComponent(tui, {
        channel,
        initialItems,
        headerHint,
        markdownTheme,
        userLineBg: (text) => theme?.bg?.("userMessageBg", text) ?? text,
        toolTextFg: (text) => theme?.fg?.("muted", text) ?? text,
        workingTextFg: (text) => theme?.fg?.("dim", text) ?? text,
        onSendError: (error) => notify(ctx, `ws: owner send failed: ${error instanceof Error ? error.message : String(error)}`, "error"),
        border: true,
        viewportHeight: () => conversationOverlayHeight(tui),
        keybindings: keybindings as { matches(data: string, id: string): boolean },
        primitives: { ScrollView: hostPiTui.ScrollView, Markdown: hostPiTui.Markdown, Text: hostPiTui.Text, Editor: hostPiTui.Editor },
        onDone: () => component.finish(),
        onEnter: () => view.setMode("interactive"),
      });
      component = new OwnerSteeringComponent(tui, view, {
        done: () => done(undefined),
        finish: async () => {
          if (sendCtx && isOwnerHeld(record)) await sendToAgent(rpcRegistry, sendCtx, agentId, OWNER_FINISH_MESSAGE, false);
        },
        interrupt: async () => {
          const live = rpcRegistry.get(agentId);
          if (live?.running && live.client) await live.client.abort();
        },
        interruptEnabled: () => rpcRegistry.get(agentId)?.running === true && rpcRegistry.get(agentId)?.client !== undefined,
        notify: (message, type) => notify(ctx, message, type),
        theme,
        matchesKey: hostPiTui.matchesKey as (data: string, keyId: string) => boolean,
      });
      activateOwnerOverlay({ token, close: () => done(undefined) });
      return component;
    }, AUDIT_OVERLAY_OPTIONS);
  } finally {
    clearOwnerOverlay(token);
  }
}

/**
 * Registers `/audit [id-or-alias]` and its picker shortcut — gated by
 * `shouldRegisterAudit` above. When the gate is false, NEITHER
 * `pi.registerCommand` NOR `pi.registerShortcut` is called (not merely
 * internally guarded), per the ticket's literal "nothing is registered"
 * wording.
 */
export function registerAuditCommands(
  pi: ExtensionAPI,
  rpcRegistry: RpcAgentRegistry,
  role: SpawnRole | undefined,
  mode: string | undefined,
  sessionCtx?: { cwd: string; extensionPath: string },
): void {
  if (!shouldRegisterAudit(role, mode)) return;

  pi.registerCommand("audit", {
    description: "Inspect or steer a subagent (usage: /audit [id-or-alias]; no id opens a picker).",
    handler: async (args, ctx) => {
      const idOrAlias = args.trim();
      if (idOrAlias) {
        const resolved = resolveAgentId(rpcRegistry, idOrAlias) ?? idOrAlias;
        await openViewer(ctx as never, rpcRegistry, resolved, sessionCtx ? { pi, ...sessionCtx } : undefined);
        return;
      }
      const selected = await openPicker(ctx as never, rpcRegistry);
      if (selected) await openViewer(ctx as never, rpcRegistry, selected, sessionCtx ? { pi, ...sessionCtx } : undefined);
    },
  });

  pi.registerShortcut("ctrl+shift+u" as never, {
    description: "Open the subagent audit picker.",
    handler: async (ctx) => {
      const selected = await openPicker(ctx as never, rpcRegistry);
      if (selected) await openViewer(ctx as never, rpcRegistry, selected, sessionCtx ? { pi, ...sessionCtx } : undefined);
    },
  });
}
