import {
  applyHotkeyUserConfig,
  buildDefaultHotkeyBindings,
  buildLeaderTree,
  checkHotkeyBindingKeys,
  chordFromKeydownEvent,
  enterLeaderPending,
  findStandaloneMatch,
  HotkeyRegistry,
  isLeaderTriggerKeydown,
  isReservedChord,
  leaderKeys,
  leaderPendingExpired,
  loadHotkeyUserConfig,
  resolveHotkeyCommand,
  saveHotkeyUserConfig,
  shouldSkipHotkeyCapture,
  stepLeaderState,
  DEFAULT_LEADER_TIMEOUT,
  type HotkeyBinding,
  type HotkeyDispatchContext,
  type HotkeyUserConfig,
} from "./hotkeys.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
  }
}

function assertDeepEqual<T>(actual: T, expected: T, label: string) {
  const actualJson = JSON.stringify(actual);
  const expectedJson = JSON.stringify(expected);
  if (actualJson !== expectedJson) {
    throw new Error(`${label}: expected ${expectedJson}, got ${actualJson}`);
  }
}

// --- Reserved-key rejection: default table (both reserved-key vectors) ---

assertEqual(
  isReservedChord({ key: "`", ctrl: true }),
  true,
  "Ctrl+` is a reserved chord",
);
assertEqual(
  isReservedChord({ key: "r", ctrl: true }),
  true,
  "Ctrl+R is a reserved chord",
);
assertEqual(
  isReservedChord({ key: "g", ctrl: true }),
  true,
  "Ctrl+G is a reserved chord",
);
assertEqual(
  isReservedChord({ key: "Enter", ctrl: true }),
  true,
  "Ctrl+Enter is a reserved chord",
);
assertEqual(
  isReservedChord({ key: "r", meta: true }),
  true,
  "Cmd+R is a reserved chord (meta parity)",
);
assertEqual(
  isReservedChord({ key: "r" }),
  false,
  "bare 'r' (no modifier) is not reserved - leader-sub leaf keys are unaffected",
);
assertEqual(
  isReservedChord({ key: "k", ctrl: true }),
  false,
  "Ctrl+K is not in the reserved set",
);

assertDeepEqual(
  checkHotkeyBindingKeys([{ key: "`", ctrl: true }]),
  { ok: false, reason: "reserved chord claimed: Ctrl+`" },
  "checkHotkeyBindingKeys rejects a reserved chord",
);
assertDeepEqual(
  checkHotkeyBindingKeys(leaderKeys("g", "r")),
  { ok: true },
  "checkHotkeyBindingKeys accepts a plain leader-sub sequence",
);

{
  const registry = new HotkeyRegistry<HotkeyDispatchContext>();
  let threw = false;
  try {
    registry.registerDefault({
      id: "bad.default",
      kind: "standalone",
      keys: [{ key: "g", ctrl: true }],
      commandId: "dashboard.refresh",
      buildPayload: () => ({ type: "refresh" }),
    });
  } catch {
    threw = true;
  }
  assertEqual(
    threw,
    true,
    "registerDefault throws (build/test-time bug) for a reserved-key default binding",
  );
}

{
  const registry = new HotkeyRegistry<HotkeyDispatchContext>();
  const result = registry.registerUser({
    id: "user.bad",
    kind: "standalone",
    keys: [{ key: "r", ctrl: true }],
    commandId: "dashboard.refresh",
    buildPayload: () => ({ type: "refresh" }),
  });
  assertEqual(
    result.ok,
    false,
    "registerUser rejects (does not throw) a reserved-key user rebind",
  );
  assertEqual(
    registry.get("user.bad"),
    undefined,
    "a rejected user rebind is never installed into the registry",
  );
}

{
  const registry = new HotkeyRegistry<HotkeyDispatchContext>();
  const result = registry.registerUser({
    id: "user.good",
    kind: "standalone",
    keys: [{ key: "k", ctrl: true }],
    commandId: "dashboard.refresh",
    buildPayload: () => ({ type: "refresh" }),
  });
  assertEqual(result.ok, true, "registerUser accepts a non-reserved chord");
  assertEqual(
    registry.get("user.good")?.id,
    "user.good",
    "an accepted user rebind is installed into the registry",
  );
}

// --- Default binding table: every default binding must itself pass the
// reserved-key check (defense in depth for the actual Phase 1 table). ---

