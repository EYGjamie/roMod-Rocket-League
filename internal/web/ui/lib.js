// Shared helpers and components of the interface.

const token = new URLSearchParams(location.search).get('k') || '';
export const eventsURL = '/api/events?k=' + encodeURIComponent(token);

// api calls the local server. Bodies are sent as JSON unless they are
// FormData; errors become exceptions with the server's message.
export async function api(path, opts = {}) {
  const init = { method: opts.method || 'GET', headers: { 'X-Token': token } };
  if (opts.body instanceof FormData) {
    init.body = opts.body;
  } else if (opts.body !== undefined) {
    init.headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(opts.body);
  }
  const res = await fetch(path, init);
  const data = await res.json().catch(() => ({}));
  if (!res.ok || (data && data.error && !opts.allowError)) throw new Error((data && data.error) || 'HTTP ' + res.status);
  return data;
}

const props = new Set(['value', 'checked', 'selected', 'disabled', 'indeterminate', 'open']);

// h builds an element: h('div', {class: 'x', onclick: fn}, child, …).
export function h(tag, attrs, ...kids) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v == null || v === false) continue;
    if (k === 'class') el.className = v;
    else if (k === 'style' && typeof v === 'object') Object.assign(el.style, v);
    else if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2), v);
    else if (props.has(k)) el[k] = v;
    else el.setAttribute(k, v === true ? '' : v);
  }
  for (const c of kids.flat(Infinity)) {
    if (c != null && c !== false) el.append(c instanceof Node ? c : String(c));
  }
  return el;
}

// ico is a Phosphor icon, sized by the font size or px.
export const ico = (name, px) => h('i', { class: 'ph ph-' + name, 'aria-hidden': 'true', style: px ? { fontSize: px + 'px' } : null });

// --- events from the server ---

const listeners = new Map();
export function on(topic, fn) {
  if (!listeners.has(topic)) listeners.set(topic, new Set());
  listeners.get(topic).add(fn);
  return () => listeners.get(topic).delete(fn);
}
export function emit(topic) {
  for (const fn of listeners.get(topic) || []) fn(topic);
  for (const fn of listeners.get('*') || []) fn(topic);
}

// debounce delays fn until calls stop for ms.
export function debounce(fn, ms = 250) {
  let t;
  return (...args) => { clearTimeout(t); t = setTimeout(() => fn(...args), ms); };
}

// --- formatting ---

const dayFmt = new Intl.DateTimeFormat('de-DE', { day: '2-digit', month: '2-digit' });
const weekdayFmt = new Intl.DateTimeFormat('de-DE', { weekday: 'short', day: '2-digit', month: '2-digit' });
const timeFmt = new Intl.DateTimeFormat('de-DE', { hour: '2-digit', minute: '2-digit' });
const valid = d => d && new Date(d).getFullYear() > 1; // Go's zero time is year 1
const daysAgo = d => { const a = new Date(d), b = new Date(); a.setHours(0, 0, 0, 0); b.setHours(0, 0, 0, 0); return Math.round((b - a) / 864e5); };
const yearOf = d => new Date(d).getFullYear() === new Date().getFullYear() ? '' : new Date(d).getFullYear();
export const fmt = {
  clock: d => valid(d) ? timeFmt.format(new Date(d)) : '–',
  // day is "heute", "gestern" or the date; title gives "Heute" or "Do., 16.10.".
  day: d => !valid(d) ? '–' : ['heute', 'gestern'][daysAgo(d)] || dayFmt.format(new Date(d)) + yearOf(d),
  title: d => !valid(d) ? '–' : ['Heute', 'Gestern'][daysAgo(d)] || weekdayFmt.format(new Date(d)) + yearOf(d),
  // when is "heute 20:36", "gestern 22:14" or "16.10. 18:10"; short drops "heute".
  when: (d, short) => !valid(d) ? '–' : (short && daysAgo(d) === 0 ? '' : fmt.day(d) + ' ') + fmt.clock(d),
  dur: s => s > 0 ? `${Math.floor(s / 60)}:${String(Math.round(s % 60)).padStart(2, '0')}` : '–',
  pct: (a, b) => b > 0 ? Math.round(100 * a / b) + '%' : '–',
  rel(d) {
    if (!valid(d)) return '';
    const s = (new Date(d) - Date.now()) / 1000;
    const abs = Math.abs(s);
    const unit = abs < 60 ? [Math.round(abs), 'Sek.'] : abs < 3600 ? [Math.round(abs / 60), 'Min.'] : abs < 86400 ? [Math.round(abs / 3600), 'Std.'] : [Math.round(abs / 86400), 'Tagen'];
    return (s < 0 ? 'vor ' : 'in ') + unit.join(' ');
  },
  num: n => typeof n === 'number' ? n.toLocaleString('de-DE', { maximumFractionDigits: 1 }) : (n ?? '–'),
};

