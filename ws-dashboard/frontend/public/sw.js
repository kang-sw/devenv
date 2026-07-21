// Minimal no-op passthrough service worker.
//
// This worker exists solely to satisfy the browser installability
// heuristic (a registered SW with a `fetch` listener). It intentionally
// does not cache or intercept any request: every request falls through to
// normal network handling. Do not add `cache.addAll`/precache logic here —
// see ai-docs/spec/ws-web-dashboard/index.md#260721-ws-dashboard-pwa-installability
// for why a caching SW is deliberately out of scope.
self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (event) => event.waitUntil(self.clients.claim()));
self.addEventListener('fetch', () => {});
