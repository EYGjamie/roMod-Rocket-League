package app

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/EYGjamie/roMod-Rocket-League/internal/config"
	"github.com/EYGjamie/roMod-Rocket-League/internal/mmr"
)

// devPassword unlocks the developer mode. It is no secret: the mode only
// keeps experimental features out of the way.
const devPassword = "romod-dev"

// MMR looks up ranks and wins of players with a second Epic account, see
// package mmr; tests use a fake.
type MMR interface {
	Login(ctx context.Context, code string) (name string, err error)
	Logout()
	SignedIn() bool
	Connect(ctx context.Context) error
	Close()
	Lookup(ctx context.Context, ids []string) (map[string]mmr.Player, error)
	Version() mmr.Version // of the game, as PsyNet expects it
}

// rlapiVersion is the game version last found on GitHub, kept across
// starts so the lookup works before the next look.
func (a *App) rlapiVersion() mmr.Version {
	var v mmr.Version
	json.Unmarshal([]byte(a.store.Meta("rlapi_version")), &v)
	return v
}

func (a *App) saveRlapiVersion(v mmr.Version) {
	data, _ := json.Marshal(v)
	if err := a.store.SetMeta("rlapi_version", string(data)); err != nil {
		log.Printf("Spielversion speichern: %v", err)
	}
	a.events.publish("settings")
}

// SetDevMode switches the developer mode; switching it on needs the
// password.
func (a *App) SetDevMode(on bool, password string) error {
	if on && password != devPassword {
		return errors.New("Falsches Passwort")
	}
	a.mu.Lock()
	old := a.cfg
	a.cfg.DevMode = on
	err := config.Save(a.opts.ConfigPath, a.cfg)
	cur := a.cfg
	a.mu.Unlock()
	if err != nil {
		return err
	}
	if on {
		log.Print("Entwicklermodus eingeschaltet")
	} else {
		log.Print("Entwicklermodus ausgeschaltet")
	}
	a.lobbyChanged(old, cur)
	a.events.publish("settings")
	a.events.publish("state")
	return nil
}

// lobbyChanged follows settings that switch the lobby lookup on or off.
func (a *App) lobbyChanged(old, cur config.Config) {
	a.bindHotkeys(old, cur)
	if !cur.LobbyOn() && old.LobbyOn() {
		a.lobby.hide()
		go a.mmr.Close()
	} else if cur.LobbyOn() && !old.LobbyOn() && a.statsAPI.Load() {
		go a.connectEpic()
	}
}

// saveEpic keeps the refresh token of the Epic account, which changes with
// every sign-in.
func (a *App) saveEpic(token, name string) {
	a.mu.Lock()
	a.cfg.EpicToken, a.cfg.EpicName = token, name
	err := config.Save(a.opts.ConfigPath, a.cfg)
	a.mu.Unlock()
	if err != nil {
		log.Printf("Epic-Anmeldung speichern: %v", err)
	}
}

// epicResult records how the last contact with Epic and Psyonix went.
func (a *App) epicResult(err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	a.mu.Lock()
	changed := a.epicErr != msg
	a.epicErr = msg
	a.mu.Unlock()
	if changed {
		if err != nil {
			log.Printf("MMR-Abfrage: %v", err)
		}
		a.events.publish("state")
	}
}

// connectEpic opens the connection when Rocket League starts, so the first
// lookup does not wait for the sign-in.
func (a *App) connectEpic() {
	if !a.config().LobbyOn() || !a.mmr.SignedIn() {
		return
	}
	ctx, cancel := context.WithTimeout(a.ctx, time.Minute)
	defer cancel()
	a.epicResult(a.mmr.Connect(ctx))
}

type epicView struct {
	Status   string `json:"status"` // none, expired, ok, error
	Name     string `json:"name"`
	Error    string `json:"error,omitempty"`
	LoginURL string `json:"login_url"`
	Version  string `json:"version"` // of the game the lookup uses
	Fetched  bool   `json:"fetched"` // newer than the built-in one, from rlapi on GitHub
}

func (a *App) epicInfo() epicView {
	a.mu.RLock()
	name, msg := a.cfg.EpicName, a.epicErr
	a.mu.RUnlock()
	game := a.mmr.Version().Game
	v := epicView{Status: "ok", Name: name, Error: msg, LoginURL: mmr.LoginURL(), Version: game, Fetched: game != mmr.Builtin().Game}
	switch {
	case !a.mmr.SignedIn() && name != "":
		v.Status = "expired"
	case !a.mmr.SignedIn():
		v.Status = "none"
	case msg != "":
		v.Status = "error"
	}
	return v
}

func (a *App) apiDevMode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	on := r.Method == http.MethodPost
	if on && !decode(w, r, &in) {
		return
	}
	if err := a.SetDevMode(on, in.Password); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, a.settingsView())
}

func (a *App) apiEpic(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.epicInfo())
}

func (a *App) apiEpicLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	name, err := a.mmr.Login(ctx, in.Code)
	if name == "" && err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	a.epicResult(err)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errBody("Angemeldet als "+name+", aber die Verbindung zu Rocket League klappt nicht: "+err.Error()))
		return
	}
	log.Printf("MMR-Abfrage: bei Epic angemeldet als %s", name)
	writeJSON(w, http.StatusOK, a.epicInfo())
}

// apiEpicOpen opens Epic's sign-in page in the browser, also from the
// overlay.
func (a *App) apiEpicOpen(w http.ResponseWriter, r *http.Request) {
	a.desk.OpenBrowser(mmr.LoginURL())
	ok(w)
}

func (a *App) apiEpicLogout(w http.ResponseWriter, r *http.Request) {
	a.mmr.Logout()
	a.epicResult(nil)
	log.Print("MMR-Abfrage: von Epic abgemeldet")
	writeJSON(w, http.StatusOK, a.epicInfo())
}