export const visibilityNames = { public: 'Öffentlich', unlisted: 'Nicht gelistet', private: 'Privat' };

// tierNames are the patron tiers of ballchasing.
export const tierNames = { regular: 'ohne Patreon', gold: 'Gold', diamond: 'Diamond', champion: 'Champion', gc: 'Grand Champion' };

const playlistNames = {
  'ranked-duels': 'Ranked 1s', 'ranked-doubles': 'Ranked 2s', 'ranked-standard': 'Ranked 3s', 'ranked-solo-standard': 'Solo 3s',
  'unranked-duels': 'Casual 1s', 'unranked-doubles': 'Casual 2s', 'unranked-standard': 'Casual 3s', 'unranked-chaos': 'Casual 4s',
  tournament: 'Turnier', private: 'Privat',
};
// plName is the short name of a playlist, else the server's name.
export const plName = (id, name) => playlistNames[id] || name || id || '–';

// uploadQuota is the 24-hour upload count of an account, or null when
// ballchasing sent no limit.
export function uploadQuota(acc) {
  const q = acc?.uploads_24h;
  if (acc?.status !== 'ok' || !q?.max) return null;
  return { used: q.used, max: q.max, full: q.used >= q.max };
}

// system describes the operating system the tool runs on, from the state's
// platform: what it offers and how to word things there.
export function system(state) {
  const p = state?.platform || {};
  const os = p.os || 'windows';
  return { ...p, os, windows: os === 'windows', mac: os === 'darwin', linux: os === 'linux' };
}
export const stateNames = {
  queued: 'wartet', uploading: 'lädt hoch', processing: 'wird verarbeitet', done: 'fertig',
  retry: 'neuer Versuch', failed: 'fehlgeschlagen', rejected: 'abgelehnt', seeded: 'nicht hochgeladen',
};

// --- feedback ---

export function flash(text, kind = 'ok') {
  const el = h('div', { class: kind === 'err' ? 'toast bad' : 'toast' }, ico(kind === 'err' ? 'warning' : 'check'), h('span', null, text),
    h('button', { 'aria-label': 'Schließen', onclick: () => el.remove() }, ico('x')));
  document.getElementById('flash').append(el);
  setTimeout(() => el.remove(), kind === 'err' ? 7000 : 5000);
}

// run executes an async action and reports errors.
export async function run(fn, okText) {
  try {
    const r = await fn();
    if (okText) flash(typeof okText === 'function' ? okText(r) : okText);
    return r;
  } catch (e) {
    flash(e.message, 'err');
    return undefined;
  }
}

// modal shows a dialog; build receives a close function and returns the
// body. Resolves with what close is given.
export function modal(title, build) {
  return new Promise(resolve => {
    const prev = document.activeElement;
    let bg;
    const close = v => {
      bg.remove();
      document.removeEventListener('keydown', onKey, true);
      prev?.focus?.();
      resolve(v);
    };
    const onKey = e => { if (e.key === 'Escape') { e.stopPropagation(); close(null); } };
    const box = h('div', { class: 'dialog', role: 'dialog', 'aria-modal': 'true', 'aria-label': title }, h('h2', null, title), build(close));
    bg = h('div', { class: 'backdrop', onmousedown: e => { if (e.target === bg) close(null); } }, box);
    document.body.append(bg);
    document.addEventListener('keydown', onKey, true);
    (box.querySelector('[autofocus]') || box.querySelector('input, select, .btn-primary') || box).focus?.();
  });
}

export function confirmDialog(title, text, okLabel = 'OK', danger = false) {
  return modal(title, close => [
    h('div', { class: 'dialog-body' }, text),
    h('div', { class: 'dialog-actions' },
      h('button', { class: 'btn btn-secondary', onclick: () => close(false) }, 'Abbrechen'),
      h('button', { class: 'btn btn-primary' + (danger ? ' danger' : ''), onclick: () => close(true), autofocus: true }, okLabel)),
  ]);
}

