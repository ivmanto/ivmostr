package router

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/dasiyes/ivmostr-tdd/internal/server/ivmws"
	"github.com/dasiyes/ivmostr-tdd/tools"
)

var (
	//ipCount = make(map[string]int)
	mu = &sync.Mutex{}
)

// Handles the CORS part
func accessControl(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-TOKEN-TYPE, X-GRANT-TYPE, X-IVM-CLIENT")

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
		currentTimestamp := time.Now()
		ip := getIP(r)
		rc := &ivmws.RequestContext{IP: ip}
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

		ip := getIP(r)
		if ivmws.IPCount[ip] > 0 {
			fmt.Printf("[MW-ipc] Too many requests [%d] from %s\n", ivmws.IPCount[ip], ip)
			http.Error(w, "Bad request", http.StatusForbidden)
			return
		}
		mu.Lock()
		ivmws.IPCount[ip]++
		mu.Unlock()
		fmt.Printf("[MW-ipc] [+] client IP %s increased to %d active connection\n", ip, ivmws.IPCount[ip])

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
		//h.ServeHTTP(w, r)
	})
}

// getIP identifies the real IP address of the request
func getIP(r *http.Request) string {
	var ip string

	ip = r.Header.Get("X-Real-IP")
	if ip == "" {
		ip = r.Header.Get("X-Forwarded-For")
	}

	if ip == "" {
		ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	if ip == "127.0.0.1" {
		ip = r.RemoteAddr
	}
	return ip
}
