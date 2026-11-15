'use strict';
window.werkschaftBar.onState(state => {
  document.getElementById('title').textContent = state.title;
  document.body.style.backgroundColor = state.color;
  document.body.style.color = state.symbolColor;
});
document.getElementById('menu').addEventListener('click', () => window.werkschaftBar.openMenu());
