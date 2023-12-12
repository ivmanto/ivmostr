package main

import (
	"context"
	"flag"
	"log"
	"os"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/logging"
	"github.com/dasiyes/ivmostr-tdd/configs/config"
	"github.com/dasiyes/ivmostr-tdd/internal/data/firestoredb"
	"github.com/dasiyes/ivmostr-tdd/internal/server"
	"github.com/dasiyes/ivmostr-tdd/internal/server/router"
	"github.com/dasiyes/ivmostr-tdd/tools"
)

func main() {
	var (
		// addr      = flag.String("listen", ":3333", "address to bind to")
		// debug     = flag.String("pprof", "", "address for pprof http")
		// workers = flag.Int("workers", 0, "max workers count")
		// queue   = flag.Int("queue", 0, "workers task queue size")
		// ioTimeout = flag.Duration("io_timeout", time.Millisecond*100, "i/o operations timeout")
		cfgfn    = flag.String("config", "../../configs/config_debug.yaml", "--config=<file_name> configuration file name. Default is configs/config.yaml")
		newEvent = flag.String("newEvent", "", "prints new event on the console as configured in the tools create-event")
	)

	flag.Parse()
	ctx := context.Background()

	if *newEvent != "" {
		tools.PrintNewEvent()
		os.Exit(0)
	}

	// initializing the web application as a handler
	var (
		// db *sql.DB     = &sql.DB{}
		ml *log.Logger = log.New(os.Stderr, "[main] ", log.LstdFlags)
		l  *log.Logger = log.New(os.Stderr, "[http-srv] ", log.LstdFlags)
		// pool_max_workers int
		// pool_queue       int
		clgr *logging.Logger = nil
	)

	// Load the configuration file
	cfg, err := config.LoadConfig(*cfgfn)
	if err != nil {
		ml.Printf("Error loading configuration file %s \nExit, unable to proceed", *cfgfn)
		panic(err)
	}

	// Initialize the websocket go-routine pool. The pool is used to handle the websocket connections
	// The pool's parameters workers and queue size can be set either in the configuration file or as runtime flags.
	// The priority is from the most flexible to the most rigid method: ENV_VAR (config file) > runtime flag > default value
	// if cfg.PoolMaxWorkers >= 1 {
	// 	pool_max_workers = cfg.PoolMaxWorkers
	// } else {
	// 	if *workers < 1 {
	// 		pool_max_workers = 128
	// 	} else {
	// 		pool_max_workers = *workers
	// 	}
	// }

	// if cfg.PoolQueue >= 1 {
	// 	pool_queue = cfg.PoolQueue
	// } else {
	// 	if *queue < 1 {
	// 		pool_queue = 1
	// 	} else {
	// 		pool_queue = *queue
	// 	}
	// }

	// Initialize the nostr repository
	prj := cfg.GetProjectID()
	if prj == "" {
		ml.Printf("Firestore project id %v is empty. Exit: unable to proceed.", prj)
		panic(nil)
	}
	client, err := firestore.NewClient(ctx, prj)
	if err != nil {
		ml.Printf("firestore client init error %s.\n Exit: unable to proceed.", err.Error())
		panic(err)
	}
	defer client.Close()

	// Initialize the firestore repository and configuration
	dlv := cfg.GetDLV()
	ecn := cfg.GetEventsCollectionName()

	nostrRepo, err := firestoredb.NewNostrRepository(&ctx, client, dlv, ecn)
	if err != nil {
		ml.Printf("firestore repository init error %s.\n Exit: unable to proceed.", err.Error())
		panic(err)
	}
	listRepo, err := firestoredb.NewListRepository(
		&ctx, client, cfg.GetWhiteListCollectionName(), cfg.GetBlackListCollectionName())
	if err != nil {
		ml.Printf("firestore repository init error %s.\n Exit: unable to proceed.", err.Error())
		panic(err)
	}

	// Initialize the cloud Logging client
	if cfg.CloudLoggingEnabled {
		clientLgr, err := logging.NewClient(ctx, prj)
		if err != nil {
			ml.Printf("Error while initializing cloud logging. The service will be now disbled!")
			cfg.CloudLoggingEnabled = false
		} else {

			clgr = clientLgr.Logger("ivmostr-cnn")
		}
	}

	// Init a new HTTP server instance
	httpServer := server.NewInstance()
	hdlr := router.NewHandler(l, clgr, nostrRepo, listRepo)
	errs := make(chan error, 2)
	go func() {
		addr := ":" + cfg.Port
		ml.Printf("...starting ivmostr instance at %s...", addr)
		errs <- httpServer.Start(addr, hdlr)
	}()

	ml.Printf("ivmostr http server terminated! %v", <-errs)
}
