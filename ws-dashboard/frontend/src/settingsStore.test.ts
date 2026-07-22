import { loadNamespacedPrefs, saveNamespacedPrefs } from "./settingsStore.js";

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

type FakePrefs = { readonly label: string; readonly count: number };

function parseFakePrefs(raw: unknown): FakePrefs | null {
  if (typeof raw !== "object" || raw === null) {
    return null;
  }
  const record = raw as Record<string, unknown>;
  if (typeof record.label !== "string" || typeof record.count !== "number") {
    return null;
  }
  return { label: record.label, count: record.count };
}

const defaults: FakePrefs = { label: "default", count: 0 };
const storageKey = "ws-dashboard.fakeFeature.v1";

// --- Round trip -------------------------------------------------------------

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
    loadNamespacedPrefs(storageKey, 1, parseFakePrefs, defaults, storage),
    defaults,
    "loading with nothing saved yet returns the caller-supplied defaults",
  );

  const saved: FakePrefs = { label: "custom", count: 3 };
  saveNamespacedPrefs(storageKey, 1, saved, storage);
  assertDeepEqual(
    loadNamespacedPrefs(storageKey, 1, parseFakePrefs, defaults, storage),
    saved,
    "a saved value round-trips through storage",
  );
}

// --- Version mismatch falls back to defaults --------------------------------

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

  saveNamespacedPrefs(storageKey, 1, { label: "custom", count: 3 }, storage);
  assertDeepEqual(
    loadNamespacedPrefs(storageKey, 2, parseFakePrefs, defaults, storage),
    defaults,
    "loading with a mismatched version falls back to defaults",
  );
}

// --- Malformed JSON / malformed value fall back to defaults -----------------

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

  fakeStorage.set(storageKey, "{not json");
  assertDeepEqual(
    loadNamespacedPrefs(storageKey, 1, parseFakePrefs, defaults, storage),
    defaults,
    "malformed JSON falls back to defaults",
  );

  fakeStorage.set(
    storageKey,
    JSON.stringify({ version: 1, value: { label: "custom" } }),
  );
  assertDeepEqual(
    loadNamespacedPrefs(storageKey, 1, parseFakePrefs, defaults, storage),
    defaults,
    "a value that fails the caller's parse falls back to defaults",
  );
}

// --- Null storage (no `window`/localStorage available) ---------------------

{
  assertDeepEqual(
    loadNamespacedPrefs(storageKey, 1, parseFakePrefs, defaults, null),
    defaults,
    "loading with null storage returns defaults",
  );
  // Saving with null storage must not throw.
  saveNamespacedPrefs(storageKey, 1, { label: "custom", count: 3 }, null);
}

assertEqual(true, true, "settingsStore tests completed");
