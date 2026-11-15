package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type accountTestTransport func(*http.Request) (*http.Response, error)

func (f accountTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLocalDevelopmentTLSConfigurationAndRouting(t *testing.T) {
	local := http.DefaultTransport.(*http.Transport).Clone()
	verified := http.DefaultTransport.(*http.Transport).Clone()
	local.ForceAttemptHTTP2 = false
	verified.ForceAttemptHTTP2 = false
	defer local.CloseIdleConnections()
	defer verified.CloseIdleConnections()
	var route string
	for name, transport := range map[string]*http.Transport{"local": local, "verified": verified} {
		transport.RegisterProtocol("https", accountTestTransport(func(r *http.Request) (*http.Response, error) {
			route = name
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}))
	}
	transport := &accountTransport{local: local, verified: verified}
	for _, test := range []struct {
		origin string
		local  bool
	}{
		{"https://localhost", true},
		{"https://OXEE.LOCALHOST.", true},
		{"https://oxee.localhost", true},
		{"https://shell-aw.oxee.localhost", true},
		{"https://127.0.0.1", true},
		{"https://[::1]", true},
		{"https://oxee.net", false},
		{"https://shell-aw.oxee.net", false},
		{"https://oxee.localhost.example.com", false},
		{"https://192.168.1.10", false},
	} {
		t.Run(test.origin, func(t *testing.T) {
			clientTransport := (&App{}).accountClient(test.origin).Transport.(*http.Transport)
			defer clientTransport.CloseIdleConnections()
			skipsVerification := clientTransport.TLSClientConfig != nil && clientTransport.TLSClientConfig.InsecureSkipVerify
			if skipsVerification != test.local {
				t.Fatal("development certificate policy does not match the server hostname")
			}
			request, err := http.NewRequest(http.MethodGet, test.origin, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := transport.RoundTrip(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			want := "verified"
			if test.local {
				want = "local"
			}
			if route != want {
				t.Fatal("authenticated request used the wrong certificate policy")
			}
		})
	}
}

func TestLoginRedirectReportsCanonicalPlatformAddress(t *testing.T) {
	a, err := NewApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a.accountURL = "https://localhost"
	a.accountHTTP = (&App{}).accountClient(a.accountURL)
	a.accountHTTP.Transport = accountTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/users/auth/oauth" {
			t.Fatal("unexpected login request")
		}
		return &http.Response{
			StatusCode: http.StatusPermanentRedirect, Status: "308 Permanent Redirect",
			Header: http.Header{"Location": {"https://oxee.localhost/api/users/auth/oauth"}},
			Body:   io.NopCloser(strings.NewReader("redirect")), Request: r,
		}, nil
	})
	err = a.BeginLogin()
	if err == nil || !strings.Contains(err.Error(), "308") || !strings.Contains(err.Error(), "SHELLANYWHERE_OXEE_URL to https://oxee.localhost") {
		t.Fatalf("redirect did not explain how to use the canonical platform address: %v", err)
	}
	if a.accountLogin != nil {
		t.Fatal("redirect left a pending login")
	}
}
