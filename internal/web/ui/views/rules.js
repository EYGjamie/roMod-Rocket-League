import { api, h, ico, fill, run, flash, debounce, seg, sw, promptDialog, loadGroups, groupPath, folderOptions, plName } from '../lib.js';

const extra = ['hoops', 'ranked-hoops', 'rumble', 'ranked-rumble', 'dropshot', 'ranked-dropshot', 'snowday', 'ranked-snowday', 'heatseeker', 'dropshot-rumble', 'rocketlabs'];
function playlistGroups(playlists) {
  return [
    ['Ranked', playlists.filter(p => p.id.startsWith('ranked-') && !extra.includes(p.id))],
    ['Casual', playlists.filter(p => p.id.startsWith('unranked-'))],
    ['Extra', playlists.filter(p => extra.includes(p.id))],
    ['Sonstige', playlists.filter(p => ['tournament', 'private', 'season', 'offline'].includes(p.id))],
  ];
}
// chipName drops the group from the name: "Ranked 2s" is "2s" under Ranked.
const chipName = p => plName(p.id, p.name).replace(/^(Ranked|Casual) (?=\d)/, '');

export async function mount(root, ctx) {
  const page = h('div', { class: 'page', style: { gap: '0' } });
  root.append(page);
  let data = null;
  let groups = [];
  let editing = null; // index of the rule being edited, or 'new'
  let draft = null;
  let dragID = null;

  const blank = () => ({ enabled: true, playlists: [], players: [], all_players: false, target_id: '', target_name: '' });
  const edit = i => {
    const r = data.rules[i];
    editing = i;
    draft = { ...r, playlists: [...(r.playlists || [])], players: [...(r.players || [])] };
    render();
  };
  const closeEditor = () => { editing = null; render(); };

  async function save() {
    try {
      await api('/api/rules' + (draft.id ? '/' + draft.id : ''), { method: draft.id ? 'PUT' : 'POST', body: draft });
    } catch (e) {
      return flash(e.message, 'err');
    }
    flash('Regel gespeichert');
    editing = null;
    load();
  }

  async function remove() {
    if (draft.id && !await run(() => api('/api/rules/' + draft.id, { method: 'DELETE' }), 'Regel gelöscht, einsortierte Replays bleiben, wo sie sind')) return;
    closeEditor();
  }

  async function order(ids) {
    await run(() => api('/api/rules/order', { method: 'POST', body: { ids } }));
    load();
  }

  function ruleRow(r, i) {
    const el = h('div', { class: 'g-rule rule' + (r.enabled ? '' : ' off'), draggable: 'true', title: 'Klicken zum Bearbeiten, ziehen zum Umsortieren', onclick: () => edit(i),
      ondragstart: e => { dragID = r.id; e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', String(r.id)); },
      ondragover: e => { if (dragID !== null) { e.preventDefault(); el.classList.add('drop'); } },
      ondragleave: () => el.classList.remove('drop'),
      ondrop: e => {
        e.preventDefault();
        el.classList.remove('drop');
        if (dragID === null || dragID === r.id) return;
        const ids = data.rules.map(x => x.id).filter(id => id !== dragID);
        ids.splice(ids.indexOf(r.id), 0, dragID);
        dragID = null;
        order(ids);
      } },
      h('span', { class: 'n5' }, i + 1),
      h('span', { class: 'ell' }, r.playlists?.length ? r.playlists.map((id, k) => plName(id, r.playlist_txt?.[k])).join(', ') : 'jede Playlist'),
      h('span', { class: 'ell n4' }, (r.player_txt || []).join(r.all_players ? ' und ' : ' oder ')),
      h('span', { class: 'ell' }, r.target || '(Ordner fehlt)', r.target_id ? null : h('span', { class: 'n5' }, ' (neu)')),
      sw(r.enabled, v => run(() => api('/api/rules/' + r.id, { method: 'PUT', body: { ...r, enabled: v } })), 'Regel aktiv'));
    return el;
  }

  function editor(n) {
    const r = draft;
    const name = pid => data.players.find(p => p.pid === pid)?.name || pid;
    const free = data.players.filter(p => !r.players.includes(p.pid));
    return h('div', { class: 'editor' },
      h('div', { class: 'head tight' }, h('span', { class: 'n5' }, n), h('span', null, r.id ? 'Regel bearbeiten' : 'Neue Regel')),
      h('div', { class: 'form' },
        h('span', { class: 'lbl' }, 'Playlists'),
        h('div', { class: 'stack' }, playlistGroups(data.playlists).map(([label, list]) => h('div', { class: 'chips' }, h('span', { class: 'lbl' }, label),
          list.map(p => h('button', { class: 'chip', 'aria-pressed': String(r.playlists.includes(p.id)), title: p.name, onclick: () => {
            r.playlists = r.playlists.includes(p.id) ? r.playlists.filter(x => x !== p.id) : [...r.playlists, p.id];
            render();
          } }, chipName(p)))))),
        h('span', { class: 'lbl' }, 'Mitspieler'),
        h('div', { class: 'mates' },
          r.players.map(pid => h('button', { class: 'tag tag-accent', 'aria-label': name(pid) + ' entfernen', onclick: () => { r.players = r.players.filter(x => x !== pid); render(); } },
            name(pid), ico('x', 11))),
          free.length ? h('select', { 'aria-label': 'Mitspieler hinzufügen', onchange: e => { r.players = [...r.players, e.target.value]; render(); } },
            h('option', { value: '' }, '+ Mitspieler'), free.map(p => h('option', { value: p.pid }, `${p.name} (${p.games} Spiele)`))) : null,
          h('span', { class: 'end' }, seg([[false, 'einer reicht'], [true, 'alle']], !!r.all_players, v => { r.all_players = v; }, true))),
        h('span', { class: 'lbl', style: { paddingTop: '6px' } }, 'Ordner'),
        h('select', { class: 'input', style: { maxWidth: '320px' }, 'aria-label': 'Ordner', onchange: async e => {
          if (e.target.value === '+') {
            const n = await promptDialog('Neuer Ordner', 'Wird beim ersten passenden Replay auf oberster Ebene angelegt');
            if (n) Object.assign(r, { target_id: '', target_name: n });
            return render();
          }
          Object.assign(r, { target_id: e.target.value, target_name: groupPath(groups, e.target.value) });
        } },
          r.target_id ? null : h('option', { value: '', selected: true }, r.target_name ? r.target_name + ' (neu)' : 'Ordner wählen'),
          folderOptions(groups, r.target_id), h('option', { value: '+' }, 'Neuer Ordner …'))),
      h('div', { class: 'actions' },
        h('button', { class: 'btn btn-ghost danger', onclick: remove }, ico('trash'), 'Löschen'), h('span', { class: 'grow' }),
        h('button', { class: 'btn btn-secondary', onclick: closeEditor }, 'Schließen'),
        h('button', { class: 'btn btn-primary', onclick: save }, 'Speichern')));
  }

  function render() {
    fill(page,
      h('div', { class: 'head tight', style: { marginBottom: 'var(--space-3)' } }, h('h1', null, 'Regeln'),
        h('button', { class: 'btn btn-primary end', onclick: () => { editing = 'new'; draft = blank(); render(); } }, ico('plus'), 'Neue Regel')),
      data.tips.map(t => h('div', { class: 'tip-row' }, ico('sparkle'), h('span', { class: 'grow' }, 'Vorschlag: ' + t.text),
        h('button', { class: 'btn btn-ghost sm', onclick: () => run(() => api('/api/rule-suggestions/accept', { method: 'POST', body: { key: t.key } }), 'Regel angelegt') }, 'Übernehmen'),
        h('button', { class: 'btn btn-ghost sm n4', onclick: () => run(() => api('/api/rule-suggestions/ignore', { method: 'POST', body: { key: t.key } })) }, 'Ignorieren'))),
      h('div', { class: 'g-rule rule-h' }, ['#', 'Playlists', 'Mitspieler', 'Ordner', 'Aktiv'].map(t => h('span', null, t))),
      data.rules.map((r, i) => editing === i ? editor(i + 1) : ruleRow(r, i)),
      editing === 'new' ? editor(data.rules.length + 1) : null,
      data.rules.length || editing === 'new' ? null : h('div', { class: 'empty' }, 'Noch keine Regeln. Ohne Regel landet jedes Replay im Eingang.'));
  }

  async function load() {
    [data, { groups }] = await Promise.all([api('/api/rules'), loadGroups()]);
    if (typeof editing === 'number' && !data.rules.some(r => r.id === draft.id)) editing = null;
    else if (typeof editing === 'number') editing = data.rules.findIndex(r => r.id === draft.id);
    render();
  }

  await load();
  ctx.on('rules', debounce(load, 200));
  ctx.on('groups', debounce(load, 400));
  return { refresh: load };
}
