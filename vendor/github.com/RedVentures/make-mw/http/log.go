package http

import (
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
)

type logResponseWriter struct {
	w      http.ResponseWriter
	status int
}

func (w logResponseWriter) Header() http.Header {
	return w.w.Header()
}

func (w logResponseWriter) WriteHeader(status int) {
	w.status = status
	w.w.WriteHeader(status)
}

func (w logResponseWriter) Write(b []byte) (int, error) {
	return w.w.Write(b)
}

func (w logResponseWriter) Flush() {
	w.w.(http.Flusher).Flush()
}

func (w logResponseWriter) CloseNotify() <-chan bool {
	return w.w.(http.CloseNotifier).CloseNotify()
}

func WithLog(next http.Handler, l log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := logResponseWriter{
			w:      w,
			status: http.StatusOK,
		}
		next.ServeHTTP(&lw, r)
		dur := time.Since(start)

		l.Log(
			"level", "info",
			"msg", "incoming request",
			"requestId", r.Context().Value(contextKeyRequestID),
			"method", r.Method,
			"uri", r.RequestURI,
			"status", lw.status,
			"dur", dur,
		)
	})
}
