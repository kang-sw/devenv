// 260722-feat-dashboard-hotkey-config-framework Phase 1: binding registry +
// leader-press dispatch framework. Pure, DOM-free module (no React/browser
// types) mirroring the `keydownSuppression.ts` split - callers read real
// DOM/event state into the minimal shapes below and drive the state machine
// from a thin `App.tsx` effect. Kept independent of DOM so the registry,
// leader-mode state machine, reserved-key rejection, and persistence
// round-trip are all unit-testable without jsdom, the same way
// `shouldSuppressBrowserShortcut` is.
//
// Scope note (Phase 1): this module ships the framework - the binding
// registry, the leader-press state machine, the reserved-key guard, the
// terminal-focus/IME capture guard, and browser-local persistence for user
// rebindings. It does NOT wire the full default keymap (groups
// `g`/`r`/`a`/`t`/`d`/`p`/`v` etc. from the finalized keymap spec) - only the
// SUBSET of leaves from that spec whose target already exists as a real
// `DashboardCommandId` today (see `commands.ts`), enough to exercise
// no-payload dispatch, payload-needing "opens a picker" dispatch, and the
// reserved-key rejection path end-to-end. Wiring every concrete binding
// (including every GAP id from the spec's R2 list) is explicitly out of
// scope for this phase. Every default binding's key SEQUENCE, not just its
// command id, matches the finalized spec's group/leaf placement exactly -
// see `buildDefaultHotkeyBindings` below - so no default claims a group
// prefix (`r`/`t`/`a`/`g w`) as a leaf ahead of that group's later leaves.

import {
  buildAgentChatCreateCommand,
  buildGitBranchCreateOpenCommand,
  buildGitBranchMenuOpenCommand,
  buildGitFetchCommand,
  buildGitPullFfOnlyCommand,
  buildGitPushCommand,
  buildGitRefreshCommand,
  buildGitWorktreeAddOpenCommand,
  buildRootPickerOpenCommand,
  buildTerminalCreateCommand,
  buildWorkRootActivationCommand,
  buildWorkRootCloseCommand,
  buildWorkspaceMenuOpenCommand,
  buildWorkspaceRemoveCommand,
  buildWorktreeHiddenMenuOpenCommand,
  type DashboardCommand,
  type DashboardCommandId,
  type DashboardCommandPayload,
} from "./commands.js";
import { browserStorage } from "./workRootFiles.js";

// --- Reserved keys ---------------------------------------------------------
//
// Keys default (and user) bindings must never claim, because they are
// reserved for browser/terminal-focus/in-app behavior elsewhere: `Ctrl+`` `
// (terminal focus), `Ctrl+R` (in-app reverse-history-search, see
// `keydownSuppression.ts`), `Ctrl+G`, `Ctrl+Enter`. Expressed the same
// normalized shape as `SUPPRESSED_CTRL_KEYS` in `keydownSuppression.ts`: a
// plain `Set<string>` of lowercased key values, checked only when the chord
// carries a ctrl/meta modifier.

export const RESERVED_CHORD_KEYS = new Set(["`", "r", "g", "enter"]);

export type HotkeyChord = {
  readonly key: string;
  readonly ctrl?: boolean;
  readonly meta?: boolean;
  readonly shift?: boolean;
  readonly alt?: boolean;
};

export function normalizeChordKey(key: string): string {
  const lowered = key.toLowerCase();
  if (lowered === "spacebar") return " ";
  return lowered;
}

export function isReservedChord(chord: HotkeyChord): boolean {
  if (!chord.ctrl && !chord.meta) {
    return false;
  }
  return RESERVED_CHORD_KEYS.has(normalizeChordKey(chord.key));
}

export function leaderKeys(...chars: readonly string[]): readonly HotkeyChord[] {
  return chars.map((key) => ({ key }));
}

// --- Binding registry --------------------------------------------------

export type HotkeyBindingKind = "leaderSub" | "standalone";

export type HotkeyBinding<TContext = unknown> = {
  readonly id: string;
  readonly kind: HotkeyBindingKind;
  readonly keys: readonly HotkeyChord[];
  readonly commandId: DashboardCommandId;
  readonly buildPayload: (ctx: TContext) => DashboardCommandPayload | null;
  readonly description?: string;
};

