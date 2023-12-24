package services

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	gn "github.com/nbd-wtf/go-nostr"
	"golang.org/x/sync/errgroup"
)

var (
	nbmrevents int
	stop       = make(chan struct{})
)

type Client struct {
	conn            *websocket.Conn
	lgr             *log.Logger
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

	// Receive errors from the channel and handle them
	for err := range errRM {
		if err != nil {
			fmt.Println("Error received:", err)
			// Perform error handling or logging here
			return err
		}
	}
	return nil
}

func (c *Client) write(msg *[]interface{}) error {
	errWM := make(chan error)
	go func() {
		mutex := sync.Mutex{}
		for {
			select {
			case <-stop:
				c.lgr.Printf("[write] Stopping the write channel")
				return
			default:
				// [ ]: implement the logic to send the message to the client
				if msg != nil {
					mutex.Lock()
					err := c.conn.WriteJSON(msg)
					mutex.Unlock()
					if err != nil {
						errWM <- err
					}
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

func (c *Client) dispatcher(msg *[]interface{}) error {
	// [ ]: implement the logic to dispatch the message to the client
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
		c.lgr.Printf("ERROR: unknown message type: %v", (*msg)[0])
		return c.writeCustomNotice("Error: invalid format of the received message")
	}
}

// ****************************** Messages types Handlers ***********************************************

func (c *Client) handlerEventMsgs(msg *[]interface{}) error {
	// OK messages MUST be sent in response to EVENT messages received from clients, they must have the 3rd parameter set to true when an event has been accepted by the relay, false otherwise. The 4th parameter MAY be empty when the 3rd is true, otherwise it MUST be a string containing a machine-readable single-word prefix followed by a : and then a human-readable message. The standardized machine-readable prefixes are: duplicate, pow, blocked, rate-limited, invalid, and error for when none of that fits.

	se, ok := (*msg)[1].(map[string]interface{})
	if !ok {
		lmsg := "invalid: unknown message format"
		_ = c.writeEventNotice("0", false, lmsg)
	}

	e, err := mapToEvent(se)
	if err != nil {
		// [!] protocol error
		c.errorRate[c.IP]++
		lmsg := "invalid:" + err.Error()
		return c.writeEventNotice(e.ID, false, lmsg)
	}

	rsl, errv := e.CheckSignature()
	if errv != nil {
		lmsg := fmt.Sprintf("result %v, invalid: %v", rsl, errv.Error())
		return c.writeEventNotice(e.ID, false, lmsg)
	}

	switch e.Kind {
	case 3:
		// nip-02, kind:3 replaceable, Contact List (overwrite the exiting event for the same pub key)

		// sending the event to a channel, will hijack the event to the broadcaster
		// Keep it disabled until [ ]TODO: find a way to clone the execution context
		// NewEvent <- e

		err := c.session.repo.StoreEventK3(e)
		if err != nil {
			return c.writeEventNotice(e.ID, false, composeErrorMsg(err))
		}
		return c.writeEventNotice(e.ID, true, "")

	case 4:
		// Even if the relay access level is public, the private encrypted messages kind=4 should be rejected
		if !c.Authed {
			_ = c.writeCustomNotice("restricted: we can't serve DMs to unauthenticated users, does your client implement NIP-42?")
			return c.writeEventNotice(e.ID, false, "Authentication required")
		}

	default:
		// default will handle all other kinds of events that do not have specifics handlers
		// do nothing
	}

	// sending the event to a channel, will hijack the event to the broadcaster
	// Keep it disabled until [ ]TODO: find a way to clone the execution context
	// NewEvent <- e

	err = c.session.repo.StoreEvent(e)
	if err != nil {
		return c.writeEventNotice(e.ID, false, composeErrorMsg(err))
	}

	// The customer name is required in NewEventBroadcaster method in order to skip the broadcast process for the customer that brought the event on the relay.
	e.SetExtra("cname", c.name)

	// [x]: fire-up a new go routine to handle the NEW event broadcating for all the clients having subscriptions with at least one filter matching the event paramateres.
	go c.session.BroadcasterQueue(*e)

	return c.writeEventNotice(e.ID, true, "")
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
			return c.writeCustomNotice("Error: Invalid subscription_id")
		}
	} else {
		return c.writeCustomNotice("Error: Missing subscription_id")
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

			err := c.SubscriptionSuplier()
			if err != nil {
				return err
			}

			c.lgr.Printf("UPDATE: subscription id: %s with filter %v", c.Subscription_id, msgfilters)
			c.lgr.Printf("DEBUG: subscription id: %s all filters: %v", c.Subscription_id, c.Filetrs)

			return c.writeCustomNotice("Update: The subscription filter has been overwriten.")
		} else {
			// [!] protocol error
			c.errorRate[c.IP]++
			em := fmt.Sprintf("Error: There is already an active subscription for this connection with subscription_id: %v, the new subID %v is ignored.", c.Subscription_id, subscription_id)
			return c.writeCustomNotice(em)
		}
	}
	c.Subscription_id = subscription_id
	c.Filetrs = append(c.Filetrs, msgfilters...)

	err := c.SubscriptionSuplier()
	if err != nil {
		return err
	}

	c.lgr.Printf("NEW [%s]: subscription id: %s with filter %v", c.IP, c.Subscription_id, msgfilters)
	c.lgr.Printf("DEBUG: subscription id: %s all filters: %v", c.Subscription_id, c.Filetrs)

	return c.writeCustomNotice(fmt.Sprintf("The subscription with filter %v has been created", msgfilters))
}

func (c *Client) handlerCloseSubsMsgs(msg *[]interface{}) error {

	//[x] remove subscription id

	if len(*msg) == 2 {
		subscription_id := (*msg)[1].(string)
		if c.Subscription_id == subscription_id {
			c.Subscription_id = ""
			c.Filetrs = []map[string]interface{}{}
			return c.writeCustomNotice("Update: The subscription has been clodsed")
		} else {
			return c.writeCustomNotice("Error: Invalid subscription_id provided")
		}
	} else {
		return c.writeCustomNotice("Error: Bad format of CLOSE message")
	}
}

func (c *Client) handlerAuthMsgs(msg *[]interface{}) error {
	// ["AUTH", <signed-event-json>] used to authenticate the client to the relay.

	se, ok := (*msg)[1].(map[string]interface{})
	if !ok {
		_ = c.writeEventNotice("0", false, "invalid: unknown message format")
	}

	e, err := mapToEvent(se)
	if err != nil {
		return c.writeEventNotice(e.ID, false, "invalid:"+err.Error())
	}

	// [x]: The signed event is an ephemeral event **not meant to be published or queried**,
	// [x]: It must be of `kind: 22242` and it should have at least **two tags**, one for the `relay URL` and one for the `challenge string` as received from the relay.
	// [x]: Relays MUST exclude kind: 22242 events from being broadcasted to any client.
	// [x]: created_at should be the current time (~ 10 mins max).

	rsl, errv := e.CheckSignature()
	if errv != nil {
		lmsg := fmt.Sprintf("result %v, invalid: %v", rsl, errv.Error())
		return c.writeEventNotice(e.ID, false, lmsg)
	}

	if e.Kind != 22242 {
		return c.writeEventNotice(e.ID, false, "invalid: invalid AUTH message kind")
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
			return c.writeEventNotice(e.ID, true, "")
		} else {
			return c.writeEventNotice(e.ID, false, "Authentication failed")
		}
	} else {
		return c.writeEventNotice(e.ID, false, "Authentication failed")
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
func (c *Client) writeEventNotice(event_id string, accepted bool, message string) error {
	var note = []interface{}{"OK", event_id, accepted, message}
	return c.write(&note)
}

func (c *Client) writeCustomNotice(notice_msg string) error {
	var note = []interface{}{"NOTICE", notice_msg}
	return c.write(&note)
}

// Protocol definition: ["EOSE", <subscription_id>], used to indicate that the relay has no more events to send for a subscription.
func (c *Client) writeEOSE(subID string) error {
	var eose = []interface{}{"EOSE", subID}
	return c.write(&eose)
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
func (c *Client) writeAUTHChallenge() error {
	var note = []interface{}{"AUTH", c.challenge}
	return c.write(&note)
}

// Get Filters returns the filters of the current client's subscription
func (c *Client) GetFilters() []map[string]interface{} {
	return c.Filetrs
}

func (c *Client) confirmURLDomainMatch(relayURL string) bool {

	// convert url string to URL object
	relURL, err := url.Parse(relayURL)
	if err != nil {
		_ = c.writeCustomNotice("ERROR: parsing provided relay URL")
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
	responseRate := time.Now().UnixMilli()

	err := c.fetchAllFilters(ctx)
	if err != nil {
		return err
	}

	// Send EOSE
	errcls := c.writeEOSE(c.Subscription_id)
	if errcls != nil {
		return errcls
	}

	rslt := time.Now().UnixMilli() - responseRate
	c.lgr.Printf(`{"IP":"%s","Subscription":"%s","events":%d,"servedIn": %d}`, c.IP, c.Subscription_id, nbmrevents, rslt)
	return nil
}

// This will be running all filters per subscription in parallel
func (c *Client) fetchAllFilters(ctx context.Context) error {

	eg := errgroup.Group{}

	// Run all the HTTP requests in parallel
	for _, filter := range c.Filetrs {
		fltr := filter
		eg.Go(func() error {
			return c.fetchData(fltr, &eg)
		})
	}

	// Wait for completion and return the first error (if any)
	return eg.Wait()
}

func (c *Client) fetchData(filter map[string]interface{}, eg *errgroup.Group) error {
	// Function to fetch data from the specified filter
	return func() error {

		// method is used for DEBUG info review
		//c.session.printFilter(filter, c)

		events, err := c.session.repo.GetEventsByFilter(filter)
		if err != nil {
			c.lgr.Printf(": %v from subscription %v filter: %v", err, c.Subscription_id, filter)
			return err
		}
		nbmrevents += len(events)

		err = c.write(&[]interface{}{events})
		if err != nil {
			c.lgr.Printf(": %v from subscription %v filter: %v", err, c.Subscription_id, filter)
			return err
		}
		return nil
	}()
}

// ################# End of Subscription Supplier ##############################
