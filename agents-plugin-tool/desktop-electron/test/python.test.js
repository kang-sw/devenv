'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { pythonAvailable } = require('../python');

test('Python probe uses only the daemon interpreter contract and actually runs a bounded command', async () => {
  for (const configured of [undefined, '', 'C:\\Python with spaces\\python.exe']) {
    const env = { PATH: 'inherited-path', ...(configured === undefined ? {} : { WS_MCP_HOLDER_PYTHON: configured }) };
    let calls = 0;
    assert.equal(await pythonAvailable({ env, run(command, args, options, done) {
      calls++;
      assert.equal(command, configured || 'python3');
      assert.deepEqual(args, ['-c', 'import sys']);
      assert.equal(options.env, env);
      assert.equal(options.timeout, 2000);
      assert.equal(options.windowsHide, true);
      assert.equal(options.shell, undefined);
      done(null, '', '');
    } }), true);
    assert.equal(calls, 1);
  }
});
test('spawn error, nonzero Store alias exit and timeout mean missing; no alternative search', async () => {
  for (const error of [Object.assign(new Error('not found'), { code: 'ENOENT' }),
    Object.assign(new Error('Store alias failed'), { code: 9009 }),
    Object.assign(new Error('timed out'), { killed: true, signal: 'SIGTERM' })]) {
    const calls = [];
    assert.equal(await pythonAvailable({ env: { WS_MCP_HOLDER_PYTHON: 'configured-python' }, run(command, _args, _options, done) {
      calls.push(command);
      done(error);
    } }), false);
    assert.deepEqual(calls, ['configured-python']);
  }
  assert.equal(await pythonAvailable({ run() { throw new Error('spawn failed'); } }), false);
});
