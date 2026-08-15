package registrye2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	openapi "github.com/jfxdev/grom/backend/internal/generated/openapi"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

const managementRequestTimeout = 15 * time.Second

type managementClient struct {
	baseURL string
	client  *http.Client
}

type servicePrincipal struct {
	id       string
	username string
	tokenID  string
	secret   string
}

func newManagementClient(t *testing.T, baseURL string) *managementClient {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create management API cookie jar: %v", err)
	}
	return &managementClient{
		baseURL: baseURL,
		client:  &http.Client{Jar: jar, Timeout: managementRequestTimeout},
	}
}

func (c *managementClient) login(t *testing.T) {
	t.Helper()
	user := c.loginAs(t, e2eAdminEmail, e2eAdminPassword, http.StatusOK)
	if !user.SystemAdmin {
		t.Fatal("bootstrap session is not an installation administrator")
	}
}

func (c *managementClient) loginAs(t *testing.T, email, password string, expectedStatus int) openapi.User {
	t.Helper()
	var user openapi.User
	var output any
	if expectedStatus == http.StatusOK {
		output = &user
	}
	c.doJSON(t, http.MethodPost, "/api/v1/session", openapi.LoginRequest{
		Email: openapi_types.Email(email), Password: password,
	}, output, expectedStatus)
	return user
}

func (c *managementClient) currentUser(t *testing.T, expectedStatus int) openapi.User {
	t.Helper()
	var user openapi.User
	var output any
	if expectedStatus == http.StatusOK {
		output = &user
	}
	c.doJSON(t, http.MethodGet, "/api/v1/me", nil, output, expectedStatus)
	return user
}

func (c *managementClient) createUser(t *testing.T, email, username, password string) openapi.User {
	t.Helper()
	var created openapi.CreateUserResponse
	c.doJSON(t, http.MethodPost, "/api/v1/users", openapi.CreateUserRequest{
		Email: openapi_types.Email(email), Username: username,
	}, &created, http.StatusCreated)
	registrationURL, err := url.Parse(created.RegistrationLink.Url)
	if err != nil {
		t.Fatalf("parse registration link: %v", err)
	}
	registrationToken, err := url.ParseQuery(registrationURL.Fragment)
	if err != nil {
		t.Fatalf("parse registration token: %v", err)
	}
	registrationClient := newManagementClient(t, c.baseURL)
	registrationClient.doJSON(t, http.MethodPost, "/api/v1/password-resets", openapi.CompletePasswordResetRequest{
		Token: registrationToken.Get("token"), NewPassword: password,
	}, nil, http.StatusNoContent)
	return created.User
}

func (c *managementClient) disableUser(t *testing.T, userID string) {
	t.Helper()
	c.doJSON(t, http.MethodDelete, "/api/v1/users/"+url.PathEscape(userID), nil, nil, http.StatusNoContent)
}

func (c *managementClient) createProject(t *testing.T, name, slug string) openapi.Project {
	t.Helper()
	var project openapi.Project
	c.doJSON(t, http.MethodPost, "/api/v1/projects", openapi.CreateProjectRequest{
		Name: name, Slug: slug,
	}, &project, http.StatusCreated)
	return project
}

func (c *managementClient) project(t *testing.T, slug string) openapi.Project {
	t.Helper()
	var project openapi.Project
	c.doJSON(t, http.MethodGet, "/api/v1/projects/"+url.PathEscape(slug), nil, &project, http.StatusOK)
	return project
}

func (c *managementClient) createServicePrincipal(t *testing.T, name, username string) servicePrincipal {
	t.Helper()
	var account openapi.ServiceAccount
	c.doJSON(t, http.MethodPost, "/api/v1/service-accounts", openapi.CreateServiceAccountRequest{
		Name: name, Username: username,
	}, &account, http.StatusCreated)
	principal := servicePrincipal{id: account.Id.String(), username: account.Username}
	principal.tokenID, principal.secret = c.createAccessKey(t, principal.id, "registry-e2e")
	return principal
}

func (c *managementClient) createAccessKey(t *testing.T, accountID, name string) (string, string) {
	t.Helper()
	var created openapi.CreatedToken
	c.doJSON(t, http.MethodPost, "/api/v1/service-accounts/"+url.PathEscape(accountID)+"/tokens",
		openapi.CreateServiceAccountTokenRequest{Name: name}, &created, http.StatusCreated)
	if created.Secret == "" {
		t.Fatal("management API returned an empty reveal-once access key")
	}
	return created.Token.Id.String(), created.Secret
}

