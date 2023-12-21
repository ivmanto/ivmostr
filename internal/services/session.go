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
	"github.com/dasiyes/ivmostr-tdd/configs/config"
	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/pkg/gopool"
	"github.com/dasiyes/ivmostr-tdd/tools"
	"github.com/gorilla/websocket"
	gn "github.com/nbd-wtf/go-nostr"
)

var (
	NewEvent = make(chan *gn.Event)
	Exit     = make(chan struct{})
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
	cfg     *config.ServiceConfig
}

// NewSession creates a new WebSocket session.
func NewSession(pool *gopool.Pool, cl *logging.Logger) *Session {
	session := Session{
		ns:   make(map[string]*Client),
		bq:   make(map[string]gn.Event),
		out:  make(chan []byte, 1),
		ilgr: log.New(os.Stdout, "[ivmws][info] ", log.LstdFlags),
		elgr: log.New(os.Stderr, "[ivmws][error] ", log.LstdFlags),
		clgr: cl,
		pool: pool,
	}

	// [x]: review and rework the event broadcaster
	go session.NewEventBroadcaster()

	// regular connection health checker
	go session.ConnectionHealthChecker()

	return &session
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

	switch s.cfg.Relay_access {
	case "public":

		s.clients = append(s.clients, &client)

		pld := fmt.Sprintf(`{"client":%d, "IP":"%s", "name": "%s", "ts":%d}`, client.id, client.IP, client.name, time.Now().Unix())

		s.ilgr.Printf("%v", pld)
		lep := logging.Entry{
			Severity: logging.Notice,
			Payload:  pld,
		}
		s.clgr.Log(lep)

	case "authenticated":
		// generate the challenge string required by nip-42 authentication protocol
		client.GenChallenge()

		// Challenge the client to send AUTH message
		_ = client.writeAUTHChallenge()

	case "paid":
		// [ ]: 1) required authentication - check for it and 2) check for active payment
		_ = client.writeCustomNotice("restricted: this relay provides paid access only. Visit https://relay.ivmanto.dev for more information.")

	default:
		// Unknown relay access type - by default it is public
		s.clients = append(s.clients, &client)
		_ = client.writeCustomNotice(fmt.Sprintf("connected to ivmostr relay as `%v`", client.name))
	}

	fmt.Printf(" - %d active clients connected\n", len(s.clients))

	return &client
}

// Remove removes client from session.
func (s *Session) Remove(client *Client) {
	s.mu.Lock()
	if !s.remove(client) {
		fmt.Printf(" * client with IP %v was NOT removed from session connections!\n", client.IP)
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
		stop <- struct{}{}
		client.lgr.Printf("[close]: [-] Closing client %v, code: %v, text: %v", client.IP, code, text)
		return nil
	})

	client.conn.SetPingHandler(func(appData string) error {
		if appData == "ping" || appData == "PING" || appData == "Ping" {
			fmt.Println("Received ping:", appData)
			// Send a pong message in response to the ping
			s.mu.Lock()
			err := client.conn.WriteMessage(websocket.PingMessage, []byte("pong"))
			s.mu.Unlock()
			if err != nil {
				client.lgr.Printf("ERROR client-side %s (ping-handler): %v", client.IP, err)
			}
		}
		return nil
	})
}

func (s *Session) BroadcasterQueue(e gn.Event) {
	// [x]: form a queue of events to be broadcasted
	s.mu.Lock()
	s.bq[e.ID] = e
	s.mu.Unlock()
	for _, v := range s.bq {
		NewEvent <- &v
	}
}

// [x]: to re-work the event broadcaster
func (s *Session) NewEventBroadcaster() {
	for {
		e := <-NewEvent
		if e != nil && e.Kind != 22242 {
			s.ilgr.Printf(" ...-= starting new event braodcasting =-...")
			for _, client := range s.ns {
				if client.Subscription_id == "" || client.name == e.GetExtraString("cname") {
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
							s.mu.Lock()
							err := client.write(&evs)
							s.mu.Unlock()
							if err != nil {
								client.lgr.Printf("ERROR [auth]: error while sending event (kind=4) to client: %v", err)
							}
							break
						}
					}
				}

				if filterMatch(e, client.GetFilters()) {
					evs := []interface{}{e}
					s.mu.Lock()
					err := client.write(&evs)
					s.mu.Unlock()
					if err != nil {
						client.lgr.Printf("ERROR: error while sending event to client: %v", err)
						continue
					}
				}
			}
			s.mu.Lock()
			delete(s.bq, e.ID)
			s.mu.Unlock()
			continue
		}
	}
}

func (s *Session) SetConfig(cfg *config.ServiceConfig) {
	s.cfg = cfg
}

func (s *Session) ConnectionHealthChecker() {

	var err error
	ticker := time.NewTicker(3 * time.Minute)
	pm := websocket.PingMessage
	dl := time.Millisecond * 100

	for {
		select {
		case <-ticker.C:
			fmt.Printf("   ...--- === ---...   ")
			for _, client := range s.ns {
				err = client.conn.WriteControl(pm, []byte("ping"), time.Time.Add(time.Now(), dl))
				if err != nil {
					fmt.Printf("Error sending ping message:%v\n", err)
					client.conn.Close()
					continue
				}

				// Read the pong message from the client
				mt, pongMsg, err := client.conn.ReadMessage()
				if err != nil {
					fmt.Printf("Error reading pong message:%v", err)
					client.conn.Close()
					continue
				}
				if mt != websocket.PongMessage || string(pongMsg) != "pong" {
					client.conn.Close()
				}
			}

		case <-Exit:
			break
		}
	}
}

// ================================ aux functions =================================
// Checking if the specific event `e` matches atleast one of the filters of customers subscription;
func filterMatch(e *gn.Event, filters []map[string]interface{}) bool {
	for _, filter := range filters {
		if tools.FilterMatchSingle(e, filter) {
			return true
		}
	}
	return false
}
