package ivmws

import (
	"net/http"
	_ "net/http/pprof"
	"strings"
	"sync"
	"time"

	"github.com/dasiyes/ivmostr-tdd/configs/config"
	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/internal/services"
	"github.com/dasiyes/ivmostr-tdd/pkg/gopool"
	"github.com/dasiyes/ivmostr-tdd/tools"
	"github.com/go-chi/chi"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

var (
	session *services.Session
	pool    *gopool.Pool
	wspool  *services.ConnectionPool
	clpool  *services.ClientsPool
)

type WSHandler struct {
	repo           nostr.NostrRepo
	cfg            *config.ServiceConfig
	trustedOrigins []string
	hlgr           *log.Logger
}

func NewWSHandler(repo nostr.NostrRepo, cfg *config.ServiceConfig) *WSHandler {

	trustedOrigins := cfg.TrustedOrigins
	if len(trustedOrigins) == 0 {
		// set default values only
		trustedOrigins = []string{"127.0.0.1", "localhost", "nostr.ivmanto.dev", "ivmanto.dev", "nostr"}
	}
	lgr := log.New()

	hndlr := WSHandler{
		repo:           repo,
		cfg:            cfg,
		trustedOrigins: trustedOrigins,
		hlgr:           lgr,
	}

	// Setting up the websocket session and pools
	workers := cfg.PoolMaxWorkers
	queue := cfg.PoolQueue
	spawn := 2
	// pool is a pool of go-routines avaialble
	// to handle Clients with their websocket conns
	pool = gopool.NewPool(workers, queue, spawn)

	// clpool - the clients pool should be same size as the go
	// routines pool, while each client connection will be running
	// in its own go-routine.
	clpool = services.NewClientsPool(cfg.PoolMaxWorkers)

	// The same as above is valid for websocket connections pool.
	wspool = services.NewConnectionPool(cfg.PoolMaxWorkers)
	session = services.NewSession(pool, repo, cfg, clpool, wspool)

	return &hndlr
}

func (h *WSHandler) Router() chi.Router {
	rtr := chi.NewRouter()

	// Route the static files
	// fileServer := http.FileServer(http.FS(ui.Files))
	// rtr.Handle("/static/*", fileServer)

	rtr.Route("/", func(r chi.Router) {
		r.Get("/", h.connman)
		r.Get("/hc", h.healthcheck)
	})

	return rtr
}

// healthcheck point for polling the server health
func (h *WSHandler) healthcheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// connman takes care for the connection upgrade (incl handshake) and all the negotiated subprotocols
// and contions to the websocket server
func (h *WSHandler) connman(w http.ResponseWriter, r *http.Request) {

	org := r.Header.Get("Origin")
	hst := tools.DiscoverHost(r)
	ip := tools.GetIP(r)

	if session.IsRegistered(ip) {
		h.hlgr.Infof("WSU-REQ: DUPLICATED request arived from already connected ip: %v, Host: %v, Origin: %v", ip, hst, org)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("Already connected"))
		return
	}

	h.hlgr.Infof("WSU-REQ: ... new request arived from ip: %v, Host: %v, Origin: %v", ip, hst, org)

	upgrader := websocket.Upgrader{
		Subprotocols:      []string{"nostr"},
		ReadBufferSize:    16384,
		WriteBufferSize:   4096,
		WriteBufferPool:   &sync.Pool{},
		EnableCompression: false,
		CheckOrigin: func(r *http.Request) bool {
			for _, v := range h.trustedOrigins {
				if strings.Contains(org, v) || strings.Contains(hst, v) {
					return true
				}
			}
			return false
		},
	}

	uc, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.hlgr.Warnf("[connman]: CRITICAL error upgrading the connection to websocket protocol: %v", err)
		return
	}

	// Get a connection from the pool
	conn := wspool.Get()
	conn.WS = uc

	client := session.Register(conn, ip)
	if client == nil {
		return
	}

	poolerr := pool.ScheduleTimeout(time.Duration(1)*time.Millisecond, func() {
		session.HandleClient(client)
	})
	if poolerr != nil {
		h.hlgr.Warnf("[connman]: CRITICAL error scheduling the client to the pool: %v", poolerr)
		return
	}
}
