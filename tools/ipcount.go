package tools

import "sync"

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
