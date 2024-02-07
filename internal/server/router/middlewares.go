package router

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dasiyes/ivmostr-tdd/internal/server/ivmws"
	"github.com/dasiyes/ivmostr-tdd/tools"
)

var (
	ips = []string{"188.193.116.7", "109.43.33.44"}
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

// Handles healthchecks
func healthcheck(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hc" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "OK")
			return
		}
		h.ServeHTTP(w, r)
	})
}

// Handles the rate Limit control
func rateLimiter(h *srvHandler) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			// Check if the request is for any other Path but root
			// and skip further RateLimit check for them
			if r.URL.Path != "/" {
				next.ServeHTTP(w, r)
				return
			}

			// Check if the request is a websocket request
			uh := r.Header.Get("Upgrade")
			ch := r.Header.Get("Connection")
			//wsp := r.Header.Get("Sec-WebSocket-Protocol")

			if strings.ToLower(uh) != "websocket" && strings.ToLower(ch) != "upgrade" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, "Bad request")
				return
			}

			// TODO: To review later but as of now (20231223) no-one is setting this Header properlly
			// if !strings.Contains(wsp, "nostr") {
			// 	w.WriteHeader(http.StatusFailedDependency)
			// 	fmt.Fprintf(w, "Bad protocol")
			// 	return
			// }

			// Get the current timestamp and IP address
			currentTimestamp := time.Now()
			ip := tools.GetIP(r)

			// whitelist IPs
			wlst = append(wlst, ips...)
			if tools.Contains(wlst, ip) {
				// [!]POLICY: No more than X (5?) total active connections
				// for WHITELISTED clients.
				if tools.IPCount.IPConns(ip) >= 5 {
					h.rllgr.Debugf("[rateLimiter] Too many connections from whitelisted IP address %s.", ip)
					http.Error(w, "Too many requests", http.StatusTooManyRequests)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// blacklist IPs
			if tools.Contains(blst, ip) {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprintf(w, "Forbidden")
				return
			}

			// Retrieve the RateLimit for the IP address from the context
			reqContext, ok := h.ctx.Value(ivmws.KeyRC("requestContext")).(*ivmws.RequestContext)
			if !ok {
				h.rllgr.Errorf("[rateLimiter] Failed to retrieve RateLimit from context.\n")
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "Internal Server Error")
				return
			}

			rateLimit := reqContext.WSConns[ip]
			if rateLimit == nil {
				reqContext.WSConns[ip] = &ivmws.RateLimit{
					Requests:  0,
					Timestamp: currentTimestamp,
				}
				rateLimit = reqContext.WSConns[ip]
			}

			// Check if the IP address has made too many requests recently
			if time.Since(rateLimit.Timestamp) < time.Second*time.Duration(reqContext.RateLimitDuration) {
				if rateLimit.Requests >= reqContext.RateLimitMax {
					// Block the request
					h.rllgr.Debugf("[rateLimiter] Too many requests from IP address %s within 30 minutes.\n", ip)
					go tools.AddToBlacklist(ip, h.lists)

					w.WriteHeader(http.StatusTooManyRequests)
					fmt.Fprintf(w, "TooManyRequests")

					return
				} else {
					// Update the request count
					rateLimit.Requests++
					rateLimit.Timestamp = currentTimestamp
					h.rllgr.Debugf("[rateLimiter] IP address %s made %d requests until %v.\n", ip, rateLimit.Requests, rateLimit.Timestamp)
				}
			} else {
				// Set the initial request count and timestamp for the IP address
				rateLimit.Requests = 1
				rateLimit.Timestamp = currentTimestamp
				h.rllgr.Debugf("[rateLimiter] IP address %s reset requests to 1 at %v.\n", ip, rateLimit.Timestamp)
			}

			next.ServeHTTP(w, r)
		})
	}
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
