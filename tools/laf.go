package tools

import (
	"encoding/json"
	"fmt"
	"time"
)

var (
	// The Alaf holds analytical slice of sessions grow with
	// actively subscribed clients and their subscription Filetrs
	TAlaf alaf = alaf{}
)

type Alaf interface {
	GetDistinctClients() []string
	GetFiltersList() []string
}

type alaf []laf

// The Laf (List Of Active Filetrs) is a refference to
// a map of filters that are currently active in the session ledger.
type laf struct {
	// ledger list of filetrs
	llaf map[string]interface{}
	ts   int64
}

// PullLAF to be called out from the session state
func PullLAF(sslaf map[string]interface{}) {
	lafinst := laf{
		llaf: sslaf,
		ts:   time.Now().Unix(),
	}
	TAlaf = append(TAlaf, lafinst)
}

// GetDistinctClients ()
func (l alaf) GetDistinctClients() []string {

	var clients []string

	for _, v := range l {
		for k := range v.llaf {
			if !Contains(clients, k) {
				clients = append(clients, k)
			}
		}
	}
	return clients
}

// GetDistinctClientsAndNrOfFilters ()
func (l alaf) GetDistinctClientsAndNrOfFilters() map[string]int {

	var clients = make(map[string]int)

	for idx, v := range l {
		// take the last state only
		if idx != len(l)-1 {
			continue
		}
		for k, intf := range v.llaf {
			flt_len := len(intf.([]map[string]interface{}))
			clients[k] += flt_len
		}
	}

	return clients
}

// GetFiltersList ()
func (l alaf) GetFiltersList() []string {
	var filters []string
	for idx, v := range l {
		// take the last state only
		if idx != len(l)-1 {
			continue
		}
		for _, lf := range v.llaf {
			for _, flt := range lf.([]map[string]interface{}) {

				bflt, err := json.Marshal(flt)
				if err != nil {
					fmt.Printf("Error: while marshal filter: %v", err)
					continue
				}
				filters = append(filters, string(bflt))
			}
		}
	}
	return filters
}

// New laf instance
func NewAlaf() Alaf {
	return alaf{}
}
