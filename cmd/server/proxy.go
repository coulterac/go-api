package main

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

func (h *handler) proxyHandler(w http.ResponseWriter, r *http.Request) {
	h.l.Log("level", "info", "msg", "received proxy request")

	url, err := url.Parse(h.optionProxyURL)
	if err != nil {
		h.l.Log("level", "error", "msg", "could not parse proxy url", "err", err.Error())
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	proxyReq, err := http.NewRequest(r.Method, url.String(), r.Body)
	if err != nil {
		h.l.Log("level", "error", "msg", "could not create new http request", "err", err.Error())
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	proxyReq.Header.Set("Host", r.Host)
	proxyReq.Header.Set("X-Forwarded-For", r.RemoteAddr)

	// Loop through our request headers and set them on the proxy request
	for header, values := range r.Header {
		for _, v := range values {
			proxyReq.Header.Add(header, v)
		}
	}

	client := http.Client{
		Timeout: time.Second * 5,
	}

	proxyResp, err := client.Do(proxyReq)
	if err != nil {
		h.l.Log("level", "error", "msg", "could do proxy request", "err", err.Error())
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if proxyResp.StatusCode < 200 || proxyResp.StatusCode >= 300 {
		h.l.Log("level", "info", "msg", "bad status code from proxy response", "status", proxyResp.StatusCode)
		sendError(w, proxyResp.StatusCode, fmt.Sprintf("bad status from proxy request got: %d", proxyResp.StatusCode))
		return
	}

	w.WriteHeader(proxyResp.StatusCode)
}
