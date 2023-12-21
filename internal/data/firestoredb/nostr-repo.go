package firestoredb

import (
	"context"
	"fmt"
	"log"
	"os"

	"cloud.google.com/go/firestore"
	gn "github.com/nbd-wtf/go-nostr"
	"google.golang.org/api/iterator"

	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/tools"
)

// Holds the required objects by `nostr` relay according to the protocol
type nostrRepo struct {
	ctx               *context.Context
	events_collection string
	client            *firestore.Client
	default_limit     int
	elgr              *log.Logger
	ilgr              *log.Logger
}

func (r *nostrRepo) StoreEvent(e *gn.Event) error {

	// multi-level array are not supported by firestore! It must be converted into a array of objects with string elements
	ec := *e
	var tgs gn.Tags = ec.Tags
	ec.Tags = nil

	// Filter out the events kinds that should not be stored according to the `nostr` protocol
	if ec.Kind == 22242 {
		return fmt.Errorf("AUTH event should not be stored in the repository")
	}

	if _, err := r.client.Collection(r.events_collection).Doc(e.ID).Create(*r.ctx, ec); err != nil {
		return fmt.Errorf("unable to save in clients repository. error: %v", err)
	}

	if len(tgs) < 1 {
		return nil
	}

	tags := tagsToTagMap(tgs)
	docRef := r.client.Collection(r.events_collection).Doc(ec.ID)

	if _, errt := docRef.Set(*r.ctx, tags, firestore.MergeAll); errt != nil {
		return fmt.Errorf("unable to save Tags for Event ID: %s. error: %v", ec.ID, errt)
	}

	return nil
}

// nip-02: the event is a contact list (kind=3), it should overwrite the existing contact list for the same PubKey;
func (r *nostrRepo) StoreEventK3(e *gn.Event) error {

	events := r.client.Collection(r.events_collection)
	q := events.Where("Kind", "==", 3).Where("PubKey", "==", e.PubKey).OrderBy("CreatedAt", firestore.Desc).Limit(1)

	docs, err := q.Documents(*r.ctx).GetAll()
	if err != nil {
		return fmt.Errorf("unable to get event from repository. error: %v", err)
	}
	if len(docs) < 1 {
		// no contact list for this PubKey - create new Doc
		err := r.StoreEvent(e)
		return err
	}

	ec := *e
	var tgs gn.Tags = ec.Tags
	ec.Tags = nil

	if len(tgs) < 1 {
		return nil
	}

	docID := docs[0].Ref.ID
	// delete the existing doc according to the protocol (nip-02 'Relays and clients SHOULD delete past contact lists as soon as they receive a new one.')
	_, err = events.Doc(docID).Delete(*r.ctx)
	if err != nil {
		return fmt.Errorf("unable to delete previous event from repository. error: %v", err)
	}

	return r.StoreEvent(e)

}

func tagsToTagMap(tgs gn.Tags) map[string]interface{} {

	// convert the tags into a map of string elements - TagMap
	tm := make(gn.TagMap)

	for i, tn := range tgs {
		tagslice := []string{}

		for _, tag := range tn {
			tagslice = append(tagslice, tag)
		}
		tm[fmt.Sprintf("%d", i)] = tagslice
	}

	return map[string]interface{}{"ТagsMap": tm}
}

func (r *nostrRepo) GetEvent(id string) (*gn.Event, error) {

	var e gn.Event
	doc, err := r.client.Collection(r.events_collection).Doc(id).Get(*r.ctx)

	if err != nil {
		return nil, fmt.Errorf("unable to get event from repository. error: %v", err)
	}

	if err := doc.DataTo(&e); err != nil {
		return nil, fmt.Errorf("unable to get event from repository. error: %v", err)
	}

	return &e, nil
}

func (r *nostrRepo) GetEvents(ids []string) ([]*gn.Event, error) {

	var events []*gn.Event
	for _, id := range ids {
		e, err := r.GetEvent(id)
		if err != nil {
			return nil, fmt.Errorf("unable to get events from repository. error: %v", err)
		}
		events = append(events, e)
	}

	return events, nil
}

