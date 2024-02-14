package services

import (
	"context"
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
	"github.com/dasiyes/ivmostr-tdd/tools/metrics"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	gn "github.com/studiokaiji/go-nostr"
)

var (
	NewEvent      = make(chan *gn.Event, 100)
	Exit          = make(chan struct{})
	chEM          = make(chan eventClientPair, 300)
	monitorTicker = time.NewTicker(time.Minute * 10)
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
	ldg     *ledger
	clgr    *logging.Logger
	slgr    *log.Logger
	pool    *gopool.Pool
	wspool  *ConnectionPool
	cfg     *config.ServiceConfig
	repo    nostr.NostrRepo
	lists   nostr.ListRepo
}

// NewSession creates a new WebSocket session at the time when the http server Handler WSHandler is created.
func NewSession(pool *gopool.Pool, repo nostr.NostrRepo, lists nostr.ListRepo, cfg *config.ServiceConfig, clp *ClientsPool, wspool *ConnectionPool) *Session {

	session := Session{
		Clients: clp,
		clgr:    initCloudLogger(cfg.Firestore.ProjectID, "ivmostr-cnn"),
		slgr:    log.New(),
		pool:    pool,
		wspool:  wspool,
		repo:    repo,
		lists:   lists,
		cfg:     cfg,
		ldg:     NewLedger(),
	}

	relay_access = cfg.Relay_access

	session.slgr.SetLevel(log.ErrorLevel)

	session.slgr.SetFormatter(&log.JSONFormatter{
		DisableTimestamp: true,
	})

	//)
	td, err := repo.TotalDocs2()
	if err != nil {
		session.slgr.Errorf("error getting total number of docs: %v ", err)
	}
	session.slgr.Infof("total events in the DB %d", td)

	// receives new nostr events messages to broadcast among the subscribers
	go session.NewEventBroadcaster()

	// [x]: Session monitor:
	// On regular base (10min - 1 hour) to display:
	// * the list of IP addresses of registered clients in session.ldg
	// * SubscriptionID per each registred client
	// * Filters per SubscriptionID
	go session.Monitor()

	// [x]: Filter match
	// The filterMatch routine will be addressed by the NewEventBroadcaster
	// to supply the subscribed clients with the new arriving Events that
	// match the respective clients' filters.
	go filterMatch()

	return &session
}

func (s *Session) IsRegistered(ip string) bool {

	clnt := s.ldg.Get(ip)

	if clnt != nil {
		e := clnt.Conn.WS.WriteControl(websocket.PingMessage, []byte(`ping`), time.Time.Add(time.Now(), time.Second*1))
		if e != nil {
			s.slgr.Errorf("[IsRegistered] Error [%v] while pinging connection to [%v]", e, ip)
		}
		return true
	}
	return false
}

