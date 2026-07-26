import {
  DEFAULT_NOTIFICATION_PREFS,
  loadNotificationPrefs,
  parseNotificationPrefs,
  saveNotificationPrefs,
  type NotificationPrefs,
} from "./notificationPrefs.js";

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

// --- Defaults ----------------------------------------------------------------

assertDeepEqual(
  DEFAULT_NOTIFICATION_PREFS,
  { enabled: false },
  "the default notification prefs start disabled",
);

// --- parseNotificationPrefs's own defensive checks, asserted directly ------
//
// Cycle 2 correction (test Important): a round trip through
// loadNotificationPrefs cannot discriminate a correctly rejecting parser
// from a permissive `Boolean(record.enabled)`-style coercion for these two
// inputs, because `DEFAULT_NOTIFICATION_PREFS.enabled` is `false` -
// `Boolean(undefined)` (missing `enabled`) and a thrown-and-swallowed
// property access (a non-object `raw`, caught by `loadNamespacedPrefs`'s own
// unrelated try/catch) both land on the exact same default value a correct
// rejection would produce. Asserting `parseNotificationPrefs`'s own return
// value directly - the same reason `terminalPrefs.ts` exports
// `parseTerminalFontSizeInput` for its own edge cases - closes that gap.

assertEqual(
  parseNotificationPrefs({}),
  null,
  "a payload missing the enabled field is rejected (returns null)",
);

assertEqual(
  parseNotificationPrefs(null),
  null,
  "a null value is rejected (returns null)",
);

assertEqual(
  parseNotificationPrefs("not an object"),
  null,
  "a non-object (string) value is rejected (returns null)",
);

assertEqual(
  parseNotificationPrefs({ enabled: "yes" }),
  null,
  "a non-boolean enabled field is rejected (returns null)",
);

assertDeepEqual(
  parseNotificationPrefs({ enabled: true }),
  { enabled: true },
  "a valid payload parses through unchanged",
);

// --- Persistence round trip --------------------------------------------------
//
// Mirrors terminalPrefs.test.ts's coverage of the parallel
// parseTerminalStylePrefs (review cycle 1, Important 2): this is the one
// piece of domain logic unique to notificationPrefs.ts, and it was
// previously exercised by no test at all. The malformed-payload cases below
// pin `loadNotificationPrefs`'s end-to-end fallback behavior (still real,
// still worth asserting) - they are not the proof of
// `parseNotificationPrefs`'s own rejection contract; the direct assertions
// above are.

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
    loadNotificationPrefs(storage),
    DEFAULT_NOTIFICATION_PREFS,
    "loading with nothing saved yet returns the default notification prefs",
  );

  const prefs: NotificationPrefs = { enabled: true };
  saveNotificationPrefs(prefs, storage);
  assertDeepEqual(
    loadNotificationPrefs(storage),
    prefs,
    "a saved notification pref round-trips through storage",
  );

  // Malformed payload -> falls back to defaults rather than throwing.
  fakeStorage.set("ws-dashboard.settings.notifications.v1", "{not json");
  assertDeepEqual(
    loadNotificationPrefs(storage),
    DEFAULT_NOTIFICATION_PREFS,
    "malformed JSON falls back to the default notification prefs",
  );

  // Malformed shape (missing `enabled`) -> rejected by parseNotificationPrefs,
  // falls back to defaults.
  fakeStorage.set(
    "ws-dashboard.settings.notifications.v1",
    JSON.stringify({ version: 1, value: {} }),
  );
  assertDeepEqual(
    loadNotificationPrefs(storage),
    DEFAULT_NOTIFICATION_PREFS,
    "a payload missing the enabled field falls back to the default notification prefs",
  );

  // Malformed shape (non-boolean `enabled`) -> rejected by
  // parseNotificationPrefs, falls back to defaults.
  fakeStorage.set(
    "ws-dashboard.settings.notifications.v1",
    JSON.stringify({ version: 1, value: { enabled: "yes" } }),
  );
  assertDeepEqual(
    loadNotificationPrefs(storage),
    DEFAULT_NOTIFICATION_PREFS,
    "a non-boolean enabled field falls back to the default notification prefs",
  );

  // Non-object `value` -> rejected by parseNotificationPrefs, falls back to
  // defaults.
  fakeStorage.set(
    "ws-dashboard.settings.notifications.v1",
    JSON.stringify({ version: 1, value: null }),
  );
  assertDeepEqual(
    loadNotificationPrefs(storage),
    DEFAULT_NOTIFICATION_PREFS,
    "a non-object value falls back to the default notification prefs",
  );

  // Version-mismatched payload -> falls back to defaults.
  fakeStorage.set(
    "ws-dashboard.settings.notifications.v1",
    JSON.stringify({ version: 2, value: prefs }),
  );
  assertDeepEqual(
    loadNotificationPrefs(storage),
    DEFAULT_NOTIFICATION_PREFS,
    "a version-mismatched payload falls back to the default notification prefs",
  );
}

assertEqual(true, true, "notificationPrefs tests completed");
