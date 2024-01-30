package firestoredb

import (
	"context"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/firestore/apiv1/firestorepb"
	gn "github.com/studiokaiji/go-nostr"
	"google.golang.org/api/iterator"

	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/pkg/fspool"
	log "github.com/sirupsen/logrus"
)

// Holds the required objects by `nostr` relay according to the protocol
type nostrRepo struct {
	ctx               *context.Context
	mtx               *sync.Mutex
	events_collection string
	clients           *fspool.ConnectionPool
	fsclient          *firestore.Client
	default_limit     int
	rlgr              *log.Logger
}

func (r *nostrRepo) StoreEvent(e *gn.Event) error {

	// [ ]: review ========= client per job ============
	fsclient, err := r.clients.GetClient()
	if err != nil {
		r.rlgr.Errorf("unable to get firestore client. error: %v", err)
		return err
	}

	// ==================== end of client ================

	// multi-level array are not supported by firestore! It must be converted into a array of objects with string elements
	ec := *e
	// [ ]: Add extra field ExpireAt. The value of live should be different for public (7 days = 604800s) and paid versions(TBD???).
	ec.SetExtra("ExpireAt", float64(time.Now().Unix()+604800))

	var tgs gn.Tags = ec.Tags
	ec.Tags = nil

	// Filter out the events kinds that should not be stored according to the `nostr` protocol
	if ec.Kind == 22242 {
		r.rlgr.Warnf("AUTH event should not be stored in the repository")
		return nil
	}

	r.mtx.Lock()
	_, errc := fsclient.Collection(r.events_collection).Doc(e.ID).Create(*r.ctx, ec)
	defer r.mtx.Unlock()

	if errc != nil {
		r.rlgr.Errorf("unable to save in clients repository. error: %v", errc)
		return errc
	}

	if len(tgs) < 1 {
		return nil
	}

	tags := tagsToTagMap(tgs)
	docRef := fsclient.Collection(r.events_collection).Doc(ec.ID)

	//r.mtx.Lock()
	_, errt := docRef.Set(*r.ctx, tags, firestore.MergeAll)
	//defer r.mtx.Unlock()

	if errt != nil {
		r.rlgr.Errorf("unable to save Tags for Event ID: %s. error: %v", ec.ID, errt)
		return errt
	}

	r.rlgr.Infof("Event ID: %s saved in repository.", ec.ID)
	return nil
}

// nip-02: the event is a contact list (kind=3), it should overwrite the existing contact list for the same PubKey;
func (r *nostrRepo) StoreEventK3(e *gn.Event) error {

	// [ ]: review ========= client per job ============
	fsclient, err := r.clients.GetClient()
	if err != nil {
		return fmt.Errorf("unable to get firestore client. error: %v", err)
	}
	// ==================== end of client ================

	events := fsclient.Collection(r.events_collection)
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

func (r *nostrRepo) GetEvent(id string) (*gn.Event, error) {

	// [ ]: review ========= client per job ============
	fsclient, err := r.clients.GetClient()
	if err != nil {
		return nil, fmt.Errorf("unable to get firestore client. error: %v", err)
	}
	// ==================== end of client ================

	var e gn.Event
	doc, err := fsclient.Collection(r.events_collection).Doc(id).Get(*r.ctx)

	if err != nil {
		return nil, fmt.Errorf("unable to get event from repository. error: %v", err)
	}

	if err := doc.DataTo(&e); err != nil {
		return nil, fmt.Errorf("unable to get event from repository. error: %v", err)
	}

	return &e, nil
}

func (r *nostrRepo) GetEvents(ids []string) ([]*gn.Event, error) {

	// [ ]: review for refactoring. Could be better performing if NOT calling GetEvent for each event
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

func (r *nostrRepo) TotalDocs2() (int, error) {

	// [ ]: review ========= client per job ============
	fsclient, err := r.clients.GetClient()
	if err != nil {
		return 0, fmt.Errorf("unable to get firestore client. error: %v", err)
	}
	// ==================== end of client ================

	// Calculate the total number of documents available in the collection r.events_collection

	agq := fsclient.Collection(r.events_collection).NewAggregationQuery().WithCount("total_events")

	countResult, err := agq.Get(*r.ctx)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return -1, err
	}

	count := countResult["total_events"]
	countValue := count.(*firestorepb.Value)

	return int(countValue.GetIntegerValue()), nil

}

func (r *nostrRepo) TotalDocs() (int, error) {

	// [ ]: review ========= client per job ============
	fsclient, err := r.clients.GetClient()
	if err != nil {
		return 0, fmt.Errorf("unable to get firestore client. error: %v", err)
	}
	// ==================== end of client ================

	// Calculate the total number of documents available in the collection r.events_collection
	var total int
	iter := fsclient.Collection(r.events_collection).Documents(*r.ctx)
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("unable to get total number of documents from repository. error: %v", err)
		}
		total++
	}
	return total, nil
}