{
  const defaults = buildDefaultHotkeyBindings();
  assertEqual(defaults.length > 0, true, "the default binding table is non-empty");
  const registry = new HotkeyRegistry<HotkeyDispatchContext>();
  for (const binding of defaults) {
    registry.registerDefault(binding); // throws on any reserved-key default
  }
  assertEqual(
    registry.list().length,
    defaults.length,
    "every default binding registers cleanly (none claims a reserved key)",
  );
}

// --- Dispatch: no-payload/context-free vs. payload-needing "opens a
// picker" (R1) vs. context-dependent-but-missing-context (no-op, not a
// fabricated selection). ---

{
  const defaults = buildDefaultHotkeyBindings();
  const rootPickerOpen = defaults.find((b) => b.id === "rootPicker.open");
  if (!rootPickerOpen) {
    throw new Error("expected a default rootPicker.open binding");
  }
  const command = resolveHotkeyCommand(rootPickerOpen, { activeRoot: null });
  assertEqual(
    command?.commandId,
    "rootPicker.open",
    "rootPicker.open dispatches without needing active-root context",
  );

  const gitBranchMenuOpen = defaults.find((b) => b.id === "git.branchMenu.open");
  if (!gitBranchMenuOpen) {
    throw new Error("expected a default git.branchMenu.open binding");
  }
  const noContextCommand = resolveHotkeyCommand(gitBranchMenuOpen, {
    activeRoot: null,
  });
  assertEqual(
    noContextCommand,
    null,
    "a context-needing binding resolves to null (no-op) rather than fabricating a selection when there is no active root",
  );
  const withContextCommand = resolveHotkeyCommand(gitBranchMenuOpen, {
    activeRoot: {
      workRootId: "root-1",
      serverRoute: "server-local",
      workspaceId: "workspace-1",
      activation: "online",
    },
  });
  assertEqual(
    withContextCommand?.commandId,
    "git.branchMenu.open",
    "git.branchMenu.open (R1 'opens a menu' case) dispatches once active-root context is available",
  );
}

// --- Leader-mode state machine ---

{
  const bindings: readonly HotkeyBinding<HotkeyDispatchContext>[] = [
    {
      id: "t",
      kind: "leaderSub",
      keys: leaderKeys("t"),
      commandId: "terminal.create",
      buildPayload: () => ({ type: "terminal.create", workRootId: "r" }),
    },
    {
      id: "g.r",
      kind: "leaderSub",
      keys: leaderKeys("g", "r"),
      commandId: "git.refresh",
      buildPayload: () => ({ type: "git.refresh", workRootId: "r" }),
    },
  ];
  const tree = buildLeaderTree(bindings);

  // idle -> pending on leader keydown (entry point; the actual `Ctrl+Space`
  // detection lives in the DOM-facing `isLeaderTriggerKeydown` predicate
  // tested separately below).
  const pending0 = enterLeaderPending(tree, 1_000);
  assertEqual(pending0.kind, "pending", "entering leader mode transitions to pending");

  // pending + matching single-key leaf -> resolved + idle
  const resolved = stepLeaderState(pending0, "t", 1_010);
  assertEqual(resolved.action, "resolve", "pending + matching leaf key resolves");
  assertEqual(
    resolved.binding?.id,
    "t",
    "the resolved binding is the matching leaf binding",
  );
  assertEqual(resolved.state.kind, "idle", "resolving returns to idle");

  // pending + a key that only narrows a group -> stays pending
  const narrowed = stepLeaderState(pending0, "g", 1_020);
  assertEqual(narrowed.action, "narrow", "pending + group-narrowing key narrows");
  assertEqual(narrowed.state.kind, "pending", "narrowing stays pending");

  // narrowed pending + matching leaf key -> resolved
  const resolvedAfterNarrow = stepLeaderState(narrowed.state, "r", 1_030);
  assertEqual(
    resolvedAfterNarrow.action,
    "resolve",
    "resolving after a narrow still resolves the deeper leaf",
  );
  assertEqual(
    resolvedAfterNarrow.binding?.id,
    "g.r",
    "the deeper leaf resolves to the two-key sequence's binding",
  );

  // pending + unmatched key -> idle (cancel)
  const cancelledUnmatched = stepLeaderState(pending0, "z", 1_040);
  assertEqual(
    cancelledUnmatched.action,
    "cancel",
    "pending + unmatched key cancels",
  );
  assertEqual(
    cancelledUnmatched.state.kind,
    "idle",
    "cancelling on unmatched key returns to idle",
  );

  // pending + Escape -> idle (cancel), even mid-sequence
  const cancelledEscape = stepLeaderState(narrowed.state, "Escape", 1_050);
  assertEqual(cancelledEscape.action, "cancel", "Escape cancels a pending sequence");
  assertEqual(
    cancelledEscape.state.kind,
    "idle",
    "Escape cancel returns to idle",
  );

  // idle + any key -> ignore (no active leader sequence)
  const ignored = stepLeaderState({ kind: "idle" }, "t", 1_060);
  assertEqual(ignored.action, "ignore", "idle state ignores keys until a leader press");

  // Configurable timeout, DEFAULT OFF (finalized spec supersedes the
  // ticket's older "times out/cancels" Phase 1 bullet wording).
  assertEqual(
    DEFAULT_LEADER_TIMEOUT.enabled,
    false,
    "the default leader-pending timeout is disabled",
  );
  assertEqual(
    leaderPendingExpired(pending0, 1_000_000, DEFAULT_LEADER_TIMEOUT),
    false,
    "a disabled timeout never expires pending state, no matter how much time elapses",
  );
  const enabledTimeout = { enabled: true, ms: 100 };
  assertEqual(
    leaderPendingExpired(pending0, 1_050, enabledTimeout),
    false,
    "an enabled timeout has not yet expired before its window elapses",
  );
  assertEqual(
    leaderPendingExpired(pending0, 1_150, enabledTimeout),
    true,
    "an enabled timeout expires once its window elapses",
  );
}

