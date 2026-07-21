import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './App';
import './styles.css';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

// Register the no-op passthrough service worker required for PWA
// installability. This does not add caching/offline behavior; see
// public/sw.js and ai-docs/spec/ws-web-dashboard/index.md
// #260721-ws-dashboard-pwa-installability.
if ('serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch((error) => {
      console.error('service worker registration failed', error);
    });
  });
}

// Stash the browser's install prompt so it can be triggered later from an
// in-app affordance instead of the browser's own (often easy-to-miss) UI.
// No UI is wired up to it yet — Phase 1 only needs the event captured.
window.addEventListener('beforeinstallprompt', (event) => {
  event.preventDefault();
  (window as typeof window & { deferredInstallPrompt?: Event }).deferredInstallPrompt = event;
});
