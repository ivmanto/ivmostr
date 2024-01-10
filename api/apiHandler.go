package api

import (
	"log"
	"net/http"

	"github.com/dasiyes/ivmostr-tdd/tools"
	"github.com/go-chi/chi"
)

type ApiHandler struct {
	Lgr *log.Logger
	// place any dependencies ...
}

func (ah *ApiHandler) Router() chi.Router {
	rtr := chi.NewRouter()

	// Route the static files
	// fileServer := http.FileServer(http.FS(ui.Files))
	// rtr.Handle("/static/*", fileServer)

	rtr.Route("/api", func(r chi.Router) {
		r.Get("/", ah.welcome)
		// r.Post("/send", rh.home)
		r.Get("/nip11", ah.serverinfo)
	})

	return rtr
}

func (ah *ApiHandler) welcome(w http.ResponseWriter, r *http.Request) {

	_, _ = w.Write([]byte("{\"success\":\"Welcome to ivmostr api\"}"))
}

func (ah *ApiHandler) serverinfo(w http.ResponseWriter, r *http.Request) {
	tools.ServerInfo(w, r)
}
