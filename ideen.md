Hier alle Funktionen aus den Mockups, nach Bereich. Die Beispieldaten darin (Spieler, Zahlen, Ordnernamen) sind erfunden.

Kern-Ablauf (alle Richtungen)
- Erkennt automatisch, wenn ein Spiel startet
- Drückt kurz nach Anstoß den Shortcut, damit das Replay gespeichert wird
- Überwacht den Demos-Ordner und erkennt neue Replay-Dateien
- Lädt automatisch zu ballchasing.com hoch
    - Bei erfolgreichem Hochladen, Lösche die Replay Datei aus dem Ordner (Sofern in den Einstellungen dies als Aktiviert)
- Sortiert automatisch in einen Ordner (= ballchasing-Gruppe) ein, nach Regeln die man festlegen kann
    - Regeln sind vor allem die veschiedenen Playlists (1s, 2s, 3s, Extra Modi (einzelnd), Private)
- Status & Fehler
- Warteschlange mit fehlgeschlagenen Uploads, Fehlergrund und automatischem Neuversuch (Countdown, bei 3 Fehlversuchen keine weiteren Automatischen Versuche)
    - „Jetzt erneut“ einzeln und „Alle fehlgeschlagenen erneut senden“
    - Fehler-Badge in der Navigation
- Überwachung des Ordners pausieren und fortsetzen (auch per Hotkey Strg+Alt+P)
- Auto Speichern pausieren und fortsetzen (auch per Hotkey Strg+Alt+P)
- Letzte Uploads mit Map, Playlist, Ergebnis, Dauer, Uhrzeit, Ordner und Status
- Session-Übersicht: Anzahl Spiele, Sieg/Niederlage-Bilanz, Siegesfolge als Streifen
- Replays manuell hochladen, Replay-Ordner öffnen
- Verlauf

- Suche nach Map, Spieler oder Datei
- Filter: Status (Alle / Hochgeladen / Fehler), Playlist, Zeitraum, Ordner
- Schnellaktionen pro Zeile: auf ballchasing öffnen, Menü mit weiteren Aktionen
- Endstand mit Teams, Sieg/Niederlage, Map, Playlist, Dauer und Uhrzeit
- Spielerstatistik: Punkte, Tore, Vorlagen, Paraden, Schüsse; User selbst ist hervorgehoben
- Ordner und Sichtbarkeit nachträglich ändern
- Replay-Detail von der Ballchasing API
    {
        Was die API zu einem Replay liefert (GET /api/replays/{id})
        Kopfdaten
        id, link, title, rocket_league_id, match_guid
        created (Upload-Zeitpunkt), date (Spielzeitpunkt), date_has_timezone
        uploader mit steam_id, name, profile_url, avatar, dazu recorder
        status (ok, pending oder failed), visibility
        match_type, playlist_id, playlist_name, team_size, season, season_type
        map_code, map_name
        duration, play_duration, overtime, laut Doku bei Verlängerung zusätzlich overtime_seconds
        server mit name und region
        groups[] mit id, name, link

        Pro Team (blue und orange): color, laut Doku auch name, dazu stats mit diesen Gruppen:
        ball: possession_time, time_in_side
        core: shots, shots_against, goals, goals_against, saves, assists, score, shooting_percentage, epic_saves, clears, centers, aerial_hits, first_touches, flip_resets, crossbar_hits, bicycle_hits, juggle_hits
        boost: bpm, bcpm, avg_amount, amount_collected und amount_stolen (jeweils gesamt, _big, _small), count_collected_big/_small, count_stolen_big/_small, amount_overfill, amount_overfill_stolen, amount_used_while_supersonic, time_zero_boost, time_full_boost, time_boost_0_25, _25_50, _50_75, _75_100
        movement: total_distance, time_supersonic_speed, time_boost_speed, time_slow_speed, time_ground, time_low_air, time_high_air, time_powerslide, count_powerslide
        positioning: time_defensive_third, time_neutral_third, time_offensive_third, time_defensive_half, time_offensive_half, time_behind_ball, time_infront_ball
        demo: inflicted und taken, jeweils gesamt sowie _in_defensive_half und _in_offensive_half

        Pro Spieler
        Allgemein: name, id mit platform und id, start_time und end_time (wann der Spieler im Match war), mvp, car_id, car_name, steering_sensitivity
        camera: fov, height, pitch, distance, stiffness, swivel_speed, transition_speed
        stats: dieselben Gruppen wie beim Team, ohne ball, dafür mit zusätzlichen Feldern:
        core: zusätzlich mvp
        boost: zusätzlich die Prozentwerte percent_zero_boost, percent_full_boost, percent_boost_0_25 bis _75_100
        movement: zusätzlich avg_speed, avg_speed_percentage, avg_powerslide_duration sowie die Prozentwerte percent_slow_speed, percent_boost_speed, percent_supersonic_speed, percent_ground, percent_low_air, percent_high_air
        positioning: zusätzlich avg_distance_to_ball (gesamt, _possession, _no_possession), avg_distance_to_mates, time_most_back, time_most_forward, time_closest_to_ball, time_farthest_from_ball, goals_against_while_last_defender und zu allen Zeiten ein percent_-Gegenstück

        Laut Doku fehlen manche Felder bei älteren Replays, etwa time_wall und percent_wall.
    }

