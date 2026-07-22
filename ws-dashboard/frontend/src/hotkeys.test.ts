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
  normalizeChordKey,
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

// --- Default binding table matches the finalized spec EXACTLY (regression) ---
//
// The table below is a literal transcription of the ticket's owner-approved
// "## Default Keymap & Interaction Spec (finalized 2026-07-22)" section,
// restricted to the leaves whose `DashboardCommandId` already exists today
// (the Phase 1 subset - see the exclusion list in hotkeys.ts's "Default
// bindings" section comment). It is NOT derived from
// `buildDefaultHotkeyBindings` - it is copied from the spec independently -
// so a future implementation change that repositions, drops, or adds a
// default binding fails this test in either direction, rather than only
// being caught by eyeball review. (This is the regression test for the
// Critical "8 of 13 default bindings contradict the finalized keymap"
// review finding.)

const EXPECTED_DEFAULT_KEYMAP: readonly {
  readonly keys: readonly string[];
  readonly commandId: string;
}[] = [
  // `r` Root/WorkRoot group (spec: "r o" / "r x" / "r t")
  { keys: ["r", "o"], commandId: "rootPicker.open" },
  { keys: ["r", "x"], commandId: "workRoot.close" },
  { keys: ["r", "t"], commandId: "workRoot.activation.set" },
  // `t` Terminal group (spec: "t n")
  { keys: ["t", "n"], commandId: "terminal.create" },
  // `a` Agent-chat group (spec: "a n")
  { keys: ["a", "n"], commandId: "agentChat.create" },
  // `g` Git group (spec: "g r"/"g f"/"g p"/"g l"/"g b"/"g c")
  { keys: ["g", "r"], commandId: "git.refresh" },
  { keys: ["g", "f"], commandId: "git.fetch" },
  { keys: ["g", "p"], commandId: "git.push" },
  { keys: ["g", "l"], commandId: "git.pullFfOnly" },
  { keys: ["g", "b"], commandId: "git.branchMenu.open" },
  { keys: ["g", "c"], commandId: "git.branchCreate.open" },
  // `g w` worktree sub-menu, 4-tier (spec: "g w a" / "g w x" / "g w m" +
  // "g w h" hidden-worktrees)
  { keys: ["g", "w", "a"], commandId: "gitWorktreeAdd.open" },
  { keys: ["g", "w", "x"], commandId: "workspace.remove" },
  { keys: ["g", "w", "m"], commandId: "workspace.menu.open" },
  { keys: ["g", "w", "h"], commandId: "worktreeHidden.menu.open" },
];

