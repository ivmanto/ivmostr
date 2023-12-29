package services

//write me example of mirroring websocket server using gorilla/websocket

import (
	"context"
	"log"
	"net/http"
	"strings"
	"testing"

	"github.com/dasiyes/ivmostr-tdd/internal/data/firestoredb"
	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/pkg/fspool"
	"github.com/dasiyes/ivmostr-tdd/pkg/gopool"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// connect to the echo server
var (
	rslmsg    []interface{}
	conn      *websocket.Conn
	nostrRepo nostr.NostrRepo
	err       error
	ctx              = context.Background()
	prj              = "ivm-ostr-srv"
	dlv       int    = 20
	ecn       string = "events"
	pool             = gopool.NewPool(128, 2, 1)
	session          = NewSession(pool, nostrRepo, nil)
	message   []interface{}
)

var client = Client{
	conn:            nil,
	lgr:             log.New(log.Writer(), "client: ", log.LstdFlags),
	id:              0,
	name:            "",
	session:         nil,
	challenge:       "",
	npub:            "",
	Subscription_id: "",
	Filetrs:         []map[string]interface{}{},
	errorRate:       map[string]int{},
	IP:              "",
	Authed:          false,
}

// create a new echo server

func init() {
	// Init the repo
	fsclientPool := fspool.NewConnectionPool(prj)
	if err != nil {
		client.lgr.Printf("error creating firestore client: %v", err)
		panic(err)
	}
	//defer clientFrst.Close()

	nostrRepo, err = firestoredb.NewNostrRepository(&ctx, fsclientPool, dlv, ecn)
	if err != nil {
		client.lgr.Printf("error creating firestore repo: %v", err)
		panic(err)
	}

	// create a new echo server
	go EchoServer()
	GetConnected(nil)
}

func GetConnected(t *testing.T) {
	conn, _, err = websocket.DefaultDialer.Dial("ws://localhost:8080/echo", nil)
	if err != nil {
		t.Fatalf("error connecting to echo server: %v", err)
	}

	// set the client's connection to the echo server
	client.conn = conn
	client.session = session
	client.session.Repo = nostrRepo
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

	// create a message to send to the echo server
	message := []interface{}{"hello world"}

	// write the message to the client
	msgw <- &message

	// read the message from the echo server
	err = conn.ReadJSON(&rslmsg)
	if err != nil {
		t.Fatalf("error reading message from echo server: %v", err)
	}

	// check that the message received from the echo server is the same as the message sent to the client
	if string(rslmsg[0].(string)) != "hello world" {
		t.Fatalf("error: message received from echo server is not the same as message sent to client")
	}
}

// using the EchoServer above write some tests for handlerEventMsgs
func TestHandlerEventMsgs(t *testing.T) {

	// create a message to send to the client
	message = []interface{}{"EVENT", map[string]interface{}{"id": "b22f429ac7222530b6111191c160cdf5730a482ab18177c380b138278e443afa", "pubkey": "9f9c3e46933b9a33702abde78383745a1ebcd99f59a53ec9286beee72a1072de",
		"created_at": 1697743946,
		"kind":       1,
		"tags":       []interface{}{[]string{"e", "", "wss://nostr.ivmanto.dev"}},
		"content":    "Test event by ivmanto", "sig": "77d4065b1dda7320f3257083efecbc2db6265f3b2c5b28607ffd31acb3d0f5f1a272f81d9482cd56c96368cb0c8f1a840845e9adb13bb295b639040ae91333e1",
	}}

	// first delete the event from firestore repo
	doc_id := message[1].(map[string]interface{})["id"].(string)
	if doc_id == "" {
		t.Fatalf("error: id not found or it is in wrong format")
	}

	err = client.session.Repo.DeleteEvent(doc_id)
	if err != nil {
		t.Fatalf("error deleting event id %s, from firestore repo: %v", doc_id, err)
	}

	// test TestHandlerEventMsgs -supposed to store the event in the DB
	err = client.handlerEventMsgs(&message)
	if err != nil {
		t.Fatalf("error storing event in firestore repo: %v", err)
	}

	_ = conn.ReadJSON(&rslmsg)
	// check that the message received from the echo server is the same as the message sent to the client
	// string(`["OK","b22f429ac7222530b6111191c160cdf5730a482ab18177c380b138278e443afa",true,""]`)
	if string(rslmsg[0].(string)) != "OK" || rslmsg[2].(bool) != true {
		client.lgr.Printf("msg: %v", rslmsg...)
		t.Fatalf("error: message received from echo server is not expected")
	}

	// testing TestHandlerEventMsgs function - return duplicated error message

	err = client.handlerEventMsgs(&message)
	if err != nil {
		t.Fatalf("error: should return duplicated error message")
	}

	_ = conn.ReadJSON(&rslmsg)
	// check that the message received from the echo server is the same as the message sent to the client
	// recv:
	//["OK","b22f429ac7222530b6111191c160cdf5730a482ab18177c380b138278e443afa",false,"duplicate: unable to save in clients repository. error: rpc error: code = AlreadyExists desc = Document already exists: projects/ivm-ostr-srv/databases/(default)/documents/events/b22f429ac7222530b6111191c160cdf5730a482ab18177c380b138278e443afa"]
	if string(rslmsg[0].(string)) != "OK" || string(rslmsg[1].(string)) != "b22f429ac7222530b6111191c160cdf5730a482ab18177c380b138278e443afa" || !strings.Contains(string(rslmsg[3].(string)), "duplicate:") {
		t.Fatalf("error: message received from echo server is not expected")
	}

	err = client.session.Repo.DeleteEvent(doc_id)
	if err != nil {
		t.Fatalf("error deleting event id %s, from firestore repo: %v", doc_id, err)
	}
}

func TestHandlerReqMsgs(t *testing.T) {

	client.Subscription_id = ""
	client.Filetrs = []map[string]interface{}{}

	// create a message to send to the client
	message = []interface{}{"REQ", "read_1565697166", map[string]interface{}{
		"limit": 1,
		"kinds": []int{1},
	}}

	err = client.handlerReqMsgs(&message)
	if err != nil {
		t.Fatalf("error create subscription %v", err)
	}

	if client.Subscription_id != "read_1565697166" || client.Filetrs == nil {
		t.Fatalf("subscribing the client failed")
	}
}
