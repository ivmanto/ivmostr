package services

import (
	"log"
	"math/rand"
	"os"
	"strconv"
	"sync"

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
func (s *Session) HandleWebSocket(clnt *Client) {
	//defer clnt.conn.Close()

	for {
		// Read and handle incoming messages according to the Nostr protocol
		_, message, err := clnt.conn.ReadMessage()
		if err != nil {
			log.Println("Failed to read WebSocket message:", err)
			break
		}
		log.Println("Received message:", string(message))
		// Perform actions based on the Nostr protocol
		// ...

		// Write response back to the client
		// ...
	}

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
	return &client
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
