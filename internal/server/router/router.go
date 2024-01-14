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
	"log"
	"net/http"
	"os"

	"github.com/dasiyes/ivmostr-tdd/api"
	"github.com/dasiyes/ivmostr-tdd/configs/config"
	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/internal/server/ivmws"
	"github.com/go-chi/chi"
)

// Constructing web application depenedencies in the format of handler
type srvHandler struct {
	l    *log.Logger
	repo nostr.NostrRepo
	cfg  *config.ServiceConfig
	// ... add other dependencies here
}

func (h *srvHandler) router() chi.Router {

	rtr := chi.NewRouter()

	// Building middleware chain
	rtr.Use(accessControl)
	rtr.Use(healthcheck)
	rtr.Use(serverinfo)
	rtr.Use(rateLimiter)
	rtr.Use(controlIPConn)

	// Handle requests to the root URL "/"
	rtr.Route("/", func(wr chi.Router) {
		lgr := log.New(os.Stdout, "[wsh] ", log.LstdFlags)
		ws := ivmws.NewWSHandler(lgr, h.repo, h.cfg)
		wr.Mount("/", ws.Router())
	})

	// Route the API calls to/v1/api/ ...
	rtr.Route("/api", func(r chi.Router) {
		lgr := log.New(os.Stdout, "[api] ", log.LstdFlags)
		rh := api.ApiHandler{Lgr: lgr}
		r.Mount("/v1", rh.Router())
	})

	return rtr
}

// Handler to manage endpoints
func NewHandler(l *log.Logger, repo nostr.NostrRepo, cfg *config.ServiceConfig) http.Handler {

	e := srvHandler{
		l:    l,
		repo: repo,
		cfg:  cfg,
	}
	e.l.Printf("...initializing router (http server Handler) ...")

	return e.router()
}