// GetEventsByFilter - returns a list of events that match the filter provided from a client subscription
func (r *nostrRepo) GetEventsByFilter(filter map[string]interface{}) ([]*gn.Event, error) {
	var (
		err                                             error
		events, kind_events, ids_events, authors_events []*gn.Event
		max_length                                      int
		flt_lngth                                       int = len(filter)
		kinds, ids, authors                             []interface{}
	)

	// ===================================== FILTER TEMPLATE ============================================

	//<filters> is a JSON object that determines what events will be sent in that subscription, it can have the following attributes:
	//{
	//  "ids": <a list of event ids>,
	//  "authors": <a list of lowercase pubkeys, the pubkey of an event must be one of these>,
	//  "kinds": <a list of a kind numbers>,
	//  "#<single-letter (a-zA-Z)>": <a list of tag values, for #e — a list of event ids, for #p — a list of event pubkeys etc>,
	//  "since": <an integer unix timestamp in seconds, events must be newer than this to pass>,
	//  "until": <an integer unix timestamp in seconds, events must be older than this to pass>,
	//  "limit": <maximum number of events relays SHOULD return in the initial query>
	//}
	//==================================================================================================

	// =================================== LIMIT =======================================================
	// The limit of number of events returned by the filter should be set by either 1) the fileter or 2) by server limit. The cases where the filter's limit is 0, can be handled in the future for paying clients and when this function implements paging (portions of eevents) to be sent.
	// [ ]: implement paging

	lim, ok := filter["limit"].(float64)
	if !ok {
		// set the default max_length from config file
		max_length = r.default_limit
	} else if lim == 0 || lim > 5000 {
		// until the paging is implemented for paying clients OR always for public clients
		// 5000 is the value from attribute from server_info limits
		max_length = 5000
	} else {
		// set the requested by the filter value
		max_length = int(lim)
	}
	r.ilgr.Printf("filters limit max_length is set to: %v", max_length)

	// =================================== SINCE - UNTIL ===============================================
	var since, until interface{}
	if since, ok = filter["since"]; !ok {
		since = nil
	}
	if until, ok = filter["until"]; !ok {
		until = nil
	}

	// [x]:================================= KINDS =========================================================
	// Dealing with filetrs `kinds`...
	kinds, ok = filter["kinds"].([]interface{})
	if !ok {
		// in 99% of cases the key will be missing in the filter.
		goto IDS
	}
	if len(kinds) > 30 {
		kinds = kinds[:30]
	}

	kind_events, err = r.retrieveKinds(kinds, since, until, max_length)
	if err != nil {
		r.elgr.Printf("Error retrieving `kinds` from the database: %v", err)
	}
	r.ilgr.Printf("%d events retrieved from the DB with the `kinds` filter", len(kind_events))

	if (len(events) + len(kind_events)) <= max_length {
		events = append(events, kind_events...)
	} else {
		events = append(events, kind_events[:max_length-len(events)]...)
		return events, nil
	}

	// [x]:=================================== IDS ===========================================================
	// Dealing with filetrs `IDs`...
IDS:
	ids, ok = filter["ids"].([]interface{})
	if !ok {
		goto AUTHORS
	}
	if len(ids) > 30 {
		ids = ids[:30]
	}

	ids_events, err = r.retrievIDs(ids, since, until, max_length)
	if err != nil {
		r.elgr.Printf("Error retrieving `IDs` from the database: %v", err)
	}
	r.ilgr.Printf("%d events retrieved from the DB with the `IDs` filter", len(ids_events))

	if (len(events) + len(ids_events)) <= max_length {
		events = append(events, ids_events...)
	} else {
		events = append(events, ids_events[:max_length-len(events)]...)
		return events, nil
	}

	// [x]:=================================== AUTHORS =======================================================
	// Dealing with filetrs `authors`...
AUTHORS:
	authors, ok = filter["authors"].([]interface{})
	if !ok {
		goto SINCE
	}
	if len(authors) > 30 {
		authors = authors[:30]
	}

	authors_events, err = r.retrievAuthors(authors, since, until, max_length)
	if err != nil {
		r.elgr.Printf("Error retrieving `authors` from the database: %v", err)
	}
	r.ilgr.Printf("%d events retrieved from the DB with the `Authors` filter", len(authors_events))
	if (len(events) + len(authors_events)) <= max_length {
		events = append(events, authors_events...)
	} else {
		events = append(events, authors_events[:max_length-len(events)]...)
		return events, nil
	}

	// [ ]:=================================== TAGS ==========================================================

	// ======================================= SINCE - UNTIL ==================================================
SINCE:
	if since != nil && flt_lngth == 1 {
		fromPast, err := r.retrieveFromPast(since, max_length)
		if err != nil {
			r.elgr.Printf("Error retrieving `fromPast` from the database: %v", err)
		}
		r.ilgr.Printf("%d events retrieved from the DB with the `fromPast` filter", len(fromPast))
		if (len(events) + len(fromPast)) <= max_length {
			events = append(events, fromPast...)
		} else {
			events = append(events, fromPast[:max_length-len(events)]...)
			return events, nil
		}
	}

	return events, nil
}

// DeleteEvent deletes an event identified by its id
func (r *nostrRepo) DeleteEvent(id string) error {
	_, err := r.client.Collection(r.events_collection).Doc(id).Delete(*r.ctx)
	if err != nil {
		r.elgr.Printf("Error deleting event with id: %s from the database: %v", id, err)
	}
	return err
}

