# auto-replay-upload

Lädt Rocket-League-Replays automatisch auf [ballchasing.com](https://ballchasing.com) hoch, sortiert sie in Ordner und zeigt Verlauf und Statistik, auch mit Easy Anti-Cheat und ohne BakkesMod.

Seit Easy Anti-Cheat (April 2026) läuft das BakkesMod-Plugin „AutoReplayUploader“ online nicht mehr. Dieses Tool greift nicht ins Spiel ein:

1. **Match erkennen** über die offizielle [Stats API](https://www.rocketleague.com/developer/stats-api) des Spiels (lokaler TCP-Port).
2. **Replay vormerken:** Kurz nach dem ersten Anstoß hält das Tool einmal die Speichern-Taste (Standard: Rücktaste), mit der das Spiel das Replay am Matchende automatisch speichert.
3. **Hochladen:** Neue `.replay`-Dateien im Replay-Ordner gehen über die ballchasing-API hoch, auf Wunsch landet die Datei danach im Papierkorb.
4. **Einsortieren:** Regeln (nach Playlist und Mitspielern) legen das Replay in einen ballchasing-Ordner. Ein kleines Fenster über dem Spiel zeigt das Ziel mit Countdown, Strg+Alt+1–4 wählt eine Alternative.

Dazu kommt eine Oberfläche für Übersicht, Verlauf mit allen Statistiken, Statistik-Diagramme, Ordner, Eingang und Regeln. Sie öffnet sich als Overlay über dem Spiel (Strg+Alt+B) und genauso im Browser. Das Tool läuft im Hintergrund, startet mit Windows und zeigt sich nur als Symbol im Infobereich.

Das Konzept mit allen Abläufen und Entscheidungen steht in [docs/konzept.md](docs/konzept.md).

## Voraussetzungen

- Windows 10 oder 11 mit der PC-Version von Rocket League (Steam oder Epic)
- ein Konto auf [ballchasing.com](https://ballchasing.com) und dessen Upload-Token
- [Go](https://go.dev/dl/) 1.26.2 oder neuer, um die exe zu bauen
- die WebView2-Laufzeit für das Overlay (bei Windows 10 und 11 vorinstalliert)

## Installation

### 1. Programm bauen

```bash
git clone https://github.com/EYGjamie/auto-replay-upload.git
cd auto-replay-upload
go build -trimpath -ldflags "-s -w -H windowsgui" -o dist/auto-replay-upload.exe ./cmd/auto-replay-upload
```

`-H windowsgui` sorgt dafür, dass kein Konsolenfenster aufgeht. Die fertige `dist/auto-replay-upload.exe` in einen eigenen Ordner kopieren, zum Beispiel `C:\Tools\auto-replay-upload\`. Dort legt das Tool auch Konfiguration, Datenbank und Log ab. Der Autostart zeigt auf genau diese exe; verschiebst du den Ordner, einfach einmal von dort starten.

Auf macOS oder Linux lässt sich die Windows-exe genauso bauen, mit `GOOS=windows GOARCH=amd64` vor dem `go build`.

### 2. Stats API aktivieren

Rocket League beenden. Im Installationsordner die Datei `TAGame\Config\TAStatsAPI.ini` öffnen (gibt es sie nicht, dann `DefaultStatsAPI.ini`) und im Abschnitt `[TAGame.MatchStatsExporter_TA]` setzen:

```ini
[TAGame.MatchStatsExporter_TA]
PacketSendRate=1
Port=49123
```

Bei `PacketSendRate=0` (Standard) ist die API aus. Übliche Installationsordner:

- Steam: `C:\Program Files (x86)\Steam\steamapps\common\rocketleague`
- Epic: `C:\Program Files\Epic Games\rocketleague`

Die Datei liegt meist unter `Program Files`; der Editor muss dann als Administrator laufen. Spielupdates können sie zurücksetzen, also nach einem Update prüfen.

Für das Overlay und das Schnell-Einordnen Rocket League im Anzeigemodus **„Randloses Fenster“** spielen. Im echten Vollbild kann kein anderes Fenster darüber liegen.

### 3. Erst-Einrichtung

`auto-replay-upload.exe` starten. Beim ersten Start öffnet sich der Assistent im Overlay:

1. **ballchasing verbinden:** Upload-Token von [ballchasing.com/upload](https://ballchasing.com/upload) einfügen, danach bestätigen, welcher Spieler du bist (mehrere Accounts wie Steam und Epic sind möglich).
2. **Replay-Ordner:** erkannt werden `Dokumente\My Games\Rocket League\TAGame\Demos` und `DemosEpic`, eigene lassen sich ergänzen.
3. **Speichern-Taste:** die Taste wählen, die im Spiel unter Einstellungen → Steuerung fürs Replay-Speichern eingestellt ist, und mit „Testen“ ausprobieren.
4. **Upload und Ordner:** Sichtbarkeit, Löschen nach Upload, und welche Playlist in welchen Ordner kommt (bestehender Ordner oder ein neuer).
5. **Alte Replays nachladen:** keine, die letzten 30 Tage oder alle.

„Später einrichten“ geht auf jeder Seite. Alles lässt sich danach in den Einstellungen ändern.

Wer von einer älteren Version umsteigt, behält `config.json` und den Upload-Status: Die alte `uploaded.json` wird beim ersten Start in die Datenbank übernommen (danach `uploaded.json.bak`).

## Benutzung

### Im Hintergrund

Das Tool startet mit Windows und zeigt sich nur als Symbol im Infobereich der Taskleiste. Unter Windows 11 liegt es zunächst bei den **ausgeblendeten Symbolen** (Pfeil **^** neben der Uhr). Grau heißt pausiert, ein roter Punkt heißt, dass Uploads fehlgeschlagen sind.

**Linksklick** öffnet das Overlay, **Rechtsklick** das Menü:

| Eintrag | Funktion |
|---|---|
| Pausieren / Fortsetzen | Uploads und Auto-Speichern anhalten (auch Strg+Alt+P) |
| Replays hochladen … | Dateien auswählen und hochladen |
| Replay-Ordner öffnen | im Explorer |
| Overlay öffnen | die Oberfläche über dem Spiel (auch Strg+Alt+B) |
| Im Browser öffnen | dieselbe Oberfläche im Standardbrowser |
| Einstellungen | Overlay mit den Einstellungen |
| Beenden | bis zum nächsten Start |

Startest du die exe ein zweites Mal, meldet sie nur, dass sie schon läuft.

### Nach jedem Match

1. Das Spiel speichert das Replay (dank der vorgemerkten Speichern-Taste), das Tool lädt es hoch und wartet, bis ballchasing es verarbeitet hat.
2. Passt eine Regel, erscheint oben rechts über dem Spiel das **Schnell-Einordnen**: Zielordner, Countdown (Standard 10 s) und bis zu drei Alternativen. **Strg+Alt+1–3** wählt eine Alternative, **Strg+Alt+4** schickt das Replay in den Eingang; läuft der Countdown ab, gilt der Regel-Ordner. Das Fenster nimmt dem Spiel nie den Fokus.
3. Ohne passende Regel landet das Replay im **Eingang**.
4. Eine kurze **Benachrichtigung** unten rechts sagt, wohin es ging. Benachrichtigungen werden zurückgehalten, solange ein Match läuft. Nach 30 Minuten ohne Spiel (oder wenn Rocket League geschlossen wird) gibt es eine Zusammenfassung der Session.

Hat das Vormerken nicht geklappt (zum Beispiel weil ein anderes Fenster im Vordergrund war), gibt es am Matchende einen Warnton. Dann im Spiel „Replay speichern“ wählen; der Upload passiert trotzdem automatisch.

### Die Oberfläche

| Bereich | Inhalt |
|---|---|
| Übersicht | Status (ballchasing, Rocket League, Pause), Warteschlange mit „Jetzt erneut“, letzte Uploads, aktuelle Session mit Siegesfolge, Replays hochladen (auch per Drag & Drop auf die Seite) |
| Verlauf | alle Replays deines Kontos und die Spiele mit dir, die Mitspieler hochgeladen haben (Duplikate nur einmal). Suche nach Map, Spieler oder Datei, Filter nach Status, Playlist, Zeitraum, Ordner und Herkunft. Aufklappen zeigt Punkte, Tore, Vorlagen, Paraden und Schüsse; Mehrfachauswahl zum Verschieben, Ändern der Sichtbarkeit oder Löschen |
| Replay-Details | alle Statistiken der ballchasing-API in Reitern: Überblick, Kern, Boost, Bewegung, Position, Demos, Kamera & Auto |
| Statistik | 7 Tage, 14 Tage, Monat oder Gesamt: Spiele, Siegquote, Bilanz, Uploads, Spiele pro Tag, Verteilung nach Playlist, beste Maps |
| Ordner | der ballchasing-Ordnerbaum mit Anzahl, neue Ordner und Unterordner, Replays per Drag & Drop verschieben, Vorschläge („8 Ranked-Standard-Spiele → Ranked › Threes“) und ganze Sessions auf einmal einordnen oder nach Playlist aufteilen |
| Eingang | alles ohne Ordner, mit Vorschlag pro Replay. Tastatur: **1–4** Ordner, **Enter** Vorschlag übernehmen, **N** neuer Ordner, **E** überspringen, **J/K** weiter/zurück, **O** auf ballchasing öffnen |
| Regeln | Regeln nach Playlist und Mitspielern, Reihenfolge per Ziehen; die erste passende gewinnt. Vorschläge für neue Regeln aus deinem Einordnen |
| Einstellungen | alles aus der Einrichtung; jede Änderung gilt sofort |

Replays, die Mitspieler hochgeladen haben, lassen sich nur ansehen und ins Spiel laden. Verschieben, Sichtbarkeit und Löschen erlaubt die ballchasing-API nur beim eigenen Upload.

### Ins Spiel laden

Ist ein Replay nicht mehr lokal vorhanden (etwa weil „nach dem Upload löschen“ an ist), holt **„Ins Spiel laden“** im Menü eines Replays die Datei von ballchasing in den Replay-Ordner. Im Spiel unter „Replays“ öffnen (eventuell das Menü einmal neu öffnen). Sobald du das Replay im Spiel verlässt, löscht das Tool die Datei wieder, außer du wählst „Geladene Datei behalten“. ballchasing erlaubt ohne Patreon 200 Downloads pro Stunde.

### Löschen nach dem Upload

Ist es eingeschaltet, wandert eine Datei in den **Papierkorb**, sobald ballchasing sie verarbeitet hat, und nur wenn alles davon zutrifft: dein eigener Upload, Status `ok`, aus dem Replay-Ordner (nicht von Hand hochgeladen), keine fürs Anschauen geladene Datei. Altdateien, die nie hochgeladen wurden, bleiben liegen.

## Konfiguration

Alles lässt sich in der Oberfläche einstellen. Gespeichert wird in `config.json` neben der exe (oder dort, wo `-config` hinzeigt):

```powershell
.\auto-replay-upload.exe -config D:\rl\config.json
```

| Feld | Standard | Bedeutung |
|---|---|---|
| `ballchasing_token_protected` | – | Upload-Token, mit Windows-DPAPI für deinen Benutzer verschlüsselt. Die Umgebungsvariable `BALLCHASING_TOKEN` hat Vorrang |
| `visibility` | `private` | Sichtbarkeit neuer Uploads: `public`, `unlisted`, `private` |
| `demo_dirs` | `[]` | Replay-Ordner; leer = automatisch |
| `upload_existing` | `false` | beim Start noch nicht hochgeladene Altdateien hochladen |
| `delete_after_upload` | `false` | Dateien nach dem Upload in den Papierkorb |
| `stats_api_addr` | `127.0.0.1:49123` | Adresse der Stats API (muss zu `Port` in der ini passen) |
| `auto_mark` | `true` | Speichern-Taste nach dem ersten Anstoß drücken |
| `online_only` | `true` | nur Online-/LAN-Matches vormerken |
| `mark_key` | `Backspace` | Speichern-Taste als [KeyboardEvent.code](https://developer.mozilla.org/docs/Web/API/KeyboardEvent/code) |
| `mark_hold_ms` | `1200` | wie lange die Taste gehalten wird (100 bis 5000) |
| `gui_hotkey` | `Ctrl+Alt+B` | Overlay öffnen und schließen; leer = kein Hotkey |
| `pause_hotkey` | `Ctrl+Alt+P` | pausieren und fortsetzen |
| `notify_uploaded`, `notify_failed`, `notify_session` | `true` | Benachrichtigungen |
| `quick_sort` | `true` | Schnell-Einordnen über dem Spiel; aus = Regeln ordnen ohne Rückfrage ein |
| `quick_sort_seconds` | `10` | Countdown (3 bis 60) |
| `start_minimized` | `true` | beim Start nur das Symbol zeigen |
| `setup_done` | `false` | Assistent erledigt |

Hotkeys bestehen aus `Ctrl`/`Strg`, `Alt`, `Shift`, `Win` plus Buchstabe, Ziffer oder `F1`–`F24`.

Weitere Dateien neben der `config.json`:

- `auto-replay-upload.db`: Datenbank mit Upload-Status, Replays, Ordnern, Regeln und Entscheidungen
- `auto-replay-upload.log`: das Log (über 1 MB wird es beim Start zu `.log.old`), im Overlay unter Einstellungen → Log anzeigen
- `manuell\`: Kopien von Replays, die über die Oberfläche hochgeladen wurden

Außerdem: `%LOCALAPPDATA%\auto-replay-upload\WebView2` (Browser-Cache des Overlays), der Autostart-Eintrag unter `HKEY_CURRENT_USER\…\CurrentVersion\Run` und die gemerkte Autostart-Wahl unter `HKEY_CURRENT_USER\Software\auto-replay-upload`.

**Entfernen:** In den Einstellungen „Mit Windows starten“ ausschalten, über das Symbol beenden und den Ordner löschen.

## Fehlerbehebung

Meldungen stehen im Log, die letzte auch im Tooltip des Symbols. Fehler, wegen derer das Tool nicht starten kann, erscheinen als Meldungsfenster.

| Problem | Ursache und Lösung |
|---|---|
| „Rocket League nicht verbunden“, obwohl das Spiel läuft | Stats API aus oder anderer Port. `PacketSendRate` und `Port` in der ini prüfen (siehe [Installation](#2-stats-api-aktivieren)), danach das Spiel neu starten. |
| „Token ungültig“ | Neuen Token auf [ballchasing.com/upload](https://ballchasing.com/upload) erzeugen und in den Einstellungen eintragen. Uploads warten so lange. |
| „ballchasing nicht erreichbar“, Uploads warten | Internet oder ballchasing gestört. Die Uploads laufen von selbst weiter, ein Ausfall zählt nicht als Fehlversuch. |
| Upload „fehlgeschlagen“ | Drei Versuche (sofort, nach 1 und nach 5 Minuten) sind gescheitert. In der Übersicht „Jetzt erneut“. |
| „abgelehnt“ | ballchasing kann die Datei nicht verarbeiten (z. B. beschädigt). |
| Kein Symbol in der Taskleiste | Es liegt bei den ausgeblendeten Symbolen. Fehlt es auch dort, die exe starten. |
| „läuft schon“ | Es läuft bereits eine Instanz mit derselben Konfiguration, etwa aus dem Autostart. |
| Schnell-Einordnen oder Overlay nicht über dem Spiel | Rocket League auf „Randloses Fenster“ stellen. Die Tasten Strg+Alt+1–4 funktionieren auch im Vollbild. |
| Hotkey funktioniert nicht | Ein anderes Programm nutzt die Kombination. In den Einstellungen eine andere wählen. |
| Windows meldet, dass die App blockiert wurde | Smart App Control blockiert unsignierte Programme, die Microsoft nicht kennt, und bewertet jede exe einzeln. Eine neu gebaute exe wird neu bewertet; dauerhaft hilft nur eine Code-Signatur. |

## Verhalten im Detail

- Vorgemerkt wird einmal pro Match, 3–8 Sekunden nach dem ersten Anstoß, und nur, wenn `RocketLeague.exe` das aktive Fenster ist. Sonst versucht es das Tool alle 3 Sekunden erneut, bis das Match endet. Beim Anschauen gespeicherter Replays wird nichts gedrückt.
- Neue Dateien werden hochgeladen, sobald sich ihre Größe nicht mehr ändert (Prüfung alle 3 Sekunden). Dateien, die während einer Pause entstehen, kommen danach dran.
- Nach dem Upload fragt das Tool ballchasing, bis das Replay verarbeitet ist, und speichert dann alle Statistiken lokal.
- Beim Start werden alle eigenen Replays und der Ordnerbaum von ballchasing gelesen, danach alle 15 Minuten nur Neues. „Aktualisieren“ im Ordner-Bereich liest alles neu. Fehlende Statistiken älterer Replays lädt das Tool langsam im Hintergrund nach (höchstens etwa 600 Abrufe pro Stunde, ballchasing erlaubt 1000).
- Spiele mit dir, die andere hochgeladen haben, findet das Tool über deine Spieler-IDs. Dasselbe Spiel von mehreren Uploadern erscheint einmal, dein eigener Upload hat Vorrang.
- Ein Replay kann nur in einem Ordner liegen (Vorgabe der API); Verschieben ersetzt den alten Ordner.
- Ordner löschen geht nur bei leeren Ordnern ohne Unterordner. Umbenennen geht nur auf der Website.

## Risiken und Grenzen

- **Anti-Cheat:** Das Tool liest nur die offizielle Stats API und Dateien und sendet einen einzelnen Tastendruck über die normale Windows-Eingabe (`SendInput`). Es injiziert nichts und öffnet keinen Handle auf den Spielprozess; Overlay und Schnell-Einordnen sind eigene Fenster. Psyonix verbietet Makros, die einen Vorteil im Spiel bringen; eine offizielle Aussage zu einem Tastendruck fürs Replay-Speichern gibt es nicht. Wer gar kein Risiko will, schaltet „Replays automatisch speichern“ aus.
- **Chat:** Ist im Moment des Tastendrucks der Chat offen, löscht die Rücktaste dort Zeichen.
- **ballchasing als einzige Kopie:** Mit „nach dem Upload löschen“ liegt das Replay nur noch auf ballchasing.com, einem kostenlosen Dienst ohne Garantie.
- **Ränge und Einzelereignisse** (Tore mit Zeitpunkt, Heatmaps) liefert die API nicht.
- **Konsole:** Funktioniert nicht, dort gibt es keinen Zugriff auf Replay-Dateien.
- **Plattform:** Tastendruck, Hotkeys, Overlay, Popups, Symbol und Autostart gibt es nur unter Windows. Auf anderen Systemen läuft das Tool im Terminal, lädt hoch (`demo_dirs` muss gesetzt sein) und öffnet die Oberfläche im Browser.

## Projektstruktur

```text
cmd/auto-replay-upload/   Einstiegspunkt: Log, eine Instanz, Autostart, Tray, Verdrahtung mit Windows
cmd/uidemo/               die Oberfläche mit erfundenen Daten gegen einen Fake von ballchasing
docs/konzept.md           Konzept mit allen Abläufen und Entscheidungen
internal/
  app/                    Kern: Abläufe nach dem Upload, Abgleich, Sessions, Statistik, JSON-API der Oberfläche
  ballchasing/            Client für die ballchasing-API mit Drosselung
    ballchasingtest/      nachgebaute ballchasing-API für Tests und die Demo
  config/                 config.json lesen und schreiben, Token per DPAPI
  desktop/                Symbol im Infobereich, Autostart, Papierkorb, Dateiauswahl, Meldungsfenster
  game/                   Stats API, Match-Erkennung, Speichern-Taste, Replay-Ordner
  hotkey/                 globale Tastenkombinationen
  overlay/                das Overlay-Fenster (WebView2)
  popup/                  Schnell-Einordnen und Benachrichtigungen über dem Spiel
  rules/                  Regeln, Ordner-Vorschläge, Regel-Vorschläge
  store/                  SQLite-Datenbank
  upload/                 Replay-Ordner beobachten, Warteschlange mit Neuversuchen
  web/ui/                 die Oberfläche (HTML, CSS, JavaScript-Module ohne Build-Schritt)
```

Windows-spezifischer Code steckt in Dateien mit der Endung `_windows.go`, die Gegenstücke für andere Systeme in `_other.go`.

## Entwicklung

Bauen (auch unter macOS oder Linux für Windows):

```bash
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H windowsgui" -o dist/auto-replay-upload.exe ./cmd/auto-replay-upload
```

Die Oberfläche mit Beispieldaten ausprobieren, ohne ballchasing-Konto (die Adresse steht in der Ausgabe; `-setup` startet mit dem Assistenten):

```bash
go run ./cmd/uidemo
```

Tests:

```bash
go test ./...
```

`POPUP_LIVE=1 go test -run TestLiveWindows ./internal/popup/` zeigt kurz echte Popup-Fenster und prüft, dass sie oben liegen und keinen Fokus nehmen. `POPUP_PNG_DIR=<Ordner> go test ./internal/popup/` speichert die gezeichneten Popups als PNG.
