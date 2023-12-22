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
	IP        string
	RateLimit RateLimit
}
