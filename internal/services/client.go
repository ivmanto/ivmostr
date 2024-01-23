package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"cloud.google.com/go/logging"
	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/tools"
	"github.com/gorilla/websocket"
	gn "github.com/nbd-wtf/go-nostr"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

var (
	leop = logging.Entry{Severity: logging.Info, Payload: ""}
)

type Client struct {
	Conn                *Connection
	mu                  sync.Mutex
	lgr                 *log.Logger
	cclnlgr             *logging.Logger
	repo                nostr.NostrRepo
	id                  uint
	name                string
	challenge           string
	npub                string
	Subscription_id     string
	Filetrs             []map[string]interface{}
	Relay_Host          string
	errorRate           map[string]int
	default_limit_value int
	read_events         int
	IP                  string
	Authed              bool
	responseRate        int64 // subscription suply response rate
	wrchrr              int64 // wrchrr = write channel response rate
	msgwt               chan []interface{}
	inmsg               chan []interface{}
	errFM               chan error
	errCH               chan error
}

func (c *Client) Name() string {
	return c.name
}

func (c *Client) ReceiveMsg() error {
	var errch error

	// [ ]: to be tested if it is filtering logs corectly
	c.lgr.Level = log.DebugLevel

	go func() {
		for {
			select {
			case c.errCH <- c.writeT():
			case <-c.errFM:
				c.Conn.WS.Close()
				break
			}
		}
	}()

	go func() {
		for {
			select {
			case c.errCH <- c.dispatcher():
			case <-c.errFM:
				c.Conn.WS.Close()
				break
			}
		}
	}()

	go func() {
		for errch = range c.errCH {
			if errch != nil {
				c.lgr.Errorf("[errCH] client %v rose an error: %v", c.IP, errch)
				break
			}
		}
	}()

	for {
		mt, p, err := c.Conn.WS.ReadMessage()
		if mt == -1 {
			c.errFM <- fmt.Errorf("Critical error: %v", err)
			c.Conn.WS.Close()
			errch = err
			break
		}
		if err != nil && err != io.EOF {
			c.lgr.Errorf("client %v mt:%d ...(ReadMessage)... returned error: %v", c.IP, mt, err)
			c.Conn.WS.Close()
			c.errFM <- err
			errch = err
			break
		}

		c.lgr.Debugf("read_status:%s, client:%s, message_type:%d, size:%d, ts:%d", "success", c.IP, mt, len(p), time.Now().UnixNano())
		c.inmsg <- []interface{}{mt, p}
	}

	return errch
}

func (c *Client) writeT() error {
	var (
		sb  []byte
		err error
	)

	for message := range c.msgwt {
		c.lgr.Infof("[range msgwt] received message in %d ms", time.Now().UnixMilli()-c.wrchrr)

		c.mu.Lock()
		err = c.Conn.WS.WriteJSON(message)
		c.mu.Unlock()

		if err != nil {
			c.lgr.Debugf("write_status:%s, client:%s, took:%d, size[B]: %d, error:%v", "error", c.IP, time.Now().UnixMilli()-c.wrchrr, len(sb), err)
			break
		}

		sb, _ = tools.ConvertStructToByte(message)
		c.lgr.Debugf("write_status:%s, client:%s, took:%d, size[B]:%d", "success", c.IP, time.Now().UnixMilli()-c.wrchrr, len(sb))
		c.wrchrr = 0
	}

	return err
}

