// Existing browser fixtures authenticate with the same real cookie as Go
// clients. No daemon option or production guard is changed for tests.
const playwright = require(process.env.WS_PLAYWRIGHT_REAL_MODULE || process.env.WS_PLAYWRIGHT_MODULE);
const cookies = JSON.parse(process.env.WS_DASHBOARD_TEST_COOKIES || '[]');
const chromium = Object.create(playwright.chromium);
function authenticatedPage(page) {
  const goto = page.goto.bind(page);
  page.goto = async (url, ...args) => {
    const cookie = cookies.find(c => new URL(c.url).origin === new URL(url).origin);
    if (cookie) await page.context().addCookies([cookie]);
    return goto(url, ...args);
  };
  return page;
}
function authenticatedBrowser(browser) {
  const newContext = browser.newContext.bind(browser);
  browser.newContext = async options => {
    const context = await newContext(options);
    if (cookies.length) await context.addCookies(cookies);
    const newPage = context.newPage.bind(context);
    context.newPage = async (...args) => authenticatedPage(await newPage(...args));
    return context;
  };
  const newPage = browser.newPage.bind(browser);
  browser.newPage = async options => {
    const page = await newPage(options);
    if (cookies.length) await page.context().addCookies(cookies);
    return authenticatedPage(page);
  };
  return browser;
}
for (const method of ['launch', 'connect', 'connectOverCDP']) {
  chromium[method] = async (...args) => authenticatedBrowser(await playwright.chromium[method](...args));
}
module.exports = { ...playwright, chromium };
