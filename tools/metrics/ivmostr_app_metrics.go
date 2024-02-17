package metrics

import (
	"context"
	"sync"

	log "github.com/sirupsen/logrus"
)

var (
	MetricsChan = make(chan interface{}, 1024)
	updateMutex sync.RWMutex
)

func init() {
	ctx := context.Background()
	go recordAppMetrics(MetricsChan, ctx)
	ivmMetricsRunner()
}

func recordAppMetrics(metricsChan chan<- interface{}, ctx context.Context) {
	lgr := log.New()
	lgr.SetLevel(log.DebugLevel)
	lgr.SetFormatter(&log.JSONFormatter{})
	updateMutex.Lock()
	defer updateMutex.Unlock()

	for {
		select {
		case metrics := <-MetricsChan:
			switch metric := metrics.(type) {
			case map[string]int:
				for key, val := range metric {
					switch key {
					case "evntStored":
						evntStored.Inc()
					case "evntProcessedBrdcst":
						evntProcessedBrdcst.Inc()
					case "evntBroadcasted":
						evntBroadcasted.Inc()
					case "evntSubsSupplied":
						evntSubsSupplied.Add(float64(val))
					case "clntSubscriptions":
						clntSubscriptions.Add(float64(val))
					case "clntUpdatedSubscriptions":
						clntUpdatedSubscriptions.Add(float64(val))
					case "clntNrOfSubsFilters":
						clntNrOfSubsFilters.Add(float64(val))
					case "clntProcessedMessages":
						clntProcessedMessages.Inc()
					case "connsActiveWSConns":
						connsActiveWSConns.Add(float64(val))
					case "clntAWSCUniqueIP":
						clntAWSCUniqueIP.Set(float64(val))
					default:
						continue
					}
				}
			case map[string]uint64:
				for key, val := range metric {
					switch key {
					case "netOutboundBytes":
						netOutboundBytes.Add(float64(val))
					}
				}
			case map[string]interface{}:
				for key, val := range metric {
					switch key {
					case "connsTopDemandingIP":
						ipMax, ok := val.(map[string]int)
						if ok {
							for k, v := range ipMax {
								connsTopDemandingIP.WithLabelValues(k).Set(float64(v))
							}
						}
					case "connsTotalHTTPRequests":
						nrConns, ok := val.(map[string]int)
						if ok {
							connsTotalHTTPRequests.WithLabelValues("http").Add(float64(nrConns["http"]))
						}
					}
				}

			default:
				continue
			}
		case <-ctx.Done():
			return
		}
	}
}
