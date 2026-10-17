import { api, h, fill, run, debounce, sw, modal, confirmDialog, pickFolder, loadGroups, groupPath } from '../lib.js';

export const title = 'Regeln';

function playlistGroups(playlists) {
  const extra = ['hoops', 'ranked-hoops', 'rumble', 'ranked-rumble', 'dropshot', 'ranked-dropshot', 'snowday', 'ranked-snowday', 'heatseeker', 'dropshot-rumble', 'rocketlabs'];
  return [
    ['Ranked', playlists.filter(p => p.id.startsWith('ranked-') && !extra.includes(p.id))],
    ['Casual', playlists.filter(p => p.id.startsWith('unranked-'))],
    ['Extra-Modi', playlists.filter(p => extra.includes(p.id))],
    ['Sonstige', playlists.filter(p => ['tournament', 'private', 'season', 'offline'].includes(p.id))],
  ];
}

// editRule opens the rule editor; resolves to the saved rule or null.
async function editRule(rule, playlists, players) {
  const { groups } = await loadGroups();
  const r = { enabled: true, playlists: [], players: [], all_players: false, target_id: '', target_name: '', ...rule };
  let mode = r.target_id ? 'existing' : r.target_name ? 'new' : 'existing';
  return modal(rule.id ? 'Regel bearbeiten' : 'Neue Regel', close => {
    const targetLabel = h('span', { class: 'muted' }, r.target_id ? groupPath(groups, r.target_id) || 'Ordner nicht gefunden' : 'kein Ordner gewählt');
    const nameInput = h('input', { type: 'text', value: r.target_name, placeholder: 'z. B. 2s Ranked', style: { width: '100%' }, oninput: e => { r.target_name = e.target.value; } });
    const err = h('div', { class: 'banner bad', hidden: true });
    const toggle = (list, v, on) => { const i = list.indexOf(v); if (on && i < 0) list.push(v); if (!on && i >= 0) list.splice(i, 1); };
    const existingBox = h('div', { class: 'row' }, h('button', { onclick: async () => {
      const f = await pickFolder('Zielordner wählen', { inbox: false, ok: 'Auswählen' });
      if (f) { r.target_id = f.id; targetLabel.textContent = f.name; }
    } }, 'Ordner wählen …'), targetLabel);
    const newBox = h('div', null, nameInput, h('div', { class: 'faint', style: { fontSize: '12px', marginTop: '4px' } }, 'Wird beim ersten passenden Replay auf oberster Ebene angelegt.'));
    const showMode = () => { existingBox.hidden = mode !== 'existing'; newBox.hidden = mode !== 'new'; };
    showMode();
    const save = async () => {
      const body = { ...r, target_id: mode === 'existing' ? r.target_id : '', target_name: mode === 'new' ? r.target_name.trim() : (r.target_name || '') };
      try {
        const saved = await api('/api/rules' + (r.id ? '/' + r.id : ''), { method: r.id ? 'PUT' : 'POST', body });
        close(saved);
      } catch (e) {
        err.hidden = false;
        err.textContent = e.message;
      }
    };
    return [
      h('div', { class: 'modal-body stack' },
        err,
        h('h3', null, 'Playlists'),
        h('div', { class: 'faint', style: { fontSize: '12px' } }, 'Keine Auswahl heißt: jede Playlist.'),
        playlistGroups(playlists).map(([label, list]) => h('div', null,
          h('div', { class: 'muted', style: { fontSize: '12px', margin: '6px 0 4px' } }, label),
          h('div', { class: 'row', style: { gap: '6px 16px' } }, list.map(p => h('label', { class: 'check' },
            h('input', { type: 'checkbox', checked: r.playlists.includes(p.id), onchange: e => toggle(r.playlists, p.id, e.target.checked) }), p.name))))),
        h('hr', { class: 'sep' }),
        h('h3', null, 'Mitspieler'),
        players.length ? h('div', { class: 'picker', style: { maxHeight: '160px' } }, players.map(p => h('label', { class: 'check', style: { display: 'flex', padding: '3px 6px' } },
          h('input', { type: 'checkbox', checked: r.players.includes(p.pid), onchange: e => toggle(r.players, p.pid, e.target.checked) }),
          h('span', { class: 'grow' }, p.name), h('span', { class: 'faint' }, p.games + ' Spiele'))))
          : h('div', { class: 'muted' }, 'Noch keine Mitspieler bekannt.'),
        sw(r.all_players, v => { r.all_players = v; }, 'Alle gewählten Mitspieler müssen dabei sein (sonst reicht einer)'),
        h('hr', { class: 'sep' }),
        h('h3', null, 'Zielordner'),
        h('div', { class: 'row' },
          h('label', { class: 'check' }, h('input', { type: 'radio', name: 'mode', checked: mode === 'existing', onchange: () => { mode = 'existing'; showMode(); } }), 'Bestehender Ordner'),
          h('label', { class: 'check' }, h('input', { type: 'radio', name: 'mode', checked: mode === 'new', onchange: () => { mode = 'new'; showMode(); } }), 'Neuer Ordner')),
        existingBox, newBox,
        h('hr', { class: 'sep' }),
        sw(r.enabled, v => { r.enabled = v; }, 'Regel ist aktiv')),
      h('div', { class: 'modal-foot' }, h('button', { onclick: () => close(null) }, 'Abbrechen'), h('button', { class: 'primary', onclick: save }, 'Speichern')),
    ];
  }, { wide: true });
}

