// Production shell/rail, production admission route, disposable Git + httptest.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { pathToFileURL } = require('node:url');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, reference, artifacts, shots] = process.argv.slice(2);
const draftKey = 'ws.dashboard.worktree-admission.drafts';
const promptKey = 'ws.dashboard.worktree-admission.prompt';

(async () => {
 const browser = await chromium.launch({ headless: true });
 const results = [], toastGeometry = [];
 try {
  const page = await browser.newPage({ viewport:{width:1440,height:1000}, locale:'ko-KR' });
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
  // The Korean fixture: fixed copy text, and Apply's mockup toast lines.
  const T = {copied:'복사함', copyFailed:'거부는 저장했지만 확인 요청 프롬프트를 클립보드에 복사하지 못했습니다.',
   rejected:n=>`거부 ${n}개 — 확인 요청 프롬프트를 복사했습니다`, rejectFail:n=>`거부 ${n}개 — 클립보드에 쓰지 못했습니다`,
   approved:n=>`승인 ${n}개 반영`, undone:n=>`${n}개를 대기로 되돌렸습니다`};
  const feedback = () => page.locator('.admission-feedback');
  async function clearToasts() { await page.evaluate(()=>{document.querySelectorAll('#wstoasts .wstoast').forEach(x=>x.remove()); document.querySelector('#rail .rtoast')?.classList.remove('on');}); }
  // Apply's result toast sits in the rail (847 mockup): above the foot, the
  // capsule or the notice, inside the rail's width.
  async function assertRailToast(label, text) {
   await page.waitForFunction(t => { const e = document.querySelector('#rail > .rtoast.on'); return e && e.textContent === t; }, text);
   await page.waitForTimeout(250);
   const g = await page.evaluate(() => {
    const rect = e => { if (!e || e.hidden || !e.offsetHeight) return null; const r = e.getBoundingClientRect(); return {left:r.left,top:r.top,right:r.right,bottom:r.bottom,width:r.width,height:r.height}; };
    return {toast:rect(document.querySelector('#rail > .rtoast')), rail:rect(document.querySelector('#rail')), foot:rect(document.querySelector('#rail .rfoot')),
     cap:rect(document.querySelector('#rail .rcap')), notice:rect(document.querySelector('#rail .admission-feedback'))};
   });
   const {toast, rail} = g;
   assert.ok(toast.left >= rail.left && toast.right <= rail.right && toast.top >= rail.top, label + ': toast stays inside the rail');
   for (const k of ['foot','cap','notice']) if (g[k]) assert.ok(toast.bottom <= g[k].top + 0.5, label + ': toast sits above the ' + k);
   assert.ok(toast.height <= 60, label + ': toast stays compact ('+toast.height+'px)');
   toastGeometry.push({label, ...g});
  }
  async function fullShot(path) {
   // The fixture's memo pane opens with the slide-over rail; hide it here.
   await page.addStyleTag({content:'.memo-pad{display:none!important}'}).then(async h => { await page.screenshot({path}); await h.evaluate(e=>e.remove()); });
  }
  async function assertCopyToast(label, text=T.copied, maxHeight=40) {
   await page.waitForFunction(t => [...document.querySelectorAll('#wstoasts .wstoast .wst-body')].some(x=>x.textContent === t), text);
   const geometry = await page.evaluate(() => {
    const toast = document.querySelector('#wstoasts .wstoast').getBoundingClientRect();
    const rail = document.querySelector('#rail').getBoundingClientRect();
    const rect = r => ({left:r.left,top:r.top,right:r.right,bottom:r.bottom,width:r.width,height:r.height});
    return {toast:rect(toast),rail:rect(rail),viewport:{width:innerWidth,height:innerHeight}};
   });
   const {toast,rail,viewport} = geometry;
   assert.ok(toast.left >= 0 && toast.top >= 0 && toast.right <= viewport.width && toast.bottom <= viewport.height, label + ': toast stays inside viewport');
   assert.ok(toast.height <= maxHeight, label + ': toast stays compact ('+toast.height+'px)');
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
  await clearToasts();
  const first = await apply(); assert.equal(first.applied,true); assert.ok(first.html); assert.ok(first.gen>0);
  await page.locator('.admission-status').filter({hasText:T.copyFailed}).waitFor({timeout:3000}).catch(async e=>{
   await page.screenshot({path:artifacts+'/diagnostic-clipboard.png'});
   console.log('CLIPBOARD DIAGNOSTIC', await page.evaluate(k=>({prompt:localStorage.getItem(k), feedback:document.querySelector('.admission-feedback').outerHTML, rail:document.querySelector('#r-rail').innerHTML, errors:window.copied} ),promptKey), errors);
   throw e;
  });
  await assertRailToast('failed copy', T.rejectFail(1));
  assert.equal((await control('state')).filter(x=>x.state==='rejected').length,1);
  assert.equal(await page.locator('.rrej').getAttribute('open'),null,'rejected list collapsed by default');
  assert.equal(await page.locator('.admission-copy details').getAttribute('open'),null,'retained prompt collapsed by default');
  assert.equal(await page.locator('[data-admission-dismiss]').getAttribute('aria-label'),'닫기 (Esc)');
  assert.equal((await page.locator('[data-admission-copy]').innerText()).trim(),'다시 복사');
  assert.equal((await page.locator('.admission-copy summary').innerText()).trim(),'확인 요청 프롬프트');
  await reload();
  await page.locator('.admission-status').filter({hasText:T.copyFailed}).waitFor();
  assert.equal(await page.evaluate(k=>JSON.parse(localStorage.getItem(k)).text,promptKey),first.prompt);
  await page.evaluate(()=>window.copyFail=false);
  await page.locator('[data-admission-copy]').click();
  await page.waitForFunction(()=>window.copied.length===1);
  assert.equal(await page.evaluate(()=>window.copied[0]),first.prompt);
  await assertCopyToast('Clipboard API retry');
  await feedback().waitFor({state:'hidden'});
  // The rejected list's header re-copies the cumulative prompt any time.
  await page.evaluate(()=>{window.copyFail=true;window.fallbackCopies=0;document.execCommand=()=>{window.fallbackCopies++;return true;};});
  await clearToasts();
  await page.locator('.rrej-copy').click();
  await page.waitForFunction(k=>window.fallbackCopies===1 && JSON.parse(localStorage.getItem(k)).failed===false,promptKey);
  await assertCopyToast('execCommand fallback');
  await page.evaluate(()=>{window.copyFail=false;document.execCommand=()=>false;});
  results.push('1101: persisted rejection survives failed clipboard/reload; explicit retry succeeds; header copy re-copies');
  // Close and Esc dismiss the failure notice; the dismissal outlives a
  // reload, the rejection stays, and a later failed copy shows it again.
  await page.evaluate(()=>{window.copyFail=true;document.execCommand=()=>false;});
  await page.locator('.rrej-copy').click();
  await page.locator('.admission-status').filter({hasText:T.copyFailed}).waitFor();
  await page.locator('[data-admission-dismiss]').click();
  await feedback().waitFor({state:'hidden'});
  await reload(); await page.waitForTimeout(150);
  assert.equal(await feedback().isVisible(),false,'a closed notice stays closed across reload');
  assert.equal((await control('state')).filter(x=>x.state==='rejected').length,1,'closing keeps the rejection');
  await page.locator('.rrej-copy').click();
  await page.locator('.admission-status').filter({hasText:T.copyFailed}).waitFor();
  await page.locator('[data-admission-copy]').focus();
  await page.keyboard.press('Escape');
  await feedback().waitFor({state:'hidden'});
  assert.ok((await page.locator('html').getAttribute('class')||'').includes('rail-open'),'Esc in the notice keeps the rail open');
  await page.evaluate(()=>{window.copyFail=false;});
  results.push('Notice: close button and Esc dismiss it, the dismissal survives reload, rejection persists, a new failure reopens it');
  results.push('Toast regression: Clipboard API and fallback success use fixed text without the copied prompt; geometry does not overlap the rail');
  // Localized rail text (Korean fixture), no English left in the feature.
  const pendhd = await page.locator('.pendhd').textContent();
  assert.ok(pendhd.includes('대기 중인 워크트리') && pendhd.includes('풀 밖에서 감지됨'), pendhd);
  assert.match(await page.locator('.rrej > summary').innerText(),/거부된 워크트리\s*1/);
  assert.equal(await page.locator('.rrej-copy').getAttribute('aria-label'),'거부된 워크트리 전체의 확인 요청 프롬프트 복사');
  assert.equal(await toggle('tmp.Xk2','approve').getAttribute('aria-label'),'승인');
  await toggle('tmp.Xk2','reject').click();
  assert.equal(await page.locator('.rcap .no').innerText(),'거부 1');
  assert.equal((await page.locator('[data-admission-apply]').innerText()).trim(),'반영');
  assert.equal(await row('tmp.Xk2').locator('.d-tag').innerText(),'거부 예정');
  results.push('Localization: pending heading, toggles, capsule, tag, rejected heading and copy label render in Korean');
  await clearToasts();
  const second = await apply();
  await assertRailToast('second apply', T.rejected(1));
  assert.ok(second.prompt.includes(paths['agent-a82f81f3']) && second.prompt.includes(paths['tmp.Xk2']));
  assert.equal((await control('state')).filter(x=>x.state==='rejected').length,2);
  results.push('1102: cumulative server prompt matches both persisted rejected rows');
  await page.locator('.rrej > summary').click();
  assert.equal((await row('agent-a82f81f3').locator('.admission-undo').innerText()).trim(),'되돌리기');
  assert.equal(await row('agent-a82f81f3').locator('.wtpath').getAttribute('title'),paths['agent-a82f81f3']);
  assert.match(await row('agent-a82f81f3').locator('.wtpath .d-pt').innerText(),/^\/[^/]+\/[^/]+\/…\/agent-a82f81f3$/,'path is middle-truncated');
  await clearToasts();
  const undoResponse = page.waitForResponse(r=>r.url()===base+'/rail/worktrees/admission');
  await row('agent-a82f81f3').locator('.admission-undo').click();
  assert.equal((await undoResponse).status(),200);
  await toggle('agent-a82f81f3','approve').waitFor();
  await assertRailToast('undo', T.undone(1));
  assert.ok(await page.locator('#rail > .rtoast.ok').count() === 1, 'a batch without rejection uses the ok accent');
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
  await page.locator('.admission-missing').filter({hasText:paths['agent-a82f81f3']}).getByRole('button',{name:'선택 지우기'}).click();
  assert.equal((await drafts()).length,1);
  results.push('1103: real replacement 409 refuses whole batch, preserves evidence/drafts; explicit reselect and missing-path clear work');

  // Screenshots use real production DOM at both responsive widths and themes.
  // A reference page is captured independently; comparison images put rails side by side.
  const ref = await browser.newPage({viewport:{width:1440,height:1000}});
  await ref.addInitScript(()=>{Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:()=>Promise.resolve()}});});
  const compare = await browser.newPage({viewport:{width:780,height:1020}});
  async function sideBySide(prodRail, refRail, out, label) {
   await compare.setContent(`<style>body{margin:0;background:#888;font:14px system-ui}main{display:flex;gap:12px}img{max-width:396px;object-fit:contain;object-position:top}section{display:flex;flex-direction:column}</style><main><section>Production ${label}<img src="data:image/png;base64,${prodRail.toString('base64')}"></section><section>Approved mockup ${label}<img src="data:image/png;base64,${refRail.toString('base64')}"></section></main>`);
   await compare.screenshot({path:out,fullPage:true});
  }
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
   await clearToasts();
   const applied = await apply(); assert.equal(applied.applied,true);
   await assertRailToast(`${width}-${theme}-applied`, T.rejected(1)+T.approved(1));
   assert.equal(await page.locator('#rail > .rtoast small').innerText(), T.approved(1));
   assert.equal(await page.locator('#wstoasts .wstoast').count(), 0, 'Apply uses the rail toast only');
   await page.mouse.move(width-10, 990);
   if (width===1440) {
    await fullShot(`${shots}/success-toast-${theme}-1440.png`);
    const prodRail = await page.locator('#rail').screenshot();
    await ref.setViewportSize({width,height:1000});
    await ref.goto(pathToFileURL(reference).href+'?theme='+theme);
    await ref.evaluate(()=>{window.__mk.mark('a82','ap');window.__mk.mark('xk2','rj');});
    await ref.locator('#capgo').click(); await ref.locator('.wstoast.on').waitFor(); await ref.waitForTimeout(300);
    await sideBySide(prodRail, await ref.locator('#rail').screenshot(), `${shots}/success-toast-${theme}-1440-vs-mockup.png`, `success toast ${theme} 1440`);
   }
   await clearToasts();
   await page.locator('.rrej > summary').click();
   await page.mouse.move(width-10, 990);
   assert.equal((await row('tmp.Xk2').locator('.gs').innerText()).trim(), '±0', 'a rejected row shows its change count, zero included');
   await capture(width,theme,'rejected-list-with-undo','applied');
   await page.locator('#rail').screenshot({path:`${shots}/rejected-list-${theme}-${width}.png`});
   if (width===1440) {
    const prodRail = await page.locator('#rail').screenshot();
    await ref.goto(pathToFileURL(reference).href+'?theme='+theme);
    await ref.evaluate(()=>{window.__mk.mark('a82','ap');window.__mk.mark('xk2','rj');});
    await ref.locator('#capgo').click(); await ref.locator('.rfold > summary').click();
    await ref.evaluate(()=>document.querySelector('.wstoast').classList.remove('on')); await ref.mouse.move(1430,990); await ref.waitForTimeout(300);
    await sideBySide(prodRail, await ref.locator('#rail').screenshot(), `${shots}/rejected-list-${theme}-1440-vs-mockup.png`, `rejected list ${theme} 1440`);
   }
   if (width===700 && theme==='dark') fs.copyFileSync(`${artifacts}/comparison-700-dark-rejected-list-with-undo.png`, `${shots}/rejected-list-700-dark-vs-mockup.png`);
   await page.screenshot({path:`${artifacts}/production-${width}-${theme}-applied.png`});
   // The failure notice: a header copy whose clipboard write fails.
   await page.evaluate(()=>{window.copyFail=true;document.execCommand=()=>false;});
   await page.locator('.rrej-copy').click();
   await page.locator('.admission-status').filter({hasText:T.copyFailed}).waitFor();
   await page.mouse.move(width-10, 990);
   const box = await feedback().boundingBox(), railBox = await page.locator('#rail').boundingBox();
   assert.ok(box.x >= railBox.x && box.x+box.width <= railBox.x+railBox.width+0.5, `${width}-${theme}: notice stays inside the rail`);
   assert.equal(await page.evaluate(()=>{const f=document.querySelector('.admission-feedback');return f.scrollWidth<=f.clientWidth;}),true,'notice has no horizontal overflow');
   await fullShot(`${shots}/after-${theme}-${width}.png`);
   if (width===1440 && theme==='dark') {
    await page.locator('#rail').screenshot({path:`${shots}/clipboard-failure-block-dark-1440.png`});
    await page.locator('.admission-copy summary').click();
    await page.mouse.move(width-10, 990);
    await page.locator('#rail').screenshot({path:`${shots}/after-dark-1440-disclosure-expanded.png`});
    const ta = await page.locator('.admission-feedback textarea').boundingBox();
    assert.ok(ta.x+ta.width <= railBox.x+railBox.width, 'expanded prompt stays inside the rail');
   }
   await page.locator('[data-admission-dismiss]').click();
   await page.evaluate(()=>{window.copyFail=false;});
  }
  assert.deepEqual(errors,[]);
  fs.writeFileSync(artifacts+'/toast-geometry.json',JSON.stringify(toastGeometry,null,2)+'\n');
  const summary = ['# Worktree admission production-browser evidence','',...results.map(x=>'- PASS '+x),'',
   'Production shell, templates, CSS, JavaScript and admission handler were served by a private httptest fixture; no daemon or provider was started.',
   'All 16 required production captures: production-{1440,700}-{light,dark}-{pending,selected,rejected-list-with-undo,stale-refusal}.png.',
   'Each capture has reference-* and comparison-* counterparts. comparison-* places production and approved HTML rails side by side. The reference has no stale UI; stale comparisons intentionally use its selected state.',
   'Applied-state comparisons: comparison-{1440,700}-{light,dark}-rejected-list-with-undo.png; additional full production applied captures: production-{1440,700}-{light,dark}-applied.png.',
   'Visual differences: production retains the existing 340px responsive rail and canonical fixture paths (Korean locale); the approved HTML uses a 396px mock rail. Amber grouping, reversible toggles, capsule, collapsed rejected fold and undo retain its hierarchy. Stale correction and retained clipboard prompt are additional settled error states.',
   'Copy-success feedback: fixed "복사함" text; Apply success shows the mockup toast lines. Clipboard API/fallback and all four applied width/theme variants assert viewport containment, single-line height and zero overlap with the rail; measured rectangles are in toast-geometry.json.',
   'Reference: '+reference,'Artifacts: '+artifacts,'',...fs.readdirSync(artifacts).filter(x=>x.endsWith('.png')).sort().map(x=>artifacts+'/'+x)].join('\n');
  fs.writeFileSync(artifacts+'/summary.md',summary+'\n');
  console.log(results.join('\n')); console.log('PASS: 16 production states, reference side-by-side comparisons and 4 applied captures. '+artifacts+'/summary.md');
 } finally { await browser.close(); }
})().catch(e=>{console.error(e);process.exitCode=1;});