func (c *Client) dispatcher() error {
	var (
		err error
	)

	for msg := range c.inmsg {

		c.lgr.Debugf("[disp] received message type:%d", msg[0].(int))

		if len(msg[1].([]byte)) == 0 {
			c.lgr.Debug("[disp] The received message has null payload!")
			continue
		}

		// msg is an enchance []interface{} by the ReadMsg method with the message type int as first element.
		switch msg[0].(int) {
		case websocket.TextMessage:

			if !utf8.Valid(msg[1].([]byte)) {
				text := "The received message is invalid utf8"
				_ = c.Conn.WS.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseInvalidFramePayloadData, text),
					time.Time{})
				return fmt.Errorf("[disp] %v", text)
			}

			nostr_msg := msg[1].([]byte)
			if string(msg[1].([]byte)[:1]) == "[" {
				go c.dispatchNostrMsgs(&nostr_msg)
				continue
			} else {
				// Artificial PING/PONG handler - for cases when the control message is not supported by the client(s)
				txtmsg := strings.ToLower(string(msg[1].([]byte)))
				c.wrchrr = time.Now().UnixMilli()

				if txtmsg == "ping" || txtmsg == `["ping"]` {
					c.msgwt <- []interface{}{`pong`}

				} else if txtmsg == `{"op":"ping"}` {
					c.wrchrr = time.Now().UnixMilli()
					var pr = make(map[string]interface{}, 1)
					pr["res"] = "pong"
					c.msgwt <- []interface{}{pr}

				} else {
					c.lgr.Debugf("[disp] Text message paylod is: %s", txtmsg)
					c.writeCustomNotice("not a nostr message")
				}
			}
			continue

		case websocket.BinaryMessage:
			c.lgr.Debug("[disp] received binary message! Processing not implemented")
			continue
		case websocket.CloseMessage:
			msgstr := string(msg[1].([]byte))
			c.lgr.Debugf("[disp] received a `close` message: %v from client %v", msgstr, c.IP)
			return fmt.Errorf("[disp] Client closing the connection with: [%v]", msgstr)

		case websocket.PingMessage:
			c.lgr.Debugf("[disp] Client %v sent PING message (mt = 9):%v", c.IP, string(msg[1].([]byte)))
			continue

		case websocket.PongMessage:
			c.lgr.Debugf("[disp] From client %v received PONG message (mt=10) %v", c.IP, string(msg[1].([]byte)))
			continue

		default:
			c.lgr.Debugf("[disp] Unknown message type: %v", msg[0].(int))
			continue

		}
	}
	return err
}

func (c *Client) dispatchNostrMsgs(msg *[]byte) {
	var (
		err error
	)

	jmsg, err := convertToJSON(*msg)
	if err != nil {
		c.lgr.Errorf("[dispatchNostrMsgs] provided payload [%s] is not a valid JSON. Error: %v", string(*msg), err)
		return
	}

	key, ok := jmsg[0].(string)
	if !ok {
		c.lgr.Errorf("[dispatchNostrMsgs] nostr message label [%v] is not a string!", jmsg[0])
	}

	switch key {
	case "EVENT":
		err = c.handlerEventMsgs(&jmsg)

	case "REQ":
		err = c.handlerReqMsgs(&jmsg)

	case "CLOSE":
		err = c.handlerCloseSubsMsgs(&jmsg)

	case "AUTH":
		err = c.handlerAuthMsgs(&jmsg)

	default:
		log.Printf("[dispatchNostrMsgs] Unknown message: %s", key)
		c.writeCustomNotice("Error: invalid format of the received message")
		return
	}

	if err != nil {
		c.lgr.Errorf("[dispatchNostrMsgs] A handlers' function returned Error: %v;  message type: %v", err, key)
	}
}

// ****************************** Messages types Handlers ***********************************************

func (c *Client) handlerEventMsgs(msg *[]interface{}) error {
	// OK messages MUST be sent in response to EVENT messages received from clients, they must have the 3rd parameter set to true when an event has been accepted by the relay, false otherwise. The 4th parameter MAY be empty when the 3rd is true, otherwise it MUST be a string containing a machine-readable single-word prefix followed by a : and then a human-readable message. The standardized machine-readable prefixes are: duplicate, pow, blocked, rate-limited, invalid, and error for when none of that fits.

	se, ok := (*msg)[1].(map[string]interface{})
	if !ok {
		lmsg := "invalid: unknown message format"
		c.writeEventNotice("0", false, lmsg)
		return fmt.Errorf("ERROR: invalid message. unknown format")
	}

	e, err := mapToEvent(se)
	if err != nil {
		// [!] protocol error
		c.errorRate[c.IP]++
		lmsg := "invalid:" + err.Error()
		c.writeEventNotice(e.ID, false, lmsg)
		return err
	}

	rsl, errv := e.CheckSignature()
	if errv != nil {
		lmsg := fmt.Sprintf("result %v, invalid: %v", rsl, errv.Error())
		c.writeEventNotice(e.ID, false, lmsg)
		return errv
	}

	switch e.Kind {
	case 3:
		// nip-02, kind:3 replaceable, Contact List (overwrite the exiting event for the same pub key)

		// sending the event to a channel, will hijack the event to the broadcaster
		// Keep it disabled until [ ]TODO: find a way to clone the execution context
		// NewEvent <- e

		err := c.repo.StoreEventK3(e)
		if err != nil {
			c.writeEventNotice(e.ID, false, composeErrorMsg(err))
			return err
		}
		c.writeEventNotice(e.ID, true, "")
		return nil

	case 4:
		// Even if the relay access level is public, the private encrypted messages kind=4 should be rejected
		if !c.Authed {
			c.writeCustomNotice("restricted: we can't serve DMs to unauthenticated users, does your client implement NIP-42?")
			c.writeEventNotice(e.ID, false, "Authentication required")
			return nil
		}

	default:
		// default will handle all other kinds of events that do not have specifics handlers
		// do nothing
	}

	err = c.repo.StoreEvent(e)
	if err != nil {
		c.writeEventNotice(e.ID, false, composeErrorMsg(err))
		return err
	}

	// The customer name is required in NewEventBroadcaster method in order to skip the broadcast process for the customer that brought the event on the relay.
	e.SetExtra("id", float64(c.id))

	// Send the new event to the broadcaster
	NewEvent <- e

	c.writeEventNotice(e.ID, true, "")
	return nil
}