// GetEventsByKinds - returns an array of events from a specific Kind observing the limit and time frames
func (r *nostrRepo) GetEventsByKinds(kinds []int, limit int, since, until int64) ([]*gn.Event, error) {

	ts := time.Now().UnixMilli()

	// [ ]: review ========= client per job ============
	fsclient, errc := r.clients.GetClient()
	if errc != nil {
		return nil, fmt.Errorf("unable to get firestore client. error: %v", errc)
	}
	// ==================== end of client ================

	var (
		events     []*gn.Event
		query      *firestore.DocumentIterator
		lcnt, ecnt int
	)

	if len(kinds) > 30 {
		kinds = kinds[:30]
	}

	switch {
	case since == 0 && until > 0:
		query = fsclient.Collection(r.events_collection).Where("Kind", "in", kinds).Where("CreatedAt", "<", until).OrderBy("CreatedAt", firestore.Desc).Limit(limit).Documents(*r.ctx)

	case since > 0 && until == 0:
		query = fsclient.Collection(r.events_collection).Where("Kind", "in", kinds).Where("CreatedAt", ">", since).OrderBy("CreatedAt", firestore.Desc).Limit(limit).Documents(*r.ctx)

	case since > 0 && until > 0:
		query = fsclient.Collection(r.events_collection).Where("Kind", "in", kinds).Where("CreatedAt", ">", since).Where("CreatedAt", "<", until).OrderBy("CreatedAt", firestore.Desc).Limit(limit).Documents(*r.ctx)

	default:
		// [ ]: ??? to implement `between` (from `since` to `until`) use case...
		query = fsclient.Collection(r.events_collection).Where("Kind", "in", kinds).OrderBy("CreatedAt", firestore.Desc).Limit(limit).Documents(*r.ctx)

	}

	for {
		doc, err := query.Next()
		if err != nil {
			if err == iterator.Done || ecnt > 3 {
				break
			}
			r.rlgr.Errorf("[GetEventsByKinds] ERROR raised while reading a doc from the DB: %v", err)
			ecnt++
			continue
		}

		e, err := r.transformTagMapIntoTAGS(doc)
		if err != nil {
			r.rlgr.Errorf("[GetEventsByKinds] ERROR: %v raised while converting DB doc ID: %v into nostr event", err, doc.Ref.ID)
			continue
		}

		if lcnt < limit {
			events = append(events, e)
		} else {
			break
		}
		lcnt++
	}

	rsl := fmt.Sprintf(`{"events": %d, "filter_kinds": "%v", "limit": %d, "since": %d, "until": %d, "took": %d}`, len(events), kinds, limit, since, until, time.Now().UnixMilli()-ts)

	r.rlgr.Debugf("%v", rsl)

	if len(events) == 0 {
		return nil, fmt.Errorf("[GetEventsByKinds] no events found for the provided filter")
	}

	return events, nil
}

