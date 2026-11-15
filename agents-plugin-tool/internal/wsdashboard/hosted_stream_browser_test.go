package wsdashboard

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// A hosted lead's reply streams: each published reply-so-far shows in the
// conversation as the streaming bubble, and the finished reply replaces it,
// leaving one bubble. WS_STREAM_SHOT_DIR, when set, receives dark 1440 shots
// mid-stream (stream-mid.png) and after the final reply (stream-final.png).
func TestHostedReplyStreamsThenFinishesAsOneBubbleBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for disposable Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	f := startPageFixture(t)
	slot, ok := f.d.OpenHostedSlot(SessionDir(f.dir, "livesession"))
	if !ok {
		t.Fatal("no hosted slot for the fixture session")
	}
	const final = "Rivers carve their paths slowly through stone and soil. They carry water and sediment to the sea. Life gathers along their banks."
	started := time.Now().UnixMilli()
	publish := func(reply string) {
		snap := wslive.Snapshot{SchemaVersion: 1, Adapter: "claude", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 30000}
		if reply != "" {
			snap.Lead = wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}, TurnStartedAt: started, Reply: &wslive.Reply{Text: reply, StartedAt: started}}
		}
		slot.Publish(snap)
	}
	publish(final[:24])

	shots := os.Getenv("WS_STREAM_SHOT_DIR")
	script := `
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const readline = require('node:readline');
const lines = readline.createInterface({ input: process.stdin });
const next = () => new Promise(r => lines.once('line', r));
(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: 'dark', locale: 'en-US' });
    const tab = await context.newPage();
    const errors = [];
    tab.on('pageerror', e => errors.push(e.message));
    await tab.goto(process.env.HC_BASE + '/s/livesession');
    const frame = await (await tab.locator('iframe.sframe.on').elementHandle()).contentFrame();
    const final = process.env.HC_FINAL;
    const state = () => frame.evaluate(() => ({
      streaming: [...document.querySelectorAll('.msg.fromlead.streaming .bub')].map(b => b.textContent.trim()),
      leads: [...document.querySelectorAll('.msg.fromlead')].filter(m => m.textContent.includes('Rivers carve')).length,
      finished: [...document.querySelectorAll('.msg.fromlead[data-entry] .bub')].map(b => b.textContent.trim()),
    }));
    const until = async (what, ok) => {
      for (let i = 0; i < 100; i++) { const s = await state(); if (ok(s)) return s; await tab.waitForTimeout(100); }
      throw new Error(what + ': ' + JSON.stringify(await state()));
    };
    let s = await until('first partial', s => s.streaming.some(t => t === final.slice(0, 24)));
    assert.equal(s.leads, 1);
    console.log('P1');
    await next();
    s = await until('second partial', s => s.streaming.some(t => t === final.slice(0, 70)));
    assert.equal(s.leads, 1, 'one streaming bubble, not one per partial');
    if (process.env.HC_SHOTS) await tab.screenshot({ path: process.env.HC_SHOTS + '/stream-mid.png' });
    console.log('P2');
    await next();
    s = await until('final reply', s => s.streaming.length === 0 && s.finished.some(t => t === final));
    assert.equal(s.leads, 1, 'the final reply replaces the streaming one');
    if (process.env.HC_SHOTS) await tab.screenshot({ path: process.env.HC_SHOTS + '/stream-final.png' });
    assert.deepEqual(errors, []);
    console.log('DONE');
  } finally { await browser.close(); lines.close(); }
})().catch(e => { console.error(e); process.exit(1); });
`
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "-e", script)
	cmd.Env = append(os.Environ(), "HC_BASE="+f.base, "HC_FINAL="+final, "HC_SHOTS="+shots)
	cmd.WaitDelay = 5 * time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(stdout)
	var seen []string
	for sc.Scan() {
		line := sc.Text()
		seen = append(seen, line)
		switch line {
		case "P1":
			publish(final[:70])
			stdin.Write([]byte("go\n"))
		case "P2":
			publish("")
			if err := wslive.AppendRing(wslive.RingDir(SessionDir(f.dir, "livesession")), []wslive.RingEntry{{Kind: wslive.RingReply, Text: final}}); err != nil {
				t.Fatal(err)
			}
			slot.RingAppended()
			stdin.Write([]byte("go\n"))
		}
	}
	err = cmd.Wait()
	if err != nil || len(seen) == 0 || seen[len(seen)-1] != "DONE" {
		t.Fatalf("hosted stream browser check: %v\nstdout %q\n%s", err, seen, stderr.String())
	}
	if shots != "" {
		for _, name := range []string{"stream-mid.png", "stream-final.png"} {
			if _, err := os.Stat(filepath.Join(shots, name)); err != nil {
				t.Errorf("screenshot %s: %v", name, err)
			}
		}
	}
}
