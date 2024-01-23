package services

import (
	"fmt"
	"math/rand"
	"os"
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
	NewEvent      = make(chan *gn.Event, 100)
	Exit          = make(chan struct{})
	monitorTicker = time.NewTicker(time.Second * 30)
	monitorClose  = make(chan struct{})
	relay_access  string
	clientCldLgr  *logging.Client
	cclnlgr       *logging.Logger
	lep           = logging.Entry{Severity: logging.Notice, Payload: ""}
)

// Session is global object in nostr relay. It's living over
// the entire relay's life-cycle and managing the incoming
// clients WebSocket connections.
type Session struct {
	Clients *ClientsPool
	mu      sync.Mutex
	seq     uint
	ns      sync.Map
	clgr    *logging.Logger
	slgr    *log.Logger
	pool    *gopool.Pool
	wspool  *ConnectionPool
	cfg     *config.ServiceConfig
	repo    nostr.NostrRepo
}

// NewSession creates a new WebSocket session at the time when the http server Handler WSHandler is created.
func NewSession(pool *gopool.Pool, repo nostr.NostrRepo, cfg *config.ServiceConfig, clp *ClientsPool, wspool *ConnectionPool) *Session {

	session := Session{
		Clients: clp,
		clgr:    initCloudLogger(cfg.Firestore.ProjectID, "ivmostr-cnn"),
		slgr:    log.New(),
		pool:    pool,
		wspool:  wspool,
		repo:    repo,
		cfg:     cfg,
	}

	relay_access = cfg.Relay_access

	td, err := repo.TotalDocs()
	if err != nil {
		session.slgr.Errorf("error getting total number of docs: %v ", err)
	}
	session.slgr.Infof("total events in the DB %d", td)

	// receives new nostr events messages to broadcast among the subscribers
	go session.NewEventBroadcaster()

	// [ ]: Session monitor:
	// On regular base (10min - 1 hour) to display:
	// *  the list of IP addresses ofregistered clients in session.ns
	// * SubscriptionID per each registred client
	// * Filters per SubscriptionID
	go session.Monitor()

	return &session
}

// Register upgraded websocket connection as client in the sessions
func (s *Session) Register(conn *Connection, ip string) {

	// register the clients IP in the ip-counter
	tools.IPCount.Add(ip)

	// initiate cloud Logger
	cclnlgr = clientCldLgr.Logger("ivmostr-clnops")

	client := s.Clients.Get()
	//defer s.Clients.Put(client)

	client.Conn = conn
	client.wsc = conn.WS
	client.IP = ip
	client.repo = s.repo
	client.cclnlgr = cclnlgr
	client.Authed = false
	client.errorRate = make(map[string]int)
	client.Relay_Host = s.cfg.Relay_Host
	client.default_limit_value = s.cfg.GetDLV()
	client.mu = sync.Mutex{}
	client.msgwt = make(chan []interface{}, 20)
	client.inmsg = make(chan []interface{}, 10)
	client.errRM = make(chan error)
	client.errCH = make(chan error)
	client.read_events = 0
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
		av, loaded := s.ns.LoadOrStore(client.IP, client)
		if loaded {
			s.slgr.Warnf("[Register] a connection from client [%v] already is registered as [%v].", client.IP, av)
			return
		}

		s.slgr.Infof("[Register] client from [%v] registered as [%v]", client.IP, client.name)
		s.seq++
	}
	s.mu.Unlock()

	// Fine-tune the client's websocket connection
	s.TuneClientConn(client)

	switch relay_access {
	case "public":

		lep.Payload = fmt.Sprintf(`{"client":%d, "IP":"%s", "name": "%s", "active_clients_connected":%d, "ts":%d}`, client.id, client.IP, client.name, s.Clients.Len(), time.Now().Unix())
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

	// Schedule Client connection handling into a goroutine from the pool
	s.pool.Schedule(func() {

		// Anonymous func to close the connection when Read/Write  raises an error.
		defer func() {
			// release the client resources
			s.Remove(client)
		}()

		if err := client.ReceiveMsg(); err != nil {
			s.slgr.Errorf("[handleClient-go-routine] ReceiveMsg got an error: %v", err)
		}
	})
}