// GetEventsByAuthors - ...
func (r *nostrRepo) GetEventsByAuthors(authors []string, limit int, since, until int64) ([]*gn.Event, error) {

	ts := time.Now().UnixMilli()

	// [ ]: review ========= client per job ============
	fsclient, errc := r.clients.GetClient()
	if errc != nil {
		return nil, fmt.Errorf("unable to get firestore client. error: %v", errc)
	}
	// ==================== end of client ================

	var (
		events     []*gn.Event
		query      *firestore.DocumentIterator
		lcnt, ecnt int
	)

	if len(authors) > 30 {
		authors = authors[:30]
	}

	switch {
	case since == 0 && until > 0:
		query = fsclient.Collection(r.events_collection).Where("PubKey", "in", authors).Where("CreatedAt", "<", until).OrderBy("CreatedAt", firestore.Desc).Limit(limit).Documents(*r.ctx)

	case since > 0 && until == 0:
		query = fsclient.Collection(r.events_collection).Where("PubKey", "in", authors).Where("CreatedAt", ">", since).OrderBy("CreatedAt", firestore.Desc).Limit(limit).Documents(*r.ctx)

	case since > 0 && until > 0:
		query = fsclient.Collection(r.events_collection).Where("PubKey", "in", authors).Where("CreatedAt", ">", since).Where("CreatedAt", "<", until).OrderBy("CreatedAt", firestore.Desc).Limit(limit).Documents(*r.ctx)

	default:
		// [ ]: ??? to implement `between` (from `since` to `until`) use case...
		query = fsclient.Collection(r.events_collection).Where("PubKey", "in", authors).OrderBy("CreatedAt", firestore.Desc).Limit(limit).Documents(*r.ctx)

	}

	for {
		doc, err := query.Next()
		if err != nil {
			if err == iterator.Done || ecnt > 3 {
				break
			}
			r.rlgr.Errorf("[GetEventsByAuthors] ERROR: raised while reading a doc from the DB: %v", err)
			ecnt++
			continue
		}

		e, err := r.transformTagMapIntoTAGS(doc)
		if err != nil {
			r.rlgr.Errorf("[GetEventsByAuthors] ERROR: %v raised while converting DB doc ID: %v into nostr event", err, doc.Ref.ID)
			continue
		}

		if lcnt < limit {
			events = append(events, e)
		} else {
			break
		}
		lcnt++
	}

	rsl := fmt.Sprintf(`{"events": %d, "filter_authors": "%v", "limit": %d, "since": %d, "until": %d, "took": %d}`, len(events), authors, limit, since, until, time.Now().UnixMilli()-ts)

	r.rlgr.Debugf("%v", rsl)

	if len(events) == 0 {
		return nil, fmt.Errorf("[GetEventsByAuthors] no events found for the provided filter")
	}

	return events, nil
}

// GetEventsByIds - getting events listed into the ids array
func (r *nostrRepo) GetEventsByIds(ids []string, limit int, since, until int64) ([]*gn.Event, error) {

	ts := time.Now().UnixMilli()

	// [ ]: review ========= client per job ============
	fsclient, errc := r.clients.GetClient()
	if errc != nil {
		return nil, fmt.Errorf("unable to get firestore client. error: %v", errc)
	}
	// ==================== end of client ================

	var (
		events     []*gn.Event
		query      *firestore.DocumentIterator
		lcnt, ecnt int
	)

	if len(ids) > 30 {
		ids = ids[:30]
	}

	switch {
	case since == 0 && until > 0:
		query = fsclient.Collection(r.events_collection).Where("ID", "in", ids).Where("CreatedAt", "<", until).OrderBy("CreatedAt", firestore.Desc).Limit(limit).Documents(*r.ctx)

	case since > 0 && until == 0:
		query = fsclient.Collection(r.events_collection).Where("ID", "in", ids).Where("CreatedAt", ">", since).OrderBy("CreatedAt", firestore.Desc).Limit(limit).Documents(*r.ctx)

	case since > 0 && until > 0:
		query = fsclient.Collection(r.events_collection).Where("ID", "in", ids).Where("CreatedAt", ">", since).Where("CreatedAt", "<", until).OrderBy("CreatedAt", firestore.Desc).Limit(limit).Documents(*r.ctx)

	default:
		query = fsclient.Collection(r.events_collection).Where("ID", "in", ids).OrderBy("CreatedAt", firestore.Desc).Limit(limit).Documents(*r.ctx)

	}

	for {
		doc, err := query.Next()
		if err != nil {
			if err == iterator.Done || ecnt > 3 {
				break
			}
			r.rlgr.Errorf("[GetEventsByIds] ERROR: raised while reading a doc from the DB: %v", err)
			ecnt++
			continue
		}

		e, err := r.transformTagMapIntoTAGS(doc)
		if err != nil {
			r.rlgr.Errorf("[GetEventsByIds] ERROR: %v raised while converting DB doc ID: %v into nostr event", err, doc.Ref.ID)
			continue
		}

		if lcnt < limit {
			events = append(events, e)
		} else {
			break
		}
		lcnt++
	}

	rsl := fmt.Sprintf(`{"events": %d, "filter_ids": "%v", "limit": %d, "since": %d, "until": %d, "took": %d}`, len(events), ids, limit, since, until, time.Now().UnixMilli()-ts)

	r.rlgr.Debugf("%v", rsl)

	if len(events) == 0 {
		return nil, fmt.Errorf("[GetEventsByIds] no events found for the provided filter")
	}

	return events, nil
}

