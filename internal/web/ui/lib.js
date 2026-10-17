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

// svg builds an SVG element.
export function svg(tag, attrs, ...kids) {
  const el = document.createElementNS('http://www.w3.org/2000/svg', tag);
  for (const [k, v] of Object.entries(attrs || {})) if (v != null) el.setAttribute(k, v);
  for (const c of kids.flat(Infinity)) if (c != null) el.append(c instanceof Node ? c : String(c));
  return el;
}

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

const dateFmt = new Intl.DateTimeFormat('de-DE', { day: '2-digit', month: '2-digit', year: '2-digit', hour: '2-digit', minute: '2-digit' });
const dayFmt = new Intl.DateTimeFormat('de-DE', { weekday: 'short', day: '2-digit', month: '2-digit' });
const timeFmt = new Intl.DateTimeFormat('de-DE', { hour: '2-digit', minute: '2-digit' });
const valid = d => d && !String(d).startsWith('0001');
export const fmt = {
  date: d => valid(d) ? dateFmt.format(new Date(d)) : '–',
  day: d => valid(d) ? dayFmt.format(new Date(d)) : '–',
  clock: d => valid(d) ? timeFmt.format(new Date(d)) : '–',
  dur: s => s > 0 ? `${Math.floor(s / 60)}:${String(Math.round(s % 60)).padStart(2, '0')}` : '–',
  pct: (a, b) => b > 0 ? Math.round(100 * a / b) + ' %' : '–',
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
export const stateNames = {
  queued: 'wartet', uploading: 'lädt hoch', processing: 'wird verarbeitet', done: 'fertig',
  retry: 'neuer Versuch', failed: 'fehlgeschlagen', rejected: 'abgelehnt', seeded: 'nicht hochgeladen',
};

// --- feedback ---

export function flash(text, kind = '') {
  const el = h('div', { class: 'flash ' + kind }, text);
  document.getElementById('flash').append(el);
  setTimeout(() => el.remove(), kind === 'err' ? 7000 : 3500);
}

// run executes an async action and reports errors.
export async function run(fn, okText) {
  try {
    const r = await fn();
    if (okText) flash(typeof okText === 'function' ? okText(r) : okText, 'ok');
    return r;
  } catch (e) {
    flash(e.message, 'err');
    return undefined;
  }
}

// modal shows a dialog; build receives a close function and returns the
// body. Resolves with what close is given.
export function modal(title, build, { wide } = {}) {
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
    const box = h('div', { class: 'modal', role: 'dialog', 'aria-modal': 'true', 'aria-label': title, style: wide ? { width: 'min(760px, 100%)' } : null },
      h('div', { class: 'modal-head' }, h('h2', null, title), h('button', { class: 'icon-btn', 'aria-label': 'Schließen', onclick: () => close(null) }, '✕')),
      build(close));
    bg = h('div', { class: 'overlay-bg', onmousedown: e => { if (e.target === bg) close(null); } }, box);
    document.body.append(bg);
    document.addEventListener('keydown', onKey, true);
    (box.querySelector('[autofocus]') || box.querySelector('input, select, button.primary') || box).focus?.();
  });
}

export function confirmDialog(title, text, okLabel = 'OK', danger = false) {
  return modal(title, close => [
    h('div', { class: 'modal-body' }, text),
    h('div', { class: 'modal-foot' },
      h('button', { onclick: () => close(false) }, 'Abbrechen'),
      h('button', { class: danger ? 'primary danger' : 'primary', onclick: () => close(true), autofocus: true }, okLabel)),
  ]);
}

export function promptDialog(title, label, value = '') {
  return modal(title, close => {
    const input = h('input', { type: 'text', value, autofocus: true, style: { width: '100%' }, onkeydown: e => { if (e.key === 'Enter') close(input.value.trim() || null); } });
    return [
      h('div', { class: 'modal-body stack' }, h('label', null, label), input),
      h('div', { class: 'modal-foot' }, h('button', { onclick: () => close(null) }, 'Abbrechen'),
        h('button', { class: 'primary', onclick: () => close(input.value.trim() || null) }, 'OK')),
    ];
  });
}

