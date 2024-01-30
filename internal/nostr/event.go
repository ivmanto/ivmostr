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
	GetEventsByKinds(kinds []int, limit int, since, until int64) ([]*gn.Event, error)
	GetEventsByAuthors(authors []string, limit int, since, until int64) ([]*gn.Event, error)
	GetEventsByIds(ids []string, limit int, since, until int64) ([]*gn.Event, error)
	GetEventsSince(limit int, since int64) ([]*gn.Event, error)
	GetEventsSinceUntil(limit int, since, until int64) ([]*gn.Event, error)
	GetLastNEvents(limit int) ([]*gn.Event, error)
	GetEventsBtAuthorsAndKinds(authors []string, kinds []int, limit int, since, until int64) ([]*gn.Event, error)
	DeleteEvent(id string) error
	DeleteEvents(ids []string) error
	TotalDocs2() (int, error)
}