export type HotkeyRegistrationResult =
  | { readonly ok: true }
  | { readonly ok: false; readonly reason: string };

export function checkHotkeyBindingKeys(
  keys: readonly HotkeyChord[],
): HotkeyRegistrationResult {
  const reserved = keys.find(isReservedChord);
  if (reserved) {
    return {
      ok: false,
      reason: `reserved chord claimed: ${describeChord(reserved)}`,
    };
  }
  return { ok: true };
}

export function describeChord(chord: HotkeyChord): string {
  const parts: string[] = [];
  if (chord.ctrl) parts.push("Ctrl");
  if (chord.meta) parts.push("Cmd");
  if (chord.alt) parts.push("Alt");
  if (chord.shift) parts.push("Shift");
  parts.push(chord.key === " " ? "Space" : chord.key);
  return parts.join("+");
}

// A binding-registration API general enough that later layers (which-key
// overlay, command bar, hint-click - all separate tickets) can register
// against it without a rewrite: `registerDefault` is fail-fast (throws) so a
// reserved-key default is a build-time/test-time bug; `registerUser` returns
// a `HotkeyRegistrationResult` so a reserved-key user rebind is a
// runtime-rejectable user error instead of a crash.
export class HotkeyRegistry<TContext = unknown> {
  private readonly bindings = new Map<string, HotkeyBinding<TContext>>();

  registerDefault(binding: HotkeyBinding<TContext>): void {
    const check = checkHotkeyBindingKeys(binding.keys);
    if (!check.ok) {
      throw new Error(
        `cannot register default hotkey binding "${binding.id}": ${check.reason}`,
      );
    }
    this.bindings.set(binding.id, binding);
  }

  registerUser(binding: HotkeyBinding<TContext>): HotkeyRegistrationResult {
    const check = checkHotkeyBindingKeys(binding.keys);
    if (!check.ok) {
      return check;
    }
    this.bindings.set(binding.id, binding);
    return { ok: true };
  }

  unregister(id: string): void {
    this.bindings.delete(id);
  }

  get(id: string): HotkeyBinding<TContext> | undefined {
    return this.bindings.get(id);
  }

  list(): readonly HotkeyBinding<TContext>[] {
    return [...this.bindings.values()];
  }
}

export function resolveHotkeyCommand<TContext>(
  binding: HotkeyBinding<TContext>,
  ctx: TContext,
): DashboardCommand | null {
  const payload = binding.buildPayload(ctx);
  if (!payload) {
    return null;
  }
  return { commandId: binding.commandId, payload };
}

// --- Leader-mode state machine -----------------------------------------
//
// idle -> pending on leader keydown; pending + matching leaf key -> resolved
// command + back to idle; pending + a key that only narrows a group -> stays
// pending with a narrowed subtree; pending + unmatched key or Escape -> idle
// (cancel). Optional configurable timeout, default DISABLED - per the
// finalized "## Default Keymap & Interaction Spec (finalized 2026-07-22)"
// section, which supersedes the ticket's own older Phase 1 bullet wording
// ("times out/cancels on an unmatched key or explicit cancel"). Unmatched-key
// and Escape remain the default cancel paths regardless of timeout config.
//
// Leaf-vs-prefix precedence (proper prefix TRIE, not a flat lookup): the
// finalized keymap spec nests leaves and sub-groups under the same parent
// (e.g. under `g`: `r`/`f`/`p`/`l`/`b`/`c` are leaves while `w` is itself a
// further 3-tier sub-menu). A `LeaderTreeNode` therefore MAY carry both a
// `binding` (it is a registered leaf) AND non-empty `children` (something
// else registered a longer sequence through the same node) at the same
// time. When that happens, **children always win**: a node with any
// children is treated purely as a group and DESCENDS (narrows) on the next
// keystroke, never shadow-firing its own `binding` - only a node with NO
// children (an actual terminal leaf) resolves. This keeps a shorter
// registration from silently stranding a longer one as unreachable dead
// code; see `stepLeaderState` below for the enforcement point.

