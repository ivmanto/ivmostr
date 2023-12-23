package ivmws

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/logging"
	"github.com/dasiyes/ivmostr-tdd/configs/config"
	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/internal/services"
	"github.com/dasiyes/ivmostr-tdd/pkg/gopool"
	"github.com/dasiyes/ivmostr-tdd/tools"
	"github.com/go-chi/chi"
	"github.com/gorilla/websocket"
)

var (
	session        *services.Session
	pool           *gopool.Pool
	repo           nostr.NostrRepo
	lrepo          nostr.ListRepo
	mu             sync.Mutex
	trustedOrigins = []string{"nostr.ivmanto.dev", "localhost", "127.0.0.1", "nostr.watch", "nostr.info", "nostr.band", "nostrcheck.me", "nostr"}
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
	session = services.NewSession(pool, cl)
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

	org := r.Header.Get("Origin")
	hst := tools.DiscoverHost(r)
	ip := tools.GetIP(r)

	h.lgr.Printf("WSU-REQ: ... ip: %v, Host: %v, Origin: %v", ip, hst, org)

	upgrader := websocket.Upgrader{
		Subprotocols:      []string{"nostr"},
		EnableCompression: true,
		CheckOrigin: func(r *http.Request) bool {
			for _, v := range trustedOrigins {
				if strings.Contains(org, v) || strings.Contains(hst, v) {
					return true
				}
			}
			return false
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.lgr.Printf("[connman]: CRITICAL error upgrading the connection to websocket protocol: %v", err)
		return
	}

	mu.Lock()
	tools.IPCount[ip]++
	mu.Unlock()
	h.lgr.Printf("[connman] [+] client IP %s increased to %d active connection", ip, tools.IPCount[ip])

	_ = pool.ScheduleTimeout(time.Millisecond, func() {
		handle(conn, ip, org, hst)
	})
}

func handle(conn *websocket.Conn, ip, org, hst string) {

	client := session.Register(conn, repo, lrepo, ip)

	fmt.Printf("[ivmws][handle]: client %v connected from [%v], Origin: [%v], Host: [%v]\n", client.Name(), ip, org, hst)

	session.TuneClientConn(client)

	// Schedule Client connection handling into a goroutine from the pool
	pool.Schedule(func() {
		if err := client.ReceiveMsg(); err != nil {
			if strings.Contains(err.Error(), "websocket: close 1001") {
				fmt.Printf("[wsh] ERROR: client %s closed the connection. %v\n", client.IP, err)
			} else {
				fmt.Printf("[wsh] ERROR: client-side %s (HandleWebSocket): %v\n", client.IP, err)
			}

			RemoveIPCount(ip)
			conn.Close()
			return
		}
	})
}

// Handling clients' IP addresses
func RemoveIPCount(ip string) {
	mu := sync.Mutex{}
	if ip != "" && tools.IPCount[ip] > 0 {
		mu.Lock()
		tools.IPCount[ip]--
		mu.Unlock()
		fmt.Printf("[wsh] [-] Closing client IP %s decreased active connections to %d\n", ip, tools.IPCount[ip])
	}
}
