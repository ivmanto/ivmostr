package nostr

import (
	gn "github.com/nbd-wtf/go-nostr"
)

// Provide access to the Nostr storage
type NostrRepo interface {
	StoreEvent(e *gn.Event) error
	StoreEventK3(e *gn.Event) error
	GetEvent(id string) (*gn.Event, error)
	GetEvents(ids []string) ([]*gn.Event, error)
	GetEventsByFilter(filter map[string]interface{}) ([]*gn.Event, error)
}
