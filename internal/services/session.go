package services

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/logging"
	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/pkg/gopool"
	"github.com/dasiyes/ivmostr-tdd/tools"
	"github.com/gorilla/websocket"
	gn "github.com/nbd-wtf/go-nostr"
)

var (
	NewEvent = make(chan *gn.Event)
)

// Session represents a WebSocket session that handles multiple client connections.
type Session struct {
	mu      sync.Mutex
	seq     uint
	clients []*Client
	ns      map[string]*Client
	bq      map[string]gn.Event
	out     chan []byte
	ilgr    *log.Logger
	elgr    *log.Logger
	clgr    *logging.Logger
	pool    *gopool.Pool
}

// NewSession creates a new WebSocket session.
func NewSession(pool *gopool.Pool) *Session {
	session := Session{
		ns:   make(map[string]*Client),
		bq:   make(map[string]gn.Event),
		out:  make(chan []byte, 1),
		ilgr: log.New(os.Stdout, "[ivmws][info] ", log.LstdFlags),
		elgr: log.New(os.Stderr, "[ivmws][error] ", log.LstdFlags),
		clgr: nil,
		pool: pool,
	}

	// [ ]: review and rework the event broadcaster
	go session.NewEventBroadcaster()

	return &session
}

// HandleWebSocket handles incoming WebSocket connections.
// func (s *Session) HandleWebSocket(client *Client) error {
// 	s.TuneClientConn(client)
// 	s.ilgr.Printf("DEBUG: client-IP %s (handle-websocket) as: %s, client conn (RemoteAddr): %v", client.IP, client.name, client.conn.RemoteAddr())
// 	//s.pool.Schedule(func() {
// 	if err := client.ReceiveMsg(); err != nil {
// 		s.elgr.Printf("ERROR client-side %s (start): %v", client.IP, err)
// 		s.Remove(client)
// 		return err
// 	}
// 	return nil
// }

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

	s.clients = append(s.clients, &client)
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
	if _, ok := s.ns[client.name]; !ok {
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
func (s *Session) TuneClientConn(client *Client) {
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

	client.conn.SetPingHandler(func(appData string) error {
		if appData == "ping" || appData == "PING" || appData == "Ping" {
			fmt.Println("Received ping:", appData)
			// Send a pong message in response to the ping
			err := client.conn.WriteMessage(websocket.PingMessage, []byte("pong"))
			if err != nil {
				client.lgr.Printf("ERROR client-side %s (ping-handler): %v", client.IP, err)
			}
		}
		return nil
	})
}

func (s *Session) BroadcasterQueue(e gn.Event) {
	// [ ]: form a queue of events to be broadcasted
	s.mu.Lock()
	s.bq[e.ID] = e
	s.mu.Unlock()
	NewEvent <- &e
}

// [ ]: to re-work the event broadcaster
func (s *Session) NewEventBroadcaster() {
	for {
		e := <-NewEvent
		if e != nil && e.Kind != 22242 {
			for _, client := range s.ns {
				if client.Subscription_id == "" {
					continue
				}
				//nip-04 requires clients authentication before sending kind:4 encrypted Dms
				if e.Kind == 4 && !client.Authed {
					continue
				} else if e.Kind == 4 && client.Authed {
					tag := e.Tags[0]
					recp := strings.Split(tag[1], ",")
					if len(recp) > 1 {
						if client.npub == recp[1] {
							evs := []interface{}{e}
							err := client.write(&evs)
							if err != nil {
								client.lgr.Printf("ERROR: error while sending event to client: %v", err)
							}
							break
						}
					}
				}

				if filterMatch(e, client.GetFilters()) {
					evs := []interface{}{e}
					err := client.write(&evs)
					if err != nil {
						client.lgr.Printf("ERROR: error while sending event to client: %v", err)
						continue
					}
					continue
				}
				//client.lgr.Printf("No matching filter found for event: %v", e)
			}
		}
		//e = nil
	}
}

// Checking if the specific event `e` matches atleast one of the filters of customers subscription;
func filterMatch(e *gn.Event, filters []map[string]interface{}) bool {
	for _, filter := range filters {
		if tools.FilterMatchSingle(e, filter) {
			return true
		}
	}
	return false
}
