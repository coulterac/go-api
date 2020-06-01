package http

import (
	"net/http"

	newrelic "github.com/newrelic/go-agent"
)

func WithNewRelic(next http.Handler, app newrelic.Application) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tx := app.StartTransaction(r.URL.Path, w, r)
		defer tx.End()

		if r.RequestURI == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Add some attributes for things we can use to identify requests
		tx.AddAttribute("request.id", r.Context().Value(contextKeyRequestID))
		writeKey, _, ok := r.BasicAuth()
		if ok {
			tx.AddAttribute("writeKey", writeKey)
		}

		// Add the transaction to the context, and pass it on with the request
		r = newrelic.RequestWithTransactionContext(r, tx)

		next.ServeHTTP(w, r)
	})
}
