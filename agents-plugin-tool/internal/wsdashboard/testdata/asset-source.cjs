const fs = require('node:fs');
const path = require('node:path');

// Read the same ordered fragments as Go, without introducing separators.
function readAssetSource(pkg, name) {
  const root = path.resolve(__dirname, '../..', pkg);
  const declarations = {
    'wsdashboard/dashboard.js': ['pages.go', 'dashboardScriptFiles'],
    'wsdashboard/dashboard.css': ['pages.go', 'dashboardStyleFiles'],
    'wsdashboard/tracker.js': ['pages.go', 'trackerScriptFiles'],
    'wswhiteboard/page.css': ['render.go', 'pageStyleFiles'],
    'wswhiteboard/stream.js': ['render.go', 'pageStreamFiles'],
  };
  const declaration = declarations[pkg + '/' + name];
  if (!declaration) return fs.readFileSync(path.join(root, 'assets', name), 'utf8');
  const [file, variable] = declaration;
  const go = fs.readFileSync(path.join(root, file), 'utf8');
  const match = go.match(new RegExp('var ' + variable + ' = \\[\\]string\\{([^}]*)\\}'));
  if (!match) throw new Error(file + ' has no explicit ' + variable + ' string list');
  const entries = [...match[1].matchAll(/"([^"\\]+)"\s*,/g)];
  if (!entries.length || match[1].replace(/"([^"\\]+)"\s*,/g, '').trim()) {
    throw new Error(file + ' has an unsupported ' + variable + ' string list');
  }
  return entries.map(entry => fs.readFileSync(path.join(root, 'assets', entry[1]), 'utf8')).join('');
}

module.exports = { readAssetSource };
