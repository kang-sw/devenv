const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, captures, mockup, workspace, root, linked, session, projectPath] = process.argv.slice(2);
const viewport = { width: 1440, height: 960 };
const endpoint = (scope, target, worktree, sid) => {
  const q = new URLSearchParams({ scope });
  if (target) q.set('target', target);
  if (worktree) q.set('worktree', worktree);
  if (sid) q.set('session', sid);
  return base + '/settings?' + q;
};
const secrets = /secret-123|example\.com|secret-env|never-return-this-session-key|\/private\/bin/;
const snapshot = '/api/config/snapshot';

(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport, locale: 'ko-KR', reducedMotion: 'reduce' });
  try {
    const page = await context.newPage();
    const errors = [];
    page.setDefaultTimeout(10000);
    page.on('pageerror', error => errors.push(error.message));
    const frame = () => page.frameLocator('iframe.sframe.on');
    const row = key => frame().locator('[data-config-key=' + JSON.stringify(key) + ']');
    const gear = (scope, target) => page.locator('[data-scope-entry=' + scope + ']' + (target ? '[data-settings-target=' + JSON.stringify(target) + ']' : ''));
    const scopeButton = name => frame().locator('[data-settings-precedence] [data-settings-scope=' + name + ']');
    const proposal = name => frame().locator('[data-settings-proposal-action=' + name + ']');
    const show = async (scope, target, worktree, sid) => {
      const wanted = new URL(endpoint(scope, target, worktree, sid));
      await page.waitForURL(url => url.pathname === '/settings' &&
        (url.searchParams.get('scope') || 'global') === scope &&
        ['target','worktree','session'].every(key => url.searchParams.get(key) === wanted.searchParams.get(key)));
      await row('workflow.lang').waitFor();
    };
    async function setLanguage(value, snapshotURL, waitsForPost = true) {
      const field = row('workflow.lang').locator('input[type=text]');
      await field.fill(value);
      const wait = waitsForPost ? page.waitForResponse(r => r.url().endsWith('/api/config/set') && r.request().method() === 'POST') : null;
      await row('workflow.lang').locator('button[type=submit]').click();
      if (wait) {
        assert.equal((await wait).status(), 200);
        const snapshotData = await (await page.request.get(snapshotURL)).json();
        assert(snapshotData.entries.some(e => e.key === 'workflow.lang' && e.layers.some(l => l.scope === 'global' && l.value === value)));
      }
    }
    async function pickSection(name) {
      await frame().locator('[data-settings-section=' + name + ']').click();
    }
    async function capturePairs() {
      const reference = await context.newPage();
      for (const theme of ['dark', 'light']) {
        await context.addCookies([{name: 'ws_dashboard_theme', value: theme, url: base},
          {name: 'ws_dashboard_lang', value: 'ko', url: base}]);
        await page.goto(base + '/settings');
        await show('global');
        assert.equal(await frame().locator('[data-settings-modes]').count(), 0);
        assert.match(await frame().locator('[data-settings-precedence] .prec-hint').textContent(), /\S/);
        const repoGear = gear('repo', root);
        assert.equal(await repoGear.evaluate(el => getComputedStyle(el).opacity), '0');
        await page.locator('.wthd', { has: repoGear }).hover();
        assert(Number(await repoGear.evaluate(el => getComputedStyle(el).opacity)) >= 0.7);
        if (theme === 'dark') {
          await page.screenshot({path: path.join(captures, 'global-dark-rail-hover.png')});
          await page.mouse.move(720, 480);
          assert.equal(await repoGear.evaluate(el => getComputedStyle(el).opacity), '0');
        }
        await gear('repo', linked).waitFor();
        await row('workflow.whiteboard_enabled').locator('[data-settings-bucket=claude]').click();
        assert.equal(await row('workflow.whiteboard_enabled').locator('[data-settings-bucket=claude]').getAttribute('data-bucket-override'), 'other');
        await row('workflow.profile').locator('[data-settings-bucket=claude]').click();
        assert.equal(await row('workflow.profile').locator('[data-settings-bucket=claude]').getAttribute('data-bucket-override'), 'own');
        const profile = row('workflow.profile');
        await profile.locator('[data-settings-option=orchestrate]').click();
        const configured = page.waitForResponse(r => r.url().endsWith('/api/config/set'));
        await profile.locator('button[type=submit]').click();
        assert.equal((await configured).status(), 200);
        await profile.locator('.inline-warn').waitFor();
        assert.doesNotMatch(await profile.locator('.inline-warn').textContent(), /Unverified harness/);
        await page.screenshot({path: path.join(captures, 'implementation-global-' + theme + '.png')});
        if (theme === 'dark') {
          for (const sec of ['agents', 'compaction', 'adapters', 'profiles']) {
            await pickSection(sec);
            const shot = sec === 'agents' ? 'agents-tiers-dark.png' : sec + '-dark.png';
            await page.screenshot({path: path.join(captures, shot)});
          }
          await pickSection('prompts');
          const prompt = row('prompt.UserPreferenceSection');
          await prompt.locator('[data-settings-bucket=claude]').click();
          await prompt.locator('[data-settings-prompt-editor]').fill('## Claude override\nPrefer concise Korean summaries.');
          const promptSet = page.waitForResponse(r => r.url().endsWith('/api/config/set'));
          await prompt.locator('button[type=submit]').click();
          assert.equal((await promptSet).status(), 200);
          assert.equal(await prompt.locator('[data-settings-bucket=claude]').getAttribute('data-bucket-override'), 'own');
          assert.equal(await frame().locator('[data-settings-prompt-point="prompt.UserPreferenceSection"]').getAttribute('data-bucket-override'), 'own');
          await page.screenshot({path: path.join(captures, 'prompts-dark.png')});
          const promptReset = page.waitForResponse(r => r.url().endsWith('/api/config/reset'));
          await prompt.locator('.reset-btn').click();
          assert.equal((await promptReset).status(), 200);
          await pickSection('workflow');
        }
        await reference.goto(pathToFileURL(mockup).href + '?theme=' + theme);
        await reference.locator('#stage .sync-card').waitFor();
        await reference.screenshot({path: path.join(captures, 'reference-global-' + theme + '.png')});

        await gear('repo', root).click();
        await show('repo', root, root);
        await row('workflow.whiteboard_enabled').locator('[data-settings-bucket=claude]').click();
        await row('workflow.profile').locator('[data-settings-bucket=claude]').click();
        await profile.locator('[data-settings-option=direct]').click();
        await profile.locator('button[type=submit]').click();
        await setLanguage('English', null, false);
        await proposal('toggle').click();
        await frame().locator('.proposal-patch').waitFor();
        await page.screenshot({path: path.join(captures, 'implementation-repo-' + theme + '.png')});
        await proposal('clear').click();
        await reference.locator('.rail-entry[data-scope-entry=repo]').click();
        const input = reference.locator('input[data-act=input][data-knob="workflow.lang"]');
        await input.fill('English');
        await input.press('Tab');
        await reference.locator('.proposal-bar').waitFor();
        await reference.screenshot({path: path.join(captures, 'reference-repo-' + theme + '.png')});
      }
      await reference.close();
    }
    await capturePairs();
    const direct = await page.goto(base + '/settings');
    assert.equal(direct.status(), 200);
    await show('global');
    assert.match(await scopeButton('session').textContent(), /미평가/);
    assert(await scopeButton('synced').isDisabled());

    const auth = await page.request.post(base + '/api/config/set', { headers: { Origin: 'http://elsewhere.test' },
      data: { scope:'global', key:'workflow.lang', value:'blocked' } });
    assert.equal(auth.status(), 403);
    const badContext = await page.request.get(base + snapshot + '?scope=builtin&worktree=stale');
    assert.equal(badContext.status(), 400);
    assert.equal((await badContext.json()).error.code, 'invalid_context');

    await setLanguage('English', base + snapshot);
    const reset = page.waitForResponse(r => r.url().endsWith('/api/config/reset') && r.request().method() === 'POST');
    await row('workflow.lang').locator('.reset-btn').click();
    assert.equal((await reset).status(), 200);
    await setLanguage('Korean', base + snapshot);

    const localSnapshot = await (await page.request.get(base + snapshot)).json();
    const namespaces = localSnapshot.entries.map(e => e.key.split('.')[0]);
    for (const adapter of ['claude', 'pi', 'agy', 'dashboard']) assert(namespaces.includes(adapter));
    await page.locator('#gear').click();
    for (const selector of ['[data-layout=split]', '[data-input=mod]', '[data-theme-set=dark]', '#glang', '#login-copy', 'a[href="/settings"]']) {
      assert(await page.locator('#gmenu ' + selector).isVisible(), selector);
    }
    await page.locator('#settings-close').click();

    await gear('project', workspace).focus();
    await page.keyboard.press('Tab');
    await page.keyboard.press('Shift+Tab');
    assert(await gear('project', workspace).evaluate(el => el.matches(':focus-visible')));
    await gear('project', workspace).press('Enter');
    await show('project', workspace);
    assert.equal(await frame().locator('[data-settings-target-display] code').textContent(), projectPath);
    await scopeButton('builtin').click();
    await show('builtin');
    assert.equal(new URLSearchParams(new URL(page.url()).hash.slice(1)).get('project'), workspace);
    await page.reload();
    await show('builtin');
    await scopeButton('global').click();
    await show('global');
    await scopeButton('project').click();
    await show('project', workspace);
    assert(await row('workflow.prefer_subagent').locator('button.tg').isDisabled());
    await gear('session', session).click();
    await show('session', session, root, session);
    await scopeButton('builtin').click();
    await show('builtin', null, root, session);
    assert(await row('workflow.lang').locator('input').isDisabled());
    await page.goBack();
    await show('session', session, root, session);

    await gear('repo', root).click();
    await show('repo', root, root);
    await scopeButton('builtin').click();
    await show('builtin', null, root);
    assert.match(await scopeButton('session').textContent(), /미평가/);
    await scopeButton('repo').click();
    await show('repo', root, root);

    await pickSection('profiles');
    const profile = row('agent.profile.reviewer-readonly');
    await profile.locator('[data-settings-profile-json] summary').click();
    const raw = await profile.locator('.prof-json').textContent();
    assert.match(raw, /\[redacted\]/);
    assert.match(raw, /"command": "node"/);
    assert.doesNotMatch(raw, secrets);
    const scoped = await page.request.get(base + snapshot + '?scope=repo&target=' + root + '&worktree=' + root);
    assert.doesNotMatch(await scoped.text(), secrets);
    const malformed = await page.request.post(base + '/api/config/set', {
      headers: { Origin: base, 'Content-Type':'application/json' },
      data: '{"scope":"global","key":"agent.profile.test","value": {"mcp_servers":[ "secret-123","https://example.com" ]'
    });
    assert.equal(malformed.status(), 400);
    assert.doesNotMatch(await malformed.text(), secrets);
    await pickSection('prompts');
    const editor = row('prompt.UserPreferenceSection').locator('[data-settings-prompt-editor]');
    await editor.fill('## Standing preferences\n<reviewed>');
    assert(await editor.evaluate(el => el === document.activeElement), 'line diff preserves focus');
    const diff = row('prompt.UserPreferenceSection').locator('[data-settings-prompt-diff]');
    assert.match(await diff.textContent(), /[+] <reviewed>/);
    assert.equal(await diff.locator('reviewed').count(), 0);

    await pickSection('workflow');
    await setLanguage('First draft', null, false);
    await setLanguage('Second draft', null, false);
    await proposal('toggle').click();
    assert.match(await frame().locator('.proposal-patch').textContent(), /Second draft/);
    assert.doesNotMatch(await frame().locator('.proposal-patch').textContent(), /First draft/);
    await scopeButton('builtin').click();
    await page.locator('dialog[open], [role=alertdialog]').first().waitFor();
    await page.keyboard.press('Escape');
    await show('repo', root, root);

    const sent = page.waitForResponse(r => r.url().endsWith('/api/config/proposal'));
    await proposal('send').click();
    assert.equal((await sent).status(), 200);
    await frame().locator('[data-settings-proposal-status=delivered]').waitFor();

    await gear('repo', linked).click();
    await show('repo', linked, linked);
    await setLanguage('Manual draft', null, false);
    const noLead = page.waitForResponse(r => r.url().endsWith('/api/config/proposal'));
    await proposal('send').click();
    assert.equal((await noLead).status(), 409);
    await frame().locator('[data-settings-proposal-status=no_active_lead]').waitFor();
    const current = await (await page.locator('iframe.sframe.on').elementHandle()).contentFrame();
    await current.evaluate(() => Object.defineProperty(navigator, 'clipboard', {
      configurable: true, value: {writeText: () => Promise.reject(Error('denied for test'))}
    }));
    await proposal('copy').click();
    const manual = frame().locator('[data-settings-proposal-manual]');
    assert.match(await manual.inputValue(), /Manual draft/);
    assert.doesNotMatch(await manual.inputValue(), secrets);
    await proposal('clear').click();
    await proposal('copy').waitFor({ state:'hidden' });

    assert.deepEqual(errors, []);
    console.log('Verified settings scopes, navigation, authorization, mutations, profiles, prompt diff, proposals and 8 visual captures.');
  } finally {
    await browser.close();
  }
})().catch(error => {
  console.error(error);
  process.exitCode = 1;
});
