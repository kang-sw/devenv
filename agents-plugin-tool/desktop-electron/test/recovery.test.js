'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { createRecovery, safeURL } = require('../recovery');

function fixture() {
  let next = 0, retries = 0;
  const timers = new Map();
  const recovery = createRecovery({ retry: () => { retries++; },
    setTimer: (fn, ms) => { const id = ++next; timers.set(id, { fn, ms }); return id; },
    clearTimer: id => timers.delete(id) });
  return { recovery, timers, fire() {
    const [id, timer] = timers.entries().next().value;
    timers.delete(id); timer.fn(); return timer.ms;
  }, get retries() { return retries; } };
}

test('main frame failures retry with capped backoff and success resets it', () => {
  const f = fixture();
  f.recovery.failure(false, -105); f.recovery.failure(true, -3);
  assert.equal(f.timers.size, 0);
  for (const delay of [1000, 2000, 4000, 8000, 10000, 10000]) {
    f.recovery.failure(true, -105);
    f.recovery.failure(true, -105); // duplicate event must not schedule a second retry
    assert.equal(f.timers.size, 1);
    assert.equal(f.fire(), delay);
  }
  assert.equal(f.retries, 6);
  f.recovery.success(); f.recovery.failure(true, -105);
  assert.equal(f.fire(), 1000);
  f.recovery.success(); f.recovery.retryIfActive();
  assert.equal(f.timers.size, 0);
});

test('renderer death and persistent hangs retry; responsive cancels the hang', () => {
  const f = fixture();
  f.recovery.gone('clean-exit'); assert.equal(f.timers.size, 0);
  f.recovery.gone('crashed'); assert.equal(f.fire(), 1000);
  f.recovery.success(); f.recovery.unresponsive(); f.recovery.responsive();
  assert.equal(f.timers.size, 0);
  f.recovery.unresponsive(); assert.equal(f.fire(), 10000);
  assert.equal(f.fire(), 1000); assert.equal(f.retries, 2);
  f.recovery.unresponsive(); f.recovery.success(); assert.equal(f.timers.size, 0);
  f.recovery.unresponsive(); f.recovery.dispose(); assert.equal(f.timers.size, 0);
});

test('diagnostic URLs never include credentials, query or fragment', () => {
  assert.equal(safeURL('http://user:pass@localhost/path?login=secret#token'), 'http://localhost/path');
  assert.equal(safeURL('not a URL?login=secret'), '<invalid URL>');
});
