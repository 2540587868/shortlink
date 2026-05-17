package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "shortlink_http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "shortlink_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	ShortLinksCreated = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "shortlink_links_created_total",
			Help: "Total number of short links created.",
		},
	)

	RedirectsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "shortlink_redirects_total",
			Help: "Total number of redirects served.",
		},
	)

	AnalyticsEventsDropped = promauto.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "shortlink_analytics_events_dropped",
			Help: "Number of analytics events dropped due to full buffer.",
		},
		func() float64 { return analyticsDropped() },
	)

	AnalyticsEventsInflight = promauto.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "shortlink_analytics_events_inflight",
			Help: "Number of analytics events currently in-flight.",
		},
		func() float64 { return analyticsInflight() },
	)
)

var (
	droppedFn  func() float64
	inflightFn func() float64
)

func analyticsDropped() float64 {
	if droppedFn != nil {
		return droppedFn()
	}
	return 0
}

func analyticsInflight() float64 {
	if inflightFn != nil {
		return inflightFn()
	}
	return 0
}

func SetAnalyticsGauges(dropped, inflight func() float64) {
	droppedFn = dropped
	inflightFn = inflight
}