func (c *Client) handlerReqMsgs(msg *[]interface{}) error {

	//["REQ", <subscription_id>, <filters JSON>...], used to request events and subscribe to new updates.
	// [x] <subscription_id> is an arbitrary, non-empty string of max length 64 chars, that should be used to represent a subscription.
	var subscription_id string
	var filter map[string]interface{}

	if len(*msg) >= 2 {
		subscription_id = (*msg)[1].(string)
		if subscription_id == "" || len(subscription_id) > 64 {
			//[!] protocol error
			c.errorRate[c.IP]++
			c.writeCustomNotice("Error: Invalid subscription_id")
			return fmt.Errorf("Error: Invalid subscription_id")
		}
	} else {
		c.writeCustomNotice("Error: Missing subscription_id")
		return fmt.Errorf("Error: Missing subscription_id")
	}

	// Identify subscription filters
	var msgfilters = []map[string]interface{}{}
	if len(*msg) >= 3 {
		for idx, v := range *msg {
			if idx > 1 && idx < 5 {
				filter = v.(map[string]interface{})
				msgfilters = append(msgfilters, filter)
			}
		}
	}

	// [x] Relays should manage <subscription_id>s independently for each WebSocket connection; even if <subscription_id>s are the same string, they should be treated as different subscriptions for different connections

	if c.Subscription_id != "" {
		if c.Subscription_id == subscription_id {
			// [x] OVERWRITE subscription fileters
			c.Filetrs = []map[string]interface{}{}
			c.Filetrs = append(c.Filetrs, msgfilters...)

			c.lgr.Printf("UPDATE: subscription id: %s with filter %v", c.Subscription_id, msgfilters)
			c.writeCustomNotice("Update: The subscription filter has been overwriten.")

			err := c.SubscriptionSuplier()
			if err != nil {
				return err
			}

			return nil
		} else {
			// [!] protocol error
			c.errorRate[c.IP]++
			em := fmt.Sprintf("Error: There is already an active subscription for this connection with subscription_id: %v, the new subID %v is ignored.", c.Subscription_id, subscription_id)
			c.writeCustomNotice(em)
			return fmt.Errorf("There is already active subscription")
		}
	}
	c.Subscription_id = subscription_id
	c.Filetrs = append(c.Filetrs, msgfilters...)

	c.writeCustomNotice(fmt.Sprintf("The subscription with filter %v has been created", msgfilters))
	log.Printf("NEW: subscription id: %s from [%s] with filter %v", c.IP, c.Subscription_id, msgfilters)

	err := c.SubscriptionSuplier()
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) handlerCloseSubsMsgs(msg *[]interface{}) error {

	//[x] remove subscription id

	if len(*msg) == 2 {
		subscription_id := (*msg)[1].(string)
		if c.Subscription_id == subscription_id {
			c.Subscription_id = ""
			c.Filetrs = []map[string]interface{}{}
			c.writeCustomNotice("Update: The subscription has been closed")
			return nil
		} else {
			c.writeCustomNotice("Error: Invalid subscription_id provided")
			return fmt.Errorf("Error: Invalid subscription_id provided")
		}
	} else {
		c.writeCustomNotice("Error: Bad format of CLOSE message")
		return fmt.Errorf("Error: Bad format of CLOSE message")
	}
}

