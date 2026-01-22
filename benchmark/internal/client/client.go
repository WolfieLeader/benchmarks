package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type Client struct {
	serverUrl  *url.URL
	httpClient http.Client
	ctx        context.Context
}

func newClient(ctx context.Context, serverUrl string) *Client {
	base, err := url.Parse(serverUrl)
	if err != nil {
		panic(fmt.Sprintf("invalid server URL: %v", err))
	}
	return &Client{serverUrl: base, httpClient: http.Client{}, ctx: ctx}
}
