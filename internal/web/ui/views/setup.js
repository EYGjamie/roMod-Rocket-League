import { api, h, fill, run, sw, on, debounce, visibilityNames, groupPath } from '../lib.js';
import { tokenForm, identityEditor, keyCapture, testKeyButton } from './settings.js';
import { folderOptions } from './history.js';

export const title = 'Einrichtung';

const stepNames = ['ballchasing', 'Replay-Ordner', 'Speichern-Taste', 'Upload & Ordner', 'Alte Replays'];

export async function mount(root, ctx) {
  let step = Number((() => { try { return sessionStorage.getItem('einrichtung'); } catch { return null; } })() || 0);
  let info = await api('/api/setup');
  let settings = await api('/api/settings');
  const rules = info.defaults.map(r => ({ ...r, on: true }));
  let backfill = 'none';
  let off = null;

  const later = h('button', { class: 'ghost', onclick: finish }, 'Später einrichten');

  async function finish() {
    await run(() => api('/api/setup/done', { method: 'POST' }));
    try { sessionStorage.removeItem('einrichtung'); } catch { /* private mode */ }
    location.hash = '#/uebersicht';
  }

  function go(n) {
    step = n;
    try { sessionStorage.setItem('einrichtung', String(n)); } catch { /* private mode */ }
    render();
  }

  const nav = (next, label = 'Weiter') => h('div', { class: 'row', style: { marginTop: '16px' } },
    step > 0 ? h('button', { onclick: () => go(step - 1) }, 'Zurück') : null, h('span', { class: 'grow' }), later,
    h('button', { class: 'primary', onclick: next }, label));

  async function stepAccount() {
    const acc = (await api('/api/state')).account;
    const ids = h('div');
    const showIDs = async () => {
      fill(ids, h('h3', { style: { marginTop: '18px' } }, 'Bist du das?'),
        h('div', { class: 'muted', style: { margin: '6px 0 10px' } }, 'Aus deinen Uploads erkannt. Damit zählen Sieg und Niederlage für dich. Erscheint hier noch nichts, läuft der Abgleich mit ballchasing noch.'),
        await identityEditor());
    };
    if (acc.status === 'ok') showIDs();
    off?.();
    off = on('state', debounce(() => { if (step === 0) showIDs(); }, 1500));
    return [
      h('h2', null, 'Mit ballchasing verbinden'),
      h('p', { class: 'muted' }, 'Das Tool lädt deine Replays mit deinem Upload-Token hoch. Melde dich auf ballchasing.com mit Steam an und kopiere den Token von der Upload-Seite.'),
      acc.status === 'ok' ? h('div', { class: 'banner info' }, 'Verbunden als ' + acc.name + '.') : tokenForm(() => render()),
      ids,
      nav(() => go(1)),
    ];
  }

  async function stepFolders() {
    info = await api('/api/setup');
    const input = h('input', { type: 'text', placeholder: 'Anderen Ordner hinzufügen', style: { flex: '1' } });
    return [
      h('h2', null, 'Replay-Ordner'),
      h('p', { class: 'muted' }, 'Hier speichert Rocket League die Replays. Steam und Epic nutzen verschiedene Ordner, beide werden beobachtet.'),
      h('div', { class: 'list' }, info.dirs.map(d => h('div', { class: 'item', style: { gridTemplateColumns: '1fr auto' } },
        h('span', { class: 'ellipsis', title: d.path }, d.path), h('span', { class: 'muted' }, d.exists ? d.replays + ' Replays' : 'noch nicht vorhanden')))),
      h('div', { class: 'row nowrap', style: { marginTop: '10px' } }, input, h('button', { onclick: async () => {
        const p = input.value.trim();
        if (!p) return;
        settings = await run(() => api('/api/settings', { method: 'PUT', body: { demo_dirs: [...(settings.demo_dirs.length ? settings.demo_dirs : info.dirs.map(d => d.path)), p] } })) || settings;
        info = await api('/api/setup');
        render();
      } }, 'Hinzufügen')),
      nav(() => go(2)),
    ];
  }

  function stepKey() {
    const st = ctx.state;
    return [
      h('h2', null, 'Spiel und Speichern-Taste'),
      st.stats_api ? h('div', { class: 'banner info' }, 'Rocket League ist verbunden.') : h('div', { class: 'banner warn stack', style: { display: 'block' } },
        h('div', null, h('strong', null, 'Rocket League ist nicht verbunden.'), ' Läuft das Spiel, ist die Stats API wahrscheinlich aus.'),
        h('div', null, 'Spiel beenden, im Installationsordner TAGame\\Config\\TAStatsAPI.ini öffnen und eintragen:'),
        h('pre', { style: { margin: '6px 0', padding: '8px 10px', background: 'var(--bg)', borderRadius: '8px' } }, '[TAGame.MatchStatsExporter_TA]\nPacketSendRate=1\nPort=49123'),
        h('div', { class: 'muted' }, 'Die Datei liegt meist unter Program Files und braucht zum Speichern Administratorrechte.')),
      h('p', { class: 'muted' }, 'Kurz nach dem Anstoß drückt das Tool einmal die Taste, mit der das Spiel das Replay am Ende speichert (Standard: Rücktaste). Hast du sie im Spiel geändert, wähle hier dieselbe.'),
      h('div', { class: 'row' }, keyCapture(settings.mark_key, code => run(() => api('/api/settings', { method: 'PUT', body: { mark_key: code } }), 'Gespeichert'))),
      h('div', { style: { marginTop: '10px' } }, testKeyButton()),
      nav(() => go(3)),
    ];
  }

  async function stepRules() {
    info = await api('/api/setup');
    settings = await api('/api/settings');
    const groups = info.groups;
    const short = {
      'ranked-duels': 'Ranked 1s', 'ranked-doubles': 'Ranked 2s', 'ranked-standard': 'Ranked 3s', 'ranked-solo-standard': 'Ranked Solo 3s',
      'unranked-duels': 'Casual 1s', 'unranked-doubles': 'Casual 2s', 'unranked-standard': 'Casual 3s', 'unranked-chaos': 'Casual 4s',
    };
    const label = p => short[p] || p.replace('ranked-', 'Ranked ').replace(/^./, c => c.toUpperCase());
    const rows = rules.map(r => {
      const name = h('input', { type: 'text', value: r.target_name, hidden: !!r.target_id, 'aria-label': 'Name des neuen Ordners', style: { flex: '1' },
        oninput: e => { r.target_name = e.target.value; } });
      const folder = h('select', { 'aria-label': 'Zielordner', style: { maxWidth: '50%' },
        onchange: e => { r.target_id = e.target.value; name.hidden = !!r.target_id; } },
        folderOptions(groups, r.target_id || '', [['', 'Neuer Ordner:']]));
      return h('div', { class: 'item', style: { gridTemplateColumns: 'auto 1fr 1fr' } },
        h('input', { type: 'checkbox', checked: r.on, 'aria-label': 'Regel verwenden', onchange: e => { r.on = e.target.checked; } }),
        h('span', null, r.playlists.map(label).join(', ')),
        h('div', { class: 'row nowrap' }, folder, name));
    });
    return [
      h('h2', null, 'Upload und Ordner'),
      h('div', { class: 'setting' }, h('div', null, 'Sichtbarkeit neuer Uploads'),
        h('select', { onchange: e => run(() => api('/api/settings', { method: 'PUT', body: { visibility: e.target.value } })) },
          Object.entries(visibilityNames).map(([v, label]) => h('option', { value: v, selected: settings.visibility === v }, label)))),
      h('div', { class: 'setting' }, h('div', null, h('div', null, 'Replays nach dem Upload löschen'), h('div', { class: 'hint' }, 'In den Papierkorb, sobald ballchasing sie verarbeitet hat. Spart Platz, zum Anschauen lassen sie sich wieder ins Spiel laden.')),
        sw(settings.delete_after_upload, v => run(() => api('/api/settings', { method: 'PUT', body: { delete_after_upload: v } })))),
      h('h3', { style: { marginTop: '16px' } }, 'Regeln: wohin welche Playlist kommt'),
      h('p', { class: 'muted' }, 'Wähle für jede Playlist einen bestehenden Ordner oder lass einen neuen anlegen (passiert beim ersten Spiel). Ohne Haken landet die Playlist im Eingang.'),
      groups.length ? null : h('div', { class: 'faint' }, info.syncing ? 'Deine Ordner werden gerade von ballchasing geladen …' : ''),
      h('div', { class: 'list' }, rows),
      nav(async () => {
        const body = rules.filter(r => r.on).map(r => ({ enabled: true, playlists: r.playlists, target_id: r.target_id || '', target_name: r.target_id ? groupPath(groups, r.target_id) : r.target_name.trim() }))
          .filter(r => r.target_id || r.target_name);
        if (await run(() => api('/api/setup/rules', { method: 'POST', body: { rules: body } }), `${body.length} Regeln angelegt`)) go(4);
      }),
    ];
  }

  async function stepOld() {
    info = await api('/api/setup');
    const opt = (v, label, n) => h('label', { class: 'check', style: { display: 'flex', padding: '8px 0' } },
      h('input', { type: 'radio', name: 'alt', checked: backfill === v, onchange: () => { backfill = v; } }), h('span', null, label, n !== undefined ? h('span', { class: 'muted' }, ` (${n} Replays)`) : null));
    return [
      h('h2', null, 'Alte Replays nachladen'),
      h('p', { class: 'muted' }, 'Replays, die schon vor dem ersten Start im Ordner lagen, wurden nur registriert. Sollen sie jetzt hochgeladen werden? Was schon auf ballchasing ist, erkennt ballchasing selbst.'),
      opt('none', 'Keine'), opt('30', 'Die letzten 30 Tage', info.old.last30), opt('all', 'Alle', info.old.all),
      nav(async () => {
        if (backfill !== 'none') await run(() => api('/api/setup/backfill', { method: 'POST', body: { mode: backfill } }), r => `${r.queued} Replays werden hochgeladen`);
        finish();
      }, 'Fertig'),
    ];
  }

  async function render() {
    const builders = [stepAccount, stepFolders, stepKey, stepRules, stepOld];
    const body = await builders[step]();
    fill(root, 
      h('div', { class: 'steps' }, stepNames.map((n, i) => h('span', { class: i === step ? 'active' : i < step ? 'done' : '' }, `${i + 1}. ${n}`))),
      h('div', { class: 'card', style: { maxWidth: '820px' } }, body));
  }

  await render();
  ctx.on('ui:state', debounce(() => { if (step === 2) render(); }, 500));
  ctx.on('groups', debounce(async () => { if (step === 3) { info = await api('/api/setup'); render(); } }, 500));
  return { destroy: () => off?.() };
}
