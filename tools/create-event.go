package tools

import (
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	gn "github.com/studiokaiji/go-nostr"
)

// online key converter at: https://nostrcheck.me/converter/

// [!]: PREFFERED!!! locally installed key-converter: https://github.com/rot13maxi/key-convertr. Insttaled localy at: /Users/tonevSr/development/key-convertr

var (
	pk string
	// this is hex value of the pubKey of the ivmostr relay (the same for ivmanto nostr account) in server_info file

	pubKh = "29480df94c3c41a416285e920cc3ba7887efe05fecb58e73a783231c426969b9"
)

var tag1 = []string{"e", "", "wss://nostr.ivmanto.dev"}
var tag2 = []string{"a", "-", "wss://nostr.ivmanto.dev"}
var tag10 = []string{"a", "0:29480df94c3c41a416285e920cc3ba7887efe05fecb58e73a783231c426969b9", "wss://nostr.ivmanto.dev"}

var tag3 = []string{"p", "91cf9..4e5ca", "wss://alicerelay.com/", "alice"}
var tag4 = []string{"p", "14aeb..8dad4", "wss://bobrelay.com/nostr", "bob"}
var tag5 = []string{"p", "612ae..e610f", "ws://carolrelay.com/ws", "carol"}

var tag6 = []string{"relay", "wss://localhost"}
var tag7 = []string{"challenge", "L0prKNsndshWxWp6"}

var tag8 = []string{"r", "wss://relay.ivmanto.dev", "read"}
var tag9 = []string{"r", "wss://nostr.ivmanto.dev", "write"}

func PrintNewEvent() {
	fmt.Println("Enter private key (HEX encoded) to sign event with:")

	// Read a single line of input
	_, err := fmt.Scan(&pk)
	if err != nil {
		fmt.Println("Error reading input:", err)
		panic(err)
	}
	fmt.Println("You entered:", pk)

	publicKeyNoSpaces := strings.Replace(pk, " ", "", -1)
	publicKeyNoCaps := strings.ToLower(publicKeyNoSpaces)
	publicKeyHex := hex.EncodeToString([]byte(publicKeyNoCaps))
	//hex.DecodeString(publicKeyHex))
	fmt.Println(publicKeyHex)

	e := CreateMetadataEvent()

	err = e.Sign(pk)
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
		Tags:      []gn.Tag{tag1, tag3, tag4, tag5},
		Content:   "My Contact List",
	}
}

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

// This will create a metadata event for the author (pubkey) ivmanto/nostr.ivmanto.dev relay
func CreateMetadataEvent() *gn.Event {
	return &gn.Event{
		ID:        "0",
		PubKey:    "npub199yqm72v83q6g93gt6fqesa60zr7lczlaj6cuua8sv33csnfdxusxlc3yg",
		CreatedAt: gn.Now(),
		Kind:      0,
		Tags:      []gn.Tag{tag2, tag10},
		Content:   "{\"name\": \"ivmostr\", \"about\":\"wss://nostr.ivmanto.dev\", \"picture\": \"https://us-southeast-1.linodeobjects.com/dufflepud/uploads/dd7f1f98-e27a-4008-834e-e1426c060d04.jpg\"}",
	}
}
