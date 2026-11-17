'use strict';
// Diagnostics are text only; the preload exposes only bounded daemon actions.
const diagnostic = new URLSearchParams(location.search);
for (const id of ['reason', 'log']) {
  const element = document.getElementById(id);
  if (element) element.textContent = diagnostic.get(id) || '';
}
const button = document.getElementById('start-daemon');
const status = document.getElementById('start-status');
const korean = navigator.language?.toLowerCase().startsWith('ko');
const label = korean ? '데몬 시작' : 'Start daemon';
button.textContent = label;
let starting = false;
window.werkschaftDesktop.daemon.availability().then(result => {
  button.disabled = !result.available;
  status.textContent = result.reason || '';
}).catch(() => { status.textContent = 'Daemon start is unavailable.'; });
button.addEventListener('click', async () => {
  if (starting || button.disabled) return;
  starting = true;
  button.disabled = true;
  button.textContent = korean ? '시작하는 중…' : 'Starting…';
  status.textContent = '';
  try { await window.werkschaftDesktop.daemon.start(); }
  catch (error) { status.textContent = error.message || String(error); }
  finally { starting = false; button.disabled = false; button.textContent = label; }
});
