package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Defined application metrics to track
var (

	// Subsystem: client
	evntStored = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "ivmostr",
		Subsystem: "client",
		Name:      "ivmostr_events_stored_total",
		Help:      "The total number of nostr events stored in the DB (server lt current session)",
	})

	evntSubsSupplied = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "ivmostr",
		Subsystem: "client",
		Name:      "ivmostr_events_supplied_total",
		Help:      "The total number of nostr events supplied to the clients (server lt current session)",
	})

	clntSubscriptions = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "ivmostr",
		Subsystem: "client",
		Name:      "ivmostr_clients_subscriptions_active",
		Help:      "The number of active clients subscriptions",
	})

	clntUpdatedSubscriptions = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "ivmostr",
		Subsystem: "client",
		Name:      "ivmostr_subscriptions_updated_total",
		Help:      "The number of updated clients subscriptions",
	})

	clntNrOfSubsFilters = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "ivmostr",
		Subsystem: "client",
		Name:      "ivmostr_subs_filters_active",
		Help:      "The number of active filters in subscriptions",
	})

	clntProcessedMessages = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "ivmostr",
		Subsystem: "client",
		Name:      "ivmostr_clients_processed_messages_total",
		Help:      "The number of messages processed by the dispatcher",
	})

	// Subsystem: session
	evntProcessedBrdcst = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "ivmostr",
		Subsystem: "session",
		Name:      "ivmostr_events_processed_broadcast_total",
		Help:      "The total number of nostr events processed by NewEventBroadcaster method",
	})

	evntBroadcasted = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "ivmostr",
		Subsystem: "session",
		Name:      "ivmostr_events_broadcasted_total",
		Help:      "The total number of nostr clients*events broadcasted to the network",
	})

	// Subsystem: connections
	connsTopDemandingIP = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "ivmostr",
		Subsystem: "connections",
		Name:      "ivmostr_top_demanding_ip",
		Help:      "The top demanding IP on number of connections",
	},
		[]string{
			"ip",
		})

	connsTotalHTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "ivmostr",
		Subsystem: "connections",
		Name:      "ivmostr_http_requests_total",
		Help:      "The total number of http requests from the network (server lt current session)",
	},
		[]string{
			"http",
		})

	connsActiveWSConns = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "ivmostr",
		Subsystem: "connections",
		Name:      "ivmostr_ws_connections_active",
		Help:      "The number of active websocket connections",
	})

	// Subsystem: network
	netOutboundBytes = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "ivmostr",
		Subsystem: "network",
		Name:      "ivmostr_network_outbound_bytes",
		Help:      "The number of active websocket connections",
	})
)
