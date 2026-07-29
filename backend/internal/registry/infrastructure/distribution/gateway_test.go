package distribution

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"testing"
)

func TestGatewayPreservesPublicForwardingHeaders(t *testing.T) {
	target, err := url.Parse("http://distribution:5000")
	if err != nil {
		t.Fatal(err)
	}
	incoming := &http.Request{
		Method: http.MethodGet,
		Host:   "registry.grom.test",
		URL:    &url.URL{Path: "/v2/"},
		Header: http.Header{"X-Forwarded-Proto": []string{"https"}},
	}
	outgoing := incoming.Clone(incoming.Context())
	outgoing.Header = incoming.Header.Clone()

	rewriteProxyRequest(target)(&httputil.ProxyRequest{In: incoming, Out: outgoing})

	if outgoing.Host != "registry.grom.test" ||
		outgoing.Header.Get("X-Forwarded-Host") != "registry.grom.test" ||
		outgoing.Header.Get("X-Forwarded-Proto") != "https" ||
		outgoing.URL.Scheme != "http" ||
		outgoing.URL.Host != "distribution:5000" ||
		outgoing.URL.Path != "/v2/" {
		t.Fatalf("unexpected forwarded request: %#v", outgoing)
	}
}
