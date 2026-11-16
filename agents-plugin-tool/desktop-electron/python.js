'use strict';
const { execFile } = require('node:child_process');

function pythonAvailable({ env = process.env, run = execFile } = {}) {
  // Keep selection in sync with internal/wsholder/manager.go: Manager.python (WS_MCP_HOLDER_PYTHON, else python3).
  const interpreter = env.WS_MCP_HOLDER_PYTHON || 'python3';
  return new Promise(resolve => {
    try {
      run(interpreter, ['-c', 'import sys'], { env, timeout: 2000, windowsHide: true }, error => resolve(!error));
    } catch { resolve(false); }
  });
}
module.exports = { pythonAvailable };