// menu opens a popup menu at the anchor. items: {label, run, danger,
// checked} or '-' for a separator.
export function menu(anchor, items) {
  document.querySelector('.menu')?.remove();
  const close = () => { m.remove(); document.removeEventListener('mousedown', outside, true); document.removeEventListener('keydown', key, true); };
  const outside = e => { if (!m.contains(e.target)) close(); };
  const key = e => {
    const buttons = [...m.querySelectorAll('button')];
    const i = buttons.indexOf(document.activeElement);
    if (e.key === 'Escape') { close(); anchor.focus(); e.stopPropagation(); }
    if (e.key === 'ArrowDown') { buttons[(i + 1) % buttons.length].focus(); e.preventDefault(); }
    if (e.key === 'ArrowUp') { buttons[(i - 1 + buttons.length) % buttons.length].focus(); e.preventDefault(); }
  };
  const m = h('div', { class: 'menu', role: 'menu' }, items.filter(Boolean).map(it => it === '-' ? h('hr') :
    h('button', { role: 'menuitem', class: it.danger ? 'danger' : '', onclick: () => { close(); it.run(); } },
      it.checked !== undefined ? (it.checked ? '✓ ' : ' ') : '', it.label)));
  document.body.append(m);
  const r = anchor.getBoundingClientRect();
  const w = m.offsetWidth, ht = m.offsetHeight;
  m.style.left = Math.max(8, Math.min(r.right - w, innerWidth - w - 8)) + 'px';
  m.style.top = (r.bottom + ht + 8 > innerHeight ? Math.max(8, r.top - ht - 4) : r.bottom + 4) + 'px';
  document.addEventListener('mousedown', outside, true);
  document.addEventListener('keydown', key, true);
  m.querySelector('button')?.focus();
}