func (c *managementClient) setMembership(t *testing.T, project string, principal servicePrincipal, role openapi.ProjectRole) {
	t.Helper()
	path := fmt.Sprintf("/api/v1/projects/%s/members/service_account/%s",
		url.PathEscape(project), url.PathEscape(principal.id))
	c.doJSON(t, http.MethodPut, path, openapi.SetMembershipRequest{Role: role}, nil, http.StatusOK)
}

func (c *managementClient) deleteMembership(t *testing.T, project string, principal servicePrincipal) {
	t.Helper()
	path := fmt.Sprintf("/api/v1/projects/%s/members/service_account/%s",
		url.PathEscape(project), url.PathEscape(principal.id))
	c.doJSON(t, http.MethodDelete, path, nil, nil, http.StatusNoContent)
}

func (c *managementClient) revokeAccessKey(t *testing.T, principal servicePrincipal) {
	t.Helper()
	path := fmt.Sprintf("/api/v1/service-accounts/%s/tokens/%s",
		url.PathEscape(principal.id), url.PathEscape(principal.tokenID))
	c.doJSON(t, http.MethodDelete, path, nil, nil, http.StatusNoContent)
}

func (c *managementClient) createRepository(
	t *testing.T,
	project, name string,
	policies []openapi.RepositoryPolicyInput,
) openapi.Repository {
	t.Helper()
	var repository openapi.Repository
	c.doJSON(t, http.MethodPost, "/api/v1/projects/"+url.PathEscape(project)+"/repositories",
		openapi.CreateRepositoryRequest{Name: name, Policies: policies},
		&repository, http.StatusCreated)
	return repository
}

func (c *managementClient) repositories(t *testing.T, project string) []openapi.Repository {
	t.Helper()
	var response json.RawMessage
	c.doJSON(t, http.MethodGet, "/api/v1/projects/"+url.PathEscape(project)+"/repositories",
		nil, &response, http.StatusOK)
	var page openapi.RepositoryPage
	if len(response) > 0 && response[0] == '[' {
		var repositories []openapi.Repository
		if err := json.Unmarshal(response, &repositories); err != nil {
			t.Fatalf("decode legacy repositories: %v", err)
		}
		return repositories
	}
	if err := json.Unmarshal(response, &page); err != nil {
		t.Fatalf("decode repositories page: %v", err)
	}
	return page.Items
}

func (c *managementClient) tags(t *testing.T, project, repository string) openapi.TagPage {
	t.Helper()
	var response json.RawMessage
	path := "/api/v1/projects/" + url.PathEscape(project) + "/repository-tags?repository=" +
		url.QueryEscape(repository)
	c.doJSON(t, http.MethodGet, path, nil, &response, http.StatusOK)
	var tags openapi.TagPage
	if err := json.Unmarshal(response, &tags); err != nil {
		t.Fatalf("decode tags page: %v", err)
	}
	if len(tags.Items) == 0 {
		var legacy struct {
			Tags []string `json:"tags"`
		}
		if err := json.Unmarshal(response, &legacy); err != nil {
			t.Fatalf("decode legacy tags: %v", err)
		}
		tags.Items = legacy.Tags
	}
	return tags
}

func (c *managementClient) inventory(t *testing.T, project, repository string) []openapi.ManifestInventory {
	t.Helper()
	var response json.RawMessage
	path := "/api/v1/projects/" + url.PathEscape(project) + "/repository-inventory?repository=" +
		url.QueryEscape(repository)
	c.doJSON(t, http.MethodGet, path, nil, &response, http.StatusOK)
	if len(response) > 0 && response[0] == '[' {
		var inventory []openapi.ManifestInventory
		if err := json.Unmarshal(response, &inventory); err != nil {
			t.Fatalf("decode legacy inventory: %v", err)
		}
		return inventory
	}
	var inventory openapi.ManifestInventoryPage
	if err := json.Unmarshal(response, &inventory); err != nil {
		t.Fatalf("decode inventory page: %v", err)
	}
	return inventory.Items
}

