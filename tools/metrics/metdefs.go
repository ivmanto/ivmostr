package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Defined application metrics to track
var (
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

	evntBroadcasted = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "ivmostr",
		Subsystem: "session",
		Name:      "ivmostr_events_broadcasted_total",
		Help:      "The total number of nostr clients*events broadcasted to the network",
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
		Help:      "The total number of updated subscriptions",
	})

	clntNrOfSubsFilters = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "ivmostr",
		Subsystem: "client",
		Name:      "ivmostr_subs_filters_active",
		Help:      "The number of active filters in subscriptions",
	})

	connsTopDemandingIP = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "ivmostr",
		Subsystem: "websocket",
		Name:      "ivmostr_top_demanding_ip",
		Help:      "The top demanding IP on number of connections",
	},
		[]string{
			"ip",
		})
)
