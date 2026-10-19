import { api, h, ico, fill, fmt, run, flash, sw, seg, debounce, visibilityNames, system, tierNames, uploadQuota } from '../lib.js';

// keyName turns a KeyboardEvent.code into a readable name.
export function keyName(code) {
  if (!code) return '–';
  const names = { Backspace: 'Rücktaste', Enter: 'Eingabe', Space: 'Leertaste', Tab: 'Tab', Delete: 'Entf', Insert: 'Einfg',
    Home: 'Pos1', End: 'Ende', PageUp: 'Bild ↑', PageDown: 'Bild ↓', ArrowUp: 'Pfeil ↑', ArrowDown: 'Pfeil ↓', ArrowLeft: 'Pfeil ←', ArrowRight: 'Pfeil →',
    ShiftLeft: 'Umschalt links', ShiftRight: 'Umschalt rechts', ControlLeft: 'Strg links', ControlRight: 'Strg rechts', AltLeft: 'Alt', AltRight: 'Alt Gr', CapsLock: 'Feststell' };
  if (names[code]) return names[code];
  if (code.startsWith('Key')) return code.slice(3);
  if (code.startsWith('Digit')) return code.slice(5);
  if (code.startsWith('Numpad')) return 'Num ' + code.slice(6);
  return code;
}

// keyCapture records a single key for the game's save key.
export function keyCapture(current, onChange, big) {
  let code = current;
  let listening = false;
  const btn = h('button', { class: 'keycap' + (big ? ' big' : '') });
  const show = () => { btn.textContent = listening ? 'Taste drücken … (Esc)' : keyName(code); btn.classList.toggle('listening', listening); };
  const onKey = e => {
    e.preventDefault();
    e.stopPropagation();
    stop();
    if (e.code !== 'Escape') { code = e.code; onChange(code); }
    show();
  };
  const stop = () => { listening = false; document.removeEventListener('keydown', onKey, true); };
  btn.onclick = () => { if (listening) { stop(); show(); return; } listening = true; show(); document.addEventListener('keydown', onKey, true); };
  btn.addEventListener('blur', () => { if (listening) { stop(); show(); } });
  show();
  return btn;
}

// hotkeyCapture records a combination like Ctrl+Alt+B; Entf clears it.
// meta names the Windows key: Win, or Super on Linux.
export function hotkeyCapture(current, onChange, meta = 'Win') {
  let value = current;
  let listening = false;
  const btn = h('button', { class: 'keycap' });
  const pretty = v => v ? v.replace('Ctrl', 'Strg').replace('Shift', 'Umschalt') : 'kein Hotkey';
  const show = () => { btn.textContent = listening ? 'Kombination drücken …' : pretty(value); btn.classList.toggle('listening', listening); };
  const onKey = e => {
    e.preventDefault();
    e.stopPropagation();
    if (e.code === 'Escape') { stop(); show(); return; }
    if (e.code === 'Delete' || e.code === 'Backspace') { stop(); value = ''; onChange(value); show(); return; }
    let key = '';
    if (e.code.startsWith('Key')) key = e.code.slice(3);
    else if (e.code.startsWith('Digit')) key = e.code.slice(5);
    else if (/^F\d{1,2}$/.test(e.code)) key = e.code;
    if (!key) return; // a modifier alone, wait for the key
    const mods = [e.ctrlKey && 'Ctrl', e.altKey && 'Alt', e.shiftKey && 'Shift', e.metaKey && meta].filter(Boolean);
    if (!mods.length && !key.startsWith('F')) { flash(`Bitte mit Strg, Alt, Umschalt oder ${meta} kombinieren`, 'err'); return; }
    stop();
    value = [...mods, key].join('+');
    onChange(value);
    show();
  };
  const stop = () => { listening = false; document.removeEventListener('keydown', onKey, true); };
  btn.onclick = () => { if (listening) { stop(); show(); return; } listening = true; show(); document.addEventListener('keydown', onKey, true); };
  btn.addEventListener('blur', () => { if (listening) { stop(); show(); } });
  show();
  return btn;
}

