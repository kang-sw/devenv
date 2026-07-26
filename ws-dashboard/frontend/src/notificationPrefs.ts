import { loadNamespacedPrefs, saveNamespacedPrefs } from "./settingsStore.js";
import { browserStorage } from "./workRootFiles.js";

// The Settings > Notifications section's persisted opt-in, mirroring
// `terminalPrefs.ts`'s exact shape (typed prefs object, defensive `parse*`,
// `load*`/`save*` wrappers over the shared versioned-JSON namespace). This is
// the ONLY thing persisted here - whether the OS `Notification` tier is
// enabled. It carries no cached `Notification.permission` value: that is
// re-read live from the `Notification` global itself (see
// `settingsSections.tsx`'s `NotificationSection`), never duplicated into
// storage, since the browser is the sole source of truth for permission
// state and a stale cached copy would drift from it across profile/OS
// changes made outside this app.
export type NotificationPrefs = {
  readonly enabled: boolean;
};

export const DEFAULT_NOTIFICATION_PREFS: NotificationPrefs = {
  enabled: false,
};

const notificationPrefsStorageKey = "ws-dashboard.settings.notifications.v1";
const notificationPrefsVersion = 1;

function parseNotificationPrefs(raw: unknown): NotificationPrefs | null {
  if (typeof raw !== "object" || raw === null) {
    return null;
  }
  const record = raw as Record<string, unknown>;
  if (typeof record.enabled !== "boolean") {
    return null;
  }
  return { enabled: record.enabled };
}

export function loadNotificationPrefs(
  storage: Pick<Storage, "getItem"> | null = browserStorage(),
): NotificationPrefs {
  return loadNamespacedPrefs(
    notificationPrefsStorageKey,
    notificationPrefsVersion,
    parseNotificationPrefs,
    DEFAULT_NOTIFICATION_PREFS,
    storage,
  );
}

export function saveNotificationPrefs(
  prefs: NotificationPrefs,
  storage: Pick<Storage, "setItem"> | null = browserStorage(),
): void {
  saveNamespacedPrefs(
    notificationPrefsStorageKey,
    notificationPrefsVersion,
    prefs,
    storage,
  );
}