// --- Leader trigger detection (`Ctrl+Space`) ---

assertEqual(
  isLeaderTriggerKeydown({
    key: " ",
    ctrlKey: true,
    metaKey: false,
    shiftKey: false,
    altKey: false,
  }),
  true,
  "Ctrl+Space is the leader trigger",
);
assertEqual(
  isLeaderTriggerKeydown({
    key: " ",
    ctrlKey: false,
    metaKey: true,
    shiftKey: false,
    altKey: false,
  }),
  false,
  "Cmd+Space is deliberately not the leader trigger (commonly OS-reserved)",
);
assertEqual(
  isLeaderTriggerKeydown({
    key: "a",
    ctrlKey: true,
    metaKey: false,
    shiftKey: false,
    altKey: false,
  }),
  false,
  "Ctrl+A is not the leader trigger",
);

// --- Standalone hotkey matching (additive user-configurable layer) ---

{
  const standaloneBinding: HotkeyBinding<HotkeyDispatchContext> = {
    id: "standalone.refresh",
    kind: "standalone",
    keys: [{ key: "k", ctrl: true }],
    commandId: "dashboard.refresh",
    buildPayload: () => ({ type: "refresh" }),
  };
  const chord = chordFromKeydownEvent({
    key: "k",
    ctrlKey: true,
    metaKey: false,
    shiftKey: false,
    altKey: false,
  });
  const match = findStandaloneMatch([standaloneBinding], chord);
  assertEqual(
    match?.id,
    "standalone.refresh",
    "a registered standalone binding matches its exact chord",
  );
  const noMatch = findStandaloneMatch(
    [standaloneBinding],
    chordFromKeydownEvent({
      key: "k",
      ctrlKey: false,
      metaKey: false,
      shiftKey: false,
      altKey: false,
    }),
  );
  assertEqual(
    noMatch,
    null,
    "a chord missing the required modifier does not match",
  );
}

// --- Persistence round-trip ---

