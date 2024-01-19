package services

import (
	"context"
	"fmt"
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
	log "github.com/sirupsen/logrus"
)

var (
	NewEvent     = make(chan *gn.Event, 100)
	Exit         = make(chan struct{})
	relay_access string
	clientCldLgr *logging.Client
	cclnlgr      *logging.Logger
	lep          = logging.Entry{Severity: logging.Notice, Payload: ""}
)

// Session represents a WebSocket session that handles multiple client connections.
type Session struct {
	mu      sync.Mutex
	seq     uint
	clients []*Client
	ns      sync.Map
	bq      sync.Map
	//out     chan []byte
	clgr *logging.Logger
	pool *gopool.Pool
	cfg  *config.ServiceConfig
	Repo nostr.NostrRepo
}

// NewSession creates a new WebSocket session.
func NewSession(pool *gopool.Pool, repo nostr.NostrRepo, cfg *config.ServiceConfig) *Session {

	// Initate Cloud logging
	ctx := context.Background()
	clientCldLgr, _ = logging.NewClient(ctx, cfg.Firestore.ProjectID)
	clientCldLgr.OnError = func(err error) {
		log.Printf("[cloud-logger] Error [%v] raised while logging to cloud logger", err)
	}
	clgr := clientCldLgr.Logger("ivmostr-cnn")

	session := Session{
		ns: sync.Map{},
		bq: sync.Map{},
		//out:  make(chan []byte, 1),
		clgr: clgr,
		pool: pool,
		Repo: repo,
		cfg:  cfg,
	}

	relay_access = cfg.Relay_access

	td, err := repo.TotalDocs()
	if err != nil {
		log.Printf("error getting total number of docs: %v ", err)
	}
	log.Printf("total events in the DB %d", td)

	// [x]: review and rework the event broadcaster
	go session.NewEventBroadcaster()

	return &session
}

// Register upgraded websocket connection as client in the sessions
func (s *Session) Register(conn *websocket.Conn, ip string) *Client {

	cclnlgr = clientCldLgr.Logger("ivmostr-clnops")

	client := Client{
		session:   s,
		conn:      conn,
		cclnlgr:   cclnlgr,
		IP:        ip,
		Authed:    false,
		errorRate: make(map[string]int),
		repo:      s.Repo,
	}

	client.lgr = &log.Logger{
		Out:   os.Stdout,
		Level: log.DebugLevel,
		Formatter: &log.JSONFormatter{
			DisableTimestamp:  true,
			DisableHTMLEscape: true,
		},
	}

	s.mu.Lock()
	{
		client.id = s.seq
		client.name = s.randName()

		s.clients = append(s.clients, &client)

		s.seq++
	}
	s.mu.Unlock()

	s.ns.Store(client.name, &client)

	switch relay_access {
	case "public":

		lep.Payload = fmt.Sprintf(`{"client":%d, "IP":"%s", "name": "%s", "active_clients_connected":%d, "ts":%d}`, client.id, client.IP, client.name, len(s.clients), time.Now().Unix())
		s.clgr.Log(lep)

	case "authenticated":
		// generate the challenge string required by nip-42 authentication protocol
		client.GenChallenge()

		// Challenge the client to send AUTH message
		client.writeAUTHChallenge()

	case "paid":
		// [ ]: 1) required authentication - check for it and 2) check for active payment
		client.writeCustomNotice("restricted: this relay provides paid access only. Visit https://relay.ivmanto.dev for more information.")

	default:
		// Unknown relay access type - by default it is publi
		client.writeCustomNotice(fmt.Sprintf("connected to ivmostr relay as `%v`", client.name))
	}
	return &client
}

// Remove removes client from session.
func (s *Session) Remove(client *Client) {
	s.mu.Lock()
	removed := s.remove(client)
	s.mu.Unlock()
	if !removed {
		log.Printf("[rmv]: * client with IP %v was NOT removed from session connections!\n", client.IP)
		return
	}

	// Only remove from IPCount if removed from the session
	s.mu.Lock()
	tools.IPCount[client.IP]--
	s.mu.Unlock()
	log.Printf("[rmv]: - %d active clients (s.clients), %d connected (IPCount)\n", len(s.clients), len(tools.IPCount))
}

