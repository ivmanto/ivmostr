package ivmws

import (
	"log"
	"net/http"

	"cloud.google.com/go/logging"
	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/internal/services"
	"github.com/dasiyes/ivmostr-tdd/pkg/ivmpool"
	"github.com/go-chi/chi"
	"github.com/gorilla/websocket"
)

type WSHandler struct {
	lgr     *log.Logger
	clgr    *logging.Logger
	repo    nostr.NostrRepo
	lrepo   nostr.ListRepo
	session *services.Session
	pool    *ivmpool.GoroutinePool
}

func NewWSHandler(l *log.Logger, cl *logging.Logger, repo nostr.NostrRepo, lrepo nostr.ListRepo) *WSHandler {
	pool := ivmpool.NewGoroutinePool(10)
	srvs := services.NewSession()
	srvs.SetPool(pool)

	return &WSHandler{l, cl, repo, lrepo, srvs, pool}
}

func (h *WSHandler) Router() chi.Router {
	rtr := chi.NewRouter()

	// Route the static files
	// fileServer := http.FileServer(http.FS(ui.Files))
	// rtr.Handle("/static/*", fileServer)

	rtr.Route("/", func(r chi.Router) {
		r.Get("/", h.connman)
	})

	return rtr
}

// connman takes care for the connection upgrade (incl handshake) and all the negotiated subprotocols
// and contions to the websocket server
func (h *WSHandler) connman(w http.ResponseWriter, r *http.Request) {

	upgrader := websocket.Upgrader{
		Subprotocols:      []string{"nostr"},
		EnableCompression: true,
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.lgr.Printf("Failed to upgrade connection: %v", err)
		return
	}
	//defer conn.Close()
	ip := r.Header.Get("X-Real-IP")

	client := h.session.Register(conn, h.repo, h.lrepo, ip)

	h.lgr.Printf("DEBUG: client %v connected from [%v], Origin: [%v]", client.Name(), client.IP, r.Header.Get("Origin"))

	// Schedule Client connection handling into a goroutine from the pool
	h.pool.Schedule(func() {
		if err := h.session.HandleWebSocket(client); err != nil {
			h.lgr.Printf("ERROR client-side %s (HandleWebSocket): %v", client.IP, err)
			h.session.Remove(client)
			return
		}
	})
}