// DeleteEvents delete a series of events identified by their ids provided as an array of strings
func (r *nostrRepo) DeleteEvents(ids []string) error {
	var err error
	for _, id := range ids {
		err = r.DeleteEvent(id)
		if err != nil {
			return err
		}
	}
	return nil
}

// NewNostrRepository - creates a new nostr relay repository
// [x]: get the repo params from the config
func NewNostrRepository(ctx *context.Context, client *firestore.Client, dlv int, ecn string) (nostr.NostrRepo, error) {
	return &nostrRepo{
		ctx:               ctx,
		events_collection: ecn,
		client:            client,
		default_limit:     dlv,
		ilgr:              log.New(os.Stdout, "[nostr-repo] ", log.LstdFlags),
		elgr:              log.New(os.Stderr, "[nostr-repo] ", log.LstdFlags),
	}, nil
}

// retrieveKinds - query events collection from the DB by kinds and if there is valid time slot limitation applies that filtration to the returned result.
func (r *nostrRepo) retrieveKinds(kinds []interface{}, since, until interface{}, limit int) ([]*gn.Event, error) {

	var events []*gn.Event
	var kindsI []int64
	var lcnt int

	for _, kind := range kinds {
		_kind, ok := kind.(float64)
		if !ok {
			kindsI = append(kindsI, int64(-1))
			continue
		}
		kindsI = append(kindsI, int64(_kind))
	}

	var query *firestore.DocumentIterator

	switch {
	case since == nil && until == nil:
		query = r.client.Collection(r.events_collection).Where("Kind", "in", kindsI).Documents(*r.ctx)

	case since != nil && until == nil:
		tsSince, err := tools.ConvertToTS(since)
		if err != nil {
			return nil, err
		}
		query = r.client.Collection(r.events_collection).Where("Kind", "in", kindsI).Where("CreatedAt", ">", tsSince).Documents(*r.ctx)

	case since == nil && until != nil:
		tsUntil, err := tools.ConvertToTS(until)
		if err != nil {
			return nil, err
		}
		query = r.client.Collection(r.events_collection).Where("Kind", "in", kindsI).Where("CreatedAt", "<", tsUntil).Documents(*r.ctx)

	default:
		// [ ]: ??? to implement `between` (from `since` to `until`) use case...
		query = r.client.Collection(r.events_collection).Where("Kind", "in", kindsI).Documents(*r.ctx)

	}

	// Query the events collection
	// iterate over docs collection where the kind matches the filter kind
	for {
		doc, err := query.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			r.elgr.Printf("Error raised while reading a doc from the DB: %v", err)
			continue
		}

		e, err := r.transformTagMapIntoTAGS(doc)
		if err != nil {
			r.elgr.Printf("Error %v raised while converting DB doc ID: %v into nostr event", err, doc.Ref.ID)
			continue
		}
		lcnt++

		if lcnt <= limit {
			events = append(events, e)
		} else {
			break
		}
	}
	return events, nil
}

// retrievIDs - query events collection from the DB by IDs and if there is valid time slot limitation applies that filtration to the returned result.
func (r *nostrRepo) retrievIDs(ids []interface{}, since, until interface{}, limit int) ([]*gn.Event, error) {

	var (
		events []*gn.Event
		lcnt   int
		query  *firestore.DocumentIterator
		_ids   []string
	)

	for _, id := range ids {
		_id, ok := id.(string)
		if !ok {
			_ids = append(_ids, "")
			continue
		}
		_ids = append(_ids, _id)
	}

	switch {
	case since == nil && until == nil:
		query = r.client.Collection(r.events_collection).Where("ID", "in", _ids).Documents(*r.ctx)

	case since != nil && until == nil:
		tsSince, err := tools.ConvertToTS(since)
		if err != nil {
			return nil, err
		}
		query = r.client.Collection(r.events_collection).Where("ID", "in", _ids).Where("CreatedAt", ">", tsSince).Documents(*r.ctx)

	case since == nil && until != nil:
		tsUntil, err := tools.ConvertToTS(until)
		if err != nil {
			return nil, err
		}
		query = r.client.Collection(r.events_collection).Where("ID", "in", _ids).Where("CreatedAt", "<", tsUntil).Documents(*r.ctx)

	default:
		// [ ]: ??? to implement `between` (from `since` to `until`) use case...
		query = r.client.Collection(r.events_collection).Where("ID", "in", _ids).Documents(*r.ctx)

	}

	// Query the events collection
	// iterate over docs collection where the IDs matches the filter array of IDs
	for {
		doc, err := query.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			r.elgr.Printf("Error raised while reading a doc from the DB: %v", err)
			continue
		}

		e, err := r.transformTagMapIntoTAGS(doc)
		if err != nil {
			r.elgr.Printf("Error %v raised while converting DB doc ID: %v into nostr event", err, doc.Ref.ID)
			continue
		}
		lcnt++

		if lcnt <= limit {
			events = append(events, e)
		} else {
			break
		}
	}
	return events, nil
}

