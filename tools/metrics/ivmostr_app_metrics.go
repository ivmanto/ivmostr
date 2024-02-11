package metrics

var (
	MetricsChan = make(chan interface{}, 1024)
)

func init() {
	go recordAppMetrics(MetricsChan)
	ivmMetricsRunner()
}

func recordAppMetrics(metricsChan chan<- interface{}) {

	for metric := range MetricsChan {

		switch metric := metric.(type) {
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
				default:
					continue
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
			continue
		}
		continue
	}
}
