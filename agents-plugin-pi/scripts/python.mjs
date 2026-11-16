import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

// Probe separately so a failing producer script is never run a second time.
export function runPython(args, options = {}, execute = execFileSync) {
  for (const [command, prefix] of [['python3', []], ['python', []], ['py', ['-3']]]) {
    try {
      execute(command, [...prefix, '-c', 'import sys; assert sys.version_info >= (3, 10)'],
        { ...options, stdio: 'pipe', timeout: 10000 });
    } catch {
      continue;
    }
    return execute(command, [...prefix, ...args], options);
  }
  throw new Error('Python 3.10+ is required; tried python3, python, and py -3.');
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    runPython(process.argv.slice(2), { stdio: 'inherit' });
  } catch (error) {
    console.error(error.message);
    process.exitCode = Number.isInteger(error.status) && error.status > 0 ? error.status : 1;
  }
}
