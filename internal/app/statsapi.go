package app

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"slices"
	"strconv"

	"github.com/EYGjamie/auto-replay-upload/internal/game"
)

// The Stats API is off in a fresh installation of the game. The setup and
// the settings find its ini file and switch it on, with administrator
// rights where the game lies under Program Files.

type statsINIView struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	On     bool   `json:"on"`
	Port   int    `json:"port"`
}

type statsAPIView struct {
	Connected bool           `json:"connected"`
	Running   bool           `json:"running"` // Rocket League runs: changes apply after a restart
	Port      int            `json:"port"`    // the port the tool connects to
	Files     []statsINIView `json:"files"`   // one per installation found
}

// gameDirs returns where the game may be installed.
func (a *App) gameDirs() []string {
	if a.opts.GameDirs != nil {
		return a.opts.GameDirs()
	}
	return game.GameDirs()
}

// statsPort returns the port of the configured Stats API address.
func (a *App) statsPort() int {
	_, p, err := net.SplitHostPort(a.config().StatsAPI)
	if n, err2 := strconv.Atoi(p); err == nil && err2 == nil && n > 0 {
		return n
	}
	return game.DefaultStatsPort
}

func (a *App) statsAPIView() statsAPIView {
	v := statsAPIView{Connected: a.statsAPI.Load(), Running: game.RocketLeagueRunning(), Port: a.statsPort(), Files: []statsINIView{}}
	for _, s := range game.StatsINIs(a.gameDirs()) {
		v.Files = append(v.Files, statsINIView{Path: s.Path, Exists: s.Exists, On: s.On(), Port: s.Port})
	}
	return v
}

func (a *App) apiStatsAPI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.statsAPIView())
}

// EnableStatsAPI switches the Stats API on in every installation found.
// Files below Program Files are written by a copy of the tool running with
// administrator rights, after one prompt for all of them. If the game
// listens on another port, the tool follows it.
func (a *App) EnableStatsAPI() error {
	port := a.statsPort()
	files := game.StatsINIs(a.gameDirs())
	if len(files) == 0 {
		return errors.New("Rocket League wurde nicht gefunden")
	}
	var admin []string
	for _, s := range files {
		if s.On() {
			continue
		}
		err := game.EnableStatsAPI(s.Path, port)
		switch {
		case err == nil:
			log.Printf("Stats API eingeschaltet in %s", s.Path)
		case errors.Is(err, fs.ErrPermission):
			admin = append(admin, s.Path)
		default:
			return err
		}
	}
	if len(admin) > 0 {
		args := []string{"-stats-api-port", strconv.Itoa(port)}
		for _, p := range admin {
			args = append(args, "-enable-stats-api", p)
		}
		if err := a.desk.RunElevated(args...); errors.Is(err, errors.ErrUnsupported) {
			return fmt.Errorf("keine Schreibrechte für %s", admin[0])
		} else if err != nil {
			return err
		}
		log.Printf("Stats API mit Administratorrechten eingeschaltet in %s", admin)
	}

	// follow a port the user set in the game
	for _, s := range game.StatsINIs(a.gameDirs()) {
		if s.On() && s.Port != port {
			addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(s.Port))
			if _, err := a.UpdateSettings([]byte(`{"stats_api_addr":` + strconv.Quote(addr) + `}`)); err != nil {
				return err
			}
			log.Printf("Stats API: Rocket League nutzt Port %d", s.Port)
			break
		}
	}
	return nil
}

func (a *App) apiEnableStatsAPI(w http.ResponseWriter, r *http.Request) {
	if err := a.EnableStatsAPI(); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, a.statsAPIView())
}

// autoEnableStatsAPI switches the Stats API on again at start where a game
// update switched it off, if the settings ask for that. Switching the
// setting on in the interface also switches the API on, with feedback there.
func (a *App) autoEnableStatsAPI() {
	off := func(s game.StatsINI) bool { return !s.On() }
	if !a.config().StatsAPIAuto || !slices.ContainsFunc(game.StatsINIs(a.gameDirs()), off) {
		return
	}
	if err := a.EnableStatsAPI(); err != nil {
		log.Printf("Stats API automatisch einschalten: %v", err)
		return
	}
	if game.RocketLeagueRunning() {
		log.Print("Rocket League läuft: Die Stats API wirkt nach dem nächsten Start des Spiels")
	}
	a.events.publish("statsapi")
}
