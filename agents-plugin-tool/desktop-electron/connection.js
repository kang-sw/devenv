'use strict';
// Diagnostics are text only; this local page has no preload or Node authority.
const diagnostic = new URLSearchParams(location.search);
for (const id of ['reason', 'log']) document.getElementById(id).textContent = diagnostic.get(id) || '';