// retrievAuthors - query events collection from the DB by authors and if there is valid time slot limitation applies that filtration to the returned result.
func (r *nostrRepo) retrievAuthors(authors []interface{}, since, until interface{}, limit int) ([]*gn.Event, error) {

	var (
		events   []*gn.Event
		lcnt     int
		_authors []string
	)

	for _, author := range authors {
		_author, ok := author.(string)
		if !ok {
			_authors = append(_authors, "")
			continue
		}
		_authors = append(_authors, _author)
	}

	var query *firestore.DocumentIterator

	switch {
	case since == nil && until == nil:
		query = r.client.Collection(r.events_collection).Where("PubKey", "in", _authors).Documents(*r.ctx)

	case since != nil && until == nil:
		tsSince, err := tools.ConvertToTS(since)
		if err != nil {
			return nil, err
		}
		query = r.client.Collection(r.events_collection).Where("PubKey", "in", _authors).Where("CreatedAt", ">", tsSince).Documents(*r.ctx)

	case since == nil && until != nil:
		tsUntil, err := tools.ConvertToTS(until)
		if err != nil {
			return nil, err
		}
		query = r.client.Collection(r.events_collection).Where("PubKey", "in", _authors).Where("CreatedAt", "<", tsUntil).Documents(*r.ctx)

	default:
		// [ ]: ??? to implement `between` (from `since` to `until`) use case...
		query = r.client.Collection(r.events_collection).Where("PubKey", "in", _authors).Documents(*r.ctx)

	}

	// Query the events collection
	// iterate over docs collection where the PubKey matches the filter array of PubKeys
	for {
		doc, err := query.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			r.elgr.Printf("Error raised while reading a doc from the DB: %v", err)
			continue
		}

		e, err := r.transformTagMapIntoTAGS(doc)
		if err != nil {
			r.elgr.Printf("Error %v raised while converting DB doc ID: %v into nostr event", err, doc.Ref.ID)
			continue
		}
		lcnt++

		if lcnt <= limit {
			events = append(events, e)
		} else {
			break
		}
	}
	return events, nil
}

// retrieveFromPast - query events collection from the DB by createdAt and if there are events created after the `since` timestamp, applies that filtration to the returned result.
func (r *nostrRepo) retrieveFromPast(since interface{}, limit int) ([]*gn.Event, error) {

	var events []*gn.Event
	var lcnt int = 0

	var query *firestore.DocumentIterator

	tsSince, err := tools.ConvertToTS(since)
	if err != nil {
		return nil, err
	}

	query = r.client.Collection(r.events_collection).Where("CreatedAt", ">", tsSince).Documents(*r.ctx)

	// Query the events collection
	// iterate over docs collection where the PubKey matches the filter array of PubKeys
	for {
		doc, err := query.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			r.elgr.Printf("Error raised while reading a doc from the DB: %v", err)
			continue
		}

		e, err := r.transformTagMapIntoTAGS(doc)
		if err != nil {
			r.elgr.Printf("Error %v raised while converting DB doc ID: %v into nostr event", err, doc.Ref.ID)
			continue
		}

		if lcnt < limit {
			events = append(events, e)
			lcnt++
		} else {
			break
		}
	}

	return events, nil

}

// transformTagMapIntoTAGS - convert the TagsMap from the doc into a Tags array in the event `e`
func (r *nostrRepo) transformTagMapIntoTAGS(doc *firestore.DocumentSnapshot) (*gn.Event, error) {

	var e gn.Event
	if err := doc.DataTo(&e); err != nil {
		return nil, fmt.Errorf("Casting doc ID: %v into nostr event raised error: %v", doc.Ref.ID, err)
	}

	// convert the TagsMap from the doc into a Tags array in the event `e`
	tagsMap, ok := doc.Data()["ТagsMap"]
	if !ok {
		if tagsMap == nil {
			e.Tags = nil
			return &e, nil
		}
		return nil, fmt.Errorf("error transforming TagsMap for doc ID: %v", doc.Ref.ID)
	}

	for _, tm := range tagsMap.(map[string]interface{}) {
		ta := []string{}
		for _, t := range tm.([]interface{}) {
			ta = append(ta, t.(string))
		}
		e.Tags = append(e.Tags, ta)
	}
	return &e, nil
}
