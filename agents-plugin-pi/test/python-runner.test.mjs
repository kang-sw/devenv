import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { runPython } from '../scripts/python.mjs';

test('Python selection supports python3, python, and Windows py -3 in order', () => {
  for (const available of ['python3', 'python', 'py']) {
    const calls = [];
    const options = { cwd: '/fixture', env: { FIXTURE: '1' } };
    const result = runPython(['-I', 'fixture.py', '--check'], options, (command, args, opts) => {
      calls.push([command, args]);
      assert.equal(opts.cwd, options.cwd);
      assert.equal(opts.env, options.env);
      if (command !== available) throw Object.assign(new Error('unavailable'), { status: 1 });
      return args.includes('fixture.py') ? Buffer.from('ok') : Buffer.alloc(0);
    });
    assert.equal(result.toString(), 'ok');
    assert.deepEqual(calls.map(([command]) => command),
      [...['python3', 'python', 'py'].slice(0, ['python3', 'python', 'py'].indexOf(available) + 1), available]);
    assert.deepEqual(calls.at(-1), [available, [...(available === 'py' ? ['-3'] : []), '-I', 'fixture.py', '--check']]);
  }
});

test('a selected Python script failure is propagated without retrying another interpreter', () => {
  const failure = Object.assign(new Error('script failed'), { status: 7 });
  const calls = [];
  assert.throws(() => runPython(['fixture.py'], {}, (command, args) => {
    calls.push(command);
    if (args.includes('fixture.py')) throw failure;
    return Buffer.alloc(0);
  }), error => error === failure);
  assert.deepEqual(calls, ['python3', 'python3']);
});

test('missing Python reports the interpreter candidates', () => {
  assert.throws(() => runPython([], {}, () => { throw new Error('not found'); }), /python3.*python.*py -3/);
});

test('package preparation and consumer extraction share the portable Python runner', () => {
  const pkg = JSON.parse(readFileSync(new URL('../package.json', import.meta.url)));
  assert.equal(pkg.scripts['workflow-bundles'], 'node scripts/python.mjs scripts/prepare-workflow-bundles.py');
  assert.equal(pkg.scripts['native-payload'], 'node scripts/python.mjs scripts/prepare-native-payload.py');
  for (const hook of ['prepare', 'prepack']) assert.match(pkg.scripts[hook], /npm run workflow-bundles/);
  const consumer = readFileSync(new URL('./workflow-bundle-consumer.test.mjs', import.meta.url), 'utf8');
  assert.match(consumer, /runPython\(\['-I', '-c'/);
  assert.doesNotMatch(consumer, /execFileSync\('python/);
});