func (c *managementClient) reconcileInventory(t *testing.T, project, repository string) {
	t.Helper()
	c.doJSON(t, http.MethodPost,
		"/api/v1/projects/"+url.PathEscape(project)+"/repository-inventory-reconciliations",
		openapi.RepositorySelection{Repository: repository}, nil, http.StatusOK)
}

func (c *managementClient) deleteArtifact(t *testing.T, project, repository, reference string) {
	t.Helper()
	request := openapi.ArtifactDeletionRequest{Repository: repository, Reference: reference}
	var preview openapi.ArtifactDeletionPreview
	c.doJSON(t, http.MethodPost, "/api/v1/projects/"+url.PathEscape(project)+"/artifact-deletion-previews", request, &preview, http.StatusOK)
	request.ExpectedDigest = &preview.Digest
	request.ExpectedTags = &preview.AffectedTags
	request.ExpectedChildDigests = &preview.ChildDigests
	var deletion openapi.ArtifactDeletion
	c.doJSON(t, http.MethodPost, "/api/v1/projects/"+url.PathEscape(project)+"/artifact-deletions", request, &deletion, http.StatusOK)
	if deletion.Status != openapi.ArtifactDeletionStatusCompleted {
		t.Fatalf("artifact deletion returned %q with message %v", deletion.Status, deletion.Message)
	}
}

func (c *managementClient) garbageCollect(t *testing.T) openapi.GarbageCollection {
	t.Helper()
	var result openapi.GarbageCollection
	c.doJSON(t, http.MethodPost, "/api/v1/garbage-collections", nil, &result, http.StatusOK)
	return result
}

func (c *managementClient) exchangeToken(
	ctx context.Context,
	username, secret, repository string,
) (openapi.RegistryToken, int, error) {
	query := url.Values{
		"service": {"grom-registry"},
		"scope":   {"repository:" + repository + ":pull,push"},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/auth/token?"+query.Encode(), nil)
	if err != nil {
		return openapi.RegistryToken{}, 0, err
	}
	request.SetBasicAuth(username, secret)
	response, err := c.client.Do(request)
	if err != nil {
		return openapi.RegistryToken{}, 0, err
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxDiagnostic+1))
	if err != nil {
		return openapi.RegistryToken{}, response.StatusCode, err
	}
	if response.StatusCode != http.StatusOK {
		var apiError openapi.Error
		_ = json.Unmarshal(body, &apiError)
		return openapi.RegistryToken{}, response.StatusCode,
			fmt.Errorf("registry token exchange returned %d (%s)", response.StatusCode, apiError.Code)
	}
	var token openapi.RegistryToken
	if err := json.Unmarshal(body, &token); err != nil {
		return openapi.RegistryToken{}, response.StatusCode, fmt.Errorf("decode registry token response: %w", err)
	}
	return token, response.StatusCode, nil
}

func (c *managementClient) manifest(ctx context.Context, bearer, repository, reference string) (int, error) {
	target := fmt.Sprintf("%s/v2/%s/manifests/%s", c.baseURL, repository, reference)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", "Bearer "+bearer)
	request.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", "))
	response, err := c.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxDiagnostic))
	return response.StatusCode, nil
}

func (c *managementClient) doJSON(
	t *testing.T,
	method, path string,
	input, output any,
	expectedStatus int,
) {
	t.Helper()
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			t.Fatalf("%s %s: encode request: %v", method, path, err)
		}
		body = bytes.NewReader(encoded)
	}
	ctx, cancel := context.WithTimeout(context.Background(), managementRequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		t.Fatalf("%s %s: create request: %v", method, path, err)
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet && method != http.MethodHead {
		request.Header.Set("Origin", c.baseURL)
	}
	response, err := c.client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: request failed: %v", method, path, err)
	}
	defer func() { _ = response.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxDiagnostic+1))
	if err != nil {
		t.Fatalf("%s %s: read response: %v", method, path, err)
	}
	if response.StatusCode != expectedStatus {
		var apiError openapi.Error
		_ = json.Unmarshal(responseBody, &apiError)
		t.Fatalf("%s %s: got HTTP %d, expected %d (code=%q message=%q)",
			method, path, response.StatusCode, expectedStatus, apiError.Code, apiError.Message)
	}
	if output != nil {
		if err := json.Unmarshal(responseBody, output); err != nil {
			// Do not include response bodies here: successful access-key creation
			// responses contain a reveal-once secret.
			t.Fatalf("%s %s: decode successful response: %v", method, path, err)
		}
	}
}
