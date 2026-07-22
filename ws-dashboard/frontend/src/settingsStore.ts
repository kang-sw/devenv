import type { ComponentType } from "react";
import { browserStorage } from "./workRootFiles.js";

// --- Section registry contract ---------------------------------------------
//
// The Settings modal shell has no knowledge of section internals: it only
// ever renders `descriptor.Component` inside a labeled nav entry keyed by
// `descriptor.id`. New sections (e.g. a future hotkey-rebind editor) register
// by adding another descriptor to an ordered list; no shell code changes.

export type SettingsSectionDescriptor = {
  readonly id: string;
  readonly title: string;
  readonly Component: ComponentType;
};

// --- Namespaced prefs persistence -------------------------------------------
//
// Generalizes the `hotkeys.ts`/`workRootFiles.ts` `browserStorage()` +
// `"ws-dashboard.<feature>.v<N>"` versioned-JSON, defensive-parse pattern:
// try/catch-swallow around `localStorage`, a `{version, value}` envelope, and
// falling back to caller-supplied defaults on anything malformed or
// version-mismatched rather than throwing.

export function loadNamespacedPrefs<T>(
  key: string,
  version: number,
  parse: (raw: unknown) => T | null,
  defaults: T,
  storage: Pick<Storage, "getItem"> | null = browserStorage(),
): T {
  if (!storage) {
    return defaults;
  }
  try {
    const raw = storage.getItem(key);
    if (!raw) {
      return defaults;
    }
    const parsed = JSON.parse(raw) as { version?: unknown; value?: unknown };
    if (parsed.version !== version) {
      return defaults;
    }
    const value = parse(parsed.value);
    return value === null ? defaults : value;
  } catch {
    return defaults;
  }
}

export function saveNamespacedPrefs<T>(
  key: string,
  version: number,
  value: T,
  storage: Pick<Storage, "setItem"> | null = browserStorage(),
): void {
  if (!storage) {
    return;
  }
  try {
    storage.setItem(key, JSON.stringify({ version, value }));
  } catch {
    // Browser persistence is best-effort; in-memory state remains canonical.
  }
}
