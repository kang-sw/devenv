/**
 * `260908-feat-ws-pi-subagent-audit-window-and-owner-steering` Phase 1: the
 * `/audit` command, its picker modal and keyboard shortcut, the session-file
 * parser to `ConversationItem`s, the `ConversationChannel` over a registry
 * record, and the `"view"`-mode overlay itself.
 *
 * Phase 1 is READ-ONLY, human-only surfaces: `ctx.ui.custom` (the picker,
 * the viewer overlay) plus `setWidget`/`setStatus`/tool-result `details`
 * conventions elsewhere in this package — this module never calls
 * `pi.sendMessage`, and `createAuditChannel`'s `ConversationChannel` omits
 * `send` entirely (that field is optional exactly so a `"view"`-only
 * consumer like this one is not forced to implement it — see
 * `conversation-view.ts`'s own header comment). Owner-steering
 * (`ConversationChannel.send`, the `[hold]/[finish]/[interrupt]` modal,
 * `lastWriter`/`ownerSends`) is Phase 2, untouched here.
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
 * Golden rule / placement: this module imports FROM `spawner.ts`,
 * `agent-widget.ts`, `ask.ts`, `conversation-view.ts`, `pi-tui.ts`, and
 * `process-role.ts` only, never the reverse.
 */

import { readFileSync } from "node:fs";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { getMarkdownTheme, getSelectListTheme } from "@earendil-works/pi-coding-agent";
import { lastActivityAt, resolveAgentId, type RpcAgentRegistry } from "./spawner.ts";
import { touchOwnership } from "./agent-storage.ts";
import { classifyRegistryRowState, formatCompactDuration, formatLatestInputTokens, rowName, type AgentRowState } from "./agent-widget.ts";
import { resolveChildLiveness } from "./ask.ts";
import {
  ConversationViewComponent,
  conversationOverlayHeight,
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
 * Phase 1's mapping (the ticket's own Phase 1 test list): `role:"user"` ->
 * always `"lead-message"` (no `ownerSends` log exists yet — Phase 2 tells
 * an owner turn apart from a lead one); `role:"assistant"` -> one
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
export function parseSessionFile(path: string): ConversationItem[] {
  let raw: string;
  try {
    raw = readFileSync(path, "utf8");
  } catch {
    return [];
  }

  const items: ConversationItem[] = [];
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
      items.push({ kind: "lead-message", text: joinTextContent(message.content) });
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
  return items;
}

// ---------------------------------------------------------------------------
// The audit `ConversationChannel` — `ask.ts`'s `createForkChannel` shape
// minus `send` (Phase 1 is view-only; see this file's header).
// ---------------------------------------------------------------------------

/**
 * Builds a `ConversationChannel` over `rpcRegistry`'s live record for
 * `agentId`. `sync()` attaches to `record.client.onEvent` only when a
 * client exists — a dormant record (`client === undefined`) is never
 * attached, which is what makes "opening a dormant child resumes nothing"
 * true for free, exactly as `ask.ts`'s `createForkChannel` already relies
 * on for the same reason. `send` is omitted entirely — the interface marks
 * it optional precisely for a `"view"`-only consumer like this one.
 */
export function createAuditChannel(rpcRegistry: RpcAgentRegistry, agentId: string): ConversationChannel {
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
      return resolveChildLiveness(rpcRegistry.get(agentId)?.streaming === true);
    },
  };
}

// ---------------------------------------------------------------------------
// The picker's row builder — the registry's three live tiers
// (`agent-widget.ts`'s own naming/ordering rules, reused verbatim) plus a
// fourth, picker-only dormant tier by last activity.
// ---------------------------------------------------------------------------

const LIVE_STATE_RANK: Record<AgentRowState, number> = {
  "awaiting-owner": 0,
  "awaiting-approval": 1,
  running: 2,
};

const LIVE_STATE_LABEL: Record<AgentRowState, string> = {
  "awaiting-owner": "awaiting owner",
  "awaiting-approval": "awaiting approval",
  running: "running",
};

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
  latestInput: string;
  activity: string;
  state?: AgentRowState;
  elapsedMs: number;
  lastActivity?: number;
}

/** Alias-first audit identity; otherwise the eight-character human-facing ID. */
function auditIdentity(record: { agentId: string; alias?: string }): string {
  return record.alias ?? record.agentId.slice(0, 8);
}

