import { api, h, ico, fill, fmt, run, flash, seg, debounce, visibilityNames, groupPath, folderOptions, system, tierNames, plName } from '../lib.js';
import { tokenForm, tokenHint, identityEditor, keyCapture, testKeyButton, saveKeyMissing, statsAPIBox } from './settings.js';

const stepNames = ['ballchasing', 'Replay-Ordner', 'Rocket League', 'Upload & Ordner', 'Alte Replays'];

export async function mount(root, ctx) {
  const sys = system(ctx.state);
  let step = Number((() => { try { return sessionStorage.getItem('einrichtung'); } catch { return null; } })() || 0);
  let info = await api('/api/setup');
  let settings = await api('/api/settings');
  const { playlists } = await api('/api/rules');
  const rules = info.defaults.map(r => ({ ...r, on: true }));
  let backfill = '30';
  const box = h('div', { class: 'wizard' });
  const ids = h('div');
  const statsBox = statsAPIBox(sys);
  root.append(h('div', { class: 'wizard-page' }, box));

  const save = async patch => { settings = await run(() => api('/api/settings', { method: 'PUT', body: patch })) || settings; };

  async function finish() {
    await run(() => api('/api/setup/done', { method: 'POST' }));
    try { sessionStorage.removeItem('einrichtung'); } catch { /* private mode */ }
    location.hash = '#/uebersicht';
  }

  async function go(n) {
    step = n;
    try { sessionStorage.setItem('einrichtung', String(n)); } catch { /* private mode */ }
    [info, settings] = await Promise.all([api('/api/setup'), api('/api/settings')]);
    if (n === 2) statsBox.refresh();
    render();
  }

  async function showIDs() {
    fill(ids, h('div', { class: 'sm n4', style: { margin: 'var(--space-4) 0 var(--space-1)' } }, 'Bist du das? Damit zählen Sieg und Niederlage für dich.'), await identityEditor());
  }

  // steps: [question, body, next]
  const steps = [
    () => [info.account.status === 'ok' ? 'Mit ballchasing verbunden' : 'Mit ballchasing verbinden', [
      tokenForm(() => go(0), true),
      info.account.status === 'ok'
        ? h('span', { class: 'sm a3' }, 'Angemeldet als ' + info.account.name + (info.account.tier ? ', ' + (tierNames[info.account.tier] || info.account.tier) : ''))
        : h('span', { class: 'sm n5' }, 'Den Token findest du auf ', h('a', { href: 'https://ballchasing.com/upload', target: '_blank', rel: 'noopener noreferrer' }, 'ballchasing.com/upload'),
          ' (nach dem Anmelden mit Steam). ' + tokenHint(sys) + '. Die Upload-Menge hängt von deinem Patreon-Status bei ballchasing ab.'),
      info.account.status === 'ok' ? ids : null,
    ], () => go(1)],
    () => [info.dirs.length ? 'Wir haben diese Replay-Ordner gefunden' : 'Wo speichert Rocket League die Replays?', [
      info.dirs.map(d => h('div', { class: 'card-row' }, ico(d.exists ? 'check' : 'warning'), h('span', { class: 'mono ell', title: d.path }, d.path),
        h('span', { class: 'n4' }, d.exists ? fmt.num(d.replays) + ' Replays' : 'noch nicht vorhanden'))),
      info.dirs.length ? null : h('div', { class: 'note warn' }, 'Kein Replay-Ordner gefunden. Bitte den Ordner TAGame/Demos des Spiels hinzufügen; er erscheint, sobald das Spiel das erste Replay gespeichert hat.'),
      h('input', { class: 'input', type: 'text', placeholder: 'Anderen Ordner hinzufügen, z. B. ' + (sys.windows ? 'D:\\Replays' : '~/Replays'), 'aria-label': 'Replay-Ordner hinzufügen',
        onkeydown: async e => {
          const p = e.target.value.trim();
          if (e.key !== 'Enter' || !p) return;
          await save({ demo_dirs: [...(settings.demo_dirs.length ? settings.demo_dirs : info.dirs.map(d => d.path)), p] });
          go(1);
        } }),
    ], () => go(2)],
    () => [sys.save_key ? 'Rocket League verbinden und Speichern-Taste wählen' : 'Rocket League verbinden', [
      statsBox,
      sys.save_key ? h('div', { class: 'ctl' }, keyCapture(settings.mark_key, code => save({ mark_key: code }), true), ...testKeyButton(true)) : saveKeyMissing(sys),
      sys.save_key ? h('span', { class: 'sm n5' }, 'Kurz nach dem Anstoß drückt das Tool diese Taste einmal. Standard ist die Rücktaste.') : null,
    ], () => go(3)],
    () => ['Wohin sollen neue Uploads?', [
      h('div', { class: 'ctl', style: { gap: 'var(--space-4)', fontSize: '13px' } }, h('span', { class: 'grow' }, 'Sichtbarkeit'),
        seg(Object.entries(visibilityNames), settings.visibility, v => save({ visibility: v }))),
      info.groups.length || !info.syncing ? null : h('div', { class: 'sm n5' }, 'Deine Ordner werden gerade von ballchasing geladen …'),
      h('div', null, rules.map(r => h('div', { class: 'map-row' + (r.on ? '' : ' off') },
        h('input', { type: 'checkbox', checked: r.on, 'aria-label': 'Regel verwenden', onchange: e => { r.on = e.target.checked; e.target.parentNode.classList.toggle('off', !r.on); } }),
        h('span', { class: 'ell' }, r.playlists.map(id => plName(id, playlists.find(p => p.id === id)?.name)).join(', ')), ico('arrow-right'),
        h('select', { class: 'input', 'aria-label': 'Zielordner', onchange: e => { r.target_id = e.target.value; } },
          h('option', { value: '' }, r.target_name + ' (neu)'), folderOptions(info.groups, r.target_id))))),
      h('span', { class: 'sm n5' }, 'Ohne Haken landet die Playlist unter „Unsortiert“ im Verlauf. Neue Ordner werden beim ersten Spiel angelegt.'),
    ], async () => {
      const body = rules.filter(r => r.on).map(r => ({ enabled: true, playlists: r.playlists, target_id: r.target_id || '',
        target_name: r.target_id ? groupPath(info.groups, r.target_id) : r.target_name.trim() })).filter(r => r.target_id || r.target_name);
      if (await run(() => api('/api/setup/rules', { method: 'POST', body: { rules: body } }))) go(4);
    }],
    () => ['Sollen ältere Replays hochgeladen werden?', [
      [['none', 'Keine', 'Nur neue Spiele ab jetzt'], ['30', 'Die letzten 30 Tage', 'Wird im Hintergrund hochgeladen', info.old.last30],
        ['all', 'Alle', 'Kann das 24-h-Kontingent übersteigen, geht dann am nächsten Tag weiter', info.old.all]].map(([v, label, hint, n]) =>
        h('label', { class: 'radio-card' + (backfill === v ? ' on' : '') },
          h('input', { type: 'radio', name: 'old', checked: backfill === v, onchange: () => { backfill = v; render(); } }), h('span', { class: 'dot' }),
          h('span', null, label, h('span', { class: 'hint', style: { display: 'block' } }, hint)),
          h('span', { class: 'n4 num', style: { fontSize: '13px' } }, n === undefined ? '' : fmt.num(n)))),
      h('span', { class: 'sm n5' }, 'Diese lagen schon vor dem ersten Start im Ordner. Was bereits auf ballchasing ist, wird erkannt.'),
    ], async () => {
      if (backfill !== 'none') await run(() => api('/api/setup/backfill', { method: 'POST', body: { mode: backfill } }));
      await finish();
      flash('Einrichtung abgeschlossen');
    }],
  ];
  const next = () => steps[step]()[2]();

  function render() {
    const [question, body] = steps[step]();
    fill(box,
      h('div', { class: 'progress' }, stepNames.map((_, i) => h('span', { class: i <= step ? 'on' : '' }))),
      h('div', { class: 'wz-info' }, h('span', { class: 'grow' }, `Schritt ${step + 1} von 5, ${stepNames[step]}`), h('button', { class: 'link', onclick: finish }, 'Später einrichten')),
      h('h3', null, question),
      h('div', { class: 'stack' }, body),
      h('div', { class: 'wz-nav' },
        step ? h('button', { class: 'btn btn-ghost big n3', onclick: () => go(step - 1) }, 'Zurück') : null, h('span', { class: 'grow' }),
        h('button', { class: 'btn btn-primary big', onclick: next }, step === 4 ? 'Fertig' : 'Weiter', h('kbd', null, 'Enter'))));
    if (step === 0 && info.account.status === 'ok') showIDs();
  }

  const onKey = e => {
    if (e.key === 'Enter' && !e.target.closest('input, button, select, textarea, a') && !document.querySelector('.backdrop, .menu')) { e.preventDefault(); next(); }
  };
  document.addEventListener('keydown', onKey);
  render();
  let connected = ctx.state.stats_api;
  ctx.on('ui:state', debounce(() => { if (ctx.state.stats_api !== connected) { connected = ctx.state.stats_api; statsBox.refresh(); } }, 500));
  ctx.on('state', debounce(() => { if (step === 0 && info.account.status === 'ok') showIDs(); }, 1500));
  ctx.on('groups', debounce(() => { if (step === 3) go(3); }, 500));
  return { destroy: () => document.removeEventListener('keydown', onKey) };
}
