# auto-replay-upload

Lädt Rocket-League-Replays automatisch auf [ballchasing.com](https://ballchasing.com) hoch, sortiert sie in Ordner und zeigt Verlauf und Statistik, auch mit Easy Anti-Cheat und ohne BakkesMod. Läuft unter **Windows, macOS und Linux**.

Seit Easy Anti-Cheat (April 2026) läuft das BakkesMod-Plugin „AutoReplayUploader“ online nicht mehr. Dieses Tool greift nicht ins Spiel ein:

1. **Match erkennen** über die offizielle [Stats API](https://www.rocketleague.com/developer/stats-api) des Spiels (lokaler TCP-Port).
2. **Replay vormerken:** Kurz nach dem ersten Anstoß hält das Tool einmal die Speichern-Taste (Standard: Rücktaste), mit der das Spiel das Replay am Matchende automatisch speichert.
3. **Hochladen:** Neue `.replay`-Dateien im Replay-Ordner gehen über die ballchasing-API hoch, auf Wunsch landet die Datei danach im Papierkorb.
4. **Einsortieren:** Regeln (nach Playlist und Mitspielern) legen das Replay in einen ballchasing-Ordner. Ein Hinweis zeigt das Ziel mit Countdown, Strg+Alt+1–4 wählt eine Alternative.

Dazu kommt eine Oberfläche für Übersicht, Verlauf mit allen Statistiken, Statistik-Diagramme, Ordner, Eingang und Regeln. Unter Windows öffnet sie sich als Overlay über dem Spiel (Strg+Alt+B), überall im Browser. Das Tool läuft im Hintergrund, startet bei der Anmeldung und zeigt sich nur als Symbol im Infobereich (macOS: in der Menüleiste).

Das Konzept mit allen Abläufen und Entscheidungen steht in [docs/konzept.md](docs/konzept.md).

## Systeme

Rocket League gibt es seit 2020 nur noch für Windows. Unter Linux läuft es über Proton (Steam) oder Wine (Heroic, Lutris, Bottles), auf dem Mac über CrossOver oder Whisky. Ob Online-Spiele dort mit Easy Anti-Cheat laufen, entscheidet Psyonix; hochladen, einordnen und auswerten klappt in jedem Fall, auch mit Replays, die von einem anderen Rechner stammen.

| Funktion | Windows | macOS | Linux |
|---|---|---|---|
| Hochladen, Einordnen, Verlauf, Statistik, Regeln | ✓ | ✓ | ✓ |
| Replay-Ordner automatisch erkannt | Steam, Epic | CrossOver, Whisky, Heroic | Steam mit Proton (auch Flatpak, Snap, weitere Bibliotheken), Heroic, Lutris, Bottles, Wine |
| Symbol | Infobereich der Taskleiste | Menüleiste | Infobereich der Leiste |
| Oberfläche | Overlay über dem Spiel und Browser | Browser | Browser |
| Start bei der Anmeldung | ✓ | ✓ (Anmeldeobjekt) | ✓ (Autostart des Desktops) |
| Speichern-Taste | ✓ | – (Hinweiston am Matchende) | ✓ unter X11 und XWayland |
| Hotkeys | ✓ | – | ✓ unter X11 und XWayland |
| Schnell-Einordnen | Fenster über dem Spiel | – (Regeln ordnen direkt ein) | Benachrichtigung mit Knöpfen |
| Hinweise | eigene Fenster über dem Spiel | Mitteilungszentrale | Benachrichtigungen des Desktops |
| Löschen nach Upload in den Papierkorb | ✓ | ✓ | ✓ |
| Token | mit DPAPI verschlüsselt | Datei nur für dich lesbar | Datei nur für dich lesbar |

Unter Wayland laufen Spiele mit Proton über XWayland. Hotkeys und Speichern-Taste wirken dann, solange das Spiel (oder ein anderes X11-Fenster) im Vordergrund ist, also genau beim Spielen.

## Voraussetzungen

- ein Konto auf [ballchasing.com](https://ballchasing.com) und dessen Upload-Token
- [Go](https://go.dev/dl/) 1.26.2 oder neuer, um das Programm zu bauen
- **Windows:** Windows 10 oder 11 mit der WebView2-Laufzeit für das Overlay (bei Windows 10 und 11 vorinstalliert)
- **macOS:** macOS 12 oder neuer und die Xcode Command Line Tools (`xcode-select --install`), die das Symbol in der Menüleiste braucht
- **Linux:** ein Desktop mit Infobereich nach StatusNotifierItem (KDE Plasma, Xfce, Cinnamon, MATE, Budgie; GNOME mit der Erweiterung [AppIndicator and KStatusNotifierItem Support](https://extensions.gnome.org/extension/615/appindicator-support/), bei Ubuntu vorinstalliert). Zum Bauen ist nichts weiter nötig.

## Installation

### 1. Programm bauen

```bash
git clone https://github.com/EYGjamie/auto-replay-upload.git
cd auto-replay-upload
```

**Windows**

```bash
go build -trimpath -ldflags "-s -w -H windowsgui" -o dist/auto-replay-upload.exe ./cmd/auto-replay-upload
```

`-H windowsgui` sorgt dafür, dass kein Konsolenfenster aufgeht. Symbol, Programmname und Manifest kommen aus den `rsrc_windows_*.syso` in `cmd/auto-replay-upload`, die `go build` von selbst einbindet. Die fertige `dist/auto-replay-upload.exe` in einen eigenen Ordner kopieren, zum Beispiel `C:\Tools\auto-replay-upload\`. Dort legt das Tool auch Konfiguration, Datenbank und Log ab. Der Autostart zeigt auf genau diese exe; verschiebst du den Ordner, einfach einmal von dort starten.

**macOS**

```bash
scripts/macos-app.sh
```

Das Skript baut `dist/auto-replay-upload.app`, eine Menüleisten-App ohne Symbol im Dock. Nach `/Programme` (oder `~/Programme`) ziehen und per Doppelklick starten. Konfiguration, Datenbank und Log liegen unter `~/Library/Application Support/auto-replay-upload/`.

**Linux**

```bash
go build -trimpath -ldflags "-s -w" -o dist/auto-replay-upload ./cmd/auto-replay-upload
install -D dist/auto-replay-upload ~/.local/bin/auto-replay-upload
```

Dann `auto-replay-upload` starten (aus dem Terminal oder per Alt+F2). Konfiguration, Datenbank und Log liegen unter `~/.config/auto-replay-upload/`.

Windows- und Linux-Programme lassen sich auf jedem System bauen, etwa mit `GOOS=windows GOARCH=amd64` vor dem `go build`. Das Mac-Programm mit Symbol in der Menüleiste braucht cgo und wird deshalb auf einem Mac gebaut; ohne cgo (`CGO_ENABLED=0 GOOS=darwin`) läuft es ohne Symbol. Fertige Programme für alle drei Systeme baut auch die CI bei jedem Push (Actions → Artefakte).

### 2. Stats API aktivieren

Das erledigt die Einrichtung (Schritt „Rocket League“) mit einem Klick auf **„Stats API einschalten“**: Das Tool sucht das Spiel in allen Steam-Bibliotheken, beim Epic Games Launcher, bei Heroic und Legendary sowie in den Wine-Präfixen und schaltet die API in jeder gefundenen Installation ein. Liegt das Spiel unter `Program Files`, fragt Windows einmal nach Administratorrechten. Läuft Rocket League gerade, wirkt die Änderung nach dem nächsten Start des Spiels. Dieselbe Anzeige steht unter Einstellungen → Spiel, etwa wenn ein Spielupdate die Datei zurückgesetzt hat. Das meldet das Tool auch selbst, sobald Rocket League eine Minute ohne Stats API läuft (siehe [Fehlerbehebung](#fehlerbehebung)). Mit **„Stats API beim Start einschalten“** prüft das Tool das bei jedem Start und schaltet die API selbst wieder ein.

Von Hand: Rocket League beenden. Im Installationsordner des Spiels die Datei `TAGame/Config/TAStatsAPI.ini` öffnen (gibt es sie nicht, dann `DefaultStatsAPI.ini`) und im Abschnitt `[TAGame.MatchStatsExporter_TA]` setzen:

```ini
[TAGame.MatchStatsExporter_TA]
PacketSendRate=1
Port=49123
```

Bei `PacketSendRate=0` (Standard) ist die API aus. Übliche Installationsordner:

- Windows, Steam: `C:\Program Files (x86)\Steam\steamapps\common\rocketleague`
- Windows, Epic: `C:\Program Files\Epic Games\rocketleague`
- Linux, Steam: `~/.local/share/Steam/steamapps/common/rocketleague` (oder die Steam-Bibliothek, in der das Spiel liegt)
- Linux, Heroic: der in Heroic gewählte Ordner, Standard `~/Games/Heroic/rocketleague`
- macOS: in der Flasche von CrossOver oder Whisky unter `drive_c`, etwa `drive_c/Program Files/Epic Games/rocketleague`

Unter Windows liegt die Datei meist unter `Program Files`; der Editor muss dann als Administrator laufen. Spielupdates können sie zurücksetzen, also nach einem Update prüfen.

Unter Windows Rocket League für das Overlay und das Schnell-Einordnen im Anzeigemodus **„Randloses Fenster“** spielen. Im echten Vollbild kann kein anderes Fenster darüber liegen.

### 3. Erst-Einrichtung

Das Programm starten. Beim ersten Start öffnet sich der Assistent (unter Windows im Overlay, sonst im Browser):

1. **ballchasing verbinden:** Upload-Token von [ballchasing.com/upload](https://ballchasing.com/upload) einfügen, danach bestätigen, welcher Spieler du bist (mehrere Accounts wie Steam und Epic sind möglich).
2. **Replay-Ordner:** Erkannt werden unter Windows `Dokumente\My Games\Rocket League\TAGame\Demos` und `DemosEpic`, unter Linux und macOS derselbe Ordner im Präfix von Proton oder Wine (etwa `…/steamapps/compatdata/252950/pfx/drive_c/users/steamuser/Documents/My Games/Rocket League/TAGame/Demos`). Eigene Ordner lassen sich ergänzen, `~` steht für dein Benutzerverzeichnis.
3. **Speichern-Taste:** die Taste wählen, die im Spiel unter Einstellungen → Steuerung fürs Replay-Speichern eingestellt ist, und mit „Testen“ ausprobieren.
4. **Upload und Ordner:** Sichtbarkeit, Löschen nach Upload, und welche Playlist in welchen Ordner kommt (bestehender Ordner oder ein neuer).
5. **Alte Replays nachladen:** keine, die letzten 30 Tage oder alle.

„Später einrichten“ geht auf jeder Seite. Alles lässt sich danach in den Einstellungen ändern.

Wer von einer älteren Version umsteigt, behält `config.json` und den Upload-Status: Die alte `uploaded.json` wird beim ersten Start in die Datenbank übernommen (danach `uploaded.json.bak`).

## Benutzung

### Im Hintergrund

Startest du das Programm selbst (Doppelklick auf die exe), öffnet sich gleich die Oberfläche; ein Konsolenfenster gibt es nicht. Läuft es schon, holt ein weiterer Start die Oberfläche des laufenden Programms nach vorn. Bei der Anmeldung startet es dagegen nur mit dem Symbol, solange „Bei der Anmeldung minimiert starten“ an ist: unter Windows im Infobereich der Taskleiste (unter Windows 11 zunächst bei den **ausgeblendeten Symbolen**, Pfeil **^** neben der Uhr), unter macOS in der Menüleiste, unter Linux im Infobereich der Leiste. Grau heißt pausiert, ein roter Punkt heißt, dass Uploads fehlgeschlagen sind.

Unter Windows öffnet ein **Linksklick** das Overlay und ein **Rechtsklick** das Menü, unter Linux ebenso, sofern die Leiste das unterscheidet. Auf dem Mac öffnet jeder Klick das Menü:

| Eintrag | Funktion |
|---|---|
| Pausieren / Fortsetzen | Uploads und Auto-Speichern anhalten (auch Strg+Alt+P) |
| Replays hochladen … | Dateien auswählen und hochladen |
| Replay-Ordner öffnen | im Explorer, Finder oder Dateimanager |
| Overlay öffnen | Windows: die Oberfläche über dem Spiel (auch Strg+Alt+B); macOS und Linux: „Oberfläche öffnen“ im Browser |
| Im Browser öffnen | Windows: dieselbe Oberfläche im Standardbrowser |
| Einstellungen | die Oberfläche mit den Einstellungen |
| Beenden | bis zum nächsten Start |

### Nach jedem Match

1. Das Spiel speichert das Replay (dank der vorgemerkten Speichern-Taste), das Tool lädt es hoch und wartet, bis ballchasing es verarbeitet hat.
2. Passt eine Regel, erscheint das **Schnell-Einordnen**: Zielordner, Countdown (Standard 10 s) und bis zu drei Alternativen. **Strg+Alt+1–3** wählt eine Alternative, **Strg+Alt+4** schickt das Replay in den Eingang; läuft der Countdown ab, gilt der Regel-Ordner.
   - Windows: ein kleines Fenster oben rechts über dem Spiel, das dem Spiel nie den Fokus nimmt.
   - Linux: eine Benachrichtigung mit Knöpfen. Benachrichtigungen zeigen höchstens drei Knöpfe; bei drei Alternativen bleibt die dritte bei Strg+Alt+3.
   - macOS: Mitteilungen haben keine Knöpfe, die Regel ordnet deshalb sofort ein.
3. Ohne passende Regel landet das Replay im **Eingang**.
4. Eine kurze **Benachrichtigung** sagt, wohin es ging. Benachrichtigungen werden zurückgehalten, solange ein Match läuft. Nach 30 Minuten ohne Spiel (oder wenn Rocket League geschlossen wird) gibt es eine Zusammenfassung der Session.

Hat das Vormerken nicht geklappt (zum Beispiel weil ein anderes Fenster im Vordergrund war, oder auf dem Mac immer), gibt es am Matchende einen Warnton. Dann im Spiel „Replay speichern“ wählen; der Upload passiert trotzdem automatisch.

### Die Oberfläche

| Bereich | Inhalt |
|---|---|
| Übersicht | Warteschlange und letzte Uploads in einer Tabelle (Alle, Wartet, Fehler, Hochgeladen, Ignoriert), Pause, fehlgeschlagene Uploads erneut senden oder dauerhaft ignorieren, Replays hochladen (auch per Drag & Drop auf die Seite). Die Statusleiste unten zeigt das Upload-Kontingent, Probleme mit ballchasing und die Pause |
| Eingang | alles ohne Ordner, mit Vorschlag pro Replay und Ordnerauswahl je Zeile; alle Vorschläge auf einmal übernehmen oder eine Auswahl in einen Ordner verschieben |
| Verlauf | alle Replays deines Kontos und die Spiele mit dir, die Mitspieler hochgeladen haben (Duplikate nur einmal), nach Session oder als Liste. Suche nach Map, Playlist oder Mitspieler; eine Session auf einmal verschieben |
| Replay-Details | Überblick mit Spielern und Ballbesitz, dazu Kern, Boost, Bewegung, Position und Demos im Vergleich mit Lobby, Mitspielern oder Gegnern (relativ oder absolut) sowie Kamera & Auto. Ins Spiel laden, Sichtbarkeit, Verschieben und Löschen |
| Statistik | 7 Tage, 14 Tage, Monat oder Gesamt: Spiele, Siegquote, Bilanz, Spiele pro Tag, Verteilung nach Playlist, beste Maps |
| Ordner | der ballchasing-Ordnerbaum mit Anzahl und Filter, neue Ordner und Unterordner, Replays per Auswahl oder Drag & Drop verschieben und ihre Sichtbarkeit ändern |
| Regeln | Regeln nach Playlist und Mitspielern, direkt in der Liste bearbeiten, Reihenfolge per Ziehen; die erste passende gewinnt. Vorschläge für neue Regeln aus deinem Einordnen |
| Einstellungen | alles aus der Einrichtung; jede Änderung gilt sofort. Was ein System nicht kann (etwa Hotkeys auf dem Mac), blendet die Oberfläche aus und sagt warum |

Das Overlay unter Windows lässt sich an freien Stellen der Symbolleiste links oder der Statusleiste unten mit der Maus verschieben. Es öffnet sich danach wieder an dieser Stelle, bis das Programm neu startet.

Replays, die Mitspieler hochgeladen haben, lassen sich nur ansehen und ins Spiel laden. Verschieben, Sichtbarkeit und Löschen erlaubt die ballchasing-API nur beim eigenen Upload.

Aktionen, die auf ballchasing warten (Verschieben, Sichtbarkeit, Löschen, Ordner anlegen, Abgleich, Token prüfen, Statistiken und Replays laden), zeigen unten rechts eine Info-Karte, bis ballchasing geantwortet hat.

Playlists zeigt die Oberfläche so, wie es sie im Spiel noch gibt: Solo Standard gibt es nicht mehr, und Hoops, Rumble, Dropshot und Snow Day nur noch als Ranked. Ältere Replays aus diesen Playlists zählen überall als ihr Nachfolger, also als Ranked 3s oder als die Ranked-Variante; Regeln, Vorschläge und Statistik behandeln beide gleich.

### Ins Spiel laden

Ist ein Replay nicht mehr lokal vorhanden (etwa weil „nach dem Upload löschen“ an ist), holt **„Ins Spiel laden“** im Menü eines Replays die Datei von ballchasing in den Replay-Ordner. Im Spiel unter „Replays“ öffnen (eventuell das Menü einmal neu öffnen). Sobald du das Replay im Spiel verlässt, löscht das Tool die Datei wieder, außer du wählst „Geladene Datei behalten“. ballchasing erlaubt ohne Patreon 200 Downloads pro Stunde.

### Löschen nach dem Upload

Ist es eingeschaltet, wandert eine Datei in den **Papierkorb**, sobald ballchasing sie verarbeitet hat, und nur wenn alles davon zutrifft: dein eigener Upload, Status `ok`, aus dem Replay-Ordner (nicht von Hand hochgeladen), keine fürs Anschauen geladene Datei. Altdateien, die nie hochgeladen wurden, bleiben liegen.

Unter Linux geht die Datei in den Papierkorb nach freedesktop.org-Standard, aus dem der Dateimanager sie wiederherstellen kann; liegt sie auf einem anderen Laufwerk (etwa einer zweiten Steam-Bibliothek), in den Papierkorb dieses Laufwerks. Auf dem Mac fragt der Finder beim ersten Mal eventuell, ob das Tool ihn steuern darf. Klappt das Verschieben nicht, bleibt die Datei liegen; endgültig gelöscht wird nie.

## Konfiguration

Alles lässt sich in der Oberfläche einstellen. Gespeichert wird in `config.json`:

- **Windows:** neben der exe
- **macOS:** `~/Library/Application Support/auto-replay-upload/config.json`
- **Linux:** `~/.config/auto-replay-upload/config.json` (genauer `$XDG_CONFIG_HOME`)

Liegt unter macOS oder Linux eine `config.json` neben dem Programm, gilt diese (alles in einem Ordner). Mit `-config` lässt sich jeder andere Ort wählen:

```bash
auto-replay-upload -config ~/rl/config.json
```

| Feld | Standard | Bedeutung |
|---|---|---|
| `ballchasing_token_protected` | – | Upload-Token. Unter Windows mit DPAPI für deinen Benutzer verschlüsselt, unter macOS und Linux nur kodiert; die Datei kann dort nur dein Benutzer lesen. Die Umgebungsvariable `BALLCHASING_TOKEN` hat Vorrang |
| `visibility` | `private` | Sichtbarkeit neuer Uploads: `public`, `unlisted`, `private` |
| `demo_dirs` | `[]` | Replay-Ordner; leer = automatisch. `~` am Anfang steht für dein Benutzerverzeichnis |
| `upload_existing` | `false` | beim Start noch nicht hochgeladene Altdateien hochladen |
| `delete_after_upload` | `false` | Dateien nach dem Upload in den Papierkorb |
| `stats_api_addr` | `127.0.0.1:49123` | Adresse der Stats API (muss zu `Port` in der ini passen) |
| `stats_api_auto` | `false` | beim Start die Stats API in der ini des Spiels einschalten, falls ein Spielupdate sie ausgeschaltet hat |
| `auto_mark` | `true` | Speichern-Taste nach dem ersten Anstoß drücken |
| `online_only` | `true` | nur Online-/LAN-Matches vormerken |
| `mark_key` | `Backspace` | Speichern-Taste als [KeyboardEvent.code](https://developer.mozilla.org/docs/Web/API/KeyboardEvent/code) |
| `mark_hold_ms` | `1200` | wie lange die Taste gehalten wird (100 bis 5000) |
| `gui_hotkey` | `Ctrl+Alt+B` | Overlay (oder die Oberfläche) öffnen; leer = kein Hotkey |
| `pause_hotkey` | `Ctrl+Alt+P` | pausieren und fortsetzen |
| `notify_uploaded`, `notify_failed`, `notify_session` | `true` | Benachrichtigungen |
| `quick_sort` | `true` | Schnell-Einordnen nach dem Upload; aus = Regeln ordnen ohne Rückfrage ein |
| `quick_sort_seconds` | `10` | Countdown (3 bis 60) |
| `start_minimized` | `true` | beim Start durch die Anmeldung nur das Symbol zeigen; ein Start von Hand öffnet die Oberfläche immer |
| `setup_done` | `false` | Assistent erledigt |

Hotkeys bestehen aus `Ctrl`/`Strg`, `Alt`, `Shift`, `Win` (unter Linux auch `Super`) plus Buchstabe, Ziffer oder `F1`–`F24`.

Weitere Dateien neben der `config.json`:

- `auto-replay-upload.db`: Datenbank mit Upload-Status, Replays, Ordnern, Regeln und Entscheidungen
- `auto-replay-upload.log`: das Log (über 1 MB wird es beim Start zu `.log.old`), in der Oberfläche unter Einstellungen → Log anzeigen
- `manuell/`: Kopien von Replays, die über die Oberfläche hochgeladen wurden
- `instance.json`: solange das Tool läuft, die Adresse seiner Oberfläche, damit ein zweiter Start sie öffnen kann

Außerdem je System:

- **Windows:** `%LOCALAPPDATA%\auto-replay-upload\WebView2` (Browser-Cache des Overlays), der Autostart-Eintrag unter `HKEY_CURRENT_USER\…\CurrentVersion\Run` und die gemerkte Autostart-Wahl unter `HKEY_CURRENT_USER\Software\auto-replay-upload`
- **macOS:** der Autostart als `~/Library/LaunchAgents/com.github.eygjamie.auto-replay-upload.plist` (in den Systemeinstellungen unter Allgemein → Anmeldeobjekte), die gemerkte Wahl in `~/Library/Application Support/auto-replay-upload/autostart`
- **Linux:** der Autostart als `~/.config/autostart/auto-replay-upload.desktop`, die gemerkte Wahl in `~/.config/auto-replay-upload/autostart`

Wer den Autostart in den Einstellungen des Systems entfernt oder abschaltet, behält das so; einschalten geht wieder in der Oberfläche.

**Entfernen:** In den Einstellungen „Mit Windows starten“ bzw. „Bei der Anmeldung starten“ ausschalten, über das Symbol beenden und das Programm sowie den Ordner mit der `config.json` löschen.

## Fehlerbehebung

Meldungen stehen im Log, die letzte auch im Tooltip des Symbols. Fehler, wegen derer das Tool nicht starten kann, erscheinen als Meldungsfenster (unter Linux mit zenity oder kdialog, falls installiert).

| Problem | Ursache und Lösung |
|---|---|
| „Rocket League nicht verbunden“, obwohl das Spiel läuft | Stats API aus oder anderer Port. Unter Einstellungen → Spiel „Stats API einschalten“ (siehe [Installation](#2-stats-api-aktivieren)), danach das Spiel neu starten. |
| Benachrichtigung „Stats API ist aus“ | Rocket League läuft seit einer Minute, ohne dass sich das Tool verbindet, und in der ini einer Installation ist die Stats API aus, meist weil ein Spielupdate die Datei zurückgesetzt hat. „Einschalten“ schaltet sie wieder ein (unter `Program Files` nach der Abfrage von Windows), danach das Spiel neu starten; „Einstellungen“ zeigt die gefundenen Dateien. Der Hinweis kommt höchstens einmal pro Spielstart und nie während eines Matches. Unter macOS hat die Mitteilung keine Knöpfe, dann Einstellungen → Spiel. |
| „Token ungültig“ | Neuen Token auf [ballchasing.com/upload](https://ballchasing.com/upload) erzeugen und in den Einstellungen eintragen. Uploads warten so lange. |
| „ballchasing nicht erreichbar“, Uploads warten | Internet oder ballchasing gestört. Die Uploads laufen von selbst weiter, ein Ausfall zählt nicht als Fehlversuch. |
| Upload „fehlgeschlagen“ | Drei Versuche (sofort, nach 1 und nach 5 Minuten) sind gescheitert. In der Übersicht „Erneut versuchen“, oder „Ignorieren“, dann lädt das Tool die Datei nie wieder hoch (rückgängig unter „Ignoriert“). |
| „abgelehnt“ | ballchasing kann die Datei nicht verarbeiten (z. B. beschädigt). Am besten ignorieren, auch direkt aus der Benachrichtigung. |
| Kein Symbol in der Taskleiste (Windows) | Es liegt bei den ausgeblendeten Symbolen. Fehlt es auch dort, die exe starten. |
| Kein Symbol (Linux) | Die Leiste zeigt keine StatusNotifierItems. Unter GNOME die Erweiterung „AppIndicator and KStatusNotifierItem Support“ installieren; das Log sagt in dem Fall „Kein Infobereich für das Symbol gefunden“. Ohne Symbol öffnet der Hotkey (Strg+Alt+B) die Oberfläche, und mit „Minimiert starten“ aus öffnet sie sich bei jedem Start. |
| „läuft schon“ | Es läuft bereits eine Instanz mit derselben Konfiguration, etwa aus dem Autostart, und ihre Oberfläche ließ sich nicht öffnen. Über das Symbol öffnen oder das Programm dort beenden. |
| Konsolenfenster beim Start (Windows) | Die exe wurde ohne `-H windowsgui` gebaut. Das Tool schließt das Fenster dann sofort selbst, ganz verschwindet es nur mit dem Build-Befehl aus der [Installation](#1-programm-bauen). |
| Kein Replay-Ordner gefunden (macOS, Linux) | Das Spiel hat noch kein Replay gespeichert, oder es liegt in einem unbekannten Präfix. Den Ordner `TAGame/Demos` in den Einstellungen hinzufügen. |
| Schnell-Einordnen oder Overlay nicht über dem Spiel (Windows) | Rocket League auf „Randloses Fenster“ stellen. Die Tasten Strg+Alt+1–4 funktionieren auch im Vollbild. |
| Hotkey funktioniert nicht | Ein anderes Programm nutzt die Kombination. In den Einstellungen eine andere wählen. Unter Linux wechselt Strg+Alt+F1 bis F12 die Konsole und taugt nicht als Hotkey. |
| Hotkeys oder Speichern-Taste wirken nicht (Linux) | Sie brauchen X11 oder XWayland. Unter Wayland wirken sie nur, solange ein X11-Fenster wie das Spiel im Vordergrund ist. |
| Keine Benachrichtigungen (macOS) | Sie kommen über AppleScript und erscheinen als „Skripteditor“. In den Systemeinstellungen unter Mitteilungen für „Skripteditor“ erlauben. |
| Keine Benachrichtigungen (Linux) | Es läuft kein Benachrichtigungsdienst (bei schlanken Fenstermanagern etwa dunst oder mako starten). |
| Windows meldet, dass die App blockiert wurde | Smart App Control blockiert unsignierte Programme, die Microsoft nicht kennt, und bewertet jede exe einzeln. Eine neu gebaute exe wird neu bewertet; dauerhaft hilft nur eine Code-Signatur. |
| macOS meldet, dass die App nicht geöffnet werden kann | Nur bei einer heruntergeladenen App (etwa aus den CI-Artefakten): im Finder Rechtsklick → Öffnen. Selbst gebaute Apps betrifft das nicht. |

## Verhalten im Detail

- Vorgemerkt wird einmal pro Match, 3–8 Sekunden nach dem ersten Anstoß, und nur, wenn Rocket League das aktive Fenster ist (unter Linux erkannt an `WM_CLASS` oder am Prozess). Sonst versucht es das Tool alle 3 Sekunden erneut, bis das Match endet. Beim Anschauen gespeicherter Replays wird nichts gedrückt.
- Neue Dateien werden hochgeladen, sobald sich ihre Größe nicht mehr ändert (Prüfung alle 3 Sekunden). Dateien, die während einer Pause entstehen, kommen danach dran.
- Unter macOS und Linux sucht das Tool die Replay-Ordner höchstens alle 30 Sekunden neu; ein neuer Präfix (etwa nach der Installation in Heroic) wird so ohne Neustart gefunden.
- Nach dem Upload fragt das Tool ballchasing, bis das Replay verarbeitet ist, und speichert dann alle Statistiken lokal.
- Beim Start werden alle eigenen Replays und der Ordnerbaum von ballchasing gelesen, danach alle 15 Minuten nur Neues. „Aktualisieren“ im Ordner-Bereich liest alles neu. Fehlende Statistiken älterer Replays lädt das Tool langsam im Hintergrund nach (höchstens etwa 600 Abrufe pro Stunde, ballchasing erlaubt 1000).
- Spiele mit dir, die andere hochgeladen haben, findet das Tool über deine Spieler-IDs. Dasselbe Spiel von mehreren Uploadern erscheint einmal, dein eigener Upload hat Vorrang.
- Ein Replay kann nur in einem Ordner liegen (Vorgabe der API); Verschieben ersetzt den alten Ordner.
- Ordner löschen geht nur bei leeren Ordnern ohne Unterordner. Umbenennen geht nur auf der Website.

## Risiken und Grenzen

- **Anti-Cheat:** Das Tool liest nur die offizielle Stats API und Dateien und sendet einen einzelnen Tastendruck über die normale Eingabe des Systems (`SendInput` unter Windows, die X-Erweiterung XTEST unter Linux). Es injiziert nichts und öffnet keinen Handle auf den Spielprozess; Overlay und Schnell-Einordnen sind eigene Fenster bzw. Benachrichtigungen. Psyonix verbietet Makros, die einen Vorteil im Spiel bringen; eine offizielle Aussage zu einem Tastendruck fürs Replay-Speichern gibt es nicht. Wer gar kein Risiko will, schaltet „Replays automatisch speichern“ aus.
- **Chat:** Ist im Moment des Tastendrucks der Chat offen, löscht die Rücktaste dort Zeichen.
- **ballchasing als einzige Kopie:** Mit „nach dem Upload löschen“ liegt das Replay nur noch auf ballchasing.com, einem kostenlosen Dienst ohne Garantie.
- **Ränge und Einzelereignisse** (Tore mit Zeitpunkt, Heatmaps) liefert die API nicht.
- **Konsole:** Funktioniert nicht, dort gibt es keinen Zugriff auf Replay-Dateien.
- **macOS:** Speichern-Taste und Hotkeys bräuchten die Bedienungshilfen-Rechte und eigenen Systemcode; das Tool lässt sie weg. Ohne Knöpfe in Mitteilungen gibt es kein Schnell-Einordnen, die Regeln ordnen direkt ein.
- **Linux unter Wayland:** Hotkeys und Speichern-Taste gehen nur über XWayland (siehe [Systeme](#systeme)).

## Projektstruktur

```text
cmd/auto-replay-upload/   Einstiegspunkt: Log, eine Instanz, Autostart, Tray, Verdrahtung mit dem System
  winres/                 Symbol, Programmname und Manifest der exe (go-winres)
cmd/uidemo/               die Oberfläche mit erfundenen Daten gegen einen Fake von ballchasing
docs/konzept.md           Konzept mit allen Abläufen und Entscheidungen
scripts/macos-app.sh      baut die Mac-App
internal/
  app/                    Kern: Abläufe nach dem Upload, Abgleich, Sessions, Statistik, JSON-API der Oberfläche
  assets/                 das Programmsymbol und gen.go, das es zeichnet
  ballchasing/            Client für die ballchasing-API mit Drosselung
    ballchasingtest/      nachgebaute ballchasing-API für Tests und die Demo
  config/                 config.json lesen und schreiben, Token per DPAPI
  desktop/                Symbol, Autostart, Papierkorb, Dateiauswahl, Meldungsfenster
  game/                   Stats API, Match-Erkennung, Speichern-Taste, Replay-Ordner
  hotkey/                 globale Tastenkombinationen
  overlay/                das Overlay-Fenster (WebView2), sonst der Browser
  popup/                  Schnell-Einordnen und Benachrichtigungen
  rules/                  Regeln, Ordner-Vorschläge, Regel-Vorschläge
  store/                  SQLite-Datenbank
  upload/                 Replay-Ordner beobachten, Warteschlange mit Neuversuchen
  web/ui/                 die Oberfläche (HTML, CSS, JavaScript-Module ohne Build-Schritt)
  x11/                    X-Server unter Linux: aktives Fenster, Tastencodes, Tastendruck
```

Systemspezifischer Code steckt in Dateien mit der Endung `_windows.go`, `_darwin.go` und `_linux.go`, was macOS und Linux teilen in `_unix.go`, Platzhalter für andere Systeme in `_other.go`. Welche Funktionen ein System bietet, meldet das Programm der Oberfläche (`platform` in `/api/state`).

## Entwicklung

Bauen siehe [Installation](#1-programm-bauen). Prüfen für alle Systeme von einem Rechner aus:

```bash
for os in windows darwin linux; do GOOS=$os go vet ./...; done
```

Die Oberfläche mit Beispieldaten ausprobieren, ohne ballchasing-Konto (die Adresse steht in der Ausgabe; `-setup` startet mit dem Assistenten, `-os` zeigt sie wie unter `windows`, `darwin` oder `linux`):

```bash
go run ./cmd/uidemo -os darwin
```

`-slow 2s` verzögert jede Anfrage an den Fake, um die Info-Karten zu sehen. Die Demo nutzt eine eigene Spiel-Installation im Temp-Ordner, nie die echte.

Tests:

```bash
go test ./...
```

Das Symbol zeichnet `internal/assets/gen.go` (nur Standardbibliothek): Tray-Symbole als ICO und PNG in allen drei Zuständen, mit einer kräftigeren Variante bis 24 Pixel, und `internal/web/ui/icon.svg` für die Oberfläche. Nach einer Änderung dort beides neu erzeugen; der zweite Befehl lädt [go-winres](https://github.com/tc-hib/go-winres) und baut die Ressourcen der exe:

```bash
go generate ./internal/assets
go generate ./cmd/auto-replay-upload
```

Unter Linux starten die Tests einen eigenen X-Server (Xvfb) und einen eigenen D-Bus, falls installiert, und prüfen damit Hotkeys, Speichern-Taste und Benachrichtigungen echt; ohne sie werden diese Tests übersprungen. Die CI (`.github/workflows/ci.yml`) testet und baut auf Windows, macOS und Linux.

`POPUP_LIVE=1 go test -run TestLiveWindows ./internal/popup/` zeigt unter Windows kurz echte Popup-Fenster und prüft, dass sie oben liegen und keinen Fokus nehmen. `POPUP_PNG_DIR=<Ordner> go test ./internal/popup/` speichert die gezeichneten Popups als PNG.
