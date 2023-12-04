/*
MIT License

# Copyright (c) 2022 nbd

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

	"cloud.google.com/go/logging"
	"github.com/dasiyes/ivmostr-tdd/pkg/nostr"
	"github.com/go-chi/chi"
)

// Constructing web application depenedencies in the format of handler
type srvHandler struct {
	l     *log.Logger
	clgr  *logging.Logger
	repo  nostr.NostrRepo
	lrepo nostr.ListRepo
	// ... add other dependencies here
}

func (h *srvHandler) router() chi.Router {

	rtr := chi.NewRouter()

	// Building middleware chain
	rtr.Use(accessControl)
	rtr.Use(controlIPConn(h.l))

	// Route the home calls
	rtr.Route("/", func(r chi.Router) {
		lgr := log.New(os.Stdout, "[http][api] ", log.LstdFlags)
		rh := apiHandler{lgr, h.repo}
		r.Mount("/v1", rh.router())
	})

	return rtr
}

// Handler to manage endpoints
func NewHandler(l *log.Logger, cl *logging.Logger, repo nostr.NostrRepo, lrepo nostr.ListRepo) http.Handler {

	e := srvHandler{
		l:     l,
		clgr:  cl,
		repo:  repo,
		lrepo: lrepo,
	}
	e.l.Printf("...initializing router (http server Handler) ...")

	return e.router()
}
