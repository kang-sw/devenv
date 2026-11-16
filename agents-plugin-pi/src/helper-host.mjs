// A helper is a plain Node child, outside Pi's extension loader aliases. Bind
// its peers to the actual running CLI, never the producer's or cwd's Pi SDK.
import { existsSync, readFileSync, realpathSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

export function helperHost(cli) {
  if (!cli) return undefined;
  let entry;
  try { entry = realpathSync(cli); } catch { return undefined; }
  for (let dir = dirname(entry); ; dir = dirname(dir)) {
    let pkg;
    try { pkg = JSON.parse(readFileSync(join(dir, 'package.json'), 'utf8')); } catch { /* continue to package boundary */ }
    if (['@earendil-works/pi-coding-agent', '@mariozechner/pi-coding-agent'].includes(pkg?.name)) {
      const sdk = join(dirname(entry), 'index.js');
      if (!existsSync(sdk)) throw Error('Pi helper host SDK unavailable');
      // Peers can expose import-only entrypoints. Resolve through the host,
      // not this helper's dependency tree or ambient Jiti aliases.
      const { createJiti } = createRequire(import.meta.url)('jiti');
      const resolver = createJiti(pathToFileURL(entry).href, { fsCache: false, moduleCache: false, alias: {}, tsconfigPaths: false });
      const alias = { '@earendil-works/pi-coding-agent': sdk };
      for (const name of ['@earendil-works/pi-tui', '@earendil-works/pi-ai', '@earendil-works/pi-ai/compat']) alias[name] = realpathSync(fileURLToPath(resolver.esmResolve(name, { conditions: ['node', 'import'] })));
      return { sdk, alias };
    }
    if (dirname(dir) === dir) return undefined;
  }
}