// GetEventsSince - get only events created after `since`
func (r *nostrRepo) GetEventsSince(limit int, since int64) ([]*gn.Event, error) {

	ts := time.Now().UnixMilli()

	// [ ]: review ========= client per job ============
	fsclient, errc := r.clients.GetClient()
	if errc != nil {
		return nil, fmt.Errorf("unable to get firestore client. error: %v", errc)
	}
	// ==================== end of client ================

	var (
		events     []*gn.Event
		query      *firestore.DocumentIterator
		lcnt, ecnt int
	)

	query = fsclient.Collection(r.events_collection).Where("CreatedAt", ">", since).OrderBy("CreatedAt", firestore.Desc).Limit(limit).Documents(*r.ctx)

	for {
		doc, err := query.Next()
		if err != nil {
			if err == iterator.Done || ecnt > 3 {
				break
			}
			r.rlgr.Errorf("[GetEventsSince] ERROR raised while reading a doc from the DB: %v", err)
			ecnt++
			continue
		}

		e, err := r.transformTagMapIntoTAGS(doc)
		if err != nil {
			r.rlgr.Errorf("[GetEventsSince] ERROR: %v raised while converting DB doc ID: %v into nostr event", err, doc.Ref.ID)
			continue
		}

		if lcnt < limit {
			events = append(events, e)
		} else {
			break
		}
		lcnt++
	}

	rsl := fmt.Sprintf(`{"events": %d, "filter_all": "all_events", "limit": %d, "since": %d, "took": %d}`, len(events), limit, since, time.Now().UnixMilli()-ts)

	r.rlgr.Debugf("%v", rsl)

	if len(events) == 0 {
		return nil, fmt.Errorf("[GetEventsSince] no events found for the provided filter")
	}

	return events, nil
}

// GetEventsSince - get only events created after `since`
func (r *nostrRepo) GetEventsSinceUntil(limit int, since, until int64) ([]*gn.Event, error) {

	ts := time.Now().UnixMilli()

	// [ ]: review ========= client per job ============
	fsclient, errc := r.clients.GetClient()
	if errc != nil {
		return nil, fmt.Errorf("unable to get firestore client. error: %v", errc)
	}
	// ==================== end of client ================

	var (
		events     []*gn.Event
		query      *firestore.DocumentIterator
		lcnt, ecnt int
	)

	query = fsclient.Collection(r.events_collection).Where("CreatedAt", ">", since).Where("CreatedAt", "<", until).OrderBy("CreatedAt", firestore.Desc).Limit(limit).Documents(*r.ctx)

	for {
		doc, err := query.Next()
		if err != nil {
			if err == iterator.Done || ecnt > 3 {
				break
			}
			r.rlgr.Errorf("[GetEventsSinceUntil] ERROR raised while reading a doc from the DB: %v", err)
			ecnt++
			continue
		}

		e, err := r.transformTagMapIntoTAGS(doc)
		if err != nil {
			r.rlgr.Errorf("[GetEventsSinceUntil] ERROR: %v raised while converting DB doc ID: %v into nostr event", err, doc.Ref.ID)
			continue
		}

		if lcnt < limit {
			events = append(events, e)
		} else {
			break
		}
		lcnt++
	}

	rsl := fmt.Sprintf(`{"events": %d, "filter_all": "all_events", "limit": %d, "since": %d, "until": %d,"took": %d}`, len(events), limit, since, until, time.Now().UnixMilli()-ts)

	r.rlgr.Debugf("%v", rsl)

	if len(events) == 0 {
		return nil, fmt.Errorf("[GetEventsSinceUntil] no events found for the provided filter")
	}

	return events, nil
}

func (r *nostrRepo) GetLastNEvents(limit int) ([]*gn.Event, error) {

	ts := time.Now().UnixMilli()

	// [ ]: review ========= client per job ============
	fsclient, errc := r.clients.GetClient()
	if errc != nil {
		return nil, fmt.Errorf("unable to get firestore client. error: %v", errc)
	}
	// ==================== end of client ================

	var (
		events []*gn.Event
		query  *firestore.DocumentIterator
		ecnt   int
	)

	// Get a reference to the "events" collection
	query = fsclient.Collection(r.events_collection).OrderBy("CreatedAt", firestore.Desc).Limit(limit).Documents(*r.ctx)

	for {
		doc, err := query.Next()
		if err != nil {
			if err == iterator.Done || ecnt > 3 {
				break
			}
			r.rlgr.Errorf("[GetLastNEvents] ERROR raised while reading a doc from the DB: %v", err)
			ecnt++
			continue
		}

		e, err := r.transformTagMapIntoTAGS(doc)
		if err != nil {
			r.rlgr.Errorf("[GetLastNEvents] ERROR: %v raised while converting DB doc ID: %v into nostr event", err, doc.Ref.ID)
			continue
		}

		events = append(events, e)
	}

	rsl := fmt.Sprintf(`{"events": %d, "filter_all": "all_events", "limit": %d, "took": %d}`, len(events), limit, time.Now().UnixMilli()-ts)

	r.rlgr.Debugf("%v", rsl)

	if len(events) == 0 {
		return nil, fmt.Errorf("[GetLastNEvents] no events found for the provided filter")
	}

	return events, nil
}

