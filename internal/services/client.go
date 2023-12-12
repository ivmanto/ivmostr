package services

import (
	"fmt"
	"log"
	"sync"

	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/gorilla/websocket"
)

type Client struct {
	conn    *websocket.Conn
	lgr     *log.Logger
	id      uint
	name    string
	session *Session
	send    chan []interface{}
	repo    nostr.NostrRepo
	lrepo   nostr.ListRepo
	//challenge       string
	//npub            string
	Subscription_id string
	Filetrs         []map[string]interface{}
	errorRate       map[string]int
	IP              string
	Authed          bool
}

func NewClient(session *Session, conn *websocket.Conn) *Client {
	client := Client{
		session: session,
		conn:    conn,
		send:    make(chan []interface{}),
	}

	return &client
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
		fmt.Println("Error received:", err)
		// Perform error handling or logging here
		return err
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
	}

	return nil
}

func (c *Client) write(msg *[]interface{}) error {
	errWM := make(chan error)
	mutex := sync.Mutex{}

	go func() {
		for {

			// [ ]: implement the logic to send the message to the client
			if msg != nil {
				mutex.Lock()
				err := c.conn.WriteJSON(msg)
				mutex.Unlock()
				if err != nil {
					log.Println("Error writing message:", err)
					errWM <- err
				}
				msg = nil
				errWM <- nil
			}
		}
	}()

	for err := range errWM {
		fmt.Println("Error received:", err)
		// Perform error handling or logging here
		return err
	}
	return nil
}

func (c *Client) handlerEventMsgs(msg *[]interface{}) error {
	// [ ]: implement the logic to handle the EVENT message type
	// [ ]: validate message
	// [ ]: store the message in the repository
	// [ ]: send the message to the clients subscribed with filters that this message matches

	if len(*msg) < 2 {
		c.lgr.Printf("ERROR: invalid EVENT message: %v", *msg)
		return fmt.Errorf("invalid EVENT message: %v", *msg)
	}
	_ = c.write(msg)
	return nil
}

func (c *Client) handlerReqMsgs(msg *[]interface{}) error {
	// [ ]: implement the logic to handle the REQ message type
	if len(*msg) < 2 {
		c.lgr.Printf("ERROR: invalid REQ message: %v", *msg)
		return fmt.Errorf("invalid REQ message: %v", *msg)
	}
	return nil
}

func (c *Client) handlerCloseSubsMsgs(msg *[]interface{}) error {
	// [ ]: implement the logic to handle the CLOSE message type
	if len(*msg) < 2 {
		c.lgr.Printf("ERROR: invalid CLOSE message: %v", *msg)
		return fmt.Errorf("invalid CLOSE message: %v", *msg)
	}
	return nil
}

func (c *Client) handlerAuthMsgs(msg *[]interface{}) error {
	// [ ]: implement the logic to handle the AUTH message type
	if len(*msg) < 2 {
		c.lgr.Printf("ERROR: invalid AUTH message: %v", *msg)
		return fmt.Errorf("invalid AUTH message: %v", *msg)
	}
	return nil
}