// testKeyButton presses the save key after a countdown; returns the button
// and its status line.
export function testKeyButton(big) {
  const out = h('span', { class: 'sm n4', role: 'status' });
  const btn = h('button', { class: 'btn btn-secondary' + (big ? ' big' : ''), onclick: async () => {
    btn.disabled = true;
    let left = 5;
    out.textContent = `Wechsle jetzt in ein Match … ${left}`;
    const t = setInterval(() => { left--; out.textContent = left > 0 ? `Wechsle jetzt in ein Match … ${left}` : 'Taste wird gedrückt …'; }, 1000);
    try {
      const r = await api('/api/test-key', { method: 'POST', body: { delay: 5 }, allowError: true });
      out.textContent = r.error ? r.error : 'Taste gedrückt, im Spiel sollte „Replay wird gespeichert“ stehen.';
    } catch (e) {
      out.textContent = e.message;
    } finally {
      clearInterval(t);
      btn.disabled = false;
    }
  } }, ico('play'), 'Testen');
  return [btn, out];
}

// tokenForm connects to ballchasing; onDone receives the account.
export function tokenForm(onDone, big) {
  const input = h('input', { class: 'input', type: 'password', placeholder: big ? 'Upload-Token' : 'Upload-Token von ballchasing.com/upload', autocomplete: 'off',
    'aria-label': 'Upload-Token', style: big ? { fontSize: '14px' } : null, onkeydown: e => { if (e.key === 'Enter') connect(); } });
  const btn = h('button', { class: 'btn btn-secondary' + (big ? ' big' : ''), style: { flex: 'none' }, onclick: () => connect() }, 'Verbinden');
  async function connect() {
    const token = input.value.trim();
    if (!token) return input.focus();
    btn.disabled = true;
    const me = await run(() => api('/api/token', { method: 'POST', body: { token } }), me => 'Angemeldet als ' + me.name);
    btn.disabled = false;
    if (me) { input.value = ''; onDone?.(me); }
  }
  return h('div', { class: 'ctl grow', style: { flexWrap: 'nowrap', gap: 'var(--space-2)' } }, input, btn);
}

// tokenHint says where the token comes from and how it is kept. Only
// Windows encrypts it (DPAPI); elsewhere the file permissions guard it.
export const tokenHint = sys => sys.windows ? 'Wird verschlüsselt gespeichert' : 'Nur für deinen Benutzer lesbar gespeichert';

// identityEditor shows and edits the user's player accounts.
export async function identityEditor(onChange) {
  const box = h('div');
  async function render() {
    const { current, candidates, players } = await api('/api/identities');
    const set = async pids => {
      await run(() => api('/api/identities', { method: 'PUT', body: { pids } }), 'Gespeichert');
      onChange?.();
      render();
    };
    const pids = current.map(c => c.pid);
    fill(box,
      current.map(c => h('div', { class: 'acct' }, h('span', null, c.name || c.pid), h('span', { class: 'sm n5 ell' }, c.pid),
        c.confirmed ? h('span', { class: 'tag tag-accent end' }, 'bestätigt') : h('button', { class: 'btn btn-ghost sm end', onclick: () => set(pids) }, 'Das bin ich'),
        h('button', { class: 'btn btn-ghost btn-icon n4', 'aria-label': 'Entfernen', title: 'Entfernen', onclick: () => set(pids.filter(p => p !== c.pid)) }, ico('x')))),
      !current.length ? h('div', { class: 'acct n4' }, candidates.length ? h('span', { class: 'grow' }, 'Vorschlag: ' + candidates.map(c => c.name).join(' und ')) : 'Wird nach den ersten Uploads erkannt.',
        candidates.length ? h('button', { class: 'btn btn-ghost sm', onclick: () => set(candidates.map(c => c.pid)) }, 'Das bin ich') : null) : null,
      players.some(p => !pids.includes(p.pid)) ? h('select', { class: 'input', style: { maxWidth: '320px', marginTop: 'var(--space-2)' }, 'aria-label': 'Account hinzufügen',
        onchange: e => { if (e.target.value) set([...pids, e.target.value]); } },
      h('option', { value: '' }, 'Account hinzufügen …'),
      players.filter(p => !pids.includes(p.pid)).map(p => h('option', { value: p.pid }, `${p.name} (${p.games} Spiele, ${p.pid})`))) : null);
  }
  await render();
  return box;
}

