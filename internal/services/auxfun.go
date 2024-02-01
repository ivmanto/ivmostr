package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/logging"
	"github.com/dasiyes/ivmostr-tdd/tools"
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
func filterMatch(e *gn.Event, filters []map[string]interface{}) bool {
	for _, filter := range filters {
		if tools.FilterMatchSingle(e, filter) {
			return true
		}
	}
	return false
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
