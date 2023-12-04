package tools

import (
	"fmt"
	"math/big"
	"strconv"

	gn "github.com/nbd-wtf/go-nostr"
)

// Filter attributes containing lists (ids, authors, kinds and tag filters like #e) are JSON arrays with one or more values.
// [x] At least one of the arrays' values must match the relevant field in an event for the condition to be considered a match.
// [ ] For scalar event attributes such as authors and kind, the attribute from the event must be contained in the filter list.
// [ ] In the case of tag attributes such as #e, for which an event may have multiple values, the event and filter condition values must have at least one item in common.

// The ids, authors, #e and #p filter lists MUST contain exact 64-character lowercase hex values.

// The since and until properties can be used to specify the time range of events returned in the subscription. If a filter includes the since property, events with created_at greater than or equal to since are considered to match the filter. The until property is similar except that created_at must be less than or equal to until. In short, an event matches a filter if since <= created_at <= until holds.

// All conditions of a filter that are specified must match for an event for it to pass the filter, i.e., multiple conditions are interpreted as && conditions.

// A REQ message may contain multiple filters. In this case, events that match any of the filters are to be returned, i.e., multiple filters are to be interpreted as || conditions.

// The limit property of a filter is only valid for the initial query and MUST be ignored afterwards. When limit: n is present it is assumed that the events returned in the initial query will be the last n events ordered by the created_at. It is safe to return less events than limit specifies, but it is expected that relays do not return (much) more events than requested so clients don't get unnecessarily overwhelmed by data.
func FilterMatchSingle(e *gn.Event, filter map[string]interface{}) bool {

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

	// [ ]: Refactor this to work with mixed filters - means when there is i.e. list of kinds [...] + until or since clause - then the filetr should run in sequens / nested format
	for k, v := range filter {
		switch k {
		case "ids":
			ids, ok := v.([]interface{})
			if !ok {
				fmt.Printf("ids value %v is not `[]interface{}`", v)
				return false
			}
			for _, id := range ids {
				if id.(string) == e.ID {
					return true
				}
			}
		case "authors":
			authors, ok := v.([]interface{})
			if !ok {
				fmt.Printf("authors value %v is not `[]interface{}`", v)
				return false
			}
			for _, author := range authors {
				if author.(string) == e.PubKey {
					return true
				}
			}
		case "kinds":
			kinds, ok := v.([]interface{})
			if !ok {
				fmt.Printf("kinds value %v is not `[]interface{}`", v)
				return false
			}
			for _, kind := range kinds {
				if kind.(float64) == float64(e.Kind) {
					return true
				}
			}
		case "since":
			since, err := ConvertToTS(v)
			if err != nil {
				fmt.Printf("Error %v converting the since value", err)
				return false
			}
			if since <= e.CreatedAt {
				return true
			}
		case "until":
			until, err := ConvertToTS(v)
			if err != nil {
				fmt.Printf("Error %v converting the until value", err)
				return false
			}
			if until >= e.CreatedAt {
				return true
			}
		}
	}

	return false
}

func scientificNotationToUInt(scientificNotation string) (uint, error) {
	flt, _, err := big.ParseFloat(scientificNotation, 10, 0, big.ToNearestEven)
	if err != nil {
		return 0, err
	}
	fltVal := fmt.Sprintf("%.0f", flt)
	intVal, err := strconv.ParseInt(fltVal, 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(intVal), nil
}

func ConvertToTS(v interface{}) (gn.Timestamp, error) {

	vt, ok := v.(float64)
	if ok {
		ts := gn.Timestamp(int64(vt))
		return ts, nil
	}

	vs, ok := v.(string)
	if ok {
		ui, err := scientificNotationToUInt(vs)
		if err != nil {
			return gn.Timestamp(int64(0)), fmt.Errorf("provided value %v is in wrong format", v)
		}
		ts := gn.Timestamp(int64(ui))
		return ts, nil
	}
	return gn.Timestamp(int64(0)), fmt.Errorf("value %v is unknown type", v)
}