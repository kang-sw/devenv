'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { LEVELS, nextZoom, destination } = require('../policy');
const pin = 'http://127.0.0.1:39418/';
test('zoom walks both ladders, snaps adjacent, resets and clamps', () => {
  for (let i = 0; i < LEVELS.length; i++) {
    assert.equal(nextZoom(LEVELS[i], 'in'), LEVELS[Math.min(i + 1, LEVELS.length - 1)]);
    assert.equal(nextZoom(LEVELS[i], 'out'), LEVELS[Math.max(0, i - 1)]);
    assert.equal(nextZoom(LEVELS[i], 'reset'), 1);
  }
  assert.equal(nextZoom(1.2, 'in'), 1.25);
  assert.equal(nextZoom(1.2, 'out'), 1.1);
});
test('exact origin stays, rediscovery replaces pin, other http(s) goes external', () => {
  for (const suffix of ['', 'tracker', 's/session?x=1#tail']) assert.equal(destination(pin + suffix, pin), 'dashboard');
  for (const url of ['http://127.0.0.1:39419/', 'http://localhost:39418/', 'http://[::1]:39418/', 'https://127.0.0.1:39418/', 'https://example.com/board']) {
    assert.equal(destination(url, pin), 'external');
    assert.equal(destination(url, pin, false), 'external');
  }
  assert.equal(destination(pin, 'http://127.0.0.1:39419/'), 'external');
  assert.equal(destination(pin, null), 'external');
});
test('schemes, userinfo and opaque top-level documents have no authority', () => {
  for (const raw of ['https://user:password@example.com/', pin.replace('127.', 'user@127.'), 'file:///etc/passwd', 'javascript:alert(1)', 'data:text/html,hi', 'mailto:user@example.com', 'ws://127.0.0.1/', 'custom:foo', 'about:blank', 'about:srcdoc', 'nonsense']) {
    assert.equal(destination(raw, pin), 'block', raw);
  }
  for (const raw of ['about:blank', 'about:srcdoc']) assert.equal(destination(raw, pin, false), 'frame');
});
