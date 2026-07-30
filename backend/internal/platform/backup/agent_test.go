package backup

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestAgentHandlerAndClientCoverLifecycleAndErrors(t *testing.T) {
	fixture := createFixture(t)
	options := AgentOptions{
		DestinationRoot: t.TempDir(),
		GromData:        fixture.gromData, SigningCerts: fixture.signing,
		RegistryData: fixture.registry, DistributionConfig: fixture.config,
	}
	handler := newAgentHandler(options)
	client := &AgentClient{client: handlerHTTPClient(handler)}
	ctx := context.Background()

	if summaries, err := client.List(ctx); err != nil || len(summaries) != 0 {
		t.Fatalf("list empty agent: summaries=%#v error=%v", summaries, err)
	}
	summary, err := client.Create(ctx, AgentCreateRequest{
		GromVersion: "test-version", DeploymentProfile: "development", Consistency: "quiesced",
	})
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}
	summaries, err := client.List(ctx)
	if err != nil || len(summaries) != 1 || summaries[0].BackupID != summary.BackupID {
		t.Fatalf("list created backup: summaries=%#v error=%v", summaries, err)
	}
	response, err := client.Download(ctx, summary.BackupID)
	if err != nil {
		t.Fatalf("download backup: %v", err)
	}
	raw, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || len(raw) == 0 ||
		response.Header.Get("Content-Disposition") == "" {
		t.Fatalf("unexpected downloaded bundle: bytes=%d header=%q error=%v", len(raw), response.Header.Get("Content-Disposition"), err)
	}
	if err := client.Delete(ctx, summary.BackupID); err != nil {
		t.Fatalf("delete backup: %v", err)
	}
	if err := client.Delete(ctx, summary.BackupID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing backup error, got %v", err)
	}
	if _, err := client.Download(ctx, summary.BackupID); err == nil {
		t.Fatal("expected missing download error")
	}

	for _, body := range []string{
		`not-json`,
		`{"gromVersion":"test","deploymentProfile":"development","consistency":"offline"}`,
		`{"gromVersion":"test","deploymentProfile":"development","consistency":"quiesced","extra":true}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/v1/backups", strings.NewReader(body))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("expected invalid create status %d, got %d for %s", http.StatusBadRequest, response.Code, body)
		}
	}

	broken := newAgentHandler(AgentOptions{
		DestinationRoot: "relative",
		GromData:        "missing", SigningCerts: "missing",
		RegistryData: "missing", DistributionConfig: "missing",
	})
	listFailure := httptest.NewRecorder()
	broken.ServeHTTP(listFailure, httptest.NewRequest(http.MethodGet, "/v1/backups", nil))
	if listFailure.Code != http.StatusInternalServerError {
		t.Fatalf("expected list failure status %d, got %d", http.StatusInternalServerError, listFailure.Code)
	}
	createFailure := httptest.NewRecorder()
	broken.ServeHTTP(
		createFailure,
		httptest.NewRequest(http.MethodPost, "/v1/backups", strings.NewReader(
			`{"gromVersion":"test","deploymentProfile":"development","consistency":"quiesced"}`,
		)),
	)
	if createFailure.Code != http.StatusInternalServerError {
		t.Fatalf("expected create failure status %d, got %d", http.StatusInternalServerError, createFailure.Code)
	}
}

func TestAgentClientMapsTransportStatusAndDecodeFailures(t *testing.T) {
	ctx := context.Background()
	transportFailure := &AgentClient{client: &http.Client{Transport: roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			return nil, errors.New("transport failed")
		},
	)}}
	if _, err := transportFailure.List(ctx); err == nil || !strings.Contains(err.Error(), "configured socket") {
		t.Fatalf("unexpected list transport error: %v", err)
	}
	if _, err := transportFailure.Download(ctx, "backup"); err == nil || !strings.Contains(err.Error(), "contact backup agent") {
		t.Fatalf("unexpected download transport error: %v", err)
	}
	if err := transportFailure.Delete(ctx, "backup"); err == nil || !strings.Contains(err.Error(), "contact backup agent") {
		t.Fatalf("unexpected delete transport error: %v", err)
	}

	statusFailure := &AgentClient{client: handlerHTTPClient(http.HandlerFunc(
		func(w http.ResponseWriter, request *http.Request) {
			if request.Method == http.MethodDelete {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			w.WriteHeader(http.StatusServiceUnavailable)
		},
	))}
	if _, err := statusFailure.List(ctx); err == nil || !strings.Contains(err.Error(), "status 503") {
		t.Fatalf("unexpected list status error: %v", err)
	}
	if _, err := statusFailure.Download(ctx, "backup"); err == nil || !strings.Contains(err.Error(), "status 503") {
		t.Fatalf("unexpected download status error: %v", err)
	}
	if err := statusFailure.Delete(ctx, "backup"); err == nil || !strings.Contains(err.Error(), "status 502") {
		t.Fatalf("unexpected delete status error: %v", err)
	}

	invalidJSON := &AgentClient{client: handlerHTTPClient(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "not-json")
		},
	))}
	if _, err := invalidJSON.List(ctx); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("unexpected decode error: %v", err)
	}
	if err := invalidJSON.doJSON(ctx, http.MethodPost, "/v1/backups", make(chan int), &struct{}{}); err == nil {
		t.Fatal("expected JSON marshal error")
	}

	missingSocket := NewAgentClient("/tmp/grom-agent-client-missing.sock")
	checkCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if missingSocket.Available(checkCtx) {
		t.Fatal("expected missing Unix socket to be unavailable")
	}
}

func TestAgentJSONHelpersSetExpectedResponse(t *testing.T) {
	response := httptest.NewRecorder()
	writeAgentJSON(response, http.StatusCreated, map[string]string{"status": "created"})
	if response.Code != http.StatusCreated ||
		response.Header().Get("Content-Type") != "application/json" ||
		!strings.Contains(response.Body.String(), `"status":"created"`) {
		t.Fatalf("unexpected JSON response %d %#v: %s", response.Code, response.Header(), response.Body.String())
	}

	errorResponse := httptest.NewRecorder()
	writeAgentError(errorResponse, http.StatusConflict)
	if errorResponse.Code != http.StatusConflict ||
		!strings.Contains(errorResponse.Body.String(), http.StatusText(http.StatusConflict)) {
		t.Fatalf("unexpected error response %d: %s", errorResponse.Code, errorResponse.Body.String())
	}
}

func handlerHTTPClient(handler http.Handler) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response.Result(), nil
	})}
}
