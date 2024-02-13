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
	"time"

	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	log "github.com/sirupsen/logrus"
	gn "github.com/studiokaiji/go-nostr"
)

var (
	WList, BList []string
)

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

func AddToBlacklist(ip string, lst nostr.ListRepo) {
	bld := nostr.BlackList{
		IP:        ip,
		CreatedAt: time.Now().Unix(),
		ExpiresAt: time.Now().Add(168 * time.Hour).Unix(),
	}

	// Keep local list updated
	BList = append(BList, ip)

	err := lst.StoreBlackList(&bld)
	if err != nil {
		log.Errorf("[srvHandler] error blacklisting ip: %s, Error:%v", ip, err)
	}
}

func GetBlackListedIPs(lst nostr.ListRepo) {

	BList, err := lst.GetBLIPS()
	if err != nil {
		log.Error("[GetBlackListedIPs] error getting blacklisted ips: ", err)
	}
	log.Debugf("[GetBlackListedIPs] black list %v", BList)

}

func GetWhiteListedIPs(lst nostr.ListRepo) {

	WList, err := lst.GetWLIPS()
	if err != nil {
		log.Error("[GetWhiteListedIPs] error getting whitelisted ips: ", err)
	}
	log.Debugf("[GetWhiteListedIPs] black list %v", WList)
}
