package client

import (
	"net"
	"net/http"
	"time"
)

func NewHTTPClient(workers int) *http.Client {
	return &http.Client{
		Transport: NewHTTPTransport(workers),
	}
}

func NewHTTPTransport(workers int) *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        workers * 2,
		MaxIdleConnsPerHost: workers * 2,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		ForceAttemptHTTP2:   false,
	}
}
