package main

import (
	"context"
	"flag"
	"os"

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
		workers  = flag.Int("workers", 0, "max workers count")
		queue    = flag.Int("queue", 0, "workers task queue size")
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

	// initializing the web application as a handler
	var (
		// db *sql.DB     = &sql.DB{}
		pool_max_workers int = 128
		pool_queue       int = 1
	)

	// Load the configuration file
	cfg, err := config.LoadConfig(*cfgfn)
	if err != nil {
		log.Printf("[main] CRITICAL:Error loading configuration file %s \nExit, unable to proceed", *cfgfn)
		panic(err)
	}

	// Initialize the websocket go-routine pool. The pool is used to handle the websocket connections
	// The pool's parameters workers and queue size can be set either in the configuration file or as runtime flags.
	// The priority is from the most flexible to the most rigid method: ENV_VAR (config file) > runtime flag > default value
	if cfg.PoolMaxWorkers >= 1 {
		pool_max_workers = cfg.PoolMaxWorkers
	} else {
		if *workers > 0 {
			pool_max_workers = *workers
		}
		// if not set in config file, setting it up for cfg object
		cfg.PoolMaxWorkers = pool_max_workers
	}

	if cfg.PoolQueue > 0 {
		pool_queue = cfg.PoolQueue
	} else {
		if *queue > 0 {
			pool_queue = *queue
		}
		// if not set in config file, setting it up for cfg object
		cfg.PoolQueue = pool_queue
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
		log.Printf("[main] CRITICAL: firestore repository init error %s.\n Exiting now, unable to proceed.", err.Error())
		panic(err)
	}

	// Init a new HTTP server instance
	httpServer := server.NewInstance()
	hdlr := router.NewHandler(nostrRepo, cfg)
	errs := make(chan error, 2)
	go func() {
		addr := ":" + cfg.Port
		log.Printf("...starting ivmostr (-tdd) instance at %s...", addr)
		errs <- httpServer.Start(addr, hdlr)
		services.Exit <- struct{}{}
	}()

	log.Printf("[main] !ivmostr http server terminated! %v", <-errs)
}
