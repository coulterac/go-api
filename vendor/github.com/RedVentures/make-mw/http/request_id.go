package http

import (
	"context"
	"fmt"
	"net/http"

	"github.com/segmentio/ksuid"
)

func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := fmt.Sprintf("req_%s", ksuid.New().String())

		w.Header().Set("Request-ID", requestID)
		ctx := context.WithValue(r.Context(), contextKeyRequestID, requestID)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}
