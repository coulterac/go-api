package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	newrelic "github.com/newrelic/go-agent"
)

func do(h handler, method, url string, header http.Header, body interface{}) (*httptest.ResponseRecorder, *http.Request) {
	nrConfig := newrelic.NewConfig("unit-test", "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	nr, err := newrelic.NewApplication(nrConfig)
	if err != nil {
		panic(err)
	}

	testRouter := newRouter(h, nr)

	b, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}

	r := httptest.NewRequest(method, url, bytes.NewBuffer(b))
	r.Header = header

	wr := httptest.NewRecorder()

	testRouter.ServeHTTP(wr, r)

	return wr, r
}