export function promptDialog(title, label, value = '') {
  return modal(title, close => {
    const input = h('input', { class: 'input', type: 'text', value, autofocus: true, 'aria-label': label, onkeydown: e => { if (e.key === 'Enter') close(input.value.trim() || null); } });
    return [
      h('div', { class: 'dialog-body stack' }, h('label', null, label), input),
      h('div', { class: 'dialog-actions' }, h('button', { class: 'btn btn-secondary', onclick: () => close(null) }, 'Abbrechen'),
        h('button', { class: 'btn btn-primary', onclick: () => close(input.value.trim() || null) }, 'OK')),
    ];
  });
}

// menu opens a dropdown under the anchor. items: {label, run, icon, on,
// danger, accent, count, pad}, {section} for a heading or '-' for a line.
// onDismiss runs when it closes without a choice.
export function menu(anchor, items, { width = 250, onDismiss } = {}) {
  document.querySelector('.menu')?.dismiss();
  const close = () => { m.remove(); document.removeEventListener('mousedown', outside, true); document.removeEventListener('keydown', key, true); };
  const dismiss = () => { close(); onDismiss?.(); };
  const outside = e => { if (!m.contains(e.target)) dismiss(); };
  const key = e => {
    const buttons = [...m.querySelectorAll('button')];
    const i = buttons.indexOf(document.activeElement);
    if (e.key === 'Escape') { dismiss(); anchor.focus(); e.stopPropagation(); }
    if (e.key === 'ArrowDown') { buttons[(i + 1) % buttons.length].focus(); e.preventDefault(); }
    if (e.key === 'ArrowUp') { buttons[(i - 1 + buttons.length) % buttons.length].focus(); e.preventDefault(); }
  };
  const m = h('div', { class: 'menu', role: 'menu', style: { width: width + 'px' } }, items.filter(Boolean).map(it => it === '-' ? h('hr')
    : it.section ? h('div', { class: 'sec' }, it.section)
      : h('button', { role: 'menuitem', class: [it.on && 'on', it.danger && 'danger', it.accent && 'accent'].filter(Boolean).join(' '),
        style: it.pad ? { paddingLeft: it.pad + 'px' } : null, onclick: () => { close(); it.run(); } },
      it.icon ? ico(it.icon) : null, h('span', null, it.label), it.count != null ? h('span', { class: 'count' }, it.count) : null,
      it.on !== undefined ? h('i', { class: 'ph ph-check', style: { visibility: it.on ? 'visible' : 'hidden' } }) : null)));
  m.dismiss = dismiss;
  document.body.append(m);
  const r = anchor.getBoundingClientRect();
  const w = m.offsetWidth, ht = m.offsetHeight;
  m.style.left = Math.max(8, Math.min(r.left, innerWidth - w - 8)) + 'px';
  m.style.top = (r.bottom + ht + 8 > innerHeight ? Math.max(8, r.top - ht - 4) : r.bottom + 4) + 'px';
  document.addEventListener('mousedown', outside, true);
  document.addEventListener('keydown', key, true);
  m.querySelector('button')?.focus();
}

// sw is a switch; it flips at once and reports the new value.
export function sw(on, onchange, label) {
  const b = h('button', { type: 'button', class: 'switch', role: 'switch', 'aria-checked': String(!!on), 'aria-label': label, onclick: e => {
    e.stopPropagation();
    on = !on;
    b.setAttribute('aria-checked', String(on));
    onchange(on);
  } });
  return b;
}

// seg is a segmented control of [value, label] options.
let segs = 0;
export function seg(options, current, onPick, small) {
  const name = 'seg' + ++segs;
  return h('span', { class: 'seg' + (small ? ' sm' : '') }, options.map(([v, label]) => h('label', { class: 'seg-opt' },
    h('input', { type: 'radio', name, checked: v === current, onchange: () => onPick(v) }), label)));
}

// --- folders ---

let groupsCache = null;
on('groups', () => { groupsCache = null; });

export async function loadGroups() {
  if (!groupsCache) groupsCache = api('/api/groups');
  try { return await groupsCache; } catch (e) { groupsCache = null; throw e; }
}

// tree turns the flat group list into nested nodes sorted by name.
export function tree(groups) {
  const byParent = new Map();
  for (const g of groups) {
    if (!byParent.has(g.parent)) byParent.set(g.parent, []);
    byParent.get(g.parent).push(g);
  }
  const build = parent => (byParent.get(parent) || []).sort((a, b) => a.name.localeCompare(b.name, 'de'))
    .map(g => ({ ...g, children: build(g.id) }));
  return build('');
}

