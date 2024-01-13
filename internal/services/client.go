package services

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/logging"
	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/tools"
	"github.com/gorilla/websocket"
	gn "github.com/nbd-wtf/go-nostr"
	"golang.org/x/sync/errgroup"
)

var (
	nbmrevents   int
	stop         = make(chan struct{})
	msgw         = make(chan *[]interface{})
	msgwt        = make(chan []byte)
	mu           sync.Mutex
	leop         = logging.Entry{Severity: logging.Info, Payload: ""}
	responseRate = int64(0)
)

type Client struct {
	conn            *websocket.Conn
	lgr             *log.Logger
	cclnlgr         *logging.Logger
	repo            nostr.NostrRepo
	id              uint
	name            string
	session         *Session
	challenge       string
	npub            string
	Subscription_id string
	Filetrs         []map[string]interface{}
	errorRate       map[string]int
	IP              string
	Authed          bool
}

func (c *Client) Name() string {
	return c.name
}

func (c *Client) ReceiveMsg() error {
	var message []interface{}
	errRM := make(chan error)

	go func() {
		for {
			err := c.conn.ReadJSON(&message)
			if err != nil {
				errRM <- fmt.Errorf("Error while reading message: %v", err)
				continue
			}
			// sending the message to the write channel
			err = c.dispatcher(&message)
			if err != nil {
				errRM <- fmt.Errorf("Error while classifing a message: %v", err)
				continue
			}
		}
	}()

	err := c.write()
	if err != nil {
		errRM <- fmt.Errorf("Error while writing message: %v", err)
	}

	err = c.writeT()
	if err != nil {
		errRM <- fmt.Errorf("Error while writing message: %v", err)
	}

	// Receive errors from the channel and handle them
	for err := range errRM {
		if err != nil {
			fmt.Println("[receiveMSG] Error received:", err)
			// Perform error handling or logging here
			return err
		}
	}
	return nil
}

func (c *Client) write() error {
	errWM := make(chan error)
	var msg *[]interface{}

	go func() {
		//mutex := sync.Mutex{}
		for {
			select {
			case <-stop:
				c.lgr.Printf("[write] Stopping the write channel")
				return
			default:
				msg = <-msgw

				if msg != nil {
					tsw := time.Now().UnixMilli()

					mu.Lock()
					err := c.conn.WriteJSON(msg)
					mu.Unlock()
					if err != nil {
						errWM <- err
					}

					// [ ]: Remove the next 2 lines for production performance
					lb := tools.CalcLenghtInBytes(msg)
					c.lgr.Printf(" ## %d bytes sent to [%s] over websocket in %d ms", lb, c.IP, time.Now().UnixMilli()-tsw)

					fmt.Printf("\n\n## [write] customer [%s] SubscriptionID: %s after-write-to-ws: %d ms\n", c.IP, c.Subscription_id, time.Now().UnixMilli()-responseRate)

					msg = nil
				}
				errWM <- nil
			}
		}
	}()

	for err := range errWM {
		if err != nil {
			fmt.Println("[write] Error received: ", err)
			// Perform error handling or logging here
			return err
		} else {
			return nil
		}
	}
	return nil
}

func (c *Client) writeT() error {
	var (
		errWM = make(chan error)
	)

	go func() {
		for message := range msgwt {
			select {
			case <-stop:
				c.lgr.Printf("[write] Stopping the write channel")
				return
			default:
				mu.Lock()
				err := c.conn.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					errWM <- err
				}
				mu.Unlock()

				fmt.Printf("\n\n## [writeT] customer [%s] SubscriptionID: %s bytes:*  %d after-writeT-to-ws: %d ms\n", c.IP, c.Subscription_id, len(message), time.Now().UnixMilli()-responseRate)
			}
		}
	}()

	for err := range errWM {
		if err != nil {
			fmt.Println("[writeT] Error received: ", err)
			// Perform error handling or logging here
			return err
		} else {
			return nil
		}
	}
	return nil
}