export type LeaderTreeNode<TContext = unknown> = {
  readonly binding?: HotkeyBinding<TContext>;
  readonly children: ReadonlyMap<string, LeaderTreeNode<TContext>>;
};

type MutableLeaderTreeNode<TContext> = {
  binding?: HotkeyBinding<TContext>;
  readonly children: Map<string, MutableLeaderTreeNode<TContext>>;
};

export function buildLeaderTree<TContext>(
  bindings: readonly HotkeyBinding<TContext>[],
): LeaderTreeNode<TContext> {
  const root: MutableLeaderTreeNode<TContext> = { children: new Map() };
  for (const binding of bindings) {
    if (binding.kind !== "leaderSub" || binding.keys.length === 0) {
      continue;
    }
    let node = root;
    for (const [index, chord] of binding.keys.entries()) {
      const key = normalizeChordKey(chord.key);
      let next = node.children.get(key);
      if (!next) {
        next = { children: new Map() };
        node.children.set(key, next);
      }
      if (index === binding.keys.length - 1) {
        next.binding = binding;
      }
      node = next;
    }
  }
  return root;
}

export type LeaderState<TContext = unknown> =
  | { readonly kind: "idle" }
  | {
      readonly kind: "pending";
      readonly node: LeaderTreeNode<TContext>;
      readonly typed: readonly string[];
      readonly enteredAtMs: number;
    };

export const IDLE_LEADER_STATE: LeaderState<never> = { kind: "idle" };

export function enterLeaderPending<TContext>(
  root: LeaderTreeNode<TContext>,
  nowMs: number,
): LeaderState<TContext> {
  return { kind: "pending", node: root, typed: [], enteredAtMs: nowMs };
}

export type LeaderStepAction = "cancel" | "resolve" | "narrow" | "ignore";

export type LeaderStepResult<TContext = unknown> = {
  readonly state: LeaderState<TContext>;
  readonly action: LeaderStepAction;
  readonly binding?: HotkeyBinding<TContext>;
};

// Bare modifier keydowns (`Shift`/`Control`/`Alt`/`Meta` reported as their
// own `key` value, e.g. holding Shift before typing a capitalized leaf like
// the finalized spec's `<leader> ?`) are not sequence input on their own -
// treating them as an ordinary unmatched key would cancel a pending
// sequence before the actual shifted key arrives. Ignore them in place
// rather than consuming a step.
const PURE_MODIFIER_KEYS = new Set(["shift", "control", "alt", "meta"]);

export function stepLeaderState<TContext>(
  state: LeaderState<TContext>,
  key: string,
  nowMs: number,
): LeaderStepResult<TContext> {
  if (state.kind === "idle") {
    return { state, action: "ignore" };
  }
  if (key === "Escape") {
    return { state: { kind: "idle" }, action: "cancel" };
  }
  const normalized = normalizeChordKey(key);
  if (PURE_MODIFIER_KEYS.has(normalized)) {
    return { state, action: "ignore" };
  }
  const next = state.node.children.get(normalized);
  if (!next) {
    return { state: { kind: "idle" }, action: "cancel" };
  }
  // Leaf-vs-prefix precedence (see the module-level comment above
  // `LeaderTreeNode`): a node with children is always a group and must
  // descend, never shadow-fire a `binding` it might also carry. Only a
  // childless node - a true terminal leaf - resolves.
  if (next.children.size > 0) {
    return {
      state: {
        kind: "pending",
        node: next,
        typed: [...state.typed, normalized],
        enteredAtMs: nowMs,
      },
      action: "narrow",
    };
  }
  if (next.binding) {
    return { state: { kind: "idle" }, action: "resolve", binding: next.binding };
  }
  // Defensive fallback: `buildLeaderTree` never produces a node with
  // neither a binding nor children, but a hand-built `LeaderTreeNode` (the
  // type is exported) could - treat it the same as an unmatched key.
  return { state: { kind: "idle" }, action: "cancel" };
}