// saveKeyMissing explains why the save key is not pressed on this system.
export function saveKeyMissing(sys) {
  return h('div', { class: 'note' }, sys.mac
    ? 'Auf dem Mac drückt das Tool die Speichern-Taste nicht (macOS erlaubt das nur mit Bedienungshilfen-Rechten). '
    : 'Ohne X11 oder XWayland kann das Tool die Speichern-Taste nicht drücken. ',
  'Am Matchende ertönt ein Hinweiston: dann im Spiel „Replay speichern“ wählen, hochgeladen wird trotzdem automatisch.');
}

export async function mount(root, ctx) {
  const sys = system(ctx.state);
  let s = await api('/api/settings');
  let active = 0;
  let sections = [];
  const toc = h('nav', { class: 'toc', 'aria-label': 'Abschnitte' });
  const body = h('div', { class: 'sections', onscroll: () => {
    let a = 0;
    sections.forEach((el, i) => { if (el.offsetTop <= body.scrollTop + 40) a = i; });
    if (a !== active) { active = a; renderToc(); }
  } });
  root.append(h('div', { class: 'settings' }, toc, body));

  const save = async patch => {
    try {
      s = await api('/api/settings', { method: 'PUT', body: patch });
    } catch (e) {
      flash(e.message, 'err');
      render();
    }
  };
  const row = (label, hint, ...control) => h('div', { class: 'set' }, h('div', null, label, hint ? h('div', { class: 'hint' }, hint) : null), h('div', { class: 'ctl' }, control));
  const toggle = (label, on, set, hint) => h('div', { class: 'set' }, h('div', null, label),
    h('div', { class: 'ctl wide' }, sw(on, set, label), hint ? h('span', { class: 'hint' }, hint) : null));
  const option = (label, key, hint) => toggle(label, s[key], v => save({ [key]: v }), hint);
  const meta = sys.linux ? 'Super' : 'Win';

  function renderToc() {
    fill(toc, h('h1', null, 'Einstellungen'), sections.map((el, i) => h('button', { class: i === active ? 'on' : '', onclick: () => {
      body.scrollTo({ top: el.offsetTop - 24, behavior: 'smooth' });
      active = i;
      renderToc();
    } }, el.dataset.title)));
  }

  async function render() {
    const acc = s.account;
    const quota = uploadQuota(acc);
    const custom = s.demo_dirs.length > 0;
    const found = { windows: 'Steam, Epic', linux: 'Steam mit Proton, Heroic, Lutris, Bottles', darwin: 'CrossOver, Whisky, Heroic' }[sys.os] || 'Steam, Epic';
    const newDir = h('input', { class: 'input', type: 'text', placeholder: 'Anderen Ordner hinzufügen, z. B. ' + (sys.windows ? 'D:\\Replays' : '~/Replays'), 'aria-label': 'Replay-Ordner hinzufügen',
      onkeydown: e => {
        const p = e.target.value.trim();
        if (e.key === 'Enter' && p) save({ demo_dirs: [...(custom ? s.demo_dirs : s.dirs.map(d => d.path)), p] }).then(render);
      } });
    const section = (title, ...rows) => h('section', { 'data-title': title }, h('h3', null, title), rows);
    sections = [
      section('ballchasing',
        row('Verbindung', acc.from_env ? 'Der Token kommt aus der Umgebungsvariable BALLCHASING_TOKEN' : null,
          h('span', { class: 'ctl', style: { gap: '6px' } }, h('span', { class: 'led' + (acc.status === 'ok' ? '' : ' off') }),
            { ok: 'Angemeldet als ' + acc.name, none: 'nicht verbunden', invalid: 'Token ungültig', offline: 'ballchasing nicht erreichbar' }[acc.status]),
          acc.status === 'ok' && acc.tier ? h('span', { class: 'n4' }, (tierNames[acc.tier] || acc.tier) + (quota ? `, ${quota.used} / ${quota.max} Uploads in 24 h` : '')) : null),
        row(acc.status === 'none' ? 'Token eintragen' : 'Token ändern', tokenHint(sys), tokenForm(async () => { s = await api('/api/settings'); render(); })),
        row('Standard-Sichtbarkeit', 'Für neue Uploads', seg(Object.entries(visibilityNames), s.visibility, v => save({ visibility: v })))),
      section('Replays',
        h('div', { class: 'set top' }, h('div', null, 'Replay-Ordner', h('div', { class: 'hint' }, custom ? 'Eigene Auswahl' : `Automatisch erkannt (${found})`)),
          h('div', { class: 'dirs' },
            s.dirs.length ? null : h('div', { class: 'n4' }, 'Keiner gefunden. Bitte den Ordner TAGame/Demos des Spiels hinzufügen.'),
            s.dirs.map(d => h('div', null, h('span', { class: 'mono ell grow', title: d.path }, d.path),
              h('span', { class: 'n4 nowrap' }, d.exists ? fmt.num(d.replays) + ' Replays' : 'fehlt'),
              custom ? h('button', { class: 'btn btn-ghost btn-icon n4', 'aria-label': 'Entfernen', title: 'Entfernen', onclick: () => save({ demo_dirs: s.demo_dirs.filter(x => x !== d.path) }).then(render) }, ico('x')) : null)),
            newDir,
            custom ? h('button', { class: 'btn btn-ghost sm', style: { alignSelf: 'start' }, onclick: () => save({ demo_dirs: [] }).then(render) }, 'Wieder automatisch erkennen') : null)),
        option('Ältere Replays beim Start hochladen', 'upload_existing', 'Lädt beim Start nur registrierte Replays hoch'),
        option('Replays nach dem Upload löschen', 'delete_after_upload', 'In den Papierkorb, sobald ballchasing sie verarbeitet hat')),
      section('Spiel',
        sys.save_key ? option('Replays automatisch speichern', 'auto_mark', 'Drückt kurz nach dem ersten Anstoß die Speichern-Taste')
          : h('div', { class: 'set' }, h('div', { style: { gridColumn: '1 / -1' } }, saveKeyMissing(sys))),
        option('Nur bei Online-Spielen', 'online_only', 'Offline-Spiele werden ignoriert'),
        sys.save_key ? row('Speichern-Taste', 'Gesendet nur, wenn Rocket League im Vordergrund ist', keyCapture(s.mark_key, code => save({ mark_key: code })), ...testKeyButton()) : null,
        sys.save_key ? row('Haltedauer', null, h('input', { class: 'input num', type: 'number', min: 100, max: 5000, step: 100, value: s.mark_hold_ms, 'aria-label': 'Haltedauer in Millisekunden',
          onchange: e => save({ mark_hold_ms: Number(e.target.value) }) }), h('span', { class: 'n4' }, 'ms')) : null,
        row('Stats-API-Adresse', 'Muss zum Port in der TAStatsAPI.ini passen',
          h('input', { class: 'input', type: 'text', value: s.stats_api_addr, style: { width: '170px', minHeight: '30px', flex: 'none' }, 'aria-label': 'Stats-API-Adresse',
            onchange: e => save({ stats_api_addr: e.target.value.trim() }) }),
          h('span', { class: 'ctl sm n4', style: { gap: '6px' } }, h('span', { class: 'led' + (ctx.state.stats_api ? '' : ' off') }), ctx.state.stats_api ? 'verbunden' : 'nicht verbunden'))),
      section('Dein Spieler',
        h('div', { class: 'hint', style: { paddingBottom: 'var(--space-2)' } }, 'Damit erkennt das Tool Sieg und Niederlage. Mehrere Accounts (Steam und Epic) sind möglich.'),
        await identityEditor()),
      section('Hotkeys',
        sys.hotkeys ? [
          sys.linux ? h('div', { class: 'hint', style: { paddingBottom: 'var(--space-2)' } }, 'Unter Wayland wirken sie nur, solange ein X11-Fenster wie das Spiel im Vordergrund ist. Strg+Alt+F1 bis F12 wechselt unter Linux die Konsole.') : null,
          row(sys.overlay ? 'Overlay öffnen und schließen' : 'Oberfläche öffnen', null, hotkeyCapture(s.gui_hotkey, v => save({ gui_hotkey: v }), meta)),
          row('Pausieren und fortsetzen', 'Uploads und Auto-Speichern zusammen', hotkeyCapture(s.pause_hotkey, v => save({ pause_hotkey: v }), meta)),
          sys.quick_sort ? row('Schnell-Einordnen', 'Solange das Popup sichtbar ist', h('span', { class: 'keycap', style: { cursor: 'default' } }, 'Strg+Alt+1 … 4')) : null,
        ] : h('div', { class: 'hint' }, sys.mac
          ? 'Systemweite Hotkeys gibt es auf dem Mac nicht. Pausieren und die Oberfläche erreichst du über das Symbol in der Menüleiste.'
          : 'Systemweite Hotkeys brauchen eine X11-Sitzung oder XWayland. Pausieren und die Oberfläche erreichst du über das Symbol im Infobereich.')),
      section('Benachrichtigungen',
        option('Upload erfolgreich', 'notify_uploaded'),
        option('Upload fehlgeschlagen', 'notify_failed'),
        option('Session beendet', 'notify_session', 'Nach 30 Min. ohne Spiel oder wenn Rocket League schließt'),
        sys.quick_sort ? option(sys.overlay ? 'Schnell-Einordnen über dem Spiel' : 'Schnell-Einordnen', 'quick_sort', 'Aus: Regeln ordnen ohne Rückfrage ein') : null,
        sys.quick_sort ? row('Countdown', 'Sekunden bis der Regel-Ordner gilt', h('input', { class: 'input num', type: 'number', min: 3, max: 60, value: s.quick_sort_seconds,
          'aria-label': 'Countdown in Sekunden', onchange: e => save({ quick_sort_seconds: Number(e.target.value) }) })) : null),
      section('Start',
        sys.autostart ? toggle(sys.windows ? 'Mit Windows starten' : 'Bei der Anmeldung starten', s.autostart,
          v => run(() => api('/api/autostart', { method: 'POST', body: { on: v } })), sys.mac ? 'Erscheint unter Allgemein → Anmeldeobjekte' : null) : null,
        option('Minimiert starten', 'start_minimized', sys.tray ? `Nur das Symbol ${sys.mac ? 'in der Menüleiste' : 'im Infobereich'}` + (sys.overlay ? ', Overlay bleibt zu' : '') : 'Die Oberfläche öffnet sich nicht im Browser')),
      section('Erweitert',
        row('Log', h('span', { class: 'mono' }, s.log_path), h('button', { class: 'btn btn-secondary', onclick: () => run(() => api('/api/open-log', { method: 'POST' })) }, ico('file-text'), 'Log anzeigen')),
        row('Einrichtung', 'Den Assistenten noch einmal durchlaufen', h('a', { class: 'btn btn-secondary', href: '#/einrichtung' }, ico('magic-wand'), 'Assistent öffnen'))),
    ];
    const top = body.scrollTop;
    fill(body, sections);
    body.scrollTop = top;
    renderToc();
  }

  await render();
  ctx.on('settings', debounce(async () => { s = await api('/api/settings'); }, 300));
  return {};
}
