package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// A held slash command cancelled from the conversation comes back into the
// composer as a fresh pick leaves it: the inline skill capsule at offset zero
// of the message box, then its arguments, and no separate selection capsule.
// WS_HELDCMD_SHOT, when set to a file path, also receives a dark 1440 shot of
// the composer after the first cancel.
func TestHeldCommandCancelRestoresInlineCapsuleBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for disposable Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	f := startPageFixture(t)
	f.withCommands(t, wslive.SlashCommand{Name: "ws:lead-audit-doc", Source: "plugin"})
	bare, _ := f.send(t, `{"command":"ws:lead-audit-doc"}`)
	withArgs, _ := f.send(t, `{"command":"ws:lead-audit-doc","args":"AGENTS.md tighten"}`)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	script := `
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: 'dark', locale: 'en-US' });
    const tab = await context.newPage();
    const errors = [];
    tab.on('pageerror', e => errors.push(e.message));
    await tab.goto(process.env.HC_BASE + '/s/livesession');
    const frame = await (await tab.locator('iframe.sframe.on').elementHandle()).contentFrame();
    const state = () => frame.evaluate(() => {
      const field = document.getElementById('ctext'), editor = field.wsc && field.wsc.editor, ccmd = document.getElementById('ccmd');
      return {
        value: field.value,
        ranges: field.wsc ? field.wsc.snapshot() : null,
        capsules: editor ? [...editor.querySelectorAll('.wsc-capsule.wsc-skill')].map(c => c.textContent) : null,
        code: editor ? editor.querySelectorAll('code').length : -1,
        editorText: editor ? editor.textContent : null,
        ccmdShown: !!ccmd && !ccmd.hidden && getComputedStyle(ccmd).display !== 'none'
      };
    });
    const cancelHeld = async id => {
      const btn = frame.locator('#r-convlive [data-cancel="' + id + '"]');
      await btn.waitFor();
      await btn.click();
      await frame.waitForFunction(() => document.getElementById('ctext').value !== '' || !document.getElementById('ccmd').hidden);
    };
    await cancelHeld(process.env.HC_BARE);
    let s = await state();
    if (process.env.HC_SHOT) await frame.locator('.composer').screenshot({ path: process.env.HC_SHOT });
    assert.equal(s.ccmdShown, false, 'no separate selection capsule after cancel');
    assert.deepEqual(s.capsules, ['/ws:lead-audit-doc'], 'one inline skill capsule, as a fresh pick');
    assert.deepEqual(s.ranges, [{ start: 0, end: 17, kind: 'skill', id: 'ws:lead-audit-doc', label: 'ws:lead-audit-doc' }]);
    assert.equal(s.value, 'ws:lead-audit-doc ', 'a bare command leaves only the capsule and its space');
    assert.equal(s.code, 0, 'no typed-line code element inside the box');
    console.log('bare command: inline capsule restored');
    await frame.evaluate(() => { document.getElementById('ctext').value = ''; });
    await frame.waitForFunction(() => document.getElementById('ccmd').hidden);
    await cancelHeld(process.env.HC_ARGS);
    s = await state();
    assert.equal(s.ccmdShown, false, 'no separate selection capsule after cancel with args');
    assert.deepEqual(s.capsules, ['/ws:lead-audit-doc']);
    assert.equal(s.value, 'ws:lead-audit-doc AGENTS.md tighten', 'the box holds the capsule then the args');
    assert.equal(s.editorText, '/ws:lead-audit-doc AGENTS.md tighten');
    assert.equal(s.code, 0);
    console.log('command with args: inline capsule and args restored');
    assert.deepEqual(errors, []);
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exit(1); });
`
	cmd := exec.CommandContext(ctx, node, "-e", script)
	cmd.Env = append(os.Environ(), "HC_BASE="+f.base, "HC_BARE="+bare, "HC_ARGS="+withArgs, "HC_SHOT="+os.Getenv("WS_HELDCMD_SHOT"))
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("held command cancel restore: %v", err)
	}
}