export function groupPath(groups, id) {
  const byId = new Map(groups.map(g => [g.id, g]));
  const parts = [];
  for (let g = byId.get(id), n = 0; g && n < 10; g = byId.get(g.parent), n++) parts.unshift(g.name);
  return parts.join(' › ');
}

// folderOptions lists the folders by path for a select.
export function folderOptions(groups, selected) {
  const out = [];
  const walk = nodes => {
    for (const g of nodes) {
      out.push(h('option', { value: g.id, selected: g.id === selected }, groupPath(groups, g.id)));
      walk(g.children);
    }
  };
  walk(tree(groups));
  return out;
}

// pickFolder opens the folder dropdown at the anchor: suggestions first,
// then all folders and a new one. Resolves to {id, name} or null.
export async function pickFolder(anchor, { suggestions = [], current = '' } = {}) {
  const { groups } = await loadGroups();
  return new Promise(resolve => {
    const pick = (id, name) => () => resolve({ id, name });
    const all = [];
    const walk = (nodes, depth) => {
      for (const g of nodes) {
        all.push({ label: g.name, icon: depth ? 'folder-simple' : 'folders', count: g.indirect, pad: 8 + 20 * depth, on: g.id === current, run: pick(g.id, groupPath(groups, g.id)) });
        walk(g.children, depth + 1);
      }
    };
    walk(tree(groups), 0);
    menu(anchor, [
      ...(suggestions.length ? [{ section: 'Vorschläge' }, ...suggestions.map(s => ({ label: s.name, icon: 'sparkle', on: s.group_id === current, run: pick(s.group_id, s.name) })), '-'] : []),
      { section: 'Alle Ordner' }, ...all, all.length ? '-' : null,
      { label: 'Neuer Ordner', icon: 'folder-simple-plus', accent: true, run: async () => {
        const name = await promptDialog('Neuer Ordner', 'Name des Ordners auf oberster Ebene');
        const g = name && await run(() => api('/api/groups', { method: 'POST', body: { name, parent: '' } }));
        resolve(g ? { id: g.id, name } : null);
      } },
    ], { onDismiss: () => resolve(null) });
  });
}

// moveTo moves replays into a folder and reports it; resolves to the number
// moved. quiet leaves the success message to the caller.
export async function moveTo(ids, f, quiet) {
  const res = await run(() => api('/api/replays/move', { method: 'POST', body: { ids, group: f.id } }));
  if (!res) return 0;
  if (res.errors.length) flash(`${res.done} verschoben, ${res.errors.length} fehlgeschlagen: ${res.errors[0]}`, 'err');
  else if (!quiet) flash(`${res.done === 1 ? 'Replay' : res.done + ' Replays'} nach „${f.name}“ verschoben`);
  return res.done;
}

// --- replays ---

export function resultBadge(r) {
  return h('span', { class: 'res ' + (r.result || 'none'), title: r.result === 'win' ? 'Sieg' : r.result === 'loss' ? 'Niederlage' : 'ohne Ergebnis' },
    r.result === 'win' ? 'S' : r.result === 'loss' ? 'N' : '–');
}

export function scoreLine(r) {
  return h('span', { class: 'nowrap' },
    h('span', { class: 'blue' }, (r.blue.name || 'Blau') + ' ' + r.blue.goals), ' : ',
    h('span', { class: 'orange' }, r.orange.goals + ' ' + (r.orange.name || 'Orange')));
}

// mates names the user's teammates: "mit Kev, Lumi", "allein" or "".
export function mates(r) {
  const team = r[r.my_team];
  if (!team?.players) return '';
  const names = team.players.filter(p => !p.me).map(p => p.name);
  return names.length ? 'mit ' + names.join(', ') : 'allein';
}

// uploadFiles sends dropped or picked replay files to the queue.
export async function uploadFiles(files) {
  const list = [...files].filter(f => f.name.toLowerCase().endsWith('.replay'));
  if (!list.length) return flash('Keine .replay-Dateien dabei', 'err');
  const form = new FormData();
  for (const f of list) form.append('files', f);
  await run(() => api('/api/upload', { method: 'POST', body: form }), r => `${r.queued} Replays werden hochgeladen`);
}

// fill replaces the children of el, skipping empty values (replaceChildren
// would insert "null").
export function fill(el, ...kids) {
  el.replaceChildren(...kids.flat(Infinity).filter(c => c != null && c !== false && c !== ''));
  return el;
}