// Register upgraded websocket connection as client in the sessions
func (s *Session) Register(conn *Connection, ip string) *Client {

	// register the clients IP in the ip-counter
	tools.IPCount.Add(ip)

	// initiate cloud Logger
	cclnlgr = clientCldLgr.Logger("ivmostr-clnops")

	client := s.Clients.Get()

	client.Conn = conn
	client.IP = ip
	client.repo = s.repo
	client.cclnlgr = cclnlgr
	client.Authed = false
	client.CreatedAt = time.Now().Unix()
	client.errorRate = make(map[string]int)
	client.Relay_Host = s.cfg.Relay_Host
	client.default_limit_value = s.cfg.GetDLV()
	client.mu = sync.Mutex{}
	client.msgwt = make(chan []interface{}, 20)
	client.inmsg = make(chan []interface{}, 10)
	client.errFM = make(chan error)
	client.errCH = make(chan error)
	client.read_events = 0
	client.lgr = &log.Logger{
		Out:   os.Stdout,
		Level: log.ErrorLevel,
		Formatter: &log.JSONFormatter{
			DisableTimestamp:  true,
			DisableHTMLEscape: true,
		},
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	{
		client.id = s.seq
		s.seq++
		client.name = s.randName()

		var (
			clnt *Client
			ok   bool
		)

		key := fmt.Sprintf("%s:%s", client.name, client.IP)

		ok, clnt = s.ldg.Add(key, client)
		if !ok {
			s.slgr.Warnf("[Register] a connection from client [%v] already is registered as [%v].", client.IP, clnt)
			return nil
		}

		s.slgr.Infof("[Register] client from [%v] registered as [%v]", client.IP, client.name)
	}

	metrics.MetricsChan <- map[string]int{"connsActiveWSConns": 1}

	// Fine-tune the client's websocket connection
	s.TuneClientConn(client)

	switch relay_access {
	case "public":

		lep.Payload = fmt.Sprintf(`{"client":%d, "IP":"%s", "name": "%s", "active_clients_connected":%d, "ts":%d}`, client.id, client.IP, client.name, s.ldg.Len(), time.Now().Unix())
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

	return client
}

func (s *Session) HandleClient(client *Client) {

	if client == nil {
		s.slgr.Errorln("[HandleClient] client is nil")
		return
	}

	// Handle client
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

	fltrs := len(client.Filetrs)

	// [ ]: Review what exactly more resources (than websocket connection) need to be released

	e := client.Conn.WS.Close()
	if e != nil {
		s.slgr.Errorf("[Remove] error closing websocket connection: %v", e)
	}

	s.wspool.Put(client.Conn)

	// Put the Client back in the client's pool
	s.Clients.Put(client)

	// Remove the client from the session's internal register
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ldg.Remove(fmt.Sprintf("%s:%s", client.name, client.IP))

	// Remove the client from IP counter
	tools.IPCount.Remove(client.IP)

	// Release client resources
	client.repo = nil
	client.cclnlgr = nil
	client.errorRate = nil
	client.msgwt = nil
	client.inmsg = nil
	client.errFM = nil
	client.errCH = nil
	client = nil

	metrics.MetricsChan <- map[string]int{"clntNrOfSubsFilters": -fltrs, "clntSubscriptions": -1, "connsActiveWSConns": -1}
	s.slgr.Debugf(
		"*** [Remove] client [%v] removed from session.Sent metric: %v",
		client.IP, map[string]int{"clntNrOfSubsFilters": -fltrs, "clntSubscriptions": -1, "connsActiveWSConns": -1},
	)
}

// Give code-word as name to the client connection
func (s *Session) randName() string {
	var suffix string
	for {
		name := codewords[rand.Intn(len(codewords))] + suffix
		if clnt := s.ldg.Get(name); clnt == nil {
			return name
		}
		suffix += strconv.Itoa(rand.Intn(10))
	}
}

// tuneClientConn tunes the client connection parameters
func (s *Session) TuneClientConn(client *Client) {
	// Set t value to 0 to disable the read deadline
	var t time.Time = time.Time{}

	err := client.Conn.WS.SetReadDeadline(t)
	if err != nil {
		log.Printf("ERROR client-side %s (set-read-deadline): %v", client.IP, err)
	}

	// Set read message size limit as stated in the server_info.json file
	client.Conn.WS.SetReadLimit(int64(16384))

	// [!] IMPORTANT: DO NOT set timeout to the write!!!
	err = client.Conn.WS.SetWriteDeadline(t)
	if err != nil {
		log.Errorf("[TuneClientConn] error for client %s (set-write-deadline): %v", client.IP, err)
	}

	// SetCloseHandler will be called by the reading methods when the client announced connection close event.
	// Full list of WebSocket Status Codes at: https://kapeli.com/cheat_sheets/WebSocket_Status_Codes.docset/Contents/Resources/Documents/index
	client.Conn.WS.SetCloseHandler(func(code int, text string) error {

		client.lgr.Errorf("[SetCloseHandler] Client [%s] sent closing websocket connection control message. Code:%d, Msg:%s.", client.IP, code, text)
		err := fmt.Errorf("[SetCloseHandler] Client closed the ws connection. [%v]", client.IP)

		if errnc := client.Conn.WS.NetConn().Close(); errnc != nil {
			log.Printf("[SetCloseHandler] Error closing underlying network connection: %v", errnc)
		}

		client.errCH <- err
		return err
	})

	// [!] PING message is a control message and the text 'ping' is optional and can be an empty string. Valid is the message type!
	client.Conn.WS.SetPingHandler(func(appData string) error {
		client.lgr.Debugf("[SetPingHandler] the perr [%s] sent Ping message! Message text is: %s", client.IP, appData)
		// Send a pong message back to the client
		err := client.Conn.WS.WriteControl(websocket.PongMessage, []byte(`"pong"`), time.Now().Add(time.Second*2))
		if err != nil {
			client.lgr.Errorf("[SetPingHandler] Error sending pong message to [%s]: %v", err, client.IP)
			return err
		}
		client.lgr.Debugf("[SetPingHandler] Pong message sent successfuly to client [%s]", client.IP)
		return nil
	})

	// [!] PONG message is a control message and the text 'ping' is optional and can be an empty string. Valid is the message type!
	client.Conn.WS.SetPongHandler(func(appData string) error {
		client.lgr.Debugf("[SetPongHandler] the peer [%s] sent Pong message! Message text is: %s", client.IP, appData)
		return nil
	})
}

// [x]: to re-work the event broadcaster
func (s *Session) NewEventBroadcaster() {

	for e := range NewEvent {

		s.slgr.Debugf(" ...-= starting new event braodcasting =-...")

		metrics.MetricsChan <- map[string]int{"evntProcessedBrdcst": 1}

		// 22242 is auth event - not to be stored or published
		if e.Kind == 22242 {
			continue
		}

		for _, client := range s.ldg.subscribers {

			if client.id == uint(e.GetExtraNumber("id")) {
				continue
			}

			if client.Subscription_id == "" || len(client.Filetrs) < 1 {
				if time.Now().Unix()-client.CreatedAt > 300 {
					s.ldg.Remove(fmt.Sprintf("%s:%s", client.name, client.IP))
					tools.IPCount.Remove(client.IP)
				}
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
						client.msgwt <- []interface{}{e}
						break
					}
				}
			}

			ecp := eventClientPair{
				event:  e,
				client: client,
			}

			chEM <- ecp
			s.slgr.Debugf("[NewEventBroadcaster] sent ecp client [%v] to filterMatch channel chEM.", ecp.client.IP)

			// next subscribed client
			continue
		}
		// next event from the channel
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
			s.slgr.Infof("... ================ ...")

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

			go func(ctx context.Context) {
				select {
				case <-ctx.Done():
					s.slgr.Debugf("[Monitor] Gracefully shutting down session state.")
					return
				default:
					s.sessionState()
					cancel()
				}
			}(ctx)
			go func(ctx context.Context) {
				select {
				case <-ctx.Done():
					s.slgr.Debugf("[Monitor] Gracefully shutting down maxConnIP.")
					return
				default:
					getMaxConnIP()
					cancel()
				}
			}(ctx)

			s.slgr.Infof("... running session state ...")
		case <-monitorClose:
			break
		}
	}
}

