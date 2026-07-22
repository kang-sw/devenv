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
// `g`/`r`/`a`/`t`/`d`/`p`/`v` etc. from the finalized keymap spec) - only a
// representative subset whose target already exists as a real
// `DashboardCommandId` today (see `commands.ts`), enough to exercise
// no-payload dispatch, payload-needing "opens a picker" dispatch, and the
// reserved-key rejection path end-to-end. Wiring every concrete binding is
// explicitly out of scope for this phase.

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
  const next = state.node.children.get(normalized);
  if (!next) {
    return { state: { kind: "idle" }, action: "cancel" };
  }
  if (next.binding) {
    return { state: { kind: "idle" }, action: "resolve", binding: next.binding };
  }
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
// Only the subset of default bindings whose target is a real, already-
// existing `DashboardCommandId` today (see `commands.ts`), enough breadth to
// exercise no-payload dispatch, payload-needing "opens a picker" dispatch
// (R1: never fabricates a selection - either dispatches directly or opens
// the relevant picker/menu), and the reserved-key rejection path. Explicitly
// excludes: every R2-listed GAP prerequisite id, the agentChat-bubble/
// prompt/history group (those command-id strings exist only as
// `data-command-id` DOM markers, not real `DashboardCommandId` union
// members - see plan Codebase Findings), and the `document.*` group (no
// existing "currently focused document pane" concept to resolve a `path`
// from without inventing new focus-tracking, which would violate R1's
// "never fabricates a selection").

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
    {
      id: "rootPicker.open",
      kind: "leaderSub",
      keys: leaderKeys("r"),
      commandId: "rootPicker.open",
      buildPayload: () => buildRootPickerOpenCommand().payload,
      description: "Open root picker",
    },
    activeRootBinding(
      "terminal.create",
      ["t"],
      "terminal.create",
      (root) => buildTerminalCreateCommand(root.workRootId, root.serverRoute).payload,
      "Open new terminal in active work root",
    ),
    activeRootBinding(
      "agentChat.create",
      ["a"],
      "agentChat.create",
      (root) => buildAgentChatCreateCommand(root.workRootId, root.serverRoute).payload,
      "Open new agent tab in active work root",
    ),
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
      ["g", "n"],
      "git.branchCreate.open",
      (root) =>
        buildGitBranchCreateOpenCommand(root.workRootId, root.serverRoute).payload,
      "Open new-branch form for active work root",
    ),
    activeRootBinding(
      "gitWorktreeAdd.open",
      ["g", "w"],
      "gitWorktreeAdd.open",
      (root) =>
        buildGitWorktreeAddOpenCommand(root.workspaceId, root.serverRoute).payload,
      "Open add-worktree form for active workspace",
    ),
    activeRootBinding(
      "workRoot.close",
      ["w", "c"],
      "workRoot.close",
      (root) => buildWorkRootCloseCommand(root.workRootId, root.serverRoute).payload,
      "Close active work root",
    ),
    activeRootBinding(
      "workRoot.activation.set",
      ["w", "o"],
      "workRoot.activation.set",
      (root) =>
        buildWorkRootActivationCommand(
          root.workRootId,
          root.activation === "online" ? "offline" : "online",
          root.serverRoute,
        ).payload,
      "Toggle active work root online/offline",
    ),
    activeRootBinding(
      "workspace.menu.open",
      ["s", "m"],
      "workspace.menu.open",
      (root) => buildWorkspaceMenuOpenCommand(root.workspaceId, root.serverRoute).payload,
      "Open workspace menu for active workspace",
    ),
    activeRootBinding(
      "workspace.remove",
      ["s", "x"],
      "workspace.remove",
      (root) => buildWorkspaceRemoveCommand(root.workspaceId, root.serverRoute).payload,
      "Remove active workspace",
    ),
  ];
}