// --- Which-key overlay support (260722-feat-dashboard-which-key-hint-
// overlay Phase 1) -----------------------------------------------------
//
// Pure, DOM-free presentation helper: given the currently-pending
// `LeaderTreeNode`, describe its immediate children as overlay-ready rows.
// Deliberately does NOT introduce its own group/leaf rule - it replicates
// the exact same children-win precedence `stepLeaderState` already
// enforces (see the module-level comment above `LeaderTreeNode`): a child
// with any children of its own is always a group and never shadow-fires a
// `binding` it might also carry.
//
// Leaf label source: the finalized which-key spec says leaf labels come
// from `dashboardCommandLabel`, but that requires resolving a
// `DashboardCommand` via `binding.buildPayload(ctx)`, which returns `null`
// (and therefore no label) for every context-dependent default binding
// whenever no work root is selected (see `activeRootBinding`). Every
// default leaf's `description` was already authored as the same
// human-readable action label `dashboardCommandLabel` would have produced,
// so `binding?.description` is used directly here instead - satisfying the
// spec's intent without the null-payload gap. This module has zero other
// `description` consumers today; this helper is its first reader.
export type LeaderChildEntry = {
  readonly key: string;
  readonly kind: "group" | "leaf";
  readonly label?: string;
};

export function describeLeaderChildren<TContext>(
  node: LeaderTreeNode<TContext>,
): readonly LeaderChildEntry[] {
  const entries: LeaderChildEntry[] = [];
  for (const [key, child] of node.children) {
    if (child.children.size > 0) {
      entries.push({ key, kind: "group" });
    } else {
      entries.push({ key, kind: "leaf", label: child.binding?.description });
    }
  }
  return entries;
}

export type LeaderTimeoutConfig = {
  readonly enabled: boolean;
  readonly ms: number;
};

// Default OFF per the finalized keymap/interaction spec's which-key section
// ("No auto-timeout by default (configurable)"), which is authoritative over
// the ticket's own older Phase 1 bullet wording.
export const DEFAULT_LEADER_TIMEOUT: LeaderTimeoutConfig = {
  enabled: false,
  ms: 1500,
};

export function leaderPendingExpired<TContext>(
  state: LeaderState<TContext>,
  nowMs: number,
  config: LeaderTimeoutConfig,
): boolean {
  if (state.kind !== "pending" || !config.enabled) {
    return false;
  }
  return nowMs - state.enteredAtMs >= config.ms;
}

export type HotkeyKeydownEvent = {
  readonly key: string;
  readonly ctrlKey: boolean;
  readonly metaKey: boolean;
  readonly shiftKey: boolean;
  readonly altKey: boolean;
};

export function chordFromKeydownEvent(evt: HotkeyKeydownEvent): HotkeyChord {
  return {
    key: normalizeChordKey(evt.key),
    ctrl: evt.ctrlKey,
    meta: evt.metaKey,
    shift: evt.shiftKey,
    alt: evt.altKey,
  };
}

// Leader-only, no modal (Decisions): `Ctrl+Space` press enters the transient
// dashboard command mode. Meta+Space is deliberately excluded (Cmd+Space is
// commonly OS-reserved, e.g. Spotlight on macOS).
export function isLeaderTriggerKeydown(evt: HotkeyKeydownEvent): boolean {
  if (!evt.ctrlKey || evt.metaKey || evt.altKey) {
    return false;
  }
  const normalized = normalizeChordKey(evt.key);
  return normalized === " " || normalized === "space";
}

export function findStandaloneMatch<TContext>(
  bindings: readonly HotkeyBinding<TContext>[],
  chord: HotkeyChord,
): HotkeyBinding<TContext> | null {
  const normalizedKey = normalizeChordKey(chord.key);
  for (const binding of bindings) {
    if (binding.kind !== "standalone" || binding.keys.length !== 1) {
      continue;
    }
    const target = binding.keys[0];
    if (
      normalizeChordKey(target.key) === normalizedKey &&
      Boolean(target.ctrl) === chord.ctrl &&
      Boolean(target.meta) === chord.meta &&
      Boolean(target.shift) === chord.shift &&
      Boolean(target.alt) === chord.alt
    ) {
      return binding;
    }
  }
  return null;
}

