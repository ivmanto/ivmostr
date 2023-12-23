package router

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/dasiyes/ivmostr-tdd/internal/server/ivmws"
	"github.com/dasiyes/ivmostr-tdd/tools"
)

var (
	ips = []string{"188.194.53.116"}
)

// Handles the CORS part
func accessControl(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Host, Origin, Content-Type, Authorization, X-TOKEN-TYPE, X-GRANT-TYPE, X-IVM-CLIENT")

		if r.Method == "OPTIONS" {
			w.Header().Set("Access-Control-Max-Age", "3600")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		h.ServeHTTP(w, r)
	})
}

// Handles the rate Limit control
func rateLimiter(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uh := r.Header.Get("Upgrade")
		ac := r.Header.Get("Accept")
		//wsp := r.Header.Get("Sec-WebSocket-Protocol")

		if uh != "websocket" && ac != "application/nostr+json" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Bad request")
			return
		}
		// [ ]: To review later but as of now (20231223) no-one is setting this Header properlly
		// if !strings.Contains(wsp, "nostr") {
		// 	w.WriteHeader(http.StatusFailedDependency)
		// 	fmt.Fprintf(w, "Bad protocol")
		// 	return
		// }

		// Get the current timestamp and IP address
		currentTimestamp := time.Now()
		ip := tools.GetIP(r)
		rc := &ivmws.RequestContext{IP: ip}

		// whitelist IPs
		if tools.Contains(ips, ip) {
			h.ServeHTTP(w, r)
			return
		}

		// Create a new RequestContext
		ctx := context.WithValue(r.Context(), ivmws.KeyRC("requestContext"), rc)

		// Retrieve the RateLimit for the IP address from the context
		rateLimit := ctx.Value(ivmws.KeyRC("requestContext")).(*ivmws.RequestContext).RateLimit

		// Check if the IP address has made too many requests recently
		if time.Since(rateLimit.Timestamp) < time.Second*10 {
			if rateLimit.Requests >= 10 {
				// Block the request
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprintf(w, "Too many requests from IP address %s within 10 seconds.\n", ip)
				// [ ]: add the ip to the blacklist (once blocking of IPs from the blacklist is implemented)
				return
			} else {
				// Update the request count
				rateLimit.Requests++
				rateLimit.Timestamp = currentTimestamp
			}
		} else {
			// Set the initial request count and timestamp for the IP address
			rateLimit.Requests = 1
			rateLimit.Timestamp = currentTimestamp
		}

		h.ServeHTTP(w, r)
	})
}

// Handles the IP address control part
func controlIPConn(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ip := tools.GetIP(r)

		// whitelist IPs
		if tools.Contains(ips, ip) {
			h.ServeHTTP(w, r)
			return
		}

		if tools.IPCount[ip] > 0 {
			fmt.Printf(
				"[MW-ipc] Too many requests [%d] from %s, headers [upgrade %v, accept %v, sec-ws-p %v], req URL [%v]\n", tools.IPCount[ip], ip, r.Header.Get("Upgrade"), r.Header.Get("Accept"), r.Header.Get("Sec-WebSocket-Protocol"), r.URL)
			http.Error(w, "Bad request", http.StatusForbidden)
			return
		}

		h.ServeHTTP(w, r)
	})
}

// Handle ServerInfo requests
func serverinfo(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if r.Header.Get("Accept") == "application/nostr+json" {
			tools.ServerInfo(w, r)
			return
		}
		h.ServeHTTP(w, r)
	})
}