func (c *Client) handlerAuthMsgs(msg *[]interface{}) error {
	// ["AUTH", <signed-event-json>] used to authenticate the client to the relay.

	se, ok := (*msg)[1].(map[string]interface{})
	if !ok {
		c.writeEventNotice("0", false, "invalid: unknown message format")
	}

	e, err := mapToEvent(se)
	if err != nil {
		c.writeEventNotice(e.ID, false, "invalid:"+err.Error())
		return err
	}

	// [x]: The signed event is an ephemeral event **not meant to be published or queried**,
	// [x]: It must be of `kind: 22242` and it should have at least **two tags**, one for the `relay URL` and one for the `challenge string` as received from the relay.
	// [x]: Relays MUST exclude kind: 22242 events from being broadcasted to any client.
	// [x]: created_at should be the current time (~ 10 mins max).

	rsl, errv := e.CheckSignature()
	if errv != nil {
		lmsg := fmt.Sprintf("result %v, invalid: %v", rsl, errv.Error())
		c.writeEventNotice(e.ID, false, lmsg)
		return errv
	}

	if e.Kind != 22242 {
		c.writeEventNotice(e.ID, false, "invalid: invalid AUTH message kind")
		return nil
	}

	var t1, t2 bool

	for _, v := range e.Tags {

		// [x]: Convert v[1] value to URL format and verify the domain & subdomain names only. Avoid path, query, schemas and fragment.
		if v[0] == "relay" && c.confirmURLDomainMatch(v[1]) {
			t1 = true
		}

		if v[0] == "challenge" && v[1] == c.challenge {
			t2 = true
		}
	}

	if t1 && t2 {
		now := gn.Timestamp(time.Now().Unix())
		age := gn.Timestamp(600)
		if e.CreatedAt+age > now {
			c.Authed = true
			c.npub = e.PubKey
			c.writeEventNotice(e.ID, true, "")
			return nil
		} else {
			c.writeEventNotice(e.ID, false, "Authentication failed")
			return fmt.Errorf("Authentication failed")
		}
	} else {
		c.writeEventNotice(e.ID, false, "Authentication failed")
		return fmt.Errorf("Authentication failed")
	}
}

// ****************************** Auxilary methods ***********************************************

// Protocol definition: ["OK", <event_id>, <true|false>, <message>], used to indicate acceptance or denial of an EVENT message.
//
// [x] `OK` messages MUST be sent in response to `EVENT` messages received from clients;
//
//	[x] they must have the 3rd parameter set to `true` when an event has been accepted by the relay, `false` otherwise.
//	[x] The 4th parameter MAY be empty when the 3rd is true, otherwise it MUST be a string containing a machine-readable single-word prefix followed by a : and then a human-readable message.
//	[ ] The standardized machine-readable prefixes are:
//		[x] `duplicate`,
//		[ ] `pow`,
//		[ ] `blocked`,
//		[ ] `rate-limited`,
//		[x] `invalid`, and
//		[x] `error` for when none of that fits.

func (c *Client) writeEventNotice(event_id string, accepted bool, message string) {
	c.wrchrr = time.Now().UnixMilli()
	c.msgwt <- []interface{}{"OK", event_id, accepted, message}
	c.lgr.Debugf("client [%v]: note sent to write in %d ms", c.IP, time.Now().UnixMilli()-c.wrchrr)
}

func (c *Client) writeCustomNotice(notice_msg string) {
	c.wrchrr = time.Now().UnixMilli()
	var note = []interface{}{"NOTICE", notice_msg}
	c.msgwt <- note
	c.lgr.Debugf("client [%v]: notice sent to write in %d ms", c.IP, time.Now().UnixMilli()-c.wrchrr)
}

// Protocol definition: ["EOSE", <subscription_id>], used to indicate that the relay has no more events to send for a subscription.
func (c *Client) writeEOSE(subID string) {
	c.wrchrr = time.Now().UnixMilli()
	var eose = []interface{}{"EOSE", subID}
	c.msgwt <- eose
	c.lgr.Debugf("client [%v]: EOSE sent to write in %d ms", c.IP, time.Now().UnixMilli()-c.wrchrr)
}

// Protocol definition: ["AUTH", <challenge>], (nip-42) used to request authentication from the client.
func (c *Client) writeAUTHChallenge() {
	c.wrchrr = time.Now().UnixMilli()
	var note = []interface{}{"AUTH", c.challenge}
	c.msgwt <- note
	c.lgr.Debugf("client [%v]: challenge sent to write in %d ms", c.IP, time.Now().UnixMilli()-c.wrchrr)
}

