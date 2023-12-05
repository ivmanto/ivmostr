package router

import (
	"log"
	"net"
	"net/http"
	"sync"
)

var (
	ipCount = make(map[string]int)
	mu      = &sync.Mutex{}
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

		ip, _, _ := net.SplitHostPort(r.RemoteAddr)

		if ip == "" || ip == "127.0.0.1" {
			ip = r.Header.Get("X-Real-IP")
		}
		if ip != "" {
			mu.Lock()
			ipCount[ip]++
			mu.Unlock()
			log.Printf("[MW-ipc] [+] IP: %s", ip)
		}

		if ipCount[ip] >= 3 {
			log.Printf("[MW-ipc] Too many requests [%d] from %s", ipCount[ip], ip)
			http.Error(w, "Bad request", http.StatusForbidden)
			return
		}

		defer func() {
			mu.Lock()
			ipCount[ip]--
			mu.Unlock()
			log.Printf("[MW-ipc] [-] IP: %s", ip)
		}()

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
