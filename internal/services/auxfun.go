package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/logging"
	"github.com/dasiyes/ivmostr-tdd/tools"
	"github.com/dasiyes/ivmostr-tdd/tools/metrics"
	log "github.com/sirupsen/logrus"
	gn "github.com/studiokaiji/go-nostr"
)

// mapToEvent converts a map[string]interface{} to a gn.Event
func mapToEvent(m map[string]interface{}) (*gn.Event, error) {
	var e gn.Event

	for key, value := range m {
		switch key {
		case "id":
			e.ID = value.(string)
		case "pubkey":
			e.PubKey = value.(string)
		case "created_at":
			e.CreatedAt = getTS(value)
		case "kind":
			e.Kind = getKind(value)
		case "tags":
			e.Tags = getTags(value.([]interface{}))
		case "content":
			e.Content = value.(string)
		case "sig":
			e.Sig = value.(string)
		default:
			log.Println("Unknown key:", key)
		}
	}

	return &e, nil
}

func getTags(value []interface{}) gn.Tags {
	var t gn.Tags

	for _, v := range value {
		ss := []string{}
		if _, ok := v.([]interface{}); !ok {
			if _, ok := v.([]string); ok {
				ss = append(ss, v.([]string)...)
			} else {
				t = gn.Tags{}
				break
			}
		} else {
			for _, v2 := range v.([]interface{}) {
				ss = append(ss, fmt.Sprint(v2))
			}
		}
		t = append(t, ss)
	}
	return t
}

func getTS(value interface{}) gn.Timestamp {
	if value == nil {
		return gn.Timestamp(time.Now().Unix())
	}

	if _, ok := value.(float64); !ok {
		if _, ok := value.(int); !ok {
			if _, ok := value.(int64); !ok {
				log.Println("invalid timestamp")
				return 0
			} else {
				return gn.Timestamp(value.(int64))
			}
		} else {
			return gn.Timestamp(value.(int))
		}
	} else {
		return gn.Timestamp(value.(float64))
	}
}

func getKind(value interface{}) int {
	if _, ok := value.(float64); !ok {
		if _, ok := value.(int64); !ok {
			if _, ok := value.(int); !ok {
				return -1
			} else {
				return value.(int)
			}
		} else {
			return int(value.(int64))
		}
	} else {
		return int(value.(float64))
	}
}

func composeErrorMsg(err error) string {
	var emsg string

	if strings.Contains(err.Error(), "code = AlreadyExists") {
		emsg = "duplicate: " + err.Error()
	} else {
		emsg = "error:" + err.Error()
	}

	return emsg
}

// convertToJSON converts a []byte into a JSON object []interface{}
func convertToJSON(payload []byte) ([]interface{}, error) {
	var imsg []interface{}

	err := json.Unmarshal(payload, &imsg)
	if err != nil {
		return nil, err
	}
	return imsg, nil
}

// convertIfc - converts json interface into JSON object and sends it into the writer stream as NewLineEncoded JSON (useful for BigQuery)
// func convertIfc(w io.Writer, v interface{}) error {
// 	err := json.NewEncoder(w).Encode(v)
// 	if err != nil {
// 		return err
// 	}
// 	return nil
// }

// Initiate GCP Cloud logger
func initCloudLogger(project_id, log_name string) *logging.Logger {
	// Initate Cloud logging
	ctx := context.Background()
	clientCldLgr, _ = logging.NewClient(ctx, project_id)
	clientCldLgr.OnError = func(err error) {
		log.Errorf("[cloud-logger] Error [%v] raised while logging to cloud logger", err)
	}
	return clientCldLgr.Logger(log_name)
}

// Checking if the specific event `e` matches atleast one of the filters of customers subscription;
func filterMatch() {

	fmlgr := log.New()
	fmlgr.SetLevel(log.DebugLevel)
	fmlgr.Debug("... Spining up filterMatch ...")

	for pair := range chEM {

		fmlgr.Debugf("[filterMatch] a new pair arrived in the chEM channel as event: [%v] and client: [%v]", *pair.event, pair.client)

		evnt := pair.event
		clnt := pair.client
		filters := pair.client.GetFilters()

		var (
			result = make(chan bool)
			fltr   map[string]interface{}
			wg     sync.WaitGroup
		)

		for _, filter := range filters {

			wg.Add(1)
			fltr = filter
			fmlgr.Debugf("[filterMatch] processing filter [%v]", filter)

			go filterMatchSingle(evnt, filter, result)
		}

		match := <-result
		if match {
			clnt.msgwt <- []interface{}{*evnt}
			// Updating the metrics channel
			metrics.ChBroadcastEvent <- 1

		} else {
			// [ ]: TO REMOVE this else clause after debug. NO required
			fmlgr.Debugf("*** Filter [%v] does not match the event [%v]!", fltr, *evnt)
		}
		wg.Wait()
		close(result)
	}
}

// === NOTES:
// Filter attributes containing lists (ids, authors, kinds and tag filters like #e) are JSON arrays with one or more values.
// [x] At least one of the arrays' values must match the relevant field in an event for the condition to be considered a match.
// [ ] For scalar event attributes such as authors and kind, the attribute from the event must be contained in the filter list.
// [ ] In the case of tag attributes such as #e, for which an event may have multiple values, the event and filter condition values must have at least one item in common.
//
// The ids, authors, #e and #p filter lists MUST contain exact 64-character lowercase hex values.
//
// The since and until properties can be used to specify the time range of events returned in the subscription. If a filter includes the since property, events with created_at greater than or equal to since are considered to match the filter. The until property is similar except that created_at must be less than or equal to until. In short, an event matches a filter if since <= created_at <= until holds.
//
// All conditions of a filter that are specified must match for an event for it to pass the filter, i.e., multiple conditions are interpreted as && conditions.
//
// A REQ message may contain multiple filters. In this case, events that match any of the filters are to be returned, i.e., multiple filters are to be interpreted as || conditions.
//
// The limit property of a filter is only valid for the initial query and MUST be ignored afterwards. When limit: n is present it is assumed that the events returned in the initial query will be the last n events ordered by the created_at. It is safe to return less events than limit specifies, but it is expected that relays do not return (much) more events than requested so clients don't get unnecessarily overwhelmed by data.

