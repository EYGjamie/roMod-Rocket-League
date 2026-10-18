package app

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// hub fans out change notifications to the open interfaces (overlay and
// browser) over Server-Sent Events. A notification only names a topic;
// the page reloads what it shows.
type hub struct {
	mu   sync.Mutex
	subs map[chan string]struct{}
}

func newHub() *hub { return &hub{subs: map[chan string]struct{}{}} }

func (h *hub) publish(topic string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- topic:
		default: // slow client, it reloads on the next event anyway
		}
	}
}

func (h *hub) subscribe() (chan string, func()) {
	ch := make(chan string, 32)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

func (h *hub) serve(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	ch, done := h.subscribe()
	defer done()
	fmt.Fprint(w, ": verbunden\n\n")
	flusher.Flush()
	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case topic := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", topic)
		case <-keepAlive.C:
			fmt.Fprint(w, ": ping\n\n")
		}
		flusher.Flush()
	}
}