// sessionState will get the list of registred clients with their attributes
func (s *Session) sessionState() {

	var (
		clnt_count = 0
		laf        = make(map[string]interface{})
	)

	s.mu.Lock()
	defer s.mu.Unlock()
	for key, client := range s.ldg.subscribers {

		if client == nil {
			s.slgr.Errorf("[session state] in key [%v] the value is not a client object!", key)
			client.errFM <- fmt.Errorf("[session state] Inconsistent client [%v] registration!", client.IP)
			s.ldg.Remove(key)
			continue
		}

		if client.Subscription_id == "" || len(client.Filetrs) == 0 {
			s.slgr.Debugf("[session state] Not subscribed client [%s]/[%s]", client.IP, client.name)
			if time.Now().Unix()-client.CreatedAt > 300 {
				s.ldg.Remove(client.IP)
				tools.IPCount.Remove(client.IP)
			}
			continue
		}

		laf[key] = client.GetFilters()

		s.slgr.WithFields(log.Fields{
			"clientID":   client.id,
			"clientName": client.name,
			"SubID":      client.Subscription_id,
			"Filters":    client.Filetrs,
		}).Debugf("[session state] %v", key)

		clnt_count++
		continue
	}

	tools.PullLAF(laf)

	s.slgr.Infof("[session state] total consistant clients:%v", clnt_count)
	s.slgr.Info("... session state complete ...")
}

// Close should ensure proper session closure and
// make sure the resources are released - graceful shoutdown
func (s *Session) Close() bool {
	monitorClose <- struct{}{}
	<-Exit
	//[ ]TODO: release resources and gracefully close the session
	return true
}

func getMaxConnIP() {

	tip, max := tools.IPCount.TopIP()
	metrics.MetricsChan <- map[string]interface{}{"connsTopDemandingIP": map[string]int{tip: max}}
}
