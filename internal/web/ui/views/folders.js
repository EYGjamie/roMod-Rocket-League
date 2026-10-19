import { api, h, ico, fill, fmt, run, flash, menu, debounce, confirmDialog, promptDialog, pickFolder, moveTo, syncNow, loadGroups, tree, groupPath, visibilityNames, resultBadge, scoreLine, mates } from '../lib.js';

const open = new Set();

export async function mount(root, ctx) {
  let selected = (() => { try { return sessionStorage.getItem('ordner'); } catch { return null; } })() || '';
  let filter = '';
  let groups = [];
  let synced = '';
  let rules = [];
  let items = [];
  let total = 0;
  const sel = new Set();
  const nodes = h('div', { style: { display: 'contents' }, role: 'tree', 'aria-label': 'Ordner' });
  const sync = h('div', { class: 'sync' });
  const pane = h('div', { class: 'pane' });
  root.append(h('div', { class: 'panes' },
    h('div', { class: 'side' },
      h('div', { class: 'head' }, h('h1', null, 'Ordner'),
        h('button', { class: 'btn btn-ghost btn-icon', 'aria-label': 'Neuer Ordner', title: 'Neuer Ordner', onclick: () => newFolder('') }, ico('folder-simple-plus')),
        h('button', { class: 'btn btn-ghost btn-icon', 'aria-label': 'Mit ballchasing abgleichen', title: 'Mit ballchasing abgleichen',
          onclick: syncNow }, ico('arrows-clockwise'))),
      h('label', { class: 'search flat' }, ico('magnifying-glass'),
        h('input', { type: 'search', placeholder: 'Ordner filtern', 'aria-label': 'Ordner filtern', oninput: e => { filter = e.target.value.trim().toLowerCase(); renderTree(); } })),
      h('div', { class: 'hsep' }), nodes, sync),
    pane));

  async function newFolder(parent) {
    const name = await promptDialog(parent ? 'Neuer Unterordner' : 'Neuer Ordner', parent ? `Name des Unterordners in „${groupPath(groups, parent)}“` : 'Name des Ordners auf oberster Ebene');
    if (!name) return;
    const g = await run(() => api('/api/groups', { method: 'POST', body: { name, parent } }), 'Ordner angelegt', `Ordner „${name}“ wird angelegt`);
    if (g) { if (parent) open.add(parent); select(g.id); }
  }

  function dropTarget(el, g) {
    el.addEventListener('dragover', e => { if (e.dataTransfer.types.includes('application/x-replays')) { e.preventDefault(); el.classList.add('drop'); } });
    el.addEventListener('dragleave', () => el.classList.remove('drop'));
    el.addEventListener('drop', async e => {
      el.classList.remove('drop');
      const raw = e.dataTransfer.getData('application/x-replays');
      if (!raw) return;
      e.preventDefault();
      if (await moveTo(JSON.parse(raw), { id: g.id, name: groupPath(groups, g.id) })) sel.clear();
    });
  }

  function node(g, depth) {
    const kids = g.children.length > 0;
    const toggle = () => { if (kids) { open.has(g.id) ? open.delete(g.id) : open.add(g.id); renderTree(); } };
    const el = h('div', { class: 'node' + (g.id === selected ? ' on' : ''), role: 'treeitem', tabindex: 0, 'aria-selected': String(g.id === selected),
      'aria-expanded': kids ? String(open.has(g.id)) : null, style: { paddingLeft: 8 + depth * 14 + 'px' }, title: `${g.direct} direkt, ${g.indirect} mit Unterordnern`,
      onclick: () => select(g.id),
      onkeydown: e => {
        if (e.key === 'Enter') select(g.id);
        else if (e.key === 'ArrowRight' && !open.has(g.id) || e.key === 'ArrowLeft' && open.has(g.id)) toggle();
      } },
      h('button', { tabindex: -1, 'aria-label': 'Auf- oder zuklappen', onclick: e => { e.stopPropagation(); toggle(); } },
        ico(kids ? (open.has(g.id) ? 'caret-down' : 'caret-right') : 'dot-outline')),
      h('span', { class: 'grow ell' }, g.name), h('span', { class: 'sm n5 num' }, g.indirect));
    dropTarget(el, g);
    return el;
  }

  function renderTree() {
    const out = [];
    const walk = (list, depth) => {
      for (const g of list) {
        if (!filter || g.name.toLowerCase().includes(filter)) out.push(node(g, filter ? 0 : depth));
        if (filter || open.has(g.id)) walk(g.children, depth + 1);
      }
    };
    walk(tree(groups), 0);
    fill(nodes, out.length ? out : h('div', { class: 'n5', style: { padding: '5px 8px' } }, groups.length ? 'Kein Ordner passt.' : 'Noch keine Ordner.'));
    sync.textContent = ctx.state.syncing ? 'Abgleich läuft …' : synced ? 'Abgeglichen ' + fmt.rel(synced) : '';
  }

  const row = r => h('div', { class: 'row g-folder' + (sel.has(r.id) ? ' sel' : ''), draggable: 'true', onclick: () => { location.hash = '#/verlauf/' + r.id; },
    ondragstart: e => {
      e.dataTransfer.setData('application/x-replays', JSON.stringify(sel.has(r.id) ? [...sel] : [r.id]));
      e.dataTransfer.effectAllowed = 'move';
    } },
    h('input', { type: 'checkbox', 'aria-label': 'Auswählen', checked: sel.has(r.id), onclick: e => e.stopPropagation(),
      onchange: e => { e.target.checked ? sel.add(r.id) : sel.delete(r.id); renderPane(); } }),
    resultBadge(r),
    h('div', { class: 'ell' }, scoreLine(r), h('span', { class: 'sm n5' }, ' ' + mates(r))),
    h('span', { class: 'ell n3', title: r.title }, r.title),
    h('span', { class: 'ell n4' }, r.map),
    h('span', { class: 'n4 num' }, fmt.dur(r.duration)),
    h('span', { class: 'sm n4 num nowrap r' }, fmt.when(r.date)));

  function renderPane() {
    const g = groups.find(x => x.id === selected);
    if (!g) return fill(pane, h('div', { class: 'empty' }, groups.length ? 'Wähle links einen Ordner.' : 'Noch keine Ordner. Lege einen an oder gleiche mit ballchasing ab.'));
    const subs = groups.filter(x => x.parent === g.id).sort((a, b) => a.name.localeCompare(b.name, 'de'));
    const ruleText = id => { const i = rules.findIndex(r => r.target_id === id); return i < 0 ? 'von Hand gefüllt' : `Regel ${i + 1} füllt diesen Ordner`; };
    const ids = [...sel];
    fill(pane,
      h('div', { class: 'head tight', style: { flexWrap: 'wrap' } },
        g.parent ? h('span', { class: 'n4', style: { fontSize: '13px' } }, groupPath(groups, g.parent) + ' ›') : null,
        h('span', { style: { fontSize: '18px' } }, g.name),
        h('span', { class: 'end actions' },
          h('a', { class: 'btn btn-secondary', href: 'https://ballchasing.com/group/' + g.id, target: '_blank', rel: 'noopener noreferrer' }, ico('arrow-square-out'), 'ballchasing'),
          h('button', { class: 'btn btn-secondary btn-icon', 'aria-label': 'Weitere Aktionen', title: 'Weitere Aktionen', onclick: e => menu(e.currentTarget, [
            { label: 'Neuer Unterordner …', icon: 'folder-simple-plus', run: () => newFolder(g.id) },
            '-',
            { label: 'Ordner löschen …', icon: 'trash', danger: true, run: async () => {
              if (g.indirect > 0 || subs.length) return flash('Nur leere Ordner ohne Unterordner lassen sich löschen.', 'err');
              if (await confirmDialog('Ordner löschen', `„${g.name}“ wird auf ballchasing gelöscht.`, 'Löschen', true)
                && await run(() => api('/api/groups/' + g.id, { method: 'DELETE' }), 'Ordner gelöscht', `Ordner „${g.name}“ wird gelöscht`)) select(g.parent);
            } },
          ], { width: 220 }) }, ico('dots-three')))),
      subs.length || items.length ? h('div', { class: 'rows' },
        subs.map(s => h('div', { class: 'row g-folder', onclick: () => { open.add(g.id); select(s.id); } },
          h('span'), h('span', { class: 'fbox' }, ico('folder-simple')), h('span', { class: 'ell' }, s.name),
          h('span', { class: 'ell n4' }, `${s.indirect} Replays`), h('span', { class: 'ell sm n5' }, ruleText(s.id)), h('span'),
          h('i', { class: 'ph ph-caret-right n5', style: { justifySelf: 'end' } }))),
        items.map(row)) : h('div', { class: 'empty' }, 'In diesem Ordner liegen keine Replays.'),
      items.length < total ? h('button', { class: 'btn btn-secondary more', onclick: () => loadItems(true) }, 'Mehr laden') : null,
      ids.length ? h('div', { class: 'bulk' }, h('span', null, `${ids.length} ausgewählt`),
        h('button', { class: 'btn btn-primary', onclick: async e => {
          const f = await pickFolder(e.currentTarget, { current: g.id });
          if (f && await moveTo(ids, f)) sel.clear();
        } }, ico('folder-simple'), 'Verschieben'),
        h('button', { class: 'btn btn-secondary', onclick: e => menu(e.currentTarget, Object.entries(visibilityNames).map(([v, label]) => ({ label, run: async () => {
          const res = await run(() => api('/api/replays/visibility', { method: 'POST', body: { ids, visibility: v } }), null, `Sichtbarkeit von ${ids.length === 1 ? '1 Replay' : ids.length + ' Replays'} wird geändert`);
          if (res) flash(res.errors.length ? `${res.done} geändert, ${res.errors.length} fehlgeschlagen: ${res.errors[0]}` : `${res.done} Replays auf „${label}“`, res.errors.length ? 'err' : 'ok');
        } })), { width: 180 }) }, ico('eye'), 'Sichtbarkeit'),
        h('span', { class: 'vsep' }),
        h('button', { class: 'btn btn-ghost btn-icon n4', 'aria-label': 'Auswahl aufheben', title: 'Auswahl aufheben', onclick: () => { sel.clear(); renderPane(); } }, ico('x'))) : null);
  }

  async function loadItems(more = false) {
    if (!groups.some(g => g.id === selected)) { items = []; total = 0; return renderPane(); }
    const q = new URLSearchParams({ group: selected, origin: 'own', offset: more ? items.length : 0, limit: more ? 100 : Math.max(100, items.length) });
    const res = await api('/api/replays?' + q);
    items = more ? items.concat(res.items) : res.items;
    total = res.total;
    const present = new Set(items.map(r => r.id));
    for (const id of sel) if (!present.has(id)) sel.delete(id);
    renderPane();
  }

  function select(id) {
    selected = id || '';
    try { sessionStorage.setItem('ordner', selected); } catch { /* private mode */ }
    sel.clear();
    items = [];
    renderTree();
    loadItems();
  }

  async function load() {
    const [data, r] = await Promise.all([loadGroups(), api('/api/rules')]);
    groups = data.groups;
    synced = data.synced;
    rules = r.rules;
    if (!groups.some(g => g.id === selected)) selected = tree(groups)[0]?.id || '';
    for (let g = groups.find(x => x.id === selected); g?.parent; g = groups.find(x => x.id === g.parent)) open.add(g.parent);
    renderTree();
  }

  await load();
  await loadItems();
  ctx.on('groups', debounce(async () => { await load(); loadItems(); }, 300));
  ctx.on('rules', debounce(async () => { await load(); renderPane(); }, 300));
  ctx.on('replays', debounce(() => loadItems(), 400));
  ctx.on('ui:state', renderTree);
  return { refresh: async () => { await load(); loadItems(); } };
}
