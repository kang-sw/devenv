'use strict';
// Numeric core precedence only; suffix/build time/hash never order equal cores.
function core(value) {
  if (typeof value !== 'string') return null;
  const match = /^v?(\d+)\.(\d+)\.(\d+)(?:-[A-Za-z0-9.-]+)?$/.exec(value);
  if (!match) return null;
  const parts = match.slice(1, 4).map(Number);
  return parts.every(Number.isSafeInteger) ? parts : null;
}
function version(candidate, running) {
  const a = core(candidate), b = core(running);
  if (!a || !b) return null;
  for (let i = 0; i < 3; i++) if (a[i] !== b[i]) return Math.sign(a[i] - b[i]);
  return 0;
}
module.exports = { core, version };
