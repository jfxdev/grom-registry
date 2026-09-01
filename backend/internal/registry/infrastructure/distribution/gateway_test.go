package distribution

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
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

func TestGatewayLogsManifestObservationFailureWithoutChangingResponse(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	modifyResponse := manifestObservationResponseModifier(nil, func(_ context.Context, repository, reference, digest, actor string) error {
		if repository != "project/repository" || reference != "sha256:request" || digest != "sha256:observed" || actor != "" {
			t.Fatalf("unexpected observation context: %q %q %q %q", repository, reference, digest, actor)
		}
		return errors.New("inventory unavailable")
	}, logger)
	request := httptest.NewRequest(
		http.MethodPut, "/v2/project/repository/manifests/sha256:request", nil,
	)
	response := &http.Response{
		StatusCode: http.StatusCreated,
		Header:     http.Header{"Docker-Content-Digest": []string{"sha256:observed"}},
		Request:    request,
	}

	if err := modifyResponse(response); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("expected completed push response, got %d", response.StatusCode)
	}
	for _, expected := range []string{
		"observe pushed manifest",
		"repository=project/repository",
		"reference=sha256:request",
		`error="inventory unavailable"`,
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("expected log to contain %q, got %q", expected, logs.String())
		}
	}
}
