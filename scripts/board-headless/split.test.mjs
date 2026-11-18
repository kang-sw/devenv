import assert from 'node:assert/strict';
import { test } from 'node:test';
import fs from 'node:fs';
import vm from 'node:vm';
import { execFileSync } from 'node:child_process';
import { createToolbarOverview } from './toolbar-overview.mjs';
import assetSource from '../../agents-plugin-tool/internal/wsdashboard/testdata/asset-source.cjs';

test('assembled dashboard source retains its single-script syntax', () => {
  new vm.Script(assetSource.readAssetSource('wsdashboard', 'dashboard.js'));
});

// Opt-in only for the move-only acceptance batch, not a permanent freeze on
// future feature changes. No daemon, browser or network is used by this test.
test('move-only sources equal the explicitly pinned baseline', { skip: !process.env.BOARD_SPLIT_BASELINE }, () => {
  const root = new URL('../../', import.meta.url);
  const baseline = path => execFileSync('git', ['show', process.env.BOARD_SPLIT_BASELINE + ':' + path], { cwd: root, windowsHide: true });
  for (const [pkg, name] of [['wsdashboard', 'dashboard.js'], ['wsdashboard', 'dashboard.css'], ['wswhiteboard', 'page.css'], ['wsdashboard', 'cards.js'], ['wswhiteboard', 'stream.js']]) {
    assert.equal(assetSource.readAssetSource(pkg, name), baseline('agents-plugin-tool/internal/' + pkg + '/assets/' + name).toString(), pkg + '/' + name);
  }
  const old = baseline('scripts/board-headless.mjs').toString();
  const entry = fs.readFileSync(new URL('../board-headless.mjs', import.meta.url), 'utf8');
  const shared = fs.readFileSync(new URL('./shared-actions.mjs', import.meta.url), 'utf8');
  const overview = fs.readFileSync(new URL('./toolbar-overview.mjs', import.meta.url), 'utf8');
  function span(source, from, to) {
    const start = source.indexOf(from), end = source.indexOf(to, start);
    assert(start >= 0 && end > start, 'source boundary ' + from + ' -> ' + to);
    return source.slice(start, end);
  }
  assert.equal(span(shared, 'async function scenarioSharedControls', 'return { scenarioSharedControls'), span(old, 'async function scenarioSharedControls', 'async function readWebPopover'));
  assert.equal(span(overview, '// --- toolbar / overview QA', 'async function readWebPopover'), span(old, '// --- toolbar / overview QA', 'async function scenarioSharedControls'));
  assert.equal(span(overview, 'async function readWebPopover', 'const shared = createSharedActions'), span(old, 'async function readWebPopover', "// BANDS_PAGE's bandTopics"));
  assert.equal(span(entry, 'const SCENARIOS = {', '// --- main'), span(old, 'const SCENARIOS = {', '// --- main'));
  // Reconstruct the WHOLE upstream harness, not just its scenario registry:
  // this pins fixture seeding/history and all upstream changes outside moves.
  const bindingStart = entry.indexOf('// Factories retain the existing shared helpers');
  const bindingEnd = entry.indexOf("// BANDS_PAGE's bandTopics", bindingStart);
  assert(bindingStart >= 0 && bindingEnd > bindingStart, 'factory binding boundary');
  let reconstructed = entry.slice(0, bindingStart) +
    span(overview, '// --- toolbar / overview QA', 'async function readWebPopover') +
    span(shared, 'async function scenarioSharedControls', 'return { scenarioSharedControls') +
    span(overview, 'async function readWebPopover', 'const shared = createSharedActions') + entry.slice(bindingEnd);
  const imports = "import { createToolbarOverview } from './board-headless/toolbar-overview.mjs';\n" +
    "import assetSource from '../agents-plugin-tool/internal/wsdashboard/testdata/asset-source.cjs';\n\n" +
    'const { readAssetSource } = assetSource;\n';
  assert(reconstructed.includes(imports), 'explicit source-reader/factory imports');
  reconstructed = reconstructed.replace(imports, '').replace(
    "const current = readAssetSource(file.split('/')[0], path.basename(file));",
    "const current = fs.readFileSync(path.join(REPO, relative), 'utf8');");
  assert.equal(reconstructed, old, 'all upstream entry/fixture/history code is retained');
});

const scenarioNames = [
  'scenarioSharedControls', 'scenarioSharedActions', 'scenarioToolbar',
  'scenarioWebContainment', 'scenarioRecapJitter', 'scenarioOverviewFocus',
  'scenarioOverviewFinal', 'scenarioOverview', 'scenarioOverviewMotion'
];

test('scenario factories retain registrations without starting a browser', () => {
  let opened = 0;
  const scenarios = createToolbarOverview({ openBoard: () => { opened++; } });
  assert.deepEqual(Object.keys(scenarios), [...scenarioNames.slice(0, 2), 'qa', ...scenarioNames.slice(2)]);
  for (const name of scenarioNames) assert.equal(typeof scenarios[name], 'function', name);
  assert.equal(opened, 0);
});

test('extracted scenarios use the entry point browser helper', async () => {
  const marker = new Error('stop before browser operations');
  const ctx = { args: {} };
  for (const name of scenarioNames.filter(name => name !== 'scenarioOverviewFinal')) {
    let seen;
    const scenarios = createToolbarOverview({ openBoard: async value => { seen = value; throw marker; } });
    await assert.rejects(scenarios[name](ctx), error => error === marker);
    assert.equal(seen, ctx, name);
  }
});

test('shared expectations retain the entry point failure and logging paths', () => {
  const failure = new Error('entry failure');
  const messages = [];
  const scenarios = createToolbarOverview({
    assert: (condition, message) => { if (!condition) { messages.push(message); throw failure; } },
    log: message => messages.push(message)
  });
  scenarios.qa(true, 'unchanged');
  assert.throws(() => scenarios.qa(false, 'failed', { value: 1 }), error => error === failure);
  assert.deepEqual(messages, ['ok: unchanged', 'failed: {"value":1}']);
});