{
  const actual = buildDefaultHotkeyBindings();

  const actualKeySeq = (binding: HotkeyBinding<HotkeyDispatchContext>): string =>
    binding.keys.map((chord) => normalizeChordKey(chord.key)).join(",");
  const expectedKeySeq = (entry: { readonly keys: readonly string[] }): string =>
    entry.keys.map((key) => key.toLowerCase()).join(",");

  // Exhaustiveness (missing direction): every expected spec leaf has a
  // corresponding default binding, placed at exactly that key sequence.
  for (const expected of EXPECTED_DEFAULT_KEYMAP) {
    const match = actual.find((binding) => binding.commandId === expected.commandId);
    if (!match) {
      throw new Error(
        `expected a default binding for commandId "${expected.commandId}" at the finalized spec's key sequence "${expectedKeySeq(expected)}", but no default binding targets that commandId at all`,
      );
    }
    assertEqual(
      actualKeySeq(match),
      expectedKeySeq(expected),
      `default binding for "${expected.commandId}" is placed at the finalized spec's exact key sequence ("${expectedKeySeq(expected)}")`,
    );
    assertEqual(
      match.kind,
      "leaderSub",
      `default binding for "${expected.commandId}" is a leader-sub binding (spec: shipped defaults are always leader-sub)`,
    );
  }

  // Exhaustiveness (extra direction): no default binding exists for a
  // commandId outside the expected table - an extra binding would silently
  // ship keymap behavior the finalized spec doesn't define, the exact
  // Critical defect this regression test exists to catch.
  const expectedCommandIds = new Set(EXPECTED_DEFAULT_KEYMAP.map((e) => e.commandId));
  for (const binding of actual) {
    assertEqual(
      expectedCommandIds.has(binding.commandId),
      true,
      `default binding "${binding.id}" (commandId "${binding.commandId}") is not in the finalized spec's expected table - either it wasn't removed after the spec was finalized, or the table above needs updating to match a spec change`,
    );
  }

  // Exact set size (belt-and-suspenders on top of the two directions
  // above): rules out a duplicate commandId masking a missing/extra entry.
  assertEqual(
    actual.length,
    EXPECTED_DEFAULT_KEYMAP.length,
    `buildDefaultHotkeyBindings produces exactly ${EXPECTED_DEFAULT_KEYMAP.length} default bindings, matching the finalized spec's already-real-command-id subset one-to-one`,
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

  // pending + a bare modifier keydown (Shift/Control/Alt/Meta reported as
  // its own `key`) -> ignored in place, not treated as an unmatched key.
  // Otherwise holding Shift before a capitalized leaf (e.g. the finalized
  // spec's `<leader> ?`) would cancel the sequence before the actual key
  // arrives.
  for (const modifierKey of ["Shift", "Control", "Alt", "Meta"]) {
    const modifierStep = stepLeaderState(pending0, modifierKey, 1_070);
    assertEqual(
      modifierStep.action,
      "ignore",
      `pending + bare '${modifierKey}' keydown is ignored, not cancelled`,
    );
    assertEqual(
      modifierStep.state,
      pending0,
      `pending + bare '${modifierKey}' keydown leaves the pending state unchanged`,
    );
  }

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

// --- Leader tree: proper prefix TRIE resolution ---
//
// Precedence rule under test (documented alongside `LeaderTreeNode` and
// `stepLeaderState` in hotkeys.ts): a node with children is ALWAYS treated
// as a group and descends on the next keystroke; only a childless node (a
// true terminal leaf) resolves. A node may carry both a `binding` and
// children at the same time (a shorter registration sharing a prefix with a
// longer one) - when that happens, children win and the node's own binding
// never shadow-fires.

{
  // (a) Descend through a real multi-level group prefix (from the actual
  // default table's "g w a" gitWorktreeAdd.open) to its leaf and execute.
  const defaults = buildDefaultHotkeyBindings();
  const tree = buildLeaderTree(defaults);

  const afterG = stepLeaderState(enterLeaderPending(tree, 0), "g", 0);
  assertEqual(afterG.action, "narrow", "'g' narrows into the git group");
  const afterGW = stepLeaderState(afterG.state, "w", 0);
  assertEqual(
    afterGW.action,
    "narrow",
    "'g w' narrows further into the worktree sub-menu (a group, not a leaf)",
  );
  const afterGWA = stepLeaderState(afterGW.state, "a", 0);
  assertEqual(
    afterGWA.action,
    "resolve",
    "'g w a' resolves the worktree sub-menu's leaf",
  );
  assertEqual(
    afterGWA.binding?.commandId,
    "gitWorktreeAdd.open",
    "the resolved 3-deep leaf is gitWorktreeAdd.open, per the finalized spec's 'g w a' placement",
  );

  // (c) A reserved/registered group prefix with no leaf of its own stays
  // open for future children rather than resolving or cancelling - no
  // "leaf squatting" on `r`/`t`/`a`/`g w`. Using the real default table:
  // `r` alone must narrow (it holds `r o`/`r x`/`r t`, never a bare-`r`
  // binding), and `g w` alone must narrow (it holds `g w a`/`g w x`/`g w m`,
  // never a bare-`g w` binding).
  const afterR = stepLeaderState(enterLeaderPending(tree, 0), "r", 0);
  assertEqual(
    afterR.action,
    "narrow",
    "bare 'r' (the Root/WorkRoot group prefix) narrows rather than resolving or cancelling - it is not squatted by a leaf",
  );
  assertEqual(
    afterGW.action,
    "narrow",
    "bare 'g w' (the worktree sub-menu group prefix) narrows rather than resolving - not squatted by gitWorktreeAdd.open",
  );
}

{
  // (b) Synthetic mixed leaf+children node: a shorter binding registers a
  // leaf exactly where a longer binding also continues past it. Verifies
  // the state machine's children-win precedence directly (this exact shape
  // does not occur in the shipped default table, which deliberately avoids
  // it - see the "Group-prefix discipline" comment in hotkeys.ts - but the
  // trie/state-machine contract must hold for any future registration,
  // e.g. a which-key-overlay or command-bar binding set).
  const mixedBindings: readonly HotkeyBinding<HotkeyDispatchContext>[] = [
    {
      id: "shadowed-leaf",
      kind: "leaderSub",
      keys: leaderKeys("x"),
      commandId: "dashboard.refresh",
      buildPayload: () => ({ type: "refresh" }),
    },
    {
      id: "deeper-leaf",
      kind: "leaderSub",
      keys: leaderKeys("x", "y"),
      commandId: "terminal.create",
      buildPayload: () => ({ type: "terminal.create", workRootId: "r" }),
    },
  ];
  const mixedTree = buildLeaderTree(mixedBindings);
  const xNode = mixedTree.children.get("x");
  if (!xNode) {
    throw new Error("expected the 'x' node to exist in the mixed tree");
  }
  assertEqual(
    Boolean(xNode.binding),
    true,
    "the 'x' node itself carries a leaf binding (the shorter registration)",
  );
  assertEqual(
    xNode.children.size > 0,
    true,
    "the 'x' node ALSO carries children (the longer 'x y' registration)",
  );

  const stepIntoX = stepLeaderState(enterLeaderPending(mixedTree, 0), "x", 0);
  assertEqual(
    stepIntoX.action,
    "narrow",
    "a node with children always narrows and never shadow-fires its own leaf binding, even though 'x' alone is also a registered leaf",
  );
  assertEqual(
    stepIntoX.state.kind,
    "pending",
    "narrowing into the mixed node stays pending rather than resolving the shadowed leaf",
  );

  const stepIntoXY = stepLeaderState(stepIntoX.state, "y", 0);
  assertEqual(
    stepIntoXY.action,
    "resolve",
    "the deeper 'x y' leaf remains reachable and resolves normally",
  );
  assertEqual(
    stepIntoXY.binding?.id,
    "deeper-leaf",
    "'x y' resolves to the longer registration's binding",
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
