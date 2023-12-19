package ivmws

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/logging"
	"github.com/dasiyes/ivmostr-tdd/configs/config"
	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/internal/services"
	"github.com/dasiyes/ivmostr-tdd/pkg/gopool"
	"github.com/go-chi/chi"
	"github.com/gorilla/websocket"
)

var (
	session *services.Session
	pool    *gopool.Pool
	repo    nostr.NostrRepo
	lrepo   nostr.ListRepo
)

type WSHandler struct {
	lgr  *log.Logger
	clgr *logging.Logger
	cfg  *config.ServiceConfig
}

func NewWSHandler(
	l *log.Logger,
	cl *logging.Logger,
	_repo nostr.NostrRepo,
	_lrepo nostr.ListRepo,
	cfg *config.ServiceConfig,

) *WSHandler {

	hndlr := WSHandler{
		lgr:  l,
		clgr: cl,
		cfg:  cfg,
	}
	// Setting up the repositories
	repo = _repo
	lrepo = _lrepo
	// Setting up the websocket session and pool
	workers := cfg.PoolMaxWorkers
	queue := cfg.PoolQueue
	spawn := 1
	pool = gopool.NewPool(workers, queue, spawn)
	session = services.NewSession(pool)
	session.SetConfig(cfg)
	return &hndlr
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
		http.Error(w, fmt.Sprintf("Error while upgrading connection: %v", err), http.StatusBadRequest)
		return
	}

	ip := r.Header.Get("X-Real-IP")
	if ip == "" {
		ip = r.RemoteAddr
	}

	org := r.Header.Get("Origin")

	_ = pool.ScheduleTimeout(time.Millisecond, func() {

		handle(conn, ip, org)
	})
}

func handle(conn *websocket.Conn, ip, org string) {

	client := session.Register(conn, repo, lrepo, ip)

	fmt.Printf("DEBUG: client %v connected from [%v], Origin: [%v]\n", client.Name(), ip, org)

	session.TuneClientConn(client)

	// Schedule Client connection handling into a goroutine from the pool
	pool.Schedule(func() {
		if err := client.ReceiveMsg(); err != nil {
			if strings.Contains(err.Error(), "websocket: close 1001") {
				fmt.Printf("ERROR: client %s closed the connection. %v\n", client.IP, err)
			} else {
				fmt.Printf("ERROR client-side %s (HandleWebSocket): %v\n", client.IP, err)
			}

			session.Remove(client)
			conn.Close()
			return
		}
	})

}
