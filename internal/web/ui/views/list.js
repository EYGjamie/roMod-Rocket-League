// A replay list with selection, expandable rows and bulk actions, shared
// by the history and the folders view.
import { api, h, fill, run, menu, confirmDialog, pickFolder, replayRow, fileRow, visibilityNames } from '../lib.js';

// createReplayList returns {el, load}. query() gives the URL parameters.
export function createReplayList({ query, empty = 'Keine Replays gefunden.', head }) {
  let items = [];
  let total = 0;
  let lastIndex = -1;
  const selected = new Set();
  const expanded = new Set();
  const loading = new Set();
  const listEl = h('div', { class: 'list' });
  const bulk = h('div', { class: 'bulk', hidden: true });
  const el = h('div', { class: 'stack' }, listEl, bulk);

  const retry = name => run(() => api('/api/queue/retry', { method: 'POST', body: { name } }), 'Wird erneut versucht').then(() => load());

  async function loadDetail(id) {
    if (loading.has(id)) return;
    loading.add(id);
    const d = await run(() => api('/api/replays/' + encodeURIComponent(id)));
    loading.delete(id);
    if (!d) return;
    const i = items.findIndex(it => it.id === id);
    if (i >= 0) {
      items[i] = d.replay;
      render();
    }
  }

  function onSelect(id, on, shift) {
    const ids = items.filter(it => it.kind === 'replay' && it.own).map(it => it.id);
    const i = ids.indexOf(id);
    if (shift && lastIndex >= 0) {
      for (let j = Math.min(i, lastIndex); j <= Math.max(i, lastIndex); j++) on ? selected.add(ids[j]) : selected.delete(ids[j]);
    } else {
      on ? selected.add(id) : selected.delete(id);
    }
    lastIndex = i;
    render();
  }

  function onToggle(id) {
    expanded.has(id) ? expanded.delete(id) : expanded.add(id);
    render();
  }

  async function bulkRun(fn, okText) {
    const ids = [...selected];
    const res = await run(() => fn(ids));
    if (res) {
      const failed = res.errors?.length || 0;
      run(async () => { if (failed) throw new Error(`${res.done} erledigt, ${failed} fehlgeschlagen: ${res.errors[0]}`); }, `${res.done} ${okText}`);
      selected.clear();
    }
    load();
  }

  function renderBulk() {
    bulk.hidden = !selected.size;
    if (!selected.size) return;
    fill(bulk, 
      h('strong', null, `${selected.size} ausgewählt`),
      h('button', { class: 'primary', onclick: async () => {
        const f = await pickFolder(`${selected.size} Replays verschieben`);
        if (f) bulkRun(ids => api('/api/replays/move', { method: 'POST', body: { ids, group: f.id } }), 'nach „' + f.name + '“ verschoben');
      } }, 'In Ordner verschieben …'),
      h('button', { onclick: e => menu(e.currentTarget, Object.entries(visibilityNames).map(([v, label]) => ({
        label, run: () => bulkRun(ids => api('/api/replays/visibility', { method: 'POST', body: { ids, visibility: v } }), 'auf ' + label + ' gestellt'),
      }))) }, 'Sichtbarkeit ▾'),
      h('button', { class: 'danger', onclick: async () => {
        if (await confirmDialog('Replays löschen', `${selected.size} Replays werden auf ballchasing endgültig gelöscht. Das lässt sich nicht rückgängig machen.`, 'Endgültig löschen', true)) {
          bulkRun(ids => api('/api/replays/delete', { method: 'POST', body: { ids } }), 'gelöscht');
        }
      } }, 'Löschen'),
      h('span', { class: 'grow' }),
      h('button', { class: 'ghost', onclick: () => { selected.clear(); render(); } }, 'Auswahl aufheben'));
  }

  function render() {
    const own = items.filter(it => it.kind === 'replay' && it.own);
    const all = own.length > 0 && own.every(it => selected.has(it.id));
    const header = h('div', { class: 'list-head' },
      own.length ? h('input', { type: 'checkbox', checked: all, 'aria-label': 'Alle eigenen auswählen',
        onclick: e => { own.forEach(it => e.target.checked ? selected.add(it.id) : selected.delete(it.id)); render(); } }) : null,
      h('span', { class: 'grow' }, `${total} Einträge`), head ? head() : null);
    const rows = items.map(it => it.kind === 'file' ? fileRow(it, retry)
      : replayRow(it, { selected, onSelect, expanded, onToggle, loadDetail, changed: () => load() }));
    fill(listEl, header, ...(rows.length ? rows : [h('div', { class: 'empty' }, empty)]),
      items.length < total ? h('div', { class: 'more' }, h('button', { onclick: () => load(true) }, 'Mehr laden')) : null);
    renderBulk();
  }

  async function load(more = false) {
    const q = query();
    q.set('offset', more ? items.length : 0);
    q.set('limit', more ? 100 : Math.max(100, items.length));
    const res = await run(() => api('/api/replays?' + q));
    if (!res) return;
    items = more ? items.concat(res.items) : res.items;
    total = res.total;
    const present = new Set(items.map(it => it.id));
    for (const id of [...selected]) if (!present.has(id)) selected.delete(id);
    render();
  }

  return { el, load, reset: () => { items = []; selected.clear(); expanded.clear(); return load(); } };
}
