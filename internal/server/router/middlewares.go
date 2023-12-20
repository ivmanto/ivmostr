package router

import (
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/dasiyes/ivmostr-tdd/internal/server/ivmws"
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

// Handles the IP address control part
func controlIPConn(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ip string

		ip = r.Header.Get("X-Real-IP")
		if ip == "" {
			ip = r.Header.Get("X-Forwarded-For")
		}

		if ip == "" {
			ip, _, _ = net.SplitHostPort(r.RemoteAddr)
		}

		if ip != "" {
			if ip == "127.0.0.1" {
				ip = r.RemoteAddr
			}
			if ivmws.IPCount[ip] > 0 {
				fmt.Printf("[MW-ipc] Too many requests [%d] from %s\n", ivmws.IPCount[ip], ip)
				http.Error(w, "Bad request", http.StatusForbidden)
				return
			}
			mu.Lock()
			ivmws.IPCount[ip]++
			mu.Unlock()
			fmt.Printf("[MW-ipc] [+] client IP %s increased to %d active connection\n", ip, ivmws.IPCount[ip])
		}

		h.ServeHTTP(w, r)
	})
}

// Handle ServerInfo requests
func serverinfo(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if r.Header.Get("Accept") == "application/nostr+json" {
			if r.URL.Path == "" || r.URL.Path == "/" {
				r.URL.Path = "/v1/api/nip11"
			}
		}
		h.ServeHTTP(w, r)
	})
}