func (c *Client) dispatcher(msg *[]interface{}) error {
	// [x]: implement the logic to dispatch the message to the client
	switch (*msg)[0] {
	case "EVENT":
		return c.handlerEventMsgs(msg)

	case "REQ":
		return c.handlerReqMsgs(msg)

	case "CLOSE":
		return c.handlerCloseSubsMsgs(msg)

	case "AUTH":
		return c.handlerAuthMsgs(msg)
	default:
		c.writeCustomNotice("Error: invalid format of the received message")
		return fmt.Errorf("[dispatcher] ERROR: unknown message type: %v", (*msg)[0])
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
	e.SetExtra("cname", c.name)

	// [x]: fire-up a new go routine to handle the NEW event broadcating for all the clients having subscriptions with at least one filter matching the event paramateres.
	go c.session.BroadcasterQueue(*e)

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
	c.lgr.Printf("NEW: subscription id: %s from [%s] with filter %v", c.IP, c.Subscription_id, msgfilters)

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
	var note = []interface{}{"OK", event_id, accepted, message}
	msgw <- &note
}

func (c *Client) writeCustomNotice(notice_msg string) {
	var note = []interface{}{"NOTICE", notice_msg}
	msgw <- &note
}

// Protocol definition: ["EOSE", <subscription_id>], used to indicate that the relay has no more events to send for a subscription.
func (c *Client) writeEOSE(subID string) {
	var eose = []interface{}{"EOSE", subID}
	msgw <- &eose
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

// Protocol definition: ["AUTH", <challenge>], (nip-42) used to request authentication from the client.
func (c *Client) writeAUTHChallenge() {
	var note = []interface{}{"AUTH", c.challenge}
	msgw <- &note
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

	// get the relay HOST name from the configuration
	cfg_relayHost := c.session.cfg.Relay_Host

	return relURL.Hostname() == cfg_relayHost
}

// *****************************************************************************
// 		Subscription Supplier method for initial subs load
// *****************************************************************************

func (c *Client) SubscriptionSuplier() error {
	nbmrevents = 0
	ctx := context.Background()
	responseRate = time.Now().UnixMilli()

	fmt.Printf("\n\n## [SubscriptionSuplier] customer [%s] SubscriptionID: %s\n", c.IP, c.Subscription_id)

	err := c.fetchAllFilters(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("\n\n## [SubscriptionSuplier] customer [%s] SubscriptionID: %s fetchAllFilters-took: %d ms\n", c.IP, c.Subscription_id, time.Now().UnixMilli()-responseRate)

	// Send EOSE
	c.writeEOSE(c.Subscription_id)

	rslt := time.Now().UnixMilli() - responseRate

	payloadV := fmt.Sprintf(`{"method":"## SubscriptionSuplier","IP":"%s","Filters":"%v","events":%d,"servedIn": %d}`, c.IP, c.Filetrs, nbmrevents, rslt)

	leop.Payload = payloadV
	cclnlgr.Log(leop)

	c.lgr.Printf(fmt.Sprintf(`{"IP":"%s","Filters":"%v","events":%d,"servedIn": %d}`, c.IP, c.Filetrs, nbmrevents, rslt))

	return nil
}

// This will be running all filters per subscription in parallel
func (c *Client) fetchAllFilters(ctx context.Context) error {

	eg := errgroup.Group{}

	fmt.Printf("\n\n## [fetchAllFilters] customer [%s] SubscriptionID: %s start-fetchAllFilters: %d ms\n", c.IP, c.Subscription_id, time.Now().UnixMilli()-responseRate)

	// Run all the HTTP requests in parallel
	for _, filter := range c.Filetrs {
		fltr := filter
		eg.Go(func() error {
			return c.fetchData(fltr, &eg)
		})
	}

	fmt.Printf("\n\n## [fetchAllFilters] customer [%s] SubscriptionID: %s fetchAllFilters-after-for-loop: %d ms\n", c.IP, c.Subscription_id, time.Now().UnixMilli()-responseRate)

	// Wait for completion and return the first error (if any)
	return eg.Wait()
}

func (c *Client) fetchData(filter map[string]interface{}, eg *errgroup.Group) error {
	// Function to fetch data from the specified filter

	fmt.Printf("\n\n## [fetchData] customer [%s] SubscriptionID: %s fetchData-start: %d ms\n", c.IP, c.Subscription_id, time.Now().UnixMilli()-responseRate)

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
			err             error
			events, eventsC []*gn.Event
			max_events      int
			_since, _until  int64
			fltl            int
		)

		lim, ok := filter["limit"].(float64)
		if !ok {
			max_events = c.session.cfg.GetDLV()
		} else if lim == 0 || lim > 5000 {
			// until the paging is implemented for paying clients OR always for public clients
			// 5000 is the value from attribute from server_info limits
			max_events = 5000
		} else {
			// set the requested by the filter value
			max_events = int(lim)
		}

		// =================================== SINCE - UNTIL ===============================================
		if since, ok := filter["since"].(float64); !ok {
			_since = int64(0)
		} else {
			_since = int64(since)
		}
		if until, ok := filter["until"].(float64); !ok {
			_until = int64(0)
		} else {
			_until = int64(until)
		}

		fmt.Printf("\n\n## [fetchData] customer [%s] SubscriptionID: %s limit-since-until-ready: %d ms\n", c.IP, c.Subscription_id, time.Now().UnixMilli()-responseRate)

		// Parsing filter's lists
		for key, val := range filter {

			if nbmrevents > max_events {
				break
			}

			nbmrevents += len(events)

			switch {
			case key == "kinds":

				c.lgr.Printf("Filter kinds:%v", val)

				var (
					kinds  []interface{}
					kindsi []int
				)

				kinds, ok = filter["kinds"].([]interface{})
				if !ok {
					return fmt.Errorf("Wrong filter format used! Kinds are not a list")
				}

				if len(kinds) > 30 {
					kinds = kinds[:30]
				}

				for _, kind := range kinds {
					_kind, ok := kind.(float64)
					if ok {
						kindsi = append(kindsi, int(_kind))
					}
				}

				events, err = c.repo.GetEventsByKinds(kindsi, max_events, _since, _until)
				if err != nil {
					c.lgr.Printf("ERROR: %v from subscription %v filter [kinds]: %v", err, c.Subscription_id, filter)
					return err
				}

				fmt.Printf("\n\n## [fetchData] customer [%s] SubscriptionID: %s after-GetEventsByKinds: %d ms\n", c.IP, c.Subscription_id, time.Now().UnixMilli()-responseRate)

			case key == "authors":

				c.lgr.Printf("Filter authors:%v", val)

				var (
					authors  []interface{}
					_authors []string
				)

				authors, ok = filter["authors"].([]interface{})
				if !ok {
					return fmt.Errorf("Wrong filter format used! Authors are not a list")
				}

				for _, auth := range authors {
					_auth, ok := auth.(string)
					if ok {
						_authors = append(_authors, _auth)
					}
				}

				if len(_authors) > 30 {
					_authors = _authors[:30]
				}

				events, err = c.repo.GetEventsByAuthors(_authors, max_events, _since, _until)
				if err != nil {
					c.lgr.Printf("ERROR: %v from subscription %v filter: %v", err, c.Subscription_id, filter)
					return err
				}

				fmt.Printf("\n\n## [fetchData] customer [%s] SubscriptionID: %s after-GetEventsByAuthors: %d ms\n", c.IP, c.Subscription_id, time.Now().UnixMilli()-responseRate)

			case key == "ids":

				c.lgr.Printf("Filter ids:%v", val)

				var (
					ids  []interface{}
					_ids []string
				)

				ids, ok = filter["ids"].([]interface{})
				if !ok {
					return fmt.Errorf("Wrong filter format used! Ids are not a list")
				}

				for _, id := range ids {
					_id, ok := id.(string)
					if ok {
						_ids = append(_ids, _id)
					}
				}

				if len(_ids) > 30 {
					_ids = _ids[:30]
				}

				events, err = c.repo.GetEventsByIds(_ids, max_events, _since, _until)
				if err != nil {
					c.lgr.Printf("ERROR: %v from subscription %v filter: %v", err, c.Subscription_id, filter)
					return err
				}

				fmt.Printf("\n\n## [fetchData] customer [%s] SubscriptionID: %s after-GetEventsByIds: %d ms\n", c.IP, c.Subscription_id, time.Now().UnixMilli()-responseRate)

			case strings.HasPrefix(key, "#"):
				// [ ]: implement tags: "#<single-letter (a-zA-Z)>": <a list of tag values, for #e — a list of event ids, for #p — a list of event pubkeys etc>,
				// case filter["#e"]: ...

				fmt.Printf("\n\n## [fetchData] customer [%s] SubscriptionID: %s after-NotImplTags: %d ms\n", c.IP, c.Subscription_id, time.Now().UnixMilli()-responseRate)

			default:
				if _since > 0 && _until == 0 {

					events, err = c.repo.GetEventsSince(max_events, _since)
					if err != nil {
						c.lgr.Printf("ERROR: %v from subscription %v filter: %v", err, c.Subscription_id, filter)
						return err
					}

					fmt.Printf("\n\n## [fetchData] customer [%s] SubscriptionID: %s after-GetEventsSince: %d ms\n", c.IP, c.Subscription_id, time.Now().UnixMilli()-responseRate)

				} else if _since > 0 && _until > 0 {

					events, err = c.repo.GetEventsSinceUntil(max_events, _since, _until)
					if err != nil {
						c.lgr.Printf("ERROR: %v from subscription %v filter: %v", err, c.Subscription_id, filter)
						return err
					}

					fmt.Printf("\n\n## [fetchData] customer [%s] SubscriptionID: %s after-GetEventsSinceUntil: %d ms\n", c.IP, c.Subscription_id, time.Now().UnixMilli()-responseRate)

				} else {
					//events, err = c.repo.GetEventsByFilter(filter)
					// if err != nil {
					// 	c.lgr.Printf("ERROR: %v from subscription %v filter: %v", err, c.Subscription_id, filter)
					// 	return err
					// }
					c.lgr.Printf("Customer [%v]. Filter is too complex:\n %v\n", c.IP, filter)
				}
			}
			fltl++
			if len(eventsC)+len(events) < max_events {
				eventsC = append(eventsC, events...)
			} else {
				break
			}
		}

		c.lgr.Printf("Filter components: %v", fltl)

		// Convert into []byte to be easy wrriten into the websocket channel
		buf := new(bytes.Buffer)
		enc := gob.NewEncoder(buf)
		errE := enc.Encode(eventsC)
		if errE != nil {
			fmt.Println("Error encoding struct []events:", err)
			return errE
		}
		// sending []Events array as bytes to the writeT channel
		msgwt <- buf.Bytes()

		fmt.Printf("\n\n## [fetchData] customer [%s] SubscriptionID: %s sent-to-msgw-channel: %d ms\n", c.IP, c.Subscription_id, time.Now().UnixMilli()-responseRate)

		return nil
	}()
}

// ################# End of Subscription Supplier ##############################
