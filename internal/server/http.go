package server

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/logging"
)

type Instance struct {
	lg         *log.Logger
	httpServer *http.Server
}

func NewInstance() *Instance {

	s := &Instance{
		lg:         log.New(os.Stdout, "[http]: ", log.LstdFlags),
		httpServer: &http.Server{},
	}
	s.lg.Println("...initiating new Instance of HTTP server...")

	return s
}

func (s *Instance) Start(addr string, endp http.Handler) error {

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: endp,
	}

	err := s.httpServer.ListenAndServe() // Blocks!
	if err != http.ErrServerClosed {
		s.lg.Printf("http Server stopped unexpected: %v", err)
		s.Shutdown()
	} else {
		s.lg.Printf("CRITICAL: Http Server stopped: %v", err)
	}
	return err
}

func (s *Instance) Shutdown() {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := s.httpServer.Shutdown(ctx)
		if err != nil {
			s.lg.Println(logging.Critical, "Failed to shutdown http server gracefully: "+err.Error())
		} else {
			s.httpServer = nil
		}
	}
}
