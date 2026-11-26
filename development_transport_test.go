package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"
)

func TestLocalDevelopmentDialPreservesVirtualHost(t *testing.T) {
	for _, test := range []struct {
		address string
		want    string
	}{
		{"localhost:443", "127.0.0.1:443"},
		{"oxee.localhost:443", "127.0.0.1:443"},
		{"shell-aw.oxee.localhost:8443", "127.0.0.1:8443"},
		{"OXEE.LOCALHOST.:443", "127.0.0.1:443"},
		{"127.0.0.2:443", "127.0.0.2:443"},
		{"[::1]:443", "[::1]:443"},
		{"oxee.net:443", "oxee.net:443"},
		{"shell-aw.oxee.net:443", "shell-aw.oxee.net:443"},
		{"oxee.localhost.example.com:443", "oxee.localhost.example.com:443"},
	} {
		t.Run(test.address, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dialError := errors.New("test dial")
			transport := &http.Transport{DialContext: func(actual context.Context, network, address string) (net.Conn, error) {
				if actual != ctx || network != "tcp" || address != test.want {
					t.Fatalf("unexpected dial: %s %s, want tcp %s", network, address, test.want)
				}
				return nil, dialError
			}}
			configureLocalDevelopmentTransport(transport)
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+test.address+"/api/users/auth/oauth", nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = transport.DialContext(ctx, "tcp", request.URL.Host)
			if !errors.Is(err, dialError) {
				t.Fatalf("dial error was lost: %v", err)
			}
			if request.URL.Host != test.address || request.Host != test.address {
				t.Fatal("local routing changed the HTTP virtual host")
			}
		})
	}
}

func TestLocalDevelopmentBypassesProxyOnlyForLocalAddresses(t *testing.T) {
	proxyURL, err := url.Parse("http://proxy.example:8080")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		origin string
		local  bool
	}{
		{"https://oxee.localhost", true},
		{"https://shell-aw.oxee.localhost/api", true},
		{"https://LOCALHOST.", true},
		{"https://127.0.0.2", true},
		{"https://[::1]", true},
		{"https://oxee.net", false},
		{"https://shell-aw.oxee.net/api", false},
		{"https://oxee.localhost.example.com", false},
	} {
		t.Run(test.origin, func(t *testing.T) {
			called := false
			transport := &http.Transport{Proxy: func(request *http.Request) (*url.URL, error) {
				called = true
				if request.URL.String() != test.origin {
					t.Fatal("proxy received a modified URL")
				}
				return proxyURL, nil
			}}
			configureLocalDevelopmentTransport(transport)
			request, err := http.NewRequest(http.MethodGet, test.origin, nil)
			if err != nil {
				t.Fatal(err)
			}
			selected, err := transport.Proxy(request)
			if err != nil || called == test.local || (selected == nil) != test.local {
				t.Fatalf("incorrect proxy routing: selected=%v called=%v err=%v", selected, called, err)
			}
		})
	}
}
