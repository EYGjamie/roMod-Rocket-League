// Interface core: routing, navigation, status bar and live updates.
import { api, h, on, emit, debounce, eventsURL, uploadFiles, run, system } from './lib.js';
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

export const state = { current: null };
let mounted = null;
let cleanup = [];

async function refreshState() {
  try {
    Object.assign(state, await api('/api/state'));
  } catch {
    return;
  }
  renderStatus();
  renderBadges();
  emit('ui:state');
}
const refreshStateSoon = debounce(refreshState, 200);

function badge(name, n) {
  const el = document.querySelector(`[data-badge="${name}"]`);
  if (!el) return;
  el.hidden = !n;
  if (name !== 'setup') el.textContent = n > 99 ? '99+' : n;
}

function renderBadges() {
  badge('failed', state.queue?.failed || 0);
  badge('inbox', state.inbox || 0);
  badge('rules', state.rule_tips || 0);
  badge('setup', state.account?.status === 'none' ? 1 : 0);
  document.getElementById('nav-foot').textContent = 'Version ' + (state.version || '');
}

function renderStatus() {
  const acc = state.account || {};
  const chips = [];
  const accText = { ok: acc.name, none: 'ballchasing nicht verbunden', invalid: 'Token ungültig', offline: 'ballchasing nicht erreichbar' }[acc.status];
  chips.push(h('a', { class: 'chip ' + (acc.status === 'ok' ? 'ok' : 'bad'), href: acc.status === 'ok' ? 'https://ballchasing.com/' : '#/einstellungen',
    target: acc.status === 'ok' ? '_blank' : null, rel: 'noopener', title: 'ballchasing' }, h('span', { class: 'dot' }), accText || '…'));
  chips.push(h('span', { class: 'chip ' + (state.stats_api ? 'ok' : ''), title: 'Verbindung zur Stats API von Rocket League' },
    h('span', { class: 'dot' }), state.in_match ? 'Im Match' : state.stats_api ? 'Rocket League verbunden' : 'Rocket League nicht verbunden'));
  if (state.queue?.offline) chips.push(h('span', { class: 'chip warn' }, h('span', { class: 'dot' }), 'Uploads warten'));
  if (state.syncing) chips.push(h('span', { class: 'chip' }, 'Abgleich läuft …'));
  const paused = state.paused || {};
  const all = paused.watch && paused.mark;
  const any = paused.watch || paused.mark;
  chips.push(h('button', { class: 'chip ' + (any ? 'warn' : 'ok'), title: 'Pausieren oder fortsetzen' + (system(state).hotkeys ? ' (Strg+Alt+P)' : ''),
    onclick: () => run(() => api('/api/pause', { method: 'POST', body: { watch: !any, mark: !any } })) },
    h('span', { class: 'dot' }), all ? 'Pausiert' : paused.watch ? 'Uploads pausiert' : paused.mark ? 'Auto-Speichern pausiert' : 'Läuft'));
  document.getElementById('status').replaceChildren(...chips);
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
  for (const a of document.querySelectorAll('#nav a')) a.classList.toggle('active', a.dataset.route === key);
  const root = document.getElementById('view');
  root.replaceChildren();
  root.scrollTop = 0;
  document.getElementById('title').textContent = view.title || '';
  const ctx = {
    param: decodeURIComponent(rest.join('/')),
    state,
    on: (topic, fn) => cleanup.push(on(topic, fn)),
    setTitle: t => { document.getElementById('title').textContent = t; },
  };
  try {
    mounted = await view.mount(root, ctx);
  } catch (e) {
    root.replaceChildren(h('div', { class: 'banner bad' }, 'Fehler: ' + e.message));
  }
}

function connect() {
  const es = new EventSource(eventsURL);
  es.onmessage = e => {
    emit(e.data);
    if (['state', 'queue', 'inbox', 'rules', 'settings', 'replays'].includes(e.data)) refreshStateSoon();
  };
  es.onerror = () => {
    document.getElementById('status').replaceChildren(h('span', { class: 'chip bad' }, h('span', { class: 'dot' }), 'Verbindung zum Programm getrennt'));
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
    if (e.key !== 'Escape' || e.defaultPrevented || document.querySelector('.overlay-bg, .menu')) return;
    if (e.target.matches('input, textarea') && e.target.value) return; // first Esc clears the field
    window.closeOverlay();
  });
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
