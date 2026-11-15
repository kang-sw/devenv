package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// A held slash command row carries the steer control a held text row has;
// clicking it posts the steer, the command row leaves the conversation, and
// a steered text row holding the typed line takes its place, as the daemon
// now delivers it (wsinbox.Steer). WS_HELDSTEER_SHOTS, when set to a
// directory, also receives dark 1440 shots of the held rows before and after
// the click.
func TestHeldCommandSteerSendsTypedLineBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for disposable Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	f := startPageFixture(t)
	f.withCommands(t, wslive.SlashCommand{Name: "ws:lead-audit-doc", Source: "plugin"})
	cmdID, _ := f.send(t, `{"command":"ws:lead-audit-doc","args":"AGENTS.md tighten"}`)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	script := `
const assert = require('node:assert/strict');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: 'dark', locale: 'en-US' });
    const tab = await context.newPage();
    const errors = [];
    tab.on('pageerror', e => errors.push(e.message));
    const posts = [];
    tab.on('request', r => { if (r.method() === 'POST' && /\/steer$/.test(r.url())) posts.push(r.postData()); });
    await tab.goto(process.env.HS_BASE + '/s/livesession');
    const frame = await (await tab.locator('iframe.sframe.on').elementHandle()).contentFrame();
    const id = process.env.HS_CMD;
    const row = frame.locator('#r-convlive .msg.user.held[data-entry="' + id + '"]');
    await row.waitFor();
    assert.equal(await row.evaluate(n => n.classList.contains('hcmd')), true, 'the held row is a command row');
    const btn = row.locator('[data-steer="' + id + '"]');
    assert.equal(await btn.count(), 1, 'the held command row has the steer control');
    const shots = process.env.HS_SHOTS;
    const shoot = async name => {
      if (!shots) return;
      await frame.locator('#r-convlive .msg.user.held').last().scrollIntoViewIfNeeded();
      await tab.screenshot({ path: path.join(shots, name) });
      await frame.locator('#r-convlive .msg.user.held').last().screenshot({ path: path.join(shots, 'row-' + name) });
    };
    await shoot('before.png');
    await btn.click();
    await frame.waitForFunction(i => !document.querySelector('#r-convlive [data-entry="' + i + '"]'), id, { timeout: 15000 });
    assert.deepEqual(posts.map(p => JSON.parse(p).id), [id], 'one steer post for the command entry');
    const steered = frame.locator('#r-convlive .msg.user.held:has(.steertag)');
    await steered.waitFor({ timeout: 15000 });
    const s = await steered.evaluate(n => ({ cmd: n.classList.contains('hcmd'), text: n.querySelector('.bub').textContent.trim(), steerBtn: !!n.querySelector('[data-steer]'), cancelBtn: !!n.querySelector('[data-cancel]') }));
    assert.equal(s.cmd, false, 'the steered row is a text row');
    assert.equal(s.text, '/ws:lead-audit-doc AGENTS.md tighten', 'the steered row holds the typed line');
    assert.equal(s.steerBtn, false, 'a steered row offers no second steer');
    assert.equal(s.cancelBtn, true);
    await shoot('after.png');
    console.log('held command steered as its typed line');
    assert.deepEqual(errors, []);
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exit(1); });
`
	cmd := exec.CommandContext(ctx, node, "-e", script)
	cmd.Env = append(os.Environ(), "HS_BASE="+f.base, "HS_CMD="+cmdID, "HS_SHOTS="+os.Getenv("WS_HELDSTEER_SHOTS"))
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("held command steer: %v", err)
	}
	held := f.held(t)
	if len(held) != 1 || !held[0].Entry.Steer || held[0].Entry.Command != nil || inboxBody(t, held[0].Entry) != "/ws:lead-audit-doc AGENTS.md tighten" {
		t.Fatalf("held after the browser steer = %+v", held)
	}
}
