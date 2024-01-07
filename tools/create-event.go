package tools

import (
	"log"

	gn "github.com/nbd-wtf/go-nostr"
)

var (
	//pk    = "nsec13353w4p7qclh5eymfzfm5j63gxl7t6n4cs5ztp8ktc0cmvxsc89sh8leuh"
	pkhex = "8c6917543e063f7a649b4893ba4b5141bfe5ea75c4282584f65e1f8db0d0c1cb"
	//pubK  = "npub1n7wru35n8wdrxup2hhnc8qm5tg0tekvltxjnajfgd0hww2sswt0qnwtcd7"
	pubKh = "9f9c3e46933b9a33702abde78383745a1ebcd99f59a53ec9286beee72a1072de"
)

// var tag1 = []string{"e", "", "wss://nostr.ivmanto.dev"}
// var tag2 = []string{"a", "-", "wss://nostr.ivmanto.dev"}

var tag3 = []string{"p", "91cf9..4e5ca", "wss://alicerelay.com/", "alice"}
var tag4 = []string{"p", "14aeb..8dad4", "wss://bobrelay.com/nostr", "bob"}
var tag5 = []string{"p", "612ae..e610f", "ws://carolrelay.com/ws", "carol"}

func PrintNewEvent() {
	//e := CreateAuthEvent()
	e := CreateNip65Event()
	e.ID = e.GetID()
	err := e.Sign(pkhex)
	if err != nil {
		log.Printf("Error signing event: %v", err)
	}

	log.Printf("New event:\n %v", e)
}

func CreateEvent() *gn.Event {
	return &gn.Event{
		ID:        "0",
		PubKey:    pubKh,
		CreatedAt: gn.Now(),
		Kind:      3,
		Tags:      []gn.Tag{tag3, tag4, tag5},
		Content:   "My Contact List",
	}
}

var tag6 = []string{"relay", "wss://localhost"}
var tag7 = []string{"challenge", "L0prKNsndshWxWp6"}

func CreateAuthEvent() *gn.Event {
	return &gn.Event{
		ID:        "0",
		PubKey:    pubKh,
		CreatedAt: gn.Now(),
		Kind:      22242,
		Tags:      []gn.Tag{tag6, tag7},
		Content:   "My Contact List",
	}
}

var tag8 = []string{"r", "wss://relay.ivmanto.dev", "read"}
var tag9 = []string{"r", "wss://nostr.ivmanto.dev", "write"}

func CreateNip65Event() *gn.Event {
	return &gn.Event{
		ID:        "0",
		PubKey:    pubKh,
		CreatedAt: gn.Now(),
		Kind:      10002,
		Tags:      []gn.Tag{tag8, tag9},
		Content:   "Ivmanto relays List",
	}
}
