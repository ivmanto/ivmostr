package ivmws

import (
	"time"
)

type KeyRC string

type RateLimit struct {
	Requests  int
	Timestamp time.Time
}

type RequestContext struct {
	// wsconns is a map with string key as IP address
	// and interface as type RateLimit as persistent
	// data storage for WS connection in the server
	// LT session
	WSConns map[string]*RateLimit

	// Max number of allowed connections before
	// the IP is blocked
	RateLimitMax int

	// The number of seconds (to be converted
	// in time.Duration) before the counter of connections
	// is reset.
	RateLimitDuration int
}
