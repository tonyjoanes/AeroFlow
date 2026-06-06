// Package server provides the standard /health and /metrics endpoints every
// AeroFlow service exposes.
package server

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Mux returns an http.ServeMux with /health and /metrics registered, ready
// for the caller to add service-specific routes to.
func Mux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.Handle("/metrics", promhttp.Handler())

	return mux
}
