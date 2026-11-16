// Run by TestFilesWorkspacePickerBrowser; only filesystem answers are stubbed.
const assert = require('node:assert/strict');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, shots] = process.argv.slice(2);
const canonical = p => p.replace(/\\/g, '/').replace(/\/+$/, '') || '/';

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    for (const windows of [true, false]) {
      const roots = windows
        ? { host: 'fixture-host', home: 'C:\\Users\\u', volumes: ['C:\\', 'D:\\'] }
        : { host: 'fixture-host', home: '/home/u', volumes: ['/', '/mnt/data'] };
      const home = canonical(roots.home), top = windows ? 'C:' : '/';
      const entries = new Map(windows ? [
        [home, ['Documents']], ['C:/Users', ['u']], [top, ['Users']], ['D:', ['work space']], ['D:/work space', []]
      ] : [
        [home, ['Documents']], ['/home', ['u']], [top, ['home', 'mnt']], ['/mnt/data', ['work space']], ['/mnt/data/work space', []]
      ]);
      const context = await browser.newContext({ viewport: { width: 1280, height: 800 }, colorScheme: 'light' });
      await context.addCookies([
        { name: 'ws_dashboard_lang', value: 'en', url: base },
        { name: 'ws_dashboard_theme', value: 'light', url: base }
      ]);
      const page = await context.newPage(), errors = [], requests = [], previews = [];
      page.on('pageerror', e => errors.push(e.message));
      await page.route('**/s/host0/fs/roots', route => route.fulfill({ json: roots }));
      await page.route('**/s/host0/fs/list?**', route => {
        const url = new URL(route.request().url()), root = url.searchParams.get('root'), dir = url.searchParams.get('dir') || '';
        assert.ok(root, 'host0 listings always carry an explicit root');
        const folder = canonical(root.replace(/[\\/]+$/, '') + (dir ? '/' + dir : ''));
        requests.push([canonical(root), dir]);
        if (!entries.has(folder)) return route.fulfill({ status: 404, body: 'refused' });
        const normalizedRoot = canonical(root) + (/^[A-Z]:$/.test(canonical(root)) ? '/' : '');
        const abs = windows ? (folder + (/^[A-Z]:$/.test(folder) ? '/' : '')).replace(/\//g, '\\') : folder;
        return route.fulfill({ json: { root: normalizedRoot, name: normalizedRoot.split('/').filter(Boolean).pop() || '/',
          abs, sep: windows ? '\\' : '/', dir, session: false, entries: entries.get(folder).map(name => ({ name, dir: true })) } });
      });
      await page.route('**/rail/workspaces/preview', async route => {
        const body = route.request().postDataJSON(); previews.push(body.path);
        await route.fulfill({ json: { ok: true, root: body.path } });
      });
      await page.goto(base + '/s/livesession');
      const add = page.locator('.radd[data-add="ws"]');
      if (!(await add.isVisible())) await page.locator('#railbtn').click();
      await add.click();
      const pop = page.locator('.rpop:not([hidden])');
      await pop.locator('.spline .pbtn:not(.go)').click();
      const picker = page.locator('dialog.fworkspace-picker'), select = picker.locator('.froot-choices');
      const waitRoot = root => page.waitForFunction(root => {
        const n = document.querySelector('.fworkspace-picker .froot');
        return n && n.title === root;
      }, root);
      await waitRoot(home);
      console.log((windows ? 'Windows' : 'POSIX') + ' initial dropdown: ' + JSON.stringify(await select.evaluate(s => ({ value: s.value, index: s.selectedIndex, width: s.getBoundingClientRect().width, options: [...s.options].map(o => o.text) }))));
      assert.ok(await select.isVisible());
      assert.equal(await select.inputValue(), roots.home, 'root selection must match native roots despite slash-canonicalized listing');
      if (shots && windows) await page.screenshot({ path: path.join(shots, 'light-workspace-picker-windows.png') });
      const up = picker.locator('.fbar-ctl > button').nth(2);
      assert.equal(await up.isEnabled(), true, 'Up must cross the initial home boundary');
      await up.click(); await waitRoot(windows ? 'C:/Users' : '/home');
      assert.ok(await select.inputValue(), 'intermediate parent remains labeled in the root control');
      await up.click(); await waitRoot(windows ? 'C:/' : '/');
      assert.equal(await up.isEnabled(), false, 'Up stops at the filesystem/drive root');
      await picker.locator('.fhome').click(); await waitRoot(home);
      assert.equal(await select.inputValue(), roots.home, 'Home returns to the discovered home in one click');
      const other = roots.volumes[1];
      await select.selectOption(other); await waitRoot(windows ? 'D:/' : other);
      if (shots && windows) await page.screenshot({ path: path.join(shots, 'light-workspace-picker-drive.png') });
      await picker.locator('.fentry[data-entry="work space"]').dblclick();
      await page.waitForFunction(() => document.querySelector('.fworkspace-picker .fcrumb.last')?.textContent === 'work space');
      const expected = windows ? 'D:\\work space' : '/mnt/data/work space';
      await picker.locator('.fattachment .pr').click();
      await picker.waitFor({ state: 'detached' });
      assert.equal(await pop.locator('.spq').inputValue(), expected);
      await page.waitForFunction(() => !!document.querySelector('.rpop:not([hidden]) .spres .k'));
      assert.deepEqual(previews, [expected], 'Select feeds native absolute path into existing preview flow');
      assert.ok(requests.some(([root]) => root === top));
      assert.deepEqual(errors, []);
      console.log((windows ? 'Windows' : 'POSIX') + ': home -> parent -> filesystem root; Home; alternate volume; native directory selection: ok');
      await context.close();
    }
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exit(1); });
