const assert = require('node:assert/strict');
const { test } = require('node:test');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');

const source = readAssetSource('wsdashboard', 'dashboard.js');
const openStream = source.match(/  function openStream\(path, on\) \{[\s\S]*?\n  \}\n  \/\/ Immutable tree inputs/)[0].replace(/\n  \/\/ Immutable tree inputs$/, '');

test('visibility follows both sources and disposes each connection listener', () => {
  const listeners = new Map(), sockets = [];
  const doc = {
    hidden: true,
    addEventListener(type, fn) { if (!listeners.has(type)) listeners.set(type, new Set()); listeners.get(type).add(fn); },
    removeEventListener(type, fn) { listeners.get(type).delete(fn); }
  };
  function WebSocket(url) { this.url = url; this.readyState = 0; this.sent = []; this.send = s => this.sent.push(JSON.parse(s)); this.close = () => { this.readyState = 3; }; sockets.push(this); }
  WebSocket.OPEN = 1;
  const ctx = { doc, WebSocket, location: { protocol: 'http:', host: 'localhost' }, shellHidden: false };
  ctx.pageHidden = () => doc.hidden || ctx.shellHidden;
  vm.runInNewContext(openStream, ctx);
  const emit = type => { for (const fn of [...listeners.get(type)]) fn(); };
  const first = ctx.openStream('/events?h=x', {}), socket = sockets[0];
  assert.match(socket.url, /&display=0$/);
  doc.hidden = false;
  emit('visibilitychange'); // opening state: no stale send
  socket.readyState = 1; socket.onopen();
  assert.equal(socket.sent[0].visible, true);
  ctx.shellHidden = true; emit('ws-page-visibility');
  assert.equal(socket.sent[1].visible, false);
  first.close(); emit('visibilitychange');
  assert.equal(socket.sent.length, 2);
  assert.equal(listeners.get('visibilitychange').size, 0);
  const second = ctx.openStream('/rail/events', {});
  assert.match(sockets[1].url, /\?display=0$/);
  sockets[1].onerror();
  assert.equal(listeners.get('ws-page-visibility').size, 0);
  second.close();
});
