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
func controlIPConn(l *log.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			ip, _, _ := net.SplitHostPort(r.RemoteAddr)

			if ip == "" || ip == "127.0.0.1" {
				ip = r.Header.Get("X-Real-IP")
			}
			l.Printf("[MW-controlIPC] IP: %s", ip)

			mu.Lock()
			if ipCount[ip] >= 3 {
				mu.Unlock()
				l.Printf("[MW-controlIPC] Too many requests from %s", ip)
				// [ ]: Consider to build a blacklist of IPs for very exccessive number of requests for short time
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}
			ipCount[ip]++
			mu.Unlock()

			defer func() {
				mu.Lock()
				ipCount[ip]--
				mu.Unlock()
			}()

			next.ServeHTTP(w, r)
		})
	}
}
