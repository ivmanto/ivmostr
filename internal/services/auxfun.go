package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/logging"
	"github.com/dasiyes/ivmostr-tdd/tools"
	gn "github.com/nbd-wtf/go-nostr"
	log "github.com/sirupsen/logrus"
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
