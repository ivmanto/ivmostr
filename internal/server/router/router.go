/*
MIT License

# Copyright (c) 2023 ivmanto

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package router

import (
	"context"
	"net/http"
	"net/http/pprof"
	"sync"

	"github.com/dasiyes/ivmostr-tdd/api"
	"github.com/dasiyes/ivmostr-tdd/configs/config"
	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/internal/server/ivmws"
	"github.com/go-chi/chi"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	l = log.New()
)

// Constructing web application depenedencies in the format of handler
type srvHandler struct {
	repo   nostr.NostrRepo
	lists  nostr.ListRepo
	reqctx *ivmws.RequestContext
	ctx    context.Context
	cfg    *config.ServiceConfig
	mtx    sync.Mutex
	rllgr  *log.Logger
	// ... add other dependencies here
}

func (h *srvHandler) router() chi.Router {

	rtr := chi.NewRouter()

	h.rllgr = log.New()
	h.rllgr.Level = log.ErrorLevel

	// Building middleware chain
	rtr.Use(accessControl)
	// healthcheck and serverinfo, both should stay before rateLimiter
	rtr.Use(healthcheck)
	rtr.Use(serverinfo)

	// rate Limiter is intended to apply to WebSocket requests
	// addressed at root path `/`.
	rtr.Use(rateLimiter(h))

	// Handle requests to the root URL "/" - nostr websocket connections
	rtr.Route("/", func(wr chi.Router) {
		ws := ivmws.NewWSHandler(h.repo, h.lists, h.cfg)
		wr.Mount("/", ws.Router())
	})

	// Handle Prometheus metrics
	rtr.Handle("/metrics", promhttp.Handler())

	// Handle pprof endpoints
	rtr.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	rtr.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	rtr.Handle("/debug/pprof/block", pprof.Handler("block"))
	rtr.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	rtr.Handle("/debug/pprof/cmdline", pprof.Handler("cmdline"))
	rtr.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	rtr.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	rtr.Handle("/debug/pprof/trace", pprof.Handler("trace"))

	// Route the API calls to/v1/api/ ...
	rtr.Route("/v1", func(r chi.Router) {
		rh := api.ApiHandler{}
		r.Mount("/api", rh.Router())
	})

	return rtr
}

// Handler to manage endpoints
func NewHandler(repo nostr.NostrRepo, lists nostr.ListRepo, cfg *config.ServiceConfig) http.Handler {

	var (
		// New persistent storage for request context values
		rc = ivmws.RequestContext{
			WSConns:           make(map[string]*ivmws.RateLimit),
			RateLimitMax:      cfg.RateLimitMax,
			RateLimitDuration: cfg.RateLimitDuration,
		}

		// Create a new RequestContext
		ctx = context.WithValue(context.Background(), ivmws.KeyRC("requestContext"), &rc)
	)

	e := srvHandler{
		repo:   repo,
		lists:  lists,
		reqctx: &rc,
		ctx:    ctx,
		cfg:    cfg,
		mtx:    sync.Mutex{},
	}

	l.Printf("...initializing router (http server Handler) ...")

	return e.router()
}
