package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ChStoreEvent         = make(chan int, 100)
	ChBroadcastEvent     = make(chan int, 50)
	ChNewSubscription    = make(chan int, 20)
	ChUpdateSubscription = make(chan int, 20)
	ChNrOfSubsFilters    = make(chan int, 20)
	ChTopDemandingIP     = make(chan map[string]int, 2)
)

// Defined application metrics to track
var (
	evntStored = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "ivmostr",
		Subsystem: "nostr",
		Name:      "ivmostr_total_stored_events",
		Help:      "The total number of nostr events stored in the DB",
	})

	evntBroadcasted = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "ivmostr",
		Subsystem: "nostr",
		Name:      "ivmostr_total_broadcasted_events",
		Help:      "The total number of nostr events broadcasted to the network",
	})

	evntNewSubscriptions = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "ivmostr",
		Subsystem: "nostr",
		Name:      "ivmostr_total_new_subscriptions",
		Help:      "The total number of new clients subscriptions",
	})

	evntUpdatedSubscriptions = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "ivmostr",
		Subsystem: "nostr",
		Name:      "ivmostr_total_updated_subscriptions",
		Help:      "The total number of updated subscriptions",
	})

	evntNrOfSubsFilters = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "ivmostr",
		Subsystem: "nostr",
		Name:      "ivmostr_total_subscription_filters",
		Help:      "The total number of filters in subscriptions",
	})

	connsTopDemandingIP = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "ivmostr",
		Subsystem: "nostr",
		Name:      "ivmostr_top_demanding_ip",
		Help:      "The top demanding IP on number of connections",
	},
		[]string{
			"ip",
		})
)

func init() {
	recordAppMetrics()
	ivmMetricsRunner()
}

func recordAppMetrics() {

	// Worker for tracking number of stored events
	go func() {
		for range ChStoreEvent {
			evntStored.Inc()
		}
	}()

	// Worker to track number of broadcasted events
	go func() {
		for range ChBroadcastEvent {
			evntBroadcasted.Inc()
		}
	}()

	// Worker to track number of new subscriptions
	go func() {
		for range ChNewSubscription {
			evntNewSubscriptions.Inc()
		}
	}()

	// Worker to track number of updated subscriptions
	go func() {
		for range ChUpdateSubscription {
			evntUpdatedSubscriptions.Inc()
		}
	}()

	// Worker to track number of subscriptions filters
	go func() {
		for v := range ChNrOfSubsFilters {
			evntNrOfSubsFilters.Add(float64(v))
		}
	}()

	// Worker to track on most demanding IP address keep connecting to the relay
	go func() {
		for tdip := range ChTopDemandingIP {

			for ip, v := range tdip {
				connsTopDemandingIP.WithLabelValues(ip).Set(float64(v))
				break
			}
		}
	}()
}