// Generate challenge for the client to authenticate.
func (c *Client) GenChallenge() {

	const charset = "abcdefghijklmnopqrstuvwxyz" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	var seededRand *rand.Rand = rand.New(
		rand.NewSource(time.Now().UnixNano()))

	b := make([]byte, 16)

	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}

	c.challenge = string(b)
}

// Get Filters returns the filters of the current client's subscription
func (c *Client) GetFilters() []map[string]interface{} {
	return c.Filetrs
}

func (c *Client) confirmURLDomainMatch(relayURL string) bool {

	// convert url string to URL object
	relURL, err := url.Parse(relayURL)
	if err != nil {
		c.writeCustomNotice("ERROR: parsing provided relay URL")
		return false
	}

	return relURL.Hostname() == c.Relay_Host
}

// *****************************************************************************
// 		Subscription Supplier method for initial subs load
// *****************************************************************************

func (c *Client) SubscriptionSuplier() error {

	ctx := context.Background()
	c.responseRate = time.Now().UnixMilli()
	c.read_events = 0

	c.lgr.WithFields(log.Fields{"method": "[SubscriptionSuplier]", "client": c.IP, "SubscriptionID": c.Subscription_id, "Filters": c.Filetrs}).Debug("Starting SubscriptionSuplier")

	err := c.fetchAllFilters(ctx)
	if err != nil {
		return err
	}

	c.lgr.WithFields(log.Fields{"method": "[SubscriptionSuplier]", "client": c.IP, "SubscriptionID": c.Subscription_id, "func": "fetchAllFilters", "took": time.Now().UnixMilli() - c.responseRate}).Debug("fetchAllFilters completed")

	// Send EOSE
	c.writeEOSE(c.Subscription_id)

	c.lgr.WithFields(log.Fields{"method": "[SubscriptionSuplier]", "client": c.IP, "SubscriptionID": c.Subscription_id, "func": "writeEOSE", "took": time.Now().UnixMilli() - c.responseRate}).Debug("writeEOSE completed")

	payloadV := fmt.Sprintf(`{"method":"[SubscriptionSuplier]","client":"%s","Filters":"%v","events":%d,"servedIn": %d}`, c.IP, c.Filetrs, c.read_events, time.Now().UnixMilli()-c.responseRate)
	leop.Payload = payloadV
	cclnlgr.Log(leop)

	return nil
}

// This will be running all filters per subscription in parallel
func (c *Client) fetchAllFilters(ctx context.Context) error {

	eg := errgroup.Group{}
	for idx, filter := range c.Filetrs {
		fltr := filter
		c.lgr.WithFields(log.Fields{"method": "[fetchAllFilters]", "client": c.IP, "SubscriptionID": c.Subscription_id, "filter": fltr, "##idx": idx}).Info("Parsing filters")

		eg.Go(func() error {
			return c.fetchData(fltr, &eg)
		})
	}

	// Wait for completion and return the first error (if any)
	return eg.Wait()
}

