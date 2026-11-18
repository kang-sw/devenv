'use strict';

// Run the drawer fixtures directly on hosts with Node but no Go test runtime.
// The Go wrappers use the same fixture text. No subprocesses or browser needed.
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

process.chdir(path.resolve(__dirname, '..'));
const go = fs.readFileSync('drawer_height_test.go', 'utf8');
const dashboard = fs.readFileSync('assets/dashboard-drawer.js', 'utf8');
const tracker = fs.readFileSync('assets/tracker-render.js', 'utf8');
const tick = String.fromCharCode(96);

function block(src, start, end) {
  const i = src.indexOf(start);
  const j = src.indexOf(end, i + start.length);
  if (i < 0 || j < 0) throw new Error('Missing block: ' + start);
  return src.slice(i, j);
}
function functions(src, name) {
  const match = src.match(new RegExp('\\n  function ' + name + '\\([^\\n]*\\{[^\\n]*\\}\\n|\\n  function ' + name + '\\([\\s\\S]*?\\n  \\}\\n'));
  if (!match) throw new Error('Missing function: ' + name);
  return match[0];
}
function run(name, script) {
  let output = '';
  const result = { stdout: { write: s => { output += s; } }, exitCode: 0 };
  vm.runInNewContext(script, { require, process: result }, { filename: name });
  if (output.trim() || result.exitCode) throw new Error(name + ': ' + output);
  console.log('PASS ' + name);
}

run('fixed maximum and resize/CSS contracts', go.split('const drawerFixedMaxNode = ' + tick)[1].split(tick)[0]);

for (const name of ['TestDrawerRoomResizeFrame', 'TestDrawerFoldButtons']) {
  const start = go.indexOf('func ' + name + '(t *testing.T) {');
  const call = go.indexOf('runTrackerNode(t, ', start);
  const definitions = go.slice(start, call);
  const vars = {};
  for (const line of definitions.split('\n')) {
    if (!line.includes('JSBlock(t, ')) continue;
    const key = line.split(' := ')[0].trim();
    const args = JSON.parse('[' + line.split('JSBlock(t, ')[1].trim().slice(0, -1) + ']');
    vars[key] = block(line.includes('dashboardJSBlock') ? dashboard : tracker, args[0], args[1]);
  }
  vars.headerBottom = functions(dashboard, 'drHeaderBottom');
  const first = go.indexOf(tick, call);
  const last = go.indexOf('\n' + tick + ')\n}', first);
  if (start < 0 || call < 0 || first < 0 || last < 0) throw new Error('Missing fixture: ' + name);
  const parts = go.slice(first, last + 2).split(tick);
  let script = '';
  for (let i = 1; i < parts.length; i += 2) {
    script += parts[i];
    const middle = (parts[i + 1] || '').replace('dashboardFuncs(t, "drHeaderBottom")', 'headerBottom');
    for (const key of middle.split('+').map(s => s.trim()).filter(Boolean)) {
      if (!(key in vars)) throw new Error('Unknown fixture source: ' + key);
      script += vars[key];
    }
  }
  run(name, script);
}