// --- Terminal-focus / IME capture guard ---------------------------------
//
// Generalizes the verbatim guard clauses in the terminal pane's own
// `keydownFallback` (`App.tsx`): skip capture on IME composition, on
// editable targets (input/textarea/select/contentEditable), and when focus
// is already inside a terminal pane. The per-pane `containerRef.contains()`
// check there becomes a single global `document.activeElement?.closest(
// ".terminal-pane")` equivalent here, since this listener is one
// document-level listener for the whole app, not scoped to one pane.

export type HotkeyGuardEvent = {
  readonly isComposing: boolean;
  readonly key: string;
  readonly targetIsEditable: boolean;
  readonly targetInsideTerminalPane: boolean;
};

export function shouldSkipHotkeyCapture(evt: HotkeyGuardEvent): boolean {
  if (evt.isComposing || evt.key === "Process") {
    return true;
  }
  if (evt.targetIsEditable) {
    return true;
  }
  if (evt.targetInsideTerminalPane) {
    return true;
  }
  return false;
}

// --- Keydown guard-stage ordering (single source of truth) --------------
//
// `App.tsx#handleKeydown` calls this after its own leader-*pending*
// continuation branch has already run (and returned, if applicable) - this
// function only decides the *next* stage, where the leader-*entry* trigger
// (`Ctrl+Space` from idle) and the terminal/editable-target passthrough
// guard (`shouldSkipHotkeyCapture`) compete for the same keydown. The
// leader-entry trigger is checked FIRST (when not IME-composing), so
// entering leader mode is never blocked by terminal or editable focus;
// every other key still passes through `shouldSkipHotkeyCapture` exactly as
// before. Extracted as one pure function (rather than left as two
// independently-ordered call sites in `handleKeydown`) so this ordering
// contract is itself directly unit-testable and fails loudly if the checks
// are ever reordered back.
export type HotkeyKeydownGuardStageEvent = HotkeyKeydownEvent & {
  readonly isComposing: boolean;
  readonly targetIsEditable: boolean;
  readonly targetInsideTerminalPane: boolean;
};

export type HotkeyKeydownGuardStageDecision =
  | "enter-leader"
  | "skip-passthrough"
  | "fall-through";

export function decideKeydownGuardStage(
  evt: HotkeyKeydownGuardStageEvent,
): HotkeyKeydownGuardStageDecision {
  const isComposing = evt.isComposing || evt.key === "Process";
  if (!isComposing && isLeaderTriggerKeydown(evt)) {
    return "enter-leader";
  }
  if (
    shouldSkipHotkeyCapture({
      isComposing,
      key: evt.key,
      targetIsEditable: evt.targetIsEditable,
      targetInsideTerminalPane: evt.targetInsideTerminalPane,
    })
  ) {
    return "skip-passthrough";
  }
  return "fall-through";
}

// --- Persistence ----------------------------------------------------------
//
// Follows the `workRootFiles.ts` `browserStorage()`/versioned-JSON/defensive-
// parse precedent verbatim: injectable `storage` parameter defaulting to
// `browserStorage()` (reused, not reimplemented), version-tagged blob keyed
// `"ws-dashboard.hotkeys.v1"` (matching the `"ws-dashboard.<feature>.v<N>"`
// convention in `workNavOrder.ts`/`terminals.ts`), silently dropping
// malformed/version-mismatched data rather than throwing. Only user
// rebindings/added standalone hotkeys are persisted (not the full default
// table), so future default-table changes need no migration for users who
// never rebound anything: a rebind record only carries a stable binding
// `id` plus its new trigger `keys`/`kind`, and applying it looks up and
// overrides the matching entry already present in the base binding table.

export type HotkeyUserRebind = {
  readonly id: string;
  readonly kind: HotkeyBindingKind;
  readonly keys: readonly HotkeyChord[];
};

export type HotkeyUserConfig = {
  readonly rebinds: readonly HotkeyUserRebind[];
};

export const EMPTY_HOTKEY_USER_CONFIG: HotkeyUserConfig = { rebinds: [] };

const hotkeyUserConfigStorageKey = "ws-dashboard.hotkeys.v1";

