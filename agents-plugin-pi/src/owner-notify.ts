/** Shared Pi-native owner notification seam for one adapter session. */
export type OwnerNotificationType = "info" | "warning" | "error";
export type OwnershipOwnerNotificationFingerprint =
  | "ownership-metadata-update"
  | "ownership-home-removal"
  | "ownership-retention-scan"
  | "ownership-retention-start";

export interface OwnerNotifyRef {
  current: ((message: string, type?: OwnerNotificationType) => void) | undefined;
  notifyOwnershipOnce(fingerprint: OwnershipOwnerNotificationFingerprint, message: string): void;
}

let current: OwnerNotifyRef["current"];
const deliveredOwnership = new Set<OwnershipOwnerNotificationFingerprint>();

export const ownerNotifyRef: OwnerNotifyRef = {
  get current() { return current; },
  set current(next) {
    current = next;
    deliveredOwnership.clear();
  },
  notifyOwnershipOnce(fingerprint, message) {
    if (!current || deliveredOwnership.has(fingerprint)) return;
    deliveredOwnership.add(fingerprint);
    try { current(message, "warning"); } catch { /* human-only diagnostics never change operation outcomes */ }
  },
};