// filterMatchSingle verfies if an Event `e` attributes match a specific subscription filter values.
func filterMatchSingle(e *gn.Event, filter map[string]interface{}, rslt chan bool) {

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
	//
	// All conditions of a filter that are specified must match for an event for it to pass the filter, i.e., **multiple conditions are interpreted as `&&` conditions**.

	// [ ]: Refactor this to work with mixed filters - means when there is i.e. list of kinds [...] + until or since clause - then the filetr should run in sequens / nested format
	filterState := parseFilter(filter)

	switch filterState.fltState {
	// [x]: authors
	case fcAuthors:
		for _, author := range filter["authors"].([]interface{}) {
			if author.(string) == e.PubKey {
				rslt <- true
				return
			}
		}

	// [x]: ids
	case fcIds:
		for _, id := range filter["ids"].([]interface{}) {
			if id.(string) == e.ID {
				rslt <- true
				return
			}
		}

	// [x]: kinds
	case fcKinds:
		for _, kind := range filter["kinds"].([]interface{}) {
			if kind.(float64) == float64(e.Kind) {
				rslt <- true
				return
			}
		}

	// [x]: since
	case fcSince:
		if e.CreatedAt >= filterState.since {
			rslt <- true
			return
		}

	// [x]: until
	case fcUntil:
		if e.CreatedAt <= filterState.until {
			rslt <- true
			return
		}

	// [ ]: tags (implemented ONLY #e and #p)
	case fcTags:
		for tkey, tval := range filter {
			switch tkey {
			case "#e":
				for _, tag := range tval.([]interface{}) {
					if tag.(string) == e.ID {
						rslt <- true
						return
					}
				}
			case "#p":
				for _, tag := range tval.([]interface{}) {
					if tag.(string) == e.PubKey {
						rslt <- true
						return
					}
				}
			case "#d":
				//[ ]: to be implemented later on
			default:
				continue
			}
		}

	// [x]: authors && kinds
	case fcAuthorsKinds:
		var kr, ar bool

		for _, kind := range filter["kinds"].([]interface{}) {
			if kind.(float64) == float64(e.Kind) {
				kr = true
				break
			}
		}

		for _, author := range filter["authors"].([]interface{}) {
			if author.(string) == e.PubKey {
				ar = true
				break
			}
		}

		rslt <- (kr && ar)
		return

	// [x]: kinds && tags
	case fcKindsTags:
		var kr, tr bool

		for _, kind := range filter["kinds"].([]interface{}) {
			if kind.(float64) == float64(e.Kind) {
				kr = true
				break
			}
		}

		for tkey, tval := range filter {
			switch tkey {
			case "#e":
				for _, tag := range tval.([]interface{}) {
					if tag.(string) == e.ID {
						tr = true
						break
					}
				}
			case "#p":
				for _, tag := range tval.([]interface{}) {
					if tag.(string) == e.PubKey {
						tr = true
					}
				}
			default:
				continue
			}
		}
		rslt <- (kr && tr)
		return

	default:
		log.Debugf("*** Filter combination [%v] not implemented", filter)
		rslt <- false
		return
	}

	rslt <- false
}

// checkAndConvertFilterLists will work with filter's lists for `authors`, `ids`,
// `#e` and `#p` tags. All of them required to be 64 chars length strings
// representing lower case HEX value.
func checkAndConvertFilterLists(fl interface{}, key string) (cclist []string) {

	var (
		svlist  []string
		_svlist []string
		ok      bool
	)

	svlist, ok = fl.([]string)
	if !ok {
		log.Errorf("Wrong filter format used! [%s] is not a list of string values", key)
		return nil
	}

	// limit of the number of elements comes from Firestore query limit for the IN clause
	if len(svlist) > 30 {
		svlist = svlist[:30]
	}

	for _, item := range svlist {

		// nip-01: `The ids, authors, #e and #p filter lists MUST contain exact 64-character lowercase hex values.`
		_, errhx := strconv.ParseUint(item, 16, 64)
		if len(item) == 64 && errhx == nil {
			item = strings.ToLower(item)
			_svlist = append(_svlist, item)
		} else {
			log.Debugf("Wrong list value! It must be 64 chars long lowercase hex value. Skiping this value.")
		}
	}
	return _svlist
}

// validateSubsFilters makes validations check according to the requirements of nip-01
func validateSubsFilters(filter map[string]interface{}) bool {

	// Identiify filter's components that must be lowcase 64 chars hex values

	for key := range filter {
		if tools.Contains([]string{"authors", "ids", "#e", "#p"}, key) {
			collection, ok := filter[key].([]string)
			if ok {
				if !validateAIEP(collection) {
					return false
				}
			} else {
				return false
			}
		}
	}

	kinds, ok := filter["kinds"]
	if ok {
		ka, ok := kinds.([]float64)
		if len(ka) < 1 || !ok {
			return false
		}
	}

	return true
}

// validateAIEP - validates string arrays if all elements are
// 64 char length strings representing HEX values
func validateAIEP(array []string) bool {
	for _, item := range array {
		_, errhx := strconv.ParseUint(item, 16, 64)
		if len(item) != 64 && errhx != nil {
			return false
		}
	}
	return true
}
