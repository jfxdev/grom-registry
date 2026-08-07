package registrye2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	openapi "github.com/jfxdev/grom/backend/internal/generated/openapi"
)

const (
	slowUploadChunkDelay = 6 * time.Second
	slowUploadTimeout    = 45 * time.Second
)

func TestSlowStreamingUploadCompletes(t *testing.T) {
	if os.Getenv("GROM_RUN_REGISTRY_E2E") != "1" {
		t.Skip("set GROM_RUN_REGISTRY_E2E=1 or run make test-registry-e2e")
	}

	stack := startTestStack(t)
	admin := newManagementClient(t, stack.publicURL)
	admin.login(t)
	admin.createProject(t, "Slow upload", "slow-upload")
	writer := admin.createServicePrincipal(t, "Slow upload writer", "slow-upload-writer")
	admin.setMembership(t, "slow-upload", writer, openapi.Writer)

	const repository = "slow-upload/app"
	token := exchangeRegistryToken(t, admin, writer, repository)
	uploadURL := beginBlobUpload(t, stack.publicURL, repository, token)

	payload := []byte("first chunk\nsecond chunk\nthird chunk\nfourth chunk\n")
	uploadURL = streamSlowBlob(t, stack.publicURL, uploadURL, token, payload)

	// The registry validates a bearer token when each request starts. The upload
	// deliberately outlives the test stack's short registry JWT, so get a fresh
	// token before committing the completed stream.
	token = exchangeRegistryToken(t, admin, writer, repository)
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(payload))
	commitBlobUpload(t, uploadURL, token, digest)
	assertBlobContent(t, stack.publicURL, repository, token, digest, payload)
}

func exchangeRegistryToken(t *testing.T, admin *managementClient, principal servicePrincipal, repository string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), managementRequestTimeout)
	defer cancel()
	token, _, err := admin.exchangeToken(ctx, principal.username, principal.secret, repository)
	if err != nil {
		t.Fatalf("exchange registry token for %s: %v", repository, err)
	}
	return token.Token
}

func beginBlobUpload(t *testing.T, publicURL, repository, token string) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, publicURL+"/v2/"+repository+"/blobs/uploads/", nil)
	if err != nil {
		t.Fatalf("create blob upload request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response := doRegistryRequest(t, request)
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("start blob upload: status=%d", response.StatusCode)
	}
	return uploadLocation(t, publicURL, response)
}

func streamSlowBlob(t *testing.T, publicURL, uploadURL, token string, payload []byte) string {
	t.Helper()
	bodyReader, bodyWriter := io.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), slowUploadTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPatch, uploadURL, bodyReader)
	if err != nil {
		t.Fatalf("create slow blob upload request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/octet-stream")

	type uploadResult struct {
		response *http.Response
		err      error
	}
	resultCh := make(chan uploadResult, 1)
	go func() {
		response, err := (&http.Client{}).Do(request)
		resultCh <- uploadResult{response: response, err: err}
	}()

	chunkSize := len(payload) / 4
	for offset := 0; offset < len(payload); offset += chunkSize {
		end := min(offset+chunkSize, len(payload))
		if _, err := bodyWriter.Write(payload[offset:end]); err != nil {
			_ = bodyWriter.Close()
			t.Fatalf("write slow upload chunk: %v", err)
		}
		if end < len(payload) {
			time.Sleep(slowUploadChunkDelay)
		}
	}
	if err := bodyWriter.Close(); err != nil {
		t.Fatalf("finish slow upload stream: %v", err)
	}

	outcome := <-resultCh
	if outcome.err != nil {
		t.Fatalf("slow blob upload request: %v", outcome.err)
	}
	defer outcome.response.Body.Close()
	if outcome.response.StatusCode != http.StatusAccepted {
		t.Fatalf("slow blob upload: status=%d", outcome.response.StatusCode)
	}
	return uploadLocation(t, publicURL, outcome.response)
}

func commitBlobUpload(t *testing.T, uploadURL, token, digest string) {
	t.Helper()
	target, err := url.Parse(uploadURL)
	if err != nil {
		t.Fatalf("parse blob upload location: %v", err)
	}
	query := target.Query()
	query.Set("digest", digest)
	target.RawQuery = query.Encode()
	request, err := http.NewRequest(http.MethodPut, target.String(), nil)
	if err != nil {
		t.Fatalf("create blob commit request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response := doRegistryRequest(t, request)
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("commit slow blob upload: status=%d", response.StatusCode)
	}
}

func assertBlobContent(t *testing.T, publicURL, repository, token, digest string, expected []byte) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, publicURL+"/v2/"+repository+"/blobs/"+digest, nil)
	if err != nil {
		t.Fatalf("create blob read request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response := doRegistryRequest(t, request)
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("read slow uploaded blob: status=%d", response.StatusCode)
	}
	actual, err := io.ReadAll(io.LimitReader(response.Body, int64(len(expected)+1)))
	if err != nil {
		t.Fatalf("read slow uploaded blob: %v", err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("slow uploaded blob differs: got %q, want %q", actual, expected)
	}
}

func doRegistryRequest(t *testing.T, request *http.Request) *http.Response {
	t.Helper()
	response, err := (&http.Client{Timeout: slowUploadTimeout}).Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", request.Method, request.URL, err)
	}
	return response
}

func uploadLocation(t *testing.T, publicURL string, response *http.Response) string {
	t.Helper()
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil || response.Header.Get("Location") == "" {
		t.Fatalf("invalid blob upload location %q: %v", response.Header.Get("Location"), err)
	}
	baseURL, err := url.Parse(publicURL)
	if err != nil {
		t.Fatalf("parse public URL: %v", err)
	}
	return baseURL.ResolveReference(location).String()
}
