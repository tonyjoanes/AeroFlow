// Package metrics provides shared Prometheus instrumentation for every
// AeroFlow service: event counters, processing latency histograms, and an
// instrumented HTTP mux.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ServiceMetrics holds the Prometheus instruments for a single service.
// Create one at startup with New and pass it to your handlers.
type ServiceMetrics struct {
	eventsPublished   *prometheus.CounterVec
	eventsConsumed    *prometheus.CounterVec
	processingSeconds *prometheus.HistogramVec
	httpRequests      *prometheus.CounterVec
	httpSeconds       *prometheus.HistogramVec
}

// New registers and returns ServiceMetrics for the named service. Calling
// New twice with the same service name is safe — promauto reuses existing
// registrations.
func New(service string) *ServiceMetrics {
	labels := prometheus.Labels{"service": service}

	return &ServiceMetrics{
		eventsPublished: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "aeroflow_events_published_total",
			Help:        "Total events published to NATS.",
			ConstLabels: labels,
		}, []string{"subject"}),

		eventsConsumed: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "aeroflow_events_consumed_total",
			Help:        "Total events consumed from NATS.",
			ConstLabels: labels,
		}, []string{"subject", "result"}),

		processingSeconds: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "aeroflow_event_processing_duration_seconds",
			Help:        "Time spent processing a consumed event.",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}, []string{"subject"}),

		httpRequests: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "aeroflow_http_requests_total",
			Help:        "Total HTTP requests handled.",
			ConstLabels: labels,
		}, []string{"method", "path", "status"}),

		httpSeconds: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "aeroflow_http_request_duration_seconds",
			Help:        "HTTP request latency.",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}, []string{"method", "path"}),
	}
}

// RecordPublish increments the published counter for subject.
func (m *ServiceMetrics) RecordPublish(subject string) {
	m.eventsPublished.WithLabelValues(subject).Inc()
}

// ObserveConsume returns a done func. Call it when the handler returns,
// passing the error so result=ok|error is labelled correctly.
func (m *ServiceMetrics) ObserveConsume(subject string) func(err error) {
	start := time.Now()
	return func(err error) {
		result := "ok"
		if err != nil {
			result = "error"
		}
		m.eventsConsumed.WithLabelValues(subject, result).Inc()
		m.processingSeconds.WithLabelValues(subject).Observe(time.Since(start).Seconds())
	}
}

// InstrumentMux wraps each handler on mux to record HTTP metrics, returning
// the same mux so callers can chain it.
func (m *ServiceMetrics) InstrumentMux(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		mux.ServeHTTP(rw, r)
		m.httpRequests.WithLabelValues(r.Method, r.URL.Path, strconv.Itoa(rw.status)).Inc()
		m.httpSeconds.WithLabelValues(r.Method, r.URL.Path).Observe(time.Since(start).Seconds())
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
