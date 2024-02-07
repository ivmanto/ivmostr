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
	"net/http"

	"github.com/dasiyes/ivmostr-tdd/api"
	"github.com/dasiyes/ivmostr-tdd/configs/config"
	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/internal/server/ivmws"
	"github.com/dasiyes/ivmostr-tdd/tools"
	"github.com/go-chi/chi"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	l          = log.New()
	wlst, blst []string
)

// Constructing web application depenedencies in the format of handler
type srvHandler struct {
	repo  nostr.NostrRepo
	lists nostr.ListRepo
	cfg   *config.ServiceConfig
	// ... add other dependencies here
}

func (h *srvHandler) router() chi.Router {

	rtr := chi.NewRouter()

	rllgr := log.New()
	rllgr.Level = log.DebugLevel

	// Building middleware chain
	rtr.Use(accessControl)
	rtr.Use(healthcheck)
	rtr.Use(serverinfo)
	rtr.Use(rateLimiter(h.lists, wlst, blst, rllgr))
	rtr.Use(controlIPConn)

	// Handle requests to the root URL "/" - nostr websocket connections
	rtr.Route("/", func(wr chi.Router) {
		ws := ivmws.NewWSHandler(h.repo, h.lists, h.cfg, &wlst, &blst)
		wr.Mount("/", ws.Router())
	})

	// Handle Prometheus metrics
	rtr.Handle("/metrics", promhttp.Handler())

	// Route the API calls to/v1/api/ ...
	rtr.Route("/v1", func(r chi.Router) {
		rh := api.ApiHandler{}
		r.Mount("/api", rh.Router())
	})

	return rtr
}

// Handler to manage endpoints
func NewHandler(repo nostr.NostrRepo, lists nostr.ListRepo, cfg *config.ServiceConfig) http.Handler {

	e := srvHandler{
		repo:  repo,
		lists: lists,
		cfg:   cfg,
	}
	l.Printf("...initializing router (http server Handler) ...")

	wlst = tools.GetWhiteListedIPs(lists)
	blst = tools.GetBlackListedIPs(lists)
	l.Debugf("...white list: %d ...", len(wlst))
	l.Debugf("...black list: %d ...", len(blst))

	return e.router()
}
