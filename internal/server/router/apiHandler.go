package router

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/dasiyes/ivmostr-tdd/pkg/nostr"
	"github.com/go-chi/chi"
)

type apiHandler struct {
	l    *log.Logger
	repo nostr.NostrRepo
	// place any dependencies ...
}

func (ah *apiHandler) router() chi.Router {
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

func (ah *apiHandler) welcome(w http.ResponseWriter, r *http.Request) {

	_, _ = w.Write([]byte("{\"success\":\"Welcome to ivmostr api\"}"))
}

func (ah *apiHandler) serverinfo(w http.ResponseWriter, r *http.Request) {

	assetsPath, err := filepath.Abs("assets")
	if err != nil {
		ah.l.Printf("Failed to get absolute path to assets folder: %v", err)

	}

	// Read the contents of the server_info.json file
	filePath := filepath.Join(assetsPath, "server_info.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		ah.l.Printf("Failed to read server_info.json file from path %v, error: %v", filePath, err)
	}

	if len(data) > 0 {
		w.Header().Set("Content-Type", "application/nostr+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	} else {
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("{\"name\":\"ivmostr\"}"))
	}
}
