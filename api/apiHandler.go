package api

import (
	"fmt"
	"net/http"

	"github.com/dasiyes/ivmostr-tdd/tools"
	"github.com/go-chi/chi"
)

type ApiHandler struct {
	// place any dependencies ...
}

func (ah *ApiHandler) Router() chi.Router {
	rtr := chi.NewRouter()

	// Route the static files
	// fileServer := http.FileServer(http.FS(ui.Files))
	// rtr.Handle("/static/*", fileServer)

	rtr.Route("/", func(r chi.Router) {
		r.Get("/", ah.welcome)
		// r.Post("/send", rh.home)
		r.Get("/nip11", ah.serverinfo)
		r.Get("/ipcount", ah.ipcount)
		r.Get("/filters", ah.filters)
	})

	return rtr
}

func (ah *ApiHandler) welcome(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("{\"success\":\"Welcome to ivmostr api\"}"))
}

func (ah *ApiHandler) serverinfo(w http.ResponseWriter, r *http.Request) {
	tools.ServerInfo(w, r)
}

func (ah *ApiHandler) ipcount(w http.ResponseWriter, r *http.Request) {
	ipcount := tools.GetIPCount()
	ip, max := tools.IPCount.TopIP()
	_, _ = w.Write([]byte(fmt.Sprintf("{\"Active_IP_Connections\": %v,\"Max_connections_from_[%s]\": %v}", ipcount, ip, max)))
}

func (ah *ApiHandler) filters(w http.ResponseWriter, r *http.Request) {

	flt := tools.TAlaf.GetFiltersList()
	_, _ = w.Write([]byte(fmt.Sprintf("{\"Active_Filters\": \"%s\"}", flt)))
}
