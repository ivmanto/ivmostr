package services

import (
	"log"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"cloud.google.com/go/logging"
	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/pkg/ivmpool"
	"github.com/gorilla/websocket"
)

// Session represents a WebSocket session that handles multiple client connections.
type Session struct {
	mu      sync.Mutex
	seq     uint
	clients []*Client
	ns      map[string]*Client
	out     chan []byte
	ilgr    *log.Logger
	elgr    *log.Logger
	clgr    *logging.Logger
	pool    *ivmpool.GoroutinePool
}

// NewSession creates a new WebSocket session.
func NewSession() *Session {
	pool := ivmpool.NewGoroutinePool(10)
	return &Session{
		pool: pool,
		ns:   make(map[string]*Client),
		out:  make(chan []byte, 1),
		ilgr: log.New(os.Stdout, "[ivmws][info] ", log.LstdFlags),
		elgr: log.New(os.Stderr, "[ivmws][error] ", log.LstdFlags),
		clgr: nil,
	}
}

// HandleWebSocket handles incoming WebSocket connections.
func (s *Session) HandleWebSocket(client *Client) {

	//s.tuneClientConn(client)

	s.ilgr.Printf("DEBUG: client-IP %s (handle-websocket) as: %s, client conn (RemoteAddr): %v", client.IP, client.name, client.conn.RemoteAddr())

	//s.pool.Schedule(func() {
	// if err := client.Start(); err != nil {
	// [ ]: classify the retiurned errors and handle the connections accordingly
	client.Start()
	s.Remove(client)

	//}
	//})

}

// Register upgraded websocket connection as client in the sessions
func (s *Session) Register(
	conn *websocket.Conn, rp nostr.NostrRepo, lrepo nostr.ListRepo, ip string) *Client {

	client := Client{
		session:   s,
		conn:      conn,
		lgr:       log.New(os.Stderr, "[client] ", log.LstdFlags),
		repo:      rp,
		lrepo:     lrepo,
		IP:        ip,
		Authed:    false,
		errorRate: make(map[string]int),
	}

	s.mu.Lock()
	{
		client.id = s.seq
		client.name = s.randName()

		s.clients = append(s.clients, &client)
		s.ns[client.name] = &client

		s.seq++
	}
	s.mu.Unlock()

	s.clients = append(s.clients, NewClient(s, conn))
	s.ilgr.Printf("DEBUG: client-IP %s (register) as: %s", client.IP, client.name)

	return &client
}

// Remove removes client from session.
func (s *Session) Remove(client *Client) {
	s.mu.Lock()
	if !s.remove(client) {
		return
	}
	s.mu.Unlock()

	// if !removed {
	// 	return
	// }

	//_ = s.Broadcast("goodbye", Object{
	//	"name": client.name,
	//	"time": timestamp(),
	//})
}

// Give code-word as name to the client connection
func (s *Session) randName() string {
	var suffix string
	for {
		name := codewords[rand.Intn(len(codewords))] + suffix
		if _, has := s.ns[name]; !has {
			return name
		}
		suffix += strconv.Itoa(rand.Intn(10))
	}
	// return ""
}

// mutex must be held.
func (s *Session) remove(client *Client) bool {
	if _, has := s.ns[client.name]; !has {
		return false
	}

	delete(s.ns, client.name)

	i := sort.Search(len(s.clients), func(i int) bool {
		return s.clients[i].id >= client.id
	})
	if i >= len(s.clients) {
		panic("session: inconsistent state")
	}

	without := make([]*Client, len(s.clients)-1)
	copy(without[:i], s.clients[:i])
	copy(without[i:], s.clients[i+1:])
	s.clients = without

	return true
}

// tuneClientConn tunes the client connection parameters
func (s *Session) tuneClientConn(client *Client) {
	// Set t value to 0 to disable the read deadline
	var t time.Time = time.Time{}

	err := client.conn.SetReadDeadline(t)
	if err != nil {
		s.elgr.Printf("ERROR client-side %s (set-read-deadline): %v", client.IP, err)
	}
	err = client.conn.SetWriteDeadline(t)
	if err != nil {
		s.elgr.Printf("ERROR client-side %s (set-write-deadline): %v", client.IP, err)
	}

	client.conn.SetCloseHandler(func(code int, text string) error {
		client.lgr.Printf("DEBUG: Closing client %v, code: %v, text: %v", client.conn.RemoteAddr().String(), code, text)
		return nil
	})
}