function parseHotkeyChord(value: unknown): HotkeyChord | null {
  if (typeof value !== "object" || value === null) {
    return null;
  }
  const record = value as Record<string, unknown>;
  if (typeof record.key !== "string") {
    return null;
  }
  return {
    key: record.key,
    ctrl: record.ctrl === true,
    meta: record.meta === true,
    shift: record.shift === true,
    alt: record.alt === true,
  };
}

function isHotkeyBindingKind(value: unknown): value is HotkeyBindingKind {
  return value === "leaderSub" || value === "standalone";
}

function parseHotkeyUserRebind(value: unknown): HotkeyUserRebind | null {
  if (typeof value !== "object" || value === null) {
    return null;
  }
  const record = value as Record<string, unknown>;
  if (typeof record.id !== "string") {
    return null;
  }
  if (!isHotkeyBindingKind(record.kind)) {
    return null;
  }
  if (!Array.isArray(record.keys)) {
    return null;
  }
  const keys = record.keys.map(parseHotkeyChord);
  if (keys.some((chord) => chord === null)) {
    return null;
  }
  return { id: record.id, kind: record.kind, keys: keys as HotkeyChord[] };
}

export function loadHotkeyUserConfig(
  storage: Pick<Storage, "getItem"> | null = browserStorage(),
): HotkeyUserConfig {
  if (!storage) {
    return EMPTY_HOTKEY_USER_CONFIG;
  }
  try {
    const raw = storage.getItem(hotkeyUserConfigStorageKey);
    if (!raw) {
      return EMPTY_HOTKEY_USER_CONFIG;
    }
    const parsed = JSON.parse(raw) as { version?: unknown; rebinds?: unknown };
    if (parsed.version !== 1 || !Array.isArray(parsed.rebinds)) {
      return EMPTY_HOTKEY_USER_CONFIG;
    }
    const rebinds = parsed.rebinds.flatMap((value): HotkeyUserRebind[] => {
      const rebind = parseHotkeyUserRebind(value);
      return rebind ? [rebind] : [];
    });
    return { rebinds };
  } catch {
    return EMPTY_HOTKEY_USER_CONFIG;
  }
}

export function saveHotkeyUserConfig(
  config: HotkeyUserConfig,
  storage: Pick<Storage, "setItem" | "removeItem"> | null = browserStorage(),
): void {
  if (!storage) {
    return;
  }
  try {
    if (config.rebinds.length === 0) {
      storage.removeItem(hotkeyUserConfigStorageKey);
      return;
    }
    storage.setItem(
      hotkeyUserConfigStorageKey,
      JSON.stringify({ version: 1, rebinds: config.rebinds }),
    );
  } catch {
    // Browser persistence is best-effort; the default/live binding table
    // remains canonical.
  }
}

export type ApplyHotkeyUserConfigResult<TContext> = {
  readonly bindings: readonly HotkeyBinding<TContext>[];
  readonly rejected: readonly { readonly id: string; readonly reason: string }[];
};

export function applyHotkeyUserConfig<TContext>(
  defaults: readonly HotkeyBinding<TContext>[],
  config: HotkeyUserConfig,
): ApplyHotkeyUserConfigResult<TContext> {
  const byId = new Map(defaults.map((binding) => [binding.id, binding]));
  const rejected: { id: string; reason: string }[] = [];
  for (const rebind of config.rebinds) {
    const base = byId.get(rebind.id);
    if (!base) {
      rejected.push({ id: rebind.id, reason: "unknown binding id" });
      continue;
    }
    const check = checkHotkeyBindingKeys(rebind.keys);
    if (!check.ok) {
      rejected.push({ id: rebind.id, reason: check.reason });
      continue;
    }
    byId.set(rebind.id, { ...base, kind: rebind.kind, keys: rebind.keys });
  }
  return { bindings: [...byId.values()], rejected };
}

