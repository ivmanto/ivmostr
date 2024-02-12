package main

import (
	"context"
	"flag"
	"os"
	_ "runtime/pprof"

	"github.com/dasiyes/ivmostr-tdd/configs/config"
	"github.com/dasiyes/ivmostr-tdd/internal/data/firestoredb"
	"github.com/dasiyes/ivmostr-tdd/internal/server"
	"github.com/dasiyes/ivmostr-tdd/internal/server/router"
	"github.com/dasiyes/ivmostr-tdd/internal/services"
	"github.com/dasiyes/ivmostr-tdd/pkg/fspool"
	"github.com/dasiyes/ivmostr-tdd/tools"
	log "github.com/sirupsen/logrus"
)

func main() {
	var (
		debug    = flag.Bool("debug", false, "debug mode")
		vers     = flag.Bool("version", false, "prints version")
		cfgfn    = flag.String("config", "configs/config.yaml", "--config=<file_name> configuration file name. Default is configs/config.yaml")
		newEvent = flag.String("newEvent", "", "prints new event on the console as configured in the tools create-event")
	)

	flag.Parse()
	ctx := context.Background()

	if *newEvent != "" {
		tools.PrintNewEvent()
		os.Exit(0)
	}

	// Request to print out the build version
	if *vers {
		tools.PrintVersion()
		os.Exit(0)
	}

	log.SetLevel(log.InfoLevel)

	// Check debug mode request
	if *debug {
		log.SetLevel(log.DebugLevel)
		*cfgfn = "../../configs/config_debug.yaml"
	}

	// Load the configuration file
	cfg, err := config.LoadConfig(*cfgfn)
	if err != nil {
		log.Printf("[main] CRITICAL:Error loading configuration file %s \nExit, unable to proceed", *cfgfn)
		panic(err)
	}

	// Initialize the websocket go-routine pool. The pool is used to handle the websocket connections
	// The pool's parameters workers and queue size can be set either in the configuration file or env vars.
	// Ensure default values
	if cfg.PoolMaxWorkers < 2 {
		cfg.PoolMaxWorkers = 128
	}

	if cfg.PoolQueue < 1 {
		cfg.PoolQueue = 1
	}

	// Initialize the nostr repository
	prj := cfg.GetProjectID()
	if prj == "" {
		log.Printf("[main] CRITICAL: Firestore project id %v is empty. Exiting now, unable to proceed.", prj)
		panic(nil)
	}

	fsClientsPool := fspool.NewConnectionPool(prj)

	if fsClientsPool == nil {
		log.Printf("[main] CRITICAL: firestore client pool initiation failed...\n Exiting now, unable to proceed.")
		panic(err)
	}

	// Initialize the firestore repository and configuration
	dlv := cfg.GetDLV()
	ecn := cfg.GetEventsCollectionName()

	nostrRepo, err := firestoredb.NewNostrRepository(&ctx, fsClientsPool, dlv, ecn)
	if err != nil {
		log.Errorf("[main] CRITICAL: firestore repository init error %s.\n Exiting now, unable to proceed.", err.Error())
		panic(err)
	}

	var wl, bl string
	if wl = cfg.Firestore.WhiteListCollectionName; wl == "" {
		wl = "whitelist"
	}

	if bl = cfg.Firestore.BlackListCollectionName; bl == "" {
		bl = "blacklist"
	}

	// Init the list repository
	lstsRepo, err := firestoredb.NewListRepository(&ctx, fsClientsPool, wl, bl)
	if err != nil {
		log.Errorf("[main] CRITICAL: firestore lists repository init error %s.\n Exiting now, unable to proceed.", err.Error())
		panic(err)
	}

	// Init a new HTTP server instance
	httpServer := server.NewInstance()
	hdlr := router.NewHandler(nostrRepo, lstsRepo, cfg)
	errs := make(chan error, 2)
	go func() {
		addr := ":" + cfg.Port
		log.Printf("...starting ivmostr (-tdd) instance at %s...", addr)
		errs <- httpServer.Start(addr, hdlr)
		services.Exit <- struct{}{}
	}()

	log.Printf("[main] !ivmostr http server terminated! %v", <-errs)
}
