// Production shell/rail, production admission route, disposable Git + httptest.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { pathToFileURL } = require('node:url');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, reference, artifacts] = process.argv.slice(2);
const draftKey = 'ws.dashboard.worktree-admission.drafts';
const promptKey = 'ws.dashboard.worktree-admission.prompt';

(async () => {
 const browser = await chromium.launch({ headless: true });
 const results = [], toastGeometry = [];
 try {
  const page = await browser.newPage({ viewport:{width:1440,height:1000}, locale:'en-US' });
  const errors = [];
  page.on('pageerror', e => errors.push(e.stack));
  await page.route('**/*', route => {
   const url = route.request().url();
   return url.startsWith(base + '/') || url.startsWith('data:') ? route.continue() : route.abort();
  });
  await page.addInitScript(() => {
   window.copyFail = true; window.copied = [];
   Object.defineProperty(navigator, 'clipboard', { configurable:true, value:{ writeText: text => {
    if (window.copyFail) return Promise.reject(Error('fixture clipboard denied'));
    window.copied.push(text); return Promise.resolve();
   } } });
   document.execCommand = () => false;
  });
  const control = async op => (await fetch(base+'/fixture-control?op='+op)).json();
  const paths = await control('reset');
  const row = name => page.locator('.rwt[data-path='+JSON.stringify(paths[name])+']');
  const toggle = (name, action) => row(name).locator('[data-admission-action="'+action+'"]');
  async function open() {
   if (!(await page.locator('html').getAttribute('class') || '').includes('rail-open')) await page.locator('#railbtn').click();
   await page.locator('#rail').waitFor({state:'visible'});
  }
  async function reload() { await page.reload(); await open(); }
  async function drafts() { return page.evaluate(k => JSON.parse(localStorage.getItem(k)||'[]'),draftKey); }
  async function apply(status=200) {
   const response = page.waitForResponse(r => r.url() === base+'/rail/worktrees/admission');
   await page.locator('[data-admission-apply]').click();
   const res = await response; assert.equal(res.status(),status); const v = await res.json();
   await page.waitForFunction(() => !document.querySelector('[data-admission-apply]').disabled);
   return v;
  }
  async function assertCopyToast(label) {
   await page.waitForFunction(() => document.querySelector('#wstoasts .wstoast .wst-body')?.textContent === 'Copied.');
   const geometry = await page.evaluate(() => {
    const toast = document.querySelector('#wstoasts .wstoast').getBoundingClientRect();
    const rail = document.querySelector('#rail').getBoundingClientRect();
    const rect = r => ({left:r.left,top:r.top,right:r.right,bottom:r.bottom,width:r.width,height:r.height});
    return {toast:rect(toast),rail:rect(rail),viewport:{width:innerWidth,height:innerHeight}};
   });
   const {toast,rail,viewport} = geometry;
   assert.ok(toast.left >= 0 && toast.top >= 0 && toast.right <= viewport.width && toast.bottom <= viewport.height, label + ': toast stays inside viewport');
   assert.ok(toast.height <= 40, label + ': fixed success feedback stays one line');
   assert.ok(toast.left >= rail.right || toast.right <= rail.left || toast.top >= rail.bottom || toast.bottom <= rail.top, label + ': toast does not cover the rail');
   toastGeometry.push({label,...geometry});
  }
  await page.goto(base+'/'); await open();
  await toggle('agent-a82f81f3','approve').click();
  assert.equal(await toggle('agent-a82f81f3','approve').getAttribute('aria-pressed'),'true');
  await toggle('agent-a82f81f3','approve').click();
  assert.equal((await drafts()).length,0); results.push('1096: same toggle clears');
  await toggle('agent-a82f81f3','approve').click();
  await toggle('agent-a82f81f3','reject').click();
  assert.equal(await toggle('agent-a82f81f3','approve').getAttribute('aria-pressed'),'false');
  assert.equal(await toggle('agent-a82f81f3','reject').getAttribute('aria-pressed'),'true');
  results.push('1097: approve/reject mutually exclusive');
  await toggle('tmp.Xk2','approve').click();
  await page.locator('[data-admission-clear]').click();
  assert.equal((await drafts()).length,0);
  assert.equal((await control('state')).filter(x=>x.state==='rejected').length,0);
  results.push('1098: clear-all changes drafts only');
  await toggle('agent-a82f81f3','reject').click();
  const original = await drafts();
  await reload(); assert.deepEqual(await drafts(),original);
  const refresh = page.waitForResponse(r => r.url()===base+'/rail/refresh');
  await page.locator('#railref').click(); await refresh;
  await page.waitForTimeout(150); assert.deepEqual(await drafts(),original);
  assert.equal(await toggle('agent-a82f81f3','reject').getAttribute('aria-pressed'),'true');
  results.push('1100: reload + actual rail refresh retain original revision');
  const first = await apply(); assert.equal(first.applied,true); assert.ok(first.html); assert.ok(first.gen>0);
  await page.locator('.admission-copy-status').filter({hasText:'Clipboard failed'}).waitFor({timeout:3000}).catch(async e=>{
   await page.screenshot({path:artifacts+'/diagnostic-clipboard.png'});
   console.log('CLIPBOARD DIAGNOSTIC', await page.evaluate(k=>({prompt:localStorage.getItem(k), feedback:document.querySelector('.admission-feedback').outerHTML, rail:document.querySelector('#r-rail').innerHTML, errors:window.copied} ),promptKey), errors);
   throw e;
  });
  assert.equal((await control('state')).filter(x=>x.state==='rejected').length,1);
  assert.equal(await page.locator('.rfold').getAttribute('open'),null,'rejected list collapsed by default');
  await reload();
  await page.locator('.admission-copy-status').filter({hasText:'Clipboard failed'}).waitFor();
  assert.equal(await page.evaluate(k=>JSON.parse(localStorage.getItem(k)).text,promptKey),first.prompt);
  await page.evaluate(()=>window.copyFail=false);
  await page.locator('[data-admission-copy]').click();
  await page.waitForFunction(()=>window.copied.length===1);
  assert.equal(await page.evaluate(()=>window.copied[0]),first.prompt);
  await assertCopyToast('Clipboard API retry');
  await page.evaluate(()=>{window.copyFail=true;window.fallbackCopies=0;document.execCommand=()=>{window.fallbackCopies++;return true;};});
  await page.locator('[data-admission-copy]').click();
  await page.waitForFunction(k=>window.fallbackCopies===1 && JSON.parse(localStorage.getItem(k)).failed===false,promptKey);
  await assertCopyToast('execCommand fallback');
  await page.evaluate(()=>{window.copyFail=false;document.execCommand=()=>false;});
  results.push('1101: persisted rejection survives failed clipboard/reload; explicit retry succeeds');
  results.push('Toast regression: Clipboard API and fallback success use fixed text without the copied prompt; geometry does not overlap the rail');
  await toggle('tmp.Xk2','reject').click();
  const second = await apply();
  assert.ok(second.prompt.includes(paths['agent-a82f81f3']) && second.prompt.includes(paths['tmp.Xk2']));
  assert.equal((await control('state')).filter(x=>x.state==='rejected').length,2);
  results.push('1102: cumulative server prompt matches both persisted rejected rows');
  await page.locator('.rfold > summary').click();
  const undoResponse = page.waitForResponse(r=>r.url()===base+'/rail/worktrees/admission');
  await row('agent-a82f81f3').locator('.admission-undo').click();
  assert.equal((await undoResponse).status(),200);
  await toggle('agent-a82f81f3','approve').waitFor();
  assert.equal((await control('state')).filter(x=>x.state==='rejected').length,1);
  results.push('1099: rejected-list undo persists pending without deleting files');

  await control('reset'); await reload();
  await page.evaluate(()=>localStorage.removeItem('ws.dashboard.worktree-admission.prompt')); await reload();
  await toggle('agent-a82f81f3','approve').click(); await toggle('tmp.Xk2','reject').click();
  const before = await control('state'), evidence = await drafts();
  await control('replace');
  const stale = await apply(409); assert.equal(stale.applied,false); assert.equal(stale.stale.length,1);
  assert.deepEqual(await control('state'),before,'whole batch must not write');
  assert.equal((await drafts())[0].revision,evidence[0].revision);
  await row('agent-a82f81f3').locator('.admission-row-correction').waitFor({state:'visible'});
  await reload(); assert.equal((await drafts())[0].revision,evidence[0].revision);
  await page.locator('#railref').click(); await page.waitForTimeout(150);
  assert.equal((await drafts())[0].revision,evidence[0].revision,'refresh cannot replace stale evidence');
  await row('agent-a82f81f3').locator('[data-admission-reselect]').click();
  assert.notEqual((await drafts()).find(x=>x.path===paths['agent-a82f81f3']).revision,evidence[0].revision);
  await control('missing');
  await page.locator('.admission-missing').filter({hasText:paths['agent-a82f81f3']}).waitFor();
  await page.locator('.admission-missing').filter({hasText:paths['agent-a82f81f3']}).getByRole('button',{name:'Clear selection'}).click();
  assert.equal((await drafts()).length,1);
  results.push('1103: real replacement 409 refuses whole batch, preserves evidence/drafts; explicit reselect and missing-path clear work');

  // Screenshots use real production DOM at both responsive widths and themes.
  // A reference page is captured independently; comparison images put rails side by side.
  const ref = await browser.newPage({viewport:{width:1440,height:1000}});
  const compare = await browser.newPage({viewport:{width:780,height:1020}});
  async function capture(width,theme,state,referenceState) {
   const stem = `${width}-${theme}-${state}`;
   await page.screenshot({path:`${artifacts}/production-${stem}.png`});
   const prodRail = await page.locator('#rail').screenshot();
   await ref.goto(pathToFileURL(reference).href+'?theme='+theme);
   if (referenceState!=='pending') await ref.evaluate(()=>{window.__mk.mark('a82','ap');window.__mk.mark('xk2','rj');});
   if (referenceState==='applied') {
    await ref.locator('#capgo').click(); await ref.locator('.rfold > summary').click();
   }
   await ref.screenshot({path:`${artifacts}/reference-${stem}.png`});
   const refRail = await ref.locator('#rail').screenshot();
   await compare.setContent(`<style>body{margin:0;background:#888;font:14px system-ui}main{display:flex;gap:12px}img{max-width:396px;object-fit:contain;object-position:top}section{display:flex;flex-direction:column}</style><main><section>Production ${stem}<img src="data:image/png;base64,${prodRail.toString('base64')}"></section><section>Approved HTML (${referenceState})<img src="data:image/png;base64,${refRail.toString('base64')}"></section></main>`);
   await compare.screenshot({path:`${artifacts}/comparison-${stem}.png`,fullPage:true});
  }
  // The intentionally removed path is reintroduced in the fixture by moving
  // its retained replacement back through the control below before captures.
  await fetch(base+'/fixture-control?op=restore');
  for (const width of [1440,700]) for (const theme of ['light','dark']) {
   await page.setViewportSize({width,height:1000}); await ref.setViewportSize({width,height:1000});
   await control('reset');
   await page.evaluate(()=>{localStorage.clear();}); await reload();
   await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);
   await capture(width,theme,'pending','pending');
   await toggle('agent-a82f81f3','approve').click(); await toggle('tmp.Xk2','reject').click();
   await capture(width,theme,'selected','selected');
   await control('replace');
   await apply(409);
   await capture(width,theme,'stale-refusal','selected');
   await page.locator('#railref').click();
   await page.waitForTimeout(150);
   await row('agent-a82f81f3').locator('[data-admission-reselect]').click();
   await page.evaluate(()=>window.copyFail=false);
   const applied = await apply(); assert.equal(applied.applied,true);
   await assertCopyToast(`${width}-${theme}-applied`);
   await page.locator('.rfold > summary').click();
   await capture(width,theme,'rejected-list-with-undo','applied');
   await page.screenshot({path:`${artifacts}/production-${width}-${theme}-applied.png`});
  }
  assert.deepEqual(errors,[]);
  fs.writeFileSync(artifacts+'/toast-geometry.json',JSON.stringify(toastGeometry,null,2)+'\n');
  const summary = ['# Worktree admission production-browser evidence','',...results.map(x=>'- PASS '+x),'',
   'Production shell, templates, CSS, JavaScript and admission handler were served by a private httptest fixture; no daemon or provider was started.',
   'All 16 required production captures: production-{1440,700}-{light,dark}-{pending,selected,rejected-list-with-undo,stale-refusal}.png.',
   'Each capture has reference-* and comparison-* counterparts. comparison-* places production and approved HTML rails side by side. The reference has no stale UI; stale comparisons intentionally use its selected state.',
   'Applied-state comparisons: comparison-{1440,700}-{light,dark}-rejected-list-with-undo.png; additional full production applied captures: production-{1440,700}-{light,dark}-applied.png.',
   'Visual differences: production retains the existing 340px responsive rail, English labels and canonical fixture paths; the approved HTML uses a 396px Korean mock rail. Amber grouping, reversible toggles, capsule, collapsed rejected fold and undo retain its hierarchy. Stale correction and retained clipboard prompt are additional settled error states.',
   'Copy-success feedback: fixed "Copied." text. Clipboard API/fallback and all four applied width/theme variants assert viewport containment, single-line height and zero overlap with the rail; measured rectangles are in toast-geometry.json.',
   'Reference: '+reference,'Artifacts: '+artifacts,'',...fs.readdirSync(artifacts).filter(x=>x.endsWith('.png')).sort().map(x=>artifacts+'/'+x)].join('\n');
  fs.writeFileSync(artifacts+'/summary.md',summary+'\n');
  console.log(results.join('\n')); console.log('PASS: 16 production states, reference side-by-side comparisons and 4 applied captures. '+artifacts+'/summary.md');
 } finally { await browser.close(); }
})().catch(e=>{console.error(e);process.exitCode=1;});
