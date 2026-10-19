// Interface core: routing, navigation, status bar and live updates.
import { api, h, ico, fill, on, emit, debounce, eventsURL, uploadFiles, run, system, uploadQuota } from './lib.js';
import * as overview from './views/overview.js';
import * as history from './views/history.js';
import * as detail from './views/detail.js';
import * as stats from './views/stats.js';
import * as folders from './views/folders.js';
import * as inbox from './views/inbox.js';
import * as rules from './views/rules.js';
import * as settings from './views/settings.js';
import * as setup from './views/setup.js';

const views = {
  uebersicht: overview, verlauf: history, statistik: stats, ordner: folders,
  eingang: inbox, regeln: rules, einstellungen: settings, einrichtung: setup,
};

const state = {};
let mounted = null;
let cleanup = [];

async function refreshState() {
  try {
    Object.assign(state, await api('/api/state'));
  } catch {
    return;
  }
  renderFoot();
  renderNav();
  emit('ui:state');
}
const refreshStateSoon = debounce(refreshState, 200);

function renderNav() {
  const failed = state.queue?.failed || 0;
  const dot = (name, show, route, title) => {
    document.querySelector(`[data-dot="${name}"]`).hidden = !show;
    const a = document.querySelector(`[data-route="${route}"]`);
    a.title = title;
    a.setAttribute('aria-label', title);
  };
  dot('failed', failed, 'uebersicht', failed ? `Übersicht, ${failed} Fehler` : 'Übersicht');
  dot('inbox', state.inbox, 'eingang', state.inbox ? `Eingang, ${state.inbox} unsortiert` : 'Eingang');
  const setupNeeded = ['none', 'invalid'].includes(state.account?.status);
  dot('setup', setupNeeded, 'einstellungen', setupNeeded ? 'Einstellungen, ballchasing nicht verbunden' : 'Einstellungen');
}

// renderFoot fills the status bar; lost says the program is not reachable.
function renderFoot(lost) {
  const acc = state.account || {};
  const quota = uploadQuota(acc);
  const paused = state.paused || {};
  const any = paused.watch || paused.mark;
  const problem = lost ? 'Verbindung zum Programm getrennt'
    : { none: 'ballchasing nicht verbunden', invalid: 'Token ungültig', offline: 'ballchasing nicht erreichbar' }[acc.status];
  const v = state.version || '';
  fill(document.getElementById('foot'),
    quota ? h('span', { class: 'num' + (quota.full ? ' err' : '') }, `${quota.used} / ${quota.max} Uploads (24 h)`) : null,
    problem ? h('a', { href: acc.status === 'none' ? '#/einrichtung' : '#/einstellungen' }, problem) : null,
    state.queue?.offline ? h('span', null, 'Uploads warten') : null,
    state.syncing ? h('span', null, 'Abgleich läuft …') : null,
    h('button', { class: 'pause' + (any ? ' on' : ''), title: 'Pausieren oder fortsetzen', onclick: () => run(() => api('/api/pause', { method: 'POST', body: { watch: !any, mark: !any } })) },
      ico(any ? 'play' : 'pause', 13),
      h('span', null, (paused.watch && paused.mark ? 'Pausiert' : paused.watch ? 'Uploads pausiert' : paused.mark ? 'Auto-Speichern pausiert' : 'Läuft') + (system(state).hotkeys ? ', Strg+Alt+P' : ''))),
    h('span', { class: 'n5' }, /^\d/.test(v) ? 'v' + v : v));
}

async function route() {
  const [name = '', ...rest] = location.hash.replace(/^#\/?/, '').split('/');
  if (!state.version) await refreshState();
  let key = name;
  let view = views[key];
  if (key === 'verlauf' && rest.length) view = detail;
  if (!view) {
    key = state.setup_done ? 'uebersicht' : 'einrichtung';
    window.history.replaceState(null, '', '#/' + key);
    view = views[key];
  }
  mounted?.destroy?.();
  cleanup.forEach(f => f());
  cleanup = [];
  const navKey = key === 'einrichtung' ? 'einstellungen' : key;
  for (const a of document.querySelectorAll('#nav a')) a.classList.toggle('active', a.dataset.route === navKey);
  const root = document.getElementById('view');
  root.replaceChildren();
  const ctx = {
    param: decodeURIComponent(rest.join('/')),
    state,
    on: (topic, fn) => cleanup.push(on(topic, fn)),
  };
  try {
    mounted = await view.mount(root, ctx);
  } catch (e) {
    root.replaceChildren(h('div', { class: 'page' }, h('div', { class: 'note warn' }, 'Fehler: ' + e.message)));
  }
}

function connect() {
  const es = new EventSource(eventsURL);
  es.onmessage = e => {
    emit(e.data);
    if (['state', 'queue', 'inbox', 'rules', 'settings', 'replays'].includes(e.data)) refreshStateSoon();
  };
  es.onerror = () => {
    renderFoot(true);
    es.close();
    setTimeout(() => { refreshState(); connect(); }, 3000);
  };
}

function setupOverlay() {
  if (typeof window.closeOverlay !== 'function') return;
  const close = document.getElementById('close');
  close.hidden = false;
  close.onclick = () => window.closeOverlay();
  document.addEventListener('keydown', e => {
    if (e.key !== 'Escape' || e.defaultPrevented || document.querySelector('.backdrop, .menu')) return;
    if (e.target.matches('input, textarea') && e.target.value) return; // first Esc clears the field
    window.closeOverlay();
  });
  // the window has no title bar: free spots in the side and status bar move it
  const drag = e => {
    if (e.button !== 0 || e.target.closest('a, button')) return;
    e.preventDefault();
    window.dragOverlay();
  };
  document.getElementById('nav').addEventListener('mousedown', drag);
  document.getElementById('foot').addEventListener('mousedown', drag);
}

function setupDrop() {
  const drop = document.getElementById('drop');
  let depth = 0;
  const hasFiles = e => [...(e.dataTransfer?.types || [])].includes('Files');
  addEventListener('dragenter', e => { if (hasFiles(e)) { depth++; drop.hidden = false; } });
  addEventListener('dragleave', e => { if (hasFiles(e) && --depth <= 0) { depth = 0; drop.hidden = true; } });
  addEventListener('dragover', e => { if (hasFiles(e)) e.preventDefault(); });
  addEventListener('drop', e => {
    if (!hasFiles(e)) return;
    e.preventDefault();
    depth = 0;
    drop.hidden = true;
    uploadFiles(e.dataTransfer.files);
  });
}

window.onOverlayShown = () => {
  refreshState();
  mounted?.refresh?.();
};

addEventListener('hashchange', route);
setupOverlay();
setupDrop();
connect();
route();
