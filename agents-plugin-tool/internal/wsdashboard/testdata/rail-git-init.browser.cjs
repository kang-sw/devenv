// Run by TestGitInitBrowser against a disposable daemon, never the user's.
const assert = require('node:assert/strict');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, light, dark, shots] = process.argv.slice(2);

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    for (const [theme, folder] of [['light', light], ['dark', dark]]) {
      const context = await browser.newContext({ viewport: { width: 1280, height: 800 }, colorScheme: theme });
      await context.addCookies([
        { name: 'ws_dashboard_lang', value: 'ko', url: base },
        { name: 'ws_dashboard_theme', value: theme, url: base }
      ]);
      const page = await context.newPage();
      const errors = [];
      page.on('pageerror', e => errors.push(e.message));
      await page.goto(base + '/s/livesession');
      const workspace = page.locator(`.rws[data-ws=${JSON.stringify(folder)}]`);
      const button = workspace.locator('.rgit');
      const dialog = page.locator('dialog.rgitdlg');
      const shot = async name => {
        if (shots) await page.screenshot({ path: path.join(shots, theme + '-' + name + '.png') });
      };
      await button.waitFor({ state: 'visible' });
      assert.equal(await button.getAttribute('title'), 'git init하기');
      assert.equal(await button.getAttribute('aria-label'), 'git init하기');
      assert.equal(await workspace.locator('.rnew, .radd').count(), 0);
      await page.mouse.move(1250, 750);
      await page.waitForTimeout(150);
      assert.deepEqual(await button.evaluate(b => {
        const s = getComputedStyle(b), i = getComputedStyle(b.querySelector('.i'));
        return [s.width, s.height, s.opacity, i.width, i.height];
      }), ['18px', '18px', '0.6', '13px', '13px']);
      await shot('rest');

      await button.hover();
      await page.waitForTimeout(150);
      assert.equal(await button.evaluate(b => getComputedStyle(b).opacity), '1');
      await button.focus();
      await page.waitForTimeout(150);
      assert.equal(await button.evaluate(b => getComputedStyle(b).opacity), '1');
      await button.click();
      await dialog.waitFor({ state: 'visible' });
      assert.equal(await dialog.locator('.rdhd').innerText(), '이 폴더를 git 폴더로 만들까요?');
      assert.equal(await dialog.locator('.rdpath').innerText(), folder);
      assert.equal(await dialog.locator('.rdlead').innerText(), '이 폴더에서 git init을 실행합니다. 파일은 커밋하지 않으며, 끝나면 새 세션 버튼이 생깁니다.');
      assert.equal(await dialog.locator('.rdlead code').innerText(), 'git init');
      assert.equal(await dialog.locator('.pbtn.go').innerText(), 'git init');
      assert.equal(await dialog.locator('.pbtn:not(.go)').innerText(), '취소');
      assert.equal(await button.getAttribute('aria-expanded'), 'true');
      assert.equal(await dialog.locator('.rdhd .i').evaluate(i => getComputedStyle(i).color),
        await dialog.locator('.pbtn.go').evaluate(b => getComputedStyle(b).backgroundColor));
      await page.mouse.move(1250, 750);
      await shot('modal');

      await dialog.locator('.pbtn:not(.go)').click();
      await dialog.waitFor({ state: 'hidden' });
      assert.equal(await button.getAttribute('aria-expanded'), 'false');
      assert.equal(await button.evaluate(b => document.activeElement === b), true);
      await button.click();
      await page.keyboard.press('Escape');
      await dialog.waitFor({ state: 'hidden' });
      // Same sysopen visibility and dispatch gate as the other local controls.
      await page.evaluate(() => document.documentElement.removeAttribute('data-sysopen'));
      assert.equal(await button.isVisible(), false);
      await button.evaluate(b => b.click());
      assert.equal(await dialog.isVisible(), false);
      await page.evaluate(() => document.documentElement.setAttribute('data-sysopen', '1'));

      // Git-not-found uses the real modal's refusal box, not a toast.
      const missing = 'git을 찾을 수 없습니다. git을 설치한 뒤 다시 시도하세요.';
      await page.route('**/rail/workspaces/git-init', route => route.fulfill({ status: 502, contentType: 'text/plain', body: missing }));
      await button.click();
      await dialog.locator('.pbtn.go').click();
      await dialog.locator('.rdnote.bad:not([hidden])').waitFor();
      assert.equal(await dialog.locator('.rdnote.bad').innerText(), missing);
      assert.equal(await dialog.locator('.pbtn.go').isEnabled(), true);
      await page.mouse.move(1250, 750);
      await shot('error');
      await page.unroute('**/rail/workspaces/git-init');

      // Retrying executes the real route. The live rail must turn into a
      // repository with its unborn root and normal session/add controls.
      const response = page.waitForResponse(r => r.url().endsWith('/rail/workspaces/git-init'));
      await dialog.locator('.pbtn.go').click();
      assert.equal((await response).status(), 200);
      await dialog.waitFor({ state: 'hidden' });
      await button.waitFor({ state: 'detached' });
      await workspace.locator('.rnew').waitFor({ state: 'visible' });
      assert.equal(await workspace.locator('.wsbar .wsi use').getAttribute('href'), '#i-repo');
      assert.equal(await workspace.locator('.rwt .wtroot').count(), 1);
      assert.ok(await workspace.locator('.rwt .br').innerText());
      assert.equal(await workspace.locator('.wsbar .radd').count(), 1);
      assert.equal(await workspace.locator('.rnew').count(), 1);
      assert.equal(await page.locator('.wstoast').last().innerText(), 'notes을(를) git 폴더로 만들었습니다');
      // Hover the normal new-session button, matching the after mockup.
      await workspace.locator('.rnew').hover();
      await page.waitForTimeout(150);
      await shot('after');
      assert.deepEqual(errors, []);
      console.log(theme + ': rest, confirm/cancel/Escape, local gate, error/retry, unborn rail and toast: ok');
      await context.close();
    }
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exit(1); });
