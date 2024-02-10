package metrics

import (
	"sync"
)

var (
	MetricsChan = make(chan interface{}, 1024)
	wg          sync.WaitGroup
)

func init() {
	wg.Add(1)

	go recordAppMetrics(MetricsChan)
	ivmMetricsRunner()

	wg.Wait()
}

func recordAppMetrics(metricsChan chan<- interface{}) {
	defer wg.Done()

	for metric := range MetricsChan {

		switch metric := metric.(type) {
		case map[string]int:
			for key, val := range metric {
				switch key {
				case "evntStored":
					evntStored.Inc()
				case "evntBroadcasted":
					evntBroadcasted.Add(float64(val))
				case "evntSubsSupplied":
					evntSubsSupplied.Add(float64(val))
				case "clntSubscriptions":
					clntSubscriptions.Add(float64(val))
				case "clntUpdatedSubscriptions":
					clntUpdatedSubscriptions.Add(float64(val))
				case "clntNrOfSubsFilters":
					clntNrOfSubsFilters.Add(float64(val))
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
				}
			}

		default:

		}
	}
}
