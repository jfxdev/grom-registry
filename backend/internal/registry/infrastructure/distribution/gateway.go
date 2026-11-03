package distribution

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/jfxdev/grom/backend/internal/constants"
	registryapp "github.com/jfxdev/grom/backend/internal/registry/application"
)

type PullThroughSynchronizer interface {
	Ensure(ctx context.Context, repository, reference string) (bool, error)
}

type policyGateway struct {
	next        http.Handler
	tokens      *registryapp.TokenService
	client      *Client
	pullThrough PullThroughSynchronizer
	logger      *slog.Logger
}

type ManifestObserver func(ctx context.Context, repository, reference, digest, actor string) error

func NewGateway(
	target *url.URL,
	tokens *registryapp.TokenService,
	client *Client,
	pullThrough PullThroughSynchronizer,
	observer ManifestObserver,
	logger *slog.Logger,
) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	proxy := &httputil.ReverseProxy{Rewrite: rewriteProxyRequest(target)}
	proxy.ModifyResponse = manifestObservationResponseModifier(tokens, observer, logger)
	proxy.FlushInterval = -1
	return &policyGateway{next: proxy, tokens: tokens, client: client, pullThrough: pullThrough, logger: logger}
}

func manifestObservationResponseModifier(
	tokens *registryapp.TokenService,
	observer ManifestObserver,
	logger *slog.Logger,
) func(*http.Response) error {
	return func(response *http.Response) error {
		if observer == nil || response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil
		}
		repository, reference, isManifestPut := manifestPutTarget(response.Request)
		if !isManifestPut {
			return nil
		}
		actor := ""
		if rawToken, ok := strings.CutPrefix(response.Request.Header.Get("Authorization"), "Bearer "); ok {
			actor, _ = tokens.Subject(rawToken)
		}
		if err := observer(
			response.Request.Context(), repository, reference, response.Header.Get("Docker-Content-Digest"), actor,
		); err != nil {
			logger.ErrorContext(response.Request.Context(), "observe pushed manifest",
				"repository", repository, "reference", reference, "error", err)
		}
		return nil
	}
}

func rewriteProxyRequest(target *url.URL) func(*httputil.ProxyRequest) {
	return func(request *httputil.ProxyRequest) {
		host := request.In.Host
		proto := request.In.Header.Get("X-Forwarded-Proto")
		if proto == "" {
			proto = "http"
			if request.In.TLS != nil {
				proto = "https"
			}
		}
		request.SetURL(target)
		request.Out.Host = host
		request.Out.Header.Set("X-Forwarded-Host", host)
		request.Out.Header.Set("X-Forwarded-Proto", proto)
	}
}

func (g *policyGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if g.pullThrough != nil {
		repository, reference, isManifestRead := manifestReadTarget(r)
		if isManifestRead {
			rawToken, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if ok && g.tokens.HasAccess(rawToken, repository, constants.RegistryActionPull) {
				handled, err := g.pullThrough.Ensure(r.Context(), repository, reference)
				if handled {
					if err != nil {
						if g.logger != nil {
							g.logger.ErrorContext(r.Context(), "synchronize pull-through manifest", "repository", repository, "reference", reference, "error", err)
						}
						status, code, message := http.StatusBadGateway, "UPSTREAM_UNAVAILABLE", "Upstream registry is unavailable"
						switch {
						case errors.Is(err, registryapp.ErrUpstreamManifestNotFound):
							status, code, message = http.StatusNotFound, "MANIFEST_UNKNOWN", "Upstream manifest not found"
						case errors.Is(err, context.DeadlineExceeded):
							status, code, message = http.StatusGatewayTimeout, "UPSTREAM_TIMEOUT", "Upstream synchronization timed out"
						}
						writeGatewayError(w, status, code, message)
						return
					}
					g.next.ServeHTTP(w, r)
					return
				}
			}
		}
	}
	repository, reference, isManifestPut := manifestPutTarget(r)
	if !isManifestPut || strings.Contains(reference, ":") {
		g.next.ServeHTTP(w, r)
		return
	}
	rawToken, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || !g.tokens.HasAccess(rawToken, repository, constants.RegistryActionPush) {
		g.next.ServeHTTP(w, r)
		return
	}
	exists, err := g.client.ManifestExists(r.Context(), repository, reference)
	if err != nil {
		writeGatewayError(w, http.StatusBadGateway, "UNKNOWN", "Registry metadata is unavailable")
		return
	}
	if err := g.tokens.EvaluateManifestPut(r.Context(), repository, reference, exists); err != nil {
		writeGatewayError(w, http.StatusConflict, "DENIED", err.Error())
		return
	}
	g.next.ServeHTTP(w, r)
}

func manifestReadTarget(r *http.Request) (string, string, bool) {
	if (r.Method != http.MethodGet && r.Method != http.MethodHead) || !strings.HasPrefix(r.URL.Path, "/v2/") {
		return "", "", false
	}
	target := strings.TrimPrefix(r.URL.Path, "/v2/")
	repository, reference, ok := strings.Cut(target, "/manifests/")
	if !ok || repository == "" || reference == "" || strings.Contains(reference, "/") {
		return "", "", false
	}
	return repository, reference, true
}

func manifestPutTarget(r *http.Request) (string, string, bool) {
	if r.Method != http.MethodPut || !strings.HasPrefix(r.URL.Path, "/v2/") {
		return "", "", false
	}
	target := strings.TrimPrefix(r.URL.Path, "/v2/")
	repository, reference, ok := strings.Cut(target, "/manifests/")
	if !ok || repository == "" || reference == "" || strings.Contains(reference, "/") {
		return "", "", false
	}
	return repository, reference, true
}

func writeGatewayError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"errors": []map[string]string{{"code": code, "message": message}},
	})
}
