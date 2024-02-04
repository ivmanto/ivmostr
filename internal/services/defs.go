package services

import (
	"strings"

	gn "github.com/studiokaiji/go-nostr"
)

type eventClientPair struct {
	event  *gn.Event
	client *Client
}

// Filter combination bitwise
// a - authors
// i - ids
// k - kinds
// t - tags
// s - since
// u - until
//
// aiktsu
// 000000

const (
	fcAuthors           = 1 << iota                     // 100000
	fcIds                                               // 010000
	fcKinds                                             // 001000
	fcTags                                              // 000100
	fcSince                                             // 000010
	fcUntil                                             // 000001
	fcAuthorsKinds      = fcAuthors | fcKinds           // 101000
	fcAuthorsKindsSince = fcAuthors | fcKinds | fcSince // 101010
	fcAuthorsKindsUntil = fcAuthors | fcKinds | fcUntil // 101001
	fcAuthorsSince      = fcAuthors | fcSince           // 100010
	fcAuthorsUntil      = fcAuthors | fcUntil           // 100001
	fcAuthorsSinceUntil = fcAuthors | fcSince | fcUntil // 100011
	fcSinceUntil        = fcSince | fcUntil             // 000011
	fcKindsSince        = fcKinds | fcSince             // 001010
	fcKindsUntil        = fcKinds | fcUntil             // 001001
	fcKindsSinceUntil   = fcKinds | fcSince | fcUntil   // 001011
	fcKindsTags         = fcKinds | fcTags              // 001100
	fcKindsTagsSince    = fcKinds | fcTags | fcSince    // 001110
	fcKindsTagsUntil    = fcKinds | fcTags | fcUntil    // 001101
	fcAuthorsTags       = fcAuthors | fcTags            // 100100
	fcAuthorsTagsSince  = fcAuthors | fcTags | fcSince  // 100110
	fcAuthorsTagsUntil  = fcAuthors | fcTags | fcUntil  // 100101
)

// filterState
type filterState struct {
	fltState uint
	since    gn.Timestamp
	until    gn.Timestamp
}

// parseFilter

func parseFilter(filter map[string]interface{}) *filterState {

	var (
		a, i, k, t, s, u bool
		_since, _until   float64
		since, until     gn.Timestamp
	)

	_, a = filter["authors"].([]interface{})
	_, i = filter["ids"].([]interface{})
	_, k = filter["kinds"].([]interface{})

	for key, val := range filter {
		if strings.HasPrefix(key, "#") {
			_, t = val.([]interface{})
			break
		}
	}

	// ========   SINCE - UNTIL ==================
	if _since, s = filter["since"].(float64); s {
		since = gn.Timestamp(int64(_since))
	}

	if _until, u = filter["until"].(float64); u {
		until = gn.Timestamp(int64(_until))
	}

	// Calculate filter state to match the list of State Constants
	// based on available filter components
	fltst := []bool{a, i, k, t, s, u}
	var fltState uint = 0

	for i, value := range fltst {
		if value {
			fltState |= 1 << i
		}
	}

	state := filterState{
		fltState: fltState,
		since:    since,
		until:    until,
	}

	return &state
}