- Zeitraum: 7 Tage, 14 Tage, Monat, Gesamt
    - Kennzahlen: Uploads, Siegquote
    - Spiele pro Tag mit Siegen und Niederlagen
    - Verteilung nach Playlist, beste Maps mit Siegquote

- Einstellungen
    - API-Key von ballchasing (verdeckt, anzeigen, trennen) mit Verbindungsstatus
    - Replay-Ordner wählen;
    - Einstellung: Bereits vorhandene Replays beim Start nachträglich hochladen
    - Einstellung: Replays nach Hochladen automatisch Löschen
    - Shortcut-Taste festlegen können, nur senden, wenn Rocket League im Vordergrund ist
    - Standard-Sichtbarkeit (Öffentlich / Nicht gelistet / Privat)
    - Mit Windows starten, 
    - Hinweis nach jedem Upload Ja/Nein, 
    - minimiert starten, 
    - Änderungen werden sofort gespeichert

- Erst-Einrichtung (Assistent in 5 Schritten)
    - ballchasing verbinden (Token eingeben) → Replay-Ordner (vorhandene Replays werden gezählt) → Shortcut mit Test-Knopf → Upload & Ordner → alte Replays nachladen
    - „Später einrichten“

- Tray
    - Menü: Pausieren, manuell hochladen, Ordner öffnen, App bzw. Web-Dashboard öffnen, Einstellungen, Beenden
    - Toasts für „hochgeladen“, „fehlgeschlagen“ und „Session beendet“, jeweils mit Aktionen, jeweils mit Einstellungsmöglichkeit dass diese nicht kommen
    - Schnell einordnen mit Countdown für die automatische Einordnung, Alternativen per Taste 1–4, auch im Spiel per Strg+Alt+1–4
    - Web-Dashboard

- Ordnerbaum mit Unterordnern und Anzahl, neue Ordner anlegen, Abgleich mit den ballchasing-Gruppen
    - Ordner „Unsortiert“
    - Mehrfachauswahl mit Verschieben, Sichtbarkeit und Löschen
    - Drag & Drop in Ordner, oder über Baum Menü
    - Intelligente Vorschläge (z. B. „4 private Spiele → Privat & Training“), übernehmen oder ignorieren
    - ganze Sessions auf einmal einordnen oder per Vorschlag aufteilen

- Regeln & Eingang (1c)
    - Eingang für alles, was keine Regel einsortiert hat, mit Vorschlag pro Replay
    - Einordnen per Tastatur: 1–4 Ordner, N neuer Ordner, E überspringen, J/K navigieren, Enter übernimmt den Vorschlag, O öffnet ballchasing
    - Regeln nach Playlist und Mitspielern; die App schlägt neue Regeln aus deinem Verhalten vor