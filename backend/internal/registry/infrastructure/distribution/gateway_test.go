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
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	registryapp "github.com/jfxdev/grom/backend/internal/registry/application"
	"github.com/jfxdev/grom/backend/internal/registry/infrastructure/signing"
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

type gatewayPullThrough struct {
	handled    bool
	err        error
	calls      int
	repository string
	reference  string
}

func (s *gatewayPullThrough) Ensure(_ context.Context, repository, reference string) (bool, error) {
	s.calls++
	s.repository = repository
	s.reference = reference
	return s.handled, s.err
}

func newGatewayTokenService(t *testing.T) *registryapp.TokenService {
	t.Helper()
	signer, err := signing.LoadOrCreate(filepath.Join(t.TempDir(), "key.pem"), filepath.Join(t.TempDir(), "cert.pem"))
	if err != nil {
		t.Fatal(err)
	}
	return registryapp.NewTokenService(nil, nil, signer, time.Minute)
}

func TestGatewaySynchronizesOnlyAuthorizedManifestReads(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			tokens := newGatewayTokenService(t)
			raw, err := tokens.IssueInternal("reader", []registryapp.Access{{
				Type: "repository", Name: "dockerhub/alpine", Actions: []string{constants.RegistryActionPull},
			}})
			if err != nil {
				t.Fatal(err)
			}
			synchronizer := &gatewayPullThrough{handled: true}
			nextCalls := 0
			gateway := &policyGateway{
				tokens: tokens, pullThrough: synchronizer,
				next: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					nextCalls++
					w.WriteHeader(http.StatusOK)
				}),
			}
			request := httptest.NewRequest(method, "/v2/dockerhub/alpine/manifests/latest", nil)
			request.Header.Set("Authorization", "Bearer "+raw)
			response := httptest.NewRecorder()

			gateway.ServeHTTP(response, request)

			if response.Code != http.StatusOK || nextCalls != 1 || synchronizer.calls != 1 ||
				synchronizer.repository != "dockerhub/alpine" || synchronizer.reference != "latest" {
				t.Fatalf("unexpected gateway flow: status=%d next=%d synchronize=%d repository=%q reference=%q",
					response.Code, nextCalls, synchronizer.calls, synchronizer.repository, synchronizer.reference)
			}
		})
	}
}

func TestGatewayDoesNotSynchronizeWithoutPullAccess(t *testing.T) {
	synchronizer := &gatewayPullThrough{handled: true}
	gateway := &policyGateway{
		tokens: newGatewayTokenService(t), pullThrough: synchronizer,
		next: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }),
	}
	response := httptest.NewRecorder()

	gateway.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v2/dockerhub/alpine/manifests/latest", nil))

	if response.Code != http.StatusUnauthorized || synchronizer.calls != 0 {
		t.Fatalf("unauthorized read reached pull-through: status=%d calls=%d", response.Code, synchronizer.calls)
	}
}

func TestGatewayKeepsNormalFlowForNonPullThroughProjects(t *testing.T) {
	synchronizer := &gatewayPullThrough{handled: false}
	gateway := &policyGateway{
		tokens: newGatewayTokenService(t), pullThrough: synchronizer,
		next: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }),
	}
	raw, err := gateway.tokens.IssueInternal("reader", []registryapp.Access{{
		Type: "repository", Name: "standard/app", Actions: []string{constants.RegistryActionPull},
	}})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v2/standard/app/manifests/latest", nil)
	request.Header.Set("Authorization", "Bearer "+raw)
	response := httptest.NewRecorder()

	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound || synchronizer.calls != 1 {
		t.Fatalf("standard project did not use normal flow: status=%d calls=%d", response.Code, synchronizer.calls)
	}
}

func TestGatewayMapsPullThroughFailures(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "missing", err: registryapp.ErrUpstreamManifestNotFound, status: http.StatusNotFound, code: "MANIFEST_UNKNOWN"},
		{name: "timeout", err: context.DeadlineExceeded, status: http.StatusGatewayTimeout, code: "UPSTREAM_TIMEOUT"},
		{name: "unavailable", err: errors.New("upstream unavailable"), status: http.StatusBadGateway, code: "UPSTREAM_UNAVAILABLE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tokens := newGatewayTokenService(t)
			raw, err := tokens.IssueInternal("reader", []registryapp.Access{{
				Type: "repository", Name: "ghcr/owner/image", Actions: []string{constants.RegistryActionPull},
			}})
			if err != nil {
				t.Fatal(err)
			}
			gateway := &policyGateway{
				tokens: tokens, pullThrough: &gatewayPullThrough{handled: true, err: test.err},
				next: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("failed synchronization reached Distribution") }),
			}
			request := httptest.NewRequest(http.MethodGet, "/v2/ghcr/owner/image/manifests/latest", nil)
			request.Header.Set("Authorization", "Bearer "+raw)
			response := httptest.NewRecorder()

			gateway.ServeHTTP(response, request)

			if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
				t.Fatalf("unexpected upstream failure response: status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
