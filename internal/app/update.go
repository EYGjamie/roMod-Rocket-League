package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

// releaseAPI answers with the newest release on GitHub; tests point it
// elsewhere.
var releaseAPI = "https://api.github.com/repos/EYGjamie/roMod-Rocket-League/releases/latest"

type release struct {
	Tag    string `json:"tag_name"`
	URL    string `json:"html_url"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// asset is the download of the program for this system, "" if the release
// has none (macOS: the app bundle is installed by hand).
func (r *release) asset() string {
	name := fmt.Sprintf("romod-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, a := range r.Assets {
		if a.Name == name {
			return a.URL
		}
	}
	return ""
}

const updateEvery = 12 * time.Hour

// updateCheck is the last look for a release, kept across restarts.
type updateCheck struct {
	Version string    `json:"version"` // of this program then; another one looks again
	At      time.Time `json:"at"`
	Release *release  `json:"release,omitempty"`
}

// watchUpdates looks for a release newer than version, the running one,
// twice a day, counted across restarts, never while Rocket League runs.
func (a *App) watchUpdates(ctx context.Context, version string) {
	var last updateCheck
	json.Unmarshal([]byte(a.store.Meta("update_check")), &last)
	if last.Version != version || last.At.After(a.now()) {
		last = updateCheck{}
	}
	if last.Release != nil {
		a.foundRelease(last.Release, version)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(last.At.Add(updateEvery).Sub(a.now())):
		}
		if !a.waitGame(ctx) {
			return
		}
		rel, err := a.latestRelease(ctx)
		last = updateCheck{Version: version, At: a.now(), Release: last.Release}
		if err != nil {
			log.Printf("Suche nach Updates: %v", err)
		} else {
			last.Release = rel
			a.foundRelease(rel, version)
		}
		data, _ := json.Marshal(last)
		if err := a.store.SetMeta("update_check", string(data)); err != nil {
			log.Printf("Update-Suche speichern: %v", err)
		}
	}
}

// foundRelease offers a release unless it is the running version.
func (a *App) foundRelease(rel *release, version string) {
	if strings.TrimPrefix(rel.Tag, "v") == version {
		return
	}
	// ponytail: any other tag counts as newer, releases/latest never goes back
	if a.release.Swap(rel) == nil {
		log.Printf("Update auf %s verfügbar", rel.Tag)
	}
	a.events.publish("state")
}

func (a *App) latestRelease(ctx context.Context) (*release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseAPI, nil)
	if err != nil {
		return nil, err
	}
	res, err := a.opts.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub: HTTP %d", res.StatusCode)
	}
	var rel release
	return &rel, json.NewDecoder(res.Body).Decode(&rel)
}

// apiUpdate installs the new release and restarts. Without a download for
// this system it opens the release page instead.
func (a *App) apiUpdate(w http.ResponseWriter, r *http.Request) {
	rel := a.release.Load()
	if rel == nil {
		fail(w, errors.New("kein Update verfügbar"))
		return
	}
	asset := rel.asset()
	if asset == "" || a.opts.Restart == nil {
		a.desk.OpenBrowser(rel.URL)
		writeJSON(w, http.StatusOK, map[string]bool{"opened": true})
		return
	}
	exe, err := os.Executable()
	if err == nil {
		err = replaceExe(r.Context(), a.opts.HTTPClient, asset, exe)
	}
	if err != nil {
		fail(w, fmt.Errorf("Update: %w", err))
		return
	}
	log.Printf("Auf %s aktualisiert, Neustart", rel.Tag)
	ok(w)
	time.AfterFunc(500*time.Millisecond, a.opts.Restart) // let the answer reach the interface
}

// replaceExe downloads the new program next to exe and swaps it in. The
// running program moves aside to exe.old, which Windows allows too; the
// next start removes it.
func replaceExe(ctx context.Context, client *http.Client, url, exe string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Download: HTTP %d", res.StatusCode)
	}
	tmp, old := exe+".new", exe+".old"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, res.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Rename(old, exe)
		return err
	}
	return nil
}