export async function mount(root, ctx) {
  let data = null;
  let dragID = null;
  const tipsBox = h('div');
  const listBox = h('div', { class: 'list' });
  root.append(tipsBox,
    h('div', { class: 'row' }, h('div', { class: 'grow muted' }, 'Die erste passende Regel bestimmt den Ordner. Regeln mit Mitspielern gehören deshalb vor die allgemeinen Playlist-Regeln.'),
      h('button', { class: 'primary', onclick: async () => { if (await editRule({}, data.playlists, data.players)) load(); } }, '+ Neue Regel')),
    listBox);

  async function order(ids) {
    await run(() => api('/api/rules/order', { method: 'POST', body: { ids } }));
    load();
  }

  function render() {
    fill(tipsBox, data.tips.length ? h('div', { class: 'card stack' },
      h('h3', null, 'Vorschläge aus deinem Einordnen'),
      data.tips.map(t => h('div', { class: 'row' }, h('span', { class: 'grow' }, t.text),
        h('button', { class: 'small primary', onclick: () => run(() => api('/api/rule-suggestions/accept', { method: 'POST', body: { key: t.key } }), 'Regel angelegt') }, 'Als Regel übernehmen'),
        h('button', { class: 'small ghost', onclick: () => run(() => api('/api/rule-suggestions/ignore', { method: 'POST', body: { key: t.key } })) }, 'Ignorieren')))) : '');
    const ids = data.rules.map(r => r.id);
    fill(listBox, ...(data.rules.length ? data.rules.map((r, i) => {
      const cond = [];
      if (r.playlist_txt?.length) cond.push(r.playlist_txt.join(', '));
      if (r.player_txt?.length) cond.push('mit ' + r.player_txt.join(r.all_players ? ' und ' : ' oder '));
      const row = h('div', { class: 'item', draggable: 'true', style: { gridTemplateColumns: 'auto auto 1fr auto', opacity: r.enabled ? 1 : .55 },
        ondragstart: e => { dragID = r.id; e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', String(r.id)); },
        ondragover: e => { if (dragID !== null) { e.preventDefault(); row.classList.add('selected'); } },
        ondragleave: () => row.classList.remove('selected'),
        ondrop: e => {
          e.preventDefault();
          row.classList.remove('selected');
          if (dragID === null || dragID === r.id) return;
          const next = ids.filter(id => id !== dragID);
          next.splice(next.indexOf(r.id), 0, dragID);
          dragID = null;
          order(next);
        } },
        h('span', { class: 'faint', style: { cursor: 'grab' }, title: 'Ziehen zum Umsortieren', 'aria-hidden': 'true' }, '⋮⋮'),
        sw(r.enabled, v => run(() => api('/api/rules/' + r.id, { method: 'PUT', body: { ...r, enabled: v } })).then(load)),
        h('div', { class: 'grow' }, h('div', null, cond.join(' · ') || '(ohne Bedingung)'),
          h('div', { class: 'muted', style: { fontSize: '12.5px' } }, '→ ', r.target || '(Ordner fehlt)', r.target_id ? '' : h('span', { class: 'faint' }, ' · wird beim ersten Treffer angelegt'))),
        h('div', { class: 'actions' },
          h('button', { class: 'icon-btn', disabled: i === 0, 'aria-label': 'Nach oben', onclick: () => { const n = [...ids]; [n[i - 1], n[i]] = [n[i], n[i - 1]]; order(n); } }, '↑'),
          h('button', { class: 'icon-btn', disabled: i === ids.length - 1, 'aria-label': 'Nach unten', onclick: () => { const n = [...ids]; [n[i + 1], n[i]] = [n[i], n[i + 1]]; order(n); } }, '↓'),
          h('button', { class: 'icon-btn', 'aria-label': 'Bearbeiten', onclick: async () => { if (await editRule(r, data.playlists, data.players)) load(); } }, '✎'),
          h('button', { class: 'icon-btn', 'aria-label': 'Löschen', onclick: async () => {
            if (await confirmDialog('Regel löschen', 'Die Regel wird gelöscht. Bereits einsortierte Replays bleiben, wo sie sind.', 'Löschen', true)) {
              await run(() => api('/api/rules/' + r.id, { method: 'DELETE' }), 'Regel gelöscht');
              load();
            }
          } }, '🗑')));
      return row;
    }) : [h('div', { class: 'empty' }, 'Noch keine Regeln. Ohne Regel landet jedes Replay im Eingang.')]));
  }

  async function load() {
    data = await api('/api/rules');
    render();
  }

  await load();
  ctx.on('rules', debounce(load, 200));
  ctx.on('groups', debounce(load, 400));
  return { refresh: load };
}
