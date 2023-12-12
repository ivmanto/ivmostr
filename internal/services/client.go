package services

import (
	"fmt"
	"log"

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
	errRead := make(chan error, 1)

	go func() {
		for {
			err := c.conn.ReadJSON(&message)
			if err != nil {
				errRead <- fmt.Errorf("Error while reading message: %v", err)
			}
			// sending the message to the write channel
			_ = c.write(&message)
		}
	}()
	return <-errRead
}

func (c *Client) write(msg *[]interface{}) error {

	for {

		// [ ]: implement the logic to send the message to the client
		if msg != nil {
			err := c.conn.WriteJSON(msg)
			if err != nil {
				log.Println("Error writing message:", err)
				return err
			}
			msg = nil
			return nil
		}
	}
}
