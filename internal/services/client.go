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

func (c *Client) Start() {
	go c.readPump()
	go c.writePump()
}

func (c *Client) readPump() {
	defer func() {
		//c.session.unregister <- c.conn
		//c.conn.Close()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			log.Println("Error reading message:", err)
			break
		}
		c.lgr.Println("Received message:", string(message))

		// Handle incoming message according to the nostr subprotocol
		// ...

		// Example: Echo the received message back to the client
		//c.send <- message
	}
}

func (c *Client) writePump() {
	defer func() {
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			err := c.conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				log.Println("Error writing message:", err)
				return
			}
		}
	}
}
