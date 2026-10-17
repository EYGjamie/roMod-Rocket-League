# auto-replay-upload

Lädt Rocket-League-Replays automatisch auf [ballchasing.com](https://ballchasing.com) hoch, auch mit Easy Anti-Cheat und ohne BakkesMod.

Seit Easy Anti-Cheat (April 2026) läuft das BakkesMod-Plugin „AutoReplayUploader“ online nicht mehr. Dieses Tool greift nicht ins Spiel ein und erledigt drei Dinge:

1. **Match erkennen** über die offizielle [Stats API](https://www.rocketleague.com/developer/stats-api) des Spiels (lokaler TCP-Port).
2. **Replay vormerken:** Kurz nach dem ersten Anstoß hält das Tool einmal **Backspace** gedrückt (ca. 1,2 s). Das ist die eingebaute Taste, mit der das Spiel das Replay am Matchende automatisch speichert.
3. **Hochladen:** Neue `.replay`-Dateien im Replay-Ordner werden über die ballchasing-API hochgeladen.

Dazu kommt ein Overlay (**Strg+Alt+B**), mit dem du hochgeladene Replays in ballchasing-Gruppen und Untergruppen sortierst.

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

`-H windowsgui` sorgt dafür, dass beim Start kein Konsolenfenster aufgeht. Die fertige `dist/auto-replay-upload.exe` in einen eigenen Ordner kopieren, zum Beispiel `C:\Tools\auto-replay-upload\`. Dort legt das Tool später auch seine Konfiguration, seinen Status und sein Log ab. Der Autostart zeigt auf genau diese exe, verschiebst du den Ordner, einfach einmal von dort starten.

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

Spielupdates können die Datei zurücksetzen. Nach einem Update also prüfen, ob der Wert noch gesetzt ist.

### 3. Replay-Taste im Spiel prüfen

Unter Einstellungen → Steuerung muss die Aktion zum Speichern bzw. Vormerken des Replays auf **Backspace** liegen (Standard). Ausprobieren: In einem Match Backspace etwa eine Sekunde halten, dann meldet das Spiel, dass das Replay am Ende gespeichert wird.

Für das Overlay Rocket League im Anzeigemodus **„Randloses Fenster“** spielen. Im echten Vollbild kann kein anderes Fenster darüber liegen.

### 4. Token eintragen

1. `auto-replay-upload.exe` per Doppelklick starten. Beim ersten Start legt das Tool daneben eine `config.json` an, meldet das und öffnet sie im Editor.
2. Den Upload-Token von [ballchasing.com/upload](https://ballchasing.com/upload) kopieren, in der `config.json` bei `ballchasing_token` eintragen, speichern und die exe erneut starten:

   ```json
   {
     "ballchasing_token": "dein-token",
     "visibility": "private",
     ...
   }
   ```

   Alternativ den Token in der Umgebungsvariable `BALLCHASING_TOKEN` setzen, sie hat Vorrang vor der Datei.

Alle weiteren Einstellungen stehen unter [Konfiguration](#konfiguration).

## Benutzung

### Starten

Das Tool läuft im Hintergrund, ohne Fenster. Es startet automatisch, wenn du dich bei Windows anmeldest, und zeigt sich nur als Symbol im Infobereich der Taskleiste. Unter Windows 11 landet es dort zunächst bei den **ausgeblendeten Symbolen** (Pfeil **^** neben der Uhr). Wer es dauerhaft sehen will, zieht es von dort in die Taskleiste.

Ob das Spiel schon läuft, ist egal: Das Tool wartet auf die Stats API und verbindet sich neu, wenn du Rocket League neu startest. Startest du die exe ein zweites Mal, meldet sie nur, dass sie schon läuft.

**Linksklick** auf das Symbol öffnet das Overlay zum Gruppieren, ein **Rechtsklick** das Menü:

| Eintrag | Funktion |
|---|---|
| Replays gruppieren | öffnet das [Overlay](#replays-gruppieren), daneben steht der Hotkey |
| Mit Windows starten | Autostart an oder aus (Standard: an) |
| Konfiguration bearbeiten | öffnet die `config.json` im Editor, Änderungen gelten nach einem Neustart des Tools |
| Log anzeigen | öffnet das Log im Editor |
| Beenden | beendet das Tool bis zum nächsten Start |

Fährst du mit der Maus über das Symbol, zeigt der Tooltip die letzte Meldung. Das komplette Log steht in `auto-replay-upload.log` neben der Konfiguration, zum Beispiel:

```text
20:14:02 Beobachte C:\Users\du\Documents\My Games\Rocket League\TAGame\Demos
20:14:03 Bei ballchasing angemeldet als Jamie, Sichtbarkeit: private
20:14:03 Ctrl+Alt+B öffnet und schließt das Overlay zum Gruppieren
20:14:03 Replays werden nach dem ersten Anstoß automatisch vorgemerkt
20:14:03 Warte auf Rocket League (Stats API 127.0.0.1:49123 nicht erreichbar) …
20:15:40 Mit der Stats API verbunden (127.0.0.1:49123)
20:17:12 Backspace gehalten, Replay wird am Matchende gespeichert
20:23:05 4D2F9A1C8B7E4E21A6F05C3B9D7E1A42.replay hochgeladen: https://ballchasing.com/replay/…
```

Beim allerersten Start werden vorhandene Replays nur registriert, nicht hochgeladen („Erster Start: 42 vorhandene Replays werden nicht hochgeladen“). Danach merkt sich das Tool in `uploaded.json`, was erledigt ist. Replays, die gespeichert werden, während das Tool nicht läuft, werden beim nächsten Start nachgeholt.

Eine andere Konfigurationsdatei lässt sich mit `-config` angeben, `uploaded.json` liegt dann im selben Ordner:

```powershell
.\auto-replay-upload.exe -config D:\rl\config.json
```

Der Autostart übernimmt diesen Pfad. Er steht als Eintrag „auto-replay-upload“ auch im Task-Manager unter **Autostart-Apps** und lässt sich dort ebenfalls abschalten. Ob du ihn im Menü abgeschaltet hast, merkt sich das Tool unter `HKEY_CURRENT_USER\Software\auto-replay-upload`.

**Entfernen:** Im Menü den Haken bei „Mit Windows starten“ entfernen, „Beenden“ wählen und den Ordner löschen.

### Während des Spielens

Du musst nichts tun. Pro Online-Match drückt das Tool einmal Backspace, das Spiel speichert das Replay am Ende, und ein paar Sekunden später ist es auf ballchasing.

Hat das Vormerken nicht geklappt (zum Beispiel weil ein anderes Fenster im Vordergrund war), gibt es am Matchende einen **Warnton**. Dann im Spiel „Replay speichern“ wählen; der Upload passiert trotzdem automatisch.

### Replays gruppieren

**Strg+Alt+B** oder ein Linksklick auf das Symbol öffnet ein Overlay-Fenster über dem Spiel, in dem du:

- eine bestehende ballchasing-Gruppe wählst oder eine neue anlegst („Teams mit Wechselspielern“ schaltet die Teamerkennung auf `by-player-clusters`),
- darin eine Untergruppe wählst oder neu anlegst, beliebig tief verschachtelt,
- deine hochgeladenen Replays filterst (Spieler, Map, Modus, Gruppe) und anhakst,
- die ausgewählten Replays mit einem Klick in die Gruppe oder Untergruppe verschiebst.

Sobald du eine Gruppe wählst, erscheint daneben eine Auswahl mit ihren Untergruppen, etwa `Liga 2026 › Spieltag 1 › …`:

- **direkt in „…“** verschiebt in die links davon gewählte Gruppe selbst,
- eine **Untergruppe** öffnet die nächste Ebene,
- **+ Neue Untergruppe …** legt beim Verschieben eine neue Untergruppe an dieser Stelle an.

Der Button unten zeigt immer, wohin verschoben wird, zum Beispiel *In „Spieltag 1“ verschieben*. Die Zahl hinter einem Gruppennamen ist die Anzahl der Replays direkt in dieser Gruppe, ohne die ihrer Untergruppen.

Das Overlay ist rahmenlos, bleibt immer im Vordergrund und erscheint mittig auf dem Monitor, auf dem das Spiel läuft. **Esc**, das ✕ oder nochmal Strg+Alt+B schließen es, danach ist das Spiel wieder aktiv. Beim nächsten Öffnen lädt es die Replay-Liste neu.

- Das Overlay ist ein eigenes Fenster und wird nicht ins Spiel eingeblendet (das würde Easy Anti-Cheat blockieren). Solange es offen ist, hat das Spiel keinen Fokus.
- Im echten Vollbild wird das Spiel beim Öffnen minimiert, daher „Randloses Fenster“ verwenden.
- Fehlt die WebView2-Laufzeit, öffnet sich die Seite stattdessen im Browser.

Die Oberfläche läuft intern als lokale Seite (`127.0.0.1`), die nur Anfragen mit einem bei jedem Start neuen Schlüssel annimmt. Ist kein Hotkey gesetzt oder ist die Kombination schon belegt, öffnest du das Overlay über das Symbol; der Link zur Seite steht außerdem im Log.

Verschieben ersetzt die bisherige Gruppe eines Replays, denn die API kennt pro Replay nur ein Gruppenfeld. Wegen des Rate-Limits (2 Änderungen pro Sekunde) dauert das Verschieben etwa 0,6 s pro Replay.

### Ein Replay erneut hochladen

Tool über das Symbol beenden, den Eintrag des Replays aus `uploaded.json` löschen und die exe neu starten. Ist das Replay schon auf ballchasing, wird es nicht doppelt angelegt, sondern nur als erledigt markiert.

## Konfiguration

Die `config.json` liegt neben der exe (oder dort, wo `-config` hinzeigt). Nach Änderungen das Tool neu starten.

| Feld | Standard | Bedeutung |
|---|---|---|
| `ballchasing_token` | – | Upload-Token (alternativ Umgebungsvariable `BALLCHASING_TOKEN`) |
| `visibility` | `private` | `public`, `unlisted` oder `private` |
| `group` | – | ballchasing-Gruppen-ID, in die hochgeladen wird |
| `demo_dirs` | automatisch | Replay-Ordner; leer = `Dokumente\My Games\Rocket League\TAGame\Demos` und `DemosEpic` |
| `stats_api_addr` | `127.0.0.1:49123` | Adresse der Stats API (muss zu `Port` in der ini passen) |
| `auto_mark` | `true` | `false` = kein Tastendruck, nur Warnton am Matchende |
| `mark_hold_ms` | `1200` | Wie lange Backspace gehalten wird (mindestens 100) |
| `online_only` | `true` | Nur Online-/LAN-Matches vormerken, Offline-Matches und Training ignorieren |
| `gui_hotkey` | `Ctrl+Alt+B` | Öffnet und schließt das Overlay. Modifikatoren `Ctrl`/`Strg`, `Alt`, `Shift`, `Win` plus Buchstabe, Ziffer oder `F1`–`F24`; leer = kein Hotkey |

Das Tool legt außerdem diese Dateien an:

- `uploaded.json` neben der `config.json`: welche Replays schon erledigt sind
- `auto-replay-upload.log` neben der `config.json`: das Log; über 1 MB wird es beim Start zu `.log.old`
- `%LOCALAPPDATA%\auto-replay-upload\WebView2`: Browser-Cache des Overlays

## Fehlerbehebung

Meldungen stehen im Log (Symbol → „Log anzeigen“), die letzte auch im Tooltip des Symbols. Fehler, wegen derer das Tool nicht starten kann, erscheinen als Meldungsfenster.

| Meldung oder Problem | Ursache und Lösung |
|---|---|
| Kein Symbol in der Taskleiste | Es liegt bei den ausgeblendeten Symbolen (Pfeil **^** neben der Uhr). Fehlt es auch dort, die exe starten. |
| `auto-replay-upload läuft schon` | Es läuft bereits eine Instanz, etwa aus dem Autostart. Zum Neustart erst über das Symbol beenden. |
| Windows meldet, dass die App blockiert wurde | Smart App Control blockiert unsignierte Programme, die Microsoft nicht kennt; es bewertet jede exe einzeln. Eine neu gebaute exe wird neu bewertet. Dauerhaft hilft nur eine Code-Signatur. |
| `Warte auf Rocket League (Stats API … nicht erreichbar)`, obwohl das Spiel läuft | Stats API ist aus oder auf einem anderen Port. `PacketSendRate` und `Port` in der ini prüfen (siehe [Installation](#2-stats-api-aktivieren)), danach das Spiel neu starten. |
| `Der ballchasing-Token ist ungültig` | Neuen Token auf [ballchasing.com/upload](https://ballchasing.com/upload) erzeugen und eintragen. |
| `Noch kein Replay-Ordner vorhanden, warte auf: …` | Normal vor dem ersten gespeicherten Replay. Liegen die Replays woanders, `demo_dirs` setzen. |
| `Rocket League ist nicht im Vordergrund, versuche es weiter …` | Das Tool drückt die Taste nur, wenn das Spiel aktiv ist. Ins Spiel wechseln; klappt es bis zum Matchende nicht, kommt der Warnton. |
| `Hotkey … nicht verfügbar` | Ein anderes Programm nutzt die Kombination. `gui_hotkey` ändern oder das Overlay über das Symbol öffnen. |
| Overlay erscheint nicht über dem Spiel | Rocket League auf „Randloses Fenster“ stellen. |

## Verhalten im Detail

- Vorgemerkt wird einmal pro Match, 3–8 Sekunden nach dem ersten Anstoß (zufällig).
- Die Taste wird **nur** gesendet, wenn `RocketLeague.exe` das aktive Fenster ist. Sonst versucht es das Tool alle 3 Sekunden erneut, bis das Match endet.
- Beim Anschauen gespeicherter Replays wird nichts gedrückt.
- Neue Dateien werden erst hochgeladen, wenn sich ihre Größe nicht mehr ändert (der Ordner wird alle 3 Sekunden geprüft).
- Uploads, die fehlschlagen (Netzwerk, Rate-Limit), werden mit wachsendem Abstand wiederholt (30 s bis 10 min). Replays, die ballchasing ablehnt, werden übersprungen. Bereits vorhandene Replays zählen als erledigt.

## Risiken und Grenzen

- **Anti-Cheat:** Das Tool liest nur die offizielle Stats API und Dateien und sendet einen einzelnen Tastendruck über die normale Windows-Eingabe (`SendInput`). Es injiziert nichts und öffnet keinen Handle auf den Spielprozess. Psyonix verbietet Makros, die einen Vorteil im Spiel bringen; eine offizielle Aussage zu einem Tastendruck fürs Replay-Speichern gibt es nicht. Wer gar kein Risiko will, setzt `auto_mark` auf `false`.
- **Chat:** Ist im Moment des Tastendrucks der Chat offen, löscht Backspace dort Zeichen.
- **Konsole:** Funktioniert nicht, dort gibt es keinen Zugriff auf Replay-Dateien.
- **Plattform:** Tastendruck, Hotkey, Overlay, Symbol und Autostart gibt es nur unter Windows. Auf anderen Systemen läuft das Tool im Terminal und erledigt nur den Upload (`demo_dirs` muss dann gesetzt sein); die Gruppen-Seite erreichst du dort über den Link im Log.

## Projektstruktur

```text
cmd/auto-replay-upload/   Einstiegspunkt: liest die Konfiguration und startet alles
internal/
  ballchasing/            Client für die ballchasing-API (Upload, Replays, Gruppen)
    ballchasingtest/      nachgebaute ballchasing-API für Tests
  config/                 config.json lesen, prüfen und beim ersten Start anlegen
  desktop/                Symbol im Infobereich, Autostart, nur eine Instanz, Meldungsfenster
  game/                   Stats API, Match-Erkennung, Backspace-Tastendruck, Replay-Ordner
  gui/                    lokale Gruppen-Seite (gui.html) und Overlay-Fenster
  hotkey/                 globale Tastenkombination
  upload/                 Replay-Ordner beobachten, hochladen, Status in uploaded.json
```

Windows-spezifischer Code steckt in Dateien mit der Endung `_windows.go`, die Gegenstücke für andere Systeme in `_other.go`.

## Entwicklung

Bauen (auch unter macOS oder Linux für Windows):

```bash
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H windowsgui" -o dist/auto-replay-upload.exe ./cmd/auto-replay-upload
```

Zum Debuggen ohne `-H windowsgui` starten, dann steht das Log zusätzlich im Terminal. Eine laufende Instanz vorher über das Symbol beenden. Mit `go run` gestartet lässt das Tool den Autostart in Ruhe:

```bash
go run ./cmd/auto-replay-upload -config dist/config.json
```

Tests:

```bash
go test ./...
```
