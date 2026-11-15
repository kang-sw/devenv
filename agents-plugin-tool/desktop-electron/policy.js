'use strict';
const LEVELS = Object.freeze([0.5, 0.67, 0.75, 0.8, 0.9, 1, 1.1, 1.25, 1.5, 1.75, 2, 2.5, 3]);
function nextZoom(current, step) {
  if (step === 'reset') return 1;
  return step === 'in' ? LEVELS.find(n => n > current + 1e-6) ?? 3 :
    [...LEVELS].reverse().find(n => n < current - 1e-6) ?? 0.5;
}
function destination(raw, pin, isMainFrame = true) {
  if (!isMainFrame && (raw === 'about:blank' || raw === 'about:srcdoc')) return 'frame';
  let url;
  try { url = new URL(raw); } catch { return 'block'; }
  if (url.username || url.password) return 'block';
  if (url.protocol !== 'http:' && url.protocol !== 'https:') return 'block';
  if (pin && url.origin === new URL(pin).origin) return 'dashboard';
  return 'external';
}
module.exports = { LEVELS, nextZoom, destination };
