package distribution

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	registryapp "github.com/jfxdev/grom/backend/internal/registry/application"
)

type Client struct {
	baseURL *url.URL
	http    *http.Client
	tokens  *registryapp.TokenService
}

type RepositoryList struct {
	Repositories []string `json:"repositories"`
}

type TagList struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type TagPage struct {
	Name       string
	Tags       []string
	NextMarker string
}

type ProjectRepositoryPage struct {
	Repositories []string
	NextMarker   string
}

type manifestDocument struct {
	MediaType    string `json:"mediaType"`
	ArtifactType string `json:"artifactType"`
	Config       struct {
		MediaType string `json:"mediaType"`
	} `json:"config"`
	Layers []struct {
		MediaType string `json:"mediaType"`
	} `json:"layers"`
	Manifests []struct {
		MediaType string `json:"mediaType"`
	} `json:"manifests"`
	Subject *struct {
		Digest string `json:"digest"`
	} `json:"subject"`
}

type referrersResponse struct {
	Manifests []struct {
		Digest       string `json:"digest"`
		MediaType    string `json:"mediaType"`
		ArtifactType string `json:"artifactType"`
		Size         int64  `json:"size"`
	} `json:"manifests"`
}

func NewClient(rawURL string, tokens *registryapp.TokenService) (*Client, error) {
	baseURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 15 * time.Second},
		tokens:  tokens,
	}, nil
}

func (c *Client) Available(ctx context.Context) bool {
	endpoint := c.baseURL.ResolveReference(&url.URL{Path: "/v2/"})
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return false
	}
	response, err := c.http.Do(request)
	if err != nil {
		return false
	}
	defer func() { _ = response.Body.Close() }()
	return response.StatusCode == http.StatusOK || response.StatusCode == http.StatusUnauthorized
}

func (c *Client) ListProjectRepositories(ctx context.Context, project string) ([]string, error) {
	prefix := project + "/"
	result := make([]string, 0)
	nextPath := "/v2/_catalog?n=1000"
	for nextPath != "" {
		token, err := c.tokens.IssueCatalog("grom-internal")
		if err != nil {
			return nil, err
		}
		var response RepositoryList
		nextPath, err = c.getWithLink(ctx, nextPath, token, &response)
		if err != nil {
			return nil, err
		}
		for _, name := range response.Repositories {
			if strings.HasPrefix(name, prefix) {
				result = append(result, strings.TrimPrefix(name, prefix))
			}
		}
	}
	return result, nil
}

// ListProjectRepositoriesPage keeps Distribution's catalog Link private while
// exposing only a project-relative continuation marker to the control plane.
func (c *Client) ListProjectRepositoriesPage(ctx context.Context, project string, limit int, marker string) (*ProjectRepositoryPage, error) {
	if limit < 1 {
		return nil, fmt.Errorf("catalog page limit must be positive")
	}
	page := &ProjectRepositoryPage{}
	prefix := project + "/"
	for {
		token, err := c.tokens.IssueCatalog("grom-internal")
		if err != nil {
			return nil, err
		}
		query := url.Values{"n": []string{fmt.Sprintf("%d", limit)}}
		if marker != "" {
			query.Set("last", marker)
		}
		var response RepositoryList
		nextPath, err := c.getWithLink(ctx, "/v2/_catalog?"+query.Encode(), token, &response)
		if err != nil {
			return nil, err
		}
		page.Repositories = page.Repositories[:0]
		for _, name := range response.Repositories {
			if strings.HasPrefix(name, prefix) {
				page.Repositories = append(page.Repositories, strings.TrimPrefix(name, prefix))
			}
		}
		page.NextMarker = ""
		if nextPath != "" {
			nextURL, parseErr := url.Parse(nextPath)
			if parseErr != nil {
				return nil, parseErr
			}
			page.NextMarker = nextURL.Query().Get("last")
			if page.NextMarker == "" {
				return nil, fmt.Errorf("distribution pagination link has no catalog marker")
			}
		}
		if len(page.Repositories) > 0 || page.NextMarker == "" {
			return page, nil
		}
		marker = page.NextMarker
	}
}