export function sw(checked, onchange, label) {
  return h('label', { class: 'switch' },
    h('input', { type: 'checkbox', checked, onchange: e => onchange(e.target.checked) }),
    h('span', { class: 'track' }), label ? h('span', null, label) : null);
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

// pickFolder lets the user choose a folder, or create one. Resolves to
// {id, name} ("" id for the inbox) or null.
export async function pickFolder(title = 'In Ordner verschieben', { inbox = true, ok = 'Verschieben' } = {}) {
  const { groups } = await loadGroups();
  return modal(title, close => {
    let filter = '';
    let selected = null;
    const open = new Set();
    const list = h('div', { class: 'picker', role: 'tree' });
    const name = h('input', { type: 'text', placeholder: 'Name des neuen Ordners', style: { flex: '1' } });
    const where = h('span', { class: 'muted' });
    const render = () => {
      const match = g => !filter || g.name.toLowerCase().includes(filter) || g.children.some(match);
      const node = (g, depth) => {
        if (!match(g)) return null;
        const expanded = open.has(g.id) || filter;
        return [h('div', {
          class: 'tree-node' + (selected?.id === g.id ? ' active' : ''), role: 'treeitem', tabindex: 0,
          style: { paddingLeft: 8 + depth * 16 + 'px' },
          onclick: () => { selected = { id: g.id, name: groupPath(groups, g.id) }; render(); },
          ondblclick: () => close({ id: g.id, name: groupPath(groups, g.id) }),
          onkeydown: e => { if (e.key === 'Enter') close({ id: g.id, name: groupPath(groups, g.id) }); },
        },
          h('span', { class: 'twisty', onclick: e => { e.stopPropagation(); open.has(g.id) ? open.delete(g.id) : open.add(g.id); render(); } },
            g.children.length ? (expanded ? '▾' : '▸') : ''),
          h('span', { class: 'ellipsis' }, g.name), h('span', { class: 'count' }, g.indirect || '')),
        expanded ? g.children.map(c => node(c, depth + 1)) : null];
      };
      fill(list, 
        inbox ? h('div', { class: 'tree-node' + (selected?.id === '' ? ' active' : ''), tabindex: 0,
          onclick: () => { selected = { id: '', name: 'Eingang' }; render(); }, ondblclick: () => close({ id: '', name: 'Eingang' }) },
          h('span', { class: 'twisty' }), h('span', null, 'Eingang (kein Ordner)')) : null,
        tree(groups).map(g => node(g, 0)));
      where.textContent = selected && selected.id ? 'in „' + selected.name + '“' : 'auf oberster Ebene';
    };
    render();
    const create = async () => {
      const n = name.value.trim();
      if (!n) return name.focus();
      const parent = selected && selected.id ? selected.id : '';
      const g = await run(() => api('/api/groups', { method: 'POST', body: { name: n, parent } }));
      if (g) close({ id: g.id, name: (parent ? selected.name + ' › ' : '') + n });
    };
    return [
      h('div', { class: 'modal-body stack' },
        h('input', { type: 'search', placeholder: 'Ordner suchen …', autofocus: true, style: { width: '100%' },
          oninput: e => { filter = e.target.value.trim().toLowerCase(); render(); } }),
        list,
        h('div', { class: 'row nowrap' }, name, h('button', { onclick: create }, '+ Neuer Ordner')), where),
      h('div', { class: 'modal-foot' }, h('button', { onclick: () => close(null) }, 'Abbrechen'),
        h('button', { class: 'primary', onclick: () => selected && close(selected) }, ok)),
    ];
  });
}

// --- replays ---

export function resultBadge(r) {
  const cls = r.result || 'none';
  return h('span', { class: 'res ' + cls, title: r.result === 'win' ? 'Sieg' : r.result === 'loss' ? 'Niederlage' : 'ohne Ergebnis' },
    r.result === 'win' ? 'S' : r.result === 'loss' ? 'N' : '–');
}

export function scoreLine(r) {
  return h('span', { class: 'score' },
    h('span', { class: 'b' }, (r.blue.name || 'Blau') + ' ' + r.blue.goals), ' : ',
    h('span', { class: 'o' }, r.orange.goals + ' ' + (r.orange.name || 'Orange')));
}

const names = t => t.players.map(p => p.name).join(', ');

// playerTable shows the core numbers of both teams.
export function playerTable(r) {
  const rows = [];
  for (const [team, cls] of [[r.blue, 'b'], [r.orange, 'o']]) {
    for (const p of team.players) {
      rows.push(h('tr', { class: p.me ? 'me' : '' },
        h('td', null, h('span', { class: cls }, '●'), ' ', p.name, p.mvp ? h('span', { class: 'tag', style: { marginLeft: '6px' } }, 'MVP') : null),
        h('td', { class: 'num' }, p.score),
        h('td', { class: 'num' }, p.core ? p.goals : '–'),
        h('td', { class: 'num' }, p.core ? p.assists : '–'),
        h('td', { class: 'num' }, p.core ? p.saves : '–'),
        h('td', { class: 'num' }, p.core ? p.shots : '–')));
    }
  }
  return h('table', { class: 'stats' },
    h('thead', null, h('tr', null, ['Spieler', 'Punkte', 'Tore', 'Vorlagen', 'Paraden', 'Schüsse'].map(t => h('th', null, t)))),
    h('tbody', null, rows));
}

// replayActions returns the menu entries for a replay.
export function replayActions(r, changed) {
  const items = [];
  if (r.own) {
    items.push({ label: 'In Ordner verschieben …', run: async () => {
      const f = await pickFolder();
      if (f) await run(() => api('/api/replays/move', { method: 'POST', body: { ids: [r.id], group: f.id } }), 'Nach „' + f.name + '“ verschoben');
      changed();
    } });
    for (const v of ['public', 'unlisted', 'private']) {
      items.push({ label: 'Sichtbarkeit: ' + visibilityNames[v], checked: r.visibility === v, run: async () => {
        await run(() => api('/api/replays/visibility', { method: 'POST', body: { ids: [r.id], visibility: v } }), 'Sichtbarkeit geändert');
        changed();
      } });
    }
    items.push('-');
  }
  if (!r.borrowed) {
    items.push({ label: 'Ins Spiel laden', run: async () => {
      const res = await run(() => api('/api/replays/' + r.id + '/borrow', { method: 'POST' }), 'Liegt jetzt im Replay-Ordner. Im Spiel unter „Replays“ öffnen.');
      if (res) changed();
    } });
  } else {
    const keep = r.borrowed === 'temp';
    items.push({ label: keep ? 'Geladene Datei behalten' : 'Geladene Datei wieder löschen lassen', run: async () => {
      await run(() => api('/api/replays/' + r.id + '/keep', { method: 'POST', body: { keep } }));
      changed();
    } });
  }
  if (r.local) items.push({ label: 'Datei im Ordner zeigen', run: () => run(() => api('/api/replays/' + r.id + '/reveal', { method: 'POST' })) });
  items.push({ label: 'Alle Statistiken', run: () => { location.hash = '#/verlauf/' + r.id; } });
  items.push({ label: 'Auf ballchasing öffnen', run: () => window.open(r.link, '_blank', 'noopener') });
  if (r.own) {
    items.push('-', { label: 'Auf ballchasing löschen …', danger: true, run: async () => {
      if (await confirmDialog('Replay löschen', 'Das Replay wird auf ballchasing endgültig gelöscht. Das lässt sich nicht rückgängig machen.', 'Endgültig löschen', true)) {
        await run(() => api('/api/replays/delete', { method: 'POST', body: { ids: [r.id] } }), 'Gelöscht');
        changed();
      }
    } });
  }
  return items;
}

// replayRow renders one replay of a list. opts: selected (Set of ids,
// selection on if given), onSelect, expanded (Set), changed, current.
export function replayRow(r, opts = {}) {
  const selectable = opts.selected && r.own;
  const isSel = selectable && opts.selected.has(r.id);
  const open = opts.expanded && opts.expanded.has(r.id);
  const row = h('div', {
    class: 'item' + (isSel ? ' selected' : '') + (opts.current ? ' current' : ''), 'data-id': r.id, draggable: r.own ? 'true' : null,
    ondragstart: e => {
      const ids = opts.selected && opts.selected.has(r.id) ? [...opts.selected] : [r.id];
      e.dataTransfer.setData('application/x-replays', JSON.stringify(ids));
      e.dataTransfer.effectAllowed = 'move';
    },
  },
    selectable ? h('input', { type: 'checkbox', checked: isSel, 'aria-label': 'Auswählen',
      onclick: e => { e.stopPropagation(); opts.onSelect(r.id, e.target.checked, e.shiftKey); } }) : h('span'),
    resultBadge(r),
    h('div', { class: 'grow', style: { cursor: 'pointer' }, onclick: () => opts.onToggle && opts.onToggle(r.id) },
      scoreLine(r), h('div', { class: 'faint ellipsis', style: { fontSize: '12px' } }, names(r.blue) + ' – ' + names(r.orange))),
    h('div', { class: 'hide-sm ellipsis' }, r.playlist || '–'),
    h('div', { class: 'hide-sm ellipsis muted' }, r.map),
    h('div', { class: 'hide-sm num muted' }, fmt.dur(r.duration), r.overtime ? ' OT' : ''),
    h('div', { class: 'hide-sm keep-sm', style: { minWidth: 0 } },
      r.own ? (r.group ? h('span', { class: 'tag', title: r.group }, r.group) : h('span', { class: 'tag' }, 'Eingang'))
        : h('span', { class: 'tag', title: 'hochgeladen von ' + r.uploader }, 'von ' + r.uploader),
      r.borrowed ? h('span', { class: 'tag', style: { marginLeft: '4px' }, title: 'liegt im Replay-Ordner' }, '▶ im Spiel') : null,
      h('div', { class: 'when' }, fmt.date(r.date))),
    h('div', { class: 'actions' },
      h('a', { class: 'icon-btn', href: r.link, target: '_blank', rel: 'noopener noreferrer', title: 'Auf ballchasing öffnen', 'aria-label': 'Auf ballchasing öffnen' }, '↗'),
      h('button', { class: 'icon-btn', title: 'Weitere Aktionen', 'aria-label': 'Weitere Aktionen', onclick: e => menu(e.currentTarget, replayActions(r, opts.changed || (() => {}))) }, '⋯')),
    open ? h('div', { class: 'item-detail' },
      r.blue.players.concat(r.orange.players).some(p => p.core) ? playerTable(r) : h('div', { class: 'muted' }, 'Statistiken werden geladen …'),
      h('div', { class: 'row', style: { marginTop: '8px' } },
        h('a', { href: '#/verlauf/' + r.id }, 'Alle Statistiken'), r.own ? null : h('span', { class: 'muted' }, 'Hochgeladen von ' + r.uploader))) : null);
  if (open && !r.blue.players.concat(r.orange.players).some(p => p.core) && opts.loadDetail) opts.loadDetail(r.id);
  return row;
}

export function fileRow(f, retry) {
  return h('div', { class: 'item file' },
    h('span', { class: 'res loss', title: stateNames[f.state] }, '!'),
    h('div', { class: 'grow' }, h('div', { class: 'ellipsis' }, f.name),
      h('div', { class: 'faint', style: { fontSize: '12px' } }, stateNames[f.state] || f.state, f.error ? ' · ' + f.error : '')),
    h('div', { class: 'when' }, fmt.date(f.added)),
    retry ? h('button', { class: 'small', onclick: () => retry(f.name) }, 'Jetzt erneut') : h('span'));
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
