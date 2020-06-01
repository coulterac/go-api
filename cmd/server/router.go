package main

import (
	"net/http"

	mw "github.com/RedVentures/make-mw/http"
	"github.com/gorilla/mux"
	newrelic "github.com/newrelic/go-agent"
	"github.com/rs/cors"
)

func newRouter(h handler, nr newrelic.Application) http.Handler {
	router := mux.NewRouter()

	publicRouter := router.PathPrefix("").Subrouter()
	registerPublicRoutes(publicRouter, h)

	// Add some middleware

	out := cors.AllowAll().Handler(router)
	out = mw.WithNewRelic(out, nr)

	return router
}

func registerPublicRoutes(router *mux.Router, h handler) {
	router.HandleFunc("/health", healthHandler)
	router.HandleFunc("/v1/proxy", h.proxyHandler)
}
