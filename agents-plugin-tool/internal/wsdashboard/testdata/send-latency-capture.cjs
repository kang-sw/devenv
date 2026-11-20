const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, out] = process.argv.slice(2);
(async () => {
 const server = await chromium.launchServer({headless:true});
 console.log('fixture_node_pid='+process.pid+' chromium_pid='+server.process().pid);
 const browser = await chromium.connect(server.wsEndpoint());
 try {
  for (const theme of ['dark','light']) {
   const context = await browser.newContext({viewport:{width:1800,height:1000},locale:'en-US',colorScheme:theme});
   const page = await context.newPage();
   await page.addInitScript(theme=>{localStorage.setItem('ws.dashboard.theme',theme);},theme);
   for (const [name,path] of [['index','/'],['session','/s/latency0']]) {
    await page.goto(base+path+'?theme='+theme);
    await page.locator('#r-rail .rs[data-rid="latency0"]').waitFor();
    if(name==='session') {
     const host=page.locator('iframe.sframe.on'); await host.waitFor();
     const frame=await (await host.elementHandle()).contentFrame();
     await frame.locator('#r-entries').waitFor();
     if (!(await frame.locator('#convo').isVisible())) await frame.locator('[data-tab="convo"]').first().click();
     const history=frame.locator('.modelhistory-trigger').first();
     assert.equal(await history.count(),1,'retained model history trigger');
     await history.click();
     await frame.locator('.modelhistory-popover').waitFor({state:'visible'});
    }
    await page.locator('#r-rail .rs[data-rid="latency0"] .rl').focus();
    await page.locator('.shp').waitFor({state:'visible'});
    await page.screenshot({path:out+'/'+name+'-'+theme+'.png'});
    console.log('capture '+out+'/'+name+'-'+theme+'.png');
   }
   await context.close();
  }
 } finally {await browser.close(); await server.close();}
})().catch(e=>{console.error(e);process.exit(1);});