{
  const fakeStorage = new Map<string, string>();
  const storage = {
    getItem: (key: string) => fakeStorage.get(key) ?? null,
    setItem: (key: string, value: string) => {
      fakeStorage.set(key, value);
    },
    removeItem: (key: string) => {
      fakeStorage.delete(key);
    },
  };

  assertDeepEqual(
    loadHotkeyUserConfig(storage),
    { rebinds: [] },
    "loading with nothing saved yet returns the empty config",
  );

  const config: HotkeyUserConfig = {
    rebinds: [
      {
        id: "terminal.create",
        kind: "leaderSub",
        keys: leaderKeys("y"),
      },
    ],
  };
  saveHotkeyUserConfig(config, storage);
  const reloaded = loadHotkeyUserConfig(storage);
  // The loaded chord's optional modifier fields are normalized to explicit
  // `false` by the defensive parser (rather than staying `undefined`), so
  // compare against that normalized shape instead of the sparse input.
  assertDeepEqual(
    reloaded,
    {
      rebinds: [
        {
          id: "terminal.create",
          kind: "leaderSub",
          keys: [{ key: "y", ctrl: false, meta: false, shift: false, alt: false }],
        },
      ],
    },
    "a saved user rebind round-trips through storage (chord modifiers normalized to explicit false)",
  );

  // Malformed payload -> falls back to empty rather than throwing.
  fakeStorage.set("ws-dashboard.hotkeys.v1", "{not json");
  assertDeepEqual(
    loadHotkeyUserConfig(storage),
    { rebinds: [] },
    "malformed JSON falls back to the empty config",
  );

  // Version-mismatched payload -> falls back to empty.
  fakeStorage.set(
    "ws-dashboard.hotkeys.v1",
    JSON.stringify({ version: 2, rebinds: [] }),
  );
  assertDeepEqual(
    loadHotkeyUserConfig(storage),
    { rebinds: [] },
    "a version-mismatched payload falls back to the empty config",
  );

  // Saving an empty config clears the stored key rather than writing an
  // empty array (mirrors the workRootFiles.ts precedent).
  saveHotkeyUserConfig({ rebinds: [] }, storage);
  assertEqual(
    fakeStorage.has("ws-dashboard.hotkeys.v1"),
    false,
    "saving an empty rebind list removes the stored key",
  );
}

// --- applyHotkeyUserConfig: rebinding an existing default id, and
// rejecting an unknown id / reserved-key rebind without throwing. ---

{
  const defaults = buildDefaultHotkeyBindings();
  const applied = applyHotkeyUserConfig(defaults, {
    rebinds: [
      { id: "terminal.create", kind: "leaderSub", keys: leaderKeys("y") },
      { id: "does.not.exist", kind: "leaderSub", keys: leaderKeys("z") },
      { id: "agentChat.create", kind: "standalone", keys: [{ key: "g", ctrl: true }] },
    ],
  });
  const rebound = applied.bindings.find((b) => b.id === "terminal.create");
  assertDeepEqual(
    rebound?.keys,
    leaderKeys("y"),
    "a valid rebind overrides the default binding's trigger keys",
  );
  assertEqual(
    applied.bindings.length,
    defaults.length,
    "rebinding never changes the total binding count",
  );
  assertEqual(
    applied.rejected.some((r) => r.id === "does.not.exist"),
    true,
    "a rebind targeting an unknown binding id is rejected",
  );
  assertEqual(
    applied.rejected.some((r) => r.id === "agentChat.create"),
    true,
    "a rebind that claims a reserved chord is rejected, not silently applied",
  );
  const unchangedAgentChat = applied.bindings.find((b) => b.id === "agentChat.create");
  assertDeepEqual(
    unchangedAgentChat?.keys,
    defaults.find((b) => b.id === "agentChat.create")?.keys,
    "a rejected rebind leaves the original default binding's keys untouched",
  );
}

// --- Terminal-focus / IME capture guard ---

assertEqual(
  shouldSkipHotkeyCapture({
    isComposing: true,
    key: "t",
    targetIsEditable: false,
    targetInsideTerminalPane: false,
  }),
  true,
  "an IME composition in progress skips capture",
);
assertEqual(
  shouldSkipHotkeyCapture({
    isComposing: false,
    key: "Process",
    targetIsEditable: false,
    targetInsideTerminalPane: false,
  }),
  true,
  "key === 'Process' (IME) skips capture",
);
assertEqual(
  shouldSkipHotkeyCapture({
    isComposing: false,
    key: "t",
    targetIsEditable: true,
    targetInsideTerminalPane: false,
  }),
  true,
  "an editable target (input/textarea/select/contentEditable) skips capture",
);
assertEqual(
  shouldSkipHotkeyCapture({
    isComposing: false,
    key: "t",
    targetIsEditable: false,
    targetInsideTerminalPane: true,
  }),
  true,
  "focus already inside a terminal pane skips capture (terminal passthrough)",
);
assertEqual(
  shouldSkipHotkeyCapture({
    isComposing: false,
    key: "t",
    targetIsEditable: false,
    targetInsideTerminalPane: false,
  }),
  false,
  "a plain keydown outside any guarded target does not skip capture",
);

console.log("hotkeys.test.ts passed");
