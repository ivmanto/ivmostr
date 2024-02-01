package tools

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
	gn "github.com/studiokaiji/go-nostr"
)

var (
	IPCount *ipCount
)

func init() {
	IPCount = NewIPCount()
}

type ipCount struct {
	ipcount    map[string]int
	ipTtlCount map[string]int
	mutex      *sync.Mutex
}

func (i *ipCount) Add(ip string) {
	i.mutex.Lock()
	i.ipcount[ip]++
	i.ipTtlCount[ip]++
	i.mutex.Unlock()
}

// ipcount keeps only the ACTIVE connected clients
func (i *ipCount) Remove(ip string) {
	i.mutex.Lock()
	i.ipcount[ip]--
	if i.ipcount[ip] < 1 {
		delete(i.ipcount, ip)
	}
	i.mutex.Unlock()
}

// Returns the number of total active connected clients
func (i *ipCount) Len() int {
	return len(i.ipcount)
}

// Returns the total active connections from an IP.
func (i *ipCount) IPConns(ip string) int {
	i.mutex.Lock()
	defer i.mutex.Unlock()
	return i.ipcount[ip]
}

// Returns the number of total connections made from the IP (since the service restart)
func (i *ipCount) IPConnsTotal(ip string) int {
	i.mutex.Lock()
	defer i.mutex.Unlock()
	return i.ipTtlCount[ip]
}

// Returns the number of Total distinct ip connections ()
func (i *ipCount) TotalConns() int {
	i.mutex.Lock()
	defer i.mutex.Unlock()
	return len(i.ipTtlCount)
}

// Top demanding (in terms of number of connections) client's IP
func (i *ipCount) TopIP() (ip string, max int) {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	for k, v := range i.ipTtlCount {
		if v > max {
			max = v
			ip = k
		}
	}
	return ip, max
}

func NewIPCount() *ipCount {
	return &ipCount{
		ipcount:    make(map[string]int),
		ipTtlCount: make(map[string]int),
		mutex:      &sync.Mutex{},
	}
}

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
	//
	// All conditions of a filter that are specified must match for an event for it to pass the filter, i.e., **multiple conditions are interpreted as `&&` conditions**.

	// [ ]: Refactor this to work with mixed filters - means when there is i.e. list of kinds [...] + until or since clause - then the filetr should run in sequens / nested format

	_, flt_authors := filter["authors"].([]interface{})
	_, flt_ids := filter["ids"].([]interface{})
	_, flt_kinds := filter["kinds"].([]interface{})
	var flt_tags bool
	for key, val := range filter {
		if strings.HasPrefix(key, "#") {
			_, flt_tags = val.([]interface{})
			break
		}
	}

	switch {
	case flt_authors && !flt_ids && !flt_kinds && !flt_tags:
		for _, author := range filter["authors"].([]interface{}) {
			if author.(string) == e.PubKey {
				return true
			}
		}
	case !flt_authors && flt_ids && !flt_kinds && !flt_tags:
		for _, id := range filter["ids"].([]interface{}) {
			if id.(string) == e.ID {
				return true
			}
		}
	case !flt_authors && !flt_ids && flt_kinds && !flt_tags:
		for _, kind := range filter["kinds"].([]interface{}) {
			if kind.(float64) == float64(e.Kind) {
				return true
			}
		}
	case flt_authors && !flt_ids && flt_kinds && !flt_tags:
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

		return kr && ar

	case !flt_authors && !flt_ids && flt_kinds && flt_tags:
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
		return kr && tr

	case !flt_authors && !flt_ids && !flt_kinds && flt_tags:
		for tkey, tval := range filter {
			switch tkey {
			case "#e":
				for _, tag := range tval.([]interface{}) {
					if tag.(string) == e.ID {
						return true
					}
				}
			case "#p":
				for _, tag := range tval.([]interface{}) {
					if tag.(string) == e.PubKey {
						return true
					}
				}
			case "#d":
				//[ ]: to be implemented later on
			default:
				continue
			}
		}

	default:
		return false
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

func PrintVersion() {

	f, err := os.OpenFile("version", os.O_RDONLY, 0666)
	if err != nil {
		log.Println("Error opening version file")
		return
	}
	defer f.Close()
	b := make([]byte, 100)
	_, err = f.Read(b)
	if err != nil {
		log.Println("Error reading version file")
		return
	}
	log.Println(string(b))
}

func ServerInfo(w http.ResponseWriter, r *http.Request) {

	ip := GetIP(r)
	org := r.Header.Get("Origin")
	log.Printf("providing server info to %v, %v...\n", ip, org)

	assetsPath, err := filepath.Abs("assets")
	if err != nil {
		log.Printf("ERROR: Failed to get absolute path to assets folder: %v", err)
	}

	// Read the contents of the server_info.json file
	filePath := filepath.Join(assetsPath, "server_info.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("ERROR:Failed to read server_info.json file from path %v, error: %v", filePath, err)
	}

	if len(data) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// send the data as json object in the response body
		_, _ = w.Write(data)
	} else {
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("{\"name\":\"ivmostr\"}"))
	}
}

func Contains(slice []string, element string) bool {
	for _, item := range slice {
		if item == element {
			return true
		}
	}
	return false
}

// getIP identifies the real IP address of the request
func GetIP(r *http.Request) string {
	var ip string

	ip = r.Header.Get("X-Real-IP")
	if ip == "" {
		xff := r.Header.Get("X-Forwarded-For")
		if len(xff) > 15 {
			xffs := strings.Split(xff, ",")
			if len(xffs) > 2 {
				ip = xffs[1]
			} else {
				ip = xffs[0]
			}
		}
	}

	if ip == "" {
		ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	if ip == "127.0.0.1" {
		ip = r.RemoteAddr
	}
	return ip
}

func DiscoverHost(r *http.Request) string {
	var host string
	_host := r.Host
	h_host := r.Header.Get("Host")
	fh_host := r.Header.Get("X-Forwarded-Host")
	url_host := r.URL.Host

	// find NOT empty host
	if _host == "" {
		if h_host == "" {
			if fh_host == "" {
				if url_host == "" {
					log.Printf("HOST value not find in the request")
				}
			} else {
				host = fh_host
			}
		} else {
			host = h_host
		}
	} else {
		host = _host
	}

	return host
}

func CalcLenghtInBytes(i *[]interface{}) int {
	var wstr string
	for _, intf := range *i {

		switch reflect.TypeOf(intf).String() {
		case "bool":
			if intf.(bool) {
				wstr = wstr + "true"
			} else {
				wstr = wstr + "false"
			}
		case "[]*nostr.Event":
			out, _ := json.Marshal(intf)
			wstr = wstr + string(out)
		case "*nostr.Event":
			out, _ := json.Marshal(intf)
			wstr = wstr + string(out)
		default:
			wstr = wstr + intf.(string)
		}
	}
	return len([]byte(wstr))
}

func ConvertStructToByte(e any) ([]byte, error) {

	gob.Register(map[string]interface{}{})
	gob.Register(gn.Event{})
	gob.Register([]interface{}{})

	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	errE := enc.Encode(e)
	if errE != nil {
		log.Println("[ConvertStructToByte] Error encoding struct []events:", errE)
		return nil, errE
	}
	return buf.Bytes(), nil
}

func GetIPCount() int {
	return IPCount.Len()
}
