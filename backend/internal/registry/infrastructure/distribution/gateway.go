package distribution

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/jfxdev/grom/backend/internal/constants"
	registryapp "github.com/jfxdev/grom/backend/internal/registry/application"
)

type policyGateway struct {
	next   http.Handler
	tokens *registryapp.TokenService
	client *Client
}

type ManifestObserver func(context.Context, string, string, string) error

func NewGateway(
	target *url.URL,
	tokens *registryapp.TokenService,
	client *Client,
	observer ManifestObserver,
	logger *slog.Logger,
) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	proxy := &httputil.ReverseProxy{Rewrite: rewriteProxyRequest(target)}
	proxy.ModifyResponse = manifestObservationResponseModifier(observer, logger)
	proxy.FlushInterval = -1
	return &policyGateway{next: proxy, tokens: tokens, client: client}
}

func manifestObservationResponseModifier(
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
		if err := observer(
			response.Request.Context(), repository, reference, response.Header.Get("Docker-Content-Digest"),
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
