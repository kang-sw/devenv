import { loadNamespacedPrefs, saveNamespacedPrefs } from "./settingsStore.js";
import { browserStorage } from "./workRootFiles.js";

// Catalog of ligature-capable webfonts the owner can fetch on demand instead
// of being limited to whatever's installed on the OS. `googleFontsFamily` is
// the exact family name used both for the Google Fonts css2 API request and
// the resulting CSS `font-family` value.
export type DownloadableFontEntry = {
  readonly id: string;
  readonly label: string;
  readonly googleFontsFamily: string;
};

export const DOWNLOADABLE_FONTS: readonly DownloadableFontEntry[] = [
  { id: "fira-code", label: "Fira Code", googleFontsFamily: "Fira Code" },
  {
    id: "jetbrains-mono",
    label: "JetBrains Mono",
    googleFontsFamily: "JetBrains Mono",
  },
  {
    id: "victor-mono",
    label: "Victor Mono",
    googleFontsFamily: "Victor Mono",
  },
];

export type FontDownloadReason = "network" | "load-failed";

export class FontDownloadError extends Error {
  readonly reason: FontDownloadReason;
  constructor(reason: FontDownloadReason, message: string) {
    super(message);
    this.reason = reason;
  }
}

// Injects a `<link rel="stylesheet">` pointing at the Google Fonts css2 API
// rather than manually `fetch()`-ing + building a `FontFace(ArrayBuffer)`.
// This sidesteps any CORS uncertainty on Google's CSS endpoint entirely,
// since `<link>` subresource loading isn't subject to the same-origin fetch
// restrictions the way `fetch()` reads are.
function injectGoogleFontsLink(entry: DownloadableFontEntry): Promise<void> {
  const existing = document.querySelector(
    `link[data-downloadable-font="${entry.id}"]`,
  );
  if (existing) {
    return Promise.resolve();
  }
  return new Promise((resolve, reject) => {
    const link = document.createElement("link");
    link.rel = "stylesheet";
    const familyParam = entry.googleFontsFamily.split(" ").join("+");
    link.href = `https://fonts.googleapis.com/css2?family=${familyParam}&display=swap`;
    link.dataset.downloadableFont = entry.id;
    link.onload = () => resolve();
    link.onerror = () =>
      reject(
        new FontDownloadError(
          "network",
          `Could not reach the font stylesheet for "${entry.label}"`,
        ),
      );
    document.head.appendChild(link);
  });
}

// Fetches and registers a downloadable font's `@font-face` so it becomes
// usable for `font-family`. `document.fonts.load()` natively resolves once
// the browser has actually downloaded the matching face - no manual byte
// handling needed.
export async function downloadFont(
  entry: DownloadableFontEntry,
): Promise<void> {
  await injectGoogleFontsLink(entry); // already throws FontDownloadError on its own failure path
  let loaded: FontFace[];
  try {
    loaded = await document.fonts.load(`1em "${entry.googleFontsFamily}"`);
  } catch {
    throw new FontDownloadError(
      "load-failed",
      `The font file for "${entry.label}" failed to load`,
    );
  }
  if (loaded.length === 0) {
    throw new FontDownloadError(
      "load-failed",
      `No matching font face was found for "${entry.label}"`,
    );
  }
}

// Persistence: which font ids the owner has successfully downloaded before,
// so a page reload can silently re-fetch them. Follows the exact
// `loadTerminalStylePrefs`/`saveTerminalStylePrefs` pattern in
// `terminalPrefs.ts`.
const downloadedFontIdsStorageKey =
  "ws-dashboard.settings.terminal.downloaded-fonts.v1";
const downloadedFontIdsVersion = 1;

function parseDownloadedFontIds(raw: unknown): string[] | null {
  if (!Array.isArray(raw) || !raw.every((v) => typeof v === "string")) {
    return null;
  }
  return raw;
}

export function loadDownloadedFontIds(
  storage: Pick<Storage, "getItem"> | null = browserStorage(),
): string[] {
  return loadNamespacedPrefs(
    downloadedFontIdsStorageKey,
    downloadedFontIdsVersion,
    parseDownloadedFontIds,
    [],
    storage,
  );
}

export function saveDownloadedFontIds(
  ids: readonly string[],
  storage: Pick<Storage, "setItem"> | null = browserStorage(),
): void {
  saveNamespacedPrefs(
    downloadedFontIdsStorageKey,
    downloadedFontIdsVersion,
    ids,
    storage,
  );
}

// Best-effort re-download of every previously-downloaded font on app boot,
// since `document.fonts` state does not survive a page reload. Failures are
// swallowed silently here (this is background reconciliation, not a
// user-initiated action with visible feedback) -- a font that fails to
// re-register just falls through the CSS font-family stack unchanged.
export async function reregisterDownloadedFonts(): Promise<void> {
  const ids = loadDownloadedFontIds();
  const entries = DOWNLOADABLE_FONTS.filter((f) => ids.includes(f.id));
  await Promise.all(
    entries.map((entry) => downloadFont(entry).catch(() => {})),
  );
}