// Remove removes client from session.
func (s *Session) Remove(client *Client) {

	if client == nil {
		return
	}

	// [ ]: Review what exactly more resources (than websocket connection) need to be released

	// Close the websocket connection properly
	err := client.wsc.Close()
	if err != nil {
		s.slgr.Errorln("[Remove] Error closing client's ws connection:", err)
	}

	// Remove the client from the session's internal register
	s.ns.Delete(client.IP)

	// Put the connection shell back in the pool
	s.wspool.Put(client.Conn)

	// Put the Client back in the client's pool
	s.Clients.Put(client)

	// Remove the client from IP counter
	tools.IPCount.Remove(client.IP)

	// Release client resources
	client.repo = nil
	client.cclnlgr = nil
	client.errorRate = nil
	client.msgwt = nil
	client.inmsg = nil
	client.errRM = nil
	client.errCH = nil
	client = nil
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

// tuneClientConn tunes the client connection parameters
func (s *Session) TuneClientConn(client *Client) {
	// Set t value to 0 to disable the read deadline
	var t time.Time = time.Time{}

	err := client.wsc.SetReadDeadline(t)
	if err != nil {
		log.Printf("ERROR client-side %s (set-read-deadline): %v", client.IP, err)
	}

	// Set read message size limit as stated in the server_info.json file
	client.wsc.SetReadLimit(int64(16384))

	// [!] IMPORTANT: DO NOT set timeout to the write!!!
	err = client.wsc.SetWriteDeadline(t)
	if err != nil {
		log.Errorf("[TuneClientConn] error for client %s (set-write-deadline): %v", client.IP, err)
	}

	// SetCloseHandler will be called by the reading methods when the client announced connection close event.
	// Full list of WebSocket Status Codes at: https://kapeli.com/cheat_sheets/WebSocket_Status_Codes.docset/Contents/Resources/Documents/index
	client.wsc.SetCloseHandler(func(code int, text string) error {
		var err error
		switch code {
		case 1000:
			err = fmt.Errorf("Client's normal close. Message:[%s]", text)
		case 1001:
			err = fmt.Errorf("Client's going away. Message:[%s]", text)
		case 1002:
			err = fmt.Errorf("Client's closing due to protocol error. Message:[%s]", text)
		case 1003:
			err = fmt.Errorf("Client's closing as unsupported. Data not accepted. Message:[%s]", text)
		case 1007:
			err = fmt.Errorf("Client's closing as unsupported payload. Message:[%s]", text)
		default:
			err = fmt.Errorf("Client's closing as other mean of close. Code:[%d]. Message:[%s]", code, text)
		}
		client.lgr.Errorf("[SetCloseHandler] Client sent closing websocket connection control message. Code:%d, Msg:%s.", code, text)
		return err
	})

	client.wsc.SetPingHandler(func(appData string) error {
		client.lgr.Debugf("[SetPingHandler] Ping message received as: %v", appData)
		// Send a pong message back to the client
		err := client.wsc.WriteControl(websocket.PongMessage, []byte(`"pong"`), time.Now().Add(time.Second*2))
		if err != nil {
			client.lgr.Errorf("[SetPingHandler] Error sending pong message: %v", err)
			return err
		}
		return nil
	})

	client.wsc.SetPongHandler(func(appData string) error {
		client.lgr.Debugf("[SetPongHandler] Pong message received as: %v", appData)
		ad := strings.ToLower(appData)
		if ad != "pong" && ad != `["pong"]` {
			err = fmt.Errorf("[SetPongHandler] received appData:%v. Not a valid pong message.", appData)
			client.lgr.Errorf("%v", err)
			return err
		}
		// suposed the received pong reply is successful
		return nil
	})
}

// [x]: to re-work the event broadcaster
func (s *Session) NewEventBroadcaster() {

	for e := range NewEvent {

		log.Printf(" ...-= starting new event braodcasting =-...")

		// 22242 is auth event - not to be stored or published
		if e.Kind != 22242 {
			continue
		}

		// be, err := tools.ConvertStructToByte(e)
		// if err != nil {
		// 	log.Printf("Error: [%v] while converting event to byte array!", err)
		// 	continue
		// }

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
						client.msgwt <- []interface{}{e}

						s.mu.Unlock()
						return false
					}
				}
			}

			if filterMatch(e, client.GetFilters()) {
				client.msgwt <- []interface{}{e}
				return true
			}
			return true
		})
		s.mu.Unlock()

		continue
	}
}

func (s *Session) SetConfig(cfg *config.ServiceConfig) {
	s.cfg = cfg
}

// Monitor is to run in yet another go-routine and show
// the session state in terms of clients and their activities.
func (s *Session) Monitor() {
	for {
		select {
		case <-monitorTicker.C:
			s.slgr.Info("... monitor triggered ...")
		case <-monitorClose:
			break
		}
	}
}

// Close should ensure proper session closure and
// make sure the resources are released - graceful shoutdown
func (s *Session) Close() bool {
	monitorClose <- struct{}{}
	<-Exit
	//[ ]TODO: release resources and gracefully close the session
	return true
}
