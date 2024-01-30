package metrics

import (
	"runtime"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func ivmMetricsRunner() {
	// Create a new Counter metric for memory usage
	ivmostrMemoryUsage := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ivmostr_memory_usage_bytes",
			Help: "Current memory usage of the ivmostr application in bytes",
		},
		[]string{"type"},
	)

	// Create a new Gauge metric for CPU utilization
	ivmostrCpuUtilization := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ivmostr_cpu_utilization",
			Help: "Current CPU utilization of the ivmostr application in percentage",
		},
	)

	// Register the metrics with Prometheus
	prometheus.MustRegister(ivmostrMemoryUsage, ivmostrCpuUtilization)

	// Start monitoring memory usage and CPU utilization
	for {
		// Update memory usage metric
		stats := runtime.MemStats{}
		runtime.ReadMemStats(&stats)
		ivmostrMemoryUsage.WithLabelValues("heap").Add(float64(stats.HeapAlloc))

		// Update CPU utilization metric
		idle, total := runtime.GOMAXPROCS(0), runtime.NumGoroutine()
		busy := total - idle
		ivmostrCpuUtilization.Set(float64(busy) / float64(total) * 100)

		// Sleep for a second
		time.Sleep(time.Second)
	}
}