func (c *Client) fetchData(filter map[string]interface{}, eg *errgroup.Group) error {
	return func() error {
		// [ ]: de-compose the filter
		// ===================================== FILTER TEMPLATE ============================================

		//<filters> is a JSON object that determines what events will be sent in that subscription, it can have the following attributes:
		//{
		//  "ids": <a list of event ids>,
		//  "authors": <a list of lowercase pubkeys, the pubkey of an event must be one of these>,
		//  "kinds": <a list of a kind numbers>,
		//  "#<single-letter (a-zA-Z)>": <a list of tag values, for #e — a list of event ids, for #p — a list of event pubkeys etc>,
		//  "since": <an integer unix timestamp in seconds, events must be newer than this to pass>,
		//  "until": <an integer unix timestamp in seconds, events must be older than this to pass>,
		//  "limit": <maximum number of events relays SHOULD return in the initial query>
		//}
		//==================================================================================================

		var (
			err            error
			_events        []*gn.Event
			events         []gn.Event
			max_events     int
			_since, _until int64
		)

		// =================================== MAX_EVENTS ===============================================
		lim, flt_limit := filter["limit"].(float64)
		if !flt_limit {
			max_events = c.default_limit_value
		} else if lim == 0 || lim > 5000 {
			// until the paging is implemented for paying clients OR always for public clients
			// 5000 is the value from attribute from server_info limits
			max_events = 5000
		} else {
			// set the requested by the filter value
			max_events = int(lim)
		}

		// =================================== SINCE - UNTIL ===============================================
		if since, flt_since := filter["since"].(float64); !flt_since {
			_since = int64(0)
		} else {
			_since = int64(since)
		}
		if until, flt_until := filter["until"].(float64); !flt_until {
			_until = int64(0)
		} else {
			_until = int64(until)
		}

		// =================================== Parse the filter list components ===========================
		_, flt_authors := filter["authors"].([]interface{})
		_, flt_ids := filter["ids"].([]interface{})
		_, flt_kinds := filter["kinds"].([]interface{})
		var flt_tags bool
		for key, val := range filter {
			if strings.HasPrefix(key, "#") {
				_, flt_tags = val.([]interface{})
				break
			}
		}

		// =================================== FILTERS LIST ===============================================

		// =================================== Select the query case for the filter =======================
		//for key, val := range filter {

		events = []gn.Event{}

		switch {
		case flt_authors && !flt_ids && !flt_kinds && !flt_tags:
			// ONLY "authors" list ...

			var (
				authors  []interface{}
				_authors []string
			)

			authors, ok := filter["authors"].([]interface{})
			if !ok {
				return fmt.Errorf("Wrong filter format used! Authors are not a list")
			}

			// limit of the number of elements comes from Firestore query limit for the IN clause
			if len(_authors) >= 30 {
				_authors = _authors[:29]
			}

			for _, auth := range authors {
				_auth, ok := auth.(string)
				if ok {
					// nip-01: `The ids, authors, #e and #p filter lists MUST contain exact 64-character lowercase hex values.`
					_auth = strings.ToLower(_auth)
					if len(_auth) != 64 {
						c.lgr.Errorf("Wrong author value! It must be 64 chars long. Skiping this value.")
						continue
					}
					_authors = append(_authors, _auth)
				}
			}

			_events, err = c.repo.GetEventsByAuthors(_authors, max_events, _since, _until)
			if err != nil {
				c.lgr.Errorf("ERROR: %v from client %v subscription %v filter: %v", err, c.IP, c.Subscription_id, filter)
				return err
			}

			for _, ev := range _events {
				events = append(events, *ev)
			}

		case flt_ids && !flt_authors && !flt_kinds && !flt_tags:
			// ONLY "ids" list...

			var (
				ids  []interface{}
				_ids []string
			)

			ids, ok := filter["ids"].([]interface{})
			if !ok {
				return fmt.Errorf("Wrong filter format used! Ids are not a list")
			}

			if len(_ids) >= 30 {
				_ids = _ids[:29]
			}

			for _, id := range ids {
				_id, ok := id.(string)
				if ok {
					if len(_id) != 64 {
						c.lgr.Errorf("Wrong ID value! It must be 64 chars long. Skiping this value.")
						continue
					}
					_ids = append(_ids, _id)
				}
			}

			_events, err = c.repo.GetEventsByIds(_ids, max_events, _since, _until)
			if err != nil {
				c.lgr.Errorf("ERROR: %v from client %v subscription %v filter: %v", err, c.IP, c.Subscription_id, filter)
				return err
			}

			for _, ev := range _events {
				events = append(events, *ev)
			}

		case flt_kinds && !flt_authors && !flt_ids && !flt_tags:
			// ONLY "kinds" list ...

			var (
				kinds  []interface{}
				kindsi []int
			)

			kinds, ok := filter["kinds"].([]interface{})
			if !ok {
				return fmt.Errorf("Wrong filter format used! Kinds are not a list")
			}

			if len(kinds) >= 30 {
				kinds = kinds[:29]
			}

			for _, kind := range kinds {
				_kind, ok := kind.(float64)
				if ok {
					kindsi = append(kindsi, int(_kind))
				}
			}

			_events, err = c.repo.GetEventsByKinds(kindsi, max_events, _since, _until)
			if err != nil {
				c.lgr.Errorf("ERROR: %v from client %v subscription %v filter [kinds]: %v", err, c.IP, c.Subscription_id, filter)
				return err
			}

			for _, ev := range _events {
				events = append(events, *ev)
			}

		case flt_tags && !flt_authors && !flt_ids && !flt_kinds:
			// [ ]: implement tags: "#<single-letter (a-zA-Z)>": <a list of tag values, for #e — a list of event ids, for #p — a list of event pubkeys etc>,
			// case filter["#e"]: ...
			// case filter["#p"]: ...

		case flt_limit && !flt_tags && !flt_authors && !flt_ids && !flt_kinds:
			// ANY EVENT from the last N (limit) events arrived

			_events, err = c.repo.GetLastNEvents(max_events)
			if err != nil {
				c.lgr.Errorf("ERROR: %v from client %v subscription %v filter: %v", err, c.IP, c.Subscription_id, filter)
				return err
			}

			for _, ev := range _events {
				events = append(events, *ev)
			}

		case _since > 0 && _until == 0:
			// ONLY Events since (newer than...) the required timestamp

			_events, err = c.repo.GetEventsSince(max_events, _since)
			if err != nil {
				c.lgr.Errorf("ERROR: %v from client %v subscription %v filter: %v", err, c.IP, c.Subscription_id, filter)
				return err
			}

			for _, ev := range _events {
				events = append(events, *ev)
			}

		case _since > 0 && _until > 0:
			// ONLYEvents between `since` and `until`

			_events, err = c.repo.GetEventsSinceUntil(max_events, _since, _until)
			if err != nil {
				c.lgr.Errorf("ERROR: %v from client %v subscription %v filter: %v", err, c.IP, c.Subscription_id, filter)
				return err
			}

			for _, ev := range _events {
				events = append(events, *ev)
			}

		case flt_authors && flt_kinds && !flt_ids && !flt_tags:
			// Combined filter about events from list of AUTHORS AND specified list of KINDS of the events

			var (
				authors  []interface{}
				_authors []string
				kinds    []interface{}
				_kinds   []int
			)

			authors, ok := filter["authors"].([]interface{})
			if !ok {
				return fmt.Errorf("Wrong filter format used! Authors are not a list")
			}

			if len(_authors) >= 30 {
				_authors = _authors[:29]
			}

			for _, auth := range authors {
				_auth, ok := auth.(string)
				if ok {
					// nip-01: `The ids, authors, #e and #p filter lists MUST contain exact 64-character lowercase hex values.`
					_auth = strings.ToLower(_auth)
					if len(_auth) != 64 {
						c.lgr.Errorf("Wrong author value! It must be 64 chars long. Skiping this value.")
						continue
					}
					_authors = append(_authors, _auth)
				}
			}

			kinds, ok = filter["kinds"].([]interface{})
			if !ok {
				return fmt.Errorf("Wrong filter format used! Kinds are not a list")
			}

			if len(kinds) >= 30 {
				kinds = kinds[:29]
			}

			for _, kind := range kinds {
				_kind, ok := kind.(float64)
				if ok {
					_kinds = append(_kinds, int(_kind))
				}
			}

			_events, err = c.repo.GetEventsBtAuthorsAndKinds(_authors, _kinds, max_events, _since, _until)
			if err != nil {
				c.lgr.Errorf("ERROR: %v from client %v subscription %v filter: %v", err, c.IP, c.Subscription_id, filter)
				return err
			}

			for _, ev := range _events {
				events = append(events, *ev)
			}

		default:
			// ANT Other not implemented case of filter

			c.lgr.Errorf("Customer [%v] Subscription [%v] Not implemented filter: [%v]", c.IP, c.Subscription_id, filter)
		}

		// Count at the end of each filter component
		c.read_events += len(events)
		c.lgr.Debugf("[fetchData] Filter components: %v", len(filter))

		var wev []interface{} = []interface{}{}
		var intf = make(map[string]interface{}, 1)

		for _, evnt := range events {
			inrec, _ := json.Marshal(&evnt)
			_ = json.Unmarshal(inrec, &intf)
			wev = append(wev, intf)
		}

		// sending []Events array as bytes to the writeT channel
		c.wrchrr = time.Now().UnixMilli()
		c.msgwt <- wev

		c.lgr.WithFields(log.Fields{"method": "[fetchData]", "client": c.IP, "SubscriptionID": c.Subscription_id, "filter": filter, "Nr_of_events": len(events), "servedIn": time.Now().UnixMilli() - c.responseRate}).Debug("Sent to writeT")

		return nil
	}()
}

// ################# End of Subscription Supplier ##############################