// Give code-word as name to the client connection
func (s *Session) randName() string {
	var suffix string
	for {
		name := codewords[rand.Intn(len(codewords))] + suffix
		if _, has := s.ns.Load(name); !has {
			return name
		}
		suffix += strconv.Itoa(rand.Intn(10))
	}
}

// mutex must be held.
func (s *Session) remove(client *Client) bool {
	if _, ok := s.ns.Load(client.name); !ok {
		//return false
		log.Printf("[remove] Error getting client name")
	} else {
		s.ns.Delete(client.name)
		//delete(s.ns, client.name)
	}

	i := sort.Search(len(s.clients)-1, func(i int) bool {
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
	var twt time.Time = time.Time.Add(time.Now(), 2000*time.Millisecond)

	err := client.conn.SetReadDeadline(t)
	if err != nil {
		log.Printf("ERROR client-side %s (set-read-deadline): %v", client.IP, err)
	}

	// Set read message size limit as stated in the server_info.json file
	client.conn.SetReadLimit(int64(16384))

	err = client.conn.SetWriteDeadline(twt)
	if err != nil {
		log.Printf("ERROR client-side %s (set-write-deadline): %v", client.IP, err)
	}

	client.conn.SetCloseHandler(func(code int, text string) error {
		go s.Remove(client)
		client.conn.Close()
		client.lgr.Debugf("[close]: [-] Closing client %v, code: %v, text: %v", client.IP, code, text)
		return nil
	})

	client.conn.SetPingHandler(func(appData string) error {

		log.Printf("[SetPingHandler] Ping message received as: %v", appData)
		// Send a pong message back to the client
		err := client.conn.WriteControl(websocket.PongMessage, []byte("pong"), twt)
		if err != nil {
			log.Println("Error sending pong message:", err)
			return err
		}
		return nil
	})

	client.conn.SetPongHandler(func(appData string) error {
		log.Printf("[SetPongHandler] Pong message received as: %v", appData)
		if strings.ToLower(appData) != "pong" {
			go s.Remove(client)
			client.conn.Close()
			return fmt.Errorf(appData)
		}
		return nil
	})
}

func (s *Session) BroadcasterQueue(e gn.Event) {
	// [x]: form a queue of events to be broadcasted
	s.bq.Store(e.ID, e)
	NewEvent <- &e
}

// [x]: to re-work the event broadcaster
func (s *Session) NewEventBroadcaster() {

	for e := range NewEvent {

		log.Printf(" ...-= starting new event braodcasting =-...")

		// 22242 is auth event - not to be stored or published
		if e.Kind != 22242 {
			continue
		}

		be, err := tools.ConvertStructToByte(e)
		if err != nil {
			log.Printf("Error: [%v] while converting event to byte array!", err)
			continue
		}

		s.mu.Lock()

		s.ns.Range(func(key, value interface{}) bool {
			client := value.(*Client)
			if client.Subscription_id == "" || client.id == uint(e.GetExtraNumber("id")) {
				return true
			}
			//nip-04 requires clients authentication before sending kind:4 encrypted Dms
			if e.Kind == 4 && !client.Authed {
				return true
			} else if e.Kind == 4 && client.Authed {
				tag := e.Tags[0]
				recp := strings.Split(tag[1], ",")
				if len(recp) > 1 {
					if client.npub == recp[1] {
						msgwt <- be

						s.mu.Unlock()
						return false
					}
				}
			}

			if filterMatch(e, client.GetFilters()) {
				msgwt <- be
				return true
			}
			return true
		})
		s.mu.Unlock()

		s.bq.Delete(e.ID)
		continue
	}
}

func (s *Session) SetConfig(cfg *config.ServiceConfig) {
	s.cfg = cfg
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
