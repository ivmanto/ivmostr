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
	session *services.Session
	pool    *gopool.Pool
	repo    nostr.NostrRepo
	lrepo   nostr.ListRepo
	IPCount map[string]int = make(map[string]int)
	mu      sync.Mutex
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
	hst := r.Header.Get("Host")
	fmt.Printf("DEBUG: ... Host Header value: %v, Origin: %v\n", hst, org)

	upgrader := websocket.Upgrader{
		Subprotocols:      []string{"nostr"},
		EnableCompression: true,
		CheckOrigin: func(r *http.Request) bool {
			if org == "" && hst == "" {
				return false
			}
			trustedOrigins := []string{"nostr.ivmanto.dev", "nostr.watch", "localhost", "127.0.0.1"}
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
		fmt.Printf("[connman]: CRITICAL error upgrading the connection to websocket protocol: %v\n", err)
		return
	}

	ip := tools.GetIP(r)
	mu.Lock()
	IPCount[ip]++
	mu.Unlock()
	h.lgr.Printf("[MW-ipc] [+] client IP %s increased to %d active connection\n", ip, IPCount[ip])

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
				fmt.Printf("ERROR: client %s closed the connection. %v\n", client.IP, err)
			} else {
				fmt.Printf("ERROR client-side %s (HandleWebSocket): %v\n", client.IP, err)
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
	if ip != "" && IPCount[ip] > 0 {
		mu.Lock()
		IPCount[ip]--
		mu.Unlock()
		fmt.Printf("[ws-handle] [-] Closing client IP %s decreased active connections to %d\n", ip, IPCount[ip])
	}
}