func (r *nostrRepo) GetEventsBtAuthorsAndKinds(authors []string, kinds []int, limit int, since, until int64) ([]*gn.Event, error) {

	ts := time.Now().UnixMilli()

	// [ ]: review ========= client per job ============
	fsclient, errc := r.clients.GetClient()
	if errc != nil {
		return nil, fmt.Errorf("unable to get firestore client. error: %v", errc)
	}
	// ==================== end of client ================

	var (
		events []*gn.Event
		query  *firestore.DocumentIterator
		ecnt   int
	)

	if len(kinds) > 30 {
		kinds = kinds[:30]
	}
	if len(authors) > 30 {
		authors = authors[:30]
	}

	switch {
	case since == 0 && until > 0:
		query = fsclient.Collection(r.events_collection).Where("PubKey", "in", authors).Where("Kind", "in", kinds).Where("CreatedAt", "<", until).Limit(limit).Documents(*r.ctx)

	case since > 0 && until == 0:
		query = fsclient.Collection(r.events_collection).Where("PubKey", "in", authors).Where("Kind", "in", kinds).Where("CreatedAt", ">", since).Limit(limit).Documents(*r.ctx)

	case since > 0 && until > 0:
		query = fsclient.Collection(r.events_collection).Where("PubKey", "in", authors).Where("Kind", "in", kinds).Where("CreatedAt", ">", since).Where("CreatedAt", "<", until).Limit(limit).Documents(*r.ctx)

	default:
		// when both since and until are 0s.
		query = fsclient.Collection(r.events_collection).Where("PubKey", "in", authors).Where("Kind", "in", kinds).Limit(limit).Documents(*r.ctx)

	}

	for {
		doc, err := query.Next()
		if err != nil {
			if err == iterator.Done || ecnt > 3 {
				break
			}
			r.rlgr.Errorf("[GetEventsBtAuthorsAndKinds] ERROR raised while reading a doc from the DB: %v", err)
			ecnt++
			continue
		}

		e, err := r.transformTagMapIntoTAGS(doc)
		if err != nil {
			r.rlgr.Errorf("[GetEventsBtAuthorsAndKinds] ERROR: %v raised while converting DB doc ID: %v into nostr event", err, doc.Ref.ID)
			continue
		}

		events = append(events, e)
	}

	rsl := fmt.Sprintf(`{"events": %d, "filter_all": "all_events", "limit": %d, "took": %d}`, len(events), limit, time.Now().UnixMilli()-ts)

	r.rlgr.Debugf("%v", rsl)

	if len(events) == 0 {
		return nil, fmt.Errorf("[GetEventsBtAuthorsAndKinds] no events found for the provided filter")
	}

	return events, nil
}

// DeleteEvent deletes an event identified by its id
func (r *nostrRepo) DeleteEvent(id string) error {

	// [ ]: review ========= client per job ============
	fsclient, err := r.clients.GetClient()
	if err != nil {
		return fmt.Errorf("unable to get firestore client. error: %v", err)
	}
	// ==================== end of client ================

	_, err = fsclient.Collection(r.events_collection).Doc(id).Delete(*r.ctx)
	if err != nil {
		r.rlgr.Errorf("Error deleting event with id: %s from the database: %v", id, err)
	}

	r.clients.ReleaseClient(fsclient)
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
func NewNostrRepository(ctx *context.Context, clients *fspool.ConnectionPool, dlv int, ecn string) (nostr.NostrRepo, error) {

	client, err := clients.GetClient()
	if err != nil {
		return nil, fmt.Errorf("unable to get firestore client. error: %v", err)
	}

	nr := nostrRepo{
		ctx:               ctx,
		mtx:               &sync.Mutex{},
		events_collection: ecn,
		clients:           clients,
		fsclient:          client,
		default_limit:     dlv,
		rlgr:              log.New(),
	}

	return &nr, nil
}

// ================================== Supports functions =====================================
//

// tagsToTagMap converts the event tags into gntagMap
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
