'use strict';
const markerKey = 'suma-departure-fixture-original';
if (sessionStorage.getItem(markerKey) === null) {
  sessionStorage.setItem(markerKey, JSON.stringify({operation: 'original-unknown-fixture', body: {scope: 'same-owned-window'}}));
}
const loads = Number(sessionStorage.getItem('suma-departure-fixture-loads') || '0') + 1;
sessionStorage.setItem('suma-departure-fixture-loads', String(loads));
document.getElementById('source').textContent = location.pathname;
document.getElementById('loads').textContent = String(loads);
document.getElementById('marker').textContent = sessionStorage.getItem(markerKey);
document.getElementById('activate').addEventListener('click', () => {
  document.getElementById('activity').textContent = 'Workbench active / 工作台已激活';
});
window.addEventListener('beforeunload', event => {
  event.preventDefault();
  event.returnValue = 'An original operation remains awaiting reconciliation';
});
