package services

import "sync"

type ledger struct {
	subscribers map[string]*Client
	mtx         *sync.Mutex
}

// Add will add the client under the key=name
// if there is no already such key in the map and return true.
// Otherwise it will return false and the existing value and do nothing.
func (l *ledger) Add(name string, c *Client) (bool, *Client) {
	l.mtx.Lock()
	ec := l.subscribers[name]
	if ec != nil {
		return false, ec
	}

	l.subscribers[name] = c
	l.mtx.Unlock()
	return true, nil
}

// Set will set the client under the key=name
// If there is a value under this key, it will be overwritten.
func (l *ledger) Set(name string, c *Client) {
	l.mtx.Lock()
	l.subscribers[name] = c
	l.mtx.Unlock()
}

func (l *ledger) Remove(name string) {
	l.mtx.Lock()
	delete(l.subscribers, name)
	l.mtx.Unlock()
}

func (l *ledger) Len() int {
	return len(l.subscribers)
}

func (l *ledger) Get(name string) *Client {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	return l.subscribers[name]
}

func NewLedger() *ledger {
	return &ledger{
		subscribers: make(map[string]*Client),
		mtx:         &sync.Mutex{},
	}
}
