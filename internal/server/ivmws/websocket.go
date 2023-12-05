package ivmws

import (
	"log"
	"net/http"

	"cloud.google.com/go/logging"
	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/internal/services"
	"github.com/go-chi/chi"
	"github.com/gorilla/websocket"
)

type WSHandler struct {
	lgr     *log.Logger
	clgr    *logging.Logger
	repo    nostr.NostrRepo
	lrepo   nostr.ListRepo
	session *services.Session
}

func NewWSHandler(l *log.Logger, cl *logging.Logger, repo nostr.NostrRepo, lrepo nostr.ListRepo) *WSHandler {
	srvs := services.NewSession()
	return &WSHandler{l, cl, repo, lrepo, srvs}
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
		Subprotocols: []string{"nostr"},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.lgr.Printf("Failed to upgrade connection: %v", err)
		return
	}
	//defer conn.Close()
	ip := r.Header.Get("X-Real-IP")

	client := h.session.Register(conn, h.repo, h.lrepo, ip)

	go h.session.HandleWebSocket(client)
}