func (c *Client) ListTags(ctx context.Context, repository string) (*TagList, error) {
	var response TagList
	token, err := c.tokens.IssueInternal("grom-internal", []registryapp.Access{{
		Type: "repository", Name: repository, Actions: []string{"pull"},
	}})
	if err != nil {
		return nil, err
	}
	if err := c.get(ctx, "/v2/"+repository+"/tags/list", token, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// ListTagsPage follows Distribution's native lexical tag pagination but
// returns only its marker, never its private Link header or URL.
func (c *Client) ListTagsPage(ctx context.Context, repository string, limit int, marker string) (*TagPage, error) {
	var response TagList
	token, err := c.tokens.IssueInternal("grom-internal", []registryapp.Access{{Type: "repository", Name: repository, Actions: []string{"pull"}}})
	if err != nil {
		return nil, err
	}
	query := url.Values{"n": []string{fmt.Sprintf("%d", limit)}}
	if marker != "" {
		query.Set("last", marker)
	}
	nextPath, err := c.getWithLink(ctx, "/v2/"+repository+"/tags/list?"+query.Encode(), token, &response)
	if err != nil {
		return nil, err
	}
	page := &TagPage{Name: response.Name, Tags: response.Tags}
	if nextPath != "" {
		nextURL, parseErr := url.Parse(nextPath)
		if parseErr != nil {
			return nil, parseErr
		}
		page.NextMarker = nextURL.Query().Get("last")
		if page.NextMarker == "" {
			return nil, fmt.Errorf("distribution pagination link has no tag marker")
		}
	}
	return page, nil
}

func (c *Client) ListRepositoryTags(ctx context.Context, repository string) ([]string, error) {
	tags, err := c.ListTags(ctx, repository)
	if err != nil {
		return nil, err
	}
	return tags.Tags, nil
}

func (c *Client) FetchManifest(ctx context.Context, repository, reference string) (*registryapp.ManifestMetadata, error) {
	token, err := c.tokens.IssueInternal("grom-internal", []registryapp.Access{{
		Type: "repository", Name: repository, Actions: []string{"pull"},
	}})
	if err != nil {
		return nil, err
	}
	targetURL := c.baseURL.ResolveReference(&url.URL{Path: "/v2/" + repository + "/manifests/" + reference})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", manifestAccept)
	response, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("distribution returned %s", response.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var document manifestDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	digest := response.Header.Get("Docker-Content-Digest")
	if digest == "" {
		digest, _, err = c.ResolveManifest(ctx, repository, reference)
		if err != nil {
			return nil, err
		}
	}
	subject := ""
	if document.Subject != nil {
		subject = document.Subject.Digest
	}
	layerMediaTypes := make([]string, 0, len(document.Layers))
	for _, layer := range document.Layers {
		layerMediaTypes = append(layerMediaTypes, layer.MediaType)
	}
	descriptorMediaTypes := make([]string, 0, len(document.Manifests))
	for _, descriptor := range document.Manifests {
		descriptorMediaTypes = append(descriptorMediaTypes, descriptor.MediaType)
	}
	mediaType := document.MediaType
	if mediaType == "" {
		mediaType = response.Header.Get("Content-Type")
	}
	return &registryapp.ManifestMetadata{
		Digest: digest, MediaType: mediaType, ArtifactType: document.ArtifactType,
		SubjectDigest: subject, ManifestSize: int64(len(raw)),
		ConfigMediaType: document.Config.MediaType, LayerMediaTypes: layerMediaTypes,
		DescriptorMediaTypes: descriptorMediaTypes,
	}, nil
}

func (c *Client) ListReferrers(ctx context.Context, repository, digest string) ([]registryapp.ManifestDescriptor, error) {
	token, err := c.tokens.IssueInternal("grom-internal", []registryapp.Access{{
		Type: "repository", Name: repository, Actions: []string{"pull"},
	}})
	if err != nil {
		return nil, err
	}
	targetURL := c.baseURL.ResolveReference(&url.URL{Path: "/v2/" + repository + "/referrers/" + digest})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json")
	response, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotFound {
		return []registryapp.ManifestDescriptor{}, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("distribution returned %s", response.Status)
	}
	var payload referrersResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	result := make([]registryapp.ManifestDescriptor, 0, len(payload.Manifests))
	for _, descriptor := range payload.Manifests {
		result = append(result, registryapp.ManifestDescriptor{
			Digest: descriptor.Digest, MediaType: descriptor.MediaType,
			ArtifactType: descriptor.ArtifactType, Size: descriptor.Size,
		})
	}
	return result, nil
}

func (c *Client) RepositoryExists(ctx context.Context, repository string) (bool, error) {
	token, err := c.tokens.IssueInternal("grom-internal", []registryapp.Access{{
		Type: "repository", Name: repository, Actions: []string{"pull"},
	}})
	if err != nil {
		return false, err
	}
	targetURL := c.baseURL.ResolveReference(&url.URL{Path: "/v2/" + repository + "/tags/list"})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = response.Body.Close() }()
	switch response.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("distribution returned %s", response.Status)
	}
}

func (c *Client) ManifestExists(ctx context.Context, repository, reference string) (bool, error) {
	_, exists, err := c.ResolveManifest(ctx, repository, reference)
	return exists, err
}

func (c *Client) ResolveManifest(ctx context.Context, repository, reference string) (string, bool, error) {
	token, err := c.tokens.IssueInternal("grom-internal", []registryapp.Access{{
		Type: "repository", Name: repository, Actions: []string{"pull"},
	}})
	if err != nil {
		return "", false, err
	}
	targetURL := c.baseURL.ResolveReference(&url.URL{Path: "/v2/" + repository + "/manifests/" + reference})
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, targetURL.String(), nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", manifestAccept)
	response, err := c.http.Do(req)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = response.Body.Close() }()
	switch response.StatusCode {
	case http.StatusOK:
		digest := response.Header.Get("Docker-Content-Digest")
		if digest == "" {
			return "", false, fmt.Errorf("distribution omitted manifest digest")
		}
		return digest, true, nil
	case http.StatusNotFound:
		return "", false, nil
	default:
		return "", false, fmt.Errorf("distribution returned %s", response.Status)
	}
}

