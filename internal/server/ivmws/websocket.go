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
	l       = log.New()
	session *services.Session
	pool    *gopool.Pool
	//repo    nostr.NostrRepo
	//lrepo          nostr.ListRepo
	mu             sync.Mutex
	trustedOrigins = []string{"nostr.ivmanto.dev", "relay.ivmanto.dev", "localhost", "127.0.0.1", "nostr.watch", "nostr.info", "nostr.band", "nostrcheck.me", "flycat.club", "nostr"}
)

type WSHandler struct {
	repo nostr.NostrRepo
	cfg  *config.ServiceConfig
}

func NewWSHandler(repo nostr.NostrRepo, cfg *config.ServiceConfig) *WSHandler {

	hndlr := WSHandler{
		repo: repo,
		cfg:  cfg,
	}

	// Setting up the websocket session and pool
	workers := cfg.PoolMaxWorkers
	queue := cfg.PoolQueue
	spawn := 2
	pool = gopool.NewPool(workers, queue, spawn)
	session = services.NewSession(pool, repo, cfg)

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

	l.Printf("WSU-REQ: ... new request arived from ip: %v, Host: %v, Origin: %v", ip, hst, org)

	upgrader := websocket.Upgrader{
		Subprotocols:      []string{"nostr"},
		ReadBufferSize:    0,
		WriteBufferSize:   0,
		WriteBufferPool:   &sync.Pool{},
		EnableCompression: false,
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
		l.Printf("[connman]: CRITICAL error upgrading the connection to websocket protocol: %v", err)
		return
	}

	poolerr := pool.ScheduleTimeout(time.Millisecond, func() {
		handle(conn, ip, org, hst)
	})
	if poolerr != nil {
		l.Errorf("Error finding free go-routine for 1 ms. Error:%v", poolerr)
		w.WriteHeader(503)
		_, _ = w.Write([]byte("server not available"))
	}
}

func handle(conn *websocket.Conn, ip, org, hst string) {

	client := session.Register(conn, ip)

	mu.Lock()
	tools.IPCount[ip]++
	mu.Unlock()

	session.TuneClientConn(client)

	// Schedule Client connection handling into a goroutine from the pool
	pool.Schedule(func() {

		if err := client.ReceiveMsg(); err != nil {

			session.Remove(client)
			conn.Close()
			return
		}
	})
}

// Handling clients' IP addresses
func RemoveIPCount(ip string) {
	if ip != "" && tools.IPCount[ip] > 0 {
		mu.Lock()
		tools.IPCount[ip]--
		mu.Unlock()
		//fmt.Printf("[wsh] [-] Closing client IP %s decreased active connections to %d\n", ip, tools.IPCount[ip])
	}
}
