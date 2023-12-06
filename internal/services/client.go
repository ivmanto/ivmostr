package services

import (
	"log"
	"sync"

	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/gorilla/websocket"
)

type Client struct {
	io              sync.Mutex
	conn            *websocket.Conn
	lgr             *log.Logger
	id              uint
	name            string
	session         *Session
	send            chan []byte
	readerr         chan error
	repo            nostr.NostrRepo
	lrepo           nostr.ListRepo
	challenge       string
	npub            string
	Subscription_id string
	Filetrs         []map[string]interface{}
	errorRate       map[string]int
	IP              string
	Authed          bool
}

func NewClient(session *Session, conn *websocket.Conn) *Client {
	return &Client{
		session: session,
		conn:    conn,
		send:    make(chan []byte),
	}
}

func (c *Client) Start() error {

	c.lgr.Printf("DEBUG: Starting client %v", c.conn.RemoteAddr().String())

	go c.readPump()
	go c.writePump()

	err := <-c.readerr
	c.lgr.Printf("DEBUG: Error from readPump: %v", err)

	return err
}

func (c *Client) readPump() {
	defer func() {
		// c.session.unregister <- c.conn
		c.conn.Close()
		c.lgr.Printf("DEBUG: Closing client %v", c.conn.RemoteAddr().String())
	}()

	for {

		c.lgr.Printf("DEBUG: Waiting for message from client %v", c.conn.RemoteAddr().String())

		mt, message, err := c.conn.ReadMessage()
		if err != nil {
			c.lgr.Printf("DEBUG: Error while reading message type:%v, %v", mt, err)
			c.readerr <- err
			break
		}

		c.lgr.Printf("DEBUG: Received message:%v, (type):%v", string(message), mt)
		// [ ]: clasify the messages

		// Handle incoming message according to the nostr subprotocol
		// ...

		// Example: Echo the received message back to the client
		c.send <- message
	}
	c.lgr.Printf("DEBUG: Exiting readPump")
}

func (c *Client) writePump() {
	defer func() {
		c.conn.Close()
	}()

	for idx, message := range <-c.send {

		c.lgr.Printf("DEBUG: idx: %d, Sending message: %v", idx, string(message))

		// [ ]: implement the logic to send the message to the client
		// if !ok {
		// 	_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
		// 	return
		// }

		// err := c.conn.WriteMessage(websocket.TextMessage, message)
		// if err != nil {
		// 	log.Println("Error writing message:", err)
		// 	return
		// }

	}
	c.lgr.Printf("DEBUG: Exiting writePump")
}

func (c *Client) Name() string {
	return c.name
}