var manifestAccept = strings.Join([]string{
	"application/vnd.oci.image.index.v1+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.oci.artifact.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.docker.distribution.manifest.v2+json",
}, ", ")

func (c *Client) DeleteManifest(ctx context.Context, repository, digest string) error {
	token, err := c.tokens.IssueInternal("grom-internal", []registryapp.Access{{
		Type: "repository", Name: repository, Actions: []string{"delete"},
	}})
	if err != nil {
		return err
	}
	targetURL := c.baseURL.ResolveReference(&url.URL{Path: "/v2/" + repository + "/manifests/" + digest})
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, targetURL.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusAccepted && response.StatusCode != http.StatusNotFound {
		return fmt.Errorf("distribution returned %s", response.Status)
	}
	return nil
}

func (c *Client) get(ctx context.Context, path, token string, target any) error {
	_, err := c.getWithLink(ctx, path, token, target)
	return err
}

func (c *Client) getWithLink(ctx context.Context, path, token string, target any) (string, error) {
	reference, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	targetURL := c.baseURL.ResolveReference(reference)
	if targetURL.Scheme != c.baseURL.Scheme || targetURL.Host != c.baseURL.Host {
		return "", fmt.Errorf("distribution pagination link points to a different origin")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("distribution returned %s", response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return "", err
	}
	nextPath, err := nextLinkPath(response.Header.Get("Link"))
	if err != nil {
		return "", err
	}
	return nextPath, nil
}

func nextLinkPath(header string) (string, error) {
	for _, rawLink := range strings.Split(header, ",") {
		parts := strings.Split(rawLink, ";")
		if len(parts) < 2 {
			continue
		}
		isNext := false
		for _, parameter := range parts[1:] {
			if strings.TrimSpace(parameter) == `rel="next"` {
				isNext = true
				break
			}
		}
		if !isNext {
			continue
		}
		rawTarget := strings.TrimSpace(parts[0])
		if len(rawTarget) < 2 || rawTarget[0] != '<' || rawTarget[len(rawTarget)-1] != '>' {
			return "", fmt.Errorf("distribution returned an invalid pagination link")
		}
		target, err := url.Parse(rawTarget[1 : len(rawTarget)-1])
		if err != nil {
			return "", fmt.Errorf("parse distribution pagination link: %w", err)
		}
		return target.String(), nil
	}
	return "", nil
}
