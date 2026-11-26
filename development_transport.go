package main

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Keep the URL, Host header, and TLS server name intact for local virtual hosts.
// Only the connection destination changes, without depending on system DNS.
func configureLocalDevelopmentTransport(transport *http.Transport) {
	dial := transport.DialContext
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err == nil {
			host = strings.TrimSuffix(strings.ToLower(host), ".")
			if host == "localhost" || strings.HasSuffix(host, ".localhost") {
				address = net.JoinHostPort("127.0.0.1", port)
			}
		}
		return dial(ctx, network, address)
	}

	proxy := transport.Proxy
	transport.Proxy = func(request *http.Request) (*url.URL, error) {
		if isLocalDevelopmentHost(request.URL.Hostname()) || proxy == nil {
			return nil, nil
		}
		return proxy(request)
	}
}
