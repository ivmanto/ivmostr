package router

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/internal/server/ivmws"
	"github.com/dasiyes/ivmostr-tdd/tools"
	log "github.com/sirupsen/logrus"
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
func rateLimiter(lists nostr.ListRepo, wlst, blst []string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			rllgr := log.New()

			// Check if the request is a health-check request
			if r.URL.Path == "/metrics" || strings.HasPrefix(r.URL.Path, "/v1/api") {
				next.ServeHTTP(w, r)
				return
			}

			ac := r.Header.Get("Accept")
			if strings.Contains(ac, "application/nostr+json") {
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
			wlst = append(wlst, ips...)
			if tools.Contains(wlst, ip) {
				next.ServeHTTP(w, r)
				return
			}

			// blacklist IPs
			if tools.Contains(blst, ip) {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprintf(w, "Forbidden")
				return
			}

			// Create a new RequestContext
			ctx := context.WithValue(r.Context(), ivmws.KeyRC("requestContext"), rc)

			// Retrieve the RateLimit for the IP address from the context
			rateLimit := ctx.Value(ivmws.KeyRC("requestContext")).(*ivmws.RequestContext).RateLimit

			// Check if the IP address has made too many requests recently
			if time.Since(rateLimit.Timestamp) < time.Minute*180 {
				if rateLimit.Requests >= 1 {
					// Block the request
					rllgr.Debugf("[rateLimiter] Too many requests from IP address %s within 30 minutes.\n", ip)
					go tools.AddToBlacklist(ip, lists)

					w.WriteHeader(http.StatusTooManyRequests)
					fmt.Fprintf(w, "Too many requests! The ip will be blacklisted!")

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

			next.ServeHTTP(w, r)
		})
	}
}

// Handles the IP address control part
func controlIPConn(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if r.URL.Path == "/hc" || r.URL.Path == "/metrics" {
			h.ServeHTTP(w, r)
			return
		}

		ip := tools.GetIP(r)

		// whitelist IPs
		if tools.Contains(ips, ip) {
			h.ServeHTTP(w, r)
			return
		}

		if tools.IPCount.IPConns(ip) > 3 {
			// fmt.Printf(
			// 	"[MW-ipc] Too many requests [%d] from %s, headers [upgrade %v, accept %v, sec-ws-p %v], req URL [%v]\n", tools.IPCount[ip], ip, r.Header.Get("Upgrade"), r.Header.Get("Accept"), r.Header.Get("Sec-WebSocket-Protocol"), r.URL)
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
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