// --- Default bindings (Phase 1 subset) ----------------------------------
//
// Every entry below is a real leaf from the ticket's owner-approved
// "## Default Keymap & Interaction Spec (finalized 2026-07-22)" section,
// placed at EXACTLY the key sequence that spec assigns it, restricted to
// the leaves whose `DashboardCommandId` already exists in `commands.ts`
// today. Nothing here invents a key placement: a default is either wired at
// its spec position or omitted entirely (never approximated).
//
// Group-prefix discipline (this is what keeps the leader tree a real trie,
// not a flat table masquerading as one): `r`, `t`, `a`, and `g w` are all
// GROUP prefixes in the finalized spec, each holding further leaves of
// their own (`r o`/`r x`/`r t`; `t n`; `a n`; `g w a`/`g w x`/`g w m`).
// None of them is ever bound as a bare leaf here, so a group prefix node
// never carries a `binding` of its own to begin with - `stepLeaderState`'s
// children-win precedence (see the leader-mode state machine section above)
// is defense in depth on top of this, not a substitute for it. `<leader> w`
// itself is bound to NOTHING (spec: "intentionally UNUSED: all worktree ops
// live under `g w`, all root/workRoot ops under `r`").
//
// Excluded, and why (not an oversight):
// - Every R2-listed GAP prerequisite id (workRoot flat/hierarchical
//   select-by-index, `pane.focus.<kind>`, tab next/prev/cycle, terminal
//   scroll/clear/copy-selection, editor next/prev-file/close, left-nav row
//   select, focus-git-status-inspector) - spec leaves `g s`, `t x`, the
//   entire `p`/`d`/`v` groups, and `r`'s digit-prefixed hierarchical select
//   all depend on one of these and are omitted rather than fabricated.
// - The agentChat-bubble/prompt/history group (`a s`/`a y`/`a f`/`a
//   r`/`a c`/`a k`) - those command-id strings exist only as
//   `data-command-id` DOM markers, not real `DashboardCommandId` union
//   members (see plan Codebase Findings). Only `a n` (agentChat.create, a
//   real union member) is wired.
// - The `d` (Document) group - `document.save`/`document.revert`/etc. ARE
//   real union members, but there is no existing "currently focused
//   document pane" concept to resolve a `path` from without inventing new
//   focus-tracking, which would violate R1's "never fabricates a
//   selection".
// - Top-level `<leader> <space>` (command palette), `<leader> ?`
//   (which-key), and `<leader> f` (hint-click) - these are framework-UI
//   entry points for the two later layers
//   (`260722-feat-dashboard-which-key-hint-overlay`,
//   `260722-feat-dashboard-hint-click-fast-jump`) and the command bar
//   (`260711-idea-dashboard-command-bus-quick-open-shortcuts`), not
//   `DashboardCommandId` dispatches this registry resolves to; wiring them
//   without their owning UI would be scope creep into those tickets.

export type HotkeyActiveRootContext = {
  readonly workRootId: string;
  readonly serverRoute: string;
  readonly workspaceId: string;
  readonly activation: "online" | "offline";
};

export type HotkeyDispatchContext = {
  readonly activeRoot: HotkeyActiveRootContext | null;
};

function activeRootBinding(
  id: string,
  keys: readonly string[],
  commandId: DashboardCommandId,
  buildPayload: (root: HotkeyActiveRootContext) => DashboardCommandPayload,
  description: string,
): HotkeyBinding<HotkeyDispatchContext> {
  return {
    id,
    kind: "leaderSub",
    keys: leaderKeys(...keys),
    commandId,
    buildPayload: (ctx) => (ctx.activeRoot ? buildPayload(ctx.activeRoot) : null),
    description,
  };
}

