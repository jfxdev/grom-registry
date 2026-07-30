package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoveryHandlerServesHardenedEscapedPage(t *testing.T) {
	options := RecoveryOptions{
		BackupRoot:        t.TempDir(),
		TargetGromVersion: `v1<script>alert("x")</script>`,
		MaxUploadBytes:    1 << 20,
	}
	response := httptest.NewRecorder()
	newRecoveryHandler(options, `token"</script>`).ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/", nil),
	)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if response.Header().Get("X-Frame-Options") != "DENY" ||
		!strings.Contains(response.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatalf("recovery page is missing security headers: %#v", response.Header())
	}
	body := response.Body.String()
	if strings.Contains(body, options.TargetGromVersion) || strings.Contains(body, `token"</script>`) {
		t.Fatalf("recovery page contains an unescaped injected value")
	}
	if !strings.Contains(body, "v1&lt;script&gt;") || !strings.Contains(body, `token\"\u003c/script\u003e`) {
		t.Fatalf("recovery page does not contain the expected escaped values: %s", body)
	}
}

func TestRecoveryHandlerUploadsListsAndRestoresBundle(t *testing.T) {
	fixture := createFixture(t)
	sourceRoot := t.TempDir()
	inspection, _, err := Create(CreateOptions{
		DestinationRoot:    sourceRoot,
		GromData:           fixture.gromData,
		SigningCerts:       fixture.signing,
		RegistryData:       fixture.registry,
		DistributionConfig: fixture.config,
		GromVersion:        "test-version",
		DeploymentProfile:  "development",
	})
	if err != nil {
		t.Fatal(err)
	}
	var bundle bytes.Buffer
	if _, err := Bundle(sourceRoot, inspection.Manifest.BackupID, &bundle); err != nil {
		t.Fatal(err)
	}

	targetRoot := t.TempDir()
	restoreRoot := t.TempDir()
	options := RecoveryOptions{
		BackupRoot:               targetRoot,
		GromDataTarget:           emptyDir(t, restoreRoot, "grom"),
		SigningCertsTarget:       emptyDir(t, restoreRoot, "signing"),
		RegistryDataTarget:       emptyDir(t, restoreRoot, "registry"),
		DistributionConfigTarget: filepath.Join(restoreRoot, "distribution", "config.yml"),
		TargetGromVersion:        "test-version",
		MaxUploadBytes:           1 << 20,
	}
	const token = "recovery-token"
	handler := newRecoveryHandler(options, token)

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/api/upload", bytes.NewReader(bundle.Bytes())))
	if unauthorized.Code != http.StatusForbidden {
		t.Fatalf("expected unauthorized upload status %d, got %d", http.StatusForbidden, unauthorized.Code)
	}

	corruptRequest := httptest.NewRequest(http.MethodPost, "/api/upload", strings.NewReader("not a tar"))
	corruptRequest.Header.Set("X-Grom-Recovery-Token", token)
	corrupt := httptest.NewRecorder()
	handler.ServeHTTP(corrupt, corruptRequest)
	if corrupt.Code != http.StatusBadRequest {
		t.Fatalf("expected corrupt upload status %d, got %d", http.StatusBadRequest, corrupt.Code)
	}

	uploadRequest := httptest.NewRequest(http.MethodPost, "/api/upload", bytes.NewReader(bundle.Bytes()))
	uploadRequest.Header.Set("X-Grom-Recovery-Token", token)
	uploaded := httptest.NewRecorder()
	handler.ServeHTTP(uploaded, uploadRequest)
	if uploaded.Code != http.StatusCreated || !strings.Contains(uploaded.Body.String(), inspection.Manifest.BackupID) {
		t.Fatalf("unexpected upload response %d: %s", uploaded.Code, uploaded.Body.String())
	}

	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, httptest.NewRequest(http.MethodGet, "/api/backups", nil))
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), inspection.Manifest.BackupID) {
		t.Fatalf("unexpected list response %d: %s", listed.Code, listed.Body.String())
	}

	for _, test := range []struct {
		name   string
		token  string
		body   string
		status int
	}{
		{name: "missing token", body: `{}`, status: http.StatusForbidden},
		{name: "invalid confirmation", token: token, body: `{"backupId":"` + inspection.Manifest.BackupID + `","confirmation":"NO"}`, status: http.StatusBadRequest},
		{name: "unknown backup", token: token, body: `{"backupId":"00000000-0000-0000-0000-000000000000","confirmation":"RESTORE"}`, status: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/restore", strings.NewReader(test.body))
			request.Header.Set("X-Grom-Recovery-Token", test.token)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("expected status %d, got %d: %s", test.status, response.Code, response.Body.String())
			}
		})
	}

	restoreRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/restore",
		strings.NewReader(`{"backupId":"`+inspection.Manifest.BackupID+`","confirmation":"RESTORE"}`),
	)
	restoreRequest.Header.Set("X-Grom-Recovery-Token", token)
	restored := httptest.NewRecorder()
	handler.ServeHTTP(restored, restoreRequest)
	if restored.Code != http.StatusOK {
		t.Fatalf("unexpected restore response %d: %s", restored.Code, restored.Body.String())
	}
	var result map[string]string
	if err := json.Unmarshal(restored.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["backupId"] != inspection.Manifest.BackupID || result["status"] != "restored" {
		t.Fatalf("unexpected restore result: %#v", result)
	}
	assertFileContent(t, filepath.Join(options.GromDataTarget, "grom.db"), "sqlite")
}

func TestRecoveryHandlerReportsListAndRestoreFailures(t *testing.T) {
	listResponse := httptest.NewRecorder()
	newRecoveryHandler(RecoveryOptions{
		BackupRoot: "relative", MaxUploadBytes: 1024,
	}, "token").ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/backups", nil))
	if listResponse.Code != http.StatusInternalServerError {
		t.Fatalf("expected list failure status %d, got %d", http.StatusInternalServerError, listResponse.Code)
	}

	fixture := createFixture(t)
	root := t.TempDir()
	inspection, _, err := Create(CreateOptions{
		DestinationRoot: root, GromData: fixture.gromData, SigningCerts: fixture.signing,
		RegistryData: fixture.registry, DistributionConfig: fixture.config,
		GromVersion: "source-version", DeploymentProfile: "development",
	})
	if err != nil {
		t.Fatal(err)
	}
	targetRoot := t.TempDir()
	handler := newRecoveryHandler(RecoveryOptions{
		BackupRoot: root, GromDataTarget: emptyDir(t, targetRoot, "grom"),
		SigningCertsTarget:       emptyDir(t, targetRoot, "signing"),
		RegistryDataTarget:       emptyDir(t, targetRoot, "registry"),
		DistributionConfigTarget: filepath.Join(targetRoot, "config.yml"),
		TargetGromVersion:        "different-version", MaxUploadBytes: 1 << 20,
	}, "token")
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/restore",
		strings.NewReader(`{"backupId":"`+inspection.Manifest.BackupID+`","confirmation":"RESTORE"}`),
	)
	request.Header.Set("X-Grom-Recovery-Token", "token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("expected restore refusal status %d, got %d: %s", http.StatusConflict, response.Code, response.Body.String())
	}
}

func TestServeRecoveryValidatesLimitAndShutsDown(t *testing.T) {
	if err := ServeRecovery(context.Background(), RecoveryOptions{}); err == nil {
		t.Fatal("expected invalid upload limit rejection")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ServeRecovery(ctx, RecoveryOptions{
		ListenAddress:  "127.0.0.1:0",
		MaxUploadBytes: 1024,
	}); err != nil {
		t.Fatalf("shutdown canceled recovery server: %v", err)
	}
}