function elapsedSince(now: number, timestamp: number): number {
  return Number.isFinite(timestamp) ? Math.max(0, now - timestamp) : 0;
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
  const complete = [row.identity, row.status, row.model, row.latestInput, row.activity].join(separator);
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
    const latestInput = formatLatestInputTokens(record.telemetry?.latestInput ?? record.observedLatestInput);
    if (state === undefined) {
      const activityAt = lastActivityAt(record);
      dormant.push({
        agentId: record.agentId,
        identity,
        status: "dormant",
        model,
        latestInput,
        activity: `last active ${formatCompactDuration(elapsedSince(now, activityAt))} ago`,
        elapsedMs: elapsedSince(now, activityAt),
        lastActivity: activityAt,
      });
      continue;
    }
    const elapsedMs = elapsedSince(now, record.runStartedAt ?? now);
    live.push({
      agentId: record.agentId,
      identity,
      status: LIVE_STATE_LABEL[state],
      model,
      latestInput,
      activity: `running for ${formatCompactDuration(elapsedMs)}`,
      state,
      elapsedMs,
    });
  }

  live.sort((a, b) => {
    const rankDiff = LIVE_STATE_RANK[a.state!] - LIVE_STATE_RANK[b.state!];
    return rankDiff !== 0 ? rankDiff : b.elapsedMs - a.elapsedMs;
  });
  dormant.sort((a, b) => b.lastActivity! - a.lastActivity!);

  return [...live, ...dormant].map(({ agentId, identity, status, model, latestInput, activity }) => ({
    value: agentId,
    label: formatAuditPickerLabel({ identity, status, model, latestInput, activity }, labelWidth),
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

/**
 * Phase 1's "one overlay at a time": a second `/audit` closes the first —
 * own singleton to this module, independent of `ask.ts`'s own
 * `activeOverlay` (the two surfaces merge in Phase 2; see the plan's Out of
 * Scope). Closing never touches the child itself.
 */
let activeAuditOverlay: { token: number; close: () => void } | undefined;
let auditOverlayToken = 0;

/**
 * Opens the read-only viewer overlay for `agentId`: history from
 * `parseSessionFile(record.sessionPath)`, live tail from
 * `createAuditChannel`, `"view"` mode. Esc closes the overlay directly (no
 * modal — nothing here asks for confirmation or a summary); Enter raises the
 * component to `"interactive"` via its own `setMode` (the send path itself
 * is Phase 2 — see this file's header). Never resumes a dormant child on its
 * own: opening only reads the session file and (for a live record) the
 * existing RPC client.
 */
export async function openViewer(ctx: AuditUiCtx & { ui?: { custom?: unknown } }, rpcRegistry: RpcAgentRegistry, agentId: string): Promise<void> {
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
  if (record.ownership) touchOwnership(record.ownership.home);

  activeAuditOverlay?.close();
  activeAuditOverlay = undefined;
  const token = ++auditOverlayToken;

  const channel = createAuditChannel(rpcRegistry, agentId);
  const initialItems = parseSessionFile(record.sessionPath);
  const headerHint = `ws audit: ${rowName(record)} · Esc: close · Enter: interact (view-only in Phase 1 — nothing typed here is delivered yet)`;

  let markdownTheme: MarkdownTheme | undefined;
  try {
    markdownTheme = getMarkdownTheme();
  } catch {
    // best effort — see `ask.ts`'s `openThread` for the identical precedent.
  }

  try {
    await (ctx as unknown as AuditCustomUiCtx).ui.custom<undefined>(async (tui, theme, keybindings, done) => {
      const hostPiTui = await loadHostPiTui();
      const component: ConversationViewComponent = new ConversationViewComponent(tui, {
        channel,
        initialItems,
        headerHint,
        markdownTheme,
        userLineBg: (text) => theme?.bg?.("userMessageBg", text) ?? text,
        toolTextFg: (text) => theme?.fg?.("muted", text) ?? text,
        workingTextFg: (text) => theme?.fg?.("dim", text) ?? text,
        border: true,
        viewportHeight: () => conversationOverlayHeight(tui),
        keybindings: keybindings as { matches(data: string, id: string): boolean },
        primitives: { ScrollView: hostPiTui.ScrollView, Markdown: hostPiTui.Markdown, Text: hostPiTui.Text, Editor: hostPiTui.Editor },
        // No modal — Phase 1's "Esc closes the viewer directly" (contrast
        // `ask.ts`'s `openThread`, which routes Esc through `overlayHandle`
        // to participate in a pending-summary race this window has none of).
        onEscape: () => done(undefined),
        // Raise-only `setMode` — never touches `record`/the registry.
        onEnter: () => component.setMode("interactive"),
      });
      activeAuditOverlay = { token, close: () => done(undefined) };
      return component;
    }, AUDIT_OVERLAY_OPTIONS);
  } finally {
    if (activeAuditOverlay?.token === token) activeAuditOverlay = undefined;
  }
}

/**
 * Registers `/audit [id-or-alias]` and its picker shortcut — gated by
 * `shouldRegisterAudit` above. When the gate is false, NEITHER
 * `pi.registerCommand` NOR `pi.registerShortcut` is called (not merely
 * internally guarded), per the ticket's literal "nothing is registered"
 * wording.
 */
export function registerAuditCommands(pi: ExtensionAPI, rpcRegistry: RpcAgentRegistry, role: SpawnRole | undefined, mode: string | undefined): void {
  if (!shouldRegisterAudit(role, mode)) return;

  pi.registerCommand("audit", {
    description: "Open a subagent's read-only transcript (usage: /audit [id-or-alias]; no id opens a picker).",
    handler: async (args, ctx) => {
      const idOrAlias = args.trim();
      if (idOrAlias) {
        const resolved = resolveAgentId(rpcRegistry, idOrAlias) ?? idOrAlias;
        await openViewer(ctx as never, rpcRegistry, resolved);
        return;
      }
      const selected = await openPicker(ctx as never, rpcRegistry);
      if (selected) await openViewer(ctx as never, rpcRegistry, selected);
    },
  });

  pi.registerShortcut("ctrl+shift+u" as never, {
    description: "Open the subagent audit picker.",
    handler: async (ctx) => {
      const selected = await openPicker(ctx as never, rpcRegistry);
      if (selected) await openViewer(ctx as never, rpcRegistry, selected);
    },
  });
}