export function buildDefaultHotkeyBindings(): readonly HotkeyBinding<HotkeyDispatchContext>[] {
  return [
    // `r` Root/WorkRoot group (spec: "r o" / "r x" / "r t"; the bare `r`
    // prefix and its digit-driven hierarchical-select leaves are GAPs, R2).
    {
      id: "rootPicker.open",
      kind: "leaderSub",
      keys: leaderKeys("r", "o"),
      commandId: "rootPicker.open",
      buildPayload: () => buildRootPickerOpenCommand().payload,
      description: "Open root picker (browse filesystem to add a new root)",
    },
    activeRootBinding(
      "workRoot.close",
      ["r", "x"],
      "workRoot.close",
      (root) => buildWorkRootCloseCommand(root.workRootId, root.serverRoute).payload,
      "Close active work root",
    ),
    activeRootBinding(
      "workRoot.activation.set",
      ["r", "t"],
      "workRoot.activation.set",
      (root) =>
        buildWorkRootActivationCommand(
          root.workRootId,
          root.activation === "online" ? "offline" : "online",
          root.serverRoute,
        ).payload,
      "Toggle active work root online/offline",
    ),
    // `t` Terminal group (spec: "t n"; "t x" terminal.close is a GAP - no
    // `DashboardCommandId` member exists for it today).
    activeRootBinding(
      "terminal.create",
      ["t", "n"],
      "terminal.create",
      (root) => buildTerminalCreateCommand(root.workRootId, root.serverRoute).payload,
      "Open new terminal in active work root",
    ),
    // `a` Agent-chat group (spec: "a n"; every other `a` leaf targets a
    // DOM-marker-only id, not a real `DashboardCommandId` member).
    activeRootBinding(
      "agentChat.create",
      ["a", "n"],
      "agentChat.create",
      (root) => buildAgentChatCreateCommand(root.workRootId, root.serverRoute).payload,
      "Open new agent tab in active work root",
    ),
    // `g` Git group (spec: "g r"/"g f"/"g p"/"g l"/"g b"/"g c"; "g s" is a
    // GAP - no focus-git-status-inspector id exists yet).
    activeRootBinding(
      "git.refresh",
      ["g", "r"],
      "git.refresh",
      (root) => buildGitRefreshCommand(root.workRootId, root.serverRoute).payload,
      "Refresh Git status for active work root",
    ),
    activeRootBinding(
      "git.fetch",
      ["g", "f"],
      "git.fetch",
      (root) => buildGitFetchCommand(root.workRootId, root.serverRoute).payload,
      "Fetch Git for active work root",
    ),
    activeRootBinding(
      "git.push",
      ["g", "p"],
      "git.push",
      (root) => buildGitPushCommand(root.workRootId, root.serverRoute).payload,
      "Push Git for active work root",
    ),
    activeRootBinding(
      "git.pullFfOnly",
      ["g", "l"],
      "git.pullFfOnly",
      (root) => buildGitPullFfOnlyCommand(root.workRootId, root.serverRoute).payload,
      "Pull Git (ff-only) for active work root",
    ),
    activeRootBinding(
      "git.branchMenu.open",
      ["g", "b"],
      "git.branchMenu.open",
      (root) => buildGitBranchMenuOpenCommand(root.workRootId, root.serverRoute).payload,
      "Open branch menu for active work root",
    ),
    activeRootBinding(
      "git.branchCreate.open",
      ["g", "c"],
      "git.branchCreate.open",
      (root) =>
        buildGitBranchCreateOpenCommand(root.workRootId, root.serverRoute).payload,
      "Open new-branch form for active work root",
    ),
    // `g w` worktree sub-menu (4-tier: spec "g w a"/"g w x"/"g w m" plus
    // "g w h" hidden-worktrees; `g w` itself is a pure group prefix).
    activeRootBinding(
      "gitWorktreeAdd.open",
      ["g", "w", "a"],
      "gitWorktreeAdd.open",
      (root) =>
        buildGitWorktreeAddOpenCommand(root.workspaceId, root.serverRoute).payload,
      "Open add-worktree form for active workspace",
    ),
    activeRootBinding(
      "workspace.remove",
      ["g", "w", "x"],
      "workspace.remove",
      (root) => buildWorkspaceRemoveCommand(root.workspaceId, root.serverRoute).payload,
      "Remove active workspace (always-confirm modal)",
    ),
    activeRootBinding(
      "workspace.menu.open",
      ["g", "w", "m"],
      "workspace.menu.open",
      (root) => buildWorkspaceMenuOpenCommand(root.workspaceId, root.serverRoute).payload,
      "Open workspace menu for active workspace",
    ),
    activeRootBinding(
      "worktreeHidden.menu.open",
      ["g", "w", "h"],
      "worktreeHidden.menu.open",
      (root) =>
        buildWorktreeHiddenMenuOpenCommand(root.workspaceId, root.serverRoute).payload,
      "Open hidden worktrees for active workspace",
    ),
  ];
}
