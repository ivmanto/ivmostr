package services

//write me example of mirroring websocket server using gorilla/websocket

import (
	"log"
	"net/http"
	"testing"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var client = Client{
	conn:    nil,
	lgr:     nil,
	id:      0,
	name:    "",
	session: nil,
	repo:    nil,
	lrepo:   nil,
	//challenge:       "",
	npub:            "",
	Subscription_id: "",
	Filetrs:         []map[string]interface{}{},
	errorRate:       map[string]int{},
	IP:              "",
	Authed:          false,
}

func init() {
	// create a new echo server
	go EchoServer()
}

func Echo(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	//defer c.Close()
	for {
		mt, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("recv: %s", message)
		err = c.WriteMessage(mt, message)
		if err != nil {
			log.Println("write:", err)
			break
		}

	}
}

func EchoServer() {
	http.HandleFunc("/echo", Echo)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// generate unit test for the write method of the client using the echo server above
func TestWrite(t *testing.T) {

	// connect to the echo server
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/echo", nil)
	if err != nil {
		t.Fatalf("error connecting to echo server: %v", err)
	}

	// set the client's connection to the echo server
	client.conn = conn

	// create a message to send to the echo server
	message := []interface{}{"hello world"}

	// write the message to the client
	err = client.write(&message)
	if err != nil {
		t.Fatalf("error writing message to client: %v", err)
	}

	// read the message from the echo server
	var rslmsg []interface{}
	err = conn.ReadJSON(&rslmsg)

	if err != nil {
		t.Fatalf("error reading message from echo server: %v", err)
	}

	// check that the message received from the echo server is the same as the message sent to the client
	if string(rslmsg[0].(string)) != "hello world" {
		t.Fatalf("error: message received from echo server is not the same as message sent to client")
	}

	// close the connection to the echo server
	conn.Close()
}

// using the EchoServer above write some tests for handlerEventMsgs
func TestHandlerEventMsgs(t *testing.T) {

	// connect to the echo server
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/echo", nil)
	if err != nil {
		t.Fatalf("error connecting to echo server: %v", err)
	}

	// set the client's connection to the echo server
	client.conn = conn

	// create a message to send to the client
	message := []interface{}{"EVENT", map[string]interface{}{
		"id":         "1234567890",
		"pubkey":     "npub1234567890",
		"created_at": 1654907465,
		"kind":       3,
		"tags":       [][]string{{"contact", "list"}},
		"content":    "hello world",
		"sig":        "sig1234567890",
	}}

	// write the message to the client
	err = client.write(&message)
	if err != nil {
		t.Fatalf("error writing message to client: %v", err)
	}

	// read the message from the echo server
	var rslmsg []interface{}
	err = conn.ReadJSON(&rslmsg)

	if err != nil {
		t.Fatalf("error reading message from echo server: %v", err)
	}

	// check that the message received from the echo server is the same as the message sent to the client
	if string(rslmsg[0].(string)) != "OK" {
		t.Fatalf("error: message received from echo server is not the same as message sent to client")
	}

	// close the connection to the echo server
	conn.Close()
}
