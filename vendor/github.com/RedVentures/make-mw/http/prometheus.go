package http

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus"
)

type prometheusResponseWriter struct {
	w      http.ResponseWriter
	status int
}

func (w prometheusResponseWriter) Header() http.Header {
	return w.w.Header()
}

func (w prometheusResponseWriter) WriteHeader(status int) {
	w.status = status
	w.w.WriteHeader(status)
}

func (w prometheusResponseWriter) Write(b []byte) (int, error) {
	return w.w.Write(b)
}

func (w prometheusResponseWriter) Flush() {
	w.w.(http.Flusher).Flush()
}

func (w prometheusResponseWriter) CloseNotify() <-chan bool {
	return w.w.(http.CloseNotifier).CloseNotify()
}

var httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "http_requests_total",
	Help: "Count of all HTTP requests",
}, []string{"method", "path", "status"})

var httpLatencies = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name: "http_request_duration_milliseconds",
	Buckets: []float64{1, 10, 50, 100, 200, 300, 500, 600, 700, 800, 900, 1000},
}, []string{"method", "path", "status"})

func WithPrometheus(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		pw := &prometheusResponseWriter{
			w:      w,
			status: http.StatusOK,
		}

		// Serve the request
		next.ServeHTTP(pw, r)

		httpRequestsTotal.With(prometheus.Labels{
			"method": r.Method,
			"path": r.URL.Path,
			"status": fmt.Sprintf("%d", pw.status),
		}).Inc()

		httpLatencies.With(prometheus.Labels{
			"method": r.Method,
			"path": r.URL.Path,
			"status": fmt.Sprintf("%d", pw.status),
		}).Observe(float64(time.Since(start).Nanoseconds()) / float64(time.Millisecond))
	})
}
